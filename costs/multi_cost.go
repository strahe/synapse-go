package costs

import (
	"context"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

// MultiContextRef describes one upload target for
// [Service.CalculateMultiContextCosts]. Each ref yields its own lockup
// contribution; account-level debt, runway and buffer are computed once
// across the aggregate.
type MultiContextRef struct {
	// IsNewDataSet is true when the target will create a new data set
	// on this provider (contributes lifecycle and optional CDN/cache-miss
	// lockup). When false, CurrentDataSetLeafCount is required.
	IsNewDataSet bool

	// CurrentDataSetLeafCount is the non-negative on-chain leaf count of an
	// existing data set. Zero means known empty; nil is invalid for an existing
	// data set. Ignored when IsNewDataSet is true.
	CurrentDataSetLeafCount *big.Int
	// CurrentLifecycleReserveBalance is required and non-negative for an
	// existing data set. It is ignored when IsNewDataSet is true.
	CurrentLifecycleReserveBalance *big.Int
	// PendingOneTimePayments is the non-negative operation-fee total already
	// waiting to be paid from an existing data set's reserve. Nil defaults to
	// zero. It is ignored when IsNewDataSet is true.
	PendingOneTimePayments *big.Int
	// PDPEndEpoch is required for an existing data set. It must point to zero;
	// a non-zero epoch means the data set can no longer accept uploads. It is
	// ignored when IsNewDataSet is true.
	PDPEndEpoch *types.Epoch

	// WithCDN toggles CDN and cache-miss lockup for this target. Only
	// meaningful when IsNewDataSet is true.
	WithCDN bool
}

// MultiContextCosts is the aggregate cost view across multiple upload
// targets: a single DepositNeeded / NeedsFWSSMaxApproval / Ready, plus
// summed RatePerEpoch / RatePerMonth covering all refs.
type MultiContextCosts struct {
	// RatePerEpoch is the sum of per-context effective rates (post-upload).
	RatePerEpoch *big.Int
	// RatePerMonth is the sum of per-context monthly effective rates.
	RatePerMonth *big.Int
	// Fees is the aggregate one-time fee breakdown.
	Fees UploadFees
	// Lockup is the aggregate lockup breakdown.
	Lockup AdditionalLockup
	// DepositNeeded is the single USDFC deposit covering all contexts.
	DepositNeeded *big.Int
	// RequiredLockupPeriod is the max lockup period required for FWSS
	// approval, sourced from the price list.
	RequiredLockupPeriod *big.Int
	// NeedsFWSSMaxApproval is true when the FWSS operator does not yet
	// hold max approval for the payer.
	NeedsFWSSMaxApproval bool
	// Ready is true when DepositNeeded is zero and FWSS approval is set.
	Ready bool
}

// CalculateMultiContextCosts aggregates upload costs across multiple
// prospective contexts for one piece plan.
//
// Each ref contributes its own lockup; debt, runway and buffer are computed
// once from the payer's account state.
//
// pieceSizes contains each piece's raw payload size, replicated to every
// target. Each size must be between chain.MinUploadSize and
// chain.MaxUploadSize. Only ExtraRunwayEpochs and BufferEpochs are used from opts.
// EnableCDN, IsNewDataSet, and CurrentDataSetLeafCount in opts are ignored;
// supply the dataset state, current leaf count, and CDN setting through each ref.
// Nil opts uses defaults; a nil BufferEpochs uses DefaultBufferEpochs.
func (s *Service) CalculateMultiContextCosts(
	ctx context.Context,
	payer common.Address,
	pieceSizes []uint64,
	refs []MultiContextRef,
	opts *UploadCostOptions,
) (*MultiContextCosts, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w: refs is empty", ErrInvalidArgument)
	}
	if err := validatePieceSizes(pieceSizes); err != nil {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w", err)
	}
	if opts == nil {
		opts = &UploadCostOptions{}
	}
	if opts.ExtraRunwayEpochs < 0 {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w: ExtraRunwayEpochs must be non-negative", ErrInvalidArgument)
	}
	targets := make([]uploadCostTarget, len(refs))
	for i, ref := range refs {
		state, err := resolveDataSetCostState(
			ref.IsNewDataSet,
			ref.CurrentDataSetLeafCount,
			ref.CurrentLifecycleReserveBalance,
			ref.PendingOneTimePayments,
			ref.PDPEndEpoch,
		)
		if err != nil {
			return nil, fmt.Errorf("costs.CalculateMultiContextCosts: refs[%d]: %w", i, err)
		}
		targets[i] = uploadCostTarget{isNewDataSet: ref.IsNewDataSet, withCDN: ref.WithCDN, state: state}
	}
	bufferEpochs, err := resolveBufferEpochs(opts.BufferEpochs)
	if err != nil {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w", err)
	}

	totals, err := s.calculateUploadCosts(ctx, payer, slices.Clone(pieceSizes), targets, opts.ExtraRunwayEpochs, bufferEpochs)
	if err != nil {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w", err)
	}
	return &MultiContextCosts{
		RatePerEpoch:         totals.rate.RatePerEpoch,
		RatePerMonth:         totals.rate.RatePerMonth,
		Fees:                 totals.fees,
		Lockup:               totals.lockup,
		DepositNeeded:        totals.depositNeeded,
		RequiredLockupPeriod: totals.requiredLockupPeriod,
		NeedsFWSSMaxApproval: totals.needsApproval,
		Ready:                totals.ready(),
	}, nil
}
