package storage

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func testID(v uint64) types.BigInt {
	return types.NewBigInt(v)
}

func testIDKey(v uint64) string {
	return idconv.Key(testID(v))
}

func TestServiceResolverResolveUploadContexts_AutoSelectsApprovedProvidersAndReusesMatchingDataSet(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(2), testID(1), testID(3)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
			testPDPProvider(testID(2), "https://sp-2.example.com"),
			testPDPProvider(testID(3), "https://sp-3.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
			{DataSetID: testID(12), ProviderID: testID(1), PDPEndEpoch: 0},
			{DataSetID: testID(21), ProviderID: testID(2), PDPEndEpoch: 7},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(11): {"source": "app", "withCDN": ""},
			testIDKey(12): {"source": "other"},
		},
	})

	withCDN := true
	contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          2,
		DataSetMetadata: map[string]string{"source": "app"},
		WithCDN:         &withCDN,
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
	if !got[0].id.Equal(testID(1)) || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(11)) {
		t.Fatalf("first context provider=%d dataset=%v want provider=1 dataset=11", got[0].id, got[0].dataSetID)
	}
	if !got[1].id.Equal(testID(2)) {
		t.Fatalf("second context provider=%d want 2", got[1].id)
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
			testIDKey(7): ptrPDPProvider(testPDPProvider(testID(7), "https://sp-7.example.com")),
			testIDKey(8): ptrPDPProvider(testPDPProvider(testID(8), "https://sp-8.example.com")),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(71), ProviderID: testID(7), PDPEndEpoch: 0},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(71): {"source": "app"},
		},
	})

	contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          2,
		ProviderIDs:     []types.BigInt{testID(7), testID(7), testID(8)},
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
	if !got[0].id.Equal(testID(7)) || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(71)) {
		t.Fatalf("first context=%+v", got[0])
	}
	if !got[1].id.Equal(testID(8)) || got[1].dataSetID != nil {
		t.Fatalf("second context=%+v", got[1])
	}
}

func TestServiceResolverSelectReplacement_ExcludesUsedProviders(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1), testID(2), testID(3)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
			testPDPProvider(testID(2), "https://sp-2.example.com"),
			testPDPProvider(testID(3), "https://sp-3.example.com"),
		},
	})

	replacement, err := resolver.SelectReplacement(context.Background(), map[string]types.BigInt{
		testIDKey(1): testID(1),
		testIDKey(2): testID(2),
	}, &UploadOptions{})
	if err != nil {
		t.Fatalf("SelectReplacement: %v", err)
	}
	got := replacement.(*fakeUploadContext)
	if !got.id.Equal(testID(3)) {
		t.Fatalf("replacement provider=%d want 3", got.id)
	}
}

func TestServiceResolverResolveUploadContexts_ExplicitDataSetIDsValidateOwnership(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(33): {DataSetID: testID(33), ProviderID: testID(5), Payer: common.HexToAddress("0x00000000000000000000000000000000000000ff"), PDPEndEpoch: 0},
		},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(33)},
	})
	if err == nil || err.Error() == "" {
		t.Fatal("expected ownership error")
	}
}

func TestServiceResolverResolveUploadContexts_RetriesTransientSelectionErrors(t *testing.T) {
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
	}
	catalog := &flakyClientDataSetsCatalog{
		fakeDataSetCatalog: fakeDataSetCatalog{fixture: fixture},
	}
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:       testPayer(),
		SPRegistry:  &fakePDPProviderSource{fixture: fixture},
		WarmStorage: catalog,
		NewContext: func(selection ResolvedUploadContext, _ *UploadOptions) (UploadContext, error) {
			return &fakeUploadContext{
				id:       selection.Provider.ID,
				endpoint: selection.Provider.ServiceURL,
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if len(contexts) != 1 {
		t.Fatalf("contexts=%d want 1", len(contexts))
	}
	if attempts := catalog.attempts.Load(); attempts != 2 {
		t.Fatalf("GetClientDataSets attempts=%d want 2", attempts)
	}
}

type serviceResolverFixture struct {
	approvedProviderIDs      []types.BigInt
	activeProviders          []spregistry.PDPProvider
	clientDataSets           []*warmstorage.DataSetInfo
	dataSetMetadata          map[string]map[string]string
	providersByID            map[string]*spregistry.PDPProvider
	dataSetsByID             map[string]*warmstorage.DataSetInfo
	requirePositiveListLimit bool
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
				dataSetID:       selection.DataSetID,
				clientDataSetID: selection.ClientDataSetID,
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

func (f *fakePDPProviderSource) GetPDPProvider(_ context.Context, providerID types.BigInt) (*spregistry.PDPProvider, error) {
	if providerID.IsZero() {
		return nil, fmt.Errorf("fakePDPProviderSource.GetPDPProvider: %w: zero providerID", spregistry.ErrInvalidArgument)
	}
	if f.fixture.providersByID != nil {
		if p, ok := f.fixture.providersByID[idconv.Key(providerID)]; ok && p != nil {
			return p, nil
		}
		return nil, fmt.Errorf("fakePDPProviderSource.GetPDPProvider: %w", spregistry.ErrNotFound)
	}
	for _, provider := range f.fixture.activeProviders {
		if provider.Info.ID.Equal(providerID) {
			return ptrPDPProvider(provider), nil
		}
	}
	return nil, fmt.Errorf("fakePDPProviderSource.GetPDPProvider: %w", spregistry.ErrNotFound)
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

type flakyClientDataSetsCatalog struct {
	fakeDataSetCatalog
	attempts atomic.Int32
}

func (f *flakyClientDataSetsCatalog) GetClientDataSets(ctx context.Context, payer common.Address, opts types.ListOptions) ([]*warmstorage.DataSetInfo, error) {
	if f.attempts.Add(1) == 1 {
		return nil, fmt.Errorf("Post %q: EOF", "https://api.calibration.node.glif.io/rpc/v1")
	}
	return f.fakeDataSetCatalog.GetClientDataSets(ctx, payer, opts)
}

func (f *fakeDataSetCatalog) GetApprovedProviderIDs(_ context.Context, opts types.ListOptions) ([]types.BigInt, error) {
	if f.fixture.requirePositiveListLimit {
		if err := opts.Validate(); err != nil {
			return nil, err
		}
	}
	start := int(opts.Offset)
	if start > len(f.fixture.approvedProviderIDs) {
		start = len(f.fixture.approvedProviderIDs)
	}
	end := len(f.fixture.approvedProviderIDs)
	if opts.Limit > 0 {
		if limitEnd := start + int(opts.Limit); limitEnd < end {
			end = limitEnd
		}
	}
	out := make([]types.BigInt, end-start)
	copy(out, f.fixture.approvedProviderIDs[start:end])
	return out, nil
}

func (f *fakeDataSetCatalog) GetClientDataSets(_ context.Context, _ common.Address, opts types.ListOptions) ([]*warmstorage.DataSetInfo, error) {
	if f.fixture.requirePositiveListLimit {
		if err := opts.Validate(); err != nil {
			return nil, err
		}
	}
	start := int(opts.Offset)
	if start > len(f.fixture.clientDataSets) {
		start = len(f.fixture.clientDataSets)
	}
	end := len(f.fixture.clientDataSets)
	if opts.Limit > 0 {
		if limitEnd := start + int(opts.Limit); limitEnd < end {
			end = limitEnd
		}
	}
	out := make([]*warmstorage.DataSetInfo, 0, end-start)
	for _, dataSet := range f.fixture.clientDataSets[start:end] {
		cloned := *dataSet
		out = append(out, &cloned)
	}
	return out, nil
}

func (f *fakeDataSetCatalog) GetDataSet(_ context.Context, dataSetID types.BigInt) (*warmstorage.DataSetInfo, error) {
	if f.fixture.dataSetsByID == nil {
		return nil, fmt.Errorf("fakeDataSetCatalog.GetDataSet: %w", warmstorage.ErrNotFound)
	}
	dataSet := f.fixture.dataSetsByID[idconv.Key(dataSetID)]
	if dataSet == nil {
		return nil, fmt.Errorf("fakeDataSetCatalog.GetDataSet: %w", warmstorage.ErrNotFound)
	}
	cloned := *dataSet
	return &cloned, nil
}

func (f *fakeDataSetCatalog) GetAllDataSetMetadata(_ context.Context, dataSetID types.BigInt) (map[string]string, error) {
	if f.fixture.dataSetMetadata == nil {
		return map[string]string{}, nil
	}
	return cloneStringMap(f.fixture.dataSetMetadata[idconv.Key(dataSetID)]), nil
}

func testPDPProvider(id types.BigInt, serviceURL string) spregistry.PDPProvider {
	n, _ := id.Uint64()
	return spregistry.PDPProvider{
		Info: spregistry.ProviderInfo{
			ID:              id,
			ServiceProvider: common.HexToAddress(fmt.Sprintf("0x%040x", n)),
			Payee:           common.HexToAddress(fmt.Sprintf("0x%040x", n+100)),
			IsActive:        true,
		},
		Offering: spregistry.PDPOffering{ServiceURL: serviceURL},
	}
}

func ptrPDPProvider(provider spregistry.PDPProvider) *spregistry.PDPProvider {
	cp := provider
	return &cp
}

func containsExcludedProvider(values []types.BigInt, target types.BigInt) bool {
	for _, value := range values {
		if value.Equal(target) {
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
	clientDataSetID := testID(0xABCD)
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0, ClientDataSetID: clientDataSetID},
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
	if got[0].clientDataSetID == nil || !got[0].clientDataSetID.Equal(clientDataSetID) {
		t.Fatalf("clientDataSetID=%v want %s (ClientDataSetID not threaded through resolver)", got[0].clientDataSetID, clientDataSetID.String())
	}
}

// TestServiceResolverResolveUploadContexts_ExplicitDataSetIDsRejectsInactive proves
// that the explicit DataSetIDs path enforces the active-dataset constraint
// (PDPEndEpoch == 0), matching the behaviour of autoSelect.
func TestServiceResolverResolveUploadContexts_ExplicitDataSetIDsRejectsInactive(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(55): {
				DataSetID:   testID(55),
				ProviderID:  testID(5),
				Payer:       testPayer(),
				PDPEndEpoch: 1000, // non-zero: inactive
			},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(5): ptrPDPProvider(testPDPProvider(testID(5), "https://sp-5.example.com")),
		},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(55)},
	})
	if err == nil {
		t.Fatal("expected error for inactive dataset, got nil")
	}
}

func TestMetadataMatches(t *testing.T) {
	tests := []struct {
		name      string
		ds        map[string]string
		requested map[string]string
		want      bool
	}{
		{"both empty", nil, nil, true},
		{"both empty maps", map[string]string{}, map[string]string{}, true},
		{"equal", map[string]string{"a": "1"}, map[string]string{"a": "1"}, true},
		{"different lengths", map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}, false},
		{"different values", map[string]string{"a": "1"}, map[string]string{"a": "2"}, false},
		{"dataset has extra", map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := metadataMatches(tt.ds, tt.requested); got != tt.want {
				t.Fatalf("metadataMatches()=%v want %v", got, tt.want)
			}
		})
	}
}

func TestWithCopies(t *testing.T) {
	// nil opts
	got := withCopies(nil, 3)
	if got == nil || got.Copies != 3 {
		t.Fatalf("nil opts: got=%+v want Copies=3", got)
	}

	// non-nil opts with all fields
	orig := &UploadOptions{
		Copies:             1,
		PieceMetadata:      map[string]string{"pk": "pv"},
		DataSetMetadata:    map[string]string{"dk": "dv"},
		ProviderIDs:        []types.BigInt{testID(1)},
		DataSetIDs:         []types.BigInt{testID(2)},
		ExcludeProviderIDs: []types.BigInt{testID(3)},
	}
	cloned := withCopies(orig, 5)
	if cloned.Copies != 5 {
		t.Fatalf("Copies=%d want 5", cloned.Copies)
	}
	// Original must be unmodified
	if orig.Copies != 1 {
		t.Fatal("original was mutated")
	}
	// Cloned maps must be independent
	cloned.PieceMetadata["pk"] = "changed"
	if orig.PieceMetadata["pk"] != "pv" {
		t.Fatal("PieceMetadata clone mutated original")
	}
	// Cloned slices must be independent
	cloned.ProviderIDs[0] = testID(99)
	if !orig.ProviderIDs[0].Equal(testID(1)) {
		t.Fatal("ProviderIDs clone mutated original")
	}
	cloned.DataSetIDs[0] = testID(99)
	if !orig.DataSetIDs[0].Equal(testID(2)) {
		t.Fatal("DataSetIDs clone mutated original")
	}
	cloned.ExcludeProviderIDs[0] = testID(99)
	if !orig.ExcludeProviderIDs[0].Equal(testID(3)) {
		t.Fatal("ExcludeProviderIDs clone mutated original")
	}
}

func TestResolveByDataSetIDs_SameProviderError(t *testing.T) {
	// Two datasets resolving to the same provider — should yield an error
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(10): {DataSetID: testID(10), ProviderID: testID(1), Payer: testPayer(), PDPEndEpoch: 0},
			testIDKey(20): {DataSetID: testID(20), ProviderID: testID(1), Payer: testPayer(), PDPEndEpoch: 0},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(1): ptrPDPProvider(testPDPProvider(testID(1), "https://sp-1.example.com")),
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(10): {},
			testIDKey(20): {},
		},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(10), testID(20)},
	})
	if err == nil {
		t.Fatal("expected duplicate provider error")
	}
}

func TestResolveByDataSetIDs_NilDataSetError(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(999)},
	})
	if err == nil {
		t.Fatal("expected error for nonexistent dataset")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected %q translation of warmstorage.ErrNotFound, got %v", "does not exist", err)
	}
}

func TestResolveByDataSetIDs_NilProviderError(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(10): {DataSetID: testID(10), ProviderID: testID(99), Payer: testPayer(), PDPEndEpoch: 0},
		},
		providersByID: map[string]*spregistry.PDPProvider{},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(10)},
	})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected spregistry.ErrNotFound translation, got %v", err)
	}
}

func TestResolveByDataSetIDs_CountMismatchError(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(10): {DataSetID: testID(10), ProviderID: testID(1), Payer: testPayer(), PDPEndEpoch: 0},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(1): ptrPDPProvider(testPDPProvider(testID(1), "https://sp-1.example.com")),
		},
	})

	// Copies=3 but only 1 dataset ID
	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(10)},
		Copies:     3,
	})
	if err == nil {
		t.Fatal("expected count mismatch error")
	}
}

func TestSelectReplacement_ErrorFromAutoSelect(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
	})

	// Exclude all providers → should fail
	_, err := resolver.SelectReplacement(context.Background(), map[string]types.BigInt{
		testIDKey(1): testID(1),
	}, &UploadOptions{})
	if err == nil {
		t.Fatal("expected error when all providers excluded")
	}
}

func TestNewServiceResolver_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		opts    ServiceResolverOptions
		wantErr string
	}{
		{
			name:    "zero payer",
			opts:    ServiceResolverOptions{},
			wantErr: "zero payer",
		},
		{
			name: "nil SPRegistry",
			opts: ServiceResolverOptions{
				Payer: testPayer(),
			},
			wantErr: "nil SPRegistry",
		},
		{
			name: "nil WarmStorage",
			opts: ServiceResolverOptions{
				Payer:      testPayer(),
				SPRegistry: &fakePDPProviderSource{},
			},
			wantErr: "nil WarmStorage",
		},
		{
			name: "nil NewContext",
			opts: ServiceResolverOptions{
				Payer:       testPayer(),
				SPRegistry:  &fakePDPProviderSource{},
				WarmStorage: &fakeDataSetCatalog{},
			},
			wantErr: "nil NewContext",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServiceResolver(tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err=%q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestResolveByProviderIDs_CountMismatchError(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(1): ptrPDPProvider(testPDPProvider(testID(1), "https://sp-1.example.com")),
		},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs: []types.BigInt{testID(1)},
		Copies:      3,
	})
	if err == nil {
		t.Fatal("expected count mismatch error")
	}
}

func TestResolveByProviderIDs_ProviderNotFound(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs: []types.BigInt{testID(999)},
	})
	if err == nil {
		t.Fatal("expected error for missing provider")
	}
}

func TestDedupeIDs(t *testing.T) {
	tests := []struct {
		name string
		in   []types.BigInt
		want int
	}{
		{"empty", nil, 0},
		{"duplicates", []types.BigInt{testID(1), testID(1), testID(2)}, 2},
		{"all unique", []types.BigInt{testID(1), testID(2), testID(3)}, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeIDs(tt.in)
			if len(got) != tt.want {
				t.Fatalf("dedupeIDs len=%d want %d", len(got), tt.want)
			}
		})
	}
}

func TestServiceResolverResolveUploadContexts_ExplicitProviderIDsTraversesPagedDataSets(t *testing.T) {
	const pageBoundary = 100

	clientDataSets := make([]*warmstorage.DataSetInfo, 0, pageBoundary+1)
	for i := 1; i <= pageBoundary; i++ {
		clientDataSets = append(clientDataSets, &warmstorage.DataSetInfo{
			DataSetID:   types.NewBigInt(uint64(i)),
			ProviderID:  types.NewBigInt(uint64(i)),
			PDPEndEpoch: 0,
		})
	}
	clientDataSets = append(clientDataSets, &warmstorage.DataSetInfo{
		DataSetID:   testID(1001),
		ProviderID:  testID(999),
		PDPEndEpoch: 0,
	})

	resolver := newTestServiceResolver(t, serviceResolverFixture{
		clientDataSets: clientDataSets,
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(999): ptrPDPProvider(testPDPProvider(testID(999), "https://sp-999.example.com")),
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(1001): {"source": "paged"},
		},
		requirePositiveListLimit: true,
	})

	contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs:     []types.BigInt{testID(999)},
		DataSetMetadata: map[string]string{"source": "paged"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if !explicit {
		t.Fatal("explicit=false want true")
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 {
		t.Fatalf("contexts len=%d want 1", len(got))
	}
	if got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(1001)) {
		t.Fatalf("dataSetID=%v want 1001", got[0].dataSetID)
	}
}

func TestServiceResolverResolveUploadContexts_AutoSelectTraversesPagedApprovedProviders(t *testing.T) {
	const pageBoundary = 100

	approved := make([]types.BigInt, 0, pageBoundary+1)
	for i := 1; i <= pageBoundary; i++ {
		approved = append(approved, types.NewBigInt(uint64(i)))
	}
	approved = append(approved, testID(999))

	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: approved,
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(999), "https://sp-999.example.com"),
		},
		requirePositiveListLimit: true,
	})

	contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if explicit {
		t.Fatal("explicit=true want false")
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 {
		t.Fatalf("contexts len=%d want 1", len(got))
	}
	if !got[0].id.Equal(testID(999)) {
		t.Fatalf("provider=%d want 999", got[0].id)
	}
}
