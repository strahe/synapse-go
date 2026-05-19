package costs

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/warmstorage"
)

// MultiContextRef describes one upload target for
// [Service.CalculateMultiContextCosts]. Each ref yields its own lockup
// contribution; account-level debt, runway and buffer are computed once
// across the aggregate.
type MultiContextRef struct {
	// IsNewDataSet is true when the target will create a new data set
	// on this provider (contributes Sybil fee and optional CDN fixed
	// lockup). When false, CurrentDataSetSizeBytes is consulted so the
	// marginal rate is computed above the existing floor.
	IsNewDataSet bool

	// CurrentDataSetSizeBytes is the current on-chain size of the
	// existing data set (zero or nil when IsNewDataSet is true). When
	// unknown, pass nil and the floor-price rate is used.
	CurrentDataSetSizeBytes *big.Int

	// WithCDN toggles CDN fixed lockup for this target. Only meaningful
	// when IsNewDataSet is true — CDN lockup is only charged on creation.
	WithCDN bool
}

// MultiContextCosts is the aggregate cost view across multiple upload
// targets: a single DepositNeeded / NeedsFWSSMaxApproval / Ready, plus
// summed RatePerEpoch / RatePerMonth covering all refs.
type MultiContextCosts struct {
	// RatePerEpoch is the sum of per-context effective rates (post-upload).
	RatePerEpoch *big.Int
	// RatePerMonth is RatePerEpoch * EpochsPerMonth.
	RatePerMonth *big.Int
	// DepositNeeded is the single USDFC deposit covering all contexts.
	DepositNeeded *big.Int
	// NeedsFWSSMaxApproval is true when the FWSS operator does not yet
	// hold max approval for the payer.
	NeedsFWSSMaxApproval bool
	// Ready is true when DepositNeeded is zero and FWSS approval is set.
	Ready bool
}

// CalculateMultiContextCosts aggregates upload costs across multiple
// prospective contexts for a single uploaded payload of dataSizeBytes.
//
// Each ref contributes its own lockup; debt, runway and buffer are computed
// once from the payer's account state.
//
// Default buffer/runway follow DefaultBufferEpochs / DefaultExtraRunwayEpochs
// when the corresponding opts field is zero.
func (s *Service) CalculateMultiContextCosts(
	ctx context.Context,
	payer common.Address,
	dataSizeBytes *big.Int,
	refs []MultiContextRef,
	opts *UploadCostOptions,
) (*MultiContextCosts, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: refs is empty")
	}
	if dataSizeBytes == nil {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: dataSizeBytes is nil")
	}
	if opts == nil {
		opts = &UploadCostOptions{}
	}
	runwayEpochs := opts.ExtraRunwayEpochs
	bufferEpochs := opts.BufferEpochs
	if bufferEpochs == 0 {
		bufferEpochs = DefaultBufferEpochs
	}

	var (
		pricing       *warmstorage.ServicePrice
		account       *payments.AccountState
		approval      *payments.OperatorApproval
		usdfcSybilFee *big.Int
		mu            sync.Mutex
		errs          []error
		wg            sync.WaitGroup
	)

	wg.Add(4)

	go func() {
		defer wg.Done()
		p, err := s.ws.GetServicePrice(ctx)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("GetServicePrice: %w", err))
			return
		}
		pricing = p
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

	go func() {
		defer wg.Done()
		fee, err := s.readUsdfcSybilFee(ctx)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("USDFC_SYBIL_FEE: %w", err))
			return
		}
		usdfcSybilFee = fee
	}()

	wg.Wait()

	if len(errs) > 0 {
		return nil, fmt.Errorf("costs.CalculateMultiContextCosts: %w", errors.Join(errs...))
	}

	var epm int64
	if pricing.EpochsPerMonth != nil && pricing.EpochsPerMonth.Sign() > 0 {
		epm = pricing.EpochsPerMonth.Int64()
	}

	totalRateDelta := new(big.Int)
	totalLockup := new(big.Int)
	totalRatePerEpoch := new(big.Int)
	totalRatePerMonth := new(big.Int)
	allNewDataSets := true

	for i := range refs {
		ref := &refs[i]
		if !ref.IsNewDataSet {
			allNewDataSets = false
		}
		currentSize := ref.CurrentDataSetSizeBytes
		if currentSize == nil {
			currentSize = new(big.Int)
		}

		lockup := CalculateAdditionalLockupRequired(
			dataSizeBytes,
			currentSize,
			pricing,
			DefaultLockupPeriod,
			usdfcSybilFee,
			ref.IsNewDataSet,
			ref.WithCDN,
		)
		totalRateDelta.Add(totalRateDelta, lockup.RateDelta)
		totalLockup.Add(totalLockup, lockup.TotalLockup)

		totalSize := new(big.Int).Add(currentSize, dataSizeBytes)
		rate := CalculateEffectiveRate(
			totalSize,
			pricing.PricePerTiBPerMonthNoCDN,
			pricing.MinimumPricePerMonth,
			epm,
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
		ExtraRunwayEpochs: runwayEpochs,
		BufferEpochs:      bufferEpochs,
		IsNewDataSet:      allNewDataSets,
	})

	needsApproval := !isFWSSMaxApproved(
		approval.IsApproved,
		approval.RateAllowance,
		approval.LockupAllowance,
		approval.MaxLockupPeriod,
	)
	ready := depositNeeded.Sign() == 0 && !needsApproval

	return &MultiContextCosts{
		RatePerEpoch:         totalRatePerEpoch,
		RatePerMonth:         totalRatePerMonth,
		DepositNeeded:        depositNeeded,
		NeedsFWSSMaxApproval: needsApproval,
		Ready:                ready,
	}, nil
}
