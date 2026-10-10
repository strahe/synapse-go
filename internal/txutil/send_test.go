package txutil_test

import (
	"context"
	"errors"
	"io"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/txutil"
)

type sendFunc func(context.Context, *types.Transaction) error

func (f sendFunc) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return f(ctx, tx)
}

func TestSignAndSend(t *testing.T) {
	buildErr := errors.New("signing failed")
	for _, tt := range []struct {
		name             string
		cancelBefore     bool
		cancelAfterBuild bool
		buildErr         error
		nilTransaction   bool
		sendErr          error
		noSend           bool
		wantBuilds       int
		wantSends        int
		wantErr          error
	}{
		{name: "success", wantBuilds: 1, wantSends: 1},
		{name: "caller NoSend", noSend: true, wantBuilds: 1, wantSends: 1},
		{name: "cancelled before construction", cancelBefore: true, wantErr: context.Canceled},
		{name: "construction failure", buildErr: buildErr, wantBuilds: 1, wantErr: buildErr},
		{name: "nil constructed transaction", nilTransaction: true, wantBuilds: 1},
		{name: "cancelled after signing", cancelAfterBuild: true, wantBuilds: 1, wantErr: context.Canceled},
		{name: "accepted then disconnected", sendErr: io.ErrUnexpectedEOF, wantBuilds: 1, wantSends: 1, wantErr: io.ErrUnexpectedEOF},
		{name: "cancelled during send", sendErr: context.Canceled, wantBuilds: 1, wantSends: 1, wantErr: context.Canceled},
		{name: "deadline during send", sendErr: context.DeadlineExceeded, wantBuilds: 1, wantSends: 1, wantErr: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelBefore {
				cancel()
			}
			originalCtx := context.Background()
			opts := &bind.TransactOpts{
				Context: originalCtx, NoSend: tt.noSend,
				Nonce: big.NewInt(7), Value: big.NewInt(9), GasLimit: 120_000,
				GasFeeCap: big.NewInt(11), GasTipCap: big.NewInt(2),
			}
			tx := types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(314159), Nonce: 7, Value: big.NewInt(9)})
			builds, sends := 0, 0
			sender := sendFunc(func(sendCtx context.Context, sent *types.Transaction) error {
				sends++
				if sendCtx != ctx || sent != tx {
					t.Fatal("send did not use the original context and constructed transaction")
				}
				return tt.sendErr
			})
			got, err := txutil.SignAndSend(ctx, sender, opts, func(local *bind.TransactOpts) (*types.Transaction, error) {
				builds++
				if local == opts || local.Context != ctx || !local.NoSend {
					t.Fatal("construction must use copied options with NoSend and the call context")
				}
				if local.Nonce.Cmp(opts.Nonce) != 0 || local.Value.Cmp(opts.Value) != 0 || local.GasLimit != opts.GasLimit ||
					local.GasFeeCap.Cmp(opts.GasFeeCap) != 0 || local.GasTipCap.Cmp(opts.GasTipCap) != 0 {
					t.Fatal("transaction parameters changed during construction")
				}
				if tt.cancelAfterBuild {
					cancel()
				}
				if tt.nilTransaction {
					return nil, nil
				}
				return tx, tt.buildErr
			})
			if builds != tt.wantBuilds || sends != tt.wantSends {
				t.Fatalf("builds=%d sends=%d, want %d/%d", builds, sends, tt.wantBuilds, tt.wantSends)
			}
			if tt.nilTransaction {
				if err == nil {
					t.Fatal("nil transaction was accepted")
				}
			} else if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
			if tt.wantSends == 1 {
				if got != tx {
					t.Fatal("sent transaction identity was lost")
				}
			} else if got != nil {
				t.Fatal("pre-send failure returned a broadcast transaction")
			}
			if opts.Context != originalCtx || opts.NoSend != tt.noSend {
				t.Fatal("caller's options were modified")
			}
		})
	}
}
