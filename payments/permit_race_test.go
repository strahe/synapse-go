package payments

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/txutil"
)

func seedPermitInputs(t *testing.T, mb *mockBackend) {
	t.Helper()
	for _, m := range []struct {
		name string
		v    any
	}{
		{"name", "USDFC"},
		{"version", "1"},
		{"nonces", big.NewInt(0)},
	} {
		mth, ok := permitERC20ABI.Methods[m.name]
		if !ok {
			t.Fatalf("permit ABI missing %s", m.name)
		}
		b, err := mth.Outputs.Pack(m.v)
		if err != nil {
			t.Fatalf("pack %s: %v", m.name, err)
		}
		mb.replies[tokenAddr.Hex()+":"+m.name] = b
	}
}

func sentCount(mb *mockBackend) int {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	return len(mb.sent)
}

func waitForSentCount(t *testing.T, mb *mockBackend, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if got := sentCount(mb); got >= want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("sent count did not reach %d, got %d", want, sentCount(mb))
		case <-ticker.C:
		}
	}
}

func assertPermitWriteStillBlocked(t *testing.T, mb *mockBackend, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("permit write completed before prior receipt: %v", err)
	case <-time.After(75 * time.Millisecond):
	}
	if got := sentCount(mb); got != 1 {
		t.Fatalf("sent count while permit lock should be held = %d, want 1", got)
	}
}

func waitPermitWriteDone(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("permit write: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("permit write did not complete")
	}
}

func successfulReceipt() *types.Receipt {
	return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(5)}
}

func depositWithPermitForRaceTest(ctx context.Context, s *Service, opts ...WriteOption) error {
	_, err := s.DepositWithPermit(
		ctx,
		tokenAddr,
		common.Address{},
		big.NewInt(1_000),
		nil,
		append([]WriteOption{WithSkipPrecheck()}, opts...)...,
	)
	return err
}

func permitDeadlineFromSent(t *testing.T, mb *mockBackend, index int) *big.Int {
	t.Helper()
	mb.mu.Lock()
	if index >= len(mb.sent) {
		mb.mu.Unlock()
		t.Fatalf("sent index %d out of range, sent=%d", index, len(mb.sent))
	}
	tx := mb.sent[index]
	mb.mu.Unlock()

	args, err := mb.filPayABI.Methods["depositWithPermit"].Inputs.Unpack(tx.Data()[4:])
	if err != nil {
		t.Fatalf("unpack depositWithPermit: %v", err)
	}
	deadline, ok := args[3].(*big.Int)
	if !ok {
		t.Fatalf("deadline type = %T, want *big.Int", args[3])
	}
	return deadline
}

func TestDepositWithPermit_SerializesUntilWaitReceipt(t *testing.T) {
	s, mb := newTestService(t)
	seedPermitInputs(t, mb)
	receiptReady := make(chan struct{})
	mb.receiptFn = func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
		select {
		case <-receiptReady:
			return successfulReceipt(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- depositWithPermitForRaceTest(context.Background(), s, WithWait(5*time.Second))
	}()
	waitForSentCount(t, mb, 1)

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- depositWithPermitForRaceTest(context.Background(), s)
	}()

	assertPermitWriteStillBlocked(t, mb, secondDone)
	close(receiptReady)
	waitPermitWriteDone(t, firstDone)
	waitPermitWriteDone(t, secondDone)
	waitForSentCount(t, mb, 2)
}

func TestDepositWithPermit_DefaultDeadlineStartsAfterPermitLock(t *testing.T) {
	s, mb := newTestService(t)
	seedPermitInputs(t, mb)

	release, err := s.permits.acquire(context.Background(), permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()})
	if err != nil {
		t.Fatalf("acquire permit lock: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- depositWithPermitForRaceTest(context.Background(), s)
	}()

	time.Sleep(1200 * time.Millisecond)
	releasedAt := time.Now()
	release()

	waitPermitWriteDone(t, done)
	deadline := permitDeadlineFromSent(t, mb, 0)
	wantMin := releasedAt.Unix() + int64(PermitDeadlineDuration/time.Second)
	if deadline.Int64() < wantMin {
		t.Fatalf("default deadline = %d, want >= %d", deadline.Int64(), wantMin)
	}
}

func TestDepositWithPermit_RechecksBalanceAfterPermitLock(t *testing.T) {
	s, mb := newTestService(t)
	seedPermitInputs(t, mb)
	owner := s.Account()
	amount := big.NewInt(1_000)

	release, err := s.permits.acquire(context.Background(), permitKey{chainID: s.chainID, token: tokenAddr, owner: owner})
	if err != nil {
		t.Fatalf("acquire permit lock: %v", err)
	}
	var unlocked atomic.Bool
	balanceCalled := make(chan struct{}, 1)
	mb.callReplyFn = func(contractHex, method string, _ []byte) ([]byte, bool, error) {
		if contractHex != tokenAddr.Hex() || method != "balanceOf" {
			return nil, false, nil
		}
		select {
		case balanceCalled <- struct{}{}:
		default:
		}
		balance := amount
		if unlocked.Load() {
			balance = big.NewInt(999)
		}
		mth := mb.erc20ABI.Methods["balanceOf"]
		b, err := mth.Outputs.Pack(balance)
		return b, true, err
	}

	done := make(chan error, 1)
	go func() {
		_, err := s.DepositWithPermit(context.Background(), tokenAddr, common.Address{}, amount, nil)
		done <- err
	}()

	select {
	case <-balanceCalled:
	case <-time.After(75 * time.Millisecond):
	}
	unlocked.Store(true)
	release()

	select {
	case err := <-done:
		if !errors.Is(err, ErrInsufficientBalance) {
			t.Fatalf("permit write err=%v, want ErrInsufficientBalance", err)
		}
	case <-time.After(time.Second):
		t.Fatal("permit write did not complete")
	}
	if got := sentCount(mb); got != 0 {
		t.Fatalf("sent count = %d, want 0", got)
	}
}

func TestDepositWithPermit_NoWaitHoldsLockUntilBackgroundReceipt(t *testing.T) {
	s, mb := newTestService(t)
	seedPermitInputs(t, mb)
	receiptReady := make(chan struct{})
	mb.receiptFn = func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
		select {
		case <-receiptReady:
			return successfulReceipt(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if err := depositWithPermitForRaceTest(context.Background(), s); err != nil {
		t.Fatalf("first permit write: %v", err)
	}
	waitForSentCount(t, mb, 1)

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- depositWithPermitForRaceTest(context.Background(), s)
	}()

	assertPermitWriteStillBlocked(t, mb, secondDone)
	close(receiptReady)
	waitPermitWriteDone(t, secondDone)
	waitForSentCount(t, mb, 2)
}

func TestDepositWithPermit_WaitTimeoutKeepsLockWithBackgroundWatcher(t *testing.T) {
	s, mb := newTestService(t)
	seedPermitInputs(t, mb)
	receiptReady := make(chan struct{})
	receiptCalls := 0
	mb.receiptFn = func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
		mb.mu.Lock()
		receiptCalls++
		call := receiptCalls
		mb.mu.Unlock()
		if call == 1 {
			return nil, ethereum.NotFound
		}
		select {
		case <-receiptReady:
			return successfulReceipt(), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	err := depositWithPermitForRaceTest(context.Background(), s, WithWait(25*time.Millisecond))
	if !errors.Is(err, txutil.ErrReceiptTimeout) {
		t.Fatalf("first permit write err=%v, want ErrReceiptTimeout", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- depositWithPermitForRaceTest(context.Background(), s)
	}()

	assertPermitWriteStillBlocked(t, mb, secondDone)
	close(receiptReady)
	waitPermitWriteDone(t, secondDone)
	waitForSentCount(t, mb, 2)
}

func TestDepositWithPermit_WaitTimeoutWatcherWaitsForConfirmations(t *testing.T) {
	s, mb := newTestService(t)
	s.receiptWait = time.Second
	seedPermitInputs(t, mb)
	confirmed := make(chan struct{})
	var receiptCalls atomic.Int32
	mb.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
		if receiptCalls.Add(1) == 1 {
			return nil, ethereum.NotFound
		}
		return successfulReceipt(), nil
	}
	mb.blockFn = func(ctx context.Context) (uint64, error) {
		select {
		case <-confirmed:
			return 6, nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	err := depositWithPermitForRaceTest(context.Background(), s, WithWait(25*time.Millisecond), WithConfirmations(2))
	if !errors.Is(err, txutil.ErrReceiptTimeout) {
		t.Fatalf("first permit write err=%v, want ErrReceiptTimeout", err)
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- depositWithPermitForRaceTest(context.Background(), s)
	}()

	assertPermitWriteStillBlocked(t, mb, secondDone)
	close(confirmed)
	waitPermitWriteDone(t, secondDone)
	waitForSentCount(t, mb, 2)
}

func TestDepositWithPermit_WatcherHoldsLockUntilTimeoutOnReceiptRPCError(t *testing.T) {
	s, mb := newTestService(t)
	s.receiptWait = 150 * time.Millisecond
	seedPermitInputs(t, mb)
	mb.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
		return nil, errors.New("receipt rpc unavailable")
	}

	if err := depositWithPermitForRaceTest(context.Background(), s); err != nil {
		t.Fatalf("first permit write: %v", err)
	}
	waitForSentCount(t, mb, 1)

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- depositWithPermitForRaceTest(context.Background(), s)
	}()

	assertPermitWriteStillBlocked(t, mb, secondDone)
	waitPermitWriteDone(t, secondDone)
	waitForSentCount(t, mb, 2)
}

func TestDepositWithPermit_AcquireErrorMentionsPermitLock(t *testing.T) {
	s, _ := newTestService(t)
	release, err := s.permits.acquire(context.Background(), permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()})
	if err != nil {
		t.Fatalf("acquire permit lock: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.DepositWithPermit(ctx, tokenAddr, common.Address{}, big.NewInt(1_000), nil, WithSkipPrecheck())
	if err == nil {
		t.Fatal("DepositWithPermit err=nil, want cancellation")
	}
	if !strings.Contains(err.Error(), "permit lock") {
		t.Fatalf("DepositWithPermit err=%q, want permit lock context", err)
	}
}

func TestDepositWithPermit_ReleasesLockOnPreBroadcastError(t *testing.T) {
	for _, tt := range []struct {
		name  string
		setup func(*mockBackend)
		clear func(*mockBackend)
	}{
		{
			name: "permit input",
			setup: func(mb *mockBackend) {
				mb.errs[tokenAddr.Hex()+":name"] = errors.New("name failed")
			},
			clear: func(mb *mockBackend) {
				delete(mb.errs, tokenAddr.Hex()+":name")
			},
		},
		{
			name: "nonce",
			setup: func(mb *mockBackend) {
				mb.nonceErr = errors.New("nonce failed")
			},
			clear: func(mb *mockBackend) {
				mb.nonceErr = nil
			},
		},
		{
			name: "broadcast",
			setup: func(mb *mockBackend) {
				mb.sendErr = errors.New("send failed")
			},
			clear: func(mb *mockBackend) {
				mb.sendErr = nil
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, mb := newTestService(t)
			seedPermitInputs(t, mb)
			tt.setup(mb)
			if err := depositWithPermitForRaceTest(context.Background(), s); err == nil {
				t.Fatal("first permit write err=nil, want failure")
			}
			tt.clear(mb)
			if err := depositWithPermitForRaceTest(context.Background(), s); err != nil {
				t.Fatalf("second permit write after failed first: %v", err)
			}
		})
	}
}
