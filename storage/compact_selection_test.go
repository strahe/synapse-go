package storage

import (
	"context"
	"testing"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestCompactSelectionAcrossCatalogs(t *testing.T) {
	for _, test := range []struct {
		name          string
		limit         uint64
		mismatch      bool
		compactActive bool
		want          uint64
	}{
		{"empty compact beats active legacy", 100, false, false, 100},
		{"active compact beats older empty compact", 100, false, true, 101},
		{"legacy fallback", 100, true, false, 99},
		{"unknown layout retains activity preference", 0, false, false, 99},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := activitySelectionFixture(testID(1), []types.BigInt{testID(99), testID(100), testID(101)})
			fixture.clientDataSets[0].ClientDataSetID = testID(199)
			fixture.clientDataSets[1].ClientDataSetID = testID(200)
			fixture.clientDataSets[2].ClientDataSetID = testID(201)
			if test.mismatch {
				fixture.dataSetMetadata[testIDKey(100)] = map[string]string{"source": "other"}
				fixture.dataSetMetadata[testIDKey(101)] = map[string]string{"source": "other"}
			}
			catalog := newActivityDataSetCatalog(fixture)
			catalog.activeByID[testIDKey(99)] = true
			catalog.activeByID[testIDKey(101)] = test.compactActive
			resolver := newActivityServiceResolver(t, catalog, nil)
			resolver.legacyPieceStorageIDLimit = test.limit
			id, _, _, err := resolver.selectMatchingDataSetWithWritable(context.Background(), testID(1), fixture.clientDataSets, map[string]string{"source": "app"}, true)
			if err != nil || id == nil || !id.Equal(testID(test.want)) {
				t.Fatalf("basic id=%v err=%v want=%d", id, err, test.want)
			}
			if test.limit != 0 && !test.mismatch {
				for _, called := range catalog.activityCallIDs() {
					if called.Equal(testID(99)) {
						t.Fatal("legacy activity read despite valid compact match")
					}
				}
			}
			detailed := make([]*warmstorage.EnhancedDataSetInfo, len(fixture.clientDataSets))
			for i, dataSet := range fixture.clientDataSets {
				detailed[i] = &warmstorage.EnhancedDataSetInfo{DataSetInfo: *dataSet, IsLive: true, IsManaged: true, HasActivePieces: catalog.activeByID[idconv.Key(dataSet.DataSetID)], Metadata: fixture.dataSetMetadata[idconv.Key(dataSet.DataSetID)]}
			}
			id, _, _ = selectMatchingDetailedDataSet(testID(1), detailed, map[string]string{"source": "app"}, test.limit)
			if id == nil || !id.Equal(testID(test.want)) {
				t.Fatalf("detailed id=%v want=%d", id, test.want)
			}
			// The metadata-only catalog retains ID ordering inside each layout.
			resolver.dataSetActivity = nil
			id, _, _, err = resolver.selectMatchingDataSetWithWritable(context.Background(), testID(1), fixture.clientDataSets, map[string]string{"source": "app"}, true)
			metadataOnlyWant := test.want
			if test.compactActive {
				metadataOnlyWant = 100
			}
			if err != nil || id == nil || !id.Equal(testID(metadataOnlyWant)) {
				t.Fatalf("metadata-only id=%v err=%v want=%d", id, err, metadataOnlyWant)
			}
		})
	}
}

func TestCompactProviderPriorityPreservesQualification(t *testing.T) {
	fixture := serviceResolverFixture{
		approvedProviderIDs: []types.BigInt{testID(1), testID(2), testID(3), testID(4)},
		endorsedProviderIDs: []types.BigInt{testID(2)}, endorsementsSet: true,
		activeProviders: []spregistry.PDPProvider{
			testPDPProvider(testID(1), "https://sp-1.example.com"), testPDPProvider(testID(2), "https://sp-2.example.com"),
			testPDPProvider(testID(3), "https://sp-3.example.com"), testPDPProvider(testID(4), "https://sp-4.example.com"),
		},
		detailedDataSets: []*warmstorage.EnhancedDataSetInfo{
			{DataSetInfo: warmstorage.DataSetInfo{DataSetID: testID(99), ProviderID: testID(2)}, IsLive: true, IsManaged: true, HasActivePieces: true},
			{DataSetInfo: warmstorage.DataSetInfo{DataSetID: testID(100), ProviderID: testID(3)}, IsLive: true, IsManaged: true},
			{DataSetInfo: warmstorage.DataSetInfo{DataSetID: testID(101), ProviderID: testID(4)}, IsLive: true, IsManaged: true},
		},
	}
	for _, test := range []struct {
		name     string
		endorsed bool
		exclude  []types.BigInt
		want     []uint64
	}{
		{"compact first and stable provider order", false, nil, []uint64{3, 4, 2, 1}},
		{"endorsed legacy primary", true, nil, []uint64{2, 3, 4, 1}},
		{"excluded compact", false, []types.BigInt{testID(3)}, []uint64{4, 2, 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			resolver := newTestServiceResolver(t, fixture)
			resolver.legacyPieceStorageIDLimit = 100
			contexts, err := resolver.ResolveUploadContexts(context.Background(), SelectUploadContextsOptions{Copies: len(test.want), AllowUnendorsedPrimary: !test.endorsed, ExcludeProviderIDs: test.exclude})
			if err != nil {
				t.Fatal(err)
			}
			if len(contexts) != len(test.want) {
				t.Fatalf("contexts=%d want=%d", len(contexts), len(test.want))
			}
			for i, target := range contexts {
				if !target.ProviderID().Equal(testID(test.want[i])) {
					t.Fatalf("provider[%d]=%s want=%d", i, target.ProviderID(), test.want[i])
				}
				if target.legacyPieceStorageLimit() != 100 {
					t.Fatalf("factory cutoff=%d", target.legacyPieceStorageLimit())
				}
			}
		})
	}
}
