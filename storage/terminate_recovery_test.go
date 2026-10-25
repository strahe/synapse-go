package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func terminationRecoveryContext(t *testing.T, client PDPProviderClient, opts ...ContextOption) *DataSetContext {
	t.Helper()
	c, err := NewDataSetContext(testProvider(), client, nil, testDataSetRef(types.NewBigInt(7), types.BigInt{}), opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestWaitForTerminateService_ValidatesBeforeObserving(t *testing.T) {
	hash := common.HexToHash("0xa").Hex()
	providerSub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID}
	directSub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), Direct: true, TransactionID: hash}
	cases := []struct {
		name string
		sub  TerminateServiceSubmission
	}{
		{"zero dataset", TerminateServiceSubmission{ProviderID: testProvider().ID}},
		{"other dataset", TerminateServiceSubmission{DataSetID: types.NewBigInt(8), ProviderID: testProvider().ID}},
		{"zero provider", TerminateServiceSubmission{DataSetID: types.NewBigInt(7)}},
		{"other provider", TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: types.NewBigInt(999)}},
		{"direct without hash", TerminateServiceSubmission{DataSetID: types.NewBigInt(7), Direct: true}},
		{"direct with provider", TerminateServiceSubmission{DataSetID: types.NewBigInt(7), Direct: true, ProviderID: testProvider().ID, TransactionID: hash}},
		{"direct queued", TerminateServiceSubmission{DataSetID: types.NewBigInt(7), Direct: true, Queued: true, TransactionID: hash}},
		{"invalid hash", func() TerminateServiceSubmission { s := providerSub; s.TransactionID = "0xa"; return s }()},
		{"zero confirmed hash", func() TerminateServiceSubmission {
			s := directSub
			s.ConfirmedTransactionID = (common.Hash{}).Hex()
			return s
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			client := &fakePDPProviderClient{waitTerminateFn: func(context.Context, types.BigInt, time.Duration, func(common.Hash), ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
				calls++
				return nil, errors.New("unexpected provider query")
			}}
			term := &fakeFWSSTerminator{}
			reader := terminationRecoveryReader{read: func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) {
				calls++
				return nil, errors.New("unexpected chain query")
			}}
			c := terminationRecoveryContext(t, client, WithFWSSTerminator(term), WithFWSSDataSetReader(reader))
			result, err := c.WaitForTerminateService(context.Background(), tc.sub, nil)
			var staged *TerminateServiceError
			if result != nil || !errors.Is(err, ErrInvalidArgument) || !errors.As(err, &staged) || staged.Stage != TerminateServiceStageWait || staged.Submission == nil {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if calls != 0 || term.called || term.waitCalls != 0 {
				t.Fatalf("invalid handle performed I/O: provider/reader=%d terminator=%+v", calls, term)
			}
		})
	}
}

func TestWaitForTerminateService_UsesLatestStatusHashes(t *testing.T) {
	a, b, c, d := common.HexToHash("0xa"), common.HexToHash("0xb"), common.HexToHash("0xc"), common.HexToHash("0xd")
	observationErr := errors.New("provider observation failed")
	cases := []struct {
		name       string
		original   *common.Hash
		confirmed  *common.Hash
		wantOrigin string
		wantFinal  string
	}{
		{"status pair replaces saved pair", &c, &d, c.Hex(), d.Hex()},
		{"status without final clears saved final", &c, nil, c.Hex(), ""},
		{"final only", nil, &d, "", d.Hex()},
		{"no hashes keeps saved pair", nil, nil, a.Hex(), b.Hex()},
	}
	for _, tc := range cases {
		for _, success := range []bool{false, true} {
			name := tc.name + "/error"
			if success {
				name = tc.name + "/success"
			}
			t.Run(name, func(t *testing.T) {
				input := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID, TransactionID: a.Hex(), ConfirmedTransactionID: b.Hex()}
				client := &fakePDPProviderClient{waitTerminateFn: func(context.Context, types.BigInt, time.Duration, func(common.Hash), ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
					status := &pdp.TerminateServiceStatus{TerminationTxHash: tc.original, ConfirmedTxHash: tc.confirmed, FWSSTerminated: success, ServiceTerminationEpoch: 42}
					if success {
						return status, nil
					}
					return status, observationErr
				}}
				result, err := terminationRecoveryContext(t, client).WaitForTerminateService(context.Background(), input, nil)
				if input.Accepted || input.TransactionID != a.Hex() || input.ConfirmedTransactionID != b.Hex() {
					t.Fatalf("input modified: %+v", input)
				}
				var gotOrigin, gotFinal string
				if success {
					if err != nil || result == nil || result.EndEpoch != 42 {
						t.Fatalf("result=%+v error=%v", result, err)
					}
					if result.TxHash != nil {
						gotOrigin = result.TxHash.Hex()
					}
					if result.ConfirmedTxHash != nil {
						gotFinal = result.ConfirmedTxHash.Hex()
					}
				} else {
					var staged *TerminateServiceError
					if result != nil || !errors.Is(err, observationErr) || !errors.As(err, &staged) || staged.Stage != TerminateServiceStageWait || staged.Submission == nil || !staged.Submission.Accepted {
						t.Fatalf("result=%+v error=%v", result, err)
					}
					gotOrigin, gotFinal = staged.Submission.TransactionID, staged.Submission.ConfirmedTransactionID
				}
				if gotOrigin != tc.wantOrigin || gotFinal != tc.wantFinal {
					t.Fatalf("pair=%s/%s want %s/%s", gotOrigin, gotFinal, tc.wantOrigin, tc.wantFinal)
				}
			})
		}
	}
}

func TestWaitForTerminateService_NoNewEvidenceRetainsAccepted(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID, Accepted: accepted, TransactionID: common.HexToHash("0xa").Hex()}
		cause := errors.New("RPC unavailable")
		client := &fakePDPProviderClient{waitTerminateFn: func(context.Context, types.BigInt, time.Duration, func(common.Hash), ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
			return nil, cause
		}}
		_, err := terminationRecoveryContext(t, client).WaitForTerminateService(context.Background(), sub, nil)
		var staged *TerminateServiceError
		if !errors.As(err, &staged) || staged.Submission == nil || staged.Submission.Accepted != accepted || !errors.Is(err, cause) {
			t.Fatalf("accepted=%v error=%v", accepted, err)
		}
	}
}

func TestWaitForTerminateService_RecoversBroadcastErrorWithoutSending(t *testing.T) {
	tx := ethtypes.NewTx(&ethtypes.LegacyTx{Nonce: 3})
	sendErr := &types.BroadcastError{Transaction: tx, Cause: context.DeadlineExceeded}
	term := &fakeFWSSTerminator{terminateFn: func(context.Context, types.BigInt, FWSSTerminationOptions) (*types.WriteResult, error) {
		return nil, sendErr
	}}
	c := terminationRecoveryContext(t, &fakePDPProviderClient{}, WithFWSSTerminator(term))
	_, err := c.SubmitTerminateService(context.Background(), &TerminateServiceOptions{SkipProvider: true})
	var staged *TerminateServiceError
	if !errors.As(err, &staged) || !errors.Is(err, context.DeadlineExceeded) || staged.Stage != TerminateServiceStageSubmit || staged.Submission == nil || staged.Submission.Accepted || staged.Submission.TransactionID != tx.Hash().Hex() {
		t.Fatalf("submission error=%v", err)
	}
	encoded, err := json.Marshal(staged.Submission)
	if err != nil {
		t.Fatal(err)
	}
	var saved TerminateServiceSubmission
	if err := json.Unmarshal(encoded, &saved); err != nil {
		t.Fatal(err)
	}
	term.called = false
	receipt := &ethtypes.Receipt{TxHash: tx.Hash(), BlockNumber: big.NewInt(100), Status: ethtypes.ReceiptStatusFailed}
	term.waitFn = func(_ context.Context, _ types.BigInt, hash common.Hash, _ time.Duration, _ []warmstorage.WriteOption) (*types.WriteResult, types.Epoch, error) {
		if hash != tx.Hash() {
			t.Fatalf("wait hash=%s", hash)
		}
		return &types.WriteResult{Hash: hash, Receipt: receipt}, 0, types.ErrTxFailed
	}
	result, err := c.WaitForTerminateService(context.Background(), saved, nil)
	if result != nil || !errors.As(err, &staged) || staged.Stage != TerminateServiceStageWait || staged.Receipt != receipt || !staged.Submission.Accepted || staged.Submission.ConfirmedTransactionID != tx.Hash().Hex() || !errors.Is(err, types.ErrTxFailed) {
		t.Fatalf("recovery result=%+v error=%v", result, err)
	}
	if saved.Accepted || saved.ConfirmedTransactionID != "" || term.called || term.waitCalls != 1 {
		t.Fatalf("recovery modified input or sent again: saved=%+v terminator=%+v", saved, term)
	}
}

func TestWaitForTerminateService_DirectUsesConfirmedHashAndPreservesOriginal(t *testing.T) {
	original, confirmed := common.HexToHash("0xa"), common.HexToHash("0xb")
	term := &fakeFWSSTerminator{waitFn: func(_ context.Context, id types.BigInt, hash common.Hash, _ time.Duration, _ []warmstorage.WriteOption) (*types.WriteResult, types.Epoch, error) {
		if hash != confirmed {
			t.Fatalf("receipt lookup=%s want %s", hash, confirmed)
		}
		return &types.WriteResult{Hash: hash, Receipt: terminateReceipt(t, id, hash, 42, 1)}, 42, nil
	}}
	sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), Direct: true, TransactionID: original.Hex(), ConfirmedTransactionID: confirmed.Hex()}
	result, err := terminationRecoveryContext(t, &fakePDPProviderClient{}, WithFWSSTerminator(term)).WaitForTerminateService(context.Background(), sub, nil)
	if err != nil || result == nil || result.TxHash == nil || *result.TxHash != original || result.ConfirmedTxHash == nil || *result.ConfirmedTxHash != confirmed || term.called || term.waitCalls != 1 {
		t.Fatalf("result=%+v error=%v terminator=%+v", result, err, term)
	}
}

type terminationRecoveryReader struct {
	read func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error)
}

func (r terminationRecoveryReader) GetDataSet(ctx context.Context, id types.BigInt) (*warmstorage.DataSetInfo, error) {
	return r.read(ctx, id)
}

func (terminationRecoveryReader) FindDataSetByClientDataSetID(context.Context, common.Address, types.BigInt) (*warmstorage.DataSetInfo, error) {
	return nil, errors.New("unexpected lookup")
}

func TestWaitForTerminateService_ReconciliationUsesRemainingBudget(t *testing.T) {
	sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID}
	providerErr := errors.New("provider unavailable")
	var providerDeadline time.Time
	client := &fakePDPProviderClient{waitTerminateFn: func(ctx context.Context, _ types.BigInt, _ time.Duration, _ func(common.Hash), _ ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
		providerDeadline, _ = ctx.Deadline()
		return nil, providerErr
	}}
	reads := 0
	reader := terminationRecoveryReader{read: func(ctx context.Context, _ types.BigInt) (*warmstorage.DataSetInfo, error) {
		reads++
		deadline, ok := ctx.Deadline()
		if !ok || !deadline.Equal(providerDeadline) {
			t.Fatalf("reader deadline=%s provider deadline=%s", deadline, providerDeadline)
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	_, err := terminationRecoveryContext(t, client, WithFWSSDataSetReader(reader)).WaitForTerminateService(context.Background(), sub, &TerminateServiceWaitOptions{Timeout: 20 * time.Millisecond})
	if reads != 1 || !errors.Is(err, providerErr) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("reads=%d error=%v", reads, err)
	}
}

func TestWaitForTerminateService_ReconciliationFactsAndErrors(t *testing.T) {
	providerErr := &pdp.WaitForTerminateServiceNotFoundError{}
	readerErr := errors.New("reader unavailable")
	cases := []struct {
		name        string
		info        *warmstorage.DataSetInfo
		err         error
		wantSuccess bool
	}{
		{"arranged", &warmstorage.DataSetInfo{PDPEndEpoch: 42}, nil, true},
		{"active", &warmstorage.DataSetInfo{}, nil, false},
		{"missing", nil, warmstorage.ErrNotFound, false},
		{"reader failure", nil, readerErr, false},
		{"invalid reader result", nil, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &fakePDPProviderClient{waitTerminateFn: func(context.Context, types.BigInt, time.Duration, func(common.Hash), ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
				return nil, providerErr
			}}
			reader := terminationRecoveryReader{read: func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) { return tc.info, tc.err }}
			sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID}
			result, err := terminationRecoveryContext(t, client, WithFWSSDataSetReader(reader)).WaitForTerminateService(context.Background(), sub, nil)
			if tc.wantSuccess {
				if err != nil || result == nil || result.EndEpoch != 42 || result.TxHash != nil || result.ConfirmedTxHash != nil {
					t.Fatalf("result=%+v error=%v", result, err)
				}
			} else if result != nil || !errors.Is(err, providerErr) || (tc.err != nil && !errors.Is(err, tc.err)) {
				t.Fatalf("result=%+v error=%v", result, err)
			}
		})
	}
}

func TestWaitForTerminateService_QueuedMissingStatusRemainsNotFound(t *testing.T) {
	client := &fakePDPProviderClient{waitTerminateFn: func(context.Context, types.BigInt, time.Duration, func(common.Hash), ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
		return nil, &pdp.WaitForTerminateServiceNotFoundError{}
	}}
	reader := terminationRecoveryReader{read: func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) {
		return &warmstorage.DataSetInfo{}, nil
	}}
	sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID, Accepted: true, Queued: true}
	result, err := terminationRecoveryContext(t, client, WithFWSSDataSetReader(reader)).WaitForTerminateService(context.Background(), sub, nil)
	_, pending := errors.AsType[*pdp.TerminateServicePendingError](err)
	_, notFound := errors.AsType[*pdp.WaitForTerminateServiceNotFoundError](err)
	if result != nil || !pending || !notFound {
		t.Fatalf("result=%+v error=%v, want pending and not-found classifications", result, err)
	}
}

func TestWaitForTerminateService_CanceledReconciliationCannotSucceed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	providerErr := errors.New("provider unavailable")
	client := &fakePDPProviderClient{waitTerminateFn: func(context.Context, types.BigInt, time.Duration, func(common.Hash), ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
		return nil, providerErr
	}}
	reader := terminationRecoveryReader{read: func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) {
		cancel()
		return &warmstorage.DataSetInfo{PDPEndEpoch: 42}, nil
	}}
	sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID}
	result, err := terminationRecoveryContext(t, client, WithFWSSDataSetReader(reader)).WaitForTerminateService(ctx, sub, nil)
	if result != nil || !errors.Is(err, providerErr) || !errors.Is(err, context.Canceled) {
		t.Fatalf("result=%+v error=%v, want cancellation with original provider error", result, err)
	}
}

func TestWaitForTerminateService_ExpiredBudgetSkipsReconciliation(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		client := &fakePDPProviderClient{waitTerminateFn: func(ctx context.Context, _ types.BigInt, _ time.Duration, _ func(common.Hash), _ ...pdp.WaitForTerminateServiceOption) (*pdp.TerminateServiceStatus, error) {
			if canceled {
				cancel()
			} else {
				<-ctx.Done()
			}
			return nil, ctx.Err()
		}}
		reads := 0
		reader := terminationRecoveryReader{read: func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) {
			reads++
			return &warmstorage.DataSetInfo{PDPEndEpoch: 42}, nil
		}}
		sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: testProvider().ID}
		result, err := terminationRecoveryContext(t, client, WithFWSSDataSetReader(reader)).WaitForTerminateService(ctx, sub, &TerminateServiceWaitOptions{Timeout: 10 * time.Millisecond})
		cancel()
		want := context.DeadlineExceeded
		if canceled {
			want = context.Canceled
		}
		if result != nil || !errors.Is(err, want) || reads != 0 {
			t.Fatalf("canceled=%v result=%+v error=%v reads=%d", canceled, result, err, reads)
		}
	}
}

type terminationRecoveryLifecycle struct{ closed atomic.Bool }

func (l *terminationRecoveryLifecycle) CheckClosed() error {
	if l.closed.Load() {
		return ErrClosed
	}
	return nil
}

func TestWaitForTerminateService_ClosingDuringObservationSkipsReconciliation(t *testing.T) {
	lifecycle := &terminationRecoveryLifecycle{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lifecycle.closed.Store(true)
		http.Error(w, "invalid provider response", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)
	provider := testProvider()
	provider.ServiceURL = server.URL
	reads := 0
	reader := terminationRecoveryReader{read: func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) {
		reads++
		return &warmstorage.DataSetInfo{PDPEndEpoch: 42}, nil
	}}
	svc := mustNewService(t, Options{HTTPClient: server.Client(), Lifecycle: lifecycle, ProviderResolver: &fakeTerminationProviderResolver{provider: &provider}, FWSSDataSetReader: reader})
	sub := TerminateServiceSubmission{DataSetID: types.NewBigInt(7), ProviderID: provider.ID}
	result, err := svc.WaitForTerminateService(context.Background(), sub, nil)
	if result != nil || !errors.Is(err, ErrClosed) || reads != 0 {
		t.Fatalf("result=%+v error=%v reader calls=%d", result, err, reads)
	}
}

func TestTerminateService_SubmissionCallbackStopsWaiting(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, closed := range []bool{false, true} {
			name := fmt.Sprintf("direct=%v/closed=%v", direct, closed)
			t.Run(name, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				life := &terminationRecoveryLifecycle{}
				var posts, gets atomic.Int32
				hash := common.HexToHash("0xa")
				id := types.NewBigInt(7)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost {
						posts.Add(1)
						w.WriteHeader(http.StatusAccepted)
						return
					}
					gets.Add(1)
					_, _ = fmt.Fprintf(w, `{"terminationTxHash":%q,"fwssTerminated":true,"serviceTerminationEpoch":42}`, hash.Hex())
				}))
				t.Cleanup(server.Close)
				provider := testProvider()
				provider.ServiceURL = server.URL
				sig := mustTestSigner(t)
				waits := 0
				term := &fakeFWSSTerminator{
					res: &types.WriteResult{Hash: hash},
					waitFn: func(context.Context, types.BigInt, common.Hash, time.Duration, []warmstorage.WriteOption) (*types.WriteResult, types.Epoch, error) {
						waits++
						return &types.WriteResult{Hash: hash, Receipt: terminateReceipt(t, id, hash, 42, 99)}, 42, nil
					},
				}
				svc := mustNewService(t, Options{
					HTTPClient: server.Client(), Lifecycle: life, DataSetTerminator: term,
					ProviderResolver:   &fakeTerminationProviderResolver{provider: &provider},
					FWSSDataSetReader:  fakeTerminationDataSetReader{info: &warmstorage.DataSetInfo{DataSetID: id, ProviderID: provider.ID, Payer: sig.EVMAddress()}},
					PaymentStateReader: &fakeTerminationPaymentReader{account: &payments.AccountState{}},
					EpochReader:        fakeTerminationEpochReader{block: 10}, PaymentToken: common.HexToAddress("0x9999"),
					Signer: sig, ChainID: types.ChainID(314159), RecordKeeper: testRecordKeeper(),
				})
				notified := false
				result, err := svc.TerminateService(ctx, id, &TerminateServiceOptions{
					SkipProvider: direct,
					OnSubmitted: func(sub TerminateServiceSubmission) {
						notified = sub.Accepted
						if closed {
							life.closed.Store(true)
						} else {
							cancel()
						}
					},
				})
				want := context.Canceled
				if closed {
					want = ErrClosed
				}
				staged, ok := errors.AsType[*TerminateServiceError](err)
				if !notified || result != nil || !errors.Is(err, want) || !ok || staged.Stage != TerminateServiceStageWait || staged.Submission == nil || !staged.Submission.Accepted {
					t.Errorf("result=%+v err=%v notified=%v, want wait error %v with accepted submission", result, err, notified, want)
				}
				if gets.Load() != 0 || waits != 0 {
					t.Errorf("observation after callback stop: provider GETs=%d direct waits=%d", gets.Load(), waits)
				}
				wantPosts := int32(1)
				if direct {
					wantPosts = 0
				}
				if posts.Load() != wantPosts {
					t.Errorf("provider POSTs=%d, want %d", posts.Load(), wantPosts)
				}
			})
		}
	}
}
