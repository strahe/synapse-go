package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
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

func bigInt(v int64) *big.Int {
	return big.NewInt(v)
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
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
				Metadata:         map[string]string{"source": "app", "withCDN": ""},
			},
			{
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(12), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(0),
				Metadata:         map[string]string{"source": "other"},
			},
			{
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(21), ProviderID: testID(2), PDPEndEpoch: 7},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
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
	if !got[0].ProviderID().Equal(testID(1)) || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(11)) {
		t.Fatalf("first context provider=%s dataset=%v want provider=1 dataset=11", got[0].ProviderID(), got[0].dataSetID)
	}
	if !got[1].ProviderID().Equal(testID(2)) {
		t.Fatalf("second context provider=%s want 2", got[1].ProviderID())
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
	if !got[0].ProviderID().Equal(testID(7)) || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(71)) {
		t.Fatalf("first context=%+v", got[0])
	}
	if !got[1].ProviderID().Equal(testID(8)) || got[1].dataSetID != nil {
		t.Fatalf("second context=%+v", got[1])
	}
}

func TestServiceResolverResolveWritableUploadContexts_ProviderIDsSkipFailedValidatorCandidate(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(7): ptrPDPProvider(testPDPProvider(testID(7), "https://sp-7.example.com")),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(71), ProviderID: testID(7), PDPEndEpoch: 0},
			{DataSetID: testID(72), ProviderID: testID(7), PDPEndEpoch: 0},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(71): {"source": "app"},
			testIDKey(72): {"source": "app"},
		},
		validatorEnabled: true,
		validatorErrByID: map[string]error{
			testIDKey(71): errors.New("not writable"),
		},
	})

	contexts, _, err := resolver.resolveWritableUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs:     []types.BigInt{testID(7)},
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("resolveWritableUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(72)) {
		t.Fatalf("context=%+v want provider 7 dataSetID 72", got)
	}
}

func TestServiceResolverResolveWritableUploadContexts_ProviderIDsCreateNewWhenValidatorFailsAll(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(7): ptrPDPProvider(testPDPProvider(testID(7), "https://sp-7.example.com")),
		},
		clientDataSets: []*warmstorage.DataSetInfo{
			{DataSetID: testID(71), ProviderID: testID(7), PDPEndEpoch: 0},
		},
		dataSetMetadata: map[string]map[string]string{
			testIDKey(71): {"source": "app"},
		},
		validatorEnabled: true,
		validatorErr:     errors.New("not writable"),
	})

	contexts, _, err := resolver.resolveWritableUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs:     []types.BigInt{testID(7)},
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("resolveWritableUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || got[0].dataSetID != nil {
		t.Fatalf("context=%+v want provider 7 with new data set", got)
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
	got := replacement.(*Context)
	if !got.ProviderID().Equal(testID(3)) {
		t.Fatalf("replacement provider=%s want 3", got.ProviderID())
	}
}

func TestServiceResolverResolveUploadContexts_RejectsNilContextFactoryResult(t *testing.T) {
	providerID := testID(7)
	resolver := newNilContextServiceResolver(t, serviceResolverFixture{
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(7): ptrPDPProvider(testPDPProvider(providerID, "https://sp-7.example.com")),
		},
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs: []types.BigInt{providerID},
	})
	if err == nil {
		t.Fatal("ResolveUploadContexts returned nil error; want nil context factory error")
	}
	if !strings.Contains(err.Error(), "nil context") {
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
	if !strings.Contains(err.Error(), "nil context") {
		t.Fatalf("err=%v want nil context message", err)
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

func TestServiceResolverResolveUploadContexts_RejectsProviderIDsWithDataSetIDs(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(33): {DataSetID: testID(33), ProviderID: testID(5), Payer: testPayer(), PDPEndEpoch: 0},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(5): ptrPDPProvider(testPDPProvider(testID(5), "https://sp-5.example.com")),
		},
		validatorEnabled: true,
	})

	_, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		ProviderIDs: []types.BigInt{testID(5)},
		DataSetIDs:  []types.BigInt{testID(33)},
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
}

func TestServiceResolverResolveUploadContexts_ExplicitDataSetIDsAllowEndedRailWhenValidatorPasses(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
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

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(55)},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(55)) {
		t.Fatalf("contexts=%+v want dataSetID 55", got)
	}
}

func TestServiceResolverResolveWritableUploadContexts_ExplicitDataSetIDsRequireValidator(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(55): {DataSetID: testID(55), ProviderID: testID(5), Payer: testPayer(), PDPEndEpoch: 0},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(5): ptrPDPProvider(testPDPProvider(testID(5), "https://sp-5.example.com")),
		},
	})

	_, _, err := resolver.resolveWritableUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(55)},
	})
	if err == nil || !strings.Contains(err.Error(), "validator") {
		t.Fatalf("err=%v want validator requirement", err)
	}
}

func TestServiceResolverResolveWritableUploadContexts_ExplicitDataSetIDsSurfaceValidatorFailure(t *testing.T) {
	want := errors.New("not live")
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(55): {DataSetID: testID(55), ProviderID: testID(5), Payer: testPayer(), PDPEndEpoch: 0},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(5): ptrPDPProvider(testPDPProvider(testID(5), "https://sp-5.example.com")),
		},
		validatorEnabled: true,
		validatorErr:     want,
	})

	_, _, err := resolver.resolveWritableUploadContexts(context.Background(), &UploadOptions{
		DataSetIDs: []types.BigInt{testID(55)},
	})
	if !errors.Is(err, want) {
		t.Fatalf("err=%v want %v", err, want)
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
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:           false,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
				Metadata:         map[string]string{"source": "app"},
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
	if got[0].dataSetID != nil {
		t.Fatalf("auto-select reused dataSetID=%v want nil", got[0].dataSetID)
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
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(2),
				Metadata:         map[string]string{"source": "app"},
			},
			{
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(12), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
				Metadata:         map[string]string{"source": "app"},
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
	if len(got) != 1 || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(11)) {
		t.Fatalf("context=%+v want dataSetID 11 from detailed snapshot", got)
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
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
				Metadata:         map[string]string{"source": "app"},
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

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if len(got) != 1 || got[0].dataSetID == nil || !got[0].dataSetID.Equal(testID(11)) {
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
	if got[0].dataSetID != nil {
		t.Fatalf("auto-select reused dataSetID=%v want nil without details", got[0].dataSetID)
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
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{
		Copies:          1,
		DataSetMetadata: map[string]string{"source": "app"},
	})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if got[0].dataSetID != nil {
		t.Fatalf("auto-select reused dataSetID=%v want nil when details are unavailable", got[0].dataSetID)
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
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
				Metadata:         map[string]string{},
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

func TestServiceResolverResolveUploadContexts_AutoSelectTreatsDetailEnrichmentFailureAsNoDetails(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
		},
		dataSetDetailsErr: errors.New("dataSetLive failed"),
	})

	contexts, _, err := resolver.ResolveUploadContexts(context.Background(), &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("ResolveUploadContexts: %v", err)
	}
	got := contextsToFake(t, contexts)
	if got[0].dataSetID != nil {
		t.Fatalf("auto-select reused dataSetID=%v want nil when details fail", got[0].dataSetID)
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
}

func newTestServiceResolver(t *testing.T, fixture serviceResolverFixture) *ServiceResolver {
	t.Helper()
	var catalog DataSetCatalog = &fakeDataSetCatalog{fixture: fixture}
	if fixture.validatorEnabled || fixture.detailedDataSets != nil || fixture.dataSetDetailsErr != nil {
		catalog = &fakeEnhancedDataSetCatalog{fakeDataSetCatalog: fakeDataSetCatalog{fixture: fixture}}
	}
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

func newNilContextServiceResolver(t *testing.T, fixture serviceResolverFixture) *ServiceResolver {
	t.Helper()
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:       testPayer(),
		SPRegistry:  &fakePDPProviderSource{fixture: fixture},
		WarmStorage: &fakeDataSetCatalog{fixture: fixture},
		NewContext: func(ResolvedUploadContext, *UploadOptions) (*Context, error) {
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
		if dataSet.ActivePieceCount != nil {
			cloned.ActivePieceCount = new(big.Int).Set(dataSet.ActivePieceCount)
		}
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

func contextsToFake(t *testing.T, contexts []UploadContext) []*Context {
	t.Helper()
	out := make([]*Context, 0, len(contexts))
	for _, ctx := range contexts {
		concrete, ok := ctx.(*Context)
		if !ok {
			t.Fatalf("unexpected context type %T", ctx)
		}
		out = append(out, concrete)
	}
	return out
}

func newResolvedTestContext(selection ResolvedUploadContext) (*Context, error) {
	opts := []ContextOption{WithDataSetMetadata(selection.DataSetMetadata)}
	if selection.DataSetID != nil {
		opts = append(opts, WithDataSetID(*selection.DataSetID))
	}
	if selection.ClientDataSetID != nil {
		opts = append(opts, WithClientDataSetID(*selection.ClientDataSetID))
	}
	return NewContext(selection.Provider, &fakePDPProviderClient{}, nil, opts...)
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
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(11), ProviderID: testID(1), PDPEndEpoch: 0, ClientDataSetID: clientDataSetID},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
				Metadata:         map[string]string{},
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
	if got[0].clientDataSetID == nil || !got[0].clientDataSetID.Equal(clientDataSetID) {
		t.Fatalf("clientDataSetID=%v want %s (ClientDataSetID not threaded through resolver)", got[0].clientDataSetID, clientDataSetID.String())
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

func TestServiceResolverSelectWritableReplacement_TrustsDetailedSnapshot(t *testing.T) {
	resolver := newTestServiceResolver(t, serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1), testID(2)},
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"),
			testPDPProvider(testID(2), "https://sp-2.example.com"),
		},
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{
				DataSetInfo:      &warmstorage.DataSetInfo{DataSetID: testID(21), ProviderID: testID(2), PDPEndEpoch: 0},
				IsLive:           true,
				IsManaged:        true,
				ActivePieceCount: bigInt(1),
				Metadata:         map[string]string{},
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
	got := replacement.(*Context)
	if !got.ProviderID().Equal(testID(2)) || got.dataSetID == nil || !got.dataSetID.Equal(testID(21)) {
		t.Fatalf("replacement=%+v want provider 2 dataSetID 21 from detailed snapshot", got)
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
	if !got[0].ProviderID().Equal(testID(999)) {
		t.Fatalf("provider=%s want 999", got[0].ProviderID())
	}
}
