package warmstorage

import (
	"context"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/internal/contracts/fwssview"
	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

// EthClient is the minimal RPC surface the service needs. The interface is
// defined here so tests can substitute a mock without pulling in ethclient.
type EthClient interface {
	bind.ContractCaller
}

// Backend extends EthClient with the surface required for sending
// transactions (TopUpCDNPaymentRails). The full ethclient.Client satisfies
// this interface.
type Backend interface {
	bind.ContractBackend
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*ethtypes.Receipt, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// Service provides read access to FilecoinWarmStorageService (FWSS) and its
// companion StateView contract, plus a small number of writes that are
// semantically owned by the WarmStorage contract (TopUpCDNPaymentRails).
//
// Most writes still flow through the payments service or the curio HTTP
// client. FWSS is invoked here for queries that back SDK decisions
// (service pricing, dataset existence, provider approval, dataset
// management checks).
type Service struct {
	caller         EthClient
	backend        Backend
	chainID        types.ChainID
	fwssAddr       common.Address
	viewAddr       common.Address
	pdpVerifierAdr common.Address
	fwssBind       *fwss.FWSSCaller
	viewBind       *fwssview.FWSSViewCaller
	pdpBind        *pdpverifier.PDPVerifierCaller
	fwssWrite      *fwss.FWSSTransactor
	signer         signer.EVMSigner
	nonces         *txutil.NonceManager
	logger         *slog.Logger
	receiptWait    time.Duration
	lifecycle      *lifecycle.Lifecycle
}

// Options bundle the caller and contract addresses.
type Options struct {
	Client       EthClient
	FWSS         common.Address
	ViewContract common.Address
	// PDPVerifier is the PDPVerifier contract address. Optional. Required
	// only for methods that call PDPVerifier directly (ValidateDataSet,
	// GetActivePieceCount, GetScheduledRemovals, and the isLive / listener
	// decorators on GetClientDataSetsWithDetails).
	PDPVerifier common.Address
	// ChainID is required only when writes are used (TopUpCDNPaymentRails).
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

// New creates a Service. Both FWSS and ViewContract addresses are required.
// The root synapse Client supplies the canonical chain addresses; low-level
// callers must provide the addresses they want this Service to use.
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
	s := &Service{
		caller:         opts.Client,
		backend:        opts.Backend,
		chainID:        opts.ChainID,
		fwssAddr:       opts.FWSS,
		viewAddr:       opts.ViewContract,
		pdpVerifierAdr: opts.PDPVerifier,
		fwssBind:       fb,
		viewBind:       vb,
		signer:         opts.Signer,
		logger:         opts.Logger,
		nonces:         opts.NonceManager,
		receiptWait:    opts.ReceiptWait,
		lifecycle:      opts.Lifecycle,
	}
	if (opts.PDPVerifier != common.Address{}) {
		pb, err := pdpverifier.NewPDPVerifierCaller(opts.PDPVerifier, opts.Client)
		if err != nil {
			return nil, fmt.Errorf("warmstorage.New: bind pdpverifier: %w", err)
		}
		s.pdpBind = pb
	}
	if opts.Backend != nil {
		tw, err := fwss.NewFWSSTransactor(opts.FWSS, opts.Backend)
		if err != nil {
			return nil, fmt.Errorf("warmstorage.New: bind fwss transactor: %w", err)
		}
		s.fwssWrite = tw
	}
	if s.nonces == nil && s.signer != nil && s.backend != nil {
		s.nonces = txutil.NewNonceManager(opts.Backend, s.signer.EVMAddress())
	}
	return s, nil
}

// FWSSAddress returns the configured FWSS contract address.
func (s *Service) FWSSAddress() common.Address { return s.fwssAddr }

// ViewAddress returns the configured StateView contract address.
func (s *Service) ViewAddress() common.Address { return s.viewAddr }

// PDPVerifierAddress returns the configured PDPVerifier address. Zero when
// the Service was constructed without a PDPVerifier.
func (s *Service) PDPVerifierAddress() common.Address { return s.pdpVerifierAdr }

// ServicePrice is the FWSS pricing view. All amounts are in base units of
// the payment token.
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

// DataSetInfo is the FWSS StateView record for one data set.
type DataSetInfo struct {
	DataSetID       types.BigInt
	PDPRailID       types.BigInt // payment rail for PDP proofs
	CacheMissRailID types.BigInt // payment rail for cache-miss egress
	CDNRailID       types.BigInt // payment rail for CDN egress
	Payer           common.Address
	Payee           common.Address
	ServiceProvider common.Address
	CommissionBps   *big.Int     // commission rate in basis points (out of 10 000)
	ClientDataSetID types.BigInt // caller-assigned ID used in EIP-712 payloads
	PDPEndEpoch     types.Epoch  // epoch at which PDP service expires; 0 = indefinite
	ProviderID      types.BigInt // storage provider ID
}

func toDataSetInfo(v fwssview.FilecoinWarmStorageServiceDataSetInfoView) (*DataSetInfo, error) {
	dsID, err := idconv.FromBig("DataSetID", v.DataSetId)
	if err != nil {
		return nil, err
	}
	pdpRail, err := idconv.FromBig("PDPRailID", v.PdpRailId)
	if err != nil {
		return nil, err
	}
	cacheRail, err := idconv.FromBig("CacheMissRailID", v.CacheMissRailId)
	if err != nil {
		return nil, err
	}
	cdnRail, err := idconv.FromBig("CDNRailID", v.CdnRailId)
	if err != nil {
		return nil, err
	}
	endEpoch, err := epochFromBig("PDPEndEpoch", v.PdpEndEpoch)
	if err != nil {
		return nil, err
	}
	provID, err := idconv.FromBig("ProviderID", v.ProviderId)
	if err != nil {
		return nil, err
	}
	clientDataSetID, err := idconv.FromBig("ClientDataSetID", v.ClientDataSetId)
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
		ClientDataSetID: clientDataSetID,
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

func epochFromBig(name string, v *big.Int) (types.Epoch, error) {
	if v == nil {
		return 0, fmt.Errorf("%s: nil", name)
	}
	if v.Sign() < 0 {
		return 0, fmt.Errorf("%s: negative: %s", name, v.String())
	}
	if !v.IsUint64() {
		return 0, fmt.Errorf("%s: exceeds uint64: %s", name, v.String())
	}
	return types.Epoch(v.Uint64()), nil
}

// GetDataSet returns the data set record for dataSetID. Returns an error
// wrapping ErrNotFound when no such data set exists (the contract convention
// is pdpRailId == 0 for "not found"). RPC or ABI errors are wrapped and
// propagated as-is.
func (s *Service) GetDataSet(ctx context.Context, dataSetID types.BigInt) (*DataSetInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("warmstorage.GetDataSet: %w: zero dataSetID", ErrInvalidArgument)
	}
	v, err := s.viewBind.GetDataSet(&bind.CallOpts{Context: ctx}, dataSetID.Big())
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
func (s *Service) GetAllDataSetMetadata(ctx context.Context, dataSetID types.BigInt) (map[string]string, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("warmstorage.GetAllDataSetMetadata: %w: zero dataSetID", ErrInvalidArgument)
	}
	raw, err := s.viewBind.GetAllDataSetMetadata(&bind.CallOpts{Context: ctx}, dataSetID.Big())
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
func (s *Service) GetApprovedProviderIDs(ctx context.Context, opts types.ListOptions) ([]types.BigInt, error) {
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
	out, err := idconv.FromBigSlice("ProviderID", ids)
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
func (s *Service) IsProviderApproved(ctx context.Context, providerID types.BigInt) (bool, error) {
	if err := s.checkInit(); err != nil {
		return false, err
	}
	if providerID.IsZero() {
		return false, fmt.Errorf("warmstorage.IsProviderApproved: %w: zero providerID", ErrInvalidArgument)
	}
	ok, err := s.viewBind.IsProviderApproved(&bind.CallOpts{Context: ctx}, providerID.Big())
	if err != nil {
		return false, fmt.Errorf("warmstorage.IsProviderApproved: %w", err)
	}
	return ok, nil
}
