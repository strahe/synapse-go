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
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fakeFWSSTerminator struct {
	gotDataSetID types.BigInt
	gotOptions   FWSSTerminationOptions
	res          *types.WriteResult
	err          error
	called       bool
	terminateFn  func(context.Context, types.BigInt, FWSSTerminationOptions) (*types.WriteResult, error)
}

func (f *fakeFWSSTerminator) TerminateDataSet(ctx context.Context, id types.BigInt, opts FWSSTerminationOptions) (*types.WriteResult, error) {
	f.called = true
	f.gotDataSetID = id
	f.gotOptions = opts
	if f.terminateFn != nil {
		return f.terminateFn(ctx, id, opts)
	}
	return f.res, f.err
}

func TestContext_TerminateService_SkipProviderNotConfigured(t *testing.T) {
	var typedNil *fakeFWSSTerminator
	tests := []struct {
		name       string
		terminator FWSSTerminator
	}{
		{"nil", nil},
		{"typed nil", typedNil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, nil, testDataSetRef(types.NewBigInt(1), types.BigInt{}),
				WithFWSSTerminator(tt.terminator),
			)
			if err != nil {
				t.Fatalf("NewDataSetContext: %v", err)
			}
			if _, err := c.TerminateService(context.Background(), &TerminateServiceOptions{SkipProvider: true}); !errors.Is(err, ErrUninitialized) {
				t.Fatalf("TerminateService error = %v, want ErrUninitialized", err)
			}
		})
	}
}

func TestContext_TerminateService_SkipProviderPropagatesError(t *testing.T) {
	want := errors.New("terminate failed")
	term := &fakeFWSSTerminator{
		res: &types.WriteResult{Hash: common.HexToHash("0xdead")},
		err: want,
	}
	c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), testDataSetRef(types.NewBigInt(1), types.BigInt{}),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithFWSSTerminator(term),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	res, err := c.TerminateService(context.Background(), &TerminateServiceOptions{
		SkipProvider: true,
	})
	if !errors.Is(err, want) || res != nil {
		t.Fatalf("TerminateService = %+v, %v, want nil result and wrapped error", res, err)
	}
}

func TestTerminateService_DirectSubmissionNotification(t *testing.T) {
	broadcastErr := errors.New("broadcast rejected")
	tests := []struct {
		name         string
		sendErr      error
		waitErr      error
		cancelOnHash bool
	}{
		{name: "confirmed"},
		{name: "broadcast rejected", sendErr: broadcastErr},
		{name: "receipt timeout", waitErr: context.DeadlineExceeded},
		{name: "canceled after submission", cancelOnHash: true, waitErr: context.Canceled},
		{name: "reverted transaction", waitErr: types.ErrTxFailed},
	}
	for _, api := range []string{"service", "data set context"} {
		t.Run(api, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					dataSetID := types.NewBigInt(7)
					submittedHash := common.HexToHash("0x1234")
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					callbackCalls := 0
					broadcast := false
					term := &fakeFWSSTerminator{
						terminateFn: func(ctx context.Context, id types.BigInt, opts FWSSTerminationOptions) (*types.WriteResult, error) {
							if !id.Equal(dataSetID) || opts.WaitTimeout != 3*time.Minute || opts.OnSubmitted == nil {
								t.Fatalf("dependency arguments: id=%s wait=%s callback=%v", id, opts.WaitTimeout, opts.OnSubmitted != nil)
							}
							if tt.sendErr != nil {
								return nil, tt.sendErr
							}
							broadcast = true
							opts.OnSubmitted(submittedHash)
							if callbackCalls != 1 {
								t.Fatalf("confirmation started before submission notification: calls=%d", callbackCalls)
							}
							res := &types.WriteResult{Hash: submittedHash}
							if tt.cancelOnHash && !errors.Is(ctx.Err(), context.Canceled) {
								t.Fatal("submission callback did not cancel the caller context")
							}
							if tt.waitErr != nil {
								return res, tt.waitErr
							}
							res.Receipt = terminateReceipt(t, id, submittedHash, 456, 99)
							return res, nil
						},
					}
					var terminate func(context.Context, *TerminateServiceOptions) (*TerminateServiceResult, error)
					if api == "service" {
						svc := mustNewService(t, Options{DataSetTerminator: term})
						terminate = func(ctx context.Context, opts *TerminateServiceOptions) (*TerminateServiceResult, error) {
							return svc.TerminateService(ctx, dataSetID, opts)
						}
					} else {
						c, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{}, nil,
							testDataSetRef(dataSetID, types.BigInt{}), WithFWSSTerminator(term))
						if err != nil {
							t.Fatal(err)
						}
						terminate = c.TerminateService
					}
					res, err := terminate(ctx, &TerminateServiceOptions{
						SkipProvider:      true,
						DirectWaitTimeout: 3 * time.Minute,
						OnSubmitted: func(hash common.Hash) {
							callbackCalls++
							if !broadcast || hash != submittedHash {
								t.Fatalf("callback before broadcast or wrong hash: %s", hash)
							}
							if tt.cancelOnHash {
								cancel()
							}
						},
					})
					wantErr := tt.waitErr
					if tt.sendErr != nil {
						wantErr = tt.sendErr
					}
					if wantErr != nil {
						if !errors.Is(err, wantErr) || res != nil {
							t.Fatalf("TerminateService = %+v, %v, want nil result and %v", res, err, wantErr)
						}
					} else if err != nil || res == nil || res.TxHash == nil || *res.TxHash != submittedHash || res.EndEpoch != 456 {
						t.Fatalf("TerminateService = %+v, %v", res, err)
					}
					wantCalls := 1
					if tt.sendErr != nil {
						wantCalls = 0
					}
					if callbackCalls != wantCalls {
						t.Fatalf("callback calls=%d, want %d", callbackCalls, wantCalls)
					}
				})
			}
		})
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

func TestContext_TerminateService_SkipProviderPreservesSubmittedAndConfirmedHashes(t *testing.T) {
	dataSetID := types.NewBigInt(7)
	submittedHash := common.HexToHash("0x1234")
	confirmedHash := common.HexToHash("0x5678")
	term := &fakeFWSSTerminator{res: &types.WriteResult{
		Hash:    submittedHash,
		Receipt: terminateReceipt(t, dataSetID, confirmedHash, 456, 99),
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

	res, err := c.TerminateService(context.Background(), &TerminateServiceOptions{
		SkipProvider: true,
	})
	if err != nil {
		t.Fatalf("TerminateService: %v", err)
	}
	if !term.called || !term.gotDataSetID.Equal(dataSetID) || !res.DataSetID.Equal(dataSetID) {
		t.Fatalf("termination target=%s result=%+v, want data set %s", term.gotDataSetID, res, dataSetID)
	}
	if res.TxHash == nil || *res.TxHash != submittedHash || res.ConfirmedTxHash == nil || *res.ConfirmedTxHash != confirmedHash || res.EndEpoch != 456 {
		t.Fatalf("result=%+v want submitted and confirmed hashes with end epoch", res)
	}
}

func TestContext_TerminateService_SkipProviderFallsBackFromZeroReceiptHash(t *testing.T) {
	dataSetID := types.NewBigInt(7)
	submittedHash := common.HexToHash("0x1234")
	term := &fakeFWSSTerminator{res: &types.WriteResult{
		Hash:    submittedHash,
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

	res, err := c.TerminateService(context.Background(), &TerminateServiceOptions{SkipProvider: true})
	if err != nil {
		t.Fatalf("TerminateService: %v", err)
	}
	if res.TxHash == nil || *res.TxHash != submittedHash || res.ConfirmedTxHash == nil || *res.ConfirmedTxHash != submittedHash {
		t.Fatalf("result=%+v want submitted hash fallback", res)
	}
}

func TestService_TerminateService_SkipProviderNeedsOnlyTerminator(t *testing.T) {
	tests := []struct {
		name    string
		timeout time.Duration
		wait    time.Duration
	}{
		{"explicit timeout", time.Second, time.Second},
		{"default timeout", 0, 5 * time.Minute},
		{"negative timeout", -time.Second, 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataSetID := types.NewBigInt(23)
			hash := common.HexToHash("0x1234")
			term := &fakeFWSSTerminator{res: &types.WriteResult{
				Hash:    hash,
				Receipt: terminateReceipt(t, dataSetID, hash, 456, 99),
			}}
			svc := mustNewService(t, Options{DataSetTerminator: term})
			res, err := svc.TerminateService(context.Background(), dataSetID, &TerminateServiceOptions{
				SkipProvider:      true,
				DirectWaitTimeout: tt.timeout,
			})
			if err != nil {
				t.Fatalf("TerminateService: %v", err)
			}
			if !term.called || !term.gotDataSetID.Equal(dataSetID) || term.gotOptions.WaitTimeout != tt.wait {
				t.Fatalf("termination target=%s called=%v wait=%s, want %s and %s wait", term.gotDataSetID, term.called, term.gotOptions.WaitTimeout, dataSetID, tt.wait)
			}
			if res == nil || !res.DataSetID.Equal(dataSetID) || res.EndEpoch != 456 || res.TxHash == nil || *res.TxHash != hash || res.ConfirmedTxHash == nil || *res.ConfirmedTxHash != hash {
				t.Fatalf("TerminateService result=%+v, want confirmed data set %s with end epoch 456", res, dataSetID)
			}
		})
	}
}

func TestService_TerminateService_SkipProviderValidatesConfigurationAndID(t *testing.T) {
	var typedNil *fakeFWSSTerminator
	tests := []struct {
		name      string
		service   *Service
		dataSetID types.BigInt
		want      error
	}{
		{"uninitialized service", &Service{}, types.NewBigInt(1), ErrUninitialized},
		{"nil terminator", mustNewService(t, Options{}), types.NewBigInt(1), ErrUninitialized},
		{"typed-nil terminator", mustNewService(t, Options{DataSetTerminator: typedNil}), types.NewBigInt(1), ErrUninitialized},
		{"zero data set ID", mustNewService(t, Options{DataSetTerminator: &fakeFWSSTerminator{}}), types.BigInt{}, ErrInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := tt.service.TerminateService(context.Background(), tt.dataSetID, &TerminateServiceOptions{SkipProvider: true})
			if !errors.Is(err, tt.want) || res != nil {
				t.Fatalf("TerminateService = %+v, %v, want nil result and %v", res, err, tt.want)
			}
		})
	}
}

func TestService_TerminateService_SkipProviderRequiresReceipt(t *testing.T) {
	tests := []struct {
		name   string
		result *types.WriteResult
	}{
		{"nil result", nil},
		{"submission hash only", &types.WriteResult{Hash: common.HexToHash("0x1234")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := mustNewService(t, Options{DataSetTerminator: &fakeFWSSTerminator{res: tt.result}})
			res, err := svc.TerminateService(context.Background(), types.NewBigInt(1), &TerminateServiceOptions{SkipProvider: true})
			if err == nil || res != nil {
				t.Fatalf("TerminateService = %+v, %v, want error without a confirmed result", res, err)
			}
		})
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
	storageSigner := &storageSignerOnly{inner: mustTestSigner(t)}
	if _, ok := any(storageSigner).(signer.EVMSigner); ok {
		t.Fatal("storage-only signer unexpectedly implements EVMSigner")
	}
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
		AllowPrivateNetworks: true,
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
	// A deliberately unusable download timeout proves provider control calls use
	// the separate 30-second client rather than the long-running download client.
	mgr.httpClient.Timeout = time.Nanosecond

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

func (f fakeTerminationDataSetReader) FindDataSetByClientDataSetID(
	context.Context,
	common.Address,
	types.BigInt,
) (*warmstorage.DataSetInfo, error) {
	return nil, errors.New("unexpected FindDataSetByClientDataSetID")
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
