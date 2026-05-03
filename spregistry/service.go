package spregistry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	spr "github.com/strahe/synapse-go/internal/contracts/spregistry"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

// EthClient is the minimal RPC surface the service needs. Tests can substitute
// a mock that implements bind.ContractCaller.
type EthClient interface {
	bind.ContractCaller
}

// Backend extends EthClient with the surface required for sending
// transactions (register/update/remove provider, add/update/remove product).
// The full *ethclient.Client satisfies this interface.
type Backend interface {
	bind.ContractBackend
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*ethtypes.Receipt, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// Service provides read access to the ServiceProviderRegistry contract,
// and (when constructed with a Signer + Backend) a small, typed set of
// state-changing provider-management methods.
type Service struct {
	caller      EthClient
	backend     Backend
	chainID     types.ChainID
	addr        common.Address
	contract    *spr.SPRegistryCaller
	write       *spr.SPRegistryTransactor
	signer      signer.EVMSigner
	nonces      *txutil.NonceManager
	logger      *slog.Logger
	receiptWait time.Duration
	lifecycle   *lifecycle.Lifecycle
}

// Options configures the service.
type Options struct {
	Client  EthClient
	Address common.Address
	// ChainID is required only when writes are used.
	ChainID types.ChainID
	// Backend provides a full RPC surface for writes. When nil Service is
	// read-only; write methods return ErrWriteNotConfigured.
	Backend Backend
	// Signer signs write transactions. Optional.
	Signer signer.EVMSigner
	// NonceManager is optional. The root synapse Client injects a shared
	// coordinator across all write-capable services; standalone callers may
	// leave this nil to create one when Backend and Signer are both set.
	NonceManager *txutil.NonceManager
	// Logger is optional.
	Logger *slog.Logger
	// ReceiptWait overrides the default receipt polling timeout for
	// WithWait calls. Zero uses txutil.DefaultReceiptWaitConfig.
	ReceiptWait time.Duration
	// Lifecycle, when non-nil, ties this Service to the owning Client's
	// close state. After the Lifecycle is closed, every method returns
	// ErrClosed. Nil is allowed for standalone use.
	Lifecycle *lifecycle.Lifecycle
}

// New creates a Service bound to the given registry address.
func New(opts Options) (*Service, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("spregistry.New: %w: nil Client", ErrInvalidArgument)
	}
	if (opts.Address == common.Address{}) {
		return nil, fmt.Errorf("spregistry.New: %w: zero Address", ErrInvalidArgument)
	}
	if opts.Backend != nil && opts.Signer != nil && !opts.ChainID.IsValid() {
		return nil, fmt.Errorf("spregistry.New: %w: ChainID is required when writes are enabled (Backend+Signer provided)", ErrInvalidArgument)
	}
	c, err := spr.NewSPRegistryCaller(opts.Address, opts.Client)
	if err != nil {
		return nil, fmt.Errorf("spregistry.New: bind: %w", err)
	}
	s := &Service{
		caller:      opts.Client,
		backend:     opts.Backend,
		chainID:     opts.ChainID,
		addr:        opts.Address,
		contract:    c,
		signer:      opts.Signer,
		nonces:      opts.NonceManager,
		logger:      opts.Logger,
		receiptWait: opts.ReceiptWait,
		lifecycle:   opts.Lifecycle,
	}
	if opts.Backend != nil {
		tw, err := spr.NewSPRegistryTransactor(opts.Address, opts.Backend)
		if err != nil {
			return nil, fmt.Errorf("spregistry.New: bind transactor: %w", err)
		}
		s.write = tw
	}
	if s.nonces == nil && s.signer != nil && s.backend != nil {
		s.nonces = txutil.NewNonceManager(opts.Backend, s.signer.EVMAddress())
	}
	return s, nil
}

// Address returns the configured registry contract address.
func (s *Service) Address() common.Address { return s.addr }

// requireSigner returns ErrWriteNotConfigured unless the service was built
// with the dependencies needed to submit transactions.
func (s *Service) requireSigner() error {
	if s.signer == nil || s.backend == nil || s.write == nil || s.nonces == nil {
		return ErrWriteNotConfigured
	}
	return nil
}

// newTransactOpts obtains a fresh bind.TransactOpts bound to ctx and a
// nonce allocated from the shared NonceManager. The returned release
// function must be called exactly once.
func (s *Service) newTransactOpts(ctx context.Context) (*bind.TransactOpts, func(), error) {
	topts, err := s.signer.Transactor(s.chainID.BigInt())
	if err != nil {
		return nil, nil, fmt.Errorf("transactor: %w", err)
	}
	topts.Context = ctx
	nonce, release, err := s.nonces.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("nonce: %w", err)
	}
	topts.Nonce = new(big.Int).SetUint64(nonce)
	return topts, release, nil
}

// finalize applies WriteOption post-broadcast semantics: it returns the
// transaction hash immediately, unless the caller asked to wait for a
// receipt (WithWait). When the transaction reverts on-chain, the returned
// error wraps ErrTxFailed and the receipt is still populated for inspection.
func (s *Service) finalize(ctx context.Context, tx *ethtypes.Transaction, opts []WriteOption) (*WriteResult, error) {
	cfg := newWriteConfig(opts)
	res := &WriteResult{Hash: tx.Hash()}
	if cfg.waitTimeout <= 0 {
		return res, nil
	}
	var (
		receipt *ethtypes.Receipt
		err     error
	)
	if cfg.confirmations > 0 {
		waitCfg := txutil.DefaultReceiptWaitConfig()
		if s.receiptWait > 0 {
			waitCfg.Timeout = s.receiptWait
		}
		if cfg.waitTimeout > 0 {
			waitCfg.Timeout = cfg.waitTimeout
		}
		receipt, err = txutil.WaitForReceiptWithConfig(ctx, s.backend, tx.Hash(), waitCfg, cfg.confirmations)
	} else {
		receipt, err = txutil.WaitForReceipt(ctx, s.backend, tx.Hash(), cfg.waitTimeout)
	}
	if err != nil {
		if errors.Is(err, txutil.ErrTxFailed) {
			res.Receipt = receipt
			return res, fmt.Errorf("wait receipt: %w", err)
		}
		return res, fmt.Errorf("wait receipt: %w", err)
	}
	res.Receipt = receipt
	return res, nil
}

// GetProvider returns the provider by id. Returns an error wrapping
// ErrNotFound when no such provider exists (contract convention: a zero
// ServiceProvider address in the returned record). RPC or ABI errors are
// wrapped and propagated as-is.
func (s *Service) GetProvider(ctx context.Context, providerID types.BigInt) (*ProviderInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if providerID.IsZero() {
		return nil, fmt.Errorf("spregistry.GetProvider: %w: zero providerID", ErrInvalidArgument)
	}
	v, err := s.contract.GetProvider(&bind.CallOpts{Context: ctx}, providerID.Big())
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProvider: %w", err)
	}
	if (v.Info.ServiceProvider == common.Address{}) {
		return nil, fmt.Errorf("spregistry.GetProvider: %w", ErrNotFound)
	}
	info, err := fromRawView(v)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProvider: %w", err)
	}
	return info, nil
}

// GetProviderByAddress returns the provider record whose serviceProvider
// matches addr. Returns an error wrapping ErrNotFound when no provider is
// registered for that address.
func (s *Service) GetProviderByAddress(ctx context.Context, addr common.Address) (*ProviderInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if (addr == common.Address{}) {
		return nil, fmt.Errorf("spregistry.GetProviderByAddress: %w: zero address", ErrInvalidArgument)
	}
	v, err := s.contract.GetProviderByAddress(&bind.CallOpts{Context: ctx}, addr)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProviderByAddress: %w", err)
	}
	if (v.Info.ServiceProvider == common.Address{}) {
		return nil, fmt.Errorf("spregistry.GetProviderByAddress: %w", ErrNotFound)
	}
	info, err := fromRawView(v)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProviderByAddress: %w", err)
	}
	return info, nil
}

// GetProviderIDByAddress returns the provider ID for addr. Returns 0 when
// addr is not registered (contract convention).
func (s *Service) GetProviderIDByAddress(ctx context.Context, addr common.Address) (types.BigInt, error) {
	if err := s.checkInit(); err != nil {
		return types.BigInt{}, err
	}
	if (addr == common.Address{}) {
		return types.BigInt{}, fmt.Errorf("spregistry.GetProviderIDByAddress: %w: zero address", ErrInvalidArgument)
	}
	id, err := s.contract.GetProviderIdByAddress(&bind.CallOpts{Context: ctx}, addr)
	if err != nil {
		return types.BigInt{}, fmt.Errorf("spregistry.GetProviderIDByAddress: %w", err)
	}
	if id == nil || id.Sign() == 0 {
		return types.BigInt{}, nil
	}
	pid, err := idconv.FromBig("providerID", id)
	if err != nil {
		return types.BigInt{}, fmt.Errorf("spregistry.GetProviderIDByAddress: %w", err)
	}
	return pid, nil
}

// IsProviderActive returns true if the provider id is registered AND active.
func (s *Service) IsProviderActive(ctx context.Context, providerID types.BigInt) (bool, error) {
	if err := s.checkInit(); err != nil {
		return false, err
	}
	if providerID.IsZero() {
		return false, fmt.Errorf("spregistry.IsProviderActive: %w: zero providerID", ErrInvalidArgument)
	}
	ok, err := s.contract.IsProviderActive(&bind.CallOpts{Context: ctx}, providerID.Big())
	if err != nil {
		return false, fmt.Errorf("spregistry.IsProviderActive: %w", err)
	}
	return ok, nil
}

// GetProviderCount returns the total number of registered providers
// (active + inactive).
func (s *Service) GetProviderCount(ctx context.Context) (*big.Int, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	n, err := s.contract.GetProviderCount(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProviderCount: %w", err)
	}
	return n, nil
}

// GetActiveProviderCount returns the number of providers whose IsActive flag
// is true.
func (s *Service) GetActiveProviderCount(ctx context.Context) (*big.Int, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	n, err := s.contract.ActiveProviderCount(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetActiveProviderCount: %w", err)
	}
	return n, nil
}

// GetPDPProvider returns the provider + decoded PDP offering for providerID.
// Returns an error wrapping ErrNotFound when the provider has no PDP product
// registered.
func (s *Service) GetPDPProvider(ctx context.Context, providerID types.BigInt) (*PDPProvider, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if providerID.IsZero() {
		return nil, fmt.Errorf("spregistry.GetPDPProvider: %w: zero providerID", ErrInvalidArgument)
	}
	v, err := s.contract.GetProviderWithProduct(&bind.CallOpts{Context: ctx}, providerID.Big(), uint8(ProductTypePDP))
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetPDPProvider: %w", err)
	}
	if (v.ProviderInfo.ServiceProvider == common.Address{}) {
		return nil, fmt.Errorf("spregistry.GetPDPProvider: %w", ErrNotFound)
	}
	provider, err := decodeProviderWithProduct(v)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetPDPProvider: %w", err)
	}
	return provider, nil
}

// GetPDPProviders lists PDP providers with pagination. When onlyActive is
// true the registry filters out inactive providers BEFORE applying offset
// and limit (matches the contract semantics). opts.Limit must be > 0; use
// IterateAllPDPProviders for unbounded traversal.
func (s *Service) GetPDPProviders(ctx context.Context, onlyActive bool, opts types.ListOptions) (*PaginatedPDPProviders, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("spregistry.GetPDPProviders: %w: %w", ErrInvalidArgument, err)
	}
	offsetBig := new(big.Int).SetUint64(opts.Offset)
	limitBig := new(big.Int).SetUint64(opts.Limit)
	raw, err := s.contract.GetProvidersByProductType(&bind.CallOpts{Context: ctx}, uint8(ProductTypePDP), onlyActive, offsetBig, limitBig)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetPDPProviders: %w", err)
	}
	out := &PaginatedPDPProviders{HasMore: raw.HasMore, Providers: make([]PDPProvider, 0, len(raw.Providers))}
	for _, p := range raw.Providers {
		dec, err := decodeProviderWithProduct(p)
		if err != nil {
			return nil, fmt.Errorf("spregistry.GetPDPProviders: %w", err)
		}
		if dec != nil {
			out.Providers = append(out.Providers, *dec)
		}
	}
	return out, nil
}

// GetProvidersByIDs returns the provider records for the given ids, in the
// same order. Entries whose validIds flag was false come back as nil.
// An empty input slice returns an empty (non-nil) result slice.
func (s *Service) GetProvidersByIDs(ctx context.Context, providerIDs []types.BigInt) ([]*ProviderInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if len(providerIDs) == 0 {
		return []*ProviderInfo{}, nil
	}
	bigIDs := idconv.BigSlice(providerIDs)
	raw, err := s.contract.GetProvidersByIds(&bind.CallOpts{Context: ctx}, bigIDs)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProvidersByIDs: %w", err)
	}
	if len(raw.ValidIds) != len(providerIDs) || len(raw.ProviderInfos) != len(providerIDs) {
		return nil, fmt.Errorf("spregistry.GetProvidersByIDs: malformed response: got %d valid_ids and %d provider_infos for %d requested ids", len(raw.ValidIds), len(raw.ProviderInfos), len(providerIDs))
	}
	out := make([]*ProviderInfo, len(providerIDs))
	for i := range providerIDs {
		if !raw.ValidIds[i] {
			continue
		}
		info, err := fromRawView(raw.ProviderInfos[i])
		if err != nil {
			return nil, fmt.Errorf("spregistry.GetProvidersByIDs: %w", err)
		}
		out[i] = info
	}
	return out, nil
}

// SelectActivePDPProviders fetches all active PDP providers (handling
// pagination internally), applies f, and returns the survivors sorted
// deterministically by provider ID (ascending). This is the entry point
// that storage uses for provider selection; it deliberately avoids exposing
// raw pagination to the caller.
//
// Filter semantics (all conditions are ANDed):
//   - PieceSizeBytes: provider must satisfy
//     minPieceSizeInBytes ≤ PieceSizeBytes ≤ maxPieceSizeInBytes.
//     When maxPieceSizeInBytes is zero the upper bound is skipped.
//   - PaymentToken: when non-nil, provider must declare exactly this
//     payment token address. The zero address is a valid value and
//     means FIL, matching the on-chain/TS semantics.
//   - ExcludeIDs: providers whose ID appears in this slice are removed.
//
// An error is returned if pagination exceeds maxSelectPages to guard against
// a misbehaving RPC that returns HasMore=true indefinitely.
func (s *Service) SelectActivePDPProviders(ctx context.Context, f ProviderFilter) ([]PDPProvider, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	const (
		pageSize       = 50
		maxSelectPages = 200 // safety cap: 200 × 50 = 10 000 providers
	)

	excludeSet := make(map[string]struct{}, len(f.ExcludeIDs))
	for _, id := range f.ExcludeIDs {
		excludeSet[idconv.Key(id)] = struct{}{}
	}

	var all []PDPProvider
	offset := uint64(0)
	for page := 0; ; page++ {
		if page >= maxSelectPages {
			return nil, fmt.Errorf("spregistry.SelectActivePDPProviders: pagination exceeded %d pages; possible RPC misbehaviour", maxSelectPages)
		}
		result, err := s.GetPDPProviders(ctx, true, types.ListOptions{Offset: offset, Limit: pageSize})
		if err != nil {
			return nil, fmt.Errorf("spregistry.SelectActivePDPProviders: %w", err)
		}
		for _, p := range result.Providers {
			if p.Info.ID.IsZero() {
				return nil, errors.New("spregistry.SelectActivePDPProviders: provider list contains zero provider ID")
			}
			if matchesFilter(p, f, excludeSet) {
				all = append(all, p)
			}
		}
		if !result.HasMore {
			break
		}
		offset += pageSize
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Info.ID.Cmp(all[j].Info.ID) < 0
	})
	return all, nil
}

// matchesFilter returns true when p satisfies all criteria in f.
// Providers with a zero ID are always rejected.
func matchesFilter(p PDPProvider, f ProviderFilter, excludeSet map[string]struct{}) bool {
	if p.Info.ID.IsZero() {
		return false
	}
	if _, excluded := excludeSet[idconv.Key(p.Info.ID)]; excluded {
		return false
	}
	if f.PieceSizeBytes != nil && f.PieceSizeBytes.Sign() > 0 {
		if p.Offering.MinPieceSizeInBytes != nil && p.Offering.MinPieceSizeInBytes.Sign() > 0 {
			if f.PieceSizeBytes.Cmp(p.Offering.MinPieceSizeInBytes) < 0 {
				return false
			}
		}
		if p.Offering.MaxPieceSizeInBytes != nil && p.Offering.MaxPieceSizeInBytes.Sign() > 0 {
			if f.PieceSizeBytes.Cmp(p.Offering.MaxPieceSizeInBytes) > 0 {
				return false
			}
		}
	}
	if f.PaymentToken != nil {
		if p.Offering.PaymentTokenAddress != *f.PaymentToken {
			return false
		}
	}
	return true
}

// --- helpers ---

func fromRawView(v spr.ServiceProviderRegistryServiceProviderInfoView) (*ProviderInfo, error) {
	id, err := idconv.FromBig("providerID", v.ProviderId)
	if err != nil {
		return nil, err
	}
	return &ProviderInfo{
		ID:              id,
		ServiceProvider: v.Info.ServiceProvider,
		Payee:           v.Info.Payee,
		Name:            v.Info.Name,
		Description:     v.Info.Description,
		IsActive:        v.Info.IsActive,
	}, nil
}

func decodeProviderWithProduct(v spr.ServiceProviderRegistryStorageProviderWithProduct) (*PDPProvider, error) {
	caps := CapabilitiesListToMap(v.Product.CapabilityKeys, v.ProductCapabilityValues)
	off, err := DecodePDPOffering(caps)
	if err != nil {
		return nil, fmt.Errorf("spregistry.decodeProviderWithProduct: %w", err)
	}
	id, err := idconv.FromBig("providerID", v.ProviderId)
	if err != nil {
		return nil, fmt.Errorf("spregistry.decodeProviderWithProduct: %w", err)
	}
	return &PDPProvider{
		Info: ProviderInfo{
			ID:              id,
			ServiceProvider: v.ProviderInfo.ServiceProvider,
			Payee:           v.ProviderInfo.Payee,
			Name:            v.ProviderInfo.Name,
			Description:     v.ProviderInfo.Description,
			IsActive:        v.ProviderInfo.IsActive,
		},
		Product: ServiceProduct{
			ProductType:    ProductType(v.Product.ProductType),
			CapabilityKeys: v.Product.CapabilityKeys,
			IsActive:       v.Product.IsActive,
		},
		Offering: off,
	}, nil
}
