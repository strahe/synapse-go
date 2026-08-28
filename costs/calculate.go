package costs

import (
	"math/big"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/warmstorage"
)

// CalculateEffectiveRate computes the storage rate for the given total data size.
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

// CalculateUploadFees computes one-time upload fees from the price list.
func CalculateUploadFees(priceList *warmstorage.PriceList, isNewDataSet bool, pieceCount *big.Int) UploadFees {
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}
	pieces := copyBigOrDefault(pieceCount, bigOne)
	if pieces.Sign() <= 0 {
		pieces.SetInt64(1)
	}

	maxBatch := big.NewInt(pdp.MaxAddPiecesBatchSize)
	addPiecesOperationCount := new(big.Int).Add(pieces, new(big.Int).Sub(maxBatch, bigOne))
	addPiecesOperationCount.Div(addPiecesOperationCount, maxBatch)

	createDataSetFee := new(big.Int)
	if isNewDataSet {
		createDataSetFee.Set(zeroBig(priceList.Fees.CreateDataSetFee))
	}
	addPiecesFee := new(big.Int).Mul(zeroBig(priceList.Fees.AddPiecesBaseFee), addPiecesOperationCount)
	addPiecesFee.Add(addPiecesFee, new(big.Int).Mul(zeroBig(priceList.Fees.AddPiecesPerPieceFee), pieces))
	total := new(big.Int).Add(createDataSetFee, addPiecesFee)

	return UploadFees{
		CreateDataSetFee: createDataSetFee,
		AddPiecesFee:     addPiecesFee,
		Total:            total,
	}
}

// CalculateAdditionalLockupRequired returns the incremental lockup needed to
// store uploadSizeBytes into a dataset that currently holds currentDataSetSizeBytes.
// Nil dataSizeBytes, currentDataSetSizeBytes, and priceList use zero-value defaults.
func CalculateAdditionalLockupRequired(
	dataSizeBytes *big.Int,
	currentDataSetSizeBytes *big.Int,
	priceList *warmstorage.PriceList,
	lockupPeriod *big.Int,
	isNewDataSet bool,
	enableCDN bool,
) AdditionalLockup {
	if dataSizeBytes == nil {
		dataSizeBytes = new(big.Int)
	}
	if currentDataSetSizeBytes == nil {
		currentDataSetSizeBytes = new(big.Int)
	}
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}

	var rateDelta *big.Int
	if currentDataSetSizeBytes.Sign() > 0 && !isNewDataSet {
		newTotalSize := new(big.Int).Add(currentDataSetSizeBytes, dataSizeBytes)
		newRate := CalculateEffectiveRate(
			newTotalSize,
			priceList.Rates.StoragePerTiBPerMonth,
			priceList.Rates.DatasetFeePerMonth,
			chain.EpochsPerMonth,
		)
		currentRate := CalculateEffectiveRate(
			currentDataSetSizeBytes,
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
			dataSizeBytes,
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
		RateDeltaPerEpoch: rateDelta,
		StreamingLockup:   streamingLockup,
		LifecycleLockup:   lifecycleLockup,
		CDNLockup:         cdnLockup,
		CacheMissLockup:   cacheMissLockup,
		Total:             totalLockup,
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
	fees := zeroBig(calc.Fees)
	rateDelta := zeroBig(calc.RateDelta)
	currentLockupRate := zeroBig(calc.CurrentLockupRate)
	debt := zeroBig(calc.Debt)
	availableFunds := zeroBig(calc.AvailableFunds)
	runwayInEpochs := zeroBig(calc.RunwayInEpochs)
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
	raw.Add(raw, fees)
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

func aggregateLockup(rateDelta, streaming, lifecycle, cdn, cacheMiss, total *big.Int) AdditionalLockup {
	rateDeltaOut := copyBigOrDefault(rateDelta, nil)
	streamingOut := copyBigOrDefault(streaming, nil)
	lifecycleOut := copyBigOrDefault(lifecycle, nil)
	cdnOut := copyBigOrDefault(cdn, nil)
	cacheMissOut := copyBigOrDefault(cacheMiss, nil)
	totalOut := copyBigOrDefault(total, nil)
	return AdditionalLockup{
		RateDeltaPerEpoch: rateDeltaOut,
		StreamingLockup:   streamingOut,
		LifecycleLockup:   lifecycleOut,
		CDNLockup:         cdnOut,
		CacheMissLockup:   cacheMissOut,
		Total:             totalOut,
	}
}
