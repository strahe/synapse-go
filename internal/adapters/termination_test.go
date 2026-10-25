package adapters

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/contracts/fwss"
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

func TestFWSSTerminator_SubmitsWithoutWaiting(t *testing.T) {
	for _, sendErr := range []error{nil, errors.New("broadcast rejected")} {
		t.Run(fmt.Sprint(sendErr), func(t *testing.T) {
			backend := &directTerminationBackend{sendErr: sendErr}
			backend.receiptFn = func(context.Context, common.Hash) (*coretypes.Receipt, error) {
				t.Fatal("submission started receipt polling")
				return nil, nil
			}
			term := NewFWSSTerminator(newDirectTerminationTestService(t, backend))
			res, err := term.TerminateDataSet(context.Background(), types.NewBigInt(7), storage.FWSSTerminationOptions{
				WriteOptions: []warmstorage.WriteOption{
					warmstorage.WithWait(time.Second),
					warmstorage.WithOnSubmitted(func(common.Hash) { t.Fatal("submission invoked write-option callback") }),
				},
			})
			if sendErr != nil {
				if res != nil || !errors.Is(err, sendErr) {
					t.Fatalf("TerminateDataSet = %+v, %v, want broadcast error", res, err)
				}
				return
			}
			if err != nil || res == nil || res.Receipt != nil || backend.submitted == nil || res.Hash != backend.submitted.Hash() {
				t.Fatalf("TerminateDataSet = %+v, %v, want broadcast-only result", res, err)
			}
		})
	}
}

func (b *directTerminationBackend) BlockNumber(context.Context) (uint64, error) {
	return 1, nil
}

func TestFWSSTerminator_WaitForTermination(t *testing.T) {
	tests := []struct {
		name          string
		address       common.Address
		dataSetID     types.BigInt
		status        uint64
		pending       bool
		cancel        bool
		confirmations uint64
		wantErr       error
	}{
		{name: "confirmed", address: common.HexToAddress("0x1111"), dataSetID: types.NewBigInt(7), status: coretypes.ReceiptStatusSuccessful},
		{name: "wrong contract", address: common.HexToAddress("0x9999"), dataSetID: types.NewBigInt(7), status: coretypes.ReceiptStatusSuccessful},
		{name: "wrong data set", address: common.HexToAddress("0x1111"), dataSetID: types.NewBigInt(99), status: coretypes.ReceiptStatusSuccessful},
		{name: "failed transaction", status: coretypes.ReceiptStatusFailed, wantErr: types.ErrTxFailed},
		{name: "receipt timeout", pending: true, wantErr: types.ErrReceiptTimeout},
		{name: "canceled", cancel: true, wantErr: context.Canceled},
		{name: "unconfirmed receipt is not returned", address: common.HexToAddress("0x1111"), dataSetID: types.NewBigInt(7), status: coretypes.ReceiptStatusSuccessful, confirmations: 3, wantErr: types.ErrReceiptTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancel {
				cancel()
			}
			hash := common.HexToHash("0x1234")
			backend := &directTerminationBackend{}
			backend.receiptFn = func(context.Context, common.Hash) (*coretypes.Receipt, error) {
				if tt.pending {
					return nil, ethereum.NotFound
				}
				receipt := terminationTestReceipt(t, tt.address, tt.dataSetID, hash)
				receipt.Status = tt.status
				return receipt, nil
			}
			term := NewFWSSTerminator(newDirectTerminationTestService(t, backend))
			res, epoch, err := term.WaitForTermination(ctx, types.NewBigInt(7), hash, 25*time.Millisecond, []warmstorage.WriteOption{
				warmstorage.WithWait(0),
				warmstorage.WithConfirmations(tt.confirmations),
				warmstorage.WithOnSubmitted(func(common.Hash) { t.Fatal("waiting invoked submission callback") }),
			})
			if backend.submitted != nil {
				t.Fatal("waiting submitted a transaction")
			}
			if res == nil || res.Hash != hash {
				t.Fatalf("WaitForTermination lost hash: %+v, %v", res, err)
			}
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || epoch != 0 {
					t.Fatalf("WaitForTermination = %+v, %d, %v, want %v", res, epoch, err, tt.wantErr)
				}
				if wantReceipt := errors.Is(tt.wantErr, types.ErrTxFailed); (res.Receipt != nil) != wantReceipt {
					t.Fatalf("receipt=%v, want a receipt only for a confirmed failure", res.Receipt)
				}
				return
			}
			if tt.name != "confirmed" {
				if err == nil || epoch != 0 || res.Receipt == nil {
					t.Fatalf("unmatched event = %+v, %d, %v", res, epoch, err)
				}
				return
			}
			if err != nil || epoch != 456 || res.Receipt == nil {
				t.Fatalf("WaitForTermination = %+v, %d, %v", res, epoch, err)
			}
		})
	}
}

func terminationTestReceipt(t *testing.T, address common.Address, id types.BigInt, hash common.Hash) *coretypes.Receipt {
	t.Helper()
	contractABI, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	event := contractABI.Events["PDPPaymentTerminated"]
	data, err := event.Inputs.NonIndexed().Pack(big.NewInt(456), big.NewInt(99))
	if err != nil {
		t.Fatal(err)
	}
	return &coretypes.Receipt{
		Status:      coretypes.ReceiptStatusSuccessful,
		TxHash:      hash,
		BlockNumber: big.NewInt(1),
		Logs:        []*coretypes.Log{{Address: address, Topics: []common.Hash{event.ID, common.BigToHash(id.Big())}, Data: data}},
	}
}
