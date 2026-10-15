package txutil

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// fakeNonceClient simulates an RPC node. The pending counter advances every
// time Acquire is called and the caller signals "broadcast" by invoking
// MarkBroadcast — this matches the real-world contract that PendingNonceAt
// returns count = (last-known broadcast nonce + 1).
type fakeNonceClient struct {
	mu        sync.Mutex
	pending   uint64
	calls     atomic.Int64
	err       error
	errOnce   bool
	panicVal  any
	waitOn    chan struct{} // closed when PendingNonceAt is entered
	releaseOn chan struct{} // PendingNonceAt blocks until this is closed
}

func (f *fakeNonceClient) PendingNonceAt(ctx context.Context, _ common.Address) (uint64, error) {
	f.calls.Add(1)
	f.mu.Lock()
	wait, release := f.waitOn, f.releaseOn
	if wait != nil {
		f.waitOn = nil
		f.releaseOn = nil
	}
	if f.err != nil {
		e := f.err
		if f.errOnce {
			f.err = nil
		}
		f.mu.Unlock()
		return 0, e
	}
	if f.panicVal != nil {
		p := f.panicVal
		f.mu.Unlock()
		panic(p)
	}
	n := f.pending
	f.mu.Unlock()
	if wait != nil {
		close(wait)
		select {
		case <-release:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	return n, nil
}

// markBroadcast simulates a successful tx broadcast that bumps the pending
// counter on the node.
func (f *fakeNonceClient) markBroadcast() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending++
}

func TestNonceManager_AcquireReturnsPendingNonce(t *testing.T) {
	client := &fakeNonceClient{pending: 42}
	nm := NewNonceManager(client, common.Address{})

	got, release, err := nm.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if got != 42 {
		t.Fatalf("got %d want 42", got)
	}
	if c := client.calls.Load(); c != 1 {
		t.Fatalf("expected 1 RPC call, got %d", c)
	}
}

func TestNonceManager_SequentialAcquireRefetchesEachTime(t *testing.T) {
	client := &fakeNonceClient{pending: 100}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()

	for i, want := range []uint64{100, 101, 102} {
		got, release, err := nm.Acquire(ctx)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("iter %d: got %d want %d", i, got, want)
		}
		// Simulate broadcast: node-side pending advances.
		client.markBroadcast()
		release()
	}
	if c := client.calls.Load(); c != 3 {
		t.Fatalf("expected 3 RPC calls (one per Acquire), got %d", c)
	}
}

func TestNonceManager_ConcurrentAcquireSerializesAndProducesUniqueNonces(t *testing.T) {
	client := &fakeNonceClient{pending: 1000}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()

	const N = 50
	var wg sync.WaitGroup
	out := make(chan uint64, N)
	for range N {
		wg.Go(func() {
			n, release, err := nm.Acquire(ctx)
			if err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			// Simulate broadcast inside the critical section.
			client.markBroadcast()
			out <- n
			release()
		})
	}
	wg.Wait()
	close(out)

	seen := make(map[uint64]bool)
	for n := range out {
		if seen[n] {
			t.Fatalf("duplicate nonce %d", n)
		}
		seen[n] = true
	}
	if len(seen) != N {
		t.Fatalf("expected %d unique nonces, got %d", N, len(seen))
	}
	if c := client.calls.Load(); c != N {
		t.Fatalf("expected %d RPC calls, got %d", N, c)
	}
}

func TestNonceManager_ReleaseIsIdempotent(t *testing.T) {
	client := &fakeNonceClient{pending: 5}
	nm := NewNonceManager(client, common.Address{})
	_, release, err := nm.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
	release() // must not panic / re-unlock.
}

func TestNonceManager_AcquireRPCErrorReleasesLock(t *testing.T) {
	boom := errors.New("rpc down")
	client := &fakeNonceClient{err: boom}
	nm := NewNonceManager(client, common.Address{})

	_, release, err := nm.Acquire(context.Background())
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped rpc err, got %v", err)
	}
	release() // error-path release must remain callable.
	// Lock must be released even though Acquire failed.
	done := make(chan struct{})
	go func() {
		client.mu.Lock()
		client.err = nil
		client.pending = 1
		client.mu.Unlock()
		_, release, err := nm.Acquire(context.Background())
		if err != nil {
			t.Errorf("second Acquire failed: %v", err)
			return
		}
		release()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Acquire after RPC error blocked — lock not released")
	}
}

func TestNonceManager_AcquirePanicReleasesLock(t *testing.T) {
	boom := errors.New("rpc panic")
	client := &fakeNonceClient{panicVal: boom}
	nm := NewNonceManager(client, common.Address{})

	func() {
		defer func() {
			gotErr, ok := recover().(error)
			if !ok || !errors.Is(gotErr, boom) {
				t.Fatalf("expected panic %v, got %v", boom, gotErr)
			}
		}()
		_, _, _ = nm.Acquire(context.Background())
		t.Fatal("expected panic from PendingNonceAt")
	}()

	done := make(chan struct{})
	go func() {
		client.mu.Lock()
		client.panicVal = nil
		client.pending = 1
		client.mu.Unlock()
		_, release, err := nm.Acquire(context.Background())
		if err != nil {
			t.Errorf("second Acquire failed: %v", err)
			return
		}
		release()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Acquire after panic blocked — lock not released")
	}
}

func TestNonceManager_AcquireCanceledBeforeAcquiring(t *testing.T) {
	client := &fakeNonceClient{pending: 7}
	nm := NewNonceManager(client, common.Address{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, release, err := nm.Acquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire error = %v, want cancellation", err)
	}
	release()
	if calls := client.calls.Load(); calls != 0 {
		t.Fatalf("canceled Acquire made %d RPC calls", calls)
	}
}

func TestNonceManager_QueuedCancellationLeavesLockUsable(t *testing.T) {
	client := &fakeNonceClient{pending: 7}
	nm := NewNonceManager(client, common.Address{})

	_, release1, err := nm.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release1()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waiting := &nonceWaitContext{Context: ctx, waiting: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		_, release, err := nm.Acquire(waiting)
		release()
		done <- err
	}()
	<-waiting.waiting
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("queued Acquire error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("queued Acquire did not stop while the first caller held the lock")
	}
	if calls := client.calls.Load(); calls != 1 {
		t.Fatalf("queued canceled Acquire reached RPC: %d calls, want 1", calls)
	}

	client.mu.Lock()
	client.pending = 8
	client.mu.Unlock()
	release1()

	n, release, err := nm.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if n != 8 {
		t.Fatalf("Acquire after cancellation = %d, want 8", n)
	}
}

type nonceWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *nonceWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

func TestNonceManager_CancellationDuringRPCReleasesLock(t *testing.T) {
	entered := make(chan struct{})
	client := &fakeNonceClient{pending: 7, waitOn: entered, releaseOn: make(chan struct{})}
	nm := NewNonceManager(client, common.Address{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, release, err := nm.Acquire(ctx)
		release()
		done <- err
	}()
	<-entered
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Acquire error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Acquire RPC did not stop after cancellation")
	}
	_, release, err := nm.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

type nonceProviderFunc func(context.Context, common.Address) (uint64, error)

func (f nonceProviderFunc) PendingNonceAt(ctx context.Context, address common.Address) (uint64, error) {
	return f(ctx, address)
}

func TestNonceManager_CancellationBeforeRPCSuccessReleasesLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := nonceProviderFunc(func(context.Context, common.Address) (uint64, error) {
		cancel()
		return 7, nil
	})
	nm := NewNonceManager(client, common.Address{})
	_, release, err := nm.Acquire(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Acquire error = %v, want cancellation", err)
	}
	release()
	_, release, err = nm.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}
