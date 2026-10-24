package txutil

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rpc"
)

type fakeReceiptClient struct {
	calls     atomic.Int64
	fn        func(call int64) (*types.Receipt, error)
	block     atomic.Uint64
	contextFn func(context.Context) (*types.Receipt, error)
	blockFn   func(context.Context) (uint64, error)
}

func (f *fakeReceiptClient) TransactionReceipt(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
	n := f.calls.Add(1)
	if f.contextFn != nil {
		return f.contextFn(ctx)
	}
	return f.fn(n)
}

func (f *fakeReceiptClient) BlockNumber(ctx context.Context) (uint64, error) {
	if f.blockFn != nil {
		return f.blockFn(ctx)
	}
	return f.block.Load(), nil
}

func TestWaitForReceipt_NotFoundThenSuccess(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(call int64) (*types.Receipt, error) {
		if call < 2 {
			return nil, ethereum.NotFound
		}
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(10)}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg := ReceiptWaitConfig{Timeout: 5 * time.Second, PollInterval: 10 * time.Millisecond}
	r, err := WaitForReceiptWithConfig(ctx, c, common.Hash{}, cfg, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Status != types.ReceiptStatusSuccessful {
		t.Fatalf("status: %d", r.Status)
	}
}

func TestWaitForReceipt_Reverted(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(int64) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusFailed, BlockNumber: big.NewInt(1)}, nil
	}
	cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: 10 * time.Millisecond}
	_, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 0)
	if !errors.Is(err, ErrTxFailed) {
		t.Fatalf("want ErrTxFailed, got %v", err)
	}
}

func TestWaitForReceipt_Timeout(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(int64) (*types.Receipt, error) { return nil, ethereum.NotFound }
	cfg := ReceiptWaitConfig{Timeout: 50 * time.Millisecond, PollInterval: 10 * time.Millisecond}
	_, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 0)
	if !errors.Is(err, ErrReceiptTimeout) {
		t.Fatalf("want ErrReceiptTimeout, got %v", err)
	}
}

func TestWaitForReceipt_PollsImmediately(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(int64) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(10)}, nil
	}
	cfg := ReceiptWaitConfig{Timeout: 20 * time.Millisecond, PollInterval: time.Second}
	r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.BlockNumber.Uint64() != 10 {
		t.Fatalf("got block %d", r.BlockNumber.Uint64())
	}
	if c.calls.Load() != 1 {
		t.Fatalf("expected 1 receipt poll, got %d", c.calls.Load())
	}
}

func TestWaitForReceipt_RPCErrorsExceed(t *testing.T) {
	for _, cause := range []error{errors.New("connection reset"), &rpc.HTTPError{StatusCode: 429}, &rpc.HTTPError{StatusCode: 408}} {
		t.Run(fmt.Sprintf("%T", cause), func(t *testing.T) {
			c := &fakeReceiptClient{}
			c.fn = func(int64) (*types.Receipt, error) { return nil, cause }
			cfg := ReceiptWaitConfig{Timeout: 2 * time.Second, PollInterval: 5 * time.Millisecond, MaxConsecutiveErrors: 3}
			_, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 0)
			if !errors.Is(err, ErrReceiptRPCFailure) || !errors.Is(err, cause) || c.calls.Load() != 3 {
				t.Fatalf("err=%v polls=%d, want RPC failure preserving cause after three polls", err, c.calls.Load())
			}
		})
	}
}

func TestWaitForReceipt_RecoversFromHTTPError(t *testing.T) {
	for _, status := range []int{408, 429, 502, 503, 504} {
		for _, headFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("status=%d/head=%t", status, headFailure), func(t *testing.T) {
				cause := fmt.Errorf("RPC: %w", &rpc.HTTPError{StatusCode: status})
				c := &fakeReceiptClient{}
				c.fn = func(call int64) (*types.Receipt, error) {
					if call == 1 && !headFailure {
						return nil, cause
					}
					return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(10)}, nil
				}
				c.blockFn = func(context.Context) (uint64, error) {
					if headFailure && c.calls.Load() == 1 {
						return 0, cause
					}
					return 10, nil
				}
				cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond}
				r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 1)
				if err != nil || r == nil || c.calls.Load() != 2 {
					t.Fatalf("receipt=%v err=%v polls=%d, want success after two polls", r, err, c.calls.Load())
				}
			})
		}
	}
}

func TestWaitForReceipt_NonRetryableRPCError(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(int64) (*types.Receipt, error) { return nil, errors.New("invalid argument") }
	cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: 5 * time.Millisecond}
	_, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 0)
	if !errors.Is(err, ErrReceiptRPCFailure) {
		t.Fatalf("want ErrReceiptRPCFailure, got %v", err)
	}
}

func TestWaitForConfirmation_WaitsForDepth(t *testing.T) {
	for _, status := range []uint64{types.ReceiptStatusSuccessful, types.ReceiptStatusFailed} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			c := &fakeReceiptClient{}
			c.fn = func(int64) (*types.Receipt, error) {
				return &types.Receipt{Status: status, BlockNumber: big.NewInt(9)}, nil
			}
			c.blockFn = func(context.Context) (uint64, error) {
				// Block 9 has two confirmations at head 10 and three at head 11.
				return uint64(9 + c.calls.Load()), nil
			}
			cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond}
			r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 3)
			if r == nil || r.Status != status || c.calls.Load() != 2 {
				t.Fatalf("receipt=%v polls=%d, want status %d after two polls", r, c.calls.Load(), status)
			}
			if status == types.ReceiptStatusFailed {
				if !errors.Is(err, ErrTxFailed) {
					t.Fatalf("err=%v, want ErrTxFailed", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestWaitForConfirmation_OneConfirmationAtCurrentHead(t *testing.T) {
	c := &fakeReceiptClient{}
	c.block.Store(9)
	c.fn = func(int64) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
	}
	cfg := ReceiptWaitConfig{Timeout: 50 * time.Millisecond, PollInterval: time.Second}
	r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.BlockNumber.Uint64() != 9 {
		t.Fatalf("got block %d", r.BlockNumber.Uint64())
	}
}

func TestDefaultReceiptWaitConfig(t *testing.T) {
	cfg := DefaultReceiptWaitConfig()
	if cfg.Timeout != 5*time.Minute {
		t.Errorf("Timeout = %v, want 5m", cfg.Timeout)
	}
	if cfg.PollInterval != 2*time.Second {
		t.Errorf("PollInterval = %v, want 2s", cfg.PollInterval)
	}
	if cfg.MaxConsecutiveErrors != 5 {
		t.Errorf("MaxConsecutiveErrors = %d, want 5", cfg.MaxConsecutiveErrors)
	}
}

func TestWaitForReceipt_DelegatesToWaitForReceipt(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(int64) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(42)}, nil
	}
	r, err := WaitForReceipt(context.Background(), c, common.Hash{}, 50*time.Millisecond)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.BlockNumber.Uint64() != 42 {
		t.Fatalf("got block %d", r.BlockNumber.Uint64())
	}
}

func TestWaitForReceipt_ZeroTimeoutSucceeds(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(int64) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(1)}, nil
	}
	// timeout=0 falls back to the internal default; the call succeeds immediately so no actual wait.
	r, err := WaitForReceipt(context.Background(), c, common.Hash{}, 0)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r == nil {
		t.Fatal("expected receipt")
	}
}

func TestWaitForConfirmation_Delegates(t *testing.T) {
	c := &fakeReceiptClient{}
	c.block.Store(15)
	c.fn = func(int64) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(10)}, nil
	}
	r, err := WaitForConfirmation(context.Background(), c, common.Hash{}, 3)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.BlockNumber.Uint64() != 10 {
		t.Fatalf("got block %d", r.BlockNumber.Uint64())
	}
}

func TestHasConfirmations(t *testing.T) {
	max := new(big.Int).SetUint64(math.MaxUint64)
	for _, tt := range []struct {
		name                string
		block               *big.Int
		head, confirmations uint64
		want                bool
	}{
		{"no requirement", nil, 0, 0, true},
		{"missing block", nil, 10, 1, false},
		{"negative block", big.NewInt(-1), math.MaxUint64, 1, false},
		{"wide block", new(big.Int).Lsh(big.NewInt(1), 64), math.MaxUint64, 1, false},
		{"head behind", big.NewInt(9), 8, 1, false},
		{"inclusion", big.NewInt(9), 9, 1, true},
		{"insufficient depth", big.NewInt(9), 10, 3, false},
		{"exact depth", big.NewInt(9), 11, 3, true},
		{"max inclusion", max, math.MaxUint64, 1, true},
		{"overflow depth", max, math.MaxUint64, 2, false},
		{"max depth insufficient", big.NewInt(2), math.MaxUint64, math.MaxUint64, false},
		{"max depth reached", big.NewInt(1), math.MaxUint64, math.MaxUint64, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasConfirmations(tt.block, tt.head, tt.confirmations); got != tt.want {
				t.Fatalf("HasConfirmations()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestWaitForConfirmation_RefreshesReceipt(t *testing.T) {
	c := &fakeReceiptClient{}
	c.fn = func(call int64) (*types.Receipt, error) {
		switch call {
		case 1:
			return &types.Receipt{Status: types.ReceiptStatusFailed, BlockNumber: big.NewInt(9)}, nil
		case 2:
			return nil, nil
		case 3:
			return nil, ethereum.NotFound
		default:
			return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(20)}, nil
		}
	}
	c.blockFn = func(context.Context) (uint64, error) {
		switch c.calls.Load() {
		case 1:
			return 9, nil
		case 4:
			return 20, nil
		default:
			return 22, nil
		}
	}
	cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond}
	r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 3)
	if err != nil || r == nil || r.Status != types.ReceiptStatusSuccessful || r.BlockNumber.Uint64() != 20 || c.calls.Load() != 5 {
		t.Fatalf("receipt=%v err=%v polls=%d, want re-included success after five polls", r, err, c.calls.Load())
	}
}

func TestWaitForConfirmation_UnconfirmedReceiptTimesOut(t *testing.T) {
	for _, block := range []*big.Int{nil, big.NewInt(-1), new(big.Int).Lsh(big.NewInt(1), 64), big.NewInt(9)} {
		t.Run(fmt.Sprint(block), func(t *testing.T) {
			c := &fakeReceiptClient{}
			c.fn = func(int64) (*types.Receipt, error) {
				return &types.Receipt{Status: types.ReceiptStatusFailed, BlockNumber: block}, nil
			}
			c.block.Store(9)
			cfg := ReceiptWaitConfig{Timeout: 10 * time.Millisecond, PollInterval: time.Second}
			r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 3)
			if r != nil || !errors.Is(err, ErrReceiptTimeout) || !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrTxFailed) {
				t.Fatalf("receipt=%v err=%v, want timeout without a terminal receipt", r, err)
			}
		})
	}
}

func TestWaitForConfirmation_RPCErrorBudget(t *testing.T) {
	for _, alternate := range []bool{false, true} {
		t.Run(fmt.Sprintf("alternate_receipt_errors_%v", alternate), func(t *testing.T) {
			cause := errors.New("connection reset")
			c := &fakeReceiptClient{}
			c.fn = func(call int64) (*types.Receipt, error) {
				if alternate && call%2 == 1 {
					return nil, cause
				}
				return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
			}
			c.blockFn = func(context.Context) (uint64, error) { return 0, cause }
			cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond, MaxConsecutiveErrors: 3}
			r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 1)
			if r != nil || !errors.Is(err, ErrReceiptRPCFailure) || !errors.Is(err, cause) || c.calls.Load() != 3 {
				t.Fatalf("receipt=%v err=%v polls=%d, want RPC failure after three polls", r, err, c.calls.Load())
			}
		})
	}
}

func TestWaitForConfirmation_HealthyPollResetsRPCErrorBudget(t *testing.T) {
	for _, recovery := range []string{"not found", "nil receipt", "insufficient depth"} {
		t.Run(recovery, func(t *testing.T) {
			c := &fakeReceiptClient{}
			c.fn = func(call int64) (*types.Receipt, error) {
				if call == 3 {
					switch recovery {
					case "not found":
						return nil, ethereum.NotFound
					case "nil receipt":
						return nil, nil
					}
				}
				return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
			}
			c.blockFn = func(context.Context) (uint64, error) {
				switch c.calls.Load() {
				case 3:
					return 8, nil
				case 6:
					return 9, nil
				default:
					return 0, errors.New("connection reset")
				}
			}
			cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond, MaxConsecutiveErrors: 3}
			r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 1)
			if r == nil || err != nil || c.calls.Load() != 6 {
				t.Fatalf("receipt=%v err=%v polls=%d, want recovery after six polls", r, err, c.calls.Load())
			}
		})
	}
}

func TestWaitForReceipt_ContextClassification(t *testing.T) {
	for _, stage := range []string{"before RPC", "receipt RPC", "head RPC"} {
		for _, deadline := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deadline_%v", stage, deadline), func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if deadline {
					var deadlineCancel context.CancelFunc
					if stage == "before RPC" {
						ctx, deadlineCancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
					} else {
						ctx, deadlineCancel = context.WithTimeout(ctx, 10*time.Millisecond)
					}
					defer deadlineCancel()
				} else if stage == "before RPC" {
					cancel()
				}
				fail := func(ctx context.Context) error {
					if !deadline {
						cancel()
					}
					<-ctx.Done()
					return errors.New("transport failed")
				}
				c := &fakeReceiptClient{}
				c.contextFn = func(ctx context.Context) (*types.Receipt, error) {
					if stage == "receipt RPC" {
						return nil, fail(ctx)
					}
					return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
				}
				c.blockFn = func(ctx context.Context) (uint64, error) { return 0, fail(ctx) }
				cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond}
				r, err := WaitForReceiptWithConfig(ctx, c, common.Hash{}, cfg, 1)
				if r != nil || errors.Is(err, ErrReceiptRPCFailure) {
					t.Fatalf("receipt=%v err=%v, want context error", r, err)
				}
				if deadline {
					if !errors.Is(err, ErrReceiptTimeout) || !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("err=%v, want receipt timeout and deadline", err)
					}
				} else if !errors.Is(err, context.Canceled) || errors.Is(err, ErrReceiptTimeout) {
					t.Fatalf("err=%v, want cancellation without receipt timeout", err)
				}
				if stage == "before RPC" && c.calls.Load() != 0 {
					t.Fatalf("receipt polls=%d, want none", c.calls.Load())
				}
			})
		}
	}
}

func TestWaitForReceipt_BackendDeadlineIsRPCFailure(t *testing.T) {
	for _, headError := range []bool{false, true} {
		t.Run(fmt.Sprintf("head_%v", headError), func(t *testing.T) {
			c := &fakeReceiptClient{}
			c.fn = func(int64) (*types.Receipt, error) {
				if !headError {
					return nil, context.DeadlineExceeded
				}
				return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
			}
			c.blockFn = func(context.Context) (uint64, error) { return 0, context.DeadlineExceeded }
			cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond}
			r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 1)
			if r != nil || !errors.Is(err, ErrReceiptRPCFailure) || !errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrReceiptTimeout) || c.calls.Load() != 1 {
				t.Fatalf("receipt=%v err=%v polls=%d, want immediate RPC failure with cause", r, err, c.calls.Load())
			}
		})
	}
}

func TestWaitForReceipt_TerminalReceiptSurvivesConcurrentCancellation(t *testing.T) {
	for _, status := range []uint64{types.ReceiptStatusSuccessful, types.ReceiptStatusFailed} {
		t.Run(fmt.Sprintf("status_%d", status), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			want := &types.Receipt{Status: status}
			client := &fakeReceiptClient{fn: func(int64) (*types.Receipt, error) {
				cancel()
				return want, nil
			}}
			cfg := ReceiptWaitConfig{Timeout: time.Second, PollInterval: time.Millisecond}
			got, err := WaitForReceiptWithConfig(ctx, client, common.Hash{}, cfg, 0)
			if got != want || errors.Is(err, context.Canceled) || errors.Is(err, ErrReceiptTimeout) || errors.Is(err, ErrReceiptRPCFailure) {
				t.Fatalf("terminal receipt preserved=%v status=%d error=%v", got == want, status, err)
			}
			if status == types.ReceiptStatusFailed {
				if !errors.Is(err, ErrTxFailed) {
					t.Fatalf("error=%v, want ErrTxFailed", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}
