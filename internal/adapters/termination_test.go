package adapters

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type directTerminationBackend struct {
	warmstorage.Backend
	submitted *coretypes.Transaction
	sendErr   error
	receiptFn func(context.Context, common.Hash) (*coretypes.Receipt, error)
}

func (b *directTerminationBackend) PendingNonceAt(context.Context, common.Address) (uint64, error) {
	return 0, nil
}

func (b *directTerminationBackend) SendTransaction(_ context.Context, tx *coretypes.Transaction) error {
	if b.sendErr != nil {
		return b.sendErr
	}
	b.submitted = tx
	return nil
}

func (b *directTerminationBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*coretypes.Receipt, error) {
	return b.receiptFn(ctx, hash)
}

type directTerminationSigner struct{ signer.EVMSigner }

func (s directTerminationSigner) Transactor(chainID *big.Int) (*bind.TransactOpts, error) {
	opts, err := s.EVMSigner.Transactor(chainID)
	if err != nil {
		return nil, err
	}
	opts.GasPrice = big.NewInt(1)
	opts.GasLimit = 100_000
	return opts, nil
}

func newDirectTerminationTestService(t *testing.T, backend *directTerminationBackend) *warmstorage.Service {
	t.Helper()
	evmSigner, err := signer.NewSecp256k1SignerFromBytes([]byte{1})
	if err != nil {
		t.Fatal(err)
	}
	svc, err := warmstorage.New(warmstorage.Options{
		Client:       backend,
		Backend:      backend,
		Signer:       directTerminationSigner{evmSigner},
		ChainID:      314159,
		FWSS:         common.HexToAddress("0x1111"),
		ViewContract: common.HexToAddress("0x2222"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func TestFWSSTerminator_SubmissionAndWaitOptions(t *testing.T) {
	broadcastErr := errors.New("broadcast rejected")
	tests := []struct {
		name          string
		sendErr       error
		pending       bool
		cancelOnHash  bool
		writeHookOnly bool
		noHook        bool
		receiptStatus uint64
		wantErr       error
	}{
		{name: "explicit notification overrides write option", receiptStatus: coretypes.ReceiptStatusSuccessful},
		{name: "write-option notification", writeHookOnly: true, receiptStatus: coretypes.ReceiptStatusSuccessful},
		{name: "no notification", noHook: true, receiptStatus: coretypes.ReceiptStatusSuccessful},
		{name: "broadcast rejected", sendErr: broadcastErr, wantErr: broadcastErr},
		{name: "receipt timeout", pending: true, wantErr: context.DeadlineExceeded},
		{name: "canceled after submission", cancelOnHash: true, wantErr: context.Canceled},
		{name: "reverted transaction", receiptStatus: coretypes.ReceiptStatusFailed, wantErr: types.ErrTxFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			backend := &directTerminationBackend{sendErr: tt.sendErr}
			callbackCalls := 0
			var submitted common.Hash
			backend.receiptFn = func(ctx context.Context, hash common.Hash) (*coretypes.Receipt, error) {
				if !tt.noHook && (callbackCalls != 1 || submitted != hash) {
					t.Fatalf("receipt polling before notification: calls=%d hash=%s submitted=%s", callbackCalls, hash, submitted)
				}
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if tt.pending {
					return nil, ethereum.NotFound
				}
				return &coretypes.Receipt{TxHash: hash, Status: tt.receiptStatus}, nil
			}
			term := NewFWSSTerminator(newDirectTerminationTestService(t, backend))
			onSubmitted := func(hash common.Hash) {
				callbackCalls++
				if backend.submitted == nil || hash != backend.submitted.Hash() {
					t.Fatalf("callback before successful broadcast: hash=%s tx=%v", hash, backend.submitted)
				}
				submitted = hash
				if tt.cancelOnHash {
					cancel()
				}
			}
			opts := storage.FWSSTerminationOptions{
				WaitTimeout: 25 * time.Millisecond,
				WriteOptions: []warmstorage.WriteOption{
					warmstorage.WithWait(0),
					warmstorage.WithOnSubmitted(func(common.Hash) { t.Fatal("explicit callback did not override write option") }),
				},
				OnSubmitted: onSubmitted,
			}
			if tt.writeHookOnly {
				opts.OnSubmitted = nil
				opts.WriteOptions = []warmstorage.WriteOption{warmstorage.WithWait(0), warmstorage.WithOnSubmitted(onSubmitted)}
			}
			if tt.noHook {
				opts.OnSubmitted = nil
				opts.WriteOptions = []warmstorage.WriteOption{warmstorage.WithWait(0)}
			}
			res, err := term.TerminateDataSet(ctx, types.NewBigInt(7), opts)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("TerminateDataSet error=%v, want %v", err, tt.wantErr)
				}
				if tt.sendErr != nil {
					if res != nil {
						t.Fatalf("broadcast failure returned result: %+v", res)
					}
				} else if res == nil || res.Hash != submitted {
					t.Fatalf("waiting failure lost submitted hash: result=%+v hash=%s", res, submitted)
				}
			} else if err != nil || res == nil || res.Receipt == nil {
				t.Fatalf("TerminateDataSet = %+v, %v, want confirmed receipt despite WithWait(0)", res, err)
			}
			wantCalls := 1
			if tt.sendErr != nil || tt.noHook {
				wantCalls = 0
			}
			if callbackCalls != wantCalls {
				t.Fatalf("callback calls=%d, want %d", callbackCalls, wantCalls)
			}
		})
	}
}
