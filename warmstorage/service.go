package warmstorage

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/internal/contracts/fwssview"
)

// EthClient is the minimal RPC surface the service needs. The interface is
// defined here so tests can substitute a mock without pulling in ethclient.
type EthClient interface {
	bind.ContractCaller
}

// Service provides read access to FilecoinWarmStorageService (FWSS) and its
// companion StateView contract.
//
// All writes flow through the curio HTTP client and the payments service;
// FWSS is only invoked here for queries that back SDK decisions (service
// pricing, dataset existence, provider approval).
type Service struct {
	caller   EthClient
	fwssAddr common.Address
	viewAddr common.Address
	fwssBind *fwss.FWSSCaller
	viewBind *fwssview.FWSSViewCaller
}

// Options bundle the caller and contract addresses.
type Options struct {
	Client       EthClient
	FWSS         common.Address
	ViewContract common.Address
}

// New creates a Service. Both FWSS and ViewContract addresses are required;
// use internal/abi.ResolveAddresses to obtain them from chain data.
func New(opts Options) (*Service, error) {
	if opts.Client == nil {
		return nil, fmt.Errorf("warmstorage.New: %w: nil Client", ErrInvalidArgument)
	}
	if (opts.FWSS == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.New: %w: zero FWSS address", ErrInvalidArgument)
	}
	if (opts.ViewContract == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.New: %w: zero ViewContract address", ErrInvalidArgument)
	}
	fb, err := fwss.NewFWSSCaller(opts.FWSS, opts.Client)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.New: bind fwss: %w", err)
	}
	vb, err := fwssview.NewFWSSViewCaller(opts.ViewContract, opts.Client)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.New: bind fwssview: %w", err)
	}
	return &Service{
		caller:   opts.Client,
		fwssAddr: opts.FWSS,
		viewAddr: opts.ViewContract,
		fwssBind: fb,
		viewBind: vb,
	}, nil
}

// FWSSAddress returns the configured FWSS contract address.
func (s *Service) FWSSAddress() common.Address { return s.fwssAddr }

// ViewAddress returns the configured StateView contract address.
func (s *Service) ViewAddress() common.Address { return s.viewAddr }

// ServicePrice mirrors FilecoinWarmStorageServiceServicePricing. All amounts
// are in base units of the payment token.
type ServicePrice struct {
	PricePerTiBPerMonthNoCDN   *big.Int
	PricePerTiBCdnEgress       *big.Int
	PricePerTiBCacheMissEgress *big.Int
	TokenAddress               common.Address // EVM address of the payment token
	EpochsPerMonth             *big.Int       // Filecoin epochs per billing month
	MinimumPricePerMonth       *big.Int
}

// GetServicePrice returns the current pricing parameters.
func (s *Service) GetServicePrice(ctx context.Context) (*ServicePrice, error) {
	p, err := s.fwssBind.GetServicePrice(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetServicePrice: %w", err)
	}
	return &ServicePrice{
		PricePerTiBPerMonthNoCDN:   p.PricePerTiBPerMonthNoCDN,
		PricePerTiBCdnEgress:       p.PricePerTiBCdnEgress,
		PricePerTiBCacheMissEgress: p.PricePerTiBCacheMissEgress,
		TokenAddress:               p.TokenAddress,
		EpochsPerMonth:             p.EpochsPerMonth,
		MinimumPricePerMonth:       p.MinimumPricePerMonth,
	}, nil
}

// DataSetInfo mirrors FilecoinWarmStorageServiceDataSetInfoView.
type DataSetInfo struct {
	DataSetID       *big.Int
	PDPRailID       *big.Int // payment rail for PDP proofs
	CacheMissRailID *big.Int // payment rail for cache-miss egress
	CDNRailID       *big.Int // payment rail for CDN egress
	Payer           common.Address
	Payee           common.Address
	ServiceProvider common.Address
	CommissionBps   *big.Int // commission rate in basis points (out of 10 000)
	ClientDataSetID *big.Int // caller-assigned ID used in EIP-712 payloads
	PDPEndEpoch     *big.Int // epoch at which PDP service expires; 0 = indefinite
	ProviderID      *big.Int // numeric storage provider ID
}

func toDataSetInfo(v fwssview.FilecoinWarmStorageServiceDataSetInfoView) *DataSetInfo {
	return &DataSetInfo{
		DataSetID:       v.DataSetId,
		PDPRailID:       v.PdpRailId,
		CacheMissRailID: v.CacheMissRailId,
		CDNRailID:       v.CdnRailId,
		Payer:           v.Payer,
		Payee:           v.Payee,
		ServiceProvider: v.ServiceProvider,
		CommissionBps:   v.CommissionBps,
		ClientDataSetID: v.ClientDataSetId,
		PDPEndEpoch:     v.PdpEndEpoch,
		ProviderID:      v.ProviderId,
	}
}

// GetDataSet returns the data set record for dataSetID. Returns an error
// wrapping ErrNotFound when no such data set exists (the contract convention
// is pdpRailId == 0 for "not found"). RPC or ABI errors are wrapped and
// propagated as-is.
func (s *Service) GetDataSet(ctx context.Context, dataSetID *big.Int) (*DataSetInfo, error) {
	if dataSetID == nil {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w: nil dataSetID", ErrInvalidArgument)
	}
	v, err := s.viewBind.GetDataSet(&bind.CallOpts{Context: ctx}, dataSetID)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w", err)
	}
	if v.PdpRailId == nil || v.PdpRailId.Sign() == 0 {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w", ErrNotFound)
	}
	return toDataSetInfo(v), nil
}

// GetClientDataSets returns the paginated list of data sets owned by the
// given payer. offset and limit follow the contract semantics (limit=0 means
// "all remaining").
func (s *Service) GetClientDataSets(ctx context.Context, payer common.Address, offset, limit *big.Int) ([]*DataSetInfo, error) {
	if (payer == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.GetClientDataSets: %w: zero payer", ErrInvalidArgument)
	}
	if offset == nil {
		offset = big.NewInt(0)
	}
	if limit == nil {
		limit = big.NewInt(0)
	}
	raw, err := s.viewBind.GetClientDataSets(&bind.CallOpts{Context: ctx}, payer, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSets: %w", err)
	}
	out := make([]*DataSetInfo, 0, len(raw))
	for _, v := range raw {
		out = append(out, toDataSetInfo(v))
	}
	return out, nil
}

// GetAllDataSetMetadata returns all metadata entries for the given dataset as a
// key/value map. The returned map is non-nil; an empty map signals "no metadata
// entries" (which the contract also returns when the dataset itself does not
// exist — the two cases are indistinguishable at this layer). Mismatched
// response lengths are treated as an RPC/ABI error.
func (s *Service) GetAllDataSetMetadata(ctx context.Context, dataSetID *big.Int) (map[string]string, error) {
	if dataSetID == nil {
		return nil, fmt.Errorf("warmstorage.GetAllDataSetMetadata: %w: nil dataSetID", ErrInvalidArgument)
	}
	raw, err := s.viewBind.GetAllDataSetMetadata(&bind.CallOpts{Context: ctx}, dataSetID)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetAllDataSetMetadata: %w", err)
	}
	if len(raw.Keys) != len(raw.Values) {
		return nil, fmt.Errorf("warmstorage.GetAllDataSetMetadata: mismatched keys (%d) and values (%d)", len(raw.Keys), len(raw.Values))
	}
	out := make(map[string]string, len(raw.Keys))
	for i, key := range raw.Keys {
		out[key] = raw.Values[i]
	}
	return out, nil
}

// GetClientDataSetsLength returns the total number of data sets for a payer.
func (s *Service) GetClientDataSetsLength(ctx context.Context, payer common.Address) (*big.Int, error) {
	if (payer == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsLength: %w: zero payer", ErrInvalidArgument)
	}
	n, err := s.viewBind.GetClientDataSetsLength(&bind.CallOpts{Context: ctx}, payer)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsLength: %w", err)
	}
	return n, nil
}

// GetApprovedProviderIDs returns the approved-provider id list (paginated).
// limit=0 means all remaining starting from offset, matching the contract.
func (s *Service) GetApprovedProviderIDs(ctx context.Context, offset, limit *big.Int) ([]*big.Int, error) {
	if offset == nil {
		offset = big.NewInt(0)
	}
	if limit == nil {
		limit = big.NewInt(0)
	}
	ids, err := s.viewBind.GetApprovedProviders(&bind.CallOpts{Context: ctx}, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetApprovedProviderIDs: %w", err)
	}
	return ids, nil
}

// GetApprovedProvidersLength returns the total number of approved providers.
func (s *Service) GetApprovedProvidersLength(ctx context.Context) (*big.Int, error) {
	n, err := s.viewBind.GetApprovedProvidersLength(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetApprovedProvidersLength: %w", err)
	}
	return n, nil
}

// IsProviderApproved returns true if providerID is on the approved list.
func (s *Service) IsProviderApproved(ctx context.Context, providerID *big.Int) (bool, error) {
	if providerID == nil {
		return false, fmt.Errorf("warmstorage.IsProviderApproved: %w: nil providerID", ErrInvalidArgument)
	}
	ok, err := s.viewBind.IsProviderApproved(&bind.CallOpts{Context: ctx}, providerID)
	if err != nil {
		return false, fmt.Errorf("warmstorage.IsProviderApproved: %w", err)
	}
	return ok, nil
}
