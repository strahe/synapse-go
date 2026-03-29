package txutil

import (
	"context"
	"errors"
	"math/big"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

type fakeReceiptClient struct {
	calls atomic.Int64
	fn    func(call int64) (*types.Receipt, error)
	block atomic.Uint64
}

func (f *fakeReceiptClient) TransactionReceipt(_ context.Context, _ common.Hash) (*types.Receipt, error) {
	n := f.calls.Add(1)
	return f.fn(n)
}

func (f *fakeReceiptClient) BlockNumber(_ context.Context) (uint64, error) {
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
	c := &fakeReceiptClient{}
	c.fn = func(int64) (*types.Receipt, error) { return nil, errors.New("connection reset") }
	cfg := ReceiptWaitConfig{Timeout: 2 * time.Second, PollInterval: 5 * time.Millisecond, MaxConsecutiveErrors: 3}
	_, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 0)
	if !errors.Is(err, ErrReceiptRPCFailure) {
		t.Fatalf("want ErrReceiptRPCFailure, got %v", err)
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
	c := &fakeReceiptClient{}
	c.block.Store(10)
	c.fn = func(int64) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
	}
	// depth=3 needs head >= 12; first poll sees head=10 -> not yet. Bump head after 1 poll.
	done := make(chan *types.Receipt, 1)
	errCh := make(chan error, 1)
	cfg := ReceiptWaitConfig{Timeout: 2 * time.Second, PollInterval: 5 * time.Millisecond}
	go func() {
		r, err := WaitForReceiptWithConfig(context.Background(), c, common.Hash{}, cfg, 3)
		if err != nil {
			errCh <- err
			return
		}
		done <- r
	}()
	time.Sleep(30 * time.Millisecond)
	c.block.Store(12)
	select {
	case r := <-done:
		if r.BlockNumber.Uint64() != 9 {
			t.Fatalf("got block %d", r.BlockNumber.Uint64())
		}
	case err := <-errCh:
		t.Fatalf("err: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
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
