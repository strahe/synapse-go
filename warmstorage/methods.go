package warmstorage

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/contracts/fwssview"
	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
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

// HasActivePieces reports whether the data set contains at least one active
// piece. It reads at most one piece, so its cost does not grow with the total
// number of pieces. Missing and non-live data sets report false.
func (s *Service) HasActivePieces(ctx context.Context, dataSetID sdktypes.BigInt) (bool, error) {
	if err := s.checkInit(); err != nil {
		return false, err
	}
	if dataSetID.IsZero() {
		return false, fmt.Errorf("warmstorage.HasActivePieces: %w: zero dataSetID", ErrInvalidArgument)
	}
	if s.pdpBind == nil {
		return false, fmt.Errorf("warmstorage.HasActivePieces: %w", ErrPDPVerifierNotConfigured)
	}
	result, err := s.pdpBind.GetActivePieces(
		&bind.CallOpts{Context: ctx},
		dataSetID.Big(),
		new(big.Int),
		big.NewInt(1),
	)
	if err != nil {
		if pdpverifier.IsDataSetUnavailable(err) {
			return false, nil
		}
		return false, fmt.Errorf("warmstorage.HasActivePieces: %w", err)
	}
	return len(result.Pieces) > 0, nil
}

// GetPieceMetadata returns the (exists, value) pair for (dataSetID, pieceID, key).
func (s *Service) GetPieceMetadata(ctx context.Context, dataSetID, pieceID sdktypes.BigInt, key string) (bool, string, error) {
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
func (s *Service) GetAllPieceMetadata(ctx context.Context, dataSetID, pieceID sdktypes.BigInt) (map[string]string, error) {
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
// contract are filtered out. Detail reads use serial Multicall3 batches;
// separate batches may observe different blocks. When sub-calls fail, the
// method returns the earliest error in data-set and operation order.
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
	infos, err := s.GetAllClientDataSets(ctx, payer)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: %w", err)
	}
	if len(infos) == 0 {
		return []*EnhancedDataSetInfo{}, nil
	}

	pdpABI, err := pdpverifier.PDPVerifierMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: parse PDPVerifier ABI: %w", err)
	}
	viewABI, err := fwssview.FWSSViewMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: parse FWSSView ABI: %w", err)
	}

	states := make([]dataSetDetailsState, len(infos))
	listenerCalls := make([]iabi.Call3, len(infos))
	for i, info := range infos {
		states[i] = dataSetDetailsState{info: info}
		callData, err := pdpABI.Pack("getDataSetListener", info.DataSetID.Big())
		if err != nil {
			return nil, dataSetDetailsError("getDataSetListener", info.DataSetID, err)
		}
		listenerCalls[i] = iabi.Call3{Target: s.pdpVerifierAdr, AllowFailure: true, CallData: callData}
	}
	listenerResults, err := iabi.BatchCallChunked(ctx, s.caller, listenerCalls, s.maxMulticallCalls)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: getDataSetListener batch: %w", err)
	}
	listenerMethod := pdpABI.Methods["getDataSetListener"]
	for i, result := range listenerResults {
		values, err := unpackDataSetDetailsResult(result, listenerMethod.Outputs.Unpack)
		if err != nil {
			states[i].errs[detailsListener] = dataSetDetailsError("getDataSetListener", infos[i].DataSetID, err)
			continue
		}
		listener, ok := values[0].(common.Address)
		if !ok {
			states[i].errs[detailsListener] = dataSetDetailsError(
				"getDataSetListener",
				infos[i].DataSetID,
				fmt.Errorf("unexpected output type %T", values[0]),
			)
			continue
		}
		states[i].isManaged = listener == s.fwssAddr
	}

	var detailCalls []iabi.Call3
	var detailRefs []dataSetDetailsCallRef
	for i := range states {
		state := &states[i]
		if state.errs[detailsListener] != nil || onlyManaged && !state.isManaged {
			continue
		}
		liveData, err := pdpABI.Pack("dataSetLive", state.info.DataSetID.Big())
		if err != nil {
			return nil, dataSetDetailsError("dataSetLive", state.info.DataSetID, err)
		}
		metadataData, err := viewABI.Pack("getAllDataSetMetadata", state.info.DataSetID.Big())
		if err != nil {
			return nil, dataSetDetailsError("getAllDataSetMetadata", state.info.DataSetID, err)
		}
		detailCalls = append(detailCalls,
			iabi.Call3{Target: s.pdpVerifierAdr, AllowFailure: true, CallData: liveData},
			iabi.Call3{Target: s.viewAddr, AllowFailure: true, CallData: metadataData},
		)
		detailRefs = append(detailRefs,
			dataSetDetailsCallRef{state: i, operation: detailsLive},
			dataSetDetailsCallRef{state: i, operation: detailsMetadata},
		)
	}
	detailResults, err := iabi.BatchCallChunked(ctx, s.caller, detailCalls, s.maxMulticallCalls)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: live/metadata batch: %w", err)
	}
	liveMethod := pdpABI.Methods["dataSetLive"]
	metadataMethod := viewABI.Methods["getAllDataSetMetadata"]
	for i, result := range detailResults {
		ref := detailRefs[i]
		state := &states[ref.state]
		switch ref.operation {
		case detailsLive:
			values, err := unpackDataSetDetailsResult(result, liveMethod.Outputs.Unpack)
			if err != nil {
				state.errs[detailsLive] = dataSetDetailsError("dataSetLive", state.info.DataSetID, err)
				continue
			}
			live, ok := values[0].(bool)
			if !ok {
				state.errs[detailsLive] = dataSetDetailsError(
					"dataSetLive",
					state.info.DataSetID,
					fmt.Errorf("unexpected output type %T", values[0]),
				)
				continue
			}
			state.isLive = live
			if !live {
				state.activePieceCount = big.NewInt(0)
			}
		case detailsMetadata:
			values, err := unpackDataSetDetailsResult(result, metadataMethod.Outputs.Unpack)
			if err != nil {
				state.errs[detailsMetadata] = dataSetDetailsError("getAllDataSetMetadata", state.info.DataSetID, err)
				continue
			}
			keys, keysOK := values[0].([]string)
			metadataValues, valuesOK := values[1].([]string)
			if !keysOK || !valuesOK {
				state.errs[detailsMetadata] = dataSetDetailsError(
					"getAllDataSetMetadata",
					state.info.DataSetID,
					fmt.Errorf("unexpected output types %T and %T", values[0], values[1]),
				)
				continue
			}
			state.metadata, err = dataSetMetadataMap(keys, metadataValues)
			if err != nil {
				state.errs[detailsMetadata] = dataSetDetailsError("getAllDataSetMetadata", state.info.DataSetID, err)
			}
		}
	}

	var activeCalls []iabi.Call3
	var activeStates []int
	for i := range states {
		state := &states[i]
		if state.errs[detailsListener] != nil || state.errs[detailsLive] != nil || state.errs[detailsMetadata] != nil ||
			onlyManaged && !state.isManaged || !state.isLive {
			continue
		}
		callData, err := pdpABI.Pack("getActivePieceCount", state.info.DataSetID.Big())
		if err != nil {
			return nil, dataSetDetailsError("getActivePieceCount", state.info.DataSetID, err)
		}
		activeCalls = append(activeCalls, iabi.Call3{Target: s.pdpVerifierAdr, AllowFailure: true, CallData: callData})
		activeStates = append(activeStates, i)
	}
	activeResults, err := iabi.BatchCallChunked(ctx, s.caller, activeCalls, s.maxMulticallCalls)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.GetClientDataSetsWithDetails: getActivePieceCount batch: %w", err)
	}
	activeMethod := pdpABI.Methods["getActivePieceCount"]
	for i, result := range activeResults {
		state := &states[activeStates[i]]
		values, err := unpackDataSetDetailsResult(result, activeMethod.Outputs.Unpack)
		if err != nil {
			state.errs[detailsActivePieceCount] = dataSetDetailsError("getActivePieceCount", state.info.DataSetID, err)
			continue
		}
		count, ok := values[0].(*big.Int)
		if !ok {
			state.errs[detailsActivePieceCount] = dataSetDetailsError(
				"getActivePieceCount",
				state.info.DataSetID,
				fmt.Errorf("unexpected output type %T", values[0]),
			)
			continue
		}
		state.activePieceCount = count
	}

	out := make([]*EnhancedDataSetInfo, 0, len(states))
	for i := range states {
		state := &states[i]
		for _, err := range state.errs {
			if err != nil {
				return nil, err
			}
		}
		if onlyManaged && !state.isManaged {
			continue
		}
		_, withCDN := state.metadata["withCDN"]
		out = append(out, &EnhancedDataSetInfo{
			DataSetInfo:          state.info,
			PDPVerifierDataSetID: state.info.DataSetID,
			IsLive:               state.isLive,
			IsManaged:            state.isManaged,
			ActivePieceCount:     state.activePieceCount,
			WithCDN:              !state.info.CDNRailID.IsZero() && withCDN,
			Metadata:             state.metadata,
		})
	}
	return out, nil
}

const (
	detailsListener = iota
	detailsLive
	detailsMetadata
	detailsActivePieceCount
	detailsOperationCount
)

type dataSetDetailsState struct {
	info             *DataSetInfo
	isLive           bool
	isManaged        bool
	activePieceCount *big.Int
	metadata         map[string]string
	errs             [detailsOperationCount]error
}

type dataSetDetailsCallRef struct {
	state     int
	operation int
}

func unpackDataSetDetailsResult(result iabi.Result3, unpack func([]byte) ([]any, error)) ([]any, error) {
	if !result.Success {
		return nil, errors.New("sub-call failed")
	}
	if len(result.ReturnData) == 0 {
		return nil, errors.New("empty return data")
	}
	values, err := unpack(result.ReturnData)
	if err != nil {
		return nil, fmt.Errorf("unpack: %w", err)
	}
	if len(values) == 0 {
		return nil, errors.New("empty output")
	}
	return values, nil
}

func dataSetDetailsError(operation string, dataSetID sdktypes.BigInt, err error) error {
	return fmt.Errorf(
		"warmstorage.GetClientDataSetsWithDetails: %s dataSetID %s: %w",
		operation,
		dataSetID.String(),
		err,
	)
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
