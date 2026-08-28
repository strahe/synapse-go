//go:build integration

package integration_test

import (
	"math/big"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/payments"
)

const (
	integrationFundingExtraRunwayEpochs = chain.EpochsPerDay
	integrationFundingBufferEpochs      = 120
)

func aggregateNewUploadCosts(base *costs.UploadCosts, account *payments.AccountState, copies int) *costs.UploadCosts {
	if copies <= 0 {
		copies = 1
	}
	if base == nil {
		return &costs.UploadCosts{}
	}

	availableFunds := (*big.Int)(nil)
	currentLockupRate := (*big.Int)(nil)
	debt := new(big.Int)
	if account != nil {
		current := chain.CurrentEpoch(chain.Calibration)
		resolved := account.ResolveAt(current)
		availableFunds = resolved.AvailableFunds
		currentLockupRate = account.LockupRate
		debt = account.DebtAt(current)
	}

	multiplier := big.NewInt(int64(copies))
	totalRatePerEpoch := new(big.Int).Mul(copyBig(base.Rate.RatePerEpoch), multiplier)
	totalRatePerMonth := new(big.Int).Mul(copyBig(base.Rate.RatePerMonth), multiplier)
	totalRateDelta := new(big.Int).Mul(copyBig(base.Lockup.RateDeltaPerEpoch), multiplier)
	totalStreamingLockup := new(big.Int).Mul(copyBig(base.Lockup.StreamingLockup), multiplier)
	totalLifecycleLockup := new(big.Int).Mul(copyBig(base.Lockup.LifecycleLockup), multiplier)
	totalCDNLockup := new(big.Int).Mul(copyBig(base.Lockup.CDNLockup), multiplier)
	totalCacheMissLockup := new(big.Int).Mul(copyBig(base.Lockup.CacheMissLockup), multiplier)
	totalLockup := new(big.Int).Mul(copyBig(base.Lockup.Total), multiplier)

	depositNeeded := costs.CalculateDepositNeeded(costs.DepositCalculation{
		AdditionalLockup:  totalLockup,
		RateDelta:         totalRateDelta,
		CurrentLockupRate: currentLockupRate,
		Debt:              debt,
		AvailableFunds:    availableFunds,
		ExtraRunwayEpochs: integrationFundingExtraRunwayEpochs,
		BufferEpochs:      integrationFundingBufferEpochs,
		IsNewDataSet:      true,
	})

	return &costs.UploadCosts{
		Rate: costs.EffectiveRate{
			RatePerEpoch: totalRatePerEpoch,
			RatePerMonth: totalRatePerMonth,
		},
		Lockup: costs.AdditionalLockup{
			RateDeltaPerEpoch: totalRateDelta,
			StreamingLockup:   totalStreamingLockup,
			LifecycleLockup:   totalLifecycleLockup,
			CDNLockup:         totalCDNLockup,
			CacheMissLockup:   totalCacheMissLockup,
			Total:             totalLockup,
		},
		DepositNeeded:        depositNeeded,
		NeedsFWSSMaxApproval: base.NeedsFWSSMaxApproval,
		Ready:                depositNeeded.Sign() == 0 && !base.NeedsFWSSMaxApproval,
	}
}

func copyBig(v *big.Int) *big.Int {
	if v == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(v)
}
