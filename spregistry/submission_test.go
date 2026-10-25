package spregistry

import (
	"context"
	"errors"
	"io"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestWithOnSubmitted(t *testing.T) {
	for _, tt := range []struct {
		name    string
		wait    time.Duration
		sendErr bool
	}{
		{name: "without wait"},
		{name: "before receipt wait", wait: time.Second},
		{name: "not after send error", sendErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, backend := newWriteTestService(t)
			if tt.sendErr {
				backend.sendErr = io.ErrUnexpectedEOF
			}
			var notified common.Hash
			backend.receiptFn = func(_ context.Context, hash common.Hash) (*types.Receipt, error) {
				if notified != hash {
					t.Fatal("receipt waiting started before submission notification")
				}
				return &types.Receipt{TxHash: hash, Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(9)}, nil
			}
			opts := []WriteOption{WithWait(tt.wait), WithOnSubmitted(func(hash common.Hash) {
				if notified != (common.Hash{}) || len(backend.sent) != 1 || backend.sent[0].Hash() != hash {
					t.Fatal("submission notification repeated or preceded broadcast")
				}
				notified = hash
			})}
			res, err := svc.UpdateProviderInfo(context.Background(), "name", "description", opts...)
			if tt.sendErr {
				if _, ok := errors.AsType[*BroadcastError](err); !ok || res != nil || notified != (common.Hash{}) {
					t.Fatalf("result=%v error=%v notified=%s", res, err, notified)
				}
				return
			}
			if err != nil || res == nil || res.Hash != notified {
				t.Fatalf("result=%v error=%v notified=%s", res, err, notified)
			}
		})
	}
}
