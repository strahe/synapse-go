package warmstorage

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/txutil"
)

func TestWaitForTransaction_ReadOnly(t *testing.T) {
	for _, tt := range []struct {
		name          string
		wait          time.Duration
		confirmations uint64
		wantErr       error
	}{
		{name: "default timeout"},
		{name: "unconfirmed receipt", wait: 20 * time.Millisecond, confirmations: 10000, wantErr: txutil.ErrReceiptTimeout},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, backend := newWriteTestService(t)
			svc.signer = nil
			svc.nonces = nil
			hash := common.HexToHash("0x1234")
			receipt := &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}
			backend.receiptFn = func(ctx context.Context, got common.Hash) (*types.Receipt, error) {
				deadline, ok := ctx.Deadline()
				if got != hash || !ok || (tt.wait <= 0 && time.Until(deadline) < 4*time.Minute) {
					t.Fatalf("lookup hash=%s deadline=%s present=%v", got, deadline, ok)
				}
				return receipt, nil
			}
			res, err := svc.WaitForTransaction(context.Background(), hash, WithWait(tt.wait), WithConfirmations(tt.confirmations),
				WithOnSubmitted(func(common.Hash) { t.Fatal("read-only wait notified submission") }))
			if !errors.Is(err, tt.wantErr) || res == nil || res.Hash != hash || (res.Receipt != nil) != (tt.wantErr == nil) || len(backend.sent) != 0 {
				t.Fatalf("result=%+v error=%v sent=%d", res, err, len(backend.sent))
			}
			if err != nil && !strings.HasPrefix(err.Error(), "warmstorage.WaitForTransaction:") {
				t.Fatalf("error prefix=%v", err)
			}
		})
	}
}

func TestWaitForTransaction_RejectsZeroHash(t *testing.T) {
	svc, backend := newWriteTestService(t)
	backend.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
		t.Fatal("invalid hash reached RPC")
		return nil, nil
	}
	res, err := svc.WaitForTransaction(context.Background(), common.Hash{})
	if res != nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("result=%v error=%v", res, err)
	}
}

func TestWaitForTransaction_RequiresBackend(t *testing.T) {
	svc, _ := newWriteTestService(t)
	svc.backend = nil
	res, err := svc.WaitForTransaction(context.Background(), common.HexToHash("0x1234"))
	if res != nil || !errors.Is(err, ErrWriteNotConfigured) {
		t.Fatalf("result=%v error=%v", res, err)
	}
}
