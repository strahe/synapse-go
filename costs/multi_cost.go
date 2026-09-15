package costs

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
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
	options := *opts
	opts = &options
	pieceSizes = slices.Clone(pieceSizes)
	refs = slices.Clone(refs)
	for i := range refs {
		state, err := resolveDataSetCostState(
			refs[i].IsNewDataSet,
			refs[i].CurrentDataSetLeafCount,
			refs[i].CurrentLifecycleReserveBalance,
			refs[i].PendingOneTimePayments,
			refs[i].PDPEndEpoch,
		)
		if err != nil {
			return nil, fmt.Errorf("costs.CalculateMultiContextCosts: refs[%d]: %w", i, err)
		}
		refs[i].CurrentDataSetLeafCount = state.leaves
		refs[i].CurrentLifecycleReserveBalance = state.reserveBalance
		refs[i].PendingOneTimePayments = state.pendingPayments
		endEpoch := state.pdpEndEpoch
		refs[i].PDPEndEpoch = &endEpoch
	}
	runwayEpochs := opts.ExtraRunwayEpochs
	bufferEpochs, err := resolveBufferEpochs(opts.BufferEpochs)
	if err != nil {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w", err)
	}

	var (
		priceList *warmstorage.PriceList
		account   *payments.AccountState
		approval  *payments.OperatorApproval
		mu        sync.Mutex
		errs      []error
		wg        sync.WaitGroup
	)

	wg.Add(3)

	go func() {
		defer wg.Done()
		p, err := s.ws.GetPriceList(ctx)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("GetPriceList: %w", err))
			return
		}
		priceList = p
	}()

	go func() {
		defer wg.Done()
		a, err := s.pay.AccountInfo(ctx, s.usdfc, payer)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("AccountInfo: %w", err))
			return
		}
		account = a
	}()

	go func() {
		defer wg.Done()
		ap, err := s.pay.ServiceApproval(ctx, s.usdfc, payer, s.fwss)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("ServiceApproval: %w", err))
			return
		}
		approval = ap
	}()

	wg.Wait()

	if len(errs) > 0 {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w", errors.Join(errs...))
	}
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}

	totalRateDelta := new(big.Int)
	totalLockup := new(big.Int)
	totalLifecycleLockup := new(big.Int)
	totalReserveReplenishment := new(big.Int)
	totalStreamingLockup := new(big.Int)
	totalCDNLockup := new(big.Int)
	totalCacheMissLockup := new(big.Int)
	totalRatePerEpoch := new(big.Int)
	totalRatePerMonth := new(big.Int)
	totalCreateDataSetFee := new(big.Int)
	totalAddPiecesFee := new(big.Int)
	allNewDataSets := true
	requiredLockupPeriod := requiredLockupPeriod(priceList)
	addedLeaves := pieceSizesToLeafCount(pieceSizes)

	for i := range refs {
		ref := &refs[i]
		if !ref.IsNewDataSet {
			allNewDataSets = false
		}
		currentLeaves := ref.CurrentDataSetLeafCount

		lockup := calculateAdditionalLockupRequired(
			addedLeaves,
			currentLeaves,
			priceList,
			requiredLockupPeriod,
			ref.IsNewDataSet,
			ref.WithCDN,
		)
		fees, err := CalculateUploadFees(priceList, ref.IsNewDataSet, pieceSizes)
		if err != nil {
			return nil, fmt.Errorf("costs.CalculateMultiContextCosts: refs[%d]: %w", i, err)
		}
		reserveFunding, err := CalculateLifecycleReserveFunding(LifecycleReserveCalculation{
			PriceList:                      priceList,
			PieceSizes:                     pieceSizes,
			IsNewDataSet:                   ref.IsNewDataSet,
			CurrentLifecycleReserveBalance: ref.CurrentLifecycleReserveBalance,
			PendingOneTimePayments:         ref.PendingOneTimePayments,
		})
		if err != nil {
			return nil, fmt.Errorf("costs.CalculateMultiContextCosts: refs[%d]: %w", i, err)
		}
		lockup.ReserveReplenishment = reserveFunding.ReserveReplenishment
		lockup.Total.Add(lockup.Total, lockup.ReserveReplenishment)
		totalRateDelta.Add(totalRateDelta, lockup.RateDeltaPerEpoch)
		totalLockup.Add(totalLockup, lockup.Total)
		totalLifecycleLockup.Add(totalLifecycleLockup, lockup.LifecycleLockup)
		totalReserveReplenishment.Add(totalReserveReplenishment, lockup.ReserveReplenishment)
		totalStreamingLockup.Add(totalStreamingLockup, lockup.StreamingLockup)
		totalCDNLockup.Add(totalCDNLockup, lockup.CDNLockup)
		totalCacheMissLockup.Add(totalCacheMissLockup, lockup.CacheMissLockup)
		totalCreateDataSetFee.Add(totalCreateDataSetFee, fees.CreateDataSetFee)
		totalAddPiecesFee.Add(totalAddPiecesFee, fees.AddPiecesFee)

		totalSize := leafCountToBillableBytes(new(big.Int).Add(currentLeaves, addedLeaves))
		rate := CalculateEffectiveRate(
			totalSize,
			priceList.Rates.StoragePerTiBPerMonth,
			priceList.Rates.DatasetFeePerMonth,
			chain.EpochsPerMonth,
		)
		totalRatePerEpoch.Add(totalRatePerEpoch, rate.RatePerEpoch)
		totalRatePerMonth.Add(totalRatePerMonth, rate.RatePerMonth)
	}

	currentEpoch, err := s.currentEpoch(ctx)
	if err != nil {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w", err)
	}
	resolved := account.ResolveAt(currentEpoch)
	debt := account.DebtAt(currentEpoch)
	avail := resolved.AvailableFunds
	currentRate := account.LockupRate
	if currentRate == nil {
		currentRate = new(big.Int)
	}

	depositNeeded := CalculateDepositNeeded(DepositCalculation{
		AdditionalLockup:  totalLockup,
		RateDelta:         totalRateDelta,
		CurrentLockupRate: currentRate,
		Debt:              debt,
		AvailableFunds:    avail,
		RunwayInEpochs:    resolved.RunwayInEpochs,
		ExtraRunwayEpochs: runwayEpochs,
		BufferEpochs:      bufferEpochs,
		IsNewDataSet:      allNewDataSets,
	})

	needsApproval := !isFWSSMaxApproved(
		approval.IsApproved,
		approval.RateAllowance,
		approval.LockupAllowance,
		approval.MaxLockupPeriod,
		requiredLockupPeriod,
	)
	ready := depositNeeded.Sign() == 0 && !needsApproval

	totalFees := new(big.Int).Add(totalCreateDataSetFee, totalAddPiecesFee)
	aggregateLockup := aggregateLockup(
		totalRateDelta,
		totalStreamingLockup,
		totalLifecycleLockup,
		totalReserveReplenishment,
		totalCDNLockup,
		totalCacheMissLockup,
		totalLockup,
	)

	return &MultiContextCosts{
		RatePerEpoch:         totalRatePerEpoch,
		RatePerMonth:         totalRatePerMonth,
		Fees:                 UploadFees{CreateDataSetFee: totalCreateDataSetFee, AddPiecesFee: totalAddPiecesFee, Total: totalFees},
		Lockup:               aggregateLockup,
		DepositNeeded:        depositNeeded,
		RequiredLockupPeriod: requiredLockupPeriod,
		NeedsFWSSMaxApproval: needsApproval,
		Ready:                ready,
	}, nil
}
