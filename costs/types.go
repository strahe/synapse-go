package costs

import "math/big"

// EffectiveRate is the per-epoch and per-month storage rate for a given data size.
// RatePerEpoch uses integer division to match on-chain Solidity truncation.
// RatePerMonth preserves monthly pricing precision for display and comparison.
type EffectiveRate struct {
	RatePerEpoch *big.Int
	RatePerMonth *big.Int
}

// UploadFees is the one-time fee breakdown for an upload.
type UploadFees struct {
	CreateDataSetFee *big.Int
	AddPiecesFee     *big.Int
	Total            *big.Int
}

// AdditionalLockup is the incremental lockup required when adding data to a
// dataset.
type AdditionalLockup struct {
	RateDeltaPerEpoch *big.Int
	StreamingLockup   *big.Int
	LifecycleLockup   *big.Int
	CDNLockup         *big.Int
	CacheMissLockup   *big.Int
	Total             *big.Int
}

// UploadCosts is the result of GetUploadCosts.
type UploadCosts struct {
	Rate                 EffectiveRate
	Fees                 UploadFees
	Lockup               AdditionalLockup
	DepositNeeded        *big.Int
	RequiredLockupPeriod *big.Int
	NeedsFWSSMaxApproval bool
	Ready                bool
}

// UploadCostOptions customises the GetUploadCosts calculation.
type UploadCostOptions struct {
	// ExtraRunwayEpochs is extra epoch runway on top of lockup. Defaults to DefaultExtraRunwayEpochs (0).
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit buffer for execution latency.
	// Nil uses DefaultBufferEpochs (5); a pointer to zero disables the buffer.
	// Negative values return ErrInvalidArgument.
	BufferEpochs *int64
	// EnableCDN adds CDN and cache-miss lockup for a new dataset.
	EnableCDN bool
	// IsNewDataSet must be true when creating a fresh dataset.
	IsNewDataSet bool
	// CurrentDataSetSizeBytes is the existing payload in the dataset (0 for new datasets).
	CurrentDataSetSizeBytes *big.Int
	// PieceCount is the number of pieces added by this upload. Zero defaults
	// to one piece.
	PieceCount *big.Int
}

// DepositCalculation is the input to CalculateDepositNeeded.
type DepositCalculation struct {
	// AdditionalLockup is the incremental lockup required by the upload.
	AdditionalLockup *big.Int
	// Fees are one-time operation fees required by the upload.
	Fees *big.Int
	// RateDelta is the incremental per-epoch payment rate added by the upload.
	RateDelta *big.Int
	// CurrentLockupRate is the account's existing per-epoch payment rate.
	CurrentLockupRate *big.Int
	// Debt is already-accrued payment debt that must be covered.
	Debt *big.Int
	// AvailableFunds is the account balance available after projected lockup.
	AvailableFunds *big.Int
	// RunwayInEpochs is the current account runway after projection.
	RunwayInEpochs *big.Int
	// ExtraRunwayEpochs is extra epoch runway on top of the required lockup.
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit buffer for execution latency.
	// Zero means no buffer for direct calculations; service options apply their
	// own default before calling CalculateDepositNeeded.
	BufferEpochs int64
	// IsNewDataSet is true when creating a fresh dataset.
	IsNewDataSet bool
}
