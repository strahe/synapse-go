package costs

import (
	"math/big"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

// CalculateEffectiveRate computes the storage rate for the given total data size.
// Integer division is used to match on-chain Solidity truncation.
// If epochsPerMonth is zero or negative, chain.EpochsPerMonth is used as a safe default.
// Nil pricePerTiBPerMonth or minMonthlyRate are treated as zero.
func CalculateEffectiveRate(
	sizeBytes *big.Int,
	pricePerTiBPerMonth *big.Int,
	minMonthlyRate *big.Int,
	epochsPerMonth int64,
) EffectiveRate {
	if epochsPerMonth <= 0 {
		epochsPerMonth = chain.EpochsPerMonth
	}
	epm := big.NewInt(epochsPerMonth)

	if pricePerTiBPerMonth == nil {
		pricePerTiBPerMonth = new(big.Int)
	}
	if minMonthlyRate == nil {
		minMonthlyRate = new(big.Int)
	}

	ratePerMonth := new(big.Int).Mul(pricePerTiBPerMonth, sizeBytes)
	ratePerMonth.Div(ratePerMonth, bigTiB)
	hitMin := ratePerMonth.Cmp(minMonthlyRate) < 0
	if hitMin {
		ratePerMonth.Set(minMonthlyRate)
	}

	// ratePerEpoch is computed independently to avoid accumulating two division errors.
	ratePerEpoch := new(big.Int).Mul(pricePerTiBPerMonth, sizeBytes)
	ratePerEpoch.Div(ratePerEpoch, bigTiB)
	ratePerEpoch.Div(ratePerEpoch, epm)

	minEpochRate := new(big.Int).Div(minMonthlyRate, epm)
	if minEpochRate.Cmp(bigOne) < 0 {
		minEpochRate.Set(bigOne)
	}
	if ratePerEpoch.Cmp(minEpochRate) < 0 {
		ratePerEpoch.Set(minEpochRate)
	}

	// At the minimum floor, align ratePerMonth with the epoch-aligned rate so that
	// ratePerEpoch × epm == ratePerMonth exactly.  Above the floor the two fields
	// are intentionally computed independently to avoid accumulating two division
	// truncation errors in the more-common non-minimum case.
	if hitMin {
		ratePerMonth.Mul(ratePerEpoch, epm)
	}

	return EffectiveRate{
		RatePerEpoch: ratePerEpoch,
		RatePerMonth: ratePerMonth,
	}
}

// CalculateAdditionalLockupRequired returns the incremental lockup needed to
// store uploadSizeBytes into a dataset that currently holds currentDataSetSizeBytes.
// usdfcSybilFee may be nil (treated as zero).
func CalculateAdditionalLockupRequired(
	dataSizeBytes *big.Int,
	currentDataSetSizeBytes *big.Int,
	pricing *warmstorage.ServicePrice,
	lockupPeriod int64,
	usdfcSybilFee *big.Int,
	isNewDataSet bool,
	enableCDN bool,
) AdditionalLockup {
	newTotalSize := new(big.Int).Add(currentDataSetSizeBytes, dataSizeBytes)
	var epm int64
	if pricing.EpochsPerMonth != nil && pricing.EpochsPerMonth.Sign() > 0 {
		epm = pricing.EpochsPerMonth.Int64()
	}

	newRate := CalculateEffectiveRate(
		newTotalSize,
		pricing.PricePerTiBPerMonthNoCDN,
		pricing.MinimumPricePerMonth,
		epm,
	)

	var rateDelta *big.Int
	if isNewDataSet {
		rateDelta = new(big.Int).Set(newRate.RatePerEpoch)
	} else {
		currentRate := CalculateEffectiveRate(
			currentDataSetSizeBytes,
			pricing.PricePerTiBPerMonthNoCDN,
			pricing.MinimumPricePerMonth,
			epm,
		)
		rateDelta = new(big.Int).Sub(newRate.RatePerEpoch, currentRate.RatePerEpoch)
		if rateDelta.Sign() < 0 {
			rateDelta.SetInt64(0)
		}
	}

	if lockupPeriod <= 0 {
		lockupPeriod = DefaultLockupPeriod
	}
	rateLockup := new(big.Int).Mul(rateDelta, big.NewInt(lockupPeriod))

	cdnLockup := new(big.Int)
	if isNewDataSet && enableCDN {
		cdnLockup.Set(cdnFixedLockup)
	}

	sybilFee := new(big.Int)
	if isNewDataSet && usdfcSybilFee != nil {
		sybilFee.Set(usdfcSybilFee)
	}

	totalLockup := new(big.Int).Add(rateLockup, cdnLockup)
	totalLockup.Add(totalLockup, sybilFee)

	return AdditionalLockup{
		RateDelta:      rateDelta,
		RateLockup:     rateLockup,
		CDNFixedLockup: cdnLockup,
		SybilFee:       sybilFee,
		TotalLockup:    totalLockup,
	}
}

// CalculateDepositNeeded computes the USDFC deposit required to cover lockup,
// runway, and buffer.
//
// Buffer is skipped when currentLockupRate is zero and isNewDataSet is true:
// the deposit lands before the payment rail is created so the contract cannot
// yet drain it.
//
// Nil *big.Int fields are treated as zero. Negative epoch counts are clamped to zero.
func CalculateDepositNeeded(calc DepositCalculation) *big.Int {
	additionalLockup := zeroBig(calc.AdditionalLockup)
	rateDelta := zeroBig(calc.RateDelta)
	currentLockupRate := zeroBig(calc.CurrentLockupRate)
	debt := zeroBig(calc.Debt)
	availableFunds := zeroBig(calc.AvailableFunds)
	runwayEpochs := calc.ExtraRunwayEpochs
	if runwayEpochs < 0 {
		runwayEpochs = 0
	}
	bufferEpochs := calc.BufferEpochs
	if bufferEpochs < 0 {
		bufferEpochs = 0
	}
	bufferEpochsBig := big.NewInt(bufferEpochs)
	combinedRate := new(big.Int).Add(currentLockupRate, rateDelta)
	runway := new(big.Int).Mul(combinedRate, big.NewInt(runwayEpochs))

	raw := new(big.Int).Add(additionalLockup, runway)
	raw.Sub(raw, availableFunds)
	raw.Add(raw, debt)

	if currentLockupRate.Sign() == 0 && calc.IsNewDataSet {
		if raw.Sign() < 0 {
			return new(big.Int)
		}
		return raw
	}

	bufferCost := new(big.Int).Mul(combinedRate, bufferEpochsBig)
	if raw.Sign() > 0 {
		return raw.Add(raw, bufferCost)
	}

	remainingAfterRequirements := new(big.Int).Neg(raw)
	buffer := new(big.Int).Sub(bufferCost, remainingAfterRequirements)
	if buffer.Sign() < 0 {
		return new(big.Int)
	}
	return buffer
}

func zeroBig(v *big.Int) *big.Int {
	if v == nil {
		return new(big.Int)
	}
	return v
}

// isFWSSMaxApproved returns true when all FWSS approval conditions are met.
// Nil *big.Int fields are treated as zero (not approved).
func isFWSSMaxApproved(approved bool, rateAllowance, lockAllowance, maxLockPeriod *big.Int) bool {
	if !approved {
		return false
	}
	if rateAllowance == nil || rateAllowance.Cmp(maxUint256) != 0 {
		return false
	}
	// lockAllowance uses a threshold (not exact) because the contract decrements it on CDN payments.
	if lockAllowance == nil || lockAllowance.Cmp(halfMaxUint256) < 0 {
		return false
	}
	if maxLockPeriod == nil || maxLockPeriod.Cmp(big.NewInt(DefaultLockupPeriod)) < 0 {
		return false
	}
	return true
}
