package adapters

import (
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestBuildServiceParameters_IncludesUploadSizeBounds(t *testing.T) {
	got := buildServiceParameters(&warmstorage.ServicePrice{
		EpochsPerMonth: big.NewInt(2880),
	})

	if got.MinUploadSize != chain.MinUploadSize {
		t.Fatalf("MinUploadSize=%d want %d", got.MinUploadSize, chain.MinUploadSize)
	}
	if got.MaxUploadSize != chain.MaxUploadSize {
		t.Fatalf("MaxUploadSize=%d want %d", got.MaxUploadSize, chain.MaxUploadSize)
	}
}
