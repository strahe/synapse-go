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

// TestIntegration_Payments covers payment reads and account writes not already
// exercised by the cross-package suite. FundSync is conditional on token
// permit support. Withdraw uses one atto-USDFC and immediately deposits it
// back; terminated-rail settlement is covered by the staged storage flow.
func TestIntegration_Payments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
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
	addrs := client.ResolvedAddresses()
	page, err := p.GetRailsAsPayer(ctx, client.Address(), addrs.USDFC)
	if err != nil {
		t.Fatalf("GetRailsAsPayer: %v", err)
	}
	if page == nil {
		t.Fatal("GetRailsAsPayer returned nil page")
	}
	t.Logf("rails-as-payer: count=%d", len(page.Rails))

	fixed, err := p.TotalAccountFixedLockup(ctx, client.Address())
	if err != nil {
		t.Fatalf("TotalAccountFixedLockup: %v", err)
	}
	if fixed == nil || fixed.Sign() < 0 {
		t.Fatalf("TotalAccountFixedLockup returned %v, want >= 0", fixed)
	}
	t.Logf("total account fixed lockup: %s", fixed)

	t.Run("FundSync", func(t *testing.T) {
		// Deposit a trivial amount to assert the permit-based chain path.
		one := big.NewInt(1)
		syncRes, err := p.FundSync(ctx, one)
		if err != nil {
			if errors.Is(err, payments.ErrPermitUnsupported) {
				t.Skip("needs-usdfc-permit-support: USDFC contract does not implement EIP-2612 permit")
			}
			t.Fatalf("FundSync: %v", err)
		}
		if syncRes == nil || syncRes.Receipt == nil {
			t.Fatal("FundSync should return a non-nil receipt")
		}
		if syncRes.Receipt.Status != 1 {
			t.Fatalf("FundSync tx failed: status=%d", syncRes.Receipt.Status)
		}
		t.Logf("FundSync tx=%s", syncRes.Hash)
	})

	t.Run("WithdrawAndRestore", func(t *testing.T) {
		one := big.NewInt(1)
		before, err := p.AccountInfo(ctx, addrs.USDFC, client.Address())
		if err != nil {
			t.Fatalf("AccountInfo(before): %v", err)
		}
		if before == nil {
			t.Fatal("AccountInfo(before) returned nil")
		}
		if available := before.AvailableFunds(); available == nil || available.Cmp(one) < 0 {
			t.Fatalf("available funds %v are insufficient for a one atto-USDFC withdrawal", available)
		}
		beforeWallet, err := p.WalletBalance(ctx, addrs.USDFC, client.Address())
		if err != nil {
			t.Fatalf("WalletBalance(before): %v", err)
		}
		allowance, err := p.Allowance(ctx, addrs.USDFC, client.Address(), addrs.Payments)
		if err != nil {
			t.Fatalf("Allowance(before withdrawal): %v", err)
		}
		if allowance == nil {
			t.Fatal("Allowance(before withdrawal) returned nil")
		}
		if allowance.Cmp(one) < 0 {
			approve, err := p.Approve(ctx, addrs.USDFC, addrs.Payments, one, payments.WithWait(90*time.Second))
			if err != nil {
				t.Fatalf("Approve(restore amount): %v", err)
			}
			if approve == nil || approve.Receipt == nil || approve.Receipt.Status != 1 {
				t.Fatalf("Approve receipt = %+v", approve.Receipt)
			}
		}

		withdrawn := false
		restored := false
		t.Cleanup(func() {
			if !withdrawn || restored {
				return
			}
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cleanupCancel()
			if _, err := p.Deposit(cleanupCtx, addrs.USDFC, client.Address(), one, payments.WithWait(90*time.Second)); err != nil {
				t.Logf("cleanup Deposit(1 atto-USDFC): %v", err)
			}
		})

		withdraw, err := p.Withdraw(ctx, addrs.USDFC, one, payments.WithWait(90*time.Second))
		if err != nil {
			t.Fatalf("Withdraw(1 atto-USDFC): %v", err)
		}
		if withdraw == nil || withdraw.Receipt == nil || withdraw.Receipt.Status != 1 {
			t.Fatalf("Withdraw receipt = %+v", withdraw.Receipt)
		}
		withdrawn = true

		deposit, err := p.Deposit(ctx, addrs.USDFC, client.Address(), one, payments.WithWait(90*time.Second))
		if err != nil {
			t.Fatalf("Deposit(restore 1 atto-USDFC): %v", err)
		}
		if deposit == nil || deposit.Receipt == nil || deposit.Receipt.Status != 1 {
			t.Fatalf("Deposit receipt = %+v", deposit.Receipt)
		}
		restored = true

		after, err := p.AccountInfo(ctx, addrs.USDFC, client.Address())
		if err != nil {
			t.Fatalf("AccountInfo(after): %v", err)
		}
		if after == nil {
			t.Fatal("AccountInfo(after) returned nil")
		}
		afterWallet, err := p.WalletBalance(ctx, addrs.USDFC, client.Address())
		if err != nil {
			t.Fatalf("WalletBalance(after): %v", err)
		}
		if after.Funds.Cmp(before.Funds) != 0 {
			t.Fatalf("deposited funds after restore = %s, want %s", after.Funds, before.Funds)
		}
		t.Logf(
			"Withdraw/Deposit restored principal: account funds delta=%s, available delta=%s, wallet delta=%s, lockup-current delta=%s",
			new(big.Int).Sub(after.Funds, before.Funds),
			new(big.Int).Sub(after.AvailableFunds(), before.AvailableFunds()),
			new(big.Int).Sub(afterWallet, beforeWallet),
			new(big.Int).Sub(after.LockupCurrent, before.LockupCurrent),
		)
	})
}
