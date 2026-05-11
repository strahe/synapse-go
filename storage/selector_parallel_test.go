package storage

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// slowDataSetCatalog wraps fakeDataSetCatalog with a per-call delay and a
// concurrency counter so the selector's bounded-parallel metadata fetch can
// be verified end-to-end.
type slowDataSetCatalog struct {
	*fakeDataSetCatalog
	delay       time.Duration
	inflight    atomic.Int32
	maxInflight atomic.Int32
}

func (s *slowDataSetCatalog) GetAllDataSetMetadata(ctx context.Context, dataSetID types.BigInt) (map[string]string, error) {
	cur := s.inflight.Add(1)
	defer s.inflight.Add(-1)
	for {
		prev := s.maxInflight.Load()
		if cur <= prev || s.maxInflight.CompareAndSwap(prev, cur) {
			break
		}
	}
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	return s.fakeDataSetCatalog.GetAllDataSetMetadata(ctx, dataSetID)
}

type mixedOutcomeDataSetCatalog struct {
	*fakeDataSetCatalog
	matchingID      types.BigInt
	matchingDelay   time.Duration
	matchingStarted chan struct{}
	failID          types.BigInt
	failErr         error
}

func (m *mixedOutcomeDataSetCatalog) GetAllDataSetMetadata(ctx context.Context, dataSetID types.BigInt) (map[string]string, error) {
	switch {
	case dataSetID.Equal(m.matchingID):
		close(m.matchingStarted)
		select {
		case <-time.After(m.matchingDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return m.fakeDataSetCatalog.GetAllDataSetMetadata(ctx, dataSetID)
	case dataSetID.Equal(m.failID):
		<-m.matchingStarted
		return nil, m.failErr
	default:
		return m.fakeDataSetCatalog.GetAllDataSetMetadata(ctx, dataSetID)
	}
}

// TestServiceResolver_MetadataFetchIsConcurrent asserts that
// selectMatchingDataSet fans out GetAllDataSetMetadata calls in parallel
// (bounded by selectorMetadataConcurrency) rather than running them
// serially, which used to be an N+1 bottleneck.
func TestServiceResolver_MetadataFetchIsConcurrent(t *testing.T) {
	providerID := types.NewBigInt(1)
	const candidates = 6
	clientDataSets := make([]*warmstorage.DataSetInfo, 0, candidates)
	metadata := make(map[string]map[string]string, candidates)
	for i := 1; i <= candidates; i++ {
		dsID := types.NewBigInt(uint64(i))
		clientDataSets = append(clientDataSets, &warmstorage.DataSetInfo{
			DataSetID:       dsID,
			ProviderID:      providerID,
			ClientDataSetID: types.NewBigInt(uint64(i)),
		})
		metadata[idconv.Key(dsID)] = map[string]string{"other": "x"}
	}
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{providerID},
		activeProviders:     []spregistry.PDPProvider{testPDPProvider(providerID, "https://sp1.example.com")},
		clientDataSets:      clientDataSets,
		dataSetMetadata:     metadata,
	}
	slow := &slowDataSetCatalog{
		fakeDataSetCatalog: &fakeDataSetCatalog{fixture: fixture},
		delay:              40 * time.Millisecond,
	}
	resolver := &ServiceResolver{
		payer:       common.HexToAddress("0xabc"),
		spRegistry:  &fakePDPProviderSource{fixture: fixture},
		warmStorage: slow,
		newContext:  func(_ ResolvedUploadContext, _ *UploadOptions) (*Context, error) { return nil, nil },
	}

	start := time.Now()
	_, _, _, err := resolver.selectMatchingDataSet(
		context.Background(),
		providerID,
		slow.fixture.clientDataSets,
		map[string]string{"requested": "y"},
	)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("selectMatchingDataSet: %v", err)
	}
	if got := slow.maxInflight.Load(); got < 2 {
		t.Fatalf("maxInflight=%d want >=2 (parallelism expected)", got)
	}
	// Serial would be ~candidates*delay = 240ms. Parallel with concurrency
	// >=6 should complete within a couple of delay slots plus jitter.
	if elapsed > 180*time.Millisecond {
		t.Fatalf("elapsed=%s want <180ms with parallel fetch (serial would be ~%s)",
			elapsed, time.Duration(candidates)*slow.delay)
	}
}

// TestServiceResolver_MetadataFetchUsesCallerContextBudget verifies that
// selectMatchingDataSet does not impose its own shorter deadline on metadata
// RPCs; the caller's ctx should remain the sole time budget.
func TestServiceResolver_MetadataFetchUsesCallerContextBudget(t *testing.T) {
	providerID := types.NewBigInt(1)
	const metadataDelay = 6 * time.Second
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{providerID},
		activeProviders:     []spregistry.PDPProvider{testPDPProvider(providerID, "https://sp1.example.com")},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: types.NewBigInt(1), ProviderID: providerID, ClientDataSetID: types.NewBigInt(1)},
		},
		dataSetMetadata: map[string]map[string]string{
			idconv.Key(types.NewBigInt(1)): {"source": "app"},
		},
	}
	slow := &slowDataSetCatalog{
		fakeDataSetCatalog: &fakeDataSetCatalog{fixture: fixture},
		delay:              metadataDelay,
	}
	resolver := &ServiceResolver{
		payer:       common.HexToAddress("0xabc"),
		spRegistry:  &fakePDPProviderSource{fixture: fixture},
		warmStorage: slow,
		newContext:  func(_ ResolvedUploadContext, _ *UploadOptions) (*Context, error) { return nil, nil },
	}
	ctx, cancel := context.WithTimeout(context.Background(), metadataDelay+time.Second)
	defer cancel()
	dsID, clientID, metadata, err := resolver.selectMatchingDataSet(
		ctx,
		providerID,
		fixture.clientDataSets,
		map[string]string{"source": "app"},
	)
	if err != nil {
		t.Fatalf("selectMatchingDataSet: %v", err)
	}
	if dsID == nil || !dsID.Equal(types.NewBigInt(1)) {
		t.Fatalf("DataSetID = %v, want 1", dsID)
	}
	if clientID == nil || !clientID.Equal(types.NewBigInt(1)) {
		t.Fatalf("ClientDataSetID = %v, want 1", clientID)
	}
	if metadata["source"] != "app" {
		t.Fatalf("metadata = %#v, want reused matching metadata", metadata)
	}
}

// TestServiceResolver_MetadataFetchErrorRejectsReuse verifies that metadata
// lookup failures are not silently ignored when selecting a reusable dataset.
func TestServiceResolver_MetadataFetchErrorRejectsReuse(t *testing.T) {
	providerID := types.NewBigInt(1)
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{providerID},
		activeProviders:     []spregistry.PDPProvider{testPDPProvider(providerID, "https://sp1.example.com")},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: types.NewBigInt(1), ProviderID: providerID, ClientDataSetID: types.NewBigInt(1)},
			{DataSetID: types.NewBigInt(2), ProviderID: providerID, ClientDataSetID: types.NewBigInt(2)},
		},
		dataSetMetadata: map[string]map[string]string{
			idconv.Key(types.NewBigInt(2)): {"source": "app", "env": "prod"},
		},
	}
	catalog := &mixedOutcomeDataSetCatalog{
		fakeDataSetCatalog: &fakeDataSetCatalog{fixture: fixture},
		matchingID:         types.NewBigInt(2),
		matchingDelay:      25 * time.Millisecond,
		matchingStarted:    make(chan struct{}),
		failID:             types.NewBigInt(1),
		failErr:            context.DeadlineExceeded,
	}
	resolver := &ServiceResolver{
		payer:       common.HexToAddress("0xabc"),
		spRegistry:  &fakePDPProviderSource{fixture: fixture},
		warmStorage: catalog,
		newContext:  func(_ ResolvedUploadContext, _ *UploadOptions) (*Context, error) { return nil, nil },
	}

	dsID, clientID, metadata, err := resolver.selectMatchingDataSet(
		context.Background(),
		providerID,
		fixture.clientDataSets,
		map[string]string{"source": "app", "env": "prod"},
	)
	if err == nil {
		t.Fatalf("selectMatchingDataSet returned dsID=%v clientID=%v metadata=%#v, want metadata fetch error", dsID, clientID, metadata)
	}
}
