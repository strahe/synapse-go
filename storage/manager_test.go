package storage

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type managerDataSetFinder struct {
	payer       common.Address
	onlyManaged bool
	result      []*DataSetDetails
	err         error
}

func (f *managerDataSetFinder) FindDataSets(_ context.Context, payer common.Address, onlyManaged bool) ([]*DataSetDetails, error) {
	f.payer = payer
	f.onlyManaged = onlyManaged
	return f.result, f.err
}

type managerStorageInfoReader struct {
	client common.Address
	result *StorageInfo
	err    error
}

func (r *managerStorageInfoReader) GetStorageInfo(_ context.Context, client common.Address) (*StorageInfo, error) {
	r.client = client
	return r.result, r.err
}

type managerCostCalculator struct {
	payer  common.Address
	size   *big.Int
	refs   []costs.MultiContextRef
	opts   costs.UploadCostOptions
	result *costs.MultiContextCosts
	err    error
}

func (c *managerCostCalculator) CalculateMultiContextCosts(
	_ context.Context,
	payer common.Address,
	size *big.Int,
	refs []costs.MultiContextRef,
	opts *costs.UploadCostOptions,
) (*costs.MultiContextCosts, error) {
	c.payer = payer
	c.size = new(big.Int).Set(size)
	c.refs = refs
	c.opts = *opts
	return c.result, c.err
}

func TestServiceManagerFacades_ForwardConfiguredInputs(t *testing.T) {
	defaultPayer := common.HexToAddress("0x1001")
	override := common.HexToAddress("0x2002")
	wantSets := []*DataSetDetails{{DataSetInfo: warmstorage.DataSetInfo{DataSetID: types.NewBigInt(7)}}}
	wantInfo := &StorageInfo{}
	wantCosts := &costs.MultiContextCosts{RatePerEpoch: big.NewInt(3)}
	finder := &managerDataSetFinder{result: wantSets}
	info := &managerStorageInfoReader{result: wantInfo}
	calculator := &managerCostCalculator{result: wantCosts}
	svc, err := New(Options{
		PayerAddress:      defaultPayer,
		DataSetFinder:     finder,
		StorageInfoReader: info,
		CostCalculator:    calculator,
	})
	if err != nil {
		t.Fatal(err)
	}

	sets, err := svc.FindDataSets(context.Background(), &FindDataSetsOptions{Payer: override, OnlyManaged: true})
	if err != nil || len(sets) != 1 || sets[0] != wantSets[0] {
		t.Fatalf("FindDataSets = %+v, %v", sets, err)
	}
	if finder.payer != override || !finder.onlyManaged {
		t.Fatalf("FindDataSets forwarded payer=%s onlyManaged=%v", finder.payer, finder.onlyManaged)
	}

	gotInfo, err := svc.GetStorageInfo(context.Background(), &GetStorageInfoOptions{Client: override})
	if err != nil || gotInfo != wantInfo || info.client != override {
		t.Fatalf("GetStorageInfo = %+v, %v; client=%s", gotInfo, err, info.client)
	}

	dataSetID := types.NewBigInt(7)
	otherDataSetID := types.NewBigInt(8)
	refs := []ContextCostRef{
		{Provider: testProvider(), CurrentDataSetSizeBytes: big.NewInt(9999)},
		{DataSetID: &dataSetID, CurrentDataSetSizeBytes: big.NewInt(2048)},
		{DataSetID: &otherDataSetID, CurrentDataSetSizeBytes: big.NewInt(8192), WithCDN: true},
	}
	opts := MultiCostOptions{EnableCDN: true, PieceCount: big.NewInt(2), ExtraRunwayEpochs: 7, BufferEpochs: new(int64(0))}
	gotCosts, err := svc.CalculateMultiContextCosts(context.Background(), 4096, refs, opts, common.Address{})
	if err != nil || gotCosts == nil || gotCosts.RatePerEpoch.Cmp(wantCosts.RatePerEpoch) != 0 {
		t.Fatalf("CalculateMultiContextCosts = %+v, %v", gotCosts, err)
	}
	if calculator.payer != defaultPayer || calculator.size.Uint64() != 4096 || len(calculator.refs) != len(refs) {
		t.Fatalf("CalculateMultiContextCosts forwarded payer=%s size=%s refs=%d opts=%+v", calculator.payer, calculator.size, len(calculator.refs), calculator.opts)
	}
	if calculator.opts.BufferEpochs == nil || *calculator.opts.BufferEpochs != 0 {
		t.Fatalf("CalculateMultiContextCosts BufferEpochs=%v want explicit zero", calculator.opts.BufferEpochs)
	}
	if calculator.opts.ExtraRunwayEpochs != 7 || calculator.opts.PieceCount.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("CalculateMultiContextCosts forwarded opts=%+v", calculator.opts)
	}
	if !calculator.refs[0].IsNewDataSet || calculator.refs[0].CurrentDataSetSizeBytes != nil {
		t.Fatalf("new ref=%+v want new dataset without current size", calculator.refs[0])
	}
	for i := 1; i < len(refs); i++ {
		if calculator.refs[i].IsNewDataSet || calculator.refs[i].CurrentDataSetSizeBytes.Cmp(refs[i].CurrentDataSetSizeBytes) != 0 {
			t.Fatalf("ref[%d]=%+v want existing dataset size %s", i, calculator.refs[i], refs[i].CurrentDataSetSizeBytes)
		}
	}
	for i, ref := range calculator.refs {
		if !ref.WithCDN {
			t.Fatalf("ref[%d] CDN not enabled by global option", i)
		}
	}

	opts.EnableCDN = false
	if _, err := svc.CalculateMultiContextCosts(context.Background(), 4096, refs, opts, override); err != nil {
		t.Fatal(err)
	}
	if calculator.payer != override {
		t.Fatalf("payer=%s want explicit payer %s", calculator.payer, override)
	}
	for i, ref := range calculator.refs {
		if ref.WithCDN != refs[i].WithCDN {
			t.Fatalf("ref[%d].WithCDN=%v want %v", i, ref.WithCDN, refs[i].WithCDN)
		}
	}

	calculator.err = context.Canceled
	if _, err := svc.CalculateMultiContextCosts(context.Background(), 4096, refs, opts, override); !errors.Is(err, context.Canceled) || errors.Unwrap(err) != nil {
		t.Fatalf("CalculateMultiContextCosts error=%v want unwrapped cancellation", err)
	}
	calculator.err = nil
	calculator.result = nil
	if result, err := svc.CalculateMultiContextCosts(context.Background(), 4096, refs, opts, override); result != nil || err != nil {
		t.Fatalf("CalculateMultiContextCosts=%+v, %v want nil calculator result unchanged", result, err)
	}
}

func TestServiceManagerFacades_ValidateConfiguration(t *testing.T) {
	svc, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	typedNilSvc, err := New(Options{CostCalculator: (*costs.Service)(nil)})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{"FindDataSets", func() error { _, err := svc.FindDataSets(context.Background(), nil); return err }, ErrUninitialized},
		{"GetStorageInfo", func() error { _, err := svc.GetStorageInfo(context.Background(), nil); return err }, ErrUninitialized},
		{"CalculateMultiContextCosts", func() error {
			_, err := svc.CalculateMultiContextCosts(context.Background(), 1, []ContextCostRef{{}}, MultiCostOptions{}, common.Address{})
			return err
		}, ErrUninitialized},
		{"CalculateMultiContextCosts typed nil", func() error {
			_, err := typedNilSvc.CalculateMultiContextCosts(context.Background(), 1, []ContextCostRef{{}}, MultiCostOptions{}, common.Address{})
			return err
		}, ErrUninitialized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.call(); !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestServiceManagerFacades_ValidateArguments(t *testing.T) {
	defaultPayer := common.HexToAddress("0x3003")
	calculator := &managerCostCalculator{}
	svc, err := New(Options{
		DataSetFinder:     &managerDataSetFinder{},
		StorageInfoReader: &managerStorageInfoReader{},
		CostCalculator:    calculator,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FindDataSets(context.Background(), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("FindDataSets error = %v, want ErrInvalidArgument", err)
	}
	if _, err := svc.CalculateMultiContextCosts(context.Background(), 1, nil, MultiCostOptions{}, defaultPayer); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CalculateMultiContextCosts(empty refs) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := svc.CalculateMultiContextCosts(context.Background(), 1, []ContextCostRef{{}}, MultiCostOptions{}, common.Address{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CalculateMultiContextCosts(zero payer) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := svc.CalculateMultiContextCosts(context.Background(), 1, []ContextCostRef{{}}, MultiCostOptions{BufferEpochs: new(int64(-1))}, defaultPayer); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CalculateMultiContextCosts(negative buffer) error = %v, want ErrInvalidArgument", err)
	}
	if calculator.size != nil {
		t.Fatalf("cost calculator called with size=%s", calculator.size)
	}
}

type managerProviderSource struct {
	provider *spregistry.PDPProvider
	err      error
}

func (s *managerProviderSource) GetPDPProvider(context.Context, types.BigInt) (*spregistry.PDPProvider, error) {
	return s.provider, s.err
}

func (*managerProviderSource) SelectActivePDPProviders(context.Context, spregistry.ProviderFilter) ([]spregistry.PDPProvider, error) {
	return nil, nil
}

func TestServiceResolverResolveProvider(t *testing.T) {
	want := spregistry.PDPProvider{
		Info: spregistry.ProviderInfo{
			ID:              types.NewBigInt(44),
			ServiceProvider: common.HexToAddress("0x4401"),
			Payee:           common.HexToAddress("0x4402"),
		},
		Offering: spregistry.PDPOffering{ServiceURL: "https://provider.example"},
	}
	resolver := &ServiceResolver{spRegistry: &managerProviderSource{provider: &want}}
	got, err := resolver.ResolveProvider(context.Background(), want.Info.ID)
	if err != nil {
		t.Fatalf("ResolveProvider: %v", err)
	}
	if !got.ID.Equal(want.Info.ID) || got.ServiceURL != want.Offering.ServiceURL ||
		got.ServiceProvider != want.Info.ServiceProvider || got.Payee != want.Info.Payee {
		t.Fatalf("ResolveProvider = %+v, want %+v", got, want)
	}
	if _, err := resolver.ResolveProvider(context.Background(), types.NewBigInt(0)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ResolveProvider(0) error = %v, want ErrInvalidArgument", err)
	}
}
