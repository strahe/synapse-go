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
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/warmstorage"
)

// ContractCaller is the subset of ethereum.ContractCaller needed by Service.
type ContractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
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
}

// NewService constructs a Service using addresses from c.Addresses().
func NewService(
	c chain.Chain,
	ws WarmStorageReader,
	pay PaymentsReader,
	caller ContractCaller,
	opts ...Option,
) (*Service, error) {
	if ws == nil {
		return nil, fmt.Errorf("costs.NewService: ws is nil")
	}
	if pay == nil {
		return nil, fmt.Errorf("costs.NewService: pay is nil")
	}
	if caller == nil {
		return nil, fmt.Errorf("costs.NewService: caller is nil")
	}
	addrs := c.Addresses()
	if addrs.FWSS == (common.Address{}) {
		return nil, fmt.Errorf("costs.NewService: unsupported chain %v", c)
	}
	if addrs.USDFC == (common.Address{}) {
		return nil, fmt.Errorf("costs.NewService: unsupported chain %v: missing USDFC address", c)
	}
	if addrs.PDPVerifier == (common.Address{}) {
		return nil, fmt.Errorf("costs.NewService: unsupported chain %v: missing PDPVerifier address", c)
	}

	svc := &Service{
		c:           c,
		ws:          ws,
		pay:         pay,
		caller:      caller,
		pdpVerifier: addrs.PDPVerifier,
		usdfc:       addrs.USDFC,
		fwss:        addrs.FWSS,
	}
	for _, o := range opts {
		o(svc)
	}
	return svc, nil
}

// Option customises a Service.
type Option func(*Service)

// WithLogger sets the structured logger. If nil, logging is silent.
func WithLogger(l *slog.Logger) Option {
	return func(s *Service) { s.logger = l }
}

// computeDebt returns max(0, LockupCurrent - Funds) for the given account state.
func computeDebt(account *payments.AccountState) *big.Int {
	if account.LockupCurrent != nil && account.Funds != nil &&
		account.LockupCurrent.Cmp(account.Funds) > 0 {
		return new(big.Int).Sub(account.LockupCurrent, account.Funds)
	}
	return new(big.Int)
}

// GetServicePrice delegates to the warmstorage service.
func (s *Service) GetServicePrice(ctx context.Context) (*warmstorage.ServicePrice, error) {
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
	if opts == nil {
		opts = &UploadCostOptions{}
	}
	runwayEpochs := opts.RunwayEpochs
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

	// Compute debt = max(0, LockupCurrent - Funds).
	debt := computeDebt(account)

	avail := account.AvailableFunds()
	if avail == nil {
		avail = new(big.Int)
	}

	currentRate := account.LockupRate
	if currentRate == nil {
		currentRate = new(big.Int)
	}

	depositNeeded := CalculateDepositNeeded(
		lockup.TotalLockup,
		lockup.RateDelta,
		currentRate,
		debt,
		avail,
		runwayEpochs,
		bufferEpochs,
		opts.IsNewDataSet,
	)

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
func (s *Service) GetAccountSummary(ctx context.Context, owner common.Address) (*AccountSummary, error) {
	account, err := s.pay.AccountInfo(ctx, s.usdfc, owner)
	if err != nil {
		return nil, fmt.Errorf("costs.GetAccountSummary: %w", err)
	}

	debt := computeDebt(account)

	avail := account.AvailableFunds()
	if avail == nil {
		avail = new(big.Int)
	}

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
		Funds:              funds,
		AvailableFunds:     avail,
		Debt:               debt,
		LockupRatePerEpoch: rate,
		LockupRatePerMonth: ratePerMonth,
		FundedUntilEpoch:   account.FundedUntilEpoch,
		CurrentEpoch:       chain.CurrentEpoch(s.c),
	}, nil
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
		panic("costs: failed to parse USDFC_SYBIL_FEE ABI: " + err.Error())
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
