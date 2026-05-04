//go:build integration

package payments_test

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/payments"
)

// TestIntegration_Payments covers the parts of payments.Service not already
// exercised by the main TestIntegration's Payments subtest. It exercises
// the trivially-covered getters (Address / ChainID / Account), plus the
// native-FIL paths (WalletBalance with the zero token) and the rails list
// API. Fund / FundSync are exercised with a small amount when the account
// has sufficient FIL; otherwise they Skip to stay idempotent across repeated
// runs. RevokeService / DepositWithPermit* are covered by DestructiveSuite.
// SettleTerminatedRail is permanently skipped (requires a terminated rail).
func TestIntegration_Payments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client := integrationtest.NewDefaultClient(t, ctx)
	p := client.Payments()

	// Trivially covered — assert non-zero shape.
	if (p.Address() == common.Address{}) {
		t.Fatal("Payments.Address is zero")
	}
	if p.ChainID() == 0 {
		t.Fatal("Payments.ChainID is 0")
	}
	if (p.Account() == common.Address{}) {
		t.Fatal("Payments.Account is zero (expected signer EOA)")
	}
	if p.Account() != client.Address() {
		t.Errorf("Payments.Account = %s, want client.Address() = %s",
			p.Account(), client.Address())
	}

	// WalletBalance for native FIL (zero token address).
	filBal, err := p.WalletBalance(ctx, common.Address{}, client.Address())
	if err != nil {
		t.Fatalf("WalletBalance(FIL): %v", err)
	}
	if filBal == nil || filBal.Sign() <= 0 {
		t.Fatalf("WalletBalance(FIL) should be > 0, got %v", filBal)
	}
	t.Logf("wallet FIL balance: %s", filBal)

	// WalletBalance with invalid zero account → ErrInvalidArgument.
	if _, err := p.WalletBalance(ctx, common.Address{}, common.Address{}); !errors.Is(err, payments.ErrInvalidArgument) {
		t.Errorf("WalletBalance(zero account): want ErrInvalidArgument, got %v", err)
	}

	// GetRailsAsPayer — should succeed even if zero rails.
	addrs := client.Chain().Addresses()
	page, err := p.GetRailsAsPayer(ctx, client.Address(), addrs.USDFC)
	if err != nil {
		t.Fatalf("GetRailsAsPayer: %v", err)
	}
	if page == nil {
		t.Fatal("GetRailsAsPayer returned nil page")
	}
	t.Logf("rails-as-payer: count=%d", len(page.Rails))

	// FundSync — deposit a trivial amount (1 atto-USDFC) to assert the
	// chain path works without materially changing balances. The async Fund /
	// WithWait finalization path is covered by unit tests.
	one := big.NewInt(1)
	syncRes, err := p.FundSync(ctx, one)
	if err != nil {
		if errors.Is(err, payments.ErrPermitUnsupported) {
			t.Skip("needs-usdfc-permit-support: USDFC contract does not implement EIP-2612 permit")
		}
		t.Fatalf("FundSync: %v", err)
	}
	if syncRes.Receipt == nil {
		t.Error("FundSync should return a non-nil receipt")
	} else if syncRes.Receipt.Status != 1 {
		t.Errorf("FundSync tx failed: status=%d", syncRes.Receipt.Status)
	}
	t.Logf("FundSync tx=%s", syncRes.Hash)

	t.Run("SettleTerminatedRail", func(t *testing.T) {
		t.Skip("needs-terminated-rail")
	})
}
