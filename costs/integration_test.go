//go:build integration

package costs_test

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/internal/integrationtest"
)

// TestIntegration_Costs directly covers the read-only costs.Service surface.
// Stateful upload-cost composition remains covered by tests/integration.
func TestIntegration_Costs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := integrationtest.NewDefaultClient(t, ctx)
	c := client.Costs()

	// GetServicePrice: must agree with warmstorage.GetServicePrice for the
	// same block (they share the same view binding under the hood).
	p1, err := c.GetServicePrice(ctx)
	if err != nil {
		t.Fatalf("costs.GetServicePrice: %v", err)
	}
	p2, err := client.WarmStorage().GetServicePrice(ctx)
	if err != nil {
		t.Fatalf("warmstorage.GetServicePrice: %v", err)
	}
	if p1.PricePerTiBPerMonthNoCDN.Cmp(p2.PricePerTiBPerMonthNoCDN) != 0 {
		t.Errorf("GetServicePrice divergence: costs=%v warmstorage=%v",
			p1.PricePerTiBPerMonthNoCDN, p2.PricePerTiBPerMonthNoCDN)
	}

	priceList, err := c.GetPriceList(ctx)
	if err != nil {
		t.Fatalf("costs.GetPriceList: %v", err)
	}
	if priceList == nil {
		t.Fatal("costs.GetPriceList returned nil")
	}
	warmStoragePriceList, err := client.WarmStorage().GetPriceList(ctx)
	if err != nil {
		t.Fatalf("warmstorage.GetPriceList: %v", err)
	}
	if warmStoragePriceList == nil {
		t.Fatal("warmstorage.GetPriceList returned nil")
	}
	if priceList.Token != warmStoragePriceList.Token ||
		priceList.Rates.StoragePerTiBPerMonth.Cmp(warmStoragePriceList.Rates.StoragePerTiBPerMonth) != 0 ||
		priceList.Fees.CreateDataSetFee.Cmp(warmStoragePriceList.Fees.CreateDataSetFee) != 0 {
		t.Fatalf("GetPriceList divergence: costs=%+v warmstorage=%+v", priceList, warmStoragePriceList)
	}

	summary, err := c.GetAccountSummary(ctx, client.Address())
	if err != nil {
		t.Fatalf("costs.GetAccountSummary: %v", err)
	}
	if summary == nil || summary.Funds == nil || summary.AvailableFunds == nil || summary.Debt == nil ||
		summary.CurrentEpoch == nil || summary.CurrentEpoch.Sign() <= 0 {
		t.Fatalf("GetAccountSummary returned incomplete values: %+v", summary)
	}

	// CalculateMultiContextCosts across two prospective contexts (one new,
	// one incremental).
	dataSize := big.NewInt(256 * 1024)
	refs := []costs.MultiContextRef{
		{IsNewDataSet: true, WithCDN: false},
		{IsNewDataSet: false, CurrentDataSetSizeBytes: big.NewInt(1 << 20)},
	}
	multi, err := c.CalculateMultiContextCosts(ctx, client.Address(), dataSize, refs, nil)
	if err != nil {
		t.Fatalf("CalculateMultiContextCosts: %v", err)
	}
	if multi.RatePerEpoch == nil || multi.RatePerEpoch.Sign() <= 0 {
		t.Errorf("RatePerEpoch should be > 0, got %v", multi.RatePerEpoch)
	}
	if multi.RatePerMonth == nil || multi.RatePerMonth.Sign() <= 0 {
		t.Errorf("RatePerMonth should be > 0, got %v", multi.RatePerMonth)
	}
	if multi.DepositNeeded == nil || multi.DepositNeeded.Sign() < 0 {
		t.Errorf("DepositNeeded should be >= 0, got %v", multi.DepositNeeded)
	}
	t.Logf("multi: rate/epoch=%s rate/month=%s deposit=%s ready=%v needsApproval=%v",
		multi.RatePerEpoch, multi.RatePerMonth, multi.DepositNeeded, multi.Ready, multi.NeedsFWSSMaxApproval)
}
