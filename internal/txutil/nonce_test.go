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

type fakeNonceClient struct {
	mu      sync.Mutex
	nonce   uint64
	calls   atomic.Int64
	err     error
	errOnce bool
	waitCh  chan struct{}
	contCh  chan struct{}
}

func (f *fakeNonceClient) PendingNonceAt(ctx context.Context, _ common.Address) (uint64, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		e := f.err
		if f.errOnce {
			f.err = nil
		}
		return 0, e
	}
	if f.waitCh != nil {
		close(f.waitCh)
		<-f.contCh
		f.waitCh = nil
		f.contCh = nil
	}
	return f.nonce, nil
}

func TestNonceManager_SequentialGet(t *testing.T) {
	client := &fakeNonceClient{nonce: 42}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()

	for i, want := range []uint64{42, 43, 44} {
		got, err := nm.Get(ctx)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if got != want {
			t.Fatalf("iter %d: got %d want %d", i, got, want)
		}
	}
	if n := client.calls.Load(); n != 1 {
		t.Fatalf("expected 1 network call, got %d", n)
	}
	if nm.PendingCount() != 3 {
		t.Fatalf("expected 3 pending, got %d", nm.PendingCount())
	}
}

func TestNonceManager_ConcurrentGetUnique(t *testing.T) {
	client := &fakeNonceClient{nonce: 1000}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()

	const N = 200
	var wg sync.WaitGroup
	out := make(chan uint64, N)
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := nm.Get(ctx)
			if err != nil {
				t.Errorf("get: %v", err)
				return
			}
			out <- n
		}()
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
}

func TestNonceManager_MarkFailedRefreshes(t *testing.T) {
	client := &fakeNonceClient{nonce: 10}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()

	n1, err := nm.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 10 {
		t.Fatalf("got %d", n1)
	}
	// Simulate local failure; network nonce moved independently.
	client.mu.Lock()
	client.nonce = 20
	client.mu.Unlock()
	nm.MarkFailed(n1)

	n2, err := nm.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 20 {
		t.Fatalf("expected refresh to 20, got %d", n2)
	}
	if client.calls.Load() != 2 {
		t.Fatalf("expected 2 fetches, got %d", client.calls.Load())
	}
	if nm.PendingCount() != 1 {
		t.Fatalf("expected 1 pending after mark-failed+get, got %d", nm.PendingCount())
	}
}

func TestNonceManager_MarkFailedDoesNotReuseLowerNonce(t *testing.T) {
	client := &fakeNonceClient{nonce: 10}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()

	n1, err := nm.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	n2, err := nm.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n1 != 10 || n2 != 11 {
		t.Fatalf("got %d,%d", n1, n2)
	}

	client.mu.Lock()
	client.nonce = 10
	client.mu.Unlock()
	nm.MarkFailed(n1)

	n3, err := nm.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n3 != 12 {
		t.Fatalf("expected next nonce 12, got %d", n3)
	}
}

func TestNonceManager_MarkConfirmed(t *testing.T) {
	client := &fakeNonceClient{nonce: 5}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()
	n, _ := nm.Get(ctx)
	nm.MarkConfirmed(n)
	if nm.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", nm.PendingCount())
	}
	// Double-confirm is a no-op.
	nm.MarkConfirmed(n)
}

func TestNonceManager_MarkConfirmedDoesNotBlockDuringInitialFetch(t *testing.T) {
	client := &fakeNonceClient{nonce: 5, waitCh: make(chan struct{}), contCh: make(chan struct{})}
	nm := NewNonceManager(client, common.Address{})

	getErrCh := make(chan error, 1)
	go func() {
		_, err := nm.Get(context.Background())
		getErrCh <- err
	}()

	<-client.waitCh
	done := make(chan struct{})
	go func() {
		nm.MarkConfirmed(999)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(20 * time.Millisecond):
		t.Fatal("MarkConfirmed blocked on initial PendingNonceAt fetch")
	}

	close(client.contCh)
	if err := <-getErrCh; err != nil {
		t.Fatal(err)
	}
}

func TestNonceManager_Reset(t *testing.T) {
	client := &fakeNonceClient{nonce: 100}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()
	_, _ = nm.Get(ctx)
	_, _ = nm.Get(ctx)
	client.mu.Lock()
	client.nonce = 500
	client.mu.Unlock()
	if err := nm.Reset(ctx); err != nil {
		t.Fatal(err)
	}
	if nm.PendingCount() != 0 {
		t.Fatalf("pending not cleared: %d", nm.PendingCount())
	}
	n, _ := nm.Get(ctx)
	if n != 500 {
		t.Fatalf("expected 500, got %d", n)
	}
}

func TestNonceManager_ResetBlocksConcurrentGetUntilRefreshCompletes(t *testing.T) {
	client := &fakeNonceClient{nonce: 100}
	nm := NewNonceManager(client, common.Address{})
	ctx := context.Background()

	n0, err := nm.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n0 != 100 {
		t.Fatalf("expected 100, got %d", n0)
	}

	client.mu.Lock()
	client.nonce = 101
	client.waitCh = make(chan struct{})
	client.contCh = make(chan struct{})
	client.mu.Unlock()

	errCh := make(chan error, 1)
	go func() {
		errCh <- nm.Reset(ctx)
	}()

	<-client.waitCh
	getCh := make(chan uint64, 1)
	getErrCh := make(chan error, 1)
	go func() {
		n, err := nm.Get(ctx)
		if err != nil {
			getErrCh <- err
			return
		}
		getCh <- n
	}()

	select {
	case n := <-getCh:
		t.Fatalf("Get returned %d before Reset completed", n)
	case err := <-getErrCh:
		t.Fatalf("Get failed before Reset completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	close(client.contCh)
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-getErrCh:
		t.Fatal(err)
	case n := <-getCh:
		if n != 101 {
			t.Fatalf("expected nonce 101 after Reset, got %d", n)
		}
	case <-time.After(time.Second):
		t.Fatal("Get did not return after Reset completed")
	}
}

func TestNonceManager_GetFetchError(t *testing.T) {
	boom := errors.New("rpc down")
	client := &fakeNonceClient{err: boom}
	nm := NewNonceManager(client, common.Address{})
	_, err := nm.Get(context.Background())
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("expected wrapped rpc err, got %v", err)
	}
}

func TestNonceManager_CancelInitLocked_NilCh(t *testing.T) {
	// Exercise cancelInitLocked when initCh is nil — should be a no-op, not panic.
	client := &fakeNonceClient{nonce: 1}
	nm := NewNonceManager(client, common.Address{})
	// Directly call MarkFailed when initCh is nil (no concurrent Get in progress).
	// MarkFailed calls cancelInitLocked internally.
	n, err := nm.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// At this point initCh is nil. MarkFailed calls cancelInitLocked.
	nm.MarkFailed(n)
	// Verify the manager still works after the nil-initCh cancel.
	client.mu.Lock()
	client.nonce = 5
	client.mu.Unlock()
	n2, err := nm.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n2 != 5 {
		t.Fatalf("expected 5, got %d", n2)
	}
}
