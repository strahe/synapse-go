package costs

import (
	"math/big"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

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

// LifecycleReserveCalculation is the input to
// [CalculateLifecycleReserveFunding].
type LifecycleReserveCalculation struct {
	// PriceList is the canonical warm-storage price list. Nil uses zero values.
	PriceList *warmstorage.PriceList
	// PieceSizes contains every raw payload size in the planned upload.
	PieceSizes []uint64
	// IsNewDataSet is true when the upload creates a new data set.
	IsNewDataSet bool
	// CurrentLifecycleReserveBalance is required and non-negative for an
	// existing data set. It is ignored for a new data set.
	CurrentLifecycleReserveBalance *big.Int
	// PendingOneTimePayments is the non-negative fee total already waiting to be
	// paid from the reserve. Nil defaults to zero.
	PendingOneTimePayments *big.Int
}

// LifecycleReserveFunding is the fixed-lockup funding required for lifecycle
// operation fees.
type LifecycleReserveFunding struct {
	// InitialLockup is the reserve target locked when a new data set is created.
	InitialLockup *big.Int
	// ReserveReplenishment is the additional fixed lockup required while the
	// planned operation fees are processed.
	ReserveReplenishment *big.Int
	// Total is InitialLockup plus ReserveReplenishment.
	Total *big.Int
	// FinalReserveBalance is the simulated balance after all planned fees have
	// been processed.
	FinalReserveBalance *big.Int
}

// AdditionalLockup is the incremental lockup required when adding data to a
// dataset.
type AdditionalLockup struct {
	RateDeltaPerEpoch    *big.Int
	StreamingLockup      *big.Int
	LifecycleLockup      *big.Int
	ReserveReplenishment *big.Int
	CDNLockup            *big.Int
	CacheMissLockup      *big.Int
	Total                *big.Int
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
	// ExtraRunwayEpochs is extra epoch runway on top of lockup. Defaults to 0.
	// Negative values return ErrInvalidArgument.
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit buffer for execution latency.
	// Nil uses DefaultBufferEpochs (5); a pointer to zero disables the buffer.
	// Negative values return ErrInvalidArgument.
	BufferEpochs *int64
	// EnableCDN adds CDN and cache-miss lockup for a new dataset.
	EnableCDN bool
	// IsNewDataSet must be true when creating a fresh dataset.
	IsNewDataSet bool
	// CurrentDataSetLeafCount is required and non-negative for an existing
	// dataset. Zero means known empty. Ignored when IsNewDataSet is true.
	CurrentDataSetLeafCount *big.Int
	// CurrentLifecycleReserveBalance is required and non-negative for an
	// existing dataset. Ignored when IsNewDataSet is true.
	CurrentLifecycleReserveBalance *big.Int
	// PendingOneTimePayments is the non-negative operation-fee total already
	// waiting to be paid from an existing dataset's reserve. Nil defaults to
	// zero. Ignored when IsNewDataSet is true.
	PendingOneTimePayments *big.Int
	// PDPEndEpoch is required for an existing dataset. It must point to zero;
	// a non-zero epoch means the dataset can no longer accept uploads. Ignored
	// when IsNewDataSet is true.
	PDPEndEpoch *types.Epoch
}

// DepositCalculation is the input to CalculateDepositNeeded.
type DepositCalculation struct {
	// AdditionalLockup is the incremental lockup required by the upload.
	AdditionalLockup *big.Int
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
