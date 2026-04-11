//go:build integration

package integration_test

import (
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/payments"
)

func TestAggregateNewUploadCosts_MultipliesPerCopyLockup(t *testing.T) {
	base := &costs.UploadCosts{
		Rate: costs.EffectiveRate{
			RatePerEpoch: big.NewInt(2),
			RatePerMonth: big.NewInt(60),
		},
		Lockup: costs.AdditionalLockup{
			RateDelta:      big.NewInt(2),
			RateLockup:     big.NewInt(20),
			CDNFixedLockup: big.NewInt(30),
			SybilFee:       big.NewInt(50),
			TotalLockup:    big.NewInt(100),
		},
	}
	account := &payments.AccountState{
		Funds:         big.NewInt(150),
		LockupCurrent: big.NewInt(0),
		LockupRate:    big.NewInt(0),
	}

	oneCopy := aggregateNewUploadCosts(base, account, 1)
	if oneCopy.DepositNeeded.Sign() != 0 {
		t.Fatalf("oneCopy.DepositNeeded=%s want 0", oneCopy.DepositNeeded)
	}

	twoCopies := aggregateNewUploadCosts(base, account, 2)
	if twoCopies.DepositNeeded.Cmp(big.NewInt(50)) != 0 {
		t.Fatalf("twoCopies.DepositNeeded=%s want 50", twoCopies.DepositNeeded)
	}
	if twoCopies.Lockup.TotalLockup.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("twoCopies.Lockup.TotalLockup=%s want 200", twoCopies.Lockup.TotalLockup)
	}
	if twoCopies.Lockup.SybilFee.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("twoCopies.Lockup.SybilFee=%s want 100", twoCopies.Lockup.SybilFee)
	}
	if twoCopies.Rate.RatePerEpoch.Cmp(big.NewInt(4)) != 0 {
		t.Fatalf("twoCopies.Rate.RatePerEpoch=%s want 4", twoCopies.Rate.RatePerEpoch)
	}
}
