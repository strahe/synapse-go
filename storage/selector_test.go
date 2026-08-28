package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
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

func testID(v uint64) types.BigInt {
	return types.NewBigInt(v)
}

func testIDKey(v uint64) string {
	return idconv.Key(testID(v))
}

func bigInt(v int64) *big.Int {
	return big.NewInt(v)
}

func healthyProviderPing(context.Context, string) error {
	return nil
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
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{"source": "app", "withCDN": ""},
			},
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(12), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: false,
				Metadata:        map[string]string{"source": "other"},
			},
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(21), ProviderID: testID(2), PDPEndEpoch: 7},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
			},
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
	if !got[0].ProviderID().Equal(testID(1)) || dataSetIDOf(got[0]) == nil || !dataSetIDOf(got[0]).Equal(testID(11)) {
		t.Fatalf("first context provider=%s dataset=%v want provider=1 dataset=11", got[0].ProviderID(), dataSetIDOf(got[0]))
	}
	if !got[1].ProviderID().Equal(testID(2)) {
		t.Fatalf("second context provider=%s want 2", got[1].ProviderID())
	}
	if dataSetIDOf(got[1]) != nil {
		t.Fatalf("second context dataset=%v want nil", dataSetIDOf(got[1]))
	}
	metadata := dataSetMetadataOf(t, got[1])
	if metadata["withCDN"] != "" || metadata["source"] != "app" {
		t.Fatalf("second context metadata=%v", metadata)
	}
}

func TestServiceResolverSelectProviderContext_ReturnsUnboundHealthyProvider(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
			testPDPProvider(testID(2), "https://sp-2.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(11), ProviderID: testID(1), ClientDataSetID: testID(101)},
		},
		providerPing: func(_ context.Context, serviceURL string) error {
			if serviceURL == "https://sp-1.example.com" {
				return errors.New("provider unavailable")
			}
			return nil
		},
	})

	got, err := resolver.SelectProviderContext(context.Background(), SelectProviderContextOptions{
		ExcludeProviderIDs: []types.BigInt{testID(1)},
		DataSetMetadata:    map[string]string{"app": "photos"},
	})
	if err != nil {
		t.Fatalf("SelectProviderContext: %v", err)
	}
	if got == nil || !got.ProviderID().Equal(testID(2)) {
		t.Fatalf("provider=%v want 2", got)
	}
	if _, bound := got.DataSetRef(); bound {
		t.Fatal("SelectProviderContext returned a bound context")
	}
}

func TestServiceResolverSelectReplacement_ExcludesUsedProviders(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1), testID(2), testID(3), testID(4)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
			testPDPProvider(testID(2), "https://sp-2.example.com"),
			testPDPProvider(testID(3), "https://sp-3.example.com"),
			testPDPProvider(testID(4), "https://sp-4.example.com"),
		},
		providerPing: func(_ context.Context, serviceURL string) error {
			if serviceURL == "https://sp-3.example.com" {
				return errors.New("provider unavailable")
			}
			return nil
		},
	})

	replacement, err := resolver.SelectReplacement(context.Background(), map[string]types.BigInt{
		testIDKey(1): testID(1),
		testIDKey(2): testID(2),
	}, &UploadOptions{})
	if err != nil {
		t.Fatalf("SelectReplacement: %v", err)
	}
	if !replacement.ProviderID().Equal(testID(4)) {
		t.Fatalf("replacement provider=%s want 4", replacement.ProviderID())
	}
}

func TestServiceResolverResolveUploadContexts_HealthChecksAutoSelectedProviders(t *testing.T) {
	t.Run("skips failed candidates and fetches selection inputs once", func(t *testing.T) {
		var approvedCalls, activeCalls, detailsCalls atomic.Int32
		var pingMu sync.Mutex
		pinged := make(map[string]int)
		var deadlineErrors []error
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2), testID(3)},
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(1), "https://sp-1-duplicate.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
				testPDPProvider(testID(3), "https://sp-3.example.com"),
			},
			detailedDataSets:      []*warmstorage.EnhancedDataSetInfo{},
			approvedProviderCalls: &approvedCalls,
			activeProviderCalls:   &activeCalls,
			dataSetDetailsCalls:   &detailsCalls,
			providerPing: func(ctx context.Context, serviceURL string) error {
				deadline, ok := ctx.Deadline()
				if !ok {
					pingMu.Lock()
					deadlineErrors = append(deadlineErrors, errors.New("ProviderPing context has no deadline"))
					pingMu.Unlock()
				} else if remaining := time.Until(deadline); remaining <= 0 || remaining > providerPingTimeout {
					pingMu.Lock()
					deadlineErrors = append(deadlineErrors, fmt.Errorf("ProviderPing deadline remaining=%s want within %s", remaining, providerPingTimeout))
					pingMu.Unlock()
				}
				pingMu.Lock()
				pinged[serviceURL]++
				pingMu.Unlock()
				if serviceURL == "https://sp-1.example.com" {
					return context.DeadlineExceeded
				}
				return nil
			},
		})

		requireEndorsed := false
		contexts, explicit, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
			Copies:                 2,
			RequireEndorsedPrimary: &requireEndorsed,
		})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		if explicit {
			t.Fatal("explicit=true want false")
		}
		got := contextsToFake(t, contexts)
		if len(got) != 2 || !got[0].ProviderID().Equal(testID(2)) || !got[1].ProviderID().Equal(testID(3)) {
			t.Fatalf("providers=%v want [2 3]", providerIDs(got))
		}
		pingMu.Lock()
		defer pingMu.Unlock()
		if len(deadlineErrors) != 0 {
			t.Fatalf("ProviderPing deadline errors: %v", deadlineErrors)
		}
		for _, serviceURL := range []string{"https://sp-1.example.com", "https://sp-2.example.com", "https://sp-3.example.com"} {
			if pinged[serviceURL] != 1 {
				t.Fatalf("ProviderPing calls for %s=%d want 1; all calls=%v", serviceURL, pinged[serviceURL], pinged)
			}
		}
		if approvedCalls.Load() != 1 || activeCalls.Load() != 1 || detailsCalls.Load() != 1 {
			t.Fatalf("selection input calls approved=%d active=%d details=%d want 1 each", approvedCalls.Load(), activeCalls.Load(), detailsCalls.Load())
		}
	})

	t.Run("returns partial healthy candidates", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
			},
			providerPing: func(_ context.Context, serviceURL string) error {
				if serviceURL == "https://sp-2.example.com" {
					return errors.New("provider unavailable")
				}
				return nil
			},
		})

		contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 2})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		got := contextsToFake(t, contexts)
		if len(got) != 1 || !got[0].ProviderID().Equal(testID(1)) {
			t.Fatalf("providers=%v want [1]", providerIDs(got))
		}
	})

	t.Run("non-positive copies are rejected", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
			},
		})

		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: -1})
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("ResolveUploadContexts error=%v want ErrInvalidArgument", err)
		}
	})

	t.Run("preserves data set preference before fallback providers", func(t *testing.T) {
		fallbackReturned := make(chan struct{})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2), testID(3)},
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
				testPDPProvider(testID(3), "https://sp-3.example.com"),
			},
			detailedDataSets: []*warmstorage.EnhancedDataSetInfo{{
				DataSetInfo: &warmstorage.DataSetInfo{
					DataSetID:  testID(22),
					ProviderID: testID(2),
				},
				IsLive:    true,
				IsManaged: true,
				Metadata:  map[string]string{},
			}},
			providerPing: func(_ context.Context, serviceURL string) error {
				switch serviceURL {
				case "https://sp-1.example.com":
					close(fallbackReturned)
					return nil
				case "https://sp-2.example.com":
					<-fallbackReturned
					return errors.New("provider unavailable")
				default:
					return nil
				}
			},
		})

		contexts, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{Copies: 1})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		got := contextsToFake(t, contexts)
		if len(got) != 1 || !got[0].ProviderID().Equal(testID(1)) {
			t.Fatalf("providers=%v want [1]", providerIDs(got))
		}
	})

	t.Run("returns ErrNoHealthyProviders when all candidates fail", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
			},
			providerPing: func(context.Context, string) error {
				return errors.New("provider unavailable")
			},
		})

		requireEndorsed := false
		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
			Copies:                 2,
			RequireEndorsedPrimary: &requireEndorsed,
		})
		if !errors.Is(err, ErrNoHealthyProviders) {
			t.Fatalf("ResolveUploadContexts error=%v want ErrNoHealthyProviders", err)
		}
		if !strings.Contains(err.Error(), "provider IDs: 1, 2") {
			t.Fatalf("ResolveUploadContexts error=%q want failed provider IDs", err)
		}
	})

	t.Run("preserves provider ranking across out-of-order responses", func(t *testing.T) {
		lowerRankReturned := make(chan struct{})
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
			},
			providerPing: func(_ context.Context, serviceURL string) error {
				if serviceURL == "https://sp-2.example.com" {
					close(lowerRankReturned)
					return nil
				}
				<-lowerRankReturned
				return nil
			},
		})

		contexts, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{Copies: 1})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		got := contextsToFake(t, contexts)
		if len(got) != 1 || !got[0].ProviderID().Equal(testID(1)) {
			t.Fatalf("providers=%v want [1]", providerIDs(got))
		}
	})
}

func TestServiceResolverResolveUploadContexts_HealthCheckConcurrencyIsBounded(t *testing.T) {
	const candidateCount = providerPingConcurrency + 4
	approved := make([]types.BigInt, 0, candidateCount)
	providers := make([]spregistry.PDPProvider, 0, candidateCount)
	for i := 1; i <= candidateCount; i++ {
		id := testID(uint64(i))
		approved = append(approved, id)
		providers = append(providers, testPDPProvider(id, fmt.Sprintf("https://sp-%d.example.com", i)))
	}

	started := make(chan struct{}, candidateCount)
	release := make(chan struct{})
	var calls, inFlight, peak atomic.Int32
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: approved,
		activeProviders:     providers,
		providerPing: func(ctx context.Context, _ string) error {
			calls.Add(1)
			current := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				observed := peak.Load()
				if current <= observed || peak.CompareAndSwap(observed, current) {
					break
				}
			}
			started <- struct{}{}
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})

	type result struct {
		contexts []StorageContext
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		requireEndorsed := false
		contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
			Copies:                 candidateCount,
			RequireEndorsedPrimary: &requireEndorsed,
		})
		resultCh <- result{contexts: contexts, err: err}
	}()

	startDeadline := time.NewTimer(time.Second)
	defer startDeadline.Stop()
	for i := range providerPingConcurrency {
		select {
		case <-started:
		case <-startDeadline.C:
			t.Fatalf("only %d of %d ProviderPing calls started", i, providerPingConcurrency)
		}
	}
	select {
	case <-started:
		t.Fatalf("ProviderPing started more than %d concurrent calls", providerPingConcurrency)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("ResolveUploadContexts: %v", result.err)
		}
		got := contextsToFake(t, result.contexts)
		if len(got) != candidateCount {
			t.Fatalf("providers=%v want %d providers", providerIDs(got), candidateCount)
		}
		for i, uploadContext := range got {
			if !uploadContext.ProviderID().Equal(testID(uint64(i + 1))) {
				t.Fatalf("provider[%d]=%s want %d", i, uploadContext.ProviderID(), i+1)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("ResolveUploadContexts did not finish after probes were released")
	}
	if got := calls.Load(); got != candidateCount {
		t.Fatalf("ProviderPing calls=%d want %d", got, candidateCount)
	}
	if got := peak.Load(); got != providerPingConcurrency {
		t.Fatalf("peak ProviderPing concurrency=%d want %d", got, providerPingConcurrency)
	}
}

func TestServiceResolverResolveUploadContexts_HealthCheckCancelsSpeculativeProbes(t *testing.T) {
	const candidateCount = providerPingConcurrency + 4
	approved := make([]types.BigInt, 0, candidateCount)
	providers := make([]spregistry.PDPProvider, 0, candidateCount)
	for i := 1; i <= candidateCount; i++ {
		id := testID(uint64(i))
		approved = append(approved, id)
		providers = append(providers, testPDPProvider(id, fmt.Sprintf("https://sp-%d.example.com", i)))
	}

	var calls, canceled atomic.Int32
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: approved,
		activeProviders:     providers,
		providerPing: func(ctx context.Context, serviceURL string) error {
			calls.Add(1)
			if serviceURL == "https://sp-1.example.com" {
				return nil
			}
			<-ctx.Done()
			canceled.Add(1)
			return ctx.Err()
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	contexts, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || !got[0].ProviderID().Equal(testID(1)) {
		t.Fatalf("providers=%v want [1]", providerIDs(got))
	}
	if got := calls.Load(); got != providerPingConcurrency {
		t.Fatalf("ProviderPing calls=%d want %d", got, providerPingConcurrency)
	}
	if got := canceled.Load(); got != providerPingConcurrency-1 {
		t.Fatalf("canceled ProviderPing calls=%d want %d", got, providerPingConcurrency-1)
	}
}

func TestServiceResolverResolveUploadContexts_HealthCheckStopsAtSelectionFrontier(t *testing.T) {
	const candidateCount = providerPingConcurrency + 1
	approved := make([]types.BigInt, 0, candidateCount)
	providers := make([]spregistry.PDPProvider, 0, candidateCount)
	for i := 1; i <= candidateCount; i++ {
		id := testID(uint64(i))
		approved = append(approved, id)
		providers = append(providers, testPDPProvider(id, fmt.Sprintf("https://sp-%d.example.com", i)))
	}

	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	speculativeCanceled := make(chan struct{})
	beyondFrontierStarted := make(chan struct{})
	var canceledOnce sync.Once
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: approved,
		activeProviders:     providers,
		providerPing: func(ctx context.Context, serviceURL string) error {
			switch serviceURL {
			case "https://sp-1.example.com":
				close(firstStarted)
				select {
				case <-releaseFirst:
					return errors.New("provider unavailable")
				case <-ctx.Done():
					return ctx.Err()
				}
			case "https://sp-2.example.com":
				<-firstStarted
				return nil
			case fmt.Sprintf("https://sp-%d.example.com", candidateCount):
				close(beyondFrontierStarted)
				<-ctx.Done()
				return ctx.Err()
			default:
				<-ctx.Done()
				canceledOnce.Do(func() { close(speculativeCanceled) })
				return ctx.Err()
			}
		},
	})

	type result struct {
		contexts []StorageContext
		err      error
	}
	resultCh := make(chan result, 1)
	ctx := t.Context()
	go func() {
		contexts, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{Copies: 1})
		resultCh <- result{contexts: contexts, err: err}
	}()

	beyondFrontier := false
	select {
	case <-speculativeCanceled:
	case <-beyondFrontierStarted:
		beyondFrontier = true
	case <-time.After(time.Second):
		t.Fatal("speculative probes were not canceled after the selection frontier was known")
	}
	close(releaseFirst)

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("ResolveUploadContexts: %v", result.err)
		}
		got := contextsToFake(t, result.contexts)
		if len(got) != 1 || !got[0].ProviderID().Equal(testID(2)) {
			t.Fatalf("providers=%v want [2]", providerIDs(got))
		}
	case <-time.After(time.Second):
		t.Fatal("ResolveUploadContexts did not finish after the leading probe was released")
	}
	if beyondFrontier {
		t.Fatalf("ProviderPing started candidate %d beyond the known selection frontier", candidateCount)
	}
}

func TestServiceResolverResolveUploadContexts_HealthCheckHonorsParentCancellation(t *testing.T) {
	started := make(chan struct{})
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		providerPing: func(ctx context.Context, _ string) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{Copies: 1})
		errCh <- err
	}()
	<-started
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ResolveUploadContexts error=%v want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ResolveUploadContexts did not stop after parent cancellation")
	}
}

func TestServiceResolverResolveUploadContexts_HealthCheckHonorsShorterParentDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	wantDeadline, _ := ctx.Deadline()
	deadlineMatches := make(chan bool, 1)
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		providerPing: func(ctx context.Context, _ string) error {
			got, ok := ctx.Deadline()
			deadlineMatches <- ok && got.Equal(wantDeadline)
			<-ctx.Done()
			return ctx.Err()
		},
	})

	errCh := make(chan error, 1)
	go func() {
		_, _, err := resolver.ResolveUploadContexts(ctx, &UploadOptions{Copies: 1})
		errCh <- err
	}()
	select {
	case matches := <-deadlineMatches:
		if !matches {
			t.Fatal("ProviderPing deadline did not inherit the shorter parent deadline")
		}
	case err := <-errCh:
		t.Fatalf("ResolveUploadContexts returned before ProviderPing observed the deadline: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("ResolveUploadContexts error=%v want context.Canceled", err)
	}
}

func TestServiceResolverResolveUploadContexts_DefaultProviderPing(t *testing.T) {
	newResolver := func(t *testing.T, providers []spregistry.PDPProvider) *ServiceResolver {
		t.Helper()
		approved := make([]types.BigInt, len(providers))
		for i := range providers {
			approved[i] = providers[i].Info.ID
		}
		fixture := serviceResolverFixture{approvedProviderIDs: approved, activeProviders: providers}
		resolver, err := NewServiceResolver(ServiceResolverOptions{
			Payer:        testPayer(),
			SPRegistry:   &fakePDPProviderSource{fixture: fixture},
			Endorsements: &fakeEndorsedProviderSource{fixture: fixture},
			WarmStorage:  &fakeDataSetCatalog{fixture: fixture},
			NewContext:   newResolvedTestContext,
		})
		if err != nil {
			t.Fatalf("NewServiceResolver: %v", err)
		}
		return resolver
	}

	t.Run("retries transient failures twice", func(t *testing.T) {
		var requests atomic.Int32
		provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempt := requests.Add(1)
			if r.Method != http.MethodGet || r.URL.Path != "/pdp/ping" {
				t.Errorf("provider request=%s %s want GET /pdp/ping", r.Method, r.URL.Path)
			}
			if attempt < 3 {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_, _ = w.Write([]byte("curio-pdp"))
		}))
		defer provider.Close()

		resolver := newResolver(t, []spregistry.PDPProvider{testPDPProvider(testID(1), provider.URL)})
		contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		got := contextsToFake(t, contexts)
		if len(got) != 1 || !got[0].ProviderID().Equal(testID(1)) {
			t.Fatalf("providers=%v want [1]", providerIDs(got))
		}
		if got := requests.Load(); got != 3 {
			t.Fatalf("ping requests=%d want 3", got)
		}
	})

	t.Run("does not retry permanent client errors", func(t *testing.T) {
		var rejectedRequests, healthyRequests atomic.Int32
		rejected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			rejectedRequests.Add(1)
			http.Error(w, "bad request", http.StatusBadRequest)
		}))
		defer rejected.Close()
		healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			healthyRequests.Add(1)
			_, _ = w.Write([]byte("curio-pdp"))
		}))
		defer healthy.Close()

		resolver := newResolver(t, []spregistry.PDPProvider{
			testPDPProvider(testID(1), rejected.URL),
			testPDPProvider(testID(2), healthy.URL),
		})
		contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		got := contextsToFake(t, contexts)
		if len(got) != 1 || !got[0].ProviderID().Equal(testID(2)) {
			t.Fatalf("providers=%v want [2]", providerIDs(got))
		}
		if got := rejectedRequests.Load(); got != 1 {
			t.Fatalf("rejected provider requests=%d want 1", got)
		}
		if got := healthyRequests.Load(); got != 1 {
			t.Fatalf("healthy provider requests=%d want 1", got)
		}
	})
}

func TestServiceResolverResolveUploadContexts_RejectsNilContextFactoryResult(t *testing.T) {
	providerID := testID(7)
	resolver := newNilContextServiceResolver(t, serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(7): ptrPDPProvider(testPDPProvider(providerID, "https://sp-7.example.com")),
		},
	})

	_, err := resolver.ResolveProviderContext(context.Background(), providerID, NewProviderContextOptions{})
	if err == nil {
		t.Fatal("ResolveProviderContext returned nil error; want nil context factory error")
	}
	if !strings.Contains(err.Error(), "nil provider context") {
		t.Fatalf("err=%v want nil context message", err)
	}
}

func TestServiceResolverSelectReplacement_RejectsNilContextFactoryResult(t *testing.T) {
	providerID := testID(3)
	resolver := newNilContextServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{providerID},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(providerID, "https://sp-3.example.com"),
		},
	})

	_, err := resolver.SelectReplacement(context.Background(), nil, &UploadOptions{})
	if err == nil {
		t.Fatal("SelectReplacement returned nil error; want nil context factory error")
	}
	if !strings.Contains(err.Error(), "nil provider context") {
		t.Fatalf("err=%v want nil context message", err)
	}
}

func TestServiceResolverResolveDataSetContextValidatesOwnership(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(33): {DataSetID: testID(33), ProviderID: testID(5), Payer: common.HexToAddress("0x00000000000000000000000000000000000000ff"), PDPEndEpoch: 0},
		},
	})

	_, err := resolver.ResolveDataSetContext(context.Background(), testID(33), NewDataSetContextOptions{})
	if err == nil || err.Error() == "" {
		t.Fatal("expected ownership error")
	}
}

func TestServiceResolverResolveDataSetContextAllowsEndedRail(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		providerPing: func(context.Context, string) error {
			t.Fatal("explicit data set selection called ProviderPing")
			return nil
		},
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(55): {
				DataSetID:       testID(55),
				ProviderID:      testID(5),
				Payer:           testPayer(),
				PDPEndEpoch:     1000,
				ClientDataSetID: testID(155),
				CommissionBps:   bigInt(0),
				Payee:           common.HexToAddress("0x5005"),
				ServiceProvider: common.HexToAddress("0x5006"),
			},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(5): ptrPDPProvider(testPDPProvider(testID(5), "https://sp-5.example.com")),
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(55): {"source": "app"},
		},
		validatorEnabled: true,
	})

	dataSetCtx, err := resolver.ResolveDataSetContext(context.Background(), testID(55), NewDataSetContextOptions{})
	if err != nil {
		t.Fatalf("ResolveDataSetContext: %v", err)
	}
	if got := dataSetCtx.DataSetID(); !got.Equal(testID(55)) {
		t.Fatalf("DataSetID=%s want 55", got.String())
	}
}

func TestServiceResolverResolveUploadContexts_AutoSelectSkipsUnusableDetailedDataSets(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
		},
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:          false,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{"source": "app"},
			},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(11): {"source": "app"},
		},
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if dataSetIDOf(got[0]) != nil {
		t.Fatalf("auto-select reused dataSetID=%v want nil", dataSetIDOf(got[0]))
	}
}

func TestServiceResolverResolveWritableUploadContexts_AutoSelectTrustsDetailedSnapshot(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{"source": "app"},
			},
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(12), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{"source": "app"},
			},
		},
		validatorEnabled: true,
		validatorErr:     errors.New("validator must not run"),
	})

	contexts, _, err := resolver.resolveWritableUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("resolveWritableUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || dataSetIDOf(got[0]) == nil || !dataSetIDOf(got[0]).Equal(testID(11)) {
		t.Fatalf("context=%+v want dataSetID 11 from detailed snapshot", got)
	}
}

func TestSelectMatchingDetailedDataSet_PrefersActiveThenLowestID(t *testing.T) {
	providerID := testID(1)
	dataSetID, clientDataSetID, metadata := selectMatchingDetailedDataSet(providerID, []*warmstorage.EnhancedDataSetInfo{
		{
			DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(1), ProviderID: providerID, ClientDataSetID: testID(101)},
			IsLive:          true,
			IsManaged:       true,
			HasActivePieces: false,
			Metadata:        map[string]string{"source": "app"},
		},
		{
			DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(3), ProviderID: providerID, ClientDataSetID: testID(103)},
			IsLive:          true,
			IsManaged:       true,
			HasActivePieces: true,
			Metadata:        map[string]string{"source": "app"},
		},
		{
			DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(2), ProviderID: providerID, ClientDataSetID: testID(102)},
			IsLive:          true,
			IsManaged:       true,
			HasActivePieces: true,
			Metadata:        map[string]string{"source": "app"},
		},
	}, map[string]string{"source": "app"})

	if dataSetID == nil || !dataSetID.Equal(testID(2)) {
		t.Fatalf("DataSetID=%v want 2", dataSetID)
	}
	if clientDataSetID == nil || !clientDataSetID.Equal(testID(102)) {
		t.Fatalf("ClientDataSetID=%v want 102", clientDataSetID)
	}
	if metadata["source"] != "app" {
		t.Fatalf("metadata=%v want source=app", metadata)
	}
}

func TestServiceResolverResolveUploadContexts_AutoSelectRetriesRetryableDetailEnrichmentFailure(t *testing.T) {
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{"source": "app"},
			},
		},
	}
	want := errors.New(`Post "https://api.calibration.node.glif.io/rpc/v1": EOF`)
	catalog := &flakyDetailsCatalog{
		fakeEnhancedDataSetCatalog: fakeEnhancedDataSetCatalog{
			fakeDataSetCatalog: fakeDataSetCatalog{fixture: fixture},
		},
		firstErr: want,
	}
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:        testPayer(),
		SPRegistry:   &fakePDPProviderSource{fixture: fixture},
		Endorsements: &fakeEndorsedProviderSource{fixture: fixture},
		WarmStorage:  catalog,
		ProviderPing: healthyProviderPing,
		NewContext:   newResolvedTestContext,
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || dataSetIDOf(got[0]) == nil || !dataSetIDOf(got[0]).Equal(testID(11)) {
		t.Fatalf("context=%+v want dataSetID 11 after details retry", got)
	}
	if attempts := catalog.attempts.Load(); attempts != 2 {
		t.Fatalf("GetClientDataSetsWithDetails attempts=%d want 2", attempts)
	}
}

func TestServiceResolverDataSetAcceptsUpload_PropagatesRetryableValidatorError(t *testing.T) {
	want := errors.New(`Post "https://api.calibration.node.glif.io/rpc/v1": EOF`)
	resolver := &ServiceResolver{
		dataSetValidator: &fakeDataSetValidator{err: want},
	}

	ok, err := resolver.dataSetAcceptsUpload(context.Background(), testID(11))
	if ok || !errors.Is(err, want) {
		t.Fatalf("dataSetAcceptsUpload ok=%v err=%v want retryable validator error", ok, err)
	}
}

func TestServiceResolverDataSetAcceptsUpload_ClassifiesSkippableErrors(t *testing.T) {
	for _, err := range []error{
		warmstorage.ErrDataSetUnavailable,
		&warmstorage.DataSetNotManagedError{DataSetID: testID(11)},
	} {
		resolver := &ServiceResolver{dataSetValidator: &fakeDataSetValidator{err: err}}
		ok, gotErr := resolver.dataSetAcceptsUpload(context.Background(), testID(11))
		if ok || gotErr != nil {
			t.Fatalf("dataSetAcceptsUpload(%T) = (%t, %v), want false, nil", err, ok, gotErr)
		}
	}

	want := errors.New("ABI decode failed")
	resolver := &ServiceResolver{dataSetValidator: &fakeDataSetValidator{err: want}}
	ok, err := resolver.dataSetAcceptsUpload(context.Background(), testID(11))
	if ok || !errors.Is(err, want) {
		t.Fatalf("dataSetAcceptsUpload = (%t, %v), want propagated unknown error", ok, err)
	}
}

func TestServiceResolverResolveUploadContexts_FallsBackAfterUnavailableDetails(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
		},
		dataSetDetailsErr: warmstorage.ErrDataSetUnavailable,
		dataSetMetadata: map[string]map[string]string{
			testIDKey(11): {"source": "app"},
		},
		validatorEnabled: true,
		validatorErr:     warmstorage.ErrDataSetUnavailable,
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || dataSetIDOf(got[0]) != nil {
		t.Fatalf("contexts=%v, want a new data set after unavailable candidate", got)
	}
}

func TestServiceResolverResolveUploadContexts_AutoSelectWithoutDetailsDoesNotReuseDataSet(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(11): {"source": "app"},
		},
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if dataSetIDOf(got[0]) != nil {
		t.Fatalf("auto-select reused dataSetID=%v want nil without details", dataSetIDOf(got[0]))
	}
}

func TestServiceResolverResolveUploadContexts_AutoSelectTreatsUnconfiguredPDPVerifierAsNoDetails(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
		},
		dataSetDetailsErr: fmt.Errorf("details unavailable: %w", warmstorage.ErrPDPVerifierNotConfigured),
		dataSetMetadata: map[string]map[string]string{
			testIDKey(11): {"source": "app"},
		},
		validatorEnabled: true,
		validatorErr:     warmstorage.ErrDataSetUnavailable,
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if dataSetIDOf(got[0]) != nil {
		t.Fatalf("auto-select reused dataSetID=%v want nil when details are unavailable", dataSetIDOf(got[0]))
	}
}

func TestServiceResolverResolveUploadContexts_AutoSelectRequestsOnlyManagedDetails(t *testing.T) {
	var onlyManaged *bool
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{},
			},
		},
		dataSetDetailsOnlyManaged: &onlyManaged,
	})

	if _, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1}); err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	if onlyManaged == nil || !*onlyManaged {
		t.Fatalf("onlyManaged=%v want true", onlyManaged)
	}
}

func TestServiceResolverResolveUploadContexts_AutoSelectReturnsDetailEnrichmentFailure(t *testing.T) {
	want := errors.New("dataSetLive failed")
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		dataSetDetailsErr: want,
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
	if contexts != nil || !errors.Is(err, want) {
		t.Fatalf("ResolveUploadContexts=(%v, %v) want nil result wrapping detail error", contexts, err)
	}
}

func TestServiceResolverResolveUploadContexts_RetriesTransientSelectionErrors(t *testing.T) {
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
	}
	catalog := &flakyApprovedProviderCatalog{
		fakeDataSetCatalog: fakeDataSetCatalog{fixture: fixture},
	}
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:        testPayer(),
		SPRegistry:   &fakePDPProviderSource{fixture: fixture},
		Endorsements: &fakeEndorsedProviderSource{fixture: fixture},
		WarmStorage:  catalog,
		ProviderPing: healthyProviderPing,
		NewContext:   newResolvedTestContext,
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
		t.Fatalf("GetApprovedProviderIDs attempts=%d want 2", attempts)
	}
}

type serviceResolverFixture struct {
	approvedProviderIDs       []types.BigInt
	endorsedProviderIDs       []types.BigInt
	endorsementsSet           bool
	endorsementsErr           error
	endorsementCalls          *atomic.Int32
	activeProviders           []spregistry.PDPProvider
	clientDataSets            []*warmstorage.DataSetInfo
	detailedDataSets          []*warmstorage.EnhancedDataSetInfo
	dataSetDetailsErr         error
	dataSetDetailsOnlyManaged **bool
	dataSetMetadata           map[string]map[string]string
	providersByID             map[string]*spregistry.PDPProvider
	dataSetsByID              map[string]*warmstorage.DataSetInfo
	validatorEnabled          bool
	validatorErr              error
	validatorErrByID          map[string]error
	requirePositiveListLimit  bool
	providerPing              func(context.Context, string) error
	approvedProviderCalls     *atomic.Int32
	activeProviderCalls       *atomic.Int32
	dataSetDetailsCalls       *atomic.Int32
}

func newTestServiceResolver(t *testing.T, fixture serviceResolverFixture) *ServiceResolver {
	t.Helper()
	providerPing := fixture.providerPing
	if providerPing == nil {
		providerPing = healthyProviderPing
	}
	var catalog DataSetCatalog = &fakeDataSetCatalog{fixture: fixture}
	if fixture.validatorEnabled || fixture.detailedDataSets != nil || fixture.dataSetDetailsErr != nil {
		catalog = &fakeEnhancedDataSetCatalog{fakeDataSetCatalog: fakeDataSetCatalog{fixture: fixture}}
	}
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:        testPayer(),
		SPRegistry:   &fakePDPProviderSource{fixture: fixture},
		Endorsements: &fakeEndorsedProviderSource{fixture: fixture},
		WarmStorage:  catalog,
		ProviderPing: providerPing,
		NewContext:   newResolvedTestContext,
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}
	return resolver
}

func newNilContextServiceResolver(t *testing.T, fixture serviceResolverFixture) *ServiceResolver {
	t.Helper()
	providerPing := fixture.providerPing
	if providerPing == nil {
		providerPing = healthyProviderPing
	}
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:        testPayer(),
		SPRegistry:   &fakePDPProviderSource{fixture: fixture},
		Endorsements: &fakeEndorsedProviderSource{fixture: fixture},
		WarmStorage:  &fakeDataSetCatalog{fixture: fixture},
		ProviderPing: providerPing,
		NewContext: func(Provider, ContextFactoryOptions) (*ProviderContext, error) {
			return nil, nil
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

type fakeEndorsedProviderSource struct {
	fixture serviceResolverFixture
}

func (f *fakeEndorsedProviderSource) GetEndorsedProviderIDs(context.Context) ([]types.BigInt, error) {
	if f.fixture.endorsementCalls != nil {
		f.fixture.endorsementCalls.Add(1)
	}
	if f.fixture.endorsementsErr != nil {
		return nil, f.fixture.endorsementsErr
	}
	if f.fixture.endorsementsSet {
		return cloneBigIntSlice(f.fixture.endorsedProviderIDs), nil
	}
	return cloneBigIntSlice(f.fixture.approvedProviderIDs), nil
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
	if f.fixture.activeProviderCalls != nil {
		f.fixture.activeProviderCalls.Add(1)
	}
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

type fakeEnhancedDataSetCatalog struct {
	fakeDataSetCatalog
}

func (f *fakeEnhancedDataSetCatalog) ValidateDataSet(_ context.Context, dataSetID types.BigInt) error {
	if err := f.fixture.validatorErrByID[idconv.Key(dataSetID)]; err != nil {
		return err
	}
	if f.fixture.validatorErr != nil {
		return f.fixture.validatorErr
	}
	if f.fixture.validatorEnabled {
		return nil
	}
	return fmt.Errorf("fakeEnhancedDataSetCatalog.ValidateDataSet: unexpected dataSetID %s", dataSetID.String())
}

func (f *fakeEnhancedDataSetCatalog) GetClientDataSetsWithDetails(_ context.Context, _ common.Address, onlyManaged bool) ([]*warmstorage.EnhancedDataSetInfo, error) {
	if f.fixture.dataSetDetailsCalls != nil {
		f.fixture.dataSetDetailsCalls.Add(1)
	}
	if f.fixture.dataSetDetailsOnlyManaged != nil {
		value := onlyManaged
		*f.fixture.dataSetDetailsOnlyManaged = &value
	}
	if f.fixture.dataSetDetailsErr != nil {
		return nil, f.fixture.dataSetDetailsErr
	}
	out := make([]*warmstorage.EnhancedDataSetInfo, 0, len(f.fixture.detailedDataSets))
	for _, dataSet := range f.fixture.detailedDataSets {
		if dataSet == nil {
			continue
		}
		if onlyManaged && !dataSet.IsManaged {
			continue
		}
		cloned := *dataSet
		if dataSet.DataSetInfo != nil {
			base := *dataSet.DataSetInfo
			cloned.DataSetInfo = &base
		}
		cloned.Metadata = cloneStringMap(dataSet.Metadata)
		out = append(out, &cloned)
	}
	return out, nil
}

type flakyApprovedProviderCatalog struct {
	fakeDataSetCatalog
	attempts atomic.Int32
}

func (f *flakyApprovedProviderCatalog) GetApprovedProviderIDs(ctx context.Context, opts types.ListOptions) ([]types.BigInt, error) {
	if f.attempts.Add(1) == 1 {
		return nil, fmt.Errorf("Post %q: EOF", "https://api.calibration.node.glif.io/rpc/v1")
	}
	return f.fakeDataSetCatalog.GetApprovedProviderIDs(ctx, opts)
}

type flakyDetailsCatalog struct {
	fakeEnhancedDataSetCatalog
	attempts atomic.Int32
	firstErr error
}

func (f *flakyDetailsCatalog) GetClientDataSetsWithDetails(ctx context.Context, payer common.Address, onlyManaged bool) ([]*warmstorage.EnhancedDataSetInfo, error) {
	if f.attempts.Add(1) == 1 {
		return nil, f.firstErr
	}
	return f.fakeEnhancedDataSetCatalog.GetClientDataSetsWithDetails(ctx, payer, onlyManaged)
}

func (f *fakeDataSetCatalog) GetApprovedProviderIDs(_ context.Context, opts types.ListOptions) ([]types.BigInt, error) {
	if f.fixture.approvedProviderCalls != nil {
		f.fixture.approvedProviderCalls.Add(1)
	}
	if f.fixture.requirePositiveListLimit {
		if err := opts.Validate(); err != nil {
			return nil, err
		}
	}
	start := min(int(opts.Offset), len(f.fixture.approvedProviderIDs))
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
	start := min(int(opts.Offset), len(f.fixture.clientDataSets))
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

func contextsToFake(t *testing.T, contexts []StorageContext) []StorageContext {
	t.Helper()
	return append([]StorageContext(nil), contexts...)
}

func dataSetIDOf(ctx StorageContext) *types.BigInt {
	ref, ok := ctx.DataSetRef()
	if !ok {
		return nil
	}
	id := ref.DataSetID()
	return &id
}

func clientDataSetIDOf(ctx StorageContext) *types.BigInt {
	ref, ok := ctx.DataSetRef()
	if !ok {
		return nil
	}
	id := ref.ClientDataSetID()
	return &id
}

func dataSetMetadataOf(t *testing.T, ctx StorageContext) map[string]string {
	t.Helper()
	switch concrete := ctx.(type) {
	case *ProviderContext:
		return cloneStringMap(concrete.core.dataSetMetadata)
	case *DataSetContext:
		return cloneStringMap(concrete.core.dataSetMetadata)
	default:
		t.Fatalf("unexpected context type %T", ctx)
		return nil
	}
}

func providerIDs(contexts []StorageContext) []string {
	ids := make([]string, len(contexts))
	for i, ctx := range contexts {
		ids[i] = ctx.ProviderID().String()
	}
	return ids
}

func newResolvedTestContext(provider Provider, opts ContextFactoryOptions) (*ProviderContext, error) {
	return NewProviderContext(
		provider,
		&fakePDPProviderClient{},
		nil,
		WithDataSetMetadata(opts.DataSetMetadata),
		WithCDN(opts.WithCDN),
	)
}

// TestServiceResolverResolveUploadContexts_CarriesClientDataSetID proves that
// an automatically reused target is complete before it is returned.
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
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0, ClientDataSetID: clientDataSetID},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{},
			},
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
	if clientDataSetIDOf(got[0]) == nil || !clientDataSetIDOf(got[0]).Equal(clientDataSetID) {
		t.Fatalf("clientDataSetID=%v want %s", clientDataSetIDOf(got[0]), clientDataSetID.String())
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

func TestDetailedCandidateProvidersOnlyIncludesSelectableProviders(t *testing.T) {
	selectable := map[string]struct{}{
		testIDKey(1): {},
	}
	got := detailedCandidateProviders([]*warmstorage.EnhancedDataSetInfo{
		nil,
		{DataSetInfo: nil},
		{DataSetInfo: &warmstorage.DataSetInfo{DataSetID: testID(10), ProviderID: testID(0)}},
		{DataSetInfo: &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1)}},
		{DataSetInfo: &warmstorage.DataSetInfo{DataSetID: testID(12), ProviderID: testID(1)}},
		{DataSetInfo: &warmstorage.DataSetInfo{DataSetID: testID(22), ProviderID: testID(2)}},
	}, selectable)
	providerDataSets := got[testIDKey(1)]
	if len(providerDataSets) != 2 {
		t.Fatalf("provider 1 dataset count=%d want 2: %v", len(providerDataSets), got)
	}
	if !providerDataSets[0].DataSetID.Equal(testID(11)) || !providerDataSets[1].DataSetID.Equal(testID(12)) {
		t.Fatalf("provider 1 datasets=%v want 11,12", providerDataSets)
	}
	if _, ok := got[testIDKey(0)]; ok {
		t.Fatalf("zero provider present in detailed candidate set: %v", got)
	}
	if _, ok := got[testIDKey(1)]; !ok {
		t.Fatalf("provider 1 missing from detailed candidate set: %v", got)
	}
	if _, ok := got[testIDKey(2)]; ok {
		t.Fatalf("provider 2 present in detailed candidate set: %v", got)
	}
	if len(got) != 1 {
		t.Fatalf("detailed candidate set len=%d want 1: %v", len(got), got)
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
	cloned.ExcludeProviderIDs[0] = testID(99)
	if !orig.ExcludeProviderIDs[0].Equal(testID(3)) {
		t.Fatal("ExcludeProviderIDs clone mutated original")
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

func TestServiceResolverSelectWritableReplacement_TrustsDetailedSnapshot(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
			testPDPProvider(testID(2), "https://sp-2.example.com"),
		},
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(21), ProviderID: testID(2), PDPEndEpoch: 0},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{},
			},
		},
		validatorEnabled: true,
		validatorErr:     errors.New("not writable"),
	})

	replacement, err := resolver.selectWritableReplacement(context.Background(), map[string]types.BigInt{
		testIDKey(1): testID(1),
	}, &UploadOptions{})
	if err != nil {
		t.Fatalf("selectWritableReplacement: %v", err)
	}
	ref, ok := replacement.DataSetRef()
	if !replacement.ProviderID().Equal(testID(2)) || !ok || !ref.DataSetID().Equal(testID(21)) {
		t.Fatalf("replacement=%+v want provider 2 dataSetID 21 from detailed snapshot", replacement)
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

func TestServiceResolverRejectsFactoryTargetMismatch(t *testing.T) {
	fixture := serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(1): ptrPDPProvider(testPDPProvider(testID(1), "https://sp-1.example.com")),
		},
	}
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:       testPayer(),
		SPRegistry:  &fakePDPProviderSource{fixture: fixture},
		WarmStorage: &fakeDataSetCatalog{fixture: fixture},
		NewContext: func(provider Provider, _ ContextFactoryOptions) (*ProviderContext, error) {
			provider.ID = testID(999)
			return NewProviderContext(provider, &fakePDPProviderClient{}, nil)
		},
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}
	_, err = resolver.ResolveProviderContext(context.Background(), testID(1), NewProviderContextOptions{})
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "factory returned providerID") {
		t.Fatalf("ResolveProviderContext error=%v", err)
	}
}

func TestBuildResolvedUploadContextRequiresCompleteDataSetRef(t *testing.T) {
	provider := testPDPProvider(testID(1), "https://sp-1.example.com")
	dataSetID := testID(11)
	if _, err := buildResolvedUploadContext(provider, &dataSetID, nil, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("missing clientDataSetID error=%v", err)
	}
	clientDataSetID := types.BigInt{}
	selection, err := buildResolvedUploadContext(provider, &dataSetID, &clientDataSetID, nil)
	if err != nil {
		t.Fatalf("buildResolvedUploadContext: %v", err)
	}
	if selection.DataSet == nil || !selection.DataSet.ClientDataSetID().IsZero() {
		t.Fatalf("selection=%+v, want a complete ref with legal zero clientDataSetID", selection)
	}
}

func TestServiceResolverReturnsConcreteContextKinds(t *testing.T) {
	assertKind := func(t *testing.T, got StorageContext, wantDataSet bool) {
		t.Helper()
		if wantDataSet {
			if _, ok := got.(*DataSetContext); !ok {
				t.Fatalf("context type=%T want *DataSetContext", got)
			}
			return
		}
		if _, ok := got.(*ProviderContext); !ok {
			t.Fatalf("context type=%T want *ProviderContext", got)
		}
	}

	t.Run("explicit data set", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			dataSetsByID: map[string]*warmstorage.DataSetInfo{
				testIDKey(11): {DataSetID: testID(11), ProviderID: testID(1), Payer: testPayer(), ClientDataSetID: testID(101)},
			},
			providersByID: map[string]*spregistry.PDPProvider{
				testIDKey(1): ptrPDPProvider(testPDPProvider(testID(1), "https://sp-1.example.com")),
			},
			dataSetMetadata: map[string]map[string]string{testIDKey(11): {}},
		})
		dataSetCtx, err := resolver.ResolveDataSetContext(context.Background(), testID(11), NewDataSetContextOptions{})
		if err != nil {
			t.Fatalf("ResolveDataSetContext: %v", err)
		}
		assertKind(t, dataSetCtx, true)
	})

	t.Run("explicit provider remains unbound", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			providersByID: map[string]*spregistry.PDPProvider{
				testIDKey(1): ptrPDPProvider(testPDPProvider(testID(1), "https://sp-1.example.com")),
			},
			clientDataSets:  []*warmstorage.DataSetInfo{{DataSetID: testID(11), ProviderID: testID(1), ClientDataSetID: testID(101)}},
			dataSetMetadata: map[string]map[string]string{testIDKey(11): {}},
		})
		providerCtx, err := resolver.ResolveProviderContext(context.Background(), testID(1), NewProviderContextOptions{})
		if err != nil {
			t.Fatalf("ResolveProviderContext: %v", err)
		}
		assertKind(t, providerCtx, false)
	})

	t.Run("automatic reuse", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1)},
			activeProviders:     []spregistry.PDPProvider{testPDPProvider(testID(1), "https://sp-1.example.com")},
			detailedDataSets: []*warmstorage.EnhancedDataSetInfo{{
				DataSetInfo:     &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), ClientDataSetID: testID(101)},
				IsLive:          true,
				IsManaged:       true,
				HasActivePieces: true,
				Metadata:        map[string]string{},
			}},
		})
		selection, err := resolver.SelectUploadContexts(context.Background(), SelectUploadContextsOptions{Copies: 1})
		if err != nil {
			t.Fatalf("SelectUploadContexts: %v", err)
		}
		assertKind(t, selection.Contexts[0], true)
	})

	t.Run("new provider target", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			providersByID: map[string]*spregistry.PDPProvider{
				testIDKey(1): ptrPDPProvider(testPDPProvider(testID(1), "https://sp-1.example.com")),
			},
		})
		providerCtx, err := resolver.ResolveProviderContext(context.Background(), testID(1), NewProviderContextOptions{})
		if err != nil {
			t.Fatalf("ResolveProviderContext: %v", err)
		}
		assertKind(t, providerCtx, false)
	})
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
	if !got[0].ProviderID().Equal(testID(999)) {
		t.Fatalf("provider=%s want 999", got[0].ProviderID())
	}
}
