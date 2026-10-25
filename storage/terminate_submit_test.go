package storage

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type terminationSubmitSigner struct {
	signer.StorageSigner
	err error
}

func (s terminationSubmitSigner) SignHash([]byte) ([]byte, error) { return nil, s.err }

type terminationSubmitEpochReader struct{ err error }

func (r terminationSubmitEpochReader) BlockNumber(context.Context) (uint64, error) { return 0, r.err }

type terminationSubmitPaymentReader struct{ err error }

func (r terminationSubmitPaymentReader) AccountInfo(context.Context, common.Address, common.Address) (*payments.AccountState, error) {
	return nil, r.err
}

type terminationSubmitResolver struct{ err error }

func (r terminationSubmitResolver) ResolveProvider(context.Context, types.BigInt) (Provider, error) {
	return Provider{}, r.err
}

func terminationSubmitService(t *testing.T, url string) *Service {
	t.Helper()
	signer := mustTestSigner(t)
	provider := testProvider()
	provider.ServiceURL = url
	return mustNewService(t, Options{
		AllowPrivateNetworks: true,
		Signer:               signer, ChainID: types.ChainID(314159), RecordKeeper: testRecordKeeper(),
		FWSSDataSetReader: fakeTerminationDataSetReader{info: &warmstorage.DataSetInfo{
			DataSetID: types.NewBigInt(7), ProviderID: provider.ID, Payer: signer.EVMAddress(),
		}},
		ProviderResolver:   &fakeTerminationProviderResolver{provider: &provider},
		PaymentStateReader: &fakeTerminationPaymentReader{account: &payments.AccountState{}},
		EpochReader:        fakeTerminationEpochReader{block: 10}, PaymentToken: common.HexToAddress("0x9999"),
	})
}

func TestSubmitTerminateService_PrepareFailures(t *testing.T) {
	cause := errors.New("preparation unavailable")
	var posts atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	cases := []struct {
		name   string
		change func(*Service)
		want   error
	}{
		{"uninitialized", func(s *Service) { s.httpClient = nil }, ErrUninitialized},
		{"dataset read", func(s *Service) {
			s.dsReader = terminationRecoveryReader{read: func(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error) { return nil, cause }}
		}, cause},
		{"payer", func(s *Service) { s.payerAddr = common.HexToAddress("0x123") }, ErrInvalidArgument},
		{"epoch read", func(s *Service) { s.epochs = terminationSubmitEpochReader{cause} }, cause},
		{"account read", func(s *Service) { s.payments = terminationSubmitPaymentReader{cause} }, cause},
		{"provider resolve", func(s *Service) { s.providers = terminationSubmitResolver{cause} }, cause},
		{"provider client", func(s *Service) {
			p := testProvider()
			p.ServiceURL = "://invalid"
			s.providers = &fakeTerminationProviderResolver{provider: &p}
		}, nil},
		{"sign", func(s *Service) { s.signer = terminationSubmitSigner{StorageSigner: s.signer, err: cause} }, cause},
		{"debt", func(s *Service) {
			s.payments = &fakeTerminationPaymentReader{account: &payments.AccountState{Funds: big.NewInt(0), LockupCurrent: big.NewInt(1)}}
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := terminationSubmitService(t, server.URL)
			tc.change(s)
			callback := false
			sub, err := s.SubmitTerminateService(context.Background(), types.NewBigInt(7), &TerminateServiceOptions{OnSubmitted: func(TerminateServiceSubmission) { callback = true }})
			var staged *TerminateServiceError
			if sub != nil || !errors.As(err, &staged) || staged.Stage != TerminateServiceStagePrepare || staged.Submission != nil || callback || (tc.want != nil && !errors.Is(err, tc.want)) {
				t.Fatalf("submission=%+v error=%v callback=%v", sub, err, callback)
			}
			if tc.name == "debt" {
				if _, ok := errors.AsType[*TerminateServiceDebtError](err); !ok {
					t.Fatalf("debt cause lost: %v", err)
				}
			}
		})
	}
	if posts.Load() != 0 {
		t.Fatalf("preparation sent %d requests", posts.Load())
	}
}

func TestSubmitTerminateService_ProviderBoundary(t *testing.T) {
	cases := []struct {
		name            string
		status          int
		body            string
		queued, already bool
	}{
		{"accepted", 202, "", false, false},
		{"queued", 409, `{"code":1}`, true, false},
		{"terminated", 409, `{"code":0,"serviceTerminationEpoch":42}`, false, true},
		{"unsupported", 503, "unsupported", false, false},
		{"unrecognized", 502, "unknown", false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var posts, gets atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts.Add(1)
				} else {
					gets.Add(1)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			s := terminationSubmitService(t, server.URL)
			calls := 0
			sub, err := s.SubmitTerminateService(context.Background(), types.NewBigInt(7), &TerminateServiceOptions{
				DirectWaitTimeout: time.Nanosecond, PollInterval: time.Nanosecond,
				OnSubmitted: func(got TerminateServiceSubmission) {
					calls++
					if !got.Accepted || got.Queued != tc.queued || posts.Load() != 1 || gets.Load() != 0 {
						t.Errorf("callback=%+v POST=%d GET=%d", got, posts.Load(), gets.Load())
					}
					got.TransactionID = "mutated"
					got.Accepted = false
				},
			})
			switch {
			case tc.status == 202 || tc.queued:
				if err != nil || sub == nil || !sub.Accepted || sub.Queued != tc.queued || sub.TransactionID != "" || calls != 1 {
					t.Fatalf("submission=%+v error=%v callbacks=%d", sub, err, calls)
				}
			case tc.already:
				var already *pdp.ServiceAlreadyTerminatedError
				var staged *TerminateServiceError
				if sub != nil || !errors.As(err, &already) || already.ServiceTerminationEpoch != 42 || errors.As(err, &staged) || calls != 0 {
					t.Fatalf("submission=%+v error=%v callbacks=%d", sub, err, calls)
				}
				result, err := s.TerminateService(context.Background(), types.NewBigInt(7), nil)
				if err != nil || result == nil || result.EndEpoch != 42 {
					t.Fatalf("already terminated result=%+v error=%v", result, err)
				}
			default:
				var staged *TerminateServiceError
				if sub != nil || !errors.As(err, &staged) || staged.Stage != TerminateServiceStageSubmit || staged.Submission == nil || staged.Submission.Accepted || calls != 0 {
					t.Fatalf("submission=%+v error=%v callbacks=%d", sub, err, calls)
				}
				if tc.status == 503 {
					if _, ok := errors.AsType[*pdp.TerminateServiceNotSupportedError](err); !ok {
						t.Fatalf("unsupported cause lost: %v", err)
					}
				}
			}
			if gets.Load() != 0 {
				t.Fatalf("Submit performed %d status queries", gets.Load())
			}
		})
	}
}
