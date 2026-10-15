package payments

import (
	"context"
	"errors"
	"io"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/signer"
)

type templateOptsSigner struct {
	signer.EVMSigner
	opts *bind.TransactOpts
	err  error
}

func (s templateOptsSigner) Transactor(*big.Int) (*bind.TransactOpts, error) {
	return s.opts, s.err
}

func TestWriteRejectsInvalidSignerOptionsBeforeNonce(t *testing.T) {
	for _, name := range []string{"nil options", "nil signer", "wrong sender", "signer error"} {
		t.Run(name, func(t *testing.T) {
			svc, backend := newTestService(t)
			base := svc.signer
			opts, err := base.Transactor(svc.chainID.BigInt())
			if err != nil {
				t.Fatal(err)
			}
			wrapped := templateOptsSigner{EVMSigner: base, opts: opts}
			want := ErrInvalidArgument
			switch name {
			case "nil options":
				wrapped.opts = nil
			case "nil signer":
				opts.Signer = nil
			case "wrong sender":
				opts.From = common.HexToAddress("0xdead")
			case "signer error":
				wrapped.err = io.ErrUnexpectedEOF
				want = io.ErrUnexpectedEOF
			}
			nonces := &recordingNonceManager{}
			svc.signer, svc.nonces = wrapped, nonces
			res, err := svc.Approve(context.Background(), tokenAddr, operatorAddr, big.NewInt(1))
			if res != nil || !errors.Is(err, want) {
				t.Fatalf("result=%v error=%v, want nil result and %v", res, err, want)
			}
			if nonces.acquired != 0 || len(backend.sent) != 0 {
				t.Fatalf("nonce acquisitions=%d sent=%d, want zero", nonces.acquired, len(backend.sent))
			}
			if name == "signer error" && errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("signer error misclassified as ErrInvalidArgument: %v", err)
			}
		})
	}
}

func TestWriteCopiesSignerOptionsAndClearsValue(t *testing.T) {
	svc, backend := newTestService(t)
	base := svc.signer
	template, err := base.Transactor(svc.chainID.BigInt())
	if err != nil {
		t.Fatal(err)
	}
	template.Nonce, template.Value, template.GasPrice = big.NewInt(77), big.NewInt(19), big.NewInt(23)
	template.GasLimit = 100_000
	originalContext := context.Background()
	template.Context = originalContext
	svc.signer = templateOptsSigner{EVMSigner: base, opts: template}
	ctx := t.Context()
	for i := range 2 {
		backend.nonces[base.EVMAddress()] = uint64(i)
		if _, err := svc.Approve(ctx, tokenAddr, operatorAddr, big.NewInt(1)); err != nil {
			t.Fatal(err)
		}
	}
	for i, tx := range backend.sent {
		if tx.Value().Sign() != 0 || tx.Nonce() != uint64(i) || tx.GasPrice().Int64() != 23 || tx.Gas() != 100_000 {
			t.Fatalf("transaction %d: value=%v nonce=%d gas price=%v gas=%d", i, tx.Value(), tx.Nonce(), tx.GasPrice(), tx.Gas())
		}
	}
	if len(backend.sent) != 2 || template.Nonce.Int64() != 77 || template.Value.Int64() != 19 || template.GasPrice.Int64() != 23 || template.Context != originalContext {
		t.Fatalf("sent=%d template changed: %+v", len(backend.sent), template)
	}
}
