package storage

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	coretypes "github.com/ethereum/go-ethereum/core/types"

	fwssbind "github.com/strahe/synapse-go/internal/contracts/fwss"
	ityped "github.com/strahe/synapse-go/internal/typeddata"
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
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(types.NewBigInt(1), types.BigInt{}),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(types.NewBigInt(123), types.BigInt{}),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(types.NewBigInt(1), types.BigInt{}),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	confirmedTxHash := common.HexToHash("0x5678")
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
				ConfirmedTxHash:         &confirmedTxHash,
				FWSSTerminated:          true,
				ServiceTerminationEpoch: 456,
			}, nil
		},
	}
	c, err := NewDataSetContext(testProvider(), client, s, testDataSetRef(dataSetID, types.BigInt{}),
		WithPayer(payer),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	if res.TxHash == nil || *res.TxHash != txHash || res.ConfirmedTxHash == nil || *res.ConfirmedTxHash != confirmedTxHash || res.EndEpoch != 456 {
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
	c, err := NewDataSetContext(testProvider(), client, s, testDataSetRef(dataSetID, types.BigInt{}),
		WithPayer(s.EVMAddress()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{
		terminateServiceFn: func(context.Context, pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error) {
			calledProvider = true
			return nil, nil
		},
	}, s, testDataSetRef(types.NewBigInt(7), types.BigInt{}),
		WithPayer(s.EVMAddress()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	if _, ok := errors.AsType[*TerminateServiceDebtError](err); !ok {
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
	c, err := NewDataSetContext(testProvider(), client, s, testDataSetRef(dataSetID, types.BigInt{}),
		WithPayer(s.EVMAddress()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	if res.TxHash != nil || res.ConfirmedTxHash != nil {
		t.Fatalf("result hashes=%v/%v want nil for an older provider response", res.TxHash, res.ConfirmedTxHash)
	}
}

func TestContext_TerminateService_SkipProviderParsesReceiptEvent(t *testing.T) {
	dataSetID := types.NewBigInt(7)
	txHash := common.HexToHash("0x1234")
	term := &fakeFWSSTerminator{res: &types.WriteResult{
		Hash:    txHash,
		Receipt: terminateReceipt(t, dataSetID, txHash, 456, 99),
	}}
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(dataSetID, types.BigInt{}),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	if res.TxHash == nil || *res.TxHash != txHash || res.ConfirmedTxHash == nil || *res.ConfirmedTxHash != txHash || res.EndEpoch != 456 {
		t.Fatalf("result=%+v want tx hash and end epoch", res)
	}
}

func TestContext_TerminateService_SkipProviderRejectsZeroReceiptHash(t *testing.T) {
	dataSetID := types.NewBigInt(7)
	term := &fakeFWSSTerminator{res: &types.WriteResult{
		Hash:    common.HexToHash("0x1234"),
		Receipt: terminateReceipt(t, dataSetID, common.Hash{}, 456, 99),
	}}
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(dataSetID, types.BigInt{}),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = c.TerminateService(context.Background(), &TerminateServiceOptions{SkipProvider: true})
	if err == nil || !strings.Contains(err.Error(), "zero transaction hash") {
		t.Fatalf("err=%v want zero receipt transaction hash error", err)
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
		PayerAddress:       s.EVMAddress(),
		ChainID:            types.ChainID(314159),
		RecordKeeper:       testRecordKeeper(),
	})

	_, err := mgr.TerminateService(context.Background(), types.NewBigInt(7), nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "configured payer") {
		t.Fatalf("err=%v want configured payer", err)
	}
	if providers.called {
		t.Fatal("provider resolver should not be called for a different payer")
	}
}

func TestService_TerminateService_DerivesPayerAddress(t *testing.T) {
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

func TestService_TerminateService_ProviderRelay(t *testing.T) {
	storageSigner := mustTestSigner(t)
	payer := testPayer()
	dataSetID := types.NewBigInt(7)
	txHash := common.HexToHash("0x1234")
	var relayedExtraData []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pdp/data-sets/7/terminate" {
			t.Errorf("request path = %s", r.URL.Path)
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		switch r.Method {
		case http.MethodPost:
			var request struct {
				ExtraData string `json:"extraData"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Errorf("decode terminate request: %v", err)
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			var err error
			relayedExtraData, err = hexutil.Decode(request.ExtraData)
			if err != nil {
				t.Errorf("decode extraData: %v", err)
				http.Error(w, "invalid extraData", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"terminationTxHash":"` + txHash.Hex() + `","fwssTerminated":true,"serviceTerminationEpoch":456}`))
		default:
			t.Errorf("request method = %s", r.Method)
			http.Error(w, "unexpected method", http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(server.Close)

	provider := testProvider()
	provider.ServiceURL = server.URL
	providers := &fakeTerminationProviderResolver{provider: &provider}
	paymentReader := &fakeTerminationPaymentReader{account: &payments.AccountState{
		Funds:               big.NewInt(100),
		LockupCurrent:       new(big.Int),
		LockupRate:          new(big.Int),
		LockupLastSettledAt: new(big.Int),
	}}
	mgr := mustNewService(t, Options{
		HTTPClient: server.Client(),
		FWSSDataSetReader: fakeTerminationDataSetReader{info: &warmstorage.DataSetInfo{
			DataSetID:  dataSetID,
			ProviderID: provider.ID,
			Payer:      payer,
		}},
		ProviderResolver:   providers,
		PaymentStateReader: paymentReader,
		EpochReader:        fakeTerminationEpochReader{block: 10},
		PaymentToken:       common.HexToAddress("0x9999"),
		Signer:             storageSigner,
		PayerAddress:       payer,
		ChainID:            types.ChainID(314159),
		RecordKeeper:       testRecordKeeper(),
	})

	var submitted common.Hash
	res, err := mgr.TerminateService(context.Background(), dataSetID, &TerminateServiceOptions{
		PollInterval: time.Nanosecond,
		OnSubmitted: func(hash common.Hash) {
			submitted = hash
		},
	})
	if err != nil {
		t.Fatalf("TerminateService: %v", err)
	}
	if !providers.called || submitted != txHash || res == nil || res.TxHash == nil || *res.TxHash != txHash || res.EndEpoch != 456 {
		t.Fatalf("TerminateService result=%+v submitted=%s providerCalled=%v", res, submitted, providers.called)
	}
	if paymentReader.owner != payer {
		t.Fatalf("payment owner=%s want payer %s", paymentReader.owner, payer)
	}
	values, err := bytesArgs.Unpack(relayedExtraData)
	if err != nil {
		t.Fatalf("unpack termination extraData: %v", err)
	}
	if len(values) != 1 {
		t.Fatalf("termination extraData values=%v", values)
	}
	domain := ityped.NewDomain(big.NewInt(314159), testRecordKeeper())
	recovered := recoverRawTypedDataSigner(t, domain, "TerminateService", ityped.TerminateServiceMessage(dataSetID.Big()), values[0].([]byte))
	if recovered != storageSigner.EVMAddress() {
		t.Fatalf("termination signer=%s want %s", recovered, storageSigner.EVMAddress())
	}
}

func TestContext_Terminate_TypedNilTerminatorTreatedAsUnset(t *testing.T) {
	var term *fakeFWSSTerminator

	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(types.NewBigInt(1), types.BigInt{}),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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

func terminateReceipt(t *testing.T, dataSetID types.BigInt, txHash common.Hash, endEpoch uint64, pdpRailID int64) *coretypes.Receipt {
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
	return &coretypes.Receipt{TxHash: txHash, Logs: []*coretypes.Log{log}}
}
