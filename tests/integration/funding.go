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
	totalRateDelta := new(big.Int).Mul(copyBig(base.Lockup.RateDelta), multiplier)
	totalRateLockup := new(big.Int).Mul(copyBig(base.Lockup.RateLockup), multiplier)
	totalCDNLockup := new(big.Int).Mul(copyBig(base.Lockup.CDNFixedLockup), multiplier)
	totalSybilFee := new(big.Int).Mul(copyBig(base.Lockup.SybilFee), multiplier)
	totalLockup := new(big.Int).Mul(copyBig(base.Lockup.TotalLockup), multiplier)

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
			RateDelta:      totalRateDelta,
			RateLockup:     totalRateLockup,
			CDNFixedLockup: totalCDNLockup,
			SybilFee:       totalSybilFee,
			TotalLockup:    totalLockup,
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
