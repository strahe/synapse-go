package adapters

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestBuildServiceParameters_UsesChainGeometryDefaults(t *testing.T) {
	got := buildServiceParameters()

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

func TestBuildPricingInfo_UsesPriceList(t *testing.T) {
	priceList := &warmstorage.PriceList{
		Token: common.HexToAddress("0xbeef"),
		Rates: warmstorage.PriceListRates{
			StoragePerTiBPerMonth: big.NewInt(chain.EpochsPerMonth),
		},
		Fees: warmstorage.PriceListFees{
			CreateDataSetFee: big.NewInt(11),
		},
	}

	got := buildPricingInfo(priceList)

	if got.PriceList != priceList {
		t.Fatal("PriceList was not preserved")
	}
	if got.TokenAddress != priceList.Token {
		t.Fatalf("TokenAddress=%s want %s", got.TokenAddress, priceList.Token)
	}
	if got.NoCDN.PerMonth.Cmp(big.NewInt(chain.EpochsPerMonth)) != 0 {
		t.Fatalf("NoCDN.PerMonth=%s want %d", got.NoCDN.PerMonth, chain.EpochsPerMonth)
	}
	if got.NoCDN.PerEpoch.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("NoCDN.PerEpoch=%s want 1", got.NoCDN.PerEpoch)
	}
}

func TestApprovalLockupPeriodReader_UsesPriceListDefault(t *testing.T) {
	source := &fakePriceListSource{priceList: &warmstorage.PriceList{
		Lockups: warmstorage.PriceListLockups{
			DefaultLockupPeriod: big.NewInt(321),
		},
	}}
	reader := NewApprovalLockupPeriodReader(source)

	got, err := reader.ApprovalLockupPeriod(context.Background())
	if err != nil {
		t.Fatalf("ApprovalLockupPeriod: %v", err)
	}
	if got.Cmp(big.NewInt(321)) != 0 {
		t.Fatalf("ApprovalLockupPeriod=%s want 321", got)
	}
	got.SetInt64(1)
	if source.priceList.Lockups.DefaultLockupPeriod.Cmp(big.NewInt(321)) != 0 {
		t.Fatal("ApprovalLockupPeriod returned aliased PriceList value")
	}
}

type fakePriceListSource struct {
	priceList *warmstorage.PriceList
}

func (f *fakePriceListSource) GetPriceList(context.Context) (*warmstorage.PriceList, error) {
	return f.priceList, nil
}
