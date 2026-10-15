package sessionkey

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
			backend := newMockBackend(t)
			svc := newTestService(t, backend, newTestSigner(t))
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
			nonces := &optionsNonceManager{}
			svc.signer, svc.nonces = wrapped, nonces
			res, err := svc.Revoke(context.Background(), common.HexToAddress("0xcafe"))
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

type optionsNonceManager struct{ acquired int }

func (n *optionsNonceManager) Acquire(context.Context) (uint64, func(), error) {
	n.acquired++
	return 0, func() {}, nil
}

func TestLoginAndFundThenRevokeWithSharedSignerOptions(t *testing.T) {
	backend := newMockBackend(t)
	base := newTestSigner(t)
	template, err := base.Transactor(testChainID.BigInt())
	if err != nil {
		t.Fatal(err)
	}
	template.Nonce, template.Value, template.GasPrice = big.NewInt(77), big.NewInt(19), big.NewInt(23)
	template.GasLimit = 100_000
	originalContext := context.Background()
	template.Context = originalContext
	svc := newTestService(t, backend, templateOptsSigner{EVMSigner: base, opts: template})
	ctx := t.Context()
	key := common.HexToAddress("0xcafe")
	amount := big.NewInt(37)
	if _, err := svc.LoginAndFund(ctx, key, amount); err != nil {
		t.Fatal(err)
	}
	backend.nonces[base.EVMAddress()] = 1
	if _, err := svc.Revoke(ctx, key); err != nil {
		t.Fatal(err)
	}
	if len(backend.sent) != 2 || backend.sent[0].Value().Cmp(amount) != 0 || backend.sent[1].Value().Sign() != 0 {
		t.Fatalf("transactions = %v, want funded login followed by zero-value revoke", backend.sent)
	}
	for i, tx := range backend.sent {
		if tx.Nonce() != uint64(i) || tx.GasPrice().Int64() != 23 || tx.Gas() != 100_000 {
			t.Fatalf("transaction %d: nonce=%d gas price=%v gas=%d", i, tx.Nonce(), tx.GasPrice(), tx.Gas())
		}
	}
	if template.Nonce.Int64() != 77 || template.Value.Int64() != 19 || template.GasPrice.Int64() != 23 || template.Context != originalContext || amount.Int64() != 37 {
		t.Fatalf("template or amount changed: %+v, amount=%v", template, amount)
	}
}
