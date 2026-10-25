package payments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/signer"
)

func TestDepositWithPermit_BroadcastErrorCoordinatesNextPermit(t *testing.T) {
	for _, firstCombined := range []bool{false, true} {
		for _, status := range []uint64{types.ReceiptStatusSuccessful, types.ReceiptStatusFailed} {
			t.Run(fmt.Sprintf("combined_%t/status_%d", firstCombined, status), func(t *testing.T) {
				sg := &validationSigner{EVMSigner: newTestSigner(t)}
				s, mb := newTestServiceWith(t, sg, func(opts *Options) { opts.PermitReceiptWait = time.Second })
				seedPermitInputs(t, mb)
				mb.sendErrAfterAccept = io.ErrUnexpectedEOF
				var nonceReads atomic.Int32
				mb.callReplyFn = func(_, method string, _ []byte) ([]byte, bool, error) {
					if method == "nonces" {
						nonceReads.Add(1)
					}
					return nil, false, nil
				}
				receiptReady := make(chan struct{})
				var releaseReceipt sync.Once
				watcherStarted := make(chan context.Context, 1)
				var firstReceipt sync.Once
				mb.receiptFn = func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
					mb.mu.Lock()
					firstHash := mb.sent[0].Hash()
					mb.mu.Unlock()
					if hash != firstHash {
						return successfulReceipt(), nil
					}
					firstReceipt.Do(func() { watcherStarted <- ctx })
					select {
					case <-receiptReady:
						if status == types.ReceiptStatusSuccessful {
							mb.mu.Lock()
							mb.replies[tokenAddr.Hex()+":nonces"], _ = permitERC20ABI.Methods["nonces"].Outputs.Pack(big.NewInt(1))
							mb.mu.Unlock()
						}
						return &types.Receipt{Status: status, BlockNumber: big.NewInt(5)}, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				mb.blockFn = func(context.Context) (uint64, error) {
					t.Error("confirmations without a positive WithWait must be ignored")
					return 100, nil
				}
				t.Cleanup(func() {
					releaseReceipt.Do(func() { close(receiptReady) })
					waitForPermitLockReleased(t, s)
				})
				write := func(ctx context.Context, combined bool, opts ...WriteOption) error {
					opts = append([]WriteOption{WithSkipPrecheck()}, opts...)
					if combined {
						_, err := s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, s.Account(), big.NewInt(1_000), nil,
							operatorAddr, big.NewInt(1), big.NewInt(1), big.NewInt(1), opts...)
						return err
					}
					return depositWithPermitForRaceTest(ctx, s, opts...)
				}
				firstCtx, cancelFirst := context.WithCancel(context.Background())
				defer cancelFirst()
				err := write(firstCtx, firstCombined, WithConfirmations(100))
				broadcast, ok := errors.AsType[*BroadcastError](err)
				if !ok || !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("first permit write err=%v, want original broadcast error", err)
				}
				mb.mu.Lock()
				firstTx := mb.sent[0]
				mb.nonces[s.Account()] = 1
				mb.sendErrAfterAccept = nil
				mb.mu.Unlock()
				if broadcast.Transaction != firstTx {
					t.Fatal("broadcast error did not preserve the original transaction")
				}
				cancelFirst()
				select {
				case watcherCtx := <-watcherStarted:
					if watcherCtx.Err() != nil {
						t.Fatalf("caller cancellation canceled watcher: %v", watcherCtx.Err())
					}
				case <-time.After(time.Second):
					t.Fatal("broadcast error did not start a receipt watcher")
				}
				blockedCtx, cancelBlocked := context.WithTimeout(context.Background(), 25*time.Millisecond)
				defer cancelBlocked()
				if err := write(blockedCtx, !firstCombined); !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("waiting permit err=%v, want deadline while receipt is pending", err)
				}
				if nonceReads.Load() != 1 || sg.hashes != 1 || sg.signatures != 1 || sentCount(mb) != 1 {
					t.Fatalf("waiting write progressed: nonce reads=%d permit signatures=%d transaction signatures=%d sent=%d",
						nonceReads.Load(), sg.hashes, sg.signatures, sentCount(mb))
				}
				secondDone := make(chan error, 1)
				secondCtx, cancelSecond := context.WithTimeout(context.Background(), time.Second)
				defer cancelSecond()
				go func() { secondDone <- write(secondCtx, !firstCombined, WithWait(time.Second)) }()
				releaseReceipt.Do(func() { close(receiptReady) })
				waitPermitWriteDone(t, secondDone)
				mb.mu.Lock()
				secondTx := mb.sent[1]
				mb.mu.Unlock()
				if firstTx.Nonce() != 0 || secondTx.Nonce() != 1 {
					t.Fatalf("EOA transaction nonces=%d,%d, want 0,1", firstTx.Nonce(), secondTx.Nonce())
				}
				assertTransactionPermitNonce(t, s, mb, firstTx, 0)
				wantNonce := int64(0)
				if status == types.ReceiptStatusSuccessful {
					wantNonce = 1
				}
				assertTransactionPermitNonce(t, s, mb, secondTx, wantNonce)
			})
		}
	}
}

func assertTransactionPermitNonce(t *testing.T, s *Service, mb *mockBackend, tx *types.Transaction, nonce int64) {
	t.Helper()
	method, err := mb.filPayABI.MethodById(tx.Data()[:4])
	if err != nil {
		t.Fatal(err)
	}
	args, err := method.Inputs.Unpack(tx.Data()[4:])
	if err != nil {
		t.Fatal(err)
	}
	domain := typeddata.NewERC20PermitDomain(s.chainID.BigInt(), "USDFC", "1", tokenAddr)
	sig, err := typeddata.SignERC20Permit(func(hash []byte) ([]byte, error) { return signer.SignHash(s.signer, hash) },
		domain, s.Account(), s.filPayAddr, args[2].(*big.Int), big.NewInt(nonce), args[3].(*big.Int))
	if err != nil {
		t.Fatal(err)
	}
	if args[4] != sig.V || args[5] != sig.R || args[6] != sig.S {
		t.Fatalf("%s transaction did not authorize permit nonce %d", method.Name, nonce)
	}
}

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

func waitForPermitLockReleased(t *testing.T, s *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	release, err := s.permits.acquire(ctx, permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()})
	if err != nil {
		t.Errorf("permit watcher did not finish: %v", err)
		return
	}
	release()
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
	for _, wait := range []time.Duration{0, -time.Second, 5 * time.Second} {
		t.Run(wait.String(), func(t *testing.T) {
			s, mb := newTestService(t, func(opts *Options) {
				opts.PermitReceiptWait = wait
			})
			seedPermitInputs(t, mb)
			receiptReady := make(chan struct{})
			t.Cleanup(func() {
				select {
				case <-receiptReady:
				default:
					close(receiptReady)
				}
			})
			deadlines := make(chan time.Time, 2)
			mb.receiptFn = func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
				deadline, _ := ctx.Deadline()
				deadlines <- deadline
				select {
				case <-receiptReady:
					return successfulReceipt(), nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}

			started := time.Now()
			if err := depositWithPermitForRaceTest(context.Background(), s); err != nil {
				t.Fatalf("first permit write: %v", err)
			}
			wantWait := wait
			if wantWait <= 0 {
				wantWait = 5 * time.Minute
			}
			select {
			case deadline := <-deadlines:
				if deadline.Before(started.Add(wantWait)) || deadline.After(time.Now().Add(wantWait)) {
					t.Fatalf("watcher deadline = %s, want timeout %s", deadline, wantWait)
				}
			case <-time.After(time.Second):
				t.Fatal("background receipt watcher did not start")
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
		})
	}
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

func TestDepositWithPermit_WatcherWaitsForConfirmations(t *testing.T) {
	for _, broadcastError := range []bool{false, true} {
		for _, status := range []uint64{types.ReceiptStatusSuccessful, types.ReceiptStatusFailed} {
			t.Run(fmt.Sprintf("broadcast_%t/status_%d", broadcastError, status), func(t *testing.T) {
				s, mb := newTestService(t, func(opts *Options) {
					opts.PermitReceiptWait = 5 * time.Second
				})
				seedPermitInputs(t, mb)
				if broadcastError {
					mb.sendErrAfterAccept = io.ErrUnexpectedEOF
				}
				confirmed := make(chan struct{})
				var releaseConfirmed sync.Once
				t.Cleanup(func() {
					releaseConfirmed.Do(func() { close(confirmed) })
					waitForPermitLockReleased(t, s)
				})
				headRequested := make(chan struct{}, 1)
				mb.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
					return &types.Receipt{Status: status, BlockNumber: big.NewInt(5)}, nil
				}
				mb.blockFn = func(ctx context.Context) (uint64, error) {
					headRequested <- struct{}{}
					select {
					case <-confirmed:
						return 6, nil
					case <-ctx.Done():
						return 0, ctx.Err()
					}
				}

				err := depositWithPermitForRaceTest(context.Background(), s, WithWait(25*time.Millisecond), WithConfirmations(2))
				wantErr := error(txutil.ErrReceiptTimeout)
				if broadcastError {
					wantErr = io.ErrUnexpectedEOF
				}
				if !errors.Is(err, wantErr) {
					t.Fatalf("first permit write err=%v, want %v", err, wantErr)
				}
				select {
				case <-headRequested:
				case <-time.After(time.Second):
					t.Fatal("watcher did not check confirmation depth")
				}
				key := permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()}
				blockedCtx, cancelBlocked := context.WithTimeout(context.Background(), 25*time.Millisecond)
				defer cancelBlocked()
				if release, err := s.permits.acquire(blockedCtx, key); !errors.Is(err, context.DeadlineExceeded) {
					if release != nil {
						release()
					}
					t.Fatalf("permit acquire err=%v, want deadline while depth is unconfirmed", err)
				}
				releaseConfirmed.Do(func() { close(confirmed) })
				acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), time.Second)
				defer cancelAcquire()
				release, err := s.permits.acquire(acquireCtx, key)
				if err != nil {
					t.Fatalf("permit remained locked after confirmations: %v", err)
				}
				release()
			})
		}
	}
}

func TestDepositWithPermit_WatcherHoldsLockUntilTimeoutOnRPCError(t *testing.T) {
	for _, broadcastError := range []bool{false, true} {
		for _, tt := range []struct {
			name      string
			err       error
			headError bool
		}{
			{name: "receipt not found", err: ethereum.NotFound},
			{name: "receipt RPC error", err: errors.New("rpc unavailable")},
			{name: "head RPC error", err: errors.New("rpc unavailable"), headError: true},
		} {
			t.Run(fmt.Sprintf("broadcast_%t/%s", broadcastError, tt.name), func(t *testing.T) {
				s, mb := newTestService(t, func(opts *Options) {
					opts.PermitReceiptWait = 150 * time.Millisecond
				})
				seedPermitInputs(t, mb)
				t.Cleanup(func() { waitForPermitLockReleased(t, s) })
				if broadcastError {
					mb.sendErrAfterAccept = io.ErrUnexpectedEOF
				}
				watcherPolled := make(chan struct{})
				var errorCalls atomic.Int32
				rpcError := func() error {
					calls := errorCalls.Add(1)
					wantCalls := int32(1)
					if tt.headError && !broadcastError {
						wantCalls = 2
					}
					if calls == wantCalls {
						close(watcherPolled)
					}
					return tt.err
				}

				var opts []WriteOption
				if tt.headError {
					mb.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
						return successfulReceipt(), nil
					}
					mb.blockFn = func(context.Context) (uint64, error) {
						return 0, rpcError()
					}
					opts = []WriteOption{WithWait(time.Second), WithConfirmations(2)}
				} else {
					mb.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
						return nil, rpcError()
					}
				}
				err := depositWithPermitForRaceTest(context.Background(), s, opts...)
				switch {
				case broadcastError:
					if !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatalf("first permit write err=%v, want broadcast error", err)
					}
				case tt.headError:
					if !errors.Is(err, txutil.ErrReceiptRPCFailure) {
						t.Fatalf("first permit write err=%v, want receipt RPC failure", err)
					}
				case err != nil:
					t.Fatalf("first permit write: %v", err)
				}
				select {
				case <-watcherPolled:
				case <-time.After(time.Second):
					t.Fatal("watcher did not observe the RPC error")
				}

				key := permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()}
				blockedCtx, cancelBlocked := context.WithTimeout(context.Background(), 25*time.Millisecond)
				defer cancelBlocked()
				if release, err := s.permits.acquire(blockedCtx, key); !errors.Is(err, context.DeadlineExceeded) {
					if release != nil {
						release()
					}
					t.Fatalf("permit acquire err=%v, want deadline while watcher handles RPC error", err)
				}
				acquireCtx, cancelAcquire := context.WithTimeout(context.Background(), time.Second)
				defer cancelAcquire()
				release, err := s.permits.acquire(acquireCtx, key)
				if err != nil {
					t.Fatalf("permit remained locked after watcher timeout: %v", err)
				}
				release()
			})
		}
	}
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

func TestDepositWithPermit_ReleasesLockOnWriteError(t *testing.T) {
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
			name: "gas estimate",
			setup: func(mb *mockBackend) {
				mb.estimateGasErr = errors.New("gas estimation failed")
			},
			clear: func(mb *mockBackend) {
				mb.estimateGasErr = nil
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, mb := newTestService(t)
			seedPermitInputs(t, mb)
			mb.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
				return successfulReceipt(), nil
			}
			t.Cleanup(func() { waitForPermitLockReleased(t, s) })
			tt.setup(mb)
			if err := depositWithPermitForRaceTest(context.Background(), s); err == nil {
				t.Fatal("first permit write err=nil, want failure")
			}
			if sentCount(mb) != 0 {
				t.Fatal("pre-broadcast failure sent a transaction")
			}
			tt.clear(mb)
			if err := depositWithPermitForRaceTest(context.Background(), s, WithWait(time.Second)); err != nil {
				t.Fatalf("second permit write after failed first: %v", err)
			}
		})
	}
}

func TestDepositWithPermit_SubmissionPanicKeepsReceiptWatcher(t *testing.T) {
	for _, wait := range []time.Duration{0, time.Second} {
		t.Run(wait.String(), func(t *testing.T) {
			s, mb := newTestService(t, func(opts *Options) { opts.PermitReceiptWait = time.Second })
			seedPermitInputs(t, mb)
			started := make(chan struct{})
			receiptReady := make(chan struct{})
			var startOnce, releaseOnce sync.Once
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(receiptReady) })
				waitForPermitLockReleased(t, s)
			})
			mb.receiptFn = func(ctx context.Context, hash common.Hash) (*types.Receipt, error) {
				startOnce.Do(func() { close(started) })
				select {
				case <-receiptReady:
					return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(5)}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			want := new(int)
			var caught any
			func() {
				defer func() { caught = recover() }()
				err := depositWithPermitForRaceTest(context.Background(), s, WithWait(wait), WithConfirmations(2),
					WithOnSubmitted(func(hash common.Hash) {
						mb.mu.Lock()
						if len(mb.sent) != 1 || mb.sent[0].Hash() != hash {
							t.Error("permit notification preceded broadcast")
						}
						mb.mu.Unlock()
						panic(want)
					}))
				if err != nil {
					t.Fatalf("failed before callback: %v", err)
				}
			}()
			if caught != want {
				t.Fatalf("panic=%v, want original callback value", caught)
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("callback panic did not start receipt watcher")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			release, err := s.permits.acquire(ctx, permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()})
			if release != nil {
				release()
			}
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("permit reservation released before receipt: %v", err)
			}
			releaseOnce.Do(func() { close(receiptReady) })
			waitForPermitLockReleased(t, s)
		})
	}
}
