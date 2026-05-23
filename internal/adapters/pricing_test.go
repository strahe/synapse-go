package adapters

import (
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestBuildServiceParameters_UsesChainGeometryDefaults(t *testing.T) {
	got := buildServiceParameters(nil)

	if got.MinUploadSize != chain.MinUploadSize {
		t.Errorf("MinUploadSize=%d want %d", got.MinUploadSize, chain.MinUploadSize)
	}
	if got.MaxUploadSize != chain.MaxUploadSize {
		t.Errorf("MaxUploadSize=%d want %d", got.MaxUploadSize, chain.MaxUploadSize)
	}
	if got.EpochsPerMonth != chain.EpochsPerMonth {
		t.Errorf("EpochsPerMonth=%d want %d", got.EpochsPerMonth, chain.EpochsPerMonth)
	}
	if got.EpochsPerDay != chain.EpochsPerDay {
		t.Errorf("EpochsPerDay=%d want %d", got.EpochsPerDay, chain.EpochsPerDay)
	}
}

func TestBuildServiceParameters_UsesPositivePricingMonthWithoutDerivingDay(t *testing.T) {
	epochsPerMonth := chain.EpochsPerMonth + chain.EpochsPerDay
	got := buildServiceParameters(&warmstorage.ServicePrice{
		EpochsPerMonth: big.NewInt(epochsPerMonth),
	})

	if got.EpochsPerMonth != epochsPerMonth {
		t.Errorf("EpochsPerMonth=%d want %d", got.EpochsPerMonth, epochsPerMonth)
	}
	if got.EpochsPerDay != chain.EpochsPerDay {
		t.Errorf("EpochsPerDay=%d want %d", got.EpochsPerDay, chain.EpochsPerDay)
	}
}

func TestBuildServiceParameters_IgnoresInvalidPricingMonth(t *testing.T) {
	for _, tt := range []struct {
		name  string
		month *big.Int
	}{
		{name: "nil"},
		{name: "zero", month: new(big.Int)},
		{name: "negative", month: big.NewInt(-1)},
		{name: "overflow", month: new(big.Int).Lsh(big.NewInt(1), 63)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := buildServiceParameters(&warmstorage.ServicePrice{
				EpochsPerMonth: tt.month,
			})

			if got.EpochsPerMonth != chain.EpochsPerMonth {
				t.Errorf("EpochsPerMonth=%d want %d", got.EpochsPerMonth, chain.EpochsPerMonth)
			}
			if got.EpochsPerDay != chain.EpochsPerDay {
				t.Errorf("EpochsPerDay=%d want %d", got.EpochsPerDay, chain.EpochsPerDay)
			}
		})
	}
}
