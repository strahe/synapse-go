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
			RateDeltaPerEpoch: big.NewInt(2),
			StreamingLockup:   big.NewInt(20),
			LifecycleLockup:   big.NewInt(50),
			CDNLockup:         big.NewInt(10),
			CacheMissLockup:   big.NewInt(20),
			Total:             big.NewInt(100),
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
	if twoCopies.Lockup.RateDeltaPerEpoch.Cmp(big.NewInt(4)) != 0 {
		t.Fatalf("twoCopies.Lockup.RateDeltaPerEpoch=%s want 4", twoCopies.Lockup.RateDeltaPerEpoch)
	}
	if twoCopies.Lockup.StreamingLockup.Cmp(big.NewInt(40)) != 0 {
		t.Fatalf("twoCopies.Lockup.StreamingLockup=%s want 40", twoCopies.Lockup.StreamingLockup)
	}
	if twoCopies.Lockup.LifecycleLockup.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("twoCopies.Lockup.LifecycleLockup=%s want 100", twoCopies.Lockup.LifecycleLockup)
	}
	if twoCopies.Lockup.CDNLockup.Cmp(big.NewInt(20)) != 0 {
		t.Fatalf("twoCopies.Lockup.CDNLockup=%s want 20", twoCopies.Lockup.CDNLockup)
	}
	if twoCopies.Lockup.CacheMissLockup.Cmp(big.NewInt(40)) != 0 {
		t.Fatalf("twoCopies.Lockup.CacheMissLockup=%s want 40", twoCopies.Lockup.CacheMissLockup)
	}
	if twoCopies.Lockup.Total.Cmp(big.NewInt(200)) != 0 {
		t.Fatalf("twoCopies.Lockup.Total=%s want 200", twoCopies.Lockup.Total)
	}
	if twoCopies.Rate.RatePerEpoch.Cmp(big.NewInt(4)) != 0 {
		t.Fatalf("twoCopies.Rate.RatePerEpoch=%s want 4", twoCopies.Rate.RatePerEpoch)
	}
}
