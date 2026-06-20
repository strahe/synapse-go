package storage

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"

	fwssbind "github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakeFWSSTerminator struct {
	gotDataSetID types.BigInt
	res          *types.WriteResult
	err          error
	called       bool
}

func (f *fakeFWSSTerminator) TerminateDataSet(_ context.Context, id types.BigInt, _ ...warmstorage.WriteOption) (*types.WriteResult, error) {
	f.called = true
	f.gotDataSetID = id
	return f.res, f.err
}

func TestContext_Terminate_NotConfigured(t *testing.T) {
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.Terminate(context.Background()); err == nil {
		t.Fatal("expected error when terminator not configured")
	}
}

func TestContext_Terminate_Passthrough(t *testing.T) {
	term := &fakeFWSSTerminator{res: &types.WriteResult{Hash: common.HexToHash("0xdead")}}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(123)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	res, err := c.Terminate(context.Background())
	if err != nil {
		t.Fatalf("Terminate: %v", err)
	}
	if !term.called || !term.gotDataSetID.Equal(types.NewBigInt(123)) {
		t.Fatalf("terminator not invoked with expected id: called=%v id=%s", term.called, term.gotDataSetID.String())
	}
	if res == nil || res.Hash == (common.Hash{}) {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestContext_Terminate_PropagatesError(t *testing.T) {
	term := &fakeFWSSTerminator{err: errors.New("terminate failed")}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.Terminate(context.Background()); err == nil {
		t.Fatal("expected error from terminator")
	}
}

func TestContext_TerminateService_ProviderRelay(t *testing.T) {
	s := mustTestSigner(t)
	payer := s.EVMAddress()
	dataSetID := types.NewBigInt(7)
	txHash := common.HexToHash("0x1234")
	pay := &fakeTerminationPaymentReader{
		account: &payments.AccountState{
			Funds:         big.NewInt(100),
			LockupCurrent: new(big.Int),
			LockupRate:    new(big.Int),
		},
	}
	client := &fakePDPProviderClient{
		terminateServiceFn: func(_ context.Context, req pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error) {
			if !req.DataSetID.Equal(dataSetID) {
				t.Fatalf("DataSetID=%s want %s", req.DataSetID.String(), dataSetID.String())
			}
			if len(req.ExtraData) == 0 {
				t.Fatal("extraData is empty")
			}
			return &pdp.TerminateServiceResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitTerminateFn: func(_ context.Context, got types.BigInt, poll time.Duration, onHash func(common.Hash)) (*pdp.TerminateServiceStatus, error) {
			if !got.Equal(dataSetID) {
				t.Fatalf("wait DataSetID=%s want %s", got.String(), dataSetID.String())
			}
			if poll != time.Millisecond {
				t.Fatalf("poll=%s want 1ms", poll)
			}
			onHash(txHash)
			return &pdp.TerminateServiceStatus{
				TerminationTxHash:       &txHash,
				FWSSTerminated:          true,
				ServiceTerminationEpoch: 456,
			}, nil
		},
	}
	c, err := NewContext(testProvider(), client, s,
		WithPayer(payer),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(dataSetID),
		WithPaymentStateReader(pay, fakeTerminationEpochReader{block: 10}, common.HexToAddress("0x9999")),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var submitted common.Hash
	res, err := c.TerminateService(context.Background(), &TerminateServiceOptions{
		PollInterval: time.Millisecond,
		OnSubmitted: func(hash common.Hash) {
			submitted = hash
		},
	})
	if err != nil {
		t.Fatalf("TerminateService: %v", err)
	}
	if submitted != txHash {
		t.Fatalf("submitted=%s want %s", submitted, txHash)
	}
	if res.TxHash == nil || *res.TxHash != txHash || res.EndEpoch != 456 {
		t.Fatalf("result=%+v want tx hash and end epoch", res)
	}
	if pay.owner != payer {
		t.Fatalf("payment owner=%s want %s", pay.owner, payer)
	}
}

func TestContext_TerminateService_ProviderWaitTimeout(t *testing.T) {
	s := mustTestSigner(t)
	dataSetID := types.NewBigInt(7)
	client := &fakePDPProviderClient{
		terminateServiceFn: func(context.Context, pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error) {
			return &pdp.TerminateServiceResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitTerminateFn: func(ctx context.Context, _ types.BigInt, _ time.Duration, _ func(common.Hash)) (*pdp.TerminateServiceStatus, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	c, err := NewContext(testProvider(), client, s,
		WithPayer(s.EVMAddress()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(dataSetID),
		WithPaymentStateReader(&fakeTerminationPaymentReader{account: &payments.AccountState{}}, fakeTerminationEpochReader{block: 10}, common.HexToAddress("0x9999")),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = c.TerminateService(context.Background(), &TerminateServiceOptions{
		ProviderWaitTimeout: time.Nanosecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%T %v want context deadline", err, err)
	}
}

func TestContext_TerminateService_DebtPrecheckSkipsProvider(t *testing.T) {
	s := mustTestSigner(t)
	calledProvider := false
	c, err := NewContext(testProvider(), &fakePDPProviderClient{
		terminateServiceFn: func(context.Context, pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error) {
			calledProvider = true
			return nil, nil
		},
	}, s,
		WithPayer(s.EVMAddress()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(7)),
		WithPaymentStateReader(&fakeTerminationPaymentReader{
			account: &payments.AccountState{
				Funds:         new(big.Int),
				LockupCurrent: big.NewInt(1),
				LockupRate:    new(big.Int),
			},
		}, fakeTerminationEpochReader{block: 10}, common.HexToAddress("0x9999")),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = c.TerminateService(context.Background(), nil)
	var debtErr *TerminateServiceDebtError
	if !errors.As(err, &debtErr) {
		t.Fatalf("err=%T %v want TerminateServiceDebtError", err, err)
	}
	if calledProvider {
		t.Fatal("provider should not be called when debt pre-check fails")
	}
}

func TestContext_TerminateService_ResumesPendingRequest(t *testing.T) {
	s := mustTestSigner(t)
	dataSetID := types.NewBigInt(7)
	client := &fakePDPProviderClient{
		terminateServiceFn: func(context.Context, pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error) {
			return nil, &pdp.TerminateServicePendingError{Message: "queued"}
		},
		waitTerminateFn: func(context.Context, types.BigInt, time.Duration, func(common.Hash)) (*pdp.TerminateServiceStatus, error) {
			return &pdp.TerminateServiceStatus{FWSSTerminated: true, ServiceTerminationEpoch: 99}, nil
		},
	}
	c, err := NewContext(testProvider(), client, s,
		WithPayer(s.EVMAddress()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(dataSetID),
		WithPaymentStateReader(&fakeTerminationPaymentReader{account: &payments.AccountState{}}, fakeTerminationEpochReader{block: 10}, common.HexToAddress("0x9999")),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	res, err := c.TerminateService(context.Background(), nil)
	if err != nil {
		t.Fatalf("TerminateService: %v", err)
	}
	if res.EndEpoch != 99 {
		t.Fatalf("EndEpoch=%d want 99", res.EndEpoch)
	}
}

func TestContext_TerminateService_SkipProviderParsesReceiptEvent(t *testing.T) {
	dataSetID := types.NewBigInt(7)
	txHash := common.HexToHash("0x1234")
	term := &fakeFWSSTerminator{res: &types.WriteResult{
		Hash:    txHash,
		Receipt: terminateReceipt(t, dataSetID, 456, 99),
	}}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(dataSetID),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var submitted common.Hash
	res, err := c.TerminateService(context.Background(), &TerminateServiceOptions{
		SkipProvider: true,
		OnSubmitted: func(hash common.Hash) {
			submitted = hash
		},
	})
	if err != nil {
		t.Fatalf("TerminateService: %v", err)
	}
	if submitted != txHash {
		t.Fatalf("submitted=%s want %s", submitted, txHash)
	}
	if res.TxHash == nil || *res.TxHash != txHash || res.EndEpoch != 456 {
		t.Fatalf("result=%+v want tx hash and end epoch", res)
	}
}

func TestService_TerminateService_RejectsDataSetOwnedByDifferentPayer(t *testing.T) {
	s := mustTestSigner(t)
	providers := &fakeTerminationProviderResolver{}
	mgr := mustNewService(t, Options{
		FWSSDataSetReader: fakeTerminationDataSetReader{
			info: &warmstorage.DataSetInfo{
				DataSetID:  types.NewBigInt(7),
				ProviderID: types.NewBigInt(1),
				Payer:      common.HexToAddress("0x1111111111111111111111111111111111111111"),
			},
		},
		ProviderResolver:   providers,
		PaymentStateReader: &fakeTerminationPaymentReader{account: &payments.AccountState{}},
		EpochReader:        fakeTerminationEpochReader{block: 10},
		PaymentToken:       common.HexToAddress("0x9999"),
		Signer:             s,
		SignerAddress:      s.EVMAddress(),
		ChainID:            types.ChainID(314159),
		RecordKeeper:       testRecordKeeper(),
	})

	_, err := mgr.TerminateService(context.Background(), types.NewBigInt(7), nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
	if providers.called {
		t.Fatal("provider resolver should not be called for a different payer")
	}
}

func TestService_TerminateService_DerivesSignerAddress(t *testing.T) {
	s := mustTestSigner(t)
	provider := Provider{
		ID:              types.NewBigInt(1),
		ServiceURL:      "file:///bad",
		ServiceProvider: testProvider().ServiceProvider,
		Payee:           testProvider().Payee,
	}
	providers := &fakeTerminationProviderResolver{provider: &provider}
	mgr := mustNewService(t, Options{
		FWSSDataSetReader: fakeTerminationDataSetReader{
			info: &warmstorage.DataSetInfo{
				DataSetID:  types.NewBigInt(7),
				ProviderID: types.NewBigInt(1),
				Payer:      s.EVMAddress(),
			},
		},
		ProviderResolver:   providers,
		PaymentStateReader: &fakeTerminationPaymentReader{account: &payments.AccountState{}},
		EpochReader:        fakeTerminationEpochReader{block: 10},
		PaymentToken:       common.HexToAddress("0x9999"),
		Signer:             s,
		ChainID:            types.ChainID(314159),
		RecordKeeper:       testRecordKeeper(),
	})

	_, err := mgr.TerminateService(context.Background(), types.NewBigInt(7), nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("err=%v want provider client creation error", err)
	}
	if !providers.called {
		t.Fatal("provider resolver should be called after signer owner check passes")
	}
}

func TestContext_Terminate_TypedNilTerminatorTreatedAsUnset(t *testing.T) {
	var term *fakeFWSSTerminator

	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Terminate panicked with typed-nil terminator: %v", r)
		}
	}()

	_, err = c.Terminate(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err=%v want not configured", err)
	}
}

type fakeTerminationPaymentReader struct {
	account *payments.AccountState
	owner   common.Address
}

func (f *fakeTerminationPaymentReader) AccountInfo(_ context.Context, _, owner common.Address) (*payments.AccountState, error) {
	f.owner = owner
	return f.account, nil
}

type fakeTerminationEpochReader struct {
	block uint64
}

func (f fakeTerminationEpochReader) BlockNumber(context.Context) (uint64, error) {
	return f.block, nil
}

type fakeTerminationDataSetReader struct {
	info *warmstorage.DataSetInfo
}

func (f fakeTerminationDataSetReader) GetDataSet(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) {
	return f.info, nil
}

type fakeTerminationProviderResolver struct {
	called   bool
	provider *Provider
}

func (f *fakeTerminationProviderResolver) ResolveProvider(context.Context, types.BigInt) (Provider, error) {
	f.called = true
	if f.provider != nil {
		return *f.provider, nil
	}
	return testProvider(), nil
}

func terminateReceipt(t *testing.T, dataSetID types.BigInt, endEpoch uint64, pdpRailID int64) *coretypes.Receipt {
	t.Helper()
	contractABI, err := fwssbind.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatalf("FWSS ABI: %v", err)
	}
	event := contractABI.Events["PDPPaymentTerminated"]
	data, err := event.Inputs.NonIndexed().Pack(big.NewInt(int64(endEpoch)), big.NewInt(pdpRailID))
	if err != nil {
		t.Fatalf("pack PDPPaymentTerminated: %v", err)
	}
	log := &coretypes.Log{
		Topics: []common.Hash{
			event.ID,
			common.BigToHash(dataSetID.Big()),
		},
		Data: data,
	}
	return &coretypes.Receipt{Logs: []*coretypes.Log{log}}
}

func TestContext_Terminate_CanRunWhileDataSetCreationCompletes(t *testing.T) {
	dataSetID := types.NewBigInt(123)
	clientDataSetID := types.NewBigInt(456)
	txHash := common.HexToHash("0x1234")
	submission := CreateDataSetSubmission{
		TransactionID:   txHash.Hex(),
		StatusURL:       "https://sp.example.com/status",
		ClientDataSetID: &clientDataSetID,
	}
	client := &fakePDPProviderClient{
		waitForCreatedFn: func(_ context.Context, gotStatusURL string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			if gotStatusURL != submission.StatusURL {
				return nil, errors.New("unexpected status URL")
			}
			id := dataSetID
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: txHash,
				DataSetID:         &id,
			}, nil
		},
	}
	term := &fakeFWSSTerminator{res: &types.WriteResult{Hash: common.HexToHash("0xdead")}}
	c, err := NewContext(testProvider(), client, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	waitErr := make(chan error, 1)
	go func() {
		for i := 0; i < 2000; i++ {
			if _, err := c.WaitForDataSetCreated(context.Background(), submission); err != nil {
				waitErr <- err
				return
			}
		}
		waitErr <- nil
	}()

	for i := 0; i < 2000; i++ {
		_, err := c.Terminate(context.Background())
		if err != nil && !strings.Contains(err.Error(), "dataSetID not set") {
			t.Fatalf("Terminate: %v", err)
		}
	}
	if err := <-waitErr; err != nil {
		t.Fatalf("WaitForDataSetCreated: %v", err)
	}
	if _, err := c.Terminate(context.Background()); err != nil {
		t.Fatalf("Terminate after WaitForDataSetCreated: %v", err)
	}
}
