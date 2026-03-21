package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestServiceResolverResolveUploadContexts_AutoSelectsApprovedProvidersAndReusesMatchingDataSet(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []*big.Int{big.NewInt(2), big.NewInt(1), big.NewInt(3)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(1, "https://sp-1.example.com"),
			testPDPProvider(2, "https://sp-2.example.com"),
			testPDPProvider(3, "https://sp-3.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: big.NewInt(11), ProviderID: big.NewInt(1), PDPEndEpoch: big.NewInt(0)},
			{DataSetID: big.NewInt(12), ProviderID: big.NewInt(1), PDPEndEpoch: big.NewInt(0)},
			{DataSetID: big.NewInt(21), ProviderID: big.NewInt(2), PDPEndEpoch: big.NewInt(7)},
		},
		dataSetMetadata: map[string]map[string]string{
			"11": {"source": "app", "withCDN": ""},
			"12": {"source": "other"},
		},
	})

	contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          2,
		DataSetMetadata: map[string]string{"source": "app"},
		WithCDN:         true,
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if explicit {
		t.Fatal("explicit=true want false")
	}
	if len(contexts) != 2 {
		t.Fatalf("contexts len=%d want 2", len(contexts))
	}

	got := contextsToFake(t, contexts)
	if got[0].id.Cmp(big.NewInt(1)) != 0 || got[0].dataSetID.Cmp(big.NewInt(11)) != 0 {
		t.Fatalf("first context provider=%s dataset=%s want provider=1 dataset=11", got[0].id, got[0].dataSetID)
	}
	if got[1].id.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("second context provider=%s want 2", got[1].id)
	}
	if got[1].dataSetID != nil {
		t.Fatalf("second context dataset=%v want nil", got[1].dataSetID)
	}
	if got[1].dataSetMetadata["withCDN"] != "" || got[1].dataSetMetadata["source"] != "app" {
		t.Fatalf("second context metadata=%v", got[1].dataSetMetadata)
	}
}

func TestServiceResolverResolveUploadContexts_ExplicitProviderIDsDisableReplacement(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			"7": ptrPDPProvider(testPDPProvider(7, "https://sp-7.example.com")),
			"8": ptrPDPProvider(testPDPProvider(8, "https://sp-8.example.com")),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: big.NewInt(71), ProviderID: big.NewInt(7), PDPEndEpoch: big.NewInt(0)},
		},
		dataSetMetadata: map[string]map[string]string{
			"71": {"source": "app"},
		},
	})

	contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          2,
		ProviderIDs:     []*big.Int{big.NewInt(7), big.NewInt(7), big.NewInt(8)},
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if !explicit {
		t.Fatal("explicit=false want true")
	}
	got := contextsToFake(t, contexts)
	if len(got) != 2 {
		t.Fatalf("contexts len=%d want 2", len(got))
	}
	if got[0].id.Cmp(big.NewInt(7)) != 0 || got[0].dataSetID.Cmp(big.NewInt(71)) != 0 {
		t.Fatalf("first context=%+v", got[0])
	}
	if got[1].id.Cmp(big.NewInt(8)) != 0 || got[1].dataSetID != nil {
		t.Fatalf("second context=%+v", got[1])
	}
}

func TestServiceResolverSelectReplacement_ExcludesUsedProviders(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(1, "https://sp-1.example.com"),
			testPDPProvider(2, "https://sp-2.example.com"),
			testPDPProvider(3, "https://sp-3.example.com"),
		},
	})

	replacement, err := resolver.SelectReplacement(context.Background(), map[string]struct{}{
		"1": {},
		"2": {},
	}, &UploadOptions{})
	if err != nil {
		t.Fatalf("SelectReplacement: %v", err)
	}
	got := replacement.(*fakeUploadContext)
	if got.id.Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("replacement provider=%s want 3", got.id)
	}
}

func TestServiceResolverResolveUploadContexts_ExplicitDataSetIDsValidateOwnership(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			"33": {DataSetID: big.NewInt(33), ProviderID: big.NewInt(5), Payer: common.HexToAddress("0x00000000000000000000000000000000000000ff"), PDPEndEpoch: big.NewInt(0)},
		},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []*big.Int{big.NewInt(33)},
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected ownership error")
	}
}

type serviceResolverFixture struct {
	approvedProviderIDs []*big.Int
	activeProviders     []spregistry.PDPProvider
	clientDataSets      []*warmstorage.DataSetInfo
	dataSetMetadata     map[string]map[string]string
	providersByID       map[string]*spregistry.PDPProvider
	dataSetsByID        map[string]*warmstorage.DataSetInfo
}

func newTestServiceResolver(t *testing.T, fixture serviceResolverFixture) *ServiceResolver {
	t.Helper()
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:       testPayer(),
		SPRegistry:  &fakePDPProviderSource{fixture: fixture},
		WarmStorage: &fakeDataSetCatalog{fixture: fixture},
		NewContext: func(selection ResolvedUploadContext, _ *UploadOptions) (UploadContext, error) {
			return &fakeUploadContext{
				id:              selection.Provider.ID,
				endpoint:        selection.Provider.ServiceURL,
				dataSetID:       copyBigInt(selection.DataSetID),
				clientDataSetID: copyBigInt(selection.ClientDataSetID),
				dataSetMetadata: cloneStringMap(selection.DataSetMetadata),
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}
	return resolver
}

type fakePDPProviderSource struct {
	fixture serviceResolverFixture
}

func (f *fakePDPProviderSource) GetPDPProvider(_ context.Context, providerID *big.Int) (*spregistry.PDPProvider, error) {
	if providerID == nil {
		return nil, errors.New("nil providerID")
	}
	if f.fixture.providersByID != nil {
		return f.fixture.providersByID[providerID.String()], nil
	}
	for _, provider := range f.fixture.activeProviders {
		if provider.Info.ID.Cmp(providerID) == 0 {
			return ptrPDPProvider(provider), nil
		}
	}
	return nil, nil
}

func (f *fakePDPProviderSource) SelectActivePDPProviders(_ context.Context, filter spregistry.ProviderFilter) ([]spregistry.PDPProvider, error) {
	var out []spregistry.PDPProvider
	for _, provider := range f.fixture.activeProviders {
		if containsExcludedProvider(filter.ExcludeIDs, provider.Info.ID) {
			continue
		}
		out = append(out, provider)
	}
	return out, nil
}

type fakeDataSetCatalog struct {
	fixture serviceResolverFixture
}

func (f *fakeDataSetCatalog) GetApprovedProviderIDs(_ context.Context, _, _ *big.Int) ([]*big.Int, error) {
	return cloneBigInts(f.fixture.approvedProviderIDs), nil
}

func (f *fakeDataSetCatalog) GetClientDataSets(_ context.Context, _ common.Address, _, _ *big.Int) ([]*warmstorage.DataSetInfo, error) {
	out := make([]*warmstorage.DataSetInfo, 0, len(f.fixture.clientDataSets))
	for _, dataSet := range f.fixture.clientDataSets {
		cloned := *dataSet
		out = append(out, &cloned)
	}
	return out, nil
}

func (f *fakeDataSetCatalog) GetDataSet(_ context.Context, dataSetID *big.Int) (*warmstorage.DataSetInfo, error) {
	if f.fixture.dataSetsByID == nil {
		return nil, nil
	}
	dataSet := f.fixture.dataSetsByID[dataSetID.String()]
	if dataSet == nil {
		return nil, nil
	}
	cloned := *dataSet
	return &cloned, nil
}

func (f *fakeDataSetCatalog) GetAllDataSetMetadata(_ context.Context, dataSetID *big.Int) (map[string]string, error) {
	if f.fixture.dataSetMetadata == nil {
		return nil, nil
	}
	return cloneStringMap(f.fixture.dataSetMetadata[dataSetID.String()]), nil
}

func testPDPProvider(id int64, serviceURL string) spregistry.PDPProvider {
	return spregistry.PDPProvider{
		Info: spregistry.ProviderInfo{
			ID:              big.NewInt(id),
			ServiceProvider: common.HexToAddress(fmt.Sprintf("0x%040x", id)),
			Payee:           common.HexToAddress(fmt.Sprintf("0x%040x", id+100)),
			IsActive:        true,
		},
		Offering: spregistry.PDPOffering{ServiceURL: serviceURL},
	}
}

func ptrPDPProvider(provider spregistry.PDPProvider) *spregistry.PDPProvider {
	cp := provider
	return &cp
}

func containsExcludedProvider(values []*big.Int, target *big.Int) bool {
	for _, value := range values {
		if value != nil && value.Cmp(target) == 0 {
			return true
		}
	}
	return false
}

func contextsToFake(t *testing.T, contexts []UploadContext) []*fakeUploadContext {
	t.Helper()
	out := make([]*fakeUploadContext, 0, len(contexts))
	for _, ctx := range contexts {
		fake, ok := ctx.(*fakeUploadContext)
		if !ok {
			t.Fatalf("unexpected context type %T", ctx)
		}
		out = append(out, fake)
	}
	return out
}

// TestServiceResolverResolveUploadContexts_CarriesClientDataSetID proves that
// when an existing dataset is matched, its ClientDataSetID is present in the
// ResolvedUploadContext so the ContextFactory can pass it to WithClientDataSetID.
func TestServiceResolverResolveUploadContexts_CarriesClientDataSetID(t *testing.T) {
	clientDataSetID := big.NewInt(0xABCD)
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []*big.Int{big.NewInt(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(1, "https://sp-1.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: big.NewInt(11), ProviderID: big.NewInt(1), PDPEndEpoch: big.NewInt(0), ClientDataSetID: clientDataSetID},
		},
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("len(contexts)=%d want 1", len(contexts))
	}
	got := contextsToFake(t, contexts)
	if got[0].clientDataSetID == nil || got[0].clientDataSetID.Cmp(clientDataSetID) != 0 {
		t.Fatalf("clientDataSetID=%v want %s (ClientDataSetID not threaded through resolver)", got[0].clientDataSetID, clientDataSetID)
	}
}

// TestServiceResolverResolveUploadContexts_ExplicitDataSetIDsRejectsInactive proves
// that the explicit DataSetIDs path enforces the active-dataset constraint
// (PDPEndEpoch == 0), matching the behaviour of autoSelect.
func TestServiceResolverResolveUploadContexts_ExplicitDataSetIDsRejectsInactive(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			"55": {
				DataSetID:   big.NewInt(55),
				ProviderID:  big.NewInt(5),
				Payer:       testPayer(),
				PDPEndEpoch: big.NewInt(1000), // non-zero: inactive
			},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			"5": ptrPDPProvider(testPDPProvider(5, "https://sp-5.example.com")),
		},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []*big.Int{big.NewInt(55)},
	})
	if err == nil {
		t.Fatal("expected error for inactive dataset, got nil")
	}
}
