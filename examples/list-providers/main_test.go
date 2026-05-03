package main

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
)

func TestParseConfigRejectsNegativePieceSize(t *testing.T) {
	_, err := parseConfig([]string{"--piece-size", "-1"})
	if err == nil || !strings.Contains(err.Error(), "piece-size") {
		t.Fatalf("err=%v want piece-size error", err)
	}
}

func TestRunListProvidersPrintsCapabilities(t *testing.T) {
	fake := &fakeProviderSelector{
		providers: []spregistry.PDPProvider{
			{
				Info: spregistry.ProviderInfo{
					ID:       types.NewBigInt(42),
					Name:     "Example SP",
					IsActive: true,
				},
				Product: spregistry.ServiceProduct{IsActive: true},
				Offering: spregistry.PDPOffering{
					ServiceURL:               "https://sp.example.com",
					MinPieceSizeInBytes:      big.NewInt(128),
					MaxPieceSizeInBytes:      big.NewInt(1 << 30),
					StoragePricePerTiBPerDay: big.NewInt(99),
					Location:                 "Earth",
				},
			},
		},
	}

	var stdout bytes.Buffer
	err := runListProviders(context.Background(), providerConfig{PieceSize: 1024}, fake, &stdout)
	if err != nil {
		t.Fatalf("runListProviders: %v", err)
	}
	if fake.filter.PieceSizeBytes == nil || fake.filter.PieceSizeBytes.Int64() != 1024 {
		t.Fatalf("filter=%+v", fake.filter)
	}
	out := stdout.String()
	for _, want := range []string{
		"providerCount=1",
		"provider.1.id=42",
		"provider.1.name=Example SP",
		"provider.1.serviceURL=https://sp.example.com",
		"provider.1.minPieceSize=128",
		"provider.1.productActive=true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

type fakeProviderSelector struct {
	filter    spregistry.ProviderFilter
	providers []spregistry.PDPProvider
}

func (f *fakeProviderSelector) SelectActivePDPProviders(_ context.Context, filter spregistry.ProviderFilter) ([]spregistry.PDPProvider, error) {
	f.filter = filter
	return f.providers, nil
}
