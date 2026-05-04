//go:build integration

package warmstorage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// TestIntegration_WarmStorage covers single-package read-only surface:
// pricing, PDP config, owner/approval registry and paginated client dataset
// listings. Methods requiring an uploaded dataset (GetDataSet,
// GetActivePieceCount, GetPieceMetadata, GetAllPieceMetadata,
// GetAllDataSetMetadata, ValidateDataSet) are covered by the cross-package
// WarmStorageInspection subtest in tests/integration. TopUpCDNPaymentRails
// requires a rail-with-debt and is skipped.
func TestIntegration_WarmStorage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client := integrationtest.NewDefaultClient(t, ctx)
	ws := client.WarmStorage()

	// Address getters are trivially covered; still exercise them so the
	// matrix declaration is observable in the test log.
	if (ws.FWSSAddress() == common.Address{}) {
		t.Fatal("FWSSAddress is zero")
	}
	if (ws.ViewAddress() == common.Address{}) {
		t.Fatal("ViewAddress is zero")
	}
	if (ws.PDPVerifierAddress() == common.Address{}) {
		t.Fatal("PDPVerifierAddress is zero on calibration")
	}

	price, err := ws.GetServicePrice(ctx)
	if err != nil {
		t.Fatalf("GetServicePrice: %v", err)
	}
	if price.PricePerTiBPerMonthNoCDN == nil || price.PricePerTiBPerMonthNoCDN.Sign() <= 0 {
		t.Errorf("PricePerTiBPerMonthNoCDN should be > 0, got %v", price.PricePerTiBPerMonthNoCDN)
	}
	if price.EpochsPerMonth == nil || price.EpochsPerMonth.Sign() <= 0 {
		t.Errorf("EpochsPerMonth should be > 0, got %v", price.EpochsPerMonth)
	}
	if (price.TokenAddress == common.Address{}) {
		t.Error("TokenAddress is zero")
	}

	cfg, err := ws.GetPDPConfig(ctx)
	if err != nil {
		t.Fatalf("GetPDPConfig: %v", err)
	}
	if cfg.MaxProvingPeriod == 0 {
		t.Error("MaxProvingPeriod is 0")
	}
	if cfg.ChallengeWindowSize == nil || cfg.ChallengeWindowSize.Sign() <= 0 {
		t.Errorf("ChallengeWindowSize should be > 0, got %v", cfg.ChallengeWindowSize)
	}

	owner, err := ws.GetOwner(ctx)
	if err != nil {
		t.Fatalf("GetOwner: %v", err)
	}
	if (owner == common.Address{}) {
		t.Error("GetOwner returned zero address")
	}
	isOwner, err := ws.IsOwner(ctx, owner)
	if err != nil {
		t.Fatalf("IsOwner(owner): %v", err)
	}
	if !isOwner {
		t.Error("IsOwner(owner) should be true")
	}
	isOwner, err = ws.IsOwner(ctx, common.HexToAddress("0x1111111111111111111111111111111111111111"))
	if err != nil {
		t.Fatalf("IsOwner(other): %v", err)
	}
	if isOwner {
		t.Error("IsOwner(other) should be false")
	}

	// Approved provider registry.
	total, err := ws.GetApprovedProvidersLength(ctx)
	if err != nil {
		t.Fatalf("GetApprovedProvidersLength: %v", err)
	}
	if total == nil || total.Sign() < 0 {
		t.Fatalf("GetApprovedProvidersLength should be >= 0, got %v", total)
	}
	t.Logf("approved providers length=%s", total)

	ids, err := ws.GetApprovedProviderIDs(ctx, types.ListOptions{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("GetApprovedProviderIDs: %v", err)
	}
	if total.Sign() > 0 && len(ids) == 0 {
		t.Error("GetApprovedProviderIDs returned empty but length > 0")
	}

	// IterateAllApprovedProviderIDs — take up to 3.
	var iterCount int
	for id, err := range ws.IterateAllApprovedProviderIDs(ctx) {
		if err != nil {
			t.Fatalf("IterateAllApprovedProviderIDs: %v", err)
		}
		if id.IsZero() {
			t.Error("iterator yielded zero providerID")
		}
		iterCount++
		if iterCount >= 3 {
			break
		}
	}

	if len(ids) > 0 {
		approved, err := ws.IsProviderApproved(ctx, ids[0])
		if err != nil {
			t.Fatalf("IsProviderApproved: %v", err)
		}
		if !approved {
			t.Errorf("IsProviderApproved(%s) = false, want true", ids[0])
		}
	}

	// Client-side dataset listing. We use the test account; it may or may
	// not have datasets depending on prior runs. Assert structural
	// invariants, not counts.
	payer := client.Address()
	dsLen, err := ws.GetClientDataSetsLength(ctx, payer)
	if err != nil {
		t.Fatalf("GetClientDataSetsLength: %v", err)
	}
	if dsLen == nil || dsLen.Sign() < 0 {
		t.Fatalf("GetClientDataSetsLength returned %v", dsLen)
	}

	dsIDs, err := ws.GetClientDataSetIds(ctx, payer, types.ListOptions{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("GetClientDataSetIds: %v", err)
	}
	dsInfos, err := ws.GetClientDataSets(ctx, payer, types.ListOptions{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatalf("GetClientDataSets: %v", err)
	}
	if len(dsIDs) != len(dsInfos) {
		t.Errorf("GetClientDataSetIds (%d) and GetClientDataSets (%d) disagree", len(dsIDs), len(dsInfos))
	}
	for i, info := range dsInfos {
		if info == nil {
			t.Fatalf("GetClientDataSets[%d] nil", i)
		}
		if info.Payer != payer {
			t.Errorf("GetClientDataSets[%d].Payer = %s, want %s", i, info.Payer, payer)
		}
	}

	withDetails, err := ws.GetClientDataSetsWithDetails(ctx, payer, true)
	if err != nil {
		t.Fatalf("GetClientDataSetsWithDetails: %v", err)
	}
	// onlyManaged=true means every returned row must be managed by this FWSS.
	if len(withDetails) > int(dsLen.Int64()) {
		t.Errorf("withDetails count %d > total %s", len(withDetails), dsLen)
	}

	// Iterators — consume up to 3 entries each.
	iterCount = 0
	for info, err := range ws.IterateAllClientDataSets(ctx, payer) {
		if err != nil {
			t.Fatalf("IterateAllClientDataSets: %v", err)
		}
		if info == nil {
			t.Error("iterator yielded nil info")
		}
		iterCount++
		if iterCount >= 3 {
			break
		}
	}
	iterCount = 0
	for id, err := range ws.IterateAllClientDataSetIds(ctx, payer) {
		if err != nil {
			t.Fatalf("IterateAllClientDataSetIds: %v", err)
		}
		if id.IsZero() {
			t.Error("iterator yielded zero DataSetID")
		}
		iterCount++
		if iterCount >= 3 {
			break
		}
	}

	// ErrInvalidArgument paths.
	if _, err := ws.GetDataSet(ctx, types.NewBigInt(0)); !errors.Is(err, warmstorage.ErrInvalidArgument) {
		t.Errorf("GetDataSet(0): want ErrInvalidArgument, got %v", err)
	}
	if _, err := ws.GetClientDataSets(ctx, common.Address{}, types.ListOptions{Limit: 1}); !errors.Is(err, warmstorage.ErrInvalidArgument) {
		t.Errorf("GetClientDataSets(zero-addr): want ErrInvalidArgument, got %v", err)
	}

	t.Run("TopUpCDNPaymentRails", func(t *testing.T) {
		t.Skip("needs-cdn-rail-debt")
	})
}
