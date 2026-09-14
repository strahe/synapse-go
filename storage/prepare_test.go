package storage

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/payments"
	sdktypes "github.com/strahe/synapse-go/types"
)

type stubCostCalc struct {
	out           *costs.MultiContextCosts
	err           error
	gotPayer      common.Address
	gotPieceSizes []uint64
	gotRefs       []costs.MultiContextRef
	gotOpts       costs.UploadCostOptions
	mutateInputs  bool
}

func (s *stubCostCalc) CalculateMultiContextCosts(_ context.Context, payer common.Address, pieceSizes []uint64, refs []costs.MultiContextRef, opts *costs.UploadCostOptions) (*costs.MultiContextCosts, error) {
	s.gotPayer = payer
	s.gotPieceSizes = slices.Clone(pieceSizes)
	s.gotRefs = append([]costs.MultiContextRef(nil), refs...)
	s.gotOpts = *opts
	if s.mutateInputs {
		pieceSizes[0] = 999
	}
	return s.out, s.err
}

type stubFunder struct {
	called bool
	gotAmt *big.Int
	gotOpt int
}

type stubDataSetLeafCountReader struct {
	leaves *big.Int
	err    error
	calls  int
}

func (s *stubDataSetLeafCountReader) GetDataSetLeafCount(context.Context, sdktypes.BigInt) (*big.Int, error) {
	s.calls++
	return s.leaves, s.err
}

func (s *stubFunder) FundSync(_ context.Context, amount *big.Int, opts ...payments.WriteOption) (*sdktypes.WriteResult, error) {
	s.called = true
	s.gotAmt = amount
	s.gotOpt = len(opts)
	return &sdktypes.WriteResult{Hash: common.HexToHash("0xdeadbeef")}, nil
}

func newTestService() *Service {
	return &Service{
		httpClient:   &http.Client{},
		lifecycle:    lifecycle.New(),
		payerAddr:    testPayer(),
		chainID:      sdktypes.ChainID(314159),
		recordKeeper: testRecordKeeper(),
	}
}

func assertInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Prepare error = %v, want ErrInvalidArgument", err)
	}
}

func (c *fakeUploadContext) GetProviderInfo() Provider {
	p := testProvider()
	if !c.id.IsZero() {
		p.ID = c.id
	}
	return p
}

func (c *fakeUploadContext) CDNEnabled() bool {
	if c.dataSetMetadata == nil {
		return false
	}
	_, ok := c.dataSetMetadata["withCDN"]
	return ok
}

func TestPrepare_ReadyShortCircuits(t *testing.T) {
	svc := newTestService()
	calc := &stubCostCalc{out: &costs.MultiContextCosts{Ready: true}}
	svc.costCalc = calc
	svc.payerAddr = testPayer()

	res, err := svc.Prepare(context.Background(), &PrepareOptions{Costs: &costs.MultiContextCosts{Ready: true}})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.Transaction != nil {
		t.Fatalf("want nil Transaction when Ready=true, got %+v", res.Transaction)
	}
	if calc.gotRefs != nil {
		t.Fatalf("cost calculator called for precomputed costs: %+v", calc.gotRefs)
	}
}

func TestPrepareRefs_LeafCountModes(t *testing.T) {
	dataSetID := sdktypes.NewBigInt(10)
	uploadCtx := &fakeUploadContext{id: sdktypes.NewBigInt(1), dataSetID: &dataSetID}
	opts := &PrepareOptions{Contexts: []StorageContext{uploadCtx}}

	t.Run("configured reader propagates unavailable", func(t *testing.T) {
		reader := &stubDataSetLeafCountReader{err: ErrDataSetUnavailable}
		svc := newTestService()
		svc.leafCountReader = reader
		refs, err := svc.prepareRefs(context.Background(), opts)
		if refs != nil || !errors.Is(err, ErrDataSetUnavailable) {
			t.Fatalf("prepareRefs = (%v, %v), want unavailable sentinel", refs, err)
		}
		if reader.calls != 1 {
			t.Fatalf("leaf count reader calls = %d, want 1", reader.calls)
		}
	})

	t.Run("unconfigured reader rejects missing state", func(t *testing.T) {
		svc := newTestService()
		refs, err := svc.prepareRefs(context.Background(), opts)
		if refs != nil || !errors.Is(err, ErrUninitialized) {
			t.Fatalf("prepareRefs = (%v, %v), want ErrUninitialized", refs, err)
		}
	})
	for _, tc := range []struct {
		name   string
		leaves *big.Int
		err    error
		valid  bool
	}{
		{"known empty", big.NewInt(0), nil, true},
		{"nonempty", big.NewInt(5), nil, true},
		{"nil response", nil, nil, false},
		{"negative response", big.NewInt(-1), nil, false},
		{"read failure", nil, context.Canceled, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestService()
			reader := &stubDataSetLeafCountReader{leaves: tc.leaves, err: tc.err}
			svc.leafCountReader = reader
			refs, err := svc.prepareRefs(context.Background(), opts)
			if tc.valid {
				if err != nil || len(refs) != 1 || refs[0].CurrentDataSetLeafCount.Cmp(tc.leaves) != 0 {
					t.Fatalf("prepareRefs = (%v, %v), want leaf count %s", refs, err, tc.leaves)
				}
				tc.leaves.SetInt64(999)
				if refs[0].CurrentDataSetLeafCount.Int64() == 999 {
					t.Fatal("reader result aliases prepared state")
				}
			} else {
				if refs != nil || err == nil || errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrDataSetUnavailable) || errors.Is(err, ErrUninitialized) {
					t.Fatalf("prepareRefs = (%v, %v), want backend error", refs, err)
				}
				if tc.err != nil && !errors.Is(err, tc.err) {
					t.Fatalf("lost original cause: %v", err)
				}
			}
		})
	}
}

func TestPrepare_ForwardsPiecesAndLeafCounts(t *testing.T) {
	svc := newTestService()
	calc := &stubCostCalc{out: &costs.MultiContextCosts{Ready: true}, mutateInputs: true}
	svc.costCalc = calc
	reader := &stubDataSetLeafCountReader{leaves: big.NewInt(5)}
	svc.leafCountReader = reader
	dataSetID := sdktypes.NewBigInt(10)
	opts := &PrepareOptions{
		PieceSizes: []uint64{128, 190},
		Contexts: []StorageContext{
			&fakeUploadContext{id: sdktypes.NewBigInt(1)},
			&fakeUploadContext{id: sdktypes.NewBigInt(2), dataSetID: &dataSetID},
		},
	}
	if _, err := svc.Prepare(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	if reader.calls != 1 || !slices.Equal(calc.gotPieceSizes, []uint64{128, 190}) ||
		len(calc.gotRefs) != 2 || !calc.gotRefs[0].IsNewDataSet || calc.gotRefs[1].IsNewDataSet ||
		calc.gotRefs[1].CurrentDataSetLeafCount.Cmp(big.NewInt(5)) != 0 {
		t.Fatalf("incorrect preparation: pieces=%v refs=%+v reader calls=%d", calc.gotPieceSizes, calc.gotRefs, reader.calls)
	}
	reader.leaves.SetInt64(999)
	if opts.PieceSizes[0] != 128 || calc.gotRefs[1].CurrentDataSetLeafCount.Int64() != 5 {
		t.Fatal("preparation did not preserve independent input state")
	}
}

func TestPrepare_PrecomputedCostsSkipReader(t *testing.T) {
	svc := newTestService()
	reader := &stubDataSetLeafCountReader{err: errors.New("must not read")}
	svc.leafCountReader = reader
	for _, sizes := range [][]uint64{nil, {}} {
		got, err := svc.Prepare(context.Background(), &PrepareOptions{Costs: &costs.MultiContextCosts{Ready: true}, PieceSizes: sizes})
		if err != nil || got.Transaction != nil || reader.calls != 0 {
			t.Fatalf("precomputed preparation = (%v, %v), reader calls=%d", got, err, reader.calls)
		}
	}
}

func TestPrepareRejectsContextIdentityBeforeCostCalculation(t *testing.T) {
	calc := &stubCostCalc{out: &costs.MultiContextCosts{Ready: true}}
	svc := newTestService()
	svc.costCalc = calc
	identity := serviceTestIdentity()
	identity.Payer = common.HexToAddress("0x9999")
	uploadCtx := &fakeUploadContext{id: sdktypes.NewBigInt(1), identity: &identity}

	result, err := svc.Prepare(context.Background(), &PrepareOptions{
		PieceSizes: []uint64{1},
		Contexts:   []StorageContext{uploadCtx},
	})
	if result != nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
	}
	if calc.gotRefs != nil {
		t.Fatalf("cost calculator called with refs=%+v", calc.gotRefs)
	}
}

func TestPrepare_RejectsInvalidOptions(t *testing.T) {
	readyCosts := &costs.MultiContextCosts{Ready: true}
	uploadCtx := &fakeUploadContext{id: sdktypes.NewBigInt(1)}

	tests := []struct {
		name string
		opts *PrepareOptions
	}{
		{
			name: "nil options",
			opts: nil,
		},
		{
			name: "zero data size without costs",
			opts: &PrepareOptions{},
		},
		{
			name: "costs with contexts",
			opts: &PrepareOptions{
				Costs:    readyCosts,
				Contexts: []StorageContext{uploadCtx},
			},
		},
		{
			name: "costs with data size",
			opts: &PrepareOptions{
				Costs:      readyCosts,
				PieceSizes: []uint64{128},
			},
		},
		{
			name: "zero piece size without costs",
			opts: &PrepareOptions{
				PieceSizes: []uint64{0},
				Contexts:   []StorageContext{uploadCtx},
			},
		},
		{
			name: "costs with extra runway",
			opts: &PrepareOptions{
				Costs:             readyCosts,
				ExtraRunwayEpochs: 1,
			},
		},
		{
			name: "costs with explicit zero buffer",
			opts: &PrepareOptions{
				Costs:        readyCosts,
				BufferEpochs: new(int64(0)),
			},
		},
		{
			name: "negative extra runway",
			opts: &PrepareOptions{
				PieceSizes:        []uint64{128},
				ExtraRunwayEpochs: -1,
			},
		},
		{
			name: "negative buffer",
			opts: &PrepareOptions{
				PieceSizes:   []uint64{128},
				BufferEpochs: new(int64(-1)),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()

			_, err := svc.Prepare(context.Background(), tt.opts)
			assertInvalidArgument(t, err)
		})
	}
}

func TestPrepare_BuildsExecuteWhenNotReady(t *testing.T) {
	funder := &stubFunder{}
	svc := newTestService()
	svc.funder = funder
	svc.payerAddr = testPayer()

	res, err := svc.Prepare(context.Background(), &PrepareOptions{
		Costs: &costs.MultiContextCosts{
			Ready:                false,
			DepositNeeded:        big.NewInt(1234),
			NeedsFWSSMaxApproval: true,
		},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.Transaction == nil {
		t.Fatal("want non-nil Transaction when Ready=false")
	}
	if res.Transaction.DepositAmount.Int64() != 1234 {
		t.Fatalf("DepositAmount: got %s want 1234", res.Transaction.DepositAmount)
	}
	if !res.Transaction.IncludesApproval {
		t.Fatal("IncludesApproval should be true")
	}
	if _, err := res.Transaction.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !funder.called {
		t.Fatal("funder.FundSync not invoked")
	}
	if funder.gotAmt.Int64() != 1234 {
		t.Fatalf("funder got amount %s", funder.gotAmt)
	}
	if funder.gotOpt != 1 {
		t.Fatalf("funder got %d opts, want 1 (approval)", funder.gotOpt)
	}
}

func TestPrepareExecute_UsesComputedApprovalOptionsWithCallerWriteOptions(t *testing.T) {
	funder := &stubFunder{}
	svc := newTestService()
	svc.funder = funder
	svc.payerAddr = testPayer()

	res, err := svc.Prepare(context.Background(), &PrepareOptions{
		Costs: &costs.MultiContextCosts{
			Ready:                false,
			DepositNeeded:        big.NewInt(1234),
			NeedsFWSSMaxApproval: true,
			RequiredLockupPeriod: big.NewInt(456),
		},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if _, err := res.Transaction.Execute(
		context.Background(),
		payments.WithWait(time.Second),
		payments.WithConfirmations(2),
	); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !funder.called {
		t.Fatal("funder.FundSync not invoked")
	}
	if funder.gotOpt != 4 {
		t.Fatalf("funder got %d opts, want 4 (2 caller + approval decision + lockup period)", funder.gotOpt)
	}
}

func TestPrepare_RejectsInvalidNotReadyCosts(t *testing.T) {
	uploadCtx := &fakeUploadContext{id: sdktypes.NewBigInt(1)}
	tests := []struct {
		name  string
		opts  *PrepareOptions
		setup func(*Service)
	}{
		{
			name: "supplied nil deposit",
			opts: &PrepareOptions{Costs: &costs.MultiContextCosts{Ready: false}},
		},
		{
			name: "supplied negative deposit",
			opts: &PrepareOptions{
				Costs: &costs.MultiContextCosts{
					Ready:         false,
					DepositNeeded: big.NewInt(-1),
				},
			},
		},
		{
			name: "calculated nil deposit",
			opts: &PrepareOptions{
				PieceSizes: []uint64{128},
				Contexts: []StorageContext{
					uploadCtx,
				},
			},
			setup: func(svc *Service) {
				svc.costCalc = &stubCostCalc{out: &costs.MultiContextCosts{Ready: false}}
				svc.payerAddr = testPayer()
			},
		},
		{
			name: "calculated negative deposit",
			opts: &PrepareOptions{
				PieceSizes: []uint64{128},
				Contexts: []StorageContext{
					uploadCtx,
				},
			},
			setup: func(svc *Service) {
				svc.costCalc = &stubCostCalc{out: &costs.MultiContextCosts{
					Ready:         false,
					DepositNeeded: big.NewInt(-1),
				}}
				svc.payerAddr = testPayer()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			if tt.setup != nil {
				tt.setup(svc)
			}

			_, err := svc.Prepare(context.Background(), tt.opts)
			assertInvalidArgument(t, err)
		})
	}
}

func TestPrepare_RejectsZeroDefaultPayer(t *testing.T) {
	svc := newTestService()
	svc.payerAddr = common.Address{}
	svc.costCalc = &stubCostCalc{out: &costs.MultiContextCosts{Ready: true}}

	_, err := svc.Prepare(context.Background(), &PrepareOptions{
		PieceSizes: []uint64{128},
		Contexts:   []StorageContext{&fakeUploadContext{id: sdktypes.NewBigInt(1)}},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Prepare error = %v, want ErrInvalidArgument", err)
	}
}

func TestPrepare_RequiresExplicitContexts(t *testing.T) {
	svc := newTestService()
	svc.costCalc = &stubCostCalc{out: &costs.MultiContextCosts{Ready: true}}
	_, err := svc.Prepare(context.Background(), &PrepareOptions{PieceSizes: []uint64{128}})
	assertInvalidArgument(t, err)
}

func TestPrepare_ForwardsRunwayAndBufferOptions(t *testing.T) {
	tests := []struct {
		name         string
		bufferEpochs *int64
	}{
		{name: "default"},
		{name: "explicit zero", bufferEpochs: new(int64(0))},
		{name: "positive", bufferEpochs: new(int64(9))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			costCalc := &stubCostCalc{out: &costs.MultiContextCosts{Ready: true}}
			svc := newTestService()
			svc.costCalc = costCalc
			svc.payerAddr = testPayer()

			_, err := svc.Prepare(context.Background(), &PrepareOptions{
				PieceSizes:        []uint64{128},
				Contexts:          []StorageContext{&fakeUploadContext{id: sdktypes.NewBigInt(1)}},
				ExtraRunwayEpochs: 7,
				BufferEpochs:      tt.bufferEpochs,
			})
			if err != nil {
				t.Fatalf("Prepare: %v", err)
			}
			if costCalc.gotPayer != testPayer() || !slices.Equal(costCalc.gotPieceSizes, []uint64{128}) {
				t.Fatalf("cost calculation payer=%s pieceSizes=%v want %s and [128]", costCalc.gotPayer, costCalc.gotPieceSizes, testPayer())
			}
			if costCalc.gotOpts.ExtraRunwayEpochs != 7 {
				t.Fatalf("ExtraRunwayEpochs=%d want 7", costCalc.gotOpts.ExtraRunwayEpochs)
			}
			if tt.bufferEpochs == nil {
				if costCalc.gotOpts.BufferEpochs != nil {
					t.Fatalf("BufferEpochs=%v want nil", costCalc.gotOpts.BufferEpochs)
				}
			} else if costCalc.gotOpts.BufferEpochs == nil || *costCalc.gotOpts.BufferEpochs != *tt.bufferEpochs {
				t.Fatalf("BufferEpochs=%v want %d", costCalc.gotOpts.BufferEpochs, *tt.bufferEpochs)
			}
			if len(costCalc.gotRefs) != 1 {
				t.Fatalf("len(gotRefs)=%d want 1", len(costCalc.gotRefs))
			}
		})
	}
}

func TestPrepare_ReturnsErrorWhenCostCalculatorFails(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "nil costs"},
		{name: "calculator error", err: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestService()
			svc.costCalc = &stubCostCalc{err: tt.err}
			svc.payerAddr = testPayer()

			_, err := svc.Prepare(context.Background(), &PrepareOptions{
				PieceSizes: []uint64{128},
				Contexts: []StorageContext{
					&fakeUploadContext{id: sdktypes.NewBigInt(1)},
				},
			})
			if err == nil {
				t.Fatal("Prepare error = nil, want error")
			}
			if tt.err != nil {
				if !errors.Is(err, tt.err) || !strings.HasPrefix(err.Error(), "storage.Service.Prepare: ") {
					t.Fatalf("Prepare error = %v, want wrapped %v", err, tt.err)
				}
				return
			}
			if errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Prepare error = %v, want internal error", err)
			}
			if !strings.Contains(err.Error(), "cost calculator returned nil costs") {
				t.Fatalf("Prepare error = %v, want nil-costs message", err)
			}
		})
	}
}
