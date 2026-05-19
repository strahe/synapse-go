package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type trackingDataSetCatalog struct {
	*fakeDataSetCatalog
	mu              sync.Mutex
	errByID         map[string]error
	calls           []types.BigInt
	metadataContext context.Context
}

func (t *trackingDataSetCatalog) GetAllDataSetMetadata(ctx context.Context, dataSetID types.BigInt) (map[string]string, error) {
	t.mu.Lock()
	t.calls = append(t.calls, dataSetID)
	t.metadataContext = ctx
	err := t.errByID[idconv.Key(dataSetID)]
	t.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return t.fakeDataSetCatalog.GetAllDataSetMetadata(ctx, dataSetID)
}

func (t *trackingDataSetCatalog) metadataCalls() []types.BigInt {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]types.BigInt, len(t.calls))
	copy(out, t.calls)
	return out
}

func (t *trackingDataSetCatalog) capturedMetadataContext() context.Context {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.metadataContext
}

func newTrackingMetadataResolver(t *testing.T, fixture serviceResolverFixture, catalog *trackingDataSetCatalog) *ServiceResolver {
	t.Helper()
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:       testPayer(),
		SPRegistry:  &fakePDPProviderSource{fixture: fixture},
		WarmStorage: catalog,
		NewContext: func(selection ResolvedUploadContext, _ *UploadOptions) (*Context, error) {
			return newResolvedTestContext(selection)
		},
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}
	return resolver
}

func TestServiceResolver_MetadataFetchStopsAtFirstMatch(t *testing.T) {
	providerID := testID(1)
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{providerID},
		activeProviders:     []spregistry.PDPProvider{testPDPProvider(providerID, "https://sp1.example.com")},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(1), ProviderID: providerID, ClientDataSetID: testID(1)},
			{DataSetID: testID(2), ProviderID: providerID, ClientDataSetID: testID(2)},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(1): {"source": "app"},
			testIDKey(2): {"source": "app"},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(1): ptrPDPProvider(testPDPProvider(providerID, "https://sp1.example.com")),
		},
	}
	catalog := &trackingDataSetCatalog{
		fakeDataSetCatalog: &fakeDataSetCatalog{fixture: fixture},
		errByID: map[string]error{
			testIDKey(2): errors.New("later metadata must not be fetched"),
		},
	}
	resolver := newTrackingMetadataResolver(t, fixture, catalog)

	contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs:     []types.BigInt{providerID},
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if !explicit {
		t.Fatal("explicit=false want true")
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(1)) {
		t.Fatalf("context=%+v want dataSetID 1", got)
	}
	calls := catalog.metadataCalls()
	if len(calls) != 1 || !calls[0].Equal(testID(1)) {
		t.Fatalf("metadata calls=%v want only dataSetID 1", calls)
	}
}

func TestServiceResolver_MetadataFetchUsesCallerContextBudget(t *testing.T) {
	providerID := testID(1)
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{providerID},
		activeProviders:     []spregistry.PDPProvider{testPDPProvider(providerID, "https://sp1.example.com")},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(1), ProviderID: providerID, ClientDataSetID: testID(1)},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(1): {"source": "app"},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(1): ptrPDPProvider(testPDPProvider(providerID, "https://sp1.example.com")),
		},
	}
	catalog := &trackingDataSetCatalog{
		fakeDataSetCatalog: &fakeDataSetCatalog{fixture: fixture},
	}
	resolver := newTrackingMetadataResolver(t, fixture, catalog)

	deadline := time.Now().Add(time.Minute)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	contexts, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{
		ProviderIDs:     []types.BigInt{providerID},
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(1)) {
		t.Fatalf("context=%+v want dataSetID 1", got)
	}
	metadataCtx := catalog.capturedMetadataContext()
	gotDeadline, ok := metadataCtx.Deadline()
	if !ok {
		t.Fatal("metadata context has no deadline")
	}
	if !gotDeadline.Equal(deadline) {
		t.Fatalf("metadata deadline=%s want %s", gotDeadline, deadline)
	}
}

func TestServiceResolver_MetadataFetchErrorRejectsReuse(t *testing.T) {
	providerID := testID(1)
	want := errors.New("metadata unavailable")
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{providerID},
		activeProviders:     []spregistry.PDPProvider{testPDPProvider(providerID, "https://sp1.example.com")},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(1), ProviderID: providerID, ClientDataSetID: testID(1)},
			{DataSetID: testID(2), ProviderID: providerID, ClientDataSetID: testID(2)},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(2): {"source": "app", "env": "prod"},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(1): ptrPDPProvider(testPDPProvider(providerID, "https://sp1.example.com")),
		},
	}
	catalog := &trackingDataSetCatalog{
		fakeDataSetCatalog: &fakeDataSetCatalog{fixture: fixture},
		errByID: map[string]error{
			testIDKey(1): want,
		},
	}
	resolver := newTrackingMetadataResolver(t, fixture, catalog)

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs:     []types.BigInt{providerID},
		DataSetMetadata: map[string]string{"source": "app", "env": "prod"},
	})
	if !errors.Is(err, want) {
		t.Fatalf("ResolveUploadContexts err=%v want %v", err, want)
	}
}
