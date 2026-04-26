package main

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestRunListPrintsStorageInfoAndDatasets(t *testing.T) {
	payer := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeDatasetReader{
		info: &storage.StorageInfo{
			Pricing: storage.PricingInfo{
				NoCDN:   storage.PricePerTiB{PerMonth: big.NewInt(100)},
				WithCDN: storage.PricePerTiB{PerMonth: big.NewInt(200)},
			},
			Providers: []spregistry.PDPProvider{
				{Info: spregistry.ProviderInfo{ID: types.ProviderID(1)}},
			},
			Allowances: &storage.Allowances{
				IsApproved:      true,
				RateAllowance:   big.NewInt(300),
				LockupAllowance: big.NewInt(400),
			},
		},
		dataSets: []*storage.DataSetInfo{
			{
				DataSetInfo: &warmstorage.DataSetInfo{
					DataSetID:  types.DataSetID(10),
					ProviderID: types.ProviderID(20),
					Payer:      payer,
				},
				IsLive:           true,
				IsManaged:        true,
				WithCDN:          true,
				ActivePieceCount: big.NewInt(3),
				Metadata: map[string]string{
					"source": "example",
				},
			},
		},
	}

	var stdout bytes.Buffer
	err := runList(context.Background(), listConfig{OnlyManaged: true}, fake, &stdout)
	if err != nil {
		t.Fatalf("runList: %v", err)
	}
	if !fake.onlyManaged {
		t.Fatal("OnlyManaged=false want true")
	}
	out := stdout.String()
	for _, want := range []string{
		"providerCount=1",
		"pricing.noCDN.perMonth=100",
		"allowances.approved=true",
		"datasetCount=1",
		"dataset.1.dataSetID=10",
		"dataset.1.providerID=20",
		"dataset.1.metadata.source=example",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestRunListFiltersByDataSetID(t *testing.T) {
	fake := &fakeDatasetReader{
		info: &storage.StorageInfo{},
		dataSets: []*storage.DataSetInfo{
			{DataSetInfo: &warmstorage.DataSetInfo{DataSetID: types.DataSetID(1)}},
			{DataSetInfo: &warmstorage.DataSetInfo{DataSetID: types.DataSetID(2)}},
		},
	}
	var stdout bytes.Buffer
	if err := runList(context.Background(), listConfig{DataSetID: 2}, fake, &stdout); err != nil {
		t.Fatalf("runList: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "datasetCount=1") || !strings.Contains(out, "dataset.1.dataSetID=2") {
		t.Fatalf("output=%s", out)
	}
}

type fakeDatasetReader struct {
	info        *storage.StorageInfo
	dataSets    []*storage.DataSetInfo
	onlyManaged bool
}

func (f *fakeDatasetReader) FindDataSets(_ context.Context, opts *storage.FindDataSetsOptions) ([]*storage.DataSetInfo, error) {
	if opts != nil {
		f.onlyManaged = opts.OnlyManaged
	}
	return f.dataSets, nil
}

func (f *fakeDatasetReader) GetStorageInfo(context.Context, *storage.GetStorageInfoOptions) (*storage.StorageInfo, error) {
	return f.info, nil
}
