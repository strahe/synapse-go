package costs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/warmstorage"
)

// ContractCaller is the chain reader accepted by Service. CallContract remains
// part of the public interface for compatibility; current cost calculations
// only use BlockNumber.
type ContractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// WarmStorageReader is the subset of warmstorage.Service used by costs.
type WarmStorageReader interface {
	GetPriceList(ctx context.Context) (*warmstorage.PriceList, error)
}

//nolint:staticcheck // Compatibility surface for the deprecated ServicePrice API.
type legacyServicePriceReader interface {
	GetServicePrice(ctx context.Context) (*warmstorage.ServicePrice, error)
}

// PaymentsReader is the subset of payments.Service used by costs.
type PaymentsReader interface {
	AccountInfo(ctx context.Context, token, owner common.Address) (*payments.AccountState, error)
	ServiceApproval(ctx context.Context, token, client, operator common.Address) (*payments.OperatorApproval, error)
}

// Service computes upload costs and account summaries for the FWSS ecosystem.
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
	// non-nil value must be ready for use; a typed-nil implementation is invalid.
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
		lifecycle: opts.Lifecycle,
	}, nil
}

// GetServicePrice delegates to the warmstorage service.
//
// Deprecated: Use GetPriceList.
func (s *Service) GetServicePrice(ctx context.Context) (*warmstorage.ServicePrice, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	reader, ok := s.ws.(legacyServicePriceReader)
	if !ok {
		return nil, fmt.Errorf("costs.GetServicePrice: warmstorage reader does not support legacy GetServicePrice")
	}
	return reader.GetServicePrice(ctx)
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
// payer is the client address. dataSizeBytes is the size of the new data.
// opts may be nil (defaults apply). opts.CurrentDataSetSizeBytes defaults to zero.
func (s *Service) GetUploadCosts(
	ctx context.Context,
	payer common.Address,
	dataSizeBytes *big.Int,
	opts *UploadCostOptions,
) (*UploadCosts, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &UploadCostOptions{}
	}
	runwayEpochs := opts.ExtraRunwayEpochs
	bufferEpochs := opts.BufferEpochs
	if bufferEpochs == 0 {
		bufferEpochs = DefaultBufferEpochs
	}
	currentDataSetSize := opts.CurrentDataSetSizeBytes
	if currentDataSetSize == nil {
		currentDataSetSize = new(big.Int)
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
		return nil, fmt.Errorf("costs.GetUploadCosts: %w", errors.Join(errs...))
	}
	if priceList == nil {
		priceList = &warmstorage.PriceList{}
	}

	rate := CalculateEffectiveRate(
		new(big.Int).Add(currentDataSetSize, dataSizeBytes),
		priceList.Rates.StoragePerTiBPerMonth,
		priceList.Rates.DatasetFeePerMonth,
		chain.EpochsPerMonth,
	)

	pieceCount := opts.PieceCount
	if pieceCount == nil {
		pieceCount = bigOne
	}
	fees := CalculateUploadFees(priceList, opts.IsNewDataSet, pieceCount)
	requiredLockupPeriod := requiredLockupPeriod(priceList)
	lockup := CalculateAdditionalLockupRequired(
		dataSizeBytes,
		currentDataSetSize,
		priceList,
		requiredLockupPeriod,
		opts.IsNewDataSet,
		opts.EnableCDN,
	)

	currentEpoch, err := s.currentEpoch(ctx)
	if err != nil {
		return nil, fmt.Errorf("costs.GetUploadCosts: %w", err)
	}
	resolved := account.ResolveAt(currentEpoch)
	debt := account.DebtAt(currentEpoch)
	avail := resolved.AvailableFunds

	currentRate := account.LockupRate
	if currentRate == nil {
		currentRate = new(big.Int)
	}

	depositNeeded := CalculateDepositNeeded(DepositCalculation{
		AdditionalLockup:  lockup.Total,
		Fees:              fees.Total,
		RateDelta:         lockup.RateDeltaPerEpoch,
		CurrentLockupRate: currentRate,
		Debt:              debt,
		AvailableFunds:    avail,
		RunwayInEpochs:    resolved.RunwayInEpochs,
		ExtraRunwayEpochs: runwayEpochs,
		BufferEpochs:      bufferEpochs,
		IsNewDataSet:      opts.IsNewDataSet,
	})

	needsApproval := !isFWSSMaxApproved(
		approval.IsApproved,
		approval.RateAllowance,
		approval.LockupAllowance,
		approval.MaxLockupPeriod,
		requiredLockupPeriod,
	)
	ready := depositNeeded.Sign() == 0 && !needsApproval

	return &UploadCosts{
		Rate:                 rate,
		Fees:                 fees,
		Lockup:               lockup,
		DepositNeeded:        depositNeeded,
		RequiredLockupPeriod: requiredLockupPeriod,
		NeedsFWSSMaxApproval: needsApproval,
		Ready:                ready,
	}, nil
}

// GetAccountSummary returns a payment health snapshot for the given owner.
//
// Deprecated: Use payments.Service.AccountSummary for payment account state.
// This method is kept for compatibility.
func (s *Service) GetAccountSummary(ctx context.Context, owner common.Address) (*AccountSummary, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	account, err := s.pay.AccountInfo(ctx, s.usdfc, owner)
	if err != nil {
		return nil, fmt.Errorf("costs.GetAccountSummary: %w", err)
	}

	currentEpoch, err := s.currentEpoch(ctx)
	if err != nil {
		return nil, fmt.Errorf("costs.GetAccountSummary: %w", err)
	}
	resolved := account.ResolveAt(currentEpoch)
	debt := account.DebtAt(currentEpoch)
	avail := resolved.AvailableFunds

	funds := new(big.Int)
	if account.Funds != nil {
		funds.Set(account.Funds)
	}

	rate := account.LockupRate
	if rate == nil {
		rate = new(big.Int)
	}

	ratePerMonth := new(big.Int).Mul(rate, big.NewInt(chain.EpochsPerMonth))

	return &AccountSummary{
		Funds:                 funds,
		AvailableFunds:        avail,
		Debt:                  debt,
		LockupRatePerEpoch:    rate,
		LockupRatePerMonth:    ratePerMonth,
		FundedUntilEpoch:      account.FundedUntilEpoch,
		RunwayInEpochs:        resolved.RunwayInEpochs,
		GrossCoverageInEpochs: resolved.GrossCoverageInEpochs,
		CurrentEpoch:          currentEpoch,
	}, nil
}

func (s *Service) currentEpoch(ctx context.Context) (*big.Int, error) {
	block, err := s.caller.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("block number: %w", err)
	}
	return new(big.Int).SetUint64(block), nil
}
