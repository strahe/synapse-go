package storage

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/payments"
	sdktypes "github.com/strahe/synapse-go/types"
)

type stubCostCalc struct {
	out     *MultiContextCosts
	err     error
	gotRefs []ContextCostRef
	gotOpts MultiCostOptions
}

func (s *stubCostCalc) CalculateMultiContextCosts(_ context.Context, _ common.Address, _ *big.Int, refs []ContextCostRef, opts MultiCostOptions) (*MultiContextCosts, error) {
	s.gotRefs = append([]ContextCostRef(nil), refs...)
	s.gotOpts = opts
	return s.out, s.err
}

type stubFunder struct {
	called bool
	gotAmt *big.Int
	gotOpt int
}

func (s *stubFunder) FundSync(_ context.Context, amount *big.Int, opts ...payments.WriteOption) (*sdktypes.WriteResult, error) {
	s.called = true
	s.gotAmt = amount
	s.gotOpt = len(opts)
	return &sdktypes.WriteResult{Hash: common.HexToHash("0xdeadbeef")}, nil
}

func newTestService() *Service {
	return &Service{
		httpClient: &http.Client{},
		lifecycle:  lifecycle.New(),
	}
}

func assertInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Prepare error = %v, want ErrInvalidArgument", err)
	}
}

func (c *fakeUploadContext) DataSetID() *sdktypes.BigInt { return c.dataSetID }

func (c *fakeUploadContext) GetProviderInfo() Provider {
	p := testProvider()
	if !c.id.IsZero() {
		p.ID = c.id
	}
	return p
}

func (c *fakeUploadContext) WithCDN() bool {
	if c.dataSetMetadata == nil {
		return false
	}
	_, ok := c.dataSetMetadata["withCDN"]
	return ok
}

func TestPrepare_ReadyShortCircuits(t *testing.T) {
	svc := newTestService()
	svc.costCalc = &stubCostCalc{out: &MultiContextCosts{Ready: true}}
	svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")

	res, err := svc.Prepare(context.Background(), &PrepareOptions{Costs: &MultiContextCosts{Ready: true}})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if res.Transaction != nil {
		t.Fatalf("want nil Transaction when Ready=true, got %+v", res.Transaction)
	}
}

func TestPrepare_RejectsInvalidOptions(t *testing.T) {
	cdn := true
	readyCosts := &MultiContextCosts{Ready: true}
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
				Contexts: []UploadContext{uploadCtx},
			},
		},
		{
			name: "costs with data size",
			opts: &PrepareOptions{
				Costs:    readyCosts,
				DataSize: 128,
			},
		},
		{
			name: "costs with enable cdn",
			opts: &PrepareOptions{
				Costs:     readyCosts,
				EnableCDN: &cdn,
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
			name: "costs with buffer",
			opts: &PrepareOptions{
				Costs:        readyCosts,
				BufferEpochs: 1,
			},
		},
		{
			name: "negative extra runway",
			opts: &PrepareOptions{
				DataSize:          128,
				ExtraRunwayEpochs: -1,
			},
		},
		{
			name: "negative buffer",
			opts: &PrepareOptions{
				DataSize:     128,
				BufferEpochs: -1,
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
	svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")

	res, err := svc.Prepare(context.Background(), &PrepareOptions{
		Costs: &MultiContextCosts{
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

func TestPrepare_RejectsInvalidNotReadyCosts(t *testing.T) {
	uploadCtx := &fakeUploadContext{id: sdktypes.NewBigInt(1)}
	tests := []struct {
		name  string
		opts  *PrepareOptions
		setup func(*Service)
	}{
		{
			name: "supplied nil deposit",
			opts: &PrepareOptions{Costs: &MultiContextCosts{Ready: false}},
		},
		{
			name: "supplied negative deposit",
			opts: &PrepareOptions{
				Costs: &MultiContextCosts{
					Ready:         false,
					DepositNeeded: big.NewInt(-1),
				},
			},
		},
		{
			name: "calculated nil deposit",
			opts: &PrepareOptions{
				DataSize: 128,
				Contexts: []UploadContext{
					uploadCtx,
				},
			},
			setup: func(svc *Service) {
				svc.costCalc = &stubCostCalc{out: &MultiContextCosts{Ready: false}}
				svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")
			},
		},
		{
			name: "calculated negative deposit",
			opts: &PrepareOptions{
				DataSize: 128,
				Contexts: []UploadContext{
					uploadCtx,
				},
			},
			setup: func(svc *Service) {
				svc.costCalc = &stubCostCalc{out: &MultiContextCosts{
					Ready:         false,
					DepositNeeded: big.NewInt(-1),
				}}
				svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")
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
	svc.costCalc = &stubCostCalc{out: &MultiContextCosts{Ready: true}}

	_, err := svc.Prepare(context.Background(), &PrepareOptions{
		DataSize: 128,
		Contexts: []UploadContext{&fakeUploadContext{id: sdktypes.NewBigInt(1)}},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Prepare error = %v, want ErrInvalidArgument", err)
	}
}

func TestPrepare_AutoCreatesContextsWithContextResolver(t *testing.T) {
	costCalc := &stubCostCalc{out: &MultiContextCosts{Ready: true}}
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	svc := newTestService()
	svc.costCalc = costCalc
	svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")
	svc.contextResolver = &fakeResolver{contextContexts: []*Context{ctx}}

	_, err = svc.Prepare(context.Background(), &PrepareOptions{DataSize: 128})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if len(costCalc.gotRefs) != 1 {
		t.Fatalf("len(gotRefs)=%d want 1", len(costCalc.gotRefs))
	}
	if !costCalc.gotRefs[0].Provider.ID.Equal(testProvider().ID) {
		t.Fatalf("ProviderID=%s want %s", costCalc.gotRefs[0].Provider.ID, testProvider().ID)
	}
}

func TestPrepare_RejectsEnableCDNWhenContextsSupplied(t *testing.T) {
	costCalc := &stubCostCalc{out: &MultiContextCosts{Ready: true}}
	svc := newTestService()
	svc.costCalc = costCalc
	svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")

	cdn := true
	_, err := svc.Prepare(context.Background(), &PrepareOptions{
		DataSize:     128,
		EnableCDN:    &cdn,
		Contexts:     []UploadContext{&fakeUploadContext{id: sdktypes.NewBigInt(1)}},
		BufferEpochs: 9,
	})
	assertInvalidArgument(t, err)
}

func TestPrepare_AllowsRunwayAndBufferWhenContextsSupplied(t *testing.T) {
	costCalc := &stubCostCalc{out: &MultiContextCosts{Ready: true}}
	svc := newTestService()
	svc.costCalc = costCalc
	svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")

	_, err := svc.Prepare(context.Background(), &PrepareOptions{
		DataSize:          128,
		Contexts:          []UploadContext{&fakeUploadContext{id: sdktypes.NewBigInt(1)}},
		ExtraRunwayEpochs: 7,
		BufferEpochs:      9,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if costCalc.gotOpts.ExtraRunwayEpochs != 7 {
		t.Fatalf("ExtraRunwayEpochs=%d want 7", costCalc.gotOpts.ExtraRunwayEpochs)
	}
	if costCalc.gotOpts.BufferEpochs != 9 {
		t.Fatalf("BufferEpochs=%d want 9", costCalc.gotOpts.BufferEpochs)
	}
	if len(costCalc.gotRefs) != 1 {
		t.Fatalf("len(gotRefs)=%d want 1", len(costCalc.gotRefs))
	}
}

func TestPrepare_ReturnsErrorWhenCostCalculatorReturnsNil(t *testing.T) {
	svc := newTestService()
	svc.costCalc = &stubCostCalc{}
	svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Prepare panicked: %v", recovered)
		}
	}()

	_, err := svc.Prepare(context.Background(), &PrepareOptions{
		DataSize: 128,
		Contexts: []UploadContext{
			&fakeUploadContext{id: sdktypes.NewBigInt(1)},
		},
	})
	if err == nil {
		t.Fatal("Prepare error = nil, want error")
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Prepare error = %v, want internal error", err)
	}
	if !strings.Contains(err.Error(), "cost calculator returned nil costs") {
		t.Fatalf("Prepare error = %v, want nil-costs message", err)
	}
}
