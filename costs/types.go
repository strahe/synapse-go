package costs

import "math/big"

// EffectiveRate is the per-epoch and per-month storage rate for a given data size.
// RatePerEpoch uses integer division to match on-chain Solidity truncation.
type EffectiveRate struct {
	RatePerEpoch *big.Int
	RatePerMonth *big.Int
}

// AdditionalLockup is the incremental lockup required when adding data to a dataset.
// TotalLockup = RateLockup + CDNFixedLockup + SybilFee.
type AdditionalLockup struct {
	RateDelta      *big.Int // marginal rate per epoch added by this upload
	RateLockup     *big.Int // RateDelta * lockupPeriod
	CDNFixedLockup *big.Int // 1.0 USDFC for new CDN datasets, 0 otherwise
	SybilFee       *big.Int // anti-sybil fee for new datasets, 0 otherwise
	TotalLockup    *big.Int // sum of all components
}

// UploadCosts is the result of GetUploadCosts.
type UploadCosts struct {
	Rate                 EffectiveRate
	Lockup               AdditionalLockup
	DepositNeeded        *big.Int
	NeedsFWSSMaxApproval bool
	Ready                bool
}

// UploadCostOptions customises the GetUploadCosts calculation.
type UploadCostOptions struct {
	// RunwayEpochs is extra epoch runway on top of lockup. Defaults to DefaultRunwayEpochs (0).
	RunwayEpochs int64
	// BufferEpochs is the deposit buffer for execution latency.
	// Zero (the zero value) uses DefaultBufferEpochs (5); there is no way to
	// request a zero-epoch buffer via this field.
	BufferEpochs int64
	// EnableCDN adds CDN_FIXED_LOCKUP (1 USDFC) for a new dataset.
	EnableCDN bool
	// IsNewDataSet must be true when creating a fresh dataset (affects sybil fee and CDN lockup).
	IsNewDataSet bool
	// CurrentDataSetSizeBytes is the existing payload in the dataset (0 for new datasets).
	CurrentDataSetSizeBytes *big.Int
}

// AccountSummary is the snapshot of an account's payment state.
type AccountSummary struct {
	Funds              *big.Int
	AvailableFunds     *big.Int
	Debt               *big.Int
	LockupRatePerEpoch *big.Int
	LockupRatePerMonth *big.Int
	FundedUntilEpoch   *big.Int
	CurrentEpoch       *big.Int
}
