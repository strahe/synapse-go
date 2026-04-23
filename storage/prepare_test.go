package storage

import (
	"context"
	"errors"
	"math/big"
	"net/http"
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

func (c *fakeUploadContext) DataSetID() *sdktypes.DataSetID { return c.dataSetID }

func (c *fakeUploadContext) GetProviderInfo() Provider {
	p := testProvider()
	if c.id != 0 {
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

func TestPrepare_RejectsZeroDefaultPayer(t *testing.T) {
	svc := newTestService()
	svc.costCalc = &stubCostCalc{out: &MultiContextCosts{Ready: true}}

	_, err := svc.Prepare(context.Background(), &PrepareOptions{
		DataSize: 128,
		Contexts: []UploadContext{&fakeUploadContext{id: 1}},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Prepare error = %v, want ErrInvalidArgument", err)
	}
}

func TestPrepare_IgnoresEnableCDNWhenContextsSupplied(t *testing.T) {
	costCalc := &stubCostCalc{out: &MultiContextCosts{Ready: true}}
	svc := newTestService()
	svc.costCalc = costCalc
	svc.signerAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")

	cdn := true
	_, err := svc.Prepare(context.Background(), &PrepareOptions{
		DataSize:     128,
		EnableCDN:    &cdn,
		Contexts:     []UploadContext{&fakeUploadContext{id: 1}},
		BufferEpochs: 9,
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if costCalc.gotOpts.EnableCDN {
		t.Fatal("Prepare forwarded EnableCDN despite explicit Contexts")
	}
	if len(costCalc.gotRefs) != 1 {
		t.Fatalf("len(gotRefs)=%d want 1", len(costCalc.gotRefs))
	}
	if costCalc.gotRefs[0].WithCDN {
		t.Fatal("Prepare mutated explicit context ref to WithCDN=true")
	}
}
