package spregistry

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	spr "github.com/strahe/synapse-go/internal/contracts/spregistry"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/types"
)

// EthClient is the minimal RPC surface the service needs. Tests can substitute
// a mock that implements bind.ContractCaller.
type EthClient interface {
	bind.ContractCaller
}

// Service provides read access to the ServiceProviderRegistry contract.
//
// All state-mutating operations (register/update/remove) are out of scope
// for the SDK — providers manage their on-chain records directly via curio.
type Service struct {
	caller   EthClient
	addr     common.Address
	contract *spr.SPRegistryCaller
}

// Options configures the service.
type Options struct {
	Client  EthClient
	Address common.Address
}

// New creates a Service bound to the given registry address.
func New(opts Options) (*Service, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("spregistry.New: %w: nil Client", ErrInvalidArgument)
	}
	if (opts.Address == common.Address{}) {
		return nil, fmt.Errorf("spregistry.New: %w: zero Address", ErrInvalidArgument)
	}
	c, err := spr.NewSPRegistryCaller(opts.Address, opts.Client)
	if err != nil {
		return nil, fmt.Errorf("spregistry.New: bind: %w", err)
	}
	return &Service{caller: opts.Client, addr: opts.Address, contract: c}, nil
}

// Address returns the configured registry contract address.
func (s *Service) Address() common.Address { return s.addr }

// GetProvider returns the provider by id. Returns an error wrapping
// ErrNotFound when no such provider exists (contract convention: a zero
// ServiceProvider address in the returned record). RPC or ABI errors are
// wrapped and propagated as-is.
func (s *Service) GetProvider(ctx context.Context, providerID types.ProviderID) (*ProviderInfo, error) {
	if providerID == 0 {
		return nil, fmt.Errorf("spregistry.GetProvider: %w: zero providerID", ErrInvalidArgument)
	}
	v, err := s.contract.GetProvider(&bind.CallOpts{Context: ctx}, idconv.Big(providerID))
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
func (s *Service) GetProviderIDByAddress(ctx context.Context, addr common.Address) (types.ProviderID, error) {
	if (addr == common.Address{}) {
		return 0, fmt.Errorf("spregistry.GetProviderIDByAddress: %w: zero address", ErrInvalidArgument)
	}
	id, err := s.contract.GetProviderIdByAddress(&bind.CallOpts{Context: ctx}, addr)
	if err != nil {
		return 0, fmt.Errorf("spregistry.GetProviderIDByAddress: %w", err)
	}
	if id == nil || id.Sign() == 0 {
		return 0, nil
	}
	pid, err := idconv.Safe[types.ProviderID]("providerID", id)
	if err != nil {
		return 0, fmt.Errorf("spregistry.GetProviderIDByAddress: %w", err)
	}
	return pid, nil
}

// IsProviderActive returns true if the provider id is registered AND active.
func (s *Service) IsProviderActive(ctx context.Context, providerID types.ProviderID) (bool, error) {
	if providerID == 0 {
		return false, fmt.Errorf("spregistry.IsProviderActive: %w: zero providerID", ErrInvalidArgument)
	}
	ok, err := s.contract.IsProviderActive(&bind.CallOpts{Context: ctx}, idconv.Big(providerID))
	if err != nil {
		return false, fmt.Errorf("spregistry.IsProviderActive: %w", err)
	}
	return ok, nil
}

// GetProviderCount returns the total number of registered providers
// (active + inactive).
func (s *Service) GetProviderCount(ctx context.Context) (*big.Int, error) {
	n, err := s.contract.GetProviderCount(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProviderCount: %w", err)
	}
	return n, nil
}

// GetActiveProviderCount returns the number of providers whose IsActive flag
// is true.
func (s *Service) GetActiveProviderCount(ctx context.Context) (*big.Int, error) {
	n, err := s.contract.ActiveProviderCount(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetActiveProviderCount: %w", err)
	}
	return n, nil
}

// GetPDPProvider returns the provider + decoded PDP offering for providerID.
// Returns an error wrapping ErrNotFound when the provider has no PDP product
// registered.
func (s *Service) GetPDPProvider(ctx context.Context, providerID types.ProviderID) (*PDPProvider, error) {
	if providerID == 0 {
		return nil, fmt.Errorf("spregistry.GetPDPProvider: %w: zero providerID", ErrInvalidArgument)
	}
	v, err := s.contract.GetProviderWithProduct(&bind.CallOpts{Context: ctx}, idconv.Big(providerID), uint8(ProductTypePDP))
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
// and limit (matches the contract semantics). opts.Limit == 0 means the
// service default (currently 50).
func (s *Service) GetPDPProviders(ctx context.Context, onlyActive bool, opts types.ListOptions) (*PaginatedPDPProviders, error) {
	limit := opts.Limit
	if limit == 0 {
		limit = 50
	}
	offsetBig := new(big.Int).SetUint64(opts.Offset)
	limitBig := new(big.Int).SetUint64(limit)
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
func (s *Service) GetProvidersByIDs(ctx context.Context, providerIDs []types.ProviderID) ([]*ProviderInfo, error) {
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
	const (
		pageSize       = 50
		maxSelectPages = 200 // safety cap: 200 × 50 = 10 000 providers
	)

	excludeSet := make(map[types.ProviderID]struct{}, len(f.ExcludeIDs))
	for _, id := range f.ExcludeIDs {
		excludeSet[id] = struct{}{}
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
			if p.Info.ID == 0 {
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
		return all[i].Info.ID < all[j].Info.ID
	})
	return all, nil
}

// matchesFilter returns true when p satisfies all criteria in f.
// Providers with a zero ID are always rejected.
func matchesFilter(p PDPProvider, f ProviderFilter, excludeSet map[types.ProviderID]struct{}) bool {
	if p.Info.ID == 0 {
		return false
	}
	if _, excluded := excludeSet[p.Info.ID]; excluded {
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
	id, err := idconv.Safe[types.ProviderID]("providerID", v.ProviderId)
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
	id, err := idconv.Safe[types.ProviderID]("providerID", v.ProviderId)
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
