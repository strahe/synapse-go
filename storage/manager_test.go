package storage

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type managerDataSetFinder struct {
	payer       common.Address
	onlyManaged bool
	result      []*DataSetInfo
	err         error
}

func (f *managerDataSetFinder) FindDataSets(_ context.Context, payer common.Address, onlyManaged bool) ([]*DataSetInfo, error) {
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

type managerTerminator struct {
	dataSetID types.BigInt
	optCount  int
	result    *types.WriteResult
	err       error
}

func (t *managerTerminator) TerminateDataSet(_ context.Context, dataSetID types.BigInt, opts ...warmstorage.WriteOption) (*types.WriteResult, error) {
	t.dataSetID = dataSetID
	t.optCount = len(opts)
	return t.result, t.err
}

type managerCostCalculator struct {
	payer  common.Address
	size   *big.Int
	refs   []ContextCostRef
	opts   MultiCostOptions
	result *MultiContextCosts
	err    error
}

func (c *managerCostCalculator) CalculateMultiContextCosts(
	_ context.Context,
	payer common.Address,
	size *big.Int,
	refs []ContextCostRef,
	opts MultiCostOptions,
) (*MultiContextCosts, error) {
	c.payer = payer
	c.size = new(big.Int).Set(size)
	c.refs = refs
	c.opts = opts
	return c.result, c.err
}

func TestServiceManagerFacades_ForwardConfiguredInputs(t *testing.T) {
	defaultPayer := common.HexToAddress("0x1001")
	override := common.HexToAddress("0x2002")
	wantSets := []*DataSetInfo{{DataSetInfo: &warmstorage.DataSetInfo{DataSetID: types.NewBigInt(7)}}}
	wantInfo := &StorageInfo{}
	wantWrite := &types.WriteResult{Hash: common.HexToHash("0x1234")}
	wantCosts := &MultiContextCosts{RatePerEpoch: big.NewInt(3)}
	finder := &managerDataSetFinder{result: wantSets}
	info := &managerStorageInfoReader{result: wantInfo}
	terminator := &managerTerminator{result: wantWrite}
	calculator := &managerCostCalculator{result: wantCosts}
	svc, err := New(Options{
		PayerAddress:      defaultPayer,
		DataSetFinder:     finder,
		StorageInfoReader: info,
		DataSetTerminator: terminator,
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

	dataSetID := types.NewBigInt(9)
	gotWrite, err := svc.TerminateDataSet(context.Background(), dataSetID, &TerminateDataSetOptions{
		WriteOptions: []warmstorage.WriteOption{warmstorage.WithWait(0)},
	})
	if err != nil || gotWrite != wantWrite || !terminator.dataSetID.Equal(dataSetID) || terminator.optCount != 1 {
		t.Fatalf("TerminateDataSet = %+v, %v; forwarded id=%s opts=%d", gotWrite, err, terminator.dataSetID, terminator.optCount)
	}

	refs := []ContextCostRef{{Provider: testProvider()}}
	opts := MultiCostOptions{EnableCDN: true, PieceCount: big.NewInt(2)}
	gotCosts, err := svc.CalculateMultiContextCosts(context.Background(), 4096, refs, opts, common.Address{})
	if err != nil || gotCosts != wantCosts {
		t.Fatalf("CalculateMultiContextCosts = %+v, %v", gotCosts, err)
	}
	if calculator.payer != defaultPayer || calculator.size.Uint64() != 4096 || len(calculator.refs) != 1 || !calculator.opts.EnableCDN {
		t.Fatalf("CalculateMultiContextCosts forwarded payer=%s size=%s refs=%d opts=%+v", calculator.payer, calculator.size, len(calculator.refs), calculator.opts)
	}
}

func TestServiceManagerFacades_ValidateConfiguration(t *testing.T) {
	svc, err := New(Options{})
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
		{"TerminateDataSet", func() error {
			_, err := svc.TerminateDataSet(context.Background(), types.NewBigInt(1), nil)
			return err
		}, ErrUninitialized},
		{"CalculateMultiContextCosts", func() error {
			_, err := svc.CalculateMultiContextCosts(context.Background(), 1, []ContextCostRef{{}}, MultiCostOptions{}, common.Address{})
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
	svc, err := New(Options{
		DataSetFinder:     &managerDataSetFinder{},
		StorageInfoReader: &managerStorageInfoReader{},
		DataSetTerminator: &managerTerminator{},
		CostCalculator:    &managerCostCalculator{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FindDataSets(context.Background(), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("FindDataSets error = %v, want ErrInvalidArgument", err)
	}
	if _, err := svc.TerminateDataSet(context.Background(), types.NewBigInt(0), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("TerminateDataSet error = %v, want ErrInvalidArgument", err)
	}
	if _, err := svc.CalculateMultiContextCosts(context.Background(), 1, nil, MultiCostOptions{}, defaultPayer); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CalculateMultiContextCosts(empty refs) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := svc.CalculateMultiContextCosts(context.Background(), 1, []ContextCostRef{{}}, MultiCostOptions{}, common.Address{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CalculateMultiContextCosts(zero payer) error = %v, want ErrInvalidArgument", err)
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
