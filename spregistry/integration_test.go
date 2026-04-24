//go:build integration

package spregistry_test

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
)

// TestIntegration_SPRegistry exercises every read-only method on
// spregistry.Service against calibration. The six write methods require
// SP-operator authority and are permanently skipped here.
func TestIntegration_SPRegistry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := integrationtest.NewDefaultClient(t, ctx)
	reg := client.SPRegistry()

	// Address — trivially covered.
	if (reg.Address() == common.Address{}) {
		t.Fatal("spregistry.Address returned zero")
	}

	total, err := reg.GetProviderCount(ctx)
	if err != nil {
		t.Fatalf("GetProviderCount: %v", err)
	}
	if total == nil || total.Sign() <= 0 {
		t.Fatalf("GetProviderCount should be > 0, got %v", total)
	}
	active, err := reg.GetActiveProviderCount(ctx)
	if err != nil {
		t.Fatalf("GetActiveProviderCount: %v", err)
	}
	if active == nil || active.Sign() < 0 {
		t.Fatalf("GetActiveProviderCount should be >= 0, got %v", active)
	}
	if active.Cmp(total) > 0 {
		t.Errorf("active count %s > total %s", active, total)
	}
	t.Logf("providers: total=%s active=%s", total, active)

	// Paginated listing: first page of up to 10 active PDP providers.
	page, err := reg.GetPDPProviders(ctx, true, types.ListOptions{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("GetPDPProviders: %v", err)
	}
	if len(page.Providers) == 0 {
		t.Skip("no active PDP providers on calibration; cannot exercise per-provider reads")
	}
	first := page.Providers[0]
	if first.Info.ID == 0 {
		t.Fatalf("GetPDPProviders returned provider with zero ID")
	}

	// SelectActivePDPProviders (unbounded traversal).
	selected, err := reg.SelectActivePDPProviders(ctx, spregistry.ProviderFilter{})
	if err != nil {
		t.Fatalf("SelectActivePDPProviders: %v", err)
	}
	if len(selected) == 0 {
		t.Fatal("SelectActivePDPProviders returned empty despite GetPDPProviders seeing one")
	}
	// Sorted ascending by ID (documented invariant).
	for i := 1; i < len(selected); i++ {
		if selected[i-1].Info.ID >= selected[i].Info.ID {
			t.Errorf("SelectActivePDPProviders not sorted ascending: %d >= %d",
				selected[i-1].Info.ID, selected[i].Info.ID)
			break
		}
	}
	t.Logf("active PDP providers: page=%d select=%d", len(page.Providers), len(selected))

	// Per-provider reads on `first`.
	pdp, err := reg.GetPDPProvider(ctx, first.Info.ID)
	if err != nil {
		t.Fatalf("GetPDPProvider(%d): %v", first.Info.ID, err)
	}
	if pdp.Info.ServiceProvider != first.Info.ServiceProvider {
		t.Errorf("GetPDPProvider ServiceProvider mismatch")
	}

	info, err := reg.GetProvider(ctx, first.Info.ID)
	if err != nil {
		t.Fatalf("GetProvider(%d): %v", first.Info.ID, err)
	}
	if info.ServiceProvider != first.Info.ServiceProvider {
		t.Errorf("GetProvider ServiceProvider mismatch")
	}

	byAddr, err := reg.GetProviderByAddress(ctx, info.ServiceProvider)
	if err != nil {
		t.Fatalf("GetProviderByAddress(%s): %v", info.ServiceProvider, err)
	}
	if byAddr.ID != info.ID {
		t.Errorf("GetProviderByAddress ID mismatch: %d != %d", byAddr.ID, info.ID)
	}

	idByAddr, err := reg.GetProviderIDByAddress(ctx, info.ServiceProvider)
	if err != nil {
		t.Fatalf("GetProviderIDByAddress: %v", err)
	}
	if idByAddr != info.ID {
		t.Errorf("GetProviderIDByAddress mismatch: %d != %d", idByAddr, info.ID)
	}

	isActive, err := reg.IsProviderActive(ctx, info.ID)
	if err != nil {
		t.Fatalf("IsProviderActive: %v", err)
	}
	if !isActive {
		t.Errorf("IsProviderActive should be true for selected active provider %d", info.ID)
	}

	// Zero-address returns zero providerID (not an error for 0 input? yes — zero
	// address is rejected; use a random-but-unregistered address to assert the
	// contract-convention zero result path).
	unreg, err := reg.GetProviderIDByAddress(ctx, common.HexToAddress("0x1111111111111111111111111111111111111111"))
	if err != nil {
		t.Fatalf("GetProviderIDByAddress(unreg): %v", err)
	}
	if unreg != 0 {
		t.Errorf("GetProviderIDByAddress(unreg) = %d, want 0", unreg)
	}

	// Batch lookup including one valid ID and one sentinel invalid. Base
	// the invalid ID on the live total so we don't assume calibration has
	// fewer than 2^30 registrations.
	bogusID := types.ProviderID(total.Uint64() + 1_000_000)
	batch, err := reg.GetProvidersByIDs(ctx, []types.ProviderID{info.ID, bogusID})
	if err != nil {
		t.Fatalf("GetProvidersByIDs: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("GetProvidersByIDs len = %d, want 2", len(batch))
	}
	if batch[0] == nil || batch[0].ID != info.ID {
		t.Errorf("GetProvidersByIDs[0] ID mismatch: %+v", batch[0])
	}
	if batch[1] != nil {
		t.Errorf("GetProvidersByIDs[1] should be nil for invalid ID, got %+v", batch[1])
	}

	// IterateAllPDPProviders — stop after the first to bound duration.
	var iterCount int
	for p, err := range reg.IterateAllPDPProviders(ctx, true) {
		if err != nil {
			t.Fatalf("IterateAllPDPProviders: %v", err)
		}
		if p.Info.ID == 0 {
			t.Fatal("iterator yielded zero-ID provider")
		}
		iterCount++
		if iterCount >= 3 {
			break
		}
	}
	if iterCount == 0 {
		t.Fatal("IterateAllPDPProviders yielded zero providers")
	}

	// The six write methods are permanently skipped — they require
	// SP-operator authority the SDK's test account does not hold. Each
	// t.Run records the skip reason so the integration log makes the
	// coverage declaration explicit.
	writeMethods := []string{
		"RegisterProvider", "UpdateProviderInfo", "RemoveProvider",
		"AddPDPProduct", "UpdatePDPProduct", "RemoveProduct",
	}
	for _, m := range writeMethods {
		t.Run(m, func(t *testing.T) {
			t.Skip("needs-sp-owner")
		})
	}
}
