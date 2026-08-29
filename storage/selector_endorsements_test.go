package storage

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
)

func TestServiceResolverEndorsedPrimary(t *testing.T) {
	t.Run("primary uses endorsed pool and secondary uses approved pool", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2), testID(3)},
			endorsedProviderIDs: []types.BigInt{testID(2)},
			endorsementsSet:     true,
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
				testPDPProvider(testID(3), "https://sp-3.example.com"),
			},
		})

		contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 2})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		got := contextsToFake(t, contexts)
		if len(got) != 2 || !got[0].ProviderID().Equal(testID(2)) || !got[1].ProviderID().Equal(testID(1)) {
			t.Fatalf("providers = %v, want [2 1]", providerIDs(got))
		}
	})

	t.Run("reuses endorsed probes for secondary selection", func(t *testing.T) {
		var mu sync.Mutex
		calls := make(map[string]int)
		thirdFinished := make(chan struct{})
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2), testID(3)},
			endorsedProviderIDs: []types.BigInt{testID(2), testID(3)},
			endorsementsSet:     true,
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
				testPDPProvider(testID(3), "https://sp-3.example.com"),
			},
			providerPing: func(_ context.Context, serviceURL string) error {
				mu.Lock()
				calls[serviceURL]++
				mu.Unlock()
				switch serviceURL {
				case "https://sp-2.example.com":
					<-thirdFinished
				case "https://sp-3.example.com":
					close(thirdFinished)
				}
				return nil
			},
		})

		contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 2})
		if err != nil {
			t.Fatalf("ResolveUploadContexts: %v", err)
		}
		got := contextsToFake(t, contexts)
		if len(got) != 2 || !got[0].ProviderID().Equal(testID(2)) || !got[1].ProviderID().Equal(testID(1)) {
			t.Fatalf("providers = %v, want [2 1]", providerIDs(got))
		}
		mu.Lock()
		defer mu.Unlock()
		for _, serviceURL := range []string{"https://sp-1.example.com", "https://sp-2.example.com", "https://sp-3.example.com"} {
			if calls[serviceURL] != 1 {
				t.Fatalf("ping calls for %s = %d, want 1; all calls=%v", serviceURL, calls[serviceURL], calls)
			}
		}
	})

	t.Run("empty endorsement set", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1)},
			endorsementsSet:     true,
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
			},
		})
		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if !errors.Is(err, ErrNoEndorsedProvider) {
			t.Fatalf("error = %v, want ErrNoEndorsedProvider", err)
		}
	})

	t.Run("no approved active intersection", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1)},
			endorsedProviderIDs: []types.BigInt{testID(2)},
			endorsementsSet:     true,
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
			},
		})
		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if !errors.Is(err, ErrNoEndorsedProvider) {
			t.Fatalf("error = %v, want ErrNoEndorsedProvider", err)
		}
	})

	t.Run("no approved providers", func(t *testing.T) {
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			endorsedProviderIDs: []types.BigInt{testID(2)},
			endorsementsSet:     true,
		})
		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if !errors.Is(err, ErrNoEndorsedProvider) {
			t.Fatalf("error = %v, want ErrNoEndorsedProvider", err)
		}
	})

	t.Run("unhealthy endorsed providers preserve probe causes", func(t *testing.T) {
		want := errors.New("wrong ping identity")
		resolver := newTestServiceResolver(t, serviceResolverFixture{
			approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
			endorsedProviderIDs: []types.BigInt{testID(2)},
			endorsementsSet:     true,
			activeProviders: []spregistry.PDPProvider{
				testPDPProvider(testID(1), "https://sp-1.example.com"),
				testPDPProvider(testID(2), "https://sp-2.example.com"),
			},
			providerPing: func(_ context.Context, serviceURL string) error {
				if strings.Contains(serviceURL, "sp-2") {
					return want
				}
				return nil
			},
		})
		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if !errors.Is(err, ErrNoEndorsedProvider) || !errors.Is(err, want) {
			t.Fatalf("error = %v, want ErrNoEndorsedProvider and probe cause", err)
		}
	})
}

func TestServiceResolverEndorsementConfiguration(t *testing.T) {
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
	}
	newResolver := func(t *testing.T, endorsements EndorsedProviderSource) *ServiceResolver {
		t.Helper()
		resolver, err := NewServiceResolver(ServiceResolverOptions{
			Payer:        testPayer(),
			SPRegistry:   &fakePDPProviderSource{fixture: fixture},
			Endorsements: endorsements,
			WarmStorage:  &fakeDataSetCatalog{fixture: fixture},
			ProviderPing: healthyProviderPing,
			NewContext:   newResolvedTestContext,
		})
		if err != nil {
			t.Fatalf("NewServiceResolver: %v", err)
		}
		return resolver
	}

	t.Run("default requires configured source", func(t *testing.T) {
		resolver := newResolver(t, nil)
		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if !errors.Is(err, ErrEndorsementsNotConfigured) || !errors.Is(err, spregistry.ErrEndorsementsNotConfigured) {
			t.Fatalf("error = %v, want shared ErrEndorsementsNotConfigured", err)
		}
	})

	t.Run("query failure is preserved", func(t *testing.T) {
		want := errors.New("endorsement RPC failed")
		source := &fakeEndorsedProviderSource{fixture: serviceResolverFixture{endorsementsErr: want}}
		resolver := newResolver(t, source)
		_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want wrapped query failure", err)
		}
	})

	t.Run("allow unendorsed skips query", func(t *testing.T) {
		var calls atomic.Int32
		source := &fakeEndorsedProviderSource{fixture: serviceResolverFixture{endorsementCalls: &calls}}
		resolver := newResolver(t, source)
		contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
			Copies:                 1,
			AllowUnendorsedPrimary: true,
		})
		if err != nil || len(contexts) != 1 {
			t.Fatalf("ResolveUploadContexts = (%d, %v), want one context", len(contexts), err)
		}
		if calls.Load() != 0 {
			t.Fatalf("endorsement calls = %d, want 0", calls.Load())
		}
	})

	t.Run("SelectProviderContext skips query", func(t *testing.T) {
		var calls atomic.Int32
		source := &fakeEndorsedProviderSource{fixture: serviceResolverFixture{endorsementCalls: &calls}}
		resolver := newResolver(t, source)
		if _, err := resolver.SelectProviderContext(context.Background(), SelectProviderContextOptions{}); err != nil {
			t.Fatalf("SelectProviderContext: %v", err)
		}
		if calls.Load() != 0 {
			t.Fatalf("endorsement calls = %d, want 0", calls.Load())
		}
	})
}
