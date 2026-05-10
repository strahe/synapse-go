package warmstorage

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/txutil"
	sdktypes "github.com/strahe/synapse-go/types"
)

// ErrPDPVerifierNotConfigured is returned when a method that relies on
// direct PDPVerifier reads is called on a Service that was constructed
// without a PDPVerifier address.
var ErrPDPVerifierNotConfigured = errors.New("warmstorage: PDPVerifier address not configured")

// ErrWriteNotConfigured is returned when a write method is called on a
// Service that was constructed without a Backend / Signer.
var ErrWriteNotConfigured = errors.New("warmstorage: write backend / signer not configured")

// PDPConfig contains the FWSS StateView parameters that govern proving
// period scheduling.
type PDPConfig struct {
	MaxProvingPeriod         uint64
	ChallengeWindowSize      *big.Int
	ChallengesPerProof       *big.Int
	InitChallengeWindowStart *big.Int
}

// EnhancedDataSetInfo extends DataSetInfo with pdpverifier-derived
// liveness metadata. Returned by GetClientDataSetsWithDetails.
type EnhancedDataSetInfo struct {
	*DataSetInfo
	PDPVerifierDataSetID sdktypes.BigInt
	IsLive               bool
	IsManaged            bool
	ActivePieceCount     *big.Int
	WithCDN              bool
	Metadata             map[string]string
}

// ValidateDataSet verifies that the given data set is alive on the
// PDPVerifier contract and that its listener is this WarmStorage contract.
// Returns nil on success.
func (s *Service) ValidateDataSet(ctx context.Context, dataSetID sdktypes.BigInt) error {
	if err := s.checkInit(); err != nil {
		return err
	}
	if dataSetID.IsZero() {
		return fmt.Errorf("warmstorage.ValidateDataSet: %w: zero dataSetID", ErrInvalidArgument)
	}
	if s.pdpBind == nil {
		return fmt.Errorf("warmstorage.ValidateDataSet: %w", ErrPDPVerifierNotConfigured)
	}
	id := dataSetID.Big()
	live, err := s.pdpBind.DataSetLive(&bind.CallOpts{Context: ctx}, id)
	if err != nil {
		return fmt.Errorf("warmstorage.ValidateDataSet: dataSetLive: %w", err)
	}
	if !live {
		return &DataSetNotLiveError{DataSetID: dataSetID.Copy()}
	}
	listener, err := s.pdpBind.GetDataSetListener(&bind.CallOpts{Context: ctx}, id)
	if err != nil {
		return fmt.Errorf("warmstorage.ValidateDataSet: getDataSetListener: %w", err)
	}
	if listener != s.fwssAddr {
		return &DataSetNotManagedError{
			DataSetID:        dataSetID.Copy(),
			Listener:         listener,
			ExpectedListener: s.fwssAddr,
		}
	}
	return nil
}

// GetActivePieceCount returns the number of live (non-removed) pieces in
// the given data set.
func (s *Service) GetActivePieceCount(ctx context.Context, dataSetID sdktypes.BigInt) (*big.Int, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("warmstorage.GetActivePieceCount: %w: zero dataSetID", ErrInvalidArgument)
	}
	if s.pdpBind == nil {
		return nil, fmt.Errorf("warmstorage.GetActivePieceCount: %w", ErrPDPVerifierNotConfigured)
	}
	n, err := s.pdpBind.GetActivePieceCount(&bind.CallOpts{Context: ctx}, dataSetID.Big())
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetActivePieceCount: %w", err)
	}
	return n, nil
}

// GetPieceMetadata returns the (exists, value) pair for (dataSetID, pieceID, key).
func (s *Service) GetPieceMetadata(ctx context.Context, dataSetID sdktypes.BigInt, pieceID sdktypes.BigInt, key string) (bool, string, error) {
	if err := s.checkInit(); err != nil {
		return false, "", err
	}
	if dataSetID.IsZero() {
		return false, "", fmt.Errorf("warmstorage.GetPieceMetadata: %w: zero dataSetID", ErrInvalidArgument)
	}
	v, err := s.viewBind.GetPieceMetadata(&bind.CallOpts{Context: ctx}, dataSetID.Big(), pieceID.Big(), key)
	if err != nil {
		return false, "", fmt.Errorf("warmstorage.GetPieceMetadata: %w", err)
	}
	return v.Exists, v.Value, nil
}

// GetAllPieceMetadata returns a key/value map of all metadata for a
// specific (dataSetID, pieceID) pair.
func (s *Service) GetAllPieceMetadata(ctx context.Context, dataSetID sdktypes.BigInt, pieceID sdktypes.BigInt) (map[string]string, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("warmstorage.GetAllPieceMetadata: %w: zero dataSetID", ErrInvalidArgument)
	}
	raw, err := s.viewBind.GetAllPieceMetadata(&bind.CallOpts{Context: ctx}, dataSetID.Big(), pieceID.Big())
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetAllPieceMetadata: %w", err)
	}
	if len(raw.Keys) != len(raw.Values) {
		return nil, fmt.Errorf("warmstorage.GetAllPieceMetadata: mismatched keys (%d) and values (%d)", len(raw.Keys), len(raw.Values))
	}
	out := make(map[string]string, len(raw.Keys))
	for i, k := range raw.Keys {
		out[k] = raw.Values[i]
	}
	return out, nil
}

// GetOwner returns the current owner of the FWSS contract.
func (s *Service) GetOwner(ctx context.Context) (common.Address, error) {
	if err := s.checkInit(); err != nil {
		return common.Address{}, err
	}
	addr, err := s.fwssBind.Owner(&bind.CallOpts{Context: ctx})
	if err != nil {
		return common.Address{}, fmt.Errorf("warmstorage.GetOwner: %w", err)
	}
	return addr, nil
}

// IsOwner reports whether addr equals the current owner.
func (s *Service) IsOwner(ctx context.Context, addr common.Address) (bool, error) {
	got, err := s.GetOwner(ctx)
	if err != nil {
		return false, err
	}
	return got == addr, nil
}

// GetPDPConfig returns PDP proving-period parameters from FWSSView.
func (s *Service) GetPDPConfig(ctx context.Context) (*PDPConfig, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	v, err := s.viewBind.GetPDPConfig(&bind.CallOpts{Context: ctx})
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetPDPConfig: %w", err)
	}
	return &PDPConfig{
		MaxProvingPeriod:         v.MaxProvingPeriod,
		ChallengeWindowSize:      copyBigInt(v.ChallengeWindowSize),
		ChallengesPerProof:       copyBigInt(v.ChallengesPerProof),
		InitChallengeWindowStart: copyBigInt(v.InitChallengeWindowStart),
	}, nil
}

// GetClientDataSetIds returns the shallow list of data set IDs for payer
// with offset/limit pagination. Limit must be > 0; use
// IterateAllClientDataSetIds for unbounded traversal.
func (s *Service) GetClientDataSetIds(ctx context.Context, payer common.Address, opts sdktypes.ListOptions) ([]sdktypes.BigInt, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if (payer == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetIds: %w: zero payer", ErrInvalidArgument)
	}
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetIds: %w: %w", ErrInvalidArgument, err)
	}
	offset := new(big.Int).SetUint64(opts.Offset)
	limit := new(big.Int).SetUint64(opts.Limit)
	raw, err := s.viewBind.ClientDataSets(&bind.CallOpts{Context: ctx}, payer, offset, limit)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetIds: %w", err)
	}
	out, err := idconv.FromBigSlice("dataSetID", raw)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetIds: %w", err)
	}
	return out, nil
}

// GetClientDataSetsWithDetails returns the client's data sets enriched
// with pdpverifier liveness and active-piece counts. Requires the
// Service to have been configured with a PDPVerifier address. When
// onlyManaged is true, entries whose listener is not this WarmStorage
// contract are filtered out.
func (s *Service) GetClientDataSetsWithDetails(ctx context.Context, payer common.Address, onlyManaged bool) ([]*EnhancedDataSetInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.pdpBind == nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: %w", ErrPDPVerifierNotConfigured)
	}
	if (payer == common.Address{}) {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: %w: zero payer", ErrInvalidArgument)
	}
	// Collect all datasets for payer via paginated GetClientDataSets.
	var infos []*DataSetInfo
	var offset uint64
	const page uint64 = 100
	for {
		batch, berr := s.GetClientDataSets(ctx, payer, sdktypes.ListOptions{Offset: offset, Limit: page})
		if berr != nil {
			return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: %w", berr)
		}
		infos = append(infos, batch...)
		if uint64(len(batch)) < page {
			break
		}
		offset += uint64(len(batch))
	}
	out := make([]*EnhancedDataSetInfo, 0, len(infos))
	for _, info := range infos {
		id := info.DataSetID.Big()
		listener, lerr := s.pdpBind.GetDataSetListener(&bind.CallOpts{Context: ctx}, id)
		if lerr != nil {
			return nil, fmt.Errorf(
				"warmstorage.GetClientDataSetsWithDetails: getDataSetListener dataSetID %s: %w",
				info.DataSetID.String(),
				lerr,
			)
		}
		isManaged := listener == s.fwssAddr
		if onlyManaged && !isManaged {
			continue
		}
		live, lerr := s.pdpBind.DataSetLive(&bind.CallOpts{Context: ctx}, id)
		if lerr != nil {
			return nil, fmt.Errorf(
				"warmstorage.GetClientDataSetsWithDetails: dataSetLive dataSetID %s: %w",
				info.DataSetID.String(),
				lerr,
			)
		}
		metadata, merr := s.GetAllDataSetMetadata(ctx, info.DataSetID)
		if merr != nil {
			return nil, fmt.Errorf(
				"warmstorage.GetClientDataSetsWithDetails: getAllDataSetMetadata dataSetID %s: %w",
				info.DataSetID.String(),
				merr,
			)
		}
		var active *big.Int
		if live {
			n, cerr := s.pdpBind.GetActivePieceCount(&bind.CallOpts{Context: ctx}, id)
			if cerr != nil {
				return nil, fmt.Errorf(
					"warmstorage.GetClientDataSetsWithDetails: getActivePieceCount dataSetID %s: %w",
					info.DataSetID.String(),
					cerr,
				)
			}
			active = n
		} else {
			active = big.NewInt(0)
		}
		_, withCDN := metadata["withCDN"]
		withCDN = !info.CDNRailID.IsZero() && withCDN
		out = append(out, &EnhancedDataSetInfo{
			DataSetInfo:          info,
			PDPVerifierDataSetID: info.DataSetID,
			IsLive:               live,
			IsManaged:            isManaged,
			ActivePieceCount:     active,
			WithCDN:              withCDN,
			Metadata:             metadata,
		})
	}
	return out, nil
}

// TopUpCDNPaymentRails tops up the CDN egress and cache-miss rails
// associated with dataSetID. Requires Signer + Backend.
func (s *Service) TopUpCDNPaymentRails(
	ctx context.Context,
	dataSetID sdktypes.BigInt,
	cdnAmountToAdd, cacheMissAmountToAdd *big.Int,
	opts ...WriteOption,
) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.fwssWrite == nil || s.signer == nil || s.backend == nil || s.nonces == nil {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w", ErrWriteNotConfigured)
	}
	if !s.chainID.IsValid() {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w: invalid ChainID", ErrInvalidArgument)
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w: zero dataSetID", ErrInvalidArgument)
	}
	if cdnAmountToAdd == nil || cdnAmountToAdd.Sign() < 0 {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w: invalid cdnAmountToAdd", ErrInvalidArgument)
	}
	if cacheMissAmountToAdd == nil || cacheMissAmountToAdd.Sign() < 0 {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w: invalid cacheMissAmountToAdd", ErrInvalidArgument)
	}
	if cdnAmountToAdd.Sign() == 0 && cacheMissAmountToAdd.Sign() == 0 {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w: at least one top-up amount must be > 0", ErrInvalidArgument)
	}
	txOpts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w", err)
	}
	defer release()
	tx, err := s.fwssWrite.TopUpCDNPaymentRails(txOpts, dataSetID.Big(), cdnAmountToAdd, cacheMissAmountToAdd)
	release()
	if err != nil {
		return nil, fmt.Errorf("warmstorage.TopUpCDNPaymentRails: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

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

func (s *Service) finalize(ctx context.Context, tx *ethtypes.Transaction, opts []WriteOption) (*sdktypes.WriteResult, error) {
	cfg := newWriteConfig(opts)
	res := &sdktypes.WriteResult{Hash: tx.Hash()}
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
		}
		return res, fmt.Errorf("wait receipt: %w", err)
	}
	res.Receipt = receipt
	return res, nil
}
