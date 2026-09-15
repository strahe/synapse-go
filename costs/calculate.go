package costs

import (
	"fmt"
	"math/big"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

// CalculateEffectiveRate computes the storage rate for the given contract
// billable size in bytes, not the sum of raw piece payload sizes.
// Integer division is used to match on-chain Solidity truncation.
// If epochsPerMonth is zero or negative, chain.EpochsPerMonth is used as a safe default.
// Nil sizeBytes, pricePerTiBPerMonth, or datasetFeePerMonth are treated as zero.
func CalculateEffectiveRate(
	sizeBytes *big.Int,
	pricePerTiBPerMonth *big.Int,
	datasetFeePerMonth *big.Int,
	epochsPerMonth int64,
) EffectiveRate {
	if epochsPerMonth <= 0 {
		epochsPerMonth = chain.EpochsPerMonth
	}
	epm := big.NewInt(epochsPerMonth)

	if sizeBytes == nil {
		sizeBytes = new(big.Int)
	}
	if pricePerTiBPerMonth == nil {
		pricePerTiBPerMonth = new(big.Int)
	}
	if datasetFeePerMonth == nil {
		datasetFeePerMonth = new(big.Int)
	}

	if sizeBytes.Sign() == 0 {
		return EffectiveRate{
			RatePerEpoch: new(big.Int),
			RatePerMonth: new(big.Int),
		}
	}

	ratePerMonth := new(big.Int).Mul(pricePerTiBPerMonth, sizeBytes)
	ratePerMonth.Div(ratePerMonth, bigTiB)
	ratePerMonth.Add(ratePerMonth, datasetFeePerMonth)

	ratePerEpoch := new(big.Int).Mul(pricePerTiBPerMonth, sizeBytes)
	divisor := new(big.Int).Mul(bigTiB, epm)
	ratePerEpoch.Div(ratePerEpoch, divisor)
	ratePerEpoch.Add(ratePerEpoch, new(big.Int).Div(datasetFeePerMonth, epm))

	return EffectiveRate{
		RatePerEpoch: ratePerEpoch,
		RatePerMonth: ratePerMonth,
	}
}

// CalculateUploadFees computes a conservative one-time upload fee estimate from
// the price list. Every piece is priced as its own add-pieces operation because
// runtime batch boundaries cannot be inferred from raw sizes alone.
func CalculateUploadFees(priceList *warmstorage.PriceList, isNewDataSet bool, pieceSizes []uint64) (UploadFees, error) {
	if err := validatePieceSizes(pieceSizes); err != nil {
		return UploadFees{}, fmt.Errorf("costs.CalculateUploadFees: %w", err)
	}
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}
	pieces := new(big.Int).SetUint64(uint64(len(pieceSizes)))

	createDataSetFee := new(big.Int)
	if isNewDataSet {
		createDataSetFee.Set(zeroBig(priceList.Fees.CreateDataSetFee))
	}
	singlePieceFee := new(big.Int).Add(zeroBig(priceList.Fees.AddPiecesBaseFee), zeroBig(priceList.Fees.AddPiecesPerPieceFee))
	addPiecesFee := new(big.Int).Mul(singlePieceFee, pieces)
	total := new(big.Int).Add(createDataSetFee, addPiecesFee)

	return UploadFees{
		CreateDataSetFee: createDataSetFee,
		AddPiecesFee:     addPiecesFee,
		Total:            total,
	}, nil
}

// CalculateLifecycleReserveFunding computes the lifecycle reserve lockup needed
// to process the planned upload fees. For a new data set, the current reserve is
// ignored and the reserve starts at the configured target. Pending payments
// default to zero and, when supplied, are included for either data-set mode.
// The calculation does not modify its inputs.
func CalculateLifecycleReserveFunding(calc LifecycleReserveCalculation) (LifecycleReserveFunding, error) {
	if err := validatePieceSizes(calc.PieceSizes); err != nil {
		return LifecycleReserveFunding{}, fmt.Errorf("costs.CalculateLifecycleReserveFunding: %w", err)
	}

	pending := copyBigOrDefault(calc.PendingOneTimePayments, nil)
	if pending.Sign() < 0 {
		return LifecycleReserveFunding{}, fmt.Errorf(
			"costs.CalculateLifecycleReserveFunding: %w: PendingOneTimePayments must be non-negative",
			ErrInvalidArgument,
		)
	}
	priceList := calc.PriceList
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}
	target := zeroBig(priceList.Lockups.LifecycleReserveTarget)
	threshold := zeroBig(priceList.Lockups.ReplenishThreshold)
	initialLockup := new(big.Int)
	reserveBalance := new(big.Int)
	if calc.IsNewDataSet {
		initialLockup.Set(target)
		reserveBalance.Set(target)
		pending.Add(pending, zeroBig(priceList.Fees.CreateDataSetFee))
	} else {
		if calc.CurrentLifecycleReserveBalance == nil {
			return LifecycleReserveFunding{}, fmt.Errorf(
				"costs.CalculateLifecycleReserveFunding: %w: CurrentLifecycleReserveBalance is required for an existing dataset",
				ErrInvalidArgument,
			)
		}
		if calc.CurrentLifecycleReserveBalance.Sign() < 0 {
			return LifecycleReserveFunding{}, fmt.Errorf(
				"costs.CalculateLifecycleReserveFunding: %w: CurrentLifecycleReserveBalance must be non-negative",
				ErrInvalidArgument,
			)
		}
		reserveBalance.Set(calc.CurrentLifecycleReserveBalance)
	}

	reserveReplenishment := new(big.Int)
	singlePieceFee := new(big.Int).Add(zeroBig(priceList.Fees.AddPiecesBaseFee), zeroBig(priceList.Fees.AddPiecesPerPieceFee))
	for range calc.PieceSizes {
		pending.Add(pending, singlePieceFee)
		minimumBalance := new(big.Int).Add(pending, threshold)
		if reserveBalance.Cmp(minimumBalance) < 0 {
			replenishedBalance := new(big.Int).Add(target, pending)
			reserveReplenishment.Add(reserveReplenishment, new(big.Int).Sub(replenishedBalance, reserveBalance))
			reserveBalance.Set(replenishedBalance)
		}
		reserveBalance.Sub(reserveBalance, pending)
		pending.SetInt64(0)
	}

	return LifecycleReserveFunding{
		InitialLockup:        initialLockup,
		ReserveReplenishment: reserveReplenishment,
		Total:                new(big.Int).Add(initialLockup, reserveReplenishment),
		FinalReserveBalance:  reserveBalance,
	}, nil
}

// CalculateAdditionalLockupRequired returns the incremental lockup for pieces
// with the supplied raw payload sizes. A provided CurrentDataSetLeafCount is
// ignored for a new dataset. For an existing dataset, a negative leaf count
// returns ErrInvalidArgument. Nil leaf count and price list use zero-value
// defaults; empty pieceSizes and zero elements add no leaves.
func CalculateAdditionalLockupRequired(
	pieceSizes []uint64,
	currentDataSetLeafCount *big.Int,
	priceList *warmstorage.PriceList,
	lockupPeriod *big.Int,
	isNewDataSet bool,
	enableCDN bool,
) (AdditionalLockup, error) {
	if !isNewDataSet && currentDataSetLeafCount != nil && currentDataSetLeafCount.Sign() < 0 {
		return AdditionalLockup{}, fmt.Errorf(
			"costs.CalculateAdditionalLockupRequired: %w: CurrentDataSetLeafCount must be non-negative",
			ErrInvalidArgument,
		)
	}
	return calculateAdditionalLockupRequired(
		pieceSizesToLeafCount(pieceSizes),
		currentDataSetLeafCount,
		priceList,
		lockupPeriod,
		isNewDataSet,
		enableCDN,
	), nil
}

func calculateAdditionalLockupRequired(
	addedLeaves *big.Int,
	currentDataSetLeafCount *big.Int,
	priceList *warmstorage.PriceList,
	lockupPeriod *big.Int,
	isNewDataSet bool,
	enableCDN bool,
) AdditionalLockup {
	currentLeaves := new(big.Int)
	if !isNewDataSet && currentDataSetLeafCount != nil {
		currentLeaves.Set(currentDataSetLeafCount)
	}
	finalLeaves := new(big.Int).Add(currentLeaves, addedLeaves)
	finalSize := leafCountToBillableBytes(finalLeaves)
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}

	var rateDelta *big.Int
	if currentLeaves.Sign() > 0 {
		newRate := CalculateEffectiveRate(
			finalSize,
			priceList.Rates.StoragePerTiBPerMonth,
			priceList.Rates.DatasetFeePerMonth,
			chain.EpochsPerMonth,
		)
		currentRate := CalculateEffectiveRate(
			leafCountToBillableBytes(currentLeaves),
			priceList.Rates.StoragePerTiBPerMonth,
			priceList.Rates.DatasetFeePerMonth,
			chain.EpochsPerMonth,
		)
		rateDelta = new(big.Int).Sub(newRate.RatePerEpoch, currentRate.RatePerEpoch)
		if rateDelta.Sign() < 0 {
			rateDelta.SetInt64(0)
		}
	} else {
		newRate := CalculateEffectiveRate(
			finalSize,
			priceList.Rates.StoragePerTiBPerMonth,
			priceList.Rates.DatasetFeePerMonth,
			chain.EpochsPerMonth,
		)
		rateDelta = new(big.Int).Set(newRate.RatePerEpoch)
	}

	effectiveLockupPeriod := copyBigOrDefault(lockupPeriod, priceList.Lockups.DefaultLockupPeriod)
	if effectiveLockupPeriod.Sign() <= 0 {
		effectiveLockupPeriod.SetInt64(DefaultLockupPeriod)
	}
	streamingLockup := new(big.Int).Mul(rateDelta, effectiveLockupPeriod)

	lifecycleLockup := new(big.Int)
	if isNewDataSet {
		lifecycleLockup.Set(zeroBig(priceList.Lockups.LifecycleReserveTarget))
	}

	cdnLockup := new(big.Int)
	cacheMissLockup := new(big.Int)
	if isNewDataSet && enableCDN {
		cdnLockup.Set(zeroBig(priceList.Lockups.CDNLockupAmount))
		cacheMissLockup.Set(zeroBig(priceList.Lockups.CacheMissLockupAmount))
	}

	totalLockup := new(big.Int).Add(streamingLockup, lifecycleLockup)
	totalLockup.Add(totalLockup, cdnLockup)
	totalLockup.Add(totalLockup, cacheMissLockup)

	return AdditionalLockup{
		RateDeltaPerEpoch:    rateDelta,
		StreamingLockup:      streamingLockup,
		LifecycleLockup:      lifecycleLockup,
		ReserveReplenishment: new(big.Int),
		CDNLockup:            cdnLockup,
		CacheMissLockup:      cacheMissLockup,
		Total:                totalLockup,
	}
}

// CalculateDepositNeeded computes the USDFC deposit required to cover lockup,
// runway, and buffer.
//
// Buffer is skipped when currentLockupRate is zero and isNewDataSet is true:
// the deposit lands before the payment rail is created so the contract cannot
// yet drain it.
//
// Nil *big.Int fields are treated as zero. Negative epoch counts are clamped
// to zero. The calculation does not modify its inputs.
func CalculateDepositNeeded(calc DepositCalculation) *big.Int {
	additionalLockup := zeroBig(calc.AdditionalLockup)
	rateDelta := zeroBig(calc.RateDelta)
	currentLockupRate := zeroBig(calc.CurrentLockupRate)
	debt := zeroBig(calc.Debt)
	availableFunds := zeroBig(calc.AvailableFunds)
	runwayInEpochs := zeroBig(calc.RunwayInEpochs)
	runwayEpochs := max(calc.ExtraRunwayEpochs, 0)
	bufferEpochs := max(calc.BufferEpochs, 0)
	bufferEpochsBig := big.NewInt(bufferEpochs)
	combinedRate := new(big.Int).Add(currentLockupRate, rateDelta)
	runway := new(big.Int).Mul(combinedRate, big.NewInt(runwayEpochs))

	raw := new(big.Int).Add(additionalLockup, runway)
	raw.Sub(raw, availableFunds)
	raw.Add(raw, debt)

	skipBuffer := currentLockupRate.Sign() == 0 && calc.IsNewDataSet
	buffer := new(big.Int)
	if !skipBuffer {
		if raw.Sign() > 0 {
			buffer.Mul(combinedRate, bufferEpochsBig)
		} else if runwayInEpochs.Cmp(bufferEpochsBig) <= 0 {
			buffer.Mul(combinedRate, bufferEpochsBig)
			buffer.Sub(buffer, availableFunds)
			if buffer.Sign() < 0 {
				buffer.SetInt64(0)
			}
		}
	}

	if raw.Sign() > 0 {
		return raw.Add(raw, buffer)
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
func isFWSSMaxApproved(approved bool, rateAllowance, lockAllowance, maxLockPeriod, requiredLockupPeriod *big.Int) bool {
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
	required := copyBigOrDefault(requiredLockupPeriod, big.NewInt(DefaultLockupPeriod))
	if required.Sign() <= 0 {
		required.SetInt64(DefaultLockupPeriod)
	}
	if maxLockPeriod == nil || maxLockPeriod.Cmp(required) < 0 {
		return false
	}
	return true
}

func copyBigOrDefault(v, def *big.Int) *big.Int {
	if v != nil {
		return new(big.Int).Set(v)
	}
	if def != nil {
		return new(big.Int).Set(def)
	}
	return new(big.Int)
}

func requiredLockupPeriod(priceList *warmstorage.PriceList) *big.Int {
	if priceList != nil && priceList.Lockups.DefaultLockupPeriod != nil && priceList.Lockups.DefaultLockupPeriod.Sign() > 0 {
		return new(big.Int).Set(priceList.Lockups.DefaultLockupPeriod)
	}
	return big.NewInt(DefaultLockupPeriod)
}

func aggregateLockup(rateDelta, streaming, lifecycle, reserveReplenishment, cdn, cacheMiss, total *big.Int) AdditionalLockup {
	rateDeltaOut := copyBigOrDefault(rateDelta, nil)
	streamingOut := copyBigOrDefault(streaming, nil)
	lifecycleOut := copyBigOrDefault(lifecycle, nil)
	reserveReplenishmentOut := copyBigOrDefault(reserveReplenishment, nil)
	cdnOut := copyBigOrDefault(cdn, nil)
	cacheMissOut := copyBigOrDefault(cacheMiss, nil)
	totalOut := copyBigOrDefault(total, nil)
	return AdditionalLockup{
		RateDeltaPerEpoch:    rateDeltaOut,
		StreamingLockup:      streamingOut,
		LifecycleLockup:      lifecycleOut,
		ReserveReplenishment: reserveReplenishmentOut,
		CDNLockup:            cdnOut,
		CacheMissLockup:      cacheMissOut,
		Total:                totalOut,
	}
}
