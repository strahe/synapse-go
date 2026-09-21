package costs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/ifaceutil"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/warmstorage"
)

// ContractCaller provides the chain block number used by Service. The root
// Client uses its configured Ethereum client.
type ContractCaller interface {
	BlockNumber(ctx context.Context) (uint64, error)
}

// WarmStorageReader provides the storage price list used by Service. The root
// Client supplies warmstorage.Service.
type WarmStorageReader interface {
	GetPriceList(ctx context.Context) (*warmstorage.PriceList, error)
}

// PaymentsReader provides payment account and operator approval state for cost
// estimates. The root Client supplies payments.Service.
type PaymentsReader interface {
	AccountInfo(ctx context.Context, token, owner common.Address) (*payments.AccountState, error)
	ServiceApproval(ctx context.Context, token, client, operator common.Address) (*payments.OperatorApproval, error)
}

// Service computes upload costs for the FWSS ecosystem.
// All methods are safe for concurrent use.
type Service struct {
	c         chain.Chain
	ws        WarmStorageReader
	pay       PaymentsReader
	caller    ContractCaller
	usdfc     common.Address
	fwss      common.Address
	logger    *slog.Logger
	lifecycle interface{ CheckClosed() error }
}

// Options configures a [Service].
type Options struct {
	// Chain selects the network whose contract addresses are used.
	// Zero value is chain.Mainnet. Explicit addresses below override the
	// chain registry for callers that resolve current contracts dynamically.
	Chain chain.Chain

	// USDFCTokenAddress is the payment token used for account and approval
	// reads. Zero falls back to Chain.Addresses().USDFC.
	USDFCTokenAddress common.Address

	// WarmStorageAddress is the FWSS operator used for approval reads. Zero
	// falls back to Chain.Addresses().FWSS.
	WarmStorageAddress common.Address

	// WarmStorage reads on-chain service pricing. Required.
	WarmStorage WarmStorageReader

	// Payments reads account and allowance state. Required.
	Payments PaymentsReader

	// Caller provides chain reads for cost calculations. Required.
	Caller ContractCaller

	// Logger is the structured logger. If nil, logging is silent.
	Logger *slog.Logger

	// Lifecycle is checked before service operations that can touch configured
	// backends. Any error returned by CheckClosed is returned without touching
	// those backends. The root synapse Client injects a shared checker whose
	// closed error matches ErrClosed. Nil is allowed for standalone use. A
	// typed-nil value is treated as nil.
	Lifecycle interface{ CheckClosed() error }
}

// New constructs a [Service].
// WarmStorage, Payments and Caller must be non-nil.
func New(opts Options) (*Service, error) {
	if opts.WarmStorage == nil {
		return nil, fmt.Errorf("costs.New: WarmStorage is nil")
	}
	if opts.Payments == nil {
		return nil, fmt.Errorf("costs.New: Payments is nil")
	}
	if opts.Caller == nil {
		return nil, fmt.Errorf("costs.New: Caller is nil")
	}
	addrs := opts.Chain.Addresses()
	fwss := opts.WarmStorageAddress
	if fwss == (common.Address{}) {
		fwss = addrs.FWSS
	}
	if fwss == (common.Address{}) {
		return nil, fmt.Errorf("costs.New: %w: %v", chain.ErrUnknownChain, opts.Chain)
	}
	usdfc := opts.USDFCTokenAddress
	if usdfc == (common.Address{}) {
		usdfc = addrs.USDFC
	}
	if usdfc == (common.Address{}) {
		return nil, fmt.Errorf("costs.New: %w: %v: missing USDFC address", chain.ErrUnknownChain, opts.Chain)
	}
	return &Service{
		c:         opts.Chain,
		ws:        opts.WarmStorage,
		pay:       opts.Payments,
		caller:    opts.Caller,
		usdfc:     usdfc,
		fwss:      fwss,
		logger:    opts.Logger,
		lifecycle: ifaceutil.NormalizeNil(opts.Lifecycle),
	}, nil
}

// GetPriceList delegates to the warmstorage service.
func (s *Service) GetPriceList(ctx context.Context) (*warmstorage.PriceList, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	return s.ws.GetPriceList(ctx)
}

// GetUploadCosts returns cost and deposit information for an upload.
//
// payer is the client address. pieceSizes contains each piece's raw payload
// size and must be non-empty; every size must be between chain.MinUploadSize
// and chain.MaxUploadSize. New datasets require opts.IsNewDataSet=true;
// existing datasets require a non-negative leaf count and complete lifecycle
// reserve state. Nil or empty opts therefore returns ErrInvalidArgument.
func (s *Service) GetUploadCosts(
	ctx context.Context,
	payer common.Address,
	pieceSizes []uint64,
	opts *UploadCostOptions,
) (*UploadCosts, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &UploadCostOptions{}
	}
	if err := validatePieceSizes(pieceSizes); err != nil {
		return nil, fmt.Errorf("costs.GetUploadCosts: %w", err)
	}
	if opts.ExtraRunwayEpochs < 0 {
		return nil, fmt.Errorf("costs.GetUploadCosts: %w: ExtraRunwayEpochs must be non-negative", ErrInvalidArgument)
	}
	bufferEpochs, err := resolveBufferEpochs(opts.BufferEpochs)
	if err != nil {
		return nil, fmt.Errorf("costs.GetUploadCosts: %w", err)
	}
	dataSetState, err := resolveDataSetCostState(
		opts.IsNewDataSet,
		opts.CurrentDataSetLeafCount,
		opts.CurrentLifecycleReserveBalance,
		opts.PendingOneTimePayments,
		opts.PDPEndEpoch,
	)
	if err != nil {
		return nil, fmt.Errorf("costs.GetUploadCosts: %w", err)
	}

	totals, err := s.calculateUploadCosts(
		ctx,
		payer,
		slices.Clone(pieceSizes),
		[]uploadCostTarget{{isNewDataSet: opts.IsNewDataSet, withCDN: opts.EnableCDN, state: dataSetState}},
		opts.ExtraRunwayEpochs,
		bufferEpochs,
	)
	if err != nil {
		return nil, fmt.Errorf("costs.GetUploadCosts: %w", err)
	}
	return &UploadCosts{
		Rate:                 totals.rate,
		Fees:                 totals.fees,
		Lockup:               totals.lockup,
		DepositNeeded:        totals.depositNeeded,
		RequiredLockupPeriod: totals.requiredLockupPeriod,
		NeedsFWSSMaxApproval: totals.needsApproval,
		Ready:                totals.ready(),
	}, nil
}

// uploadCostTarget is one validated upload target. For a new data set, state
// holds zero values.
type uploadCostTarget struct {
	isNewDataSet bool
	withCDN      bool
	state        resolvedDataSetCostState
}

// uploadCostTotals sums per-target rates, fees and lockups. Debt, runway and
// buffer are applied once to the payer account.
type uploadCostTotals struct {
	rate                 EffectiveRate
	fees                 UploadFees
	lockup               AdditionalLockup
	depositNeeded        *big.Int
	requiredLockupPeriod *big.Int
	needsApproval        bool
}

func (t uploadCostTotals) ready() bool {
	return t.depositNeeded.Sign() == 0 && !t.needsApproval
}

// calculateUploadCosts prices one upload of pieceSizes on every target. Inputs
// must already be validated, and the caller owns pieceSizes and targets.
func (s *Service) calculateUploadCosts(
	ctx context.Context,
	payer common.Address,
	pieceSizes []uint64,
	targets []uploadCostTarget,
	extraRunwayEpochs int64,
	bufferEpochs int64,
) (uploadCostTotals, error) {
	priceList, account, approval, err := s.readUploadCostInputs(ctx, payer)
	if err != nil {
		return uploadCostTotals{}, err
	}

	rateDelta := new(big.Int)
	streamingLockup := new(big.Int)
	lifecycleLockup := new(big.Int)
	reserveReplenishment := new(big.Int)
	cdnLockup := new(big.Int)
	cacheMissLockup := new(big.Int)
	totalLockup := new(big.Int)
	ratePerEpoch := new(big.Int)
	ratePerMonth := new(big.Int)
	createDataSetFee := new(big.Int)
	addPiecesFee := new(big.Int)
	allNewDataSets := true
	requiredLockupPeriod := requiredLockupPeriod(priceList)
	addedLeaves := pieceSizesToLeafCount(pieceSizes)

	for i, target := range targets {
		if !target.isNewDataSet {
			allNewDataSets = false
		}
		lockup := calculateAdditionalLockupRequired(
			addedLeaves,
			target.state.leaves,
			priceList,
			requiredLockupPeriod,
			target.isNewDataSet,
			target.withCDN,
		)
		fees, err := CalculateUploadFees(priceList, target.isNewDataSet, pieceSizes)
		if err != nil {
			return uploadCostTotals{}, fmt.Errorf("refs[%d]: %w", i, err)
		}
		reserveFunding, err := CalculateLifecycleReserveFunding(LifecycleReserveCalculation{
			PriceList:                      priceList,
			PieceSizes:                     pieceSizes,
			IsNewDataSet:                   target.isNewDataSet,
			CurrentLifecycleReserveBalance: target.state.reserveBalance,
			PendingOneTimePayments:         target.state.pendingPayments,
		})
		if err != nil {
			return uploadCostTotals{}, fmt.Errorf("refs[%d]: %w", i, err)
		}
		rate := CalculateEffectiveRate(
			leafCountToBillableBytes(new(big.Int).Add(target.state.leaves, addedLeaves)),
			priceList.Rates.StoragePerTiBPerMonth,
			priceList.Rates.DatasetFeePerMonth,
			chain.EpochsPerMonth,
		)

		rateDelta.Add(rateDelta, lockup.RateDeltaPerEpoch)
		streamingLockup.Add(streamingLockup, lockup.StreamingLockup)
		lifecycleLockup.Add(lifecycleLockup, lockup.LifecycleLockup)
		reserveReplenishment.Add(reserveReplenishment, reserveFunding.ReserveReplenishment)
		cdnLockup.Add(cdnLockup, lockup.CDNLockup)
		cacheMissLockup.Add(cacheMissLockup, lockup.CacheMissLockup)
		totalLockup.Add(totalLockup, lockup.Total)
		totalLockup.Add(totalLockup, reserveFunding.ReserveReplenishment)
		ratePerEpoch.Add(ratePerEpoch, rate.RatePerEpoch)
		ratePerMonth.Add(ratePerMonth, rate.RatePerMonth)
		createDataSetFee.Add(createDataSetFee, fees.CreateDataSetFee)
		addPiecesFee.Add(addPiecesFee, fees.AddPiecesFee)
	}

	currentEpoch, err := s.currentEpoch(ctx)
	if err != nil {
		return uploadCostTotals{}, err
	}
	resolved := account.ResolveAt(currentEpoch)
	currentRate := account.LockupRate
	if currentRate == nil {
		currentRate = new(big.Int)
	}
	depositNeeded := CalculateDepositNeeded(DepositCalculation{
		AdditionalLockup:  totalLockup,
		RateDelta:         rateDelta,
		CurrentLockupRate: currentRate,
		Debt:              account.DebtAt(currentEpoch),
		AvailableFunds:    resolved.AvailableFunds,
		RunwayInEpochs:    resolved.RunwayInEpochs,
		ExtraRunwayEpochs: extraRunwayEpochs,
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

	return uploadCostTotals{
		rate: EffectiveRate{RatePerEpoch: ratePerEpoch, RatePerMonth: ratePerMonth},
		fees: UploadFees{
			CreateDataSetFee: createDataSetFee,
			AddPiecesFee:     addPiecesFee,
			Total:            new(big.Int).Add(createDataSetFee, addPiecesFee),
		},
		lockup: AdditionalLockup{
			RateDeltaPerEpoch:    rateDelta,
			StreamingLockup:      streamingLockup,
			LifecycleLockup:      lifecycleLockup,
			ReserveReplenishment: reserveReplenishment,
			CDNLockup:            cdnLockup,
			CacheMissLockup:      cacheMissLockup,
			Total:                totalLockup,
		},
		depositNeeded:        depositNeeded,
		requiredLockupPeriod: requiredLockupPeriod,
		needsApproval:        needsApproval,
	}, nil
}

// readUploadCostInputs reads the price list, payer account and FWSS approval
// concurrently. A nil price list is treated as zero-value prices.
func (s *Service) readUploadCostInputs(
	ctx context.Context,
	payer common.Address,
) (*warmstorage.PriceList, *payments.AccountState, *payments.OperatorApproval, error) {
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
		return nil, nil, nil, errors.Join(errs...)
	}
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}
	return priceList, account, approval, nil
}

func (s *Service) currentEpoch(ctx context.Context) (*big.Int, error) {
	block, err := s.caller.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("block number: %w", err)
	}
	return new(big.Int).SetUint64(block), nil
}

func resolveBufferEpochs(bufferEpochs *int64) (int64, error) {
	if bufferEpochs == nil {
		return DefaultBufferEpochs, nil
	}
	if *bufferEpochs < 0 {
		return 0, fmt.Errorf("%w: BufferEpochs must be non-negative", ErrInvalidArgument)
	}
	return *bufferEpochs, nil
}
