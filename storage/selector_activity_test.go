package storage

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type activityDataSetCatalog struct {
	*fakeDataSetCatalog

	mu              sync.Mutex
	activeByID      map[string]bool
	activityErrByID map[string]error
	metadataHook    func(context.Context, types.BigInt) (map[string]string, error)
	activityHook    func(context.Context, types.BigInt) (bool, error)
	metadataCalls   []types.BigInt
	activityCalls   []types.BigInt
}

type addressedActivityDataSetCatalog struct {
	*activityDataSetCatalog
	pdpVerifier common.Address
}

func (c *addressedActivityDataSetCatalog) PDPVerifierAddress() common.Address {
	return c.pdpVerifier
}

func (c *activityDataSetCatalog) GetAllDataSetMetadata(ctx context.Context, dataSetID types.BigInt) (map[string]string, error) {
	c.mu.Lock()
	c.metadataCalls = append(c.metadataCalls, dataSetID.Copy())
	hook := c.metadataHook
	c.mu.Unlock()
	if hook != nil {
		return hook(ctx, dataSetID)
	}
	return c.fakeDataSetCatalog.GetAllDataSetMetadata(ctx, dataSetID)
}

func (c *activityDataSetCatalog) HasActivePieces(ctx context.Context, dataSetID types.BigInt) (bool, error) {
	c.mu.Lock()
	c.activityCalls = append(c.activityCalls, dataSetID.Copy())
	hook := c.activityHook
	active := c.activeByID[idconv.Key(dataSetID)]
	err := c.activityErrByID[idconv.Key(dataSetID)]
	c.mu.Unlock()
	if hook != nil {
		return hook(ctx, dataSetID)
	}
	return active, err
}

func (c *activityDataSetCatalog) metadataCallIDs() []types.BigInt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]types.BigInt(nil), c.metadataCalls...)
}

func (c *activityDataSetCatalog) activityCallIDs() []types.BigInt {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]types.BigInt(nil), c.activityCalls...)
}

func newActivityDataSetCatalog(fixture serviceResolverFixture) *activityDataSetCatalog {
	return &activityDataSetCatalog{
		fakeDataSetCatalog: &fakeDataSetCatalog{fixture: fixture},
		activeByID:         make(map[string]bool),
		activityErrByID:    make(map[string]error),
	}
}

func newActivityServiceResolver(t *testing.T, catalog *activityDataSetCatalog, validator DataSetValidator) *ServiceResolver {
	t.Helper()
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:            testPayer(),
		SPRegistry:       &fakePDPProviderSource{fixture: catalog.fixture},
		WarmStorage:      catalog,
		DataSetValidator: validator,
		ProviderPing:     healthyProviderPing,
		NewContext: func(selection ResolvedUploadContext, _ *UploadOptions) (*Context, error) {
			return newResolvedTestContext(selection)
		},
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}
	if resolver.dataSetActivity != catalog {
		t.Fatal("NewServiceResolver did not auto-detect active-piece reads")
	}
	return resolver
}

func activitySelectionFixture(providerID types.BigInt, dataSetIDs []types.BigInt) serviceResolverFixture {
	fixture := serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			idconv.Key(providerID): ptrPDPProvider(testPDPProvider(providerID, "https://sp.example.com")),
		},
		clientDataSets:  make([]*warmstorage.DataSetInfo, 0, len(dataSetIDs)),
		dataSetMetadata: make(map[string]map[string]string, len(dataSetIDs)),
	}
	for _, dataSetID := range dataSetIDs {
		fixture.clientDataSets = append(fixture.clientDataSets, &warmstorage.DataSetInfo{
			DataSetID:  dataSetID,
			ProviderID: providerID,
		})
		fixture.dataSetMetadata[idconv.Key(dataSetID)] = map[string]string{"source": "app"}
	}
	return fixture
}

func resolveActivityProvider(t *testing.T, resolver *ServiceResolver, providerID types.BigInt, requireWritable bool) ([]*Context, error) {
	t.Helper()
	opts := &UploadOptions{
		ProviderIDs:     []types.BigInt{providerID},
		DataSetMetadata: map[string]string{"source": "app"},
	}
	var (
		contexts []UploadContext
		err      error
	)
	if requireWritable {
		contexts, _, err = resolver.resolveWritableUploadContexts(context.Background(), opts)
	} else {
		contexts, _, err = resolver.ResolveUploadContexts(context.Background(), opts)
	}
	if err != nil {
		return nil, err
	}
	return contextsToFake(t, contexts), nil
}

func requireSelectedDataSet(t *testing.T, contexts []*Context, want types.BigInt) {
	t.Helper()
	if len(contexts) != 1 || contexts[0].dataSetID == nil || !contexts[0].dataSetID.Equal(want) {
		t.Fatalf("contexts=%+v, want dataSetID %s", contexts, want.String())
	}
}

func TestServiceResolverProviderIDActivitySelection(t *testing.T) {
	providerID := testID(7)
	tests := []struct {
		name       string
		dataSetIDs []types.BigInt
		activeIDs  []types.BigInt
		want       types.BigInt
	}{
		{
			name:       "prefers non-empty match",
			dataSetIDs: []types.BigInt{testID(1), testID(2)},
			activeIDs:  []types.BigInt{testID(2)},
			want:       testID(2),
		},
		{
			name:       "selects oldest of several non-empty matches",
			dataSetIDs: []types.BigInt{testID(3), testID(1), testID(2)},
			activeIDs:  []types.BigInt{testID(3), testID(2)},
			want:       testID(2),
		},
		{
			name:       "falls back to oldest empty match",
			dataSetIDs: []types.BigInt{testID(3), testID(1), testID(2)},
			want:       testID(1),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			catalog := newActivityDataSetCatalog(activitySelectionFixture(providerID, tt.dataSetIDs))
			for _, dataSetID := range tt.activeIDs {
				catalog.activeByID[idconv.Key(dataSetID)] = true
			}
			resolver := newActivityServiceResolver(t, catalog, nil)

			contexts, err := resolveActivityProvider(t, resolver, providerID, false)
			if err != nil {
				t.Fatalf("ResolveUploadContexts: %v", err)
			}
			requireSelectedDataSet(t, contexts, tt.want)
		})
	}
}

func TestServiceResolverProviderIDActivitySelectionFallsBackWithoutPDPVerifier(t *testing.T) {
	providerID := testID(7)
	activityCatalog := newActivityDataSetCatalog(activitySelectionFixture(
		providerID,
		[]types.BigInt{testID(2), testID(1)},
	))
	activityCatalog.activeByID[testIDKey(2)] = true
	catalog := &addressedActivityDataSetCatalog{activityDataSetCatalog: activityCatalog}

	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:        testPayer(),
		SPRegistry:   &fakePDPProviderSource{fixture: catalog.fixture},
		WarmStorage:  catalog,
		ProviderPing: healthyProviderPing,
		NewContext: func(selection ResolvedUploadContext, _ *UploadOptions) (*Context, error) {
			return newResolvedTestContext(selection)
		},
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}
	if resolver.dataSetActivity != nil {
		t.Fatal("NewServiceResolver enabled active-piece reads without a configured PDPVerifier")
	}

	contexts, err := resolveActivityProvider(t, resolver, providerID, false)
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	requireSelectedDataSet(t, contexts, testID(1))
	if calls := activityCatalog.activityCallIDs(); len(calls) != 0 {
		t.Fatalf("HasActivePieces calls=%v without a configured PDPVerifier, want none", calls)
	}
	metadataCalls := activityCatalog.metadataCallIDs()
	if len(metadataCalls) != 1 || !metadataCalls[0].Equal(testID(1)) {
		t.Fatalf("metadata calls=%v, want only oldest matching dataSetID 1", metadataCalls)
	}
}

func TestServiceResolverProviderIDActivitySelectionSlidesPastFirstWindow(t *testing.T) {
	providerID := testID(7)
	dataSetIDs := make([]types.BigInt, 30)
	for i := range dataSetIDs {
		dataSetIDs[i] = testID(uint64(i + 1))
	}
	fixture := activitySelectionFixture(providerID, dataSetIDs)
	catalog := newActivityDataSetCatalog(fixture)
	catalog.activeByID[testIDKey(25)] = true
	resolver := newActivityServiceResolver(t, catalog, nil)

	contexts, err := resolveActivityProvider(t, resolver, providerID, false)
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	requireSelectedDataSet(t, contexts, testID(25))
}

func TestServiceResolverProviderIDActivitySelectionSkipsMismatchedMetadata(t *testing.T) {
	providerID := testID(7)
	fixture := activitySelectionFixture(providerID, []types.BigInt{testID(1), testID(2)})
	fixture.dataSetMetadata[testIDKey(1)] = map[string]string{"source": "other"}
	catalog := newActivityDataSetCatalog(fixture)
	catalog.activeByID[testIDKey(1)] = true
	catalog.activeByID[testIDKey(2)] = true
	resolver := newActivityServiceResolver(t, catalog, nil)

	contexts, err := resolveActivityProvider(t, resolver, providerID, false)
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	requireSelectedDataSet(t, contexts, testID(2))
	calls := catalog.activityCallIDs()
	if len(calls) != 1 || !calls[0].Equal(testID(2)) {
		t.Fatalf("HasActivePieces calls=%v, want only dataSetID 2", calls)
	}
}

func TestServiceResolverProviderIDActivitySelectionCreatesNewWhenMetadataDoesNotMatch(t *testing.T) {
	providerID := testID(7)
	fixture := activitySelectionFixture(providerID, []types.BigInt{testID(1), testID(2)})
	fixture.dataSetMetadata[testIDKey(1)] = map[string]string{"source": "other"}
	fixture.dataSetMetadata[testIDKey(2)] = map[string]string{"source": "other"}
	catalog := newActivityDataSetCatalog(fixture)
	resolver := newActivityServiceResolver(t, catalog, nil)

	contexts, err := resolveActivityProvider(t, resolver, providerID, false)
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if len(contexts) != 1 || contexts[0].dataSetID != nil {
		t.Fatalf("contexts=%+v, want a new data set", contexts)
	}
	if got := contexts[0].dataSetMetadata; len(got) != 1 || got["source"] != "app" {
		t.Fatalf("new data set metadata=%v, want source=app", got)
	}
	if calls := catalog.activityCallIDs(); len(calls) != 0 {
		t.Fatalf("HasActivePieces calls=%v for mismatched metadata, want none", calls)
	}
}

func TestServiceResolverProviderIDActivitySelectionFiltersCandidates(t *testing.T) {
	providerID := testID(7)
	fixture := activitySelectionFixture(providerID, nil)
	fixture.dataSetMetadata = map[string]map[string]string{
		testIDKey(3): {"source": "app"},
	}
	catalog := newActivityDataSetCatalog(fixture)
	catalog.activeByID[testIDKey(3)] = true
	resolver := newActivityServiceResolver(t, catalog, nil)

	dataSetID, _, _, err := resolver.selectMatchingDataSetWithWritable(
		context.Background(),
		providerID,
		[]*warmstorage.DataSetInfo{
			nil,
			{DataSetID: types.BigInt{}, ProviderID: providerID},
			{DataSetID: testID(1), ProviderID: testID(8)},
			{DataSetID: testID(2), ProviderID: providerID, PDPEndEpoch: 1},
			{DataSetID: testID(3), ProviderID: providerID},
		},
		map[string]string{"source": "app"},
		false,
	)
	if err != nil {
		t.Fatalf("selectMatchingDataSetWithWritable: %v", err)
	}
	if dataSetID == nil || !dataSetID.Equal(testID(3)) {
		t.Fatalf("dataSetID=%v, want 3", dataSetID)
	}
	metadataCalls := catalog.metadataCallIDs()
	activityCalls := catalog.activityCallIDs()
	if len(metadataCalls) != 1 || !metadataCalls[0].Equal(testID(3)) {
		t.Fatalf("metadata calls=%v, want only dataSetID 3", metadataCalls)
	}
	if len(activityCalls) != 1 || !activityCalls[0].Equal(testID(3)) {
		t.Fatalf("HasActivePieces calls=%v, want only dataSetID 3", activityCalls)
	}
}

func TestServiceResolverProviderIDActivitySelectionSortsFullBigInts(t *testing.T) {
	providerID := testID(7)
	older, err := types.BigIntFromBig(new(big.Int).Lsh(big.NewInt(1), 200))
	if err != nil {
		t.Fatalf("BigIntFromBig older: %v", err)
	}
	newer, err := types.BigIntFromBig(new(big.Int).Add(older.Big(), big.NewInt(9)))
	if err != nil {
		t.Fatalf("BigIntFromBig newer: %v", err)
	}
	catalog := newActivityDataSetCatalog(activitySelectionFixture(providerID, []types.BigInt{newer, older}))
	catalog.activeByID[idconv.Key(older)] = true
	catalog.activeByID[idconv.Key(newer)] = true
	resolver := newActivityServiceResolver(t, catalog, nil)

	contexts, err := resolveActivityProvider(t, resolver, providerID, false)
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	requireSelectedDataSet(t, contexts, older)
}

func TestServiceResolverProviderIDActivitySelectionSkipsNonWritableCandidates(t *testing.T) {
	providerID := testID(7)
	fixture := activitySelectionFixture(providerID, []types.BigInt{testID(1), testID(2)})
	fixture.validatorEnabled = true
	fixture.validatorErrByID = map[string]error{testIDKey(1): errors.New("not writable")}
	catalog := newActivityDataSetCatalog(fixture)
	catalog.activeByID[testIDKey(1)] = true
	catalog.activeByID[testIDKey(2)] = true
	validator := &fakeEnhancedDataSetCatalog{fakeDataSetCatalog: fakeDataSetCatalog{fixture: fixture}}
	resolver := newActivityServiceResolver(t, catalog, validator)

	contexts, err := resolveActivityProvider(t, resolver, providerID, true)
	if err != nil {
		t.Fatalf("resolveWritableUploadContexts: %v", err)
	}
	requireSelectedDataSet(t, contexts, testID(2))
	calls := catalog.activityCallIDs()
	if len(calls) != 1 || !calls[0].Equal(testID(2)) {
		t.Fatalf("HasActivePieces calls=%v, want only writable dataSetID 2", calls)
	}
}

type activityResolveOutcome struct {
	contexts []UploadContext
	err      error
}

func resolveActivityProviderAsync(ctx context.Context, resolver *ServiceResolver, providerID types.BigInt) <-chan activityResolveOutcome {
	done := make(chan activityResolveOutcome, 1)
	go func() {
		contexts, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{
			ProviderIDs:     []types.BigInt{providerID},
			DataSetMetadata: map[string]string{"source": "app"},
		})
		done <- activityResolveOutcome{contexts: contexts, err: err}
	}()
	return done
}

func awaitActivitySignal(t *testing.T, ch <-chan struct{}, label string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-ch:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", label)
	}
}

func awaitActivityOutcome(t *testing.T, ch <-chan activityResolveOutcome) activityResolveOutcome {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case outcome := <-ch:
		return outcome
	case <-timer.C:
		t.Fatal("timed out waiting for ResolveUploadContexts")
		return activityResolveOutcome{}
	}
}

func beginTrackedResolveCall(current, maximum *atomic.Int32) func() {
	value := current.Add(1)
	for {
		previous := maximum.Load()
		if value <= previous || maximum.CompareAndSwap(previous, value) {
			break
		}
	}
	return func() {
		current.Add(-1)
	}
}

func TestServiceResolverProviderIDActivitySelectionBoundsConcurrency(t *testing.T) {
	providerID := testID(7)
	dataSetIDs := make([]types.BigInt, 25)
	for i := range dataSetIDs {
		dataSetIDs[i] = testID(uint64(i + 1))
	}
	catalog := newActivityDataSetCatalog(activitySelectionFixture(providerID, dataSetIDs))
	started := make(chan struct{}, len(dataSetIDs))
	release := make(chan struct{})
	var current, maximum atomic.Int32
	catalog.metadataHook = func(ctx context.Context, _ types.BigInt) (map[string]string, error) {
		defer beginTrackedResolveCall(&current, &maximum)()
		started <- struct{}{}
		select {
		case <-release:
			return map[string]string{"source": "app"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	catalog.activityHook = func(context.Context, types.BigInt) (bool, error) {
		defer beginTrackedResolveCall(&current, &maximum)()
		return false, nil
	}
	resolver := newActivityServiceResolver(t, catalog, nil)
	done := resolveActivityProviderAsync(context.Background(), resolver, providerID)

	for range resolveConcurrency {
		awaitActivitySignal(t, started, "initial resolve window")
	}
	select {
	case <-started:
		t.Fatalf("more than %d resolve calls started before a slot was released", resolveConcurrency)
	default:
	}
	if got := maximum.Load(); got != resolveConcurrency {
		t.Fatalf("maximum concurrency=%d, want %d", got, resolveConcurrency)
	}
	close(release)

	outcome := awaitActivityOutcome(t, done)
	if outcome.err != nil {
		t.Fatalf("ResolveUploadContexts: %v", outcome.err)
	}
	requireSelectedDataSet(t, contextsToFake(t, outcome.contexts), testID(1))
	if got := maximum.Load(); got > resolveConcurrency {
		t.Fatalf("maximum concurrency=%d, want <=%d", got, resolveConcurrency)
	}
	if got := current.Load(); got != 0 {
		t.Fatalf("in-flight resolve calls=%d after completion, want 0", got)
	}
}

func TestServiceResolverProviderIDActivitySelectionIgnoresCompletionOrder(t *testing.T) {
	providerID := testID(7)
	catalog := newActivityDataSetCatalog(activitySelectionFixture(providerID, []types.BigInt{testID(1), testID(2)}))
	oldestStarted := make(chan struct{})
	releaseOldest := make(chan struct{})
	newerRecorded := make(chan struct{})
	var order atomic.Int32
	var oldestOrder, newerOrder atomic.Int32
	catalog.activityHook = func(ctx context.Context, dataSetID types.BigInt) (bool, error) {
		switch {
		case dataSetID.Equal(testID(1)):
			close(oldestStarted)
			select {
			case <-releaseOldest:
				oldestOrder.Store(order.Add(1))
				return true, nil
			case <-ctx.Done():
				return false, ctx.Err()
			}
		case dataSetID.Equal(testID(2)):
			select {
			case <-oldestStarted:
			case <-ctx.Done():
				return false, ctx.Err()
			}
			newerOrder.Store(order.Add(1))
			close(newerRecorded)
			return true, nil
		default:
			return false, nil
		}
	}
	resolver := newActivityServiceResolver(t, catalog, nil)
	done := resolveActivityProviderAsync(context.Background(), resolver, providerID)

	awaitActivitySignal(t, oldestStarted, "oldest activity read")
	awaitActivitySignal(t, newerRecorded, "newer activity completion")
	close(releaseOldest)
	outcome := awaitActivityOutcome(t, done)
	if outcome.err != nil {
		t.Fatalf("ResolveUploadContexts: %v", outcome.err)
	}
	requireSelectedDataSet(t, contextsToFake(t, outcome.contexts), testID(1))
	if newerOrder.Load() != 1 || oldestOrder.Load() != 2 {
		t.Fatalf("completion order newer=%d oldest=%d, want newer=1 oldest=2", newerOrder.Load(), oldestOrder.Load())
	}
}

func TestRunResolveWindowStopsStartingAfterOldestNonEmptyMatch(t *testing.T) {
	const dataSetCount = 30
	releaseLater := make(chan struct{})
	oldestSelected := make(chan struct{})
	done := make(chan error, 1)
	bestNonEmpty := dataSetCount
	var stateMu sync.Mutex
	var calls atomic.Int32

	go func() {
		done <- runResolveWindow(
			context.Background(),
			dataSetCount,
			func(index int) bool {
				stateMu.Lock()
				defer stateMu.Unlock()
				return index <= bestNonEmpty
			},
			func(_ context.Context, index int) error {
				calls.Add(1)
				if index == 0 {
					stateMu.Lock()
					bestNonEmpty = index
					stateMu.Unlock()
					close(oldestSelected)
					return nil
				}
				<-releaseLater
				return nil
			},
		)
	}()

	awaitActivitySignal(t, oldestSelected, "oldest non-empty match")
	close(releaseLater)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runResolveWindow: %v", err)
		}
	case <-timer.C:
		t.Fatal("timed out waiting for resolve window")
	}
	if got := calls.Load(); got > resolveConcurrency {
		t.Fatalf("evaluated %d data sets after oldest non-empty match, want <=%d", got, resolveConcurrency)
	}
}

func TestServiceResolverProviderIDActivitySelectionCancelsOnError(t *testing.T) {
	providerID := testID(7)
	dataSetIDs := make([]types.BigInt, resolveConcurrency)
	for i := range dataSetIDs {
		dataSetIDs[i] = testID(uint64(i + 1))
	}
	catalog := newActivityDataSetCatalog(activitySelectionFixture(providerID, dataSetIDs))
	errorReady := make(chan struct{})
	releaseError := make(chan struct{})
	otherStarted := make(chan struct{}, resolveConcurrency-1)
	otherCanceled := make(chan struct{}, resolveConcurrency-1)
	want := errors.New("activity unavailable")
	catalog.activityHook = func(ctx context.Context, dataSetID types.BigInt) (bool, error) {
		if dataSetID.Equal(testID(1)) {
			close(errorReady)
			select {
			case <-releaseError:
				return false, want
			case <-ctx.Done():
				return false, ctx.Err()
			}
		}
		otherStarted <- struct{}{}
		<-ctx.Done()
		otherCanceled <- struct{}{}
		return false, ctx.Err()
	}
	resolver := newActivityServiceResolver(t, catalog, nil)
	done := resolveActivityProviderAsync(context.Background(), resolver, providerID)

	awaitActivitySignal(t, errorReady, "failing activity read")
	for range resolveConcurrency - 1 {
		awaitActivitySignal(t, otherStarted, "other activity read")
	}
	close(releaseError)
	outcome := awaitActivityOutcome(t, done)
	if !errors.Is(outcome.err, want) {
		t.Fatalf("ResolveUploadContexts error=%v, want wrapped %v", outcome.err, want)
	}
	for range resolveConcurrency - 1 {
		awaitActivitySignal(t, otherCanceled, "canceled activity read")
	}
}

func TestServiceResolverProviderIDActivitySelectionHonorsCallerCancellation(t *testing.T) {
	providerID := testID(7)
	dataSetIDs := make([]types.BigInt, resolveConcurrency)
	for i := range dataSetIDs {
		dataSetIDs[i] = testID(uint64(i + 1))
	}
	catalog := newActivityDataSetCatalog(activitySelectionFixture(providerID, dataSetIDs))
	metadataStarted := make(chan struct{}, resolveConcurrency)
	catalog.metadataHook = func(ctx context.Context, _ types.BigInt) (map[string]string, error) {
		metadataStarted <- struct{}{}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	resolver := newActivityServiceResolver(t, catalog, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := resolveActivityProviderAsync(ctx, resolver, providerID)

	for range resolveConcurrency {
		awaitActivitySignal(t, metadataStarted, "metadata read")
	}
	cancel()
	outcome := awaitActivityOutcome(t, done)
	if !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("ResolveUploadContexts error=%v, want context.Canceled", outcome.err)
	}
	if calls := catalog.activityCallIDs(); len(calls) != 0 {
		t.Fatalf("HasActivePieces calls=%v after metadata cancellation, want none", calls)
	}
}
