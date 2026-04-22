package warmstorage

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/internal/contracts/fwssview"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/types"
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
	caller    EthClient
	fwssAddr  common.Address
	viewAddr  common.Address
	fwssBind  *fwss.FWSSCaller
	viewBind  *fwssview.FWSSViewCaller
	lifecycle *lifecycle.Lifecycle
}

// Options bundle the caller and contract addresses.
type Options struct {
	Client       EthClient
	FWSS         common.Address
	ViewContract common.Address
	// Lifecycle, when non-nil, ties this Service to the owning Client's
	// close state. After the Lifecycle is closed, every method returns
	// [ErrClosed]. Nil is allowed for standalone use.
	Lifecycle *lifecycle.Lifecycle
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
		caller:    opts.Client,
		fwssAddr:  opts.FWSS,
		viewAddr:  opts.ViewContract,
		fwssBind:  fb,
		viewBind:  vb,
		lifecycle: opts.Lifecycle,
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
	if err := s.checkInit(); err != nil {
		return nil, err
	}
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
	DataSetID       types.DataSetID
	PDPRailID       types.RailID // payment rail for PDP proofs
	CacheMissRailID types.RailID // payment rail for cache-miss egress
	CDNRailID       types.RailID // payment rail for CDN egress
	Payer           common.Address
	Payee           common.Address
	ServiceProvider common.Address
	CommissionBps   *big.Int              // commission rate in basis points (out of 10 000)
	ClientDataSetID types.ClientDataSetID // caller-assigned ID used in EIP-712 payloads
	PDPEndEpoch     types.Epoch           // epoch at which PDP service expires; 0 = indefinite
	ProviderID      types.ProviderID      // numeric storage provider ID
}

func toDataSetInfo(v fwssview.FilecoinWarmStorageServiceDataSetInfoView) (*DataSetInfo, error) {
	dsID, err := idconv.Safe[types.DataSetID]("DataSetID", v.DataSetId)
	if err != nil {
		return nil, err
	}
	pdpRail, err := idconv.Safe[types.RailID]("PDPRailID", v.PdpRailId)
	if err != nil {
		return nil, err
	}
	cacheRail, err := idconv.Safe[types.RailID]("CacheMissRailID", v.CacheMissRailId)
	if err != nil {
		return nil, err
	}
	cdnRail, err := idconv.Safe[types.RailID]("CDNRailID", v.CdnRailId)
	if err != nil {
		return nil, err
	}
	endEpoch, err := idconv.Safe[types.Epoch]("PDPEndEpoch", v.PdpEndEpoch)
	if err != nil {
		return nil, err
	}
	provID, err := idconv.Safe[types.ProviderID]("ProviderID", v.ProviderId)
	if err != nil {
		return nil, err
	}
	return &DataSetInfo{
		DataSetID:       dsID,
		PDPRailID:       pdpRail,
		CacheMissRailID: cacheRail,
		CDNRailID:       cdnRail,
		Payer:           v.Payer,
		Payee:           v.Payee,
		ServiceProvider: v.ServiceProvider,
		CommissionBps:   copyBigInt(v.CommissionBps),
		ClientDataSetID: copyBigInt(v.ClientDataSetId),
		PDPEndEpoch:     endEpoch,
		ProviderID:      provID,
	}, nil
}

func copyBigInt(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

// GetDataSet returns the data set record for dataSetID. Returns an error
// wrapping ErrNotFound when no such data set exists (the contract convention
// is pdpRailId == 0 for "not found"). RPC or ABI errors are wrapped and
// propagated as-is.
func (s *Service) GetDataSet(ctx context.Context, dataSetID types.DataSetID) (*DataSetInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID == 0 {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w: zero dataSetID", ErrInvalidArgument)
	}
	v, err := s.viewBind.GetDataSet(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w", err)
	}
	if v.PdpRailId == nil || v.PdpRailId.Sign() == 0 {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w", ErrNotFound)
	}
	info, err := toDataSetInfo(v)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w", err)
	}
	return info, nil
}

// GetClientDataSets returns one page of data sets owned by payer. opts.Limit
// must be > 0; use IterateAllClientDataSets for unbounded traversal.
func (s *Service) GetClientDataSets(ctx context.Context, payer common.Address, opts types.ListOptions) ([]*DataSetInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if (payer == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.GetClientDataSets: %w: zero payer", ErrInvalidArgument)
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSets: %w: %w", ErrInvalidArgument, err)
	}
	offset := new(big.Int).SetUint64(opts.Offset)
	limit := new(big.Int).SetUint64(opts.Limit)
	raw, err := s.viewBind.GetClientDataSets(&bind.CallOpts{Context: ctx}, payer, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSets: %w", err)
	}
	out := make([]*DataSetInfo, 0, len(raw))
	for _, v := range raw {
		info, err := toDataSetInfo(v)
		if err != nil {
			return nil, fmt.Errorf("warmstorage.GetClientDataSets: %w", err)
		}
		out = append(out, info)
	}
	return out, nil
}

// GetAllDataSetMetadata returns all metadata entries for the given dataset as a
// key/value map. The returned map is non-nil; an empty map signals "no metadata
// entries" (which the contract also returns when the dataset itself does not
// exist — the two cases are indistinguishable at this layer). Mismatched
// response lengths are treated as an RPC/ABI error.
func (s *Service) GetAllDataSetMetadata(ctx context.Context, dataSetID types.DataSetID) (map[string]string, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID == 0 {
		return nil, fmt.Errorf("warmstorage.GetAllDataSetMetadata: %w: zero dataSetID", ErrInvalidArgument)
	}
	raw, err := s.viewBind.GetAllDataSetMetadata(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
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
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if (payer == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsLength: %w: zero payer", ErrInvalidArgument)
	}
	n, err := s.viewBind.GetClientDataSetsLength(&bind.CallOpts{Context: ctx}, payer)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsLength: %w", err)
	}
	return n, nil
}

// GetApprovedProviderIDs returns one page of approved-provider ids.
// opts.Limit must be > 0; use IterateAllApprovedProviderIDs for unbounded
// traversal.
func (s *Service) GetApprovedProviderIDs(ctx context.Context, opts types.ListOptions) ([]types.ProviderID, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("warmstorage.GetApprovedProviderIDs: %w: %w", ErrInvalidArgument, err)
	}
	offset := new(big.Int).SetUint64(opts.Offset)
	limit := new(big.Int).SetUint64(opts.Limit)
	ids, err := s.viewBind.GetApprovedProviders(&bind.CallOpts{Context: ctx}, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetApprovedProviderIDs: %w", err)
	}
	out, err := idconv.SafeSlice[types.ProviderID]("ProviderID", ids)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetApprovedProviderIDs: %w", err)
	}
	return out, nil
}

// GetApprovedProvidersLength returns the total number of approved providers.
func (s *Service) GetApprovedProvidersLength(ctx context.Context) (*big.Int, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	n, err := s.viewBind.GetApprovedProvidersLength(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetApprovedProvidersLength: %w", err)
	}
	return n, nil
}

// IsProviderApproved returns true if providerID is on the approved list.
func (s *Service) IsProviderApproved(ctx context.Context, providerID types.ProviderID) (bool, error) {
	if err := s.checkInit(); err != nil {
		return false, err
	}
	if providerID == 0 {
		return false, fmt.Errorf("warmstorage.IsProviderApproved: %w: zero providerID", ErrInvalidArgument)
	}
	ok, err := s.viewBind.IsProviderApproved(&bind.CallOpts{Context: ctx}, idconv.Big(providerID))
	if err != nil {
		return false, fmt.Errorf("warmstorage.IsProviderApproved: %w", err)
	}
	return ok, nil
}
