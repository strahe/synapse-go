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
		return nil, errors.New("spregistry.New: nil Client")
	}
	if (opts.Address == common.Address{}) {
		return nil, errors.New("spregistry.New: zero Address")
	}
	c, err := spr.NewSPRegistryCaller(opts.Address, opts.Client)
	if err != nil {
		return nil, fmt.Errorf("spregistry.New: bind: %w", err)
	}
	return &Service{caller: opts.Client, addr: opts.Address, contract: c}, nil
}

// Address returns the configured registry contract address.
func (s *Service) Address() common.Address { return s.addr }

// GetProvider returns the provider by id, or nil when the call yields the
// on-chain "zero" record (zero ServiceProvider address). RPC errors,
// including explicit contract reverts, are returned to the caller.
func (s *Service) GetProvider(ctx context.Context, providerID *big.Int) (*ProviderInfo, error) {
	if providerID == nil {
		return nil, errors.New("spregistry.GetProvider: nil providerID")
	}
	v, err := s.contract.GetProvider(&bind.CallOpts{Context: ctx}, providerID)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProvider: %w", err)
	}
	if (v.Info.ServiceProvider == common.Address{}) {
		return nil, nil
	}
	return fromRawView(v), nil
}

// GetProviderByAddress returns the provider record whose serviceProvider
// matches addr, or nil if no provider is registered for that address.
func (s *Service) GetProviderByAddress(ctx context.Context, addr common.Address) (*ProviderInfo, error) {
	if (addr == common.Address{}) {
		return nil, errors.New("spregistry.GetProviderByAddress: zero address")
	}
	v, err := s.contract.GetProviderByAddress(&bind.CallOpts{Context: ctx}, addr)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProviderByAddress: %w", err)
	}
	if (v.Info.ServiceProvider == common.Address{}) {
		return nil, nil
	}
	return fromRawView(v), nil
}

// GetProviderIDByAddress returns 0 if the address is not registered.
func (s *Service) GetProviderIDByAddress(ctx context.Context, addr common.Address) (*big.Int, error) {
	if (addr == common.Address{}) {
		return nil, errors.New("spregistry.GetProviderIDByAddress: zero address")
	}
	id, err := s.contract.GetProviderIdByAddress(&bind.CallOpts{Context: ctx}, addr)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProviderIDByAddress: %w", err)
	}
	return id, nil
}

// IsProviderActive returns true if the provider id is registered AND active.
func (s *Service) IsProviderActive(ctx context.Context, providerID *big.Int) (bool, error) {
	if providerID == nil {
		return false, errors.New("spregistry.IsProviderActive: nil providerID")
	}
	ok, err := s.contract.IsProviderActive(&bind.CallOpts{Context: ctx}, providerID)
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

// GetPDPProvider returns the provider + decoded PDP offering for providerID,
// or nil if the provider has no PDP product registered.
func (s *Service) GetPDPProvider(ctx context.Context, providerID *big.Int) (*PDPProvider, error) {
	if providerID == nil {
		return nil, errors.New("spregistry.GetPDPProvider: nil providerID")
	}
	v, err := s.contract.GetProviderWithProduct(&bind.CallOpts{Context: ctx}, providerID, uint8(ProductTypePDP))
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetPDPProvider: %w", err)
	}
	if (v.ProviderInfo.ServiceProvider == common.Address{}) {
		return nil, nil
	}
	return decodeProviderWithProduct(v)
}

// GetPDPProviders lists PDP providers with pagination. When onlyActive is
// true the registry filters out inactive providers BEFORE applying offset
// and limit (matches the contract semantics).
func (s *Service) GetPDPProviders(ctx context.Context, onlyActive bool, offset, limit *big.Int) (*PaginatedPDPProviders, error) {
	if offset == nil {
		offset = big.NewInt(0)
	}
	if limit == nil {
		limit = big.NewInt(50)
	}
	raw, err := s.contract.GetProvidersByProductType(&bind.CallOpts{Context: ctx}, uint8(ProductTypePDP), onlyActive, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetPDPProviders: %w", err)
	}
	out := &PaginatedPDPProviders{HasMore: raw.HasMore, Providers: make([]PDPProvider, 0, len(raw.Providers))}
	for _, p := range raw.Providers {
		dec, err := decodeProviderWithProduct(p)
		if err != nil {
			return nil, err
		}
		if dec != nil {
			out.Providers = append(out.Providers, *dec)
		}
	}
	return out, nil
}

// GetProvidersByIDs returns the provider records for the given ids, in the
// same order. Entries whose validIds flag was false come back as nil.
func (s *Service) GetProvidersByIDs(ctx context.Context, providerIDs []*big.Int) ([]*ProviderInfo, error) {
	if len(providerIDs) == 0 {
		return nil, nil
	}
	for i, id := range providerIDs {
		if id == nil {
			return nil, fmt.Errorf("spregistry.GetProvidersByIDs: providerIDs[%d] is nil", i)
		}
	}
	raw, err := s.contract.GetProvidersByIds(&bind.CallOpts{Context: ctx}, providerIDs)
	if err != nil {
		return nil, fmt.Errorf("spregistry.GetProvidersByIDs: %w", err)
	}
	out := make([]*ProviderInfo, len(providerIDs))
	for i := range providerIDs {
		if i >= len(raw.ValidIds) || !raw.ValidIds[i] {
			continue
		}
		if i >= len(raw.ProviderInfos) {
			continue
		}
		info := fromRawView(raw.ProviderInfos[i])
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

	excludeSet := make(map[string]struct{}, len(f.ExcludeIDs))
	for _, id := range f.ExcludeIDs {
		if id != nil {
			excludeSet[id.String()] = struct{}{}
		}
	}

	var all []PDPProvider
	offset := big.NewInt(0)
	limit := big.NewInt(pageSize)
	for page := 0; ; page++ {
		if page >= maxSelectPages {
			return nil, fmt.Errorf("spregistry.SelectActivePDPProviders: pagination exceeded %d pages; possible RPC misbehaviour", maxSelectPages)
		}
		result, err := s.GetPDPProviders(ctx, true, offset, limit)
		if err != nil {
			return nil, fmt.Errorf("spregistry.SelectActivePDPProviders: %w", err)
		}
		for _, p := range result.Providers {
			if p.Info.ID == nil {
				continue // skip malformed entries with nil ID
			}
			if matchesFilter(p, f, excludeSet) {
				all = append(all, p)
			}
		}
		if !result.HasMore {
			break
		}
		offset = new(big.Int).Add(offset, limit)
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].Info.ID.Cmp(all[j].Info.ID) < 0
	})
	return all, nil
}

// matchesFilter returns true when p satisfies all criteria in f.
// Providers with a nil ID are always rejected.
func matchesFilter(p PDPProvider, f ProviderFilter, excludeSet map[string]struct{}) bool {
	if p.Info.ID == nil {
		return false
	}
	if _, excluded := excludeSet[p.Info.ID.String()]; excluded {
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

func fromRawView(v spr.ServiceProviderRegistryServiceProviderInfoView) *ProviderInfo {
	return &ProviderInfo{
		ID:              v.ProviderId,
		ServiceProvider: v.Info.ServiceProvider,
		Payee:           v.Info.Payee,
		Name:            v.Info.Name,
		Description:     v.Info.Description,
		IsActive:        v.Info.IsActive,
	}
}

func decodeProviderWithProduct(v spr.ServiceProviderRegistryStorageProviderWithProduct) (*PDPProvider, error) {
	caps := CapabilitiesListToMap(v.Product.CapabilityKeys, v.ProductCapabilityValues)
	off, err := DecodePDPOffering(caps)
	if err != nil {
		return nil, fmt.Errorf("spregistry.decodeProviderWithProduct: %w", err)
	}
	return &PDPProvider{
		Info: ProviderInfo{
			ID:              v.ProviderId,
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
