package costs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/warmstorage"
)

// ContractCaller is the subset of ethereum.ContractCaller needed by Service.
type ContractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// WarmStorageReader is the subset of warmstorage.Service used by costs.
type WarmStorageReader interface {
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
	c           chain.Chain
	ws          WarmStorageReader
	pay         PaymentsReader
	caller      ContractCaller
	pdpVerifier common.Address
	usdfc       common.Address
	fwss        common.Address
	logger      *slog.Logger
	lifecycle   *lifecycle.Lifecycle
}

// Options configures a [Service].
type Options struct {
	// Chain selects the network whose contract addresses are used.
	// Zero value is chain.Mainnet.
	Chain chain.Chain

	// WarmStorage reads on-chain service pricing. Required.
	WarmStorage WarmStorageReader

	// Payments reads account and allowance state. Required.
	Payments PaymentsReader

	// Caller issues eth_call against the configured chain. Required.
	Caller ContractCaller

	// Logger is the structured logger. If nil, logging is silent.
	Logger *slog.Logger

	// Lifecycle, when non-nil, ties this Service to the owning Client's
	// close state. After the Lifecycle is closed, every method returns
	// ErrClosed. Nil is allowed for standalone use.
	Lifecycle *lifecycle.Lifecycle
}

// New constructs a [Service] using addresses from opts.Chain.Addresses().
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
	if addrs.FWSS == (common.Address{}) {
		return nil, fmt.Errorf("costs.New: %w: %v", chain.ErrUnknownChain, opts.Chain)
	}
	if addrs.USDFC == (common.Address{}) {
		return nil, fmt.Errorf("costs.New: %w: %v: missing USDFC address", chain.ErrUnknownChain, opts.Chain)
	}
	if addrs.PDPVerifier == (common.Address{}) {
		return nil, fmt.Errorf("costs.New: %w: %v: missing PDPVerifier address", chain.ErrUnknownChain, opts.Chain)
	}

	return &Service{
		c:           opts.Chain,
		ws:          opts.WarmStorage,
		pay:         opts.Payments,
		caller:      opts.Caller,
		pdpVerifier: addrs.PDPVerifier,
		usdfc:       addrs.USDFC,
		fwss:        addrs.FWSS,
		logger:      opts.Logger,
		lifecycle:   opts.Lifecycle,
	}, nil
}

// GetServicePrice delegates to the warmstorage service.
func (s *Service) GetServicePrice(ctx context.Context) (*warmstorage.ServicePrice, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	return s.ws.GetServicePrice(ctx)
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
		return nil, fmt.Errorf("costs.GetUploadCosts: %w", errors.Join(errs...))
	}

	var epm int64
	if pricing.EpochsPerMonth != nil && pricing.EpochsPerMonth.Sign() > 0 {
		epm = pricing.EpochsPerMonth.Int64()
	}
	rate := CalculateEffectiveRate(
		new(big.Int).Add(currentDataSetSize, dataSizeBytes),
		pricing.PricePerTiBPerMonthNoCDN,
		pricing.MinimumPricePerMonth,
		epm,
	)

	lockup := CalculateAdditionalLockupRequired(
		dataSizeBytes,
		currentDataSetSize,
		pricing,
		DefaultLockupPeriod,
		usdfcSybilFee,
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
		AdditionalLockup:  lockup.TotalLockup,
		RateDelta:         lockup.RateDelta,
		CurrentLockupRate: currentRate,
		Debt:              debt,
		AvailableFunds:    avail,
		ExtraRunwayEpochs: runwayEpochs,
		BufferEpochs:      bufferEpochs,
		IsNewDataSet:      opts.IsNewDataSet,
	})

	needsApproval := !isFWSSMaxApproved(
		approval.IsApproved,
		approval.RateAllowance,
		approval.LockupAllowance,
		approval.MaxLockupPeriod,
	)
	ready := depositNeeded.Sign() == 0 && !needsApproval

	return &UploadCosts{
		Rate:                 rate,
		Lockup:               lockup,
		DepositNeeded:        depositNeeded,
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

const usdfcSybilFeeABIJSON = `[{
	"type": "function",
	"name": "USDFC_SYBIL_FEE",
	"inputs": [],
	"outputs": [{"name": "", "type": "uint256"}],
	"stateMutability": "view"
}]`

var usdfcSybilFeeABI abi.ABI

func init() {
	var err error
	usdfcSybilFeeABI, err = abi.JSON(strings.NewReader(usdfcSybilFeeABIJSON))
	if err != nil {
		panic("costs: failed to parse USDFC_SYBIL_FEE ABI: " + err.Error()) //nolint:forbidigo // init() ABI parse: error implies a build/codegen bug, not a runtime condition
	}
}

func (s *Service) readUsdfcSybilFee(ctx context.Context) (*big.Int, error) {
	data, err := usdfcSybilFeeABI.Pack("USDFC_SYBIL_FEE")
	if err != nil {
		return nil, fmt.Errorf("costs.readUsdfcSybilFee: pack: %w", err)
	}

	result, err := s.caller.CallContract(ctx, ethereum.CallMsg{
		To:   &s.pdpVerifier,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("costs.readUsdfcSybilFee: call: %w", err)
	}

	values, err := usdfcSybilFeeABI.Unpack("USDFC_SYBIL_FEE", result)
	if err != nil {
		return nil, fmt.Errorf("costs.readUsdfcSybilFee: unpack: %w", err)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("costs.readUsdfcSybilFee: empty result")
	}

	fee, ok := values[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("costs.readUsdfcSybilFee: unexpected type %T", values[0])
	}
	return fee, nil
}
