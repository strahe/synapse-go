package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

const defaultTerminateWait = 5 * time.Minute

// TerminateServiceOptions configures termination submission and confirmation.
type TerminateServiceOptions struct {
	// SkipProvider broadcasts through FWSS. The service and its payments
	// continue until EndEpoch. The default provider path requires settlement.
	SkipProvider bool
	// WriteOptions apply only to direct submission and receipt waiting.
	// WithWait and WithOnSubmitted are overridden; WithConfirmations is kept.
	WriteOptions []warmstorage.WriteOption
	// DirectWaitTimeout bounds direct receipt waiting. A non-positive value
	// selects five minutes. SubmitTerminateService does not wait for a receipt.
	DirectWaitTimeout time.Duration
	// ProviderWaitTimeout bounds provider submission and, for TerminateService,
	// polling and reconciliation together. Zero selects five minutes; negative
	// adds no deadline beyond ctx.
	ProviderWaitTimeout time.Duration
	// PollInterval applies only to provider polling, not submission.
	PollInterval time.Duration
	// OnSubmitted receives an independent handle synchronously after positive
	// acknowledgement and before waiting. It does not indicate confirmation.
	// Errors also preserve recovery information; callback panics propagate.
	OnSubmitted func(TerminateServiceSubmission)
}

// TerminateServiceResult records an arranged service termination. It does not
// wait for EndEpoch or for data-set cleanup. Provider reconciliation can confirm
// the goal state without proving that the original relay transaction succeeded.
type TerminateServiceResult struct {
	// TxHash is the original transaction hash when available.
	TxHash *common.Hash
	// ConfirmedTxHash is the observed on-chain hash after any replacement.
	// Prefer it for receipt lookups when available; otherwise use TxHash.
	ConfirmedTxHash *common.Hash
	DataSetID       types.BigInt
	// EndEpoch is when service and payments end, not a data deletion time.
	EndEpoch types.Epoch
}

type terminationTarget struct {
	dataSetID   types.BigInt
	providerID  types.BigInt
	direct      bool
	client      PDPProviderClient
	extraData   []byte
	terminator  FWSSTerminator
	reader      FWSSDataSetReader
	checkClosed func() error
}

// SubmitTerminateService submits once without waiting for confirmation.
// Prepare errors prove this call did not submit. Submit errors preserve a
// handle with an unknown outcome; inspect TerminateServiceError and its cause.
// A provider reporting that the service already ended returns
// *pdp.ServiceAlreadyTerminatedError without a TerminateServiceError.
func (s *Service) SubmitTerminateService(ctx context.Context, dataSetID types.BigInt, opts *TerminateServiceOptions) (*TerminateServiceSubmission, error) {
	const op = "storage.Service.SubmitTerminateService"
	target, err := s.prepareTermination(ctx, dataSetID, opts)
	if err != nil {
		return nil, terminationError(op, TerminateServiceStagePrepare, nil, nil, err)
	}
	return submitPreparedTermination(ctx, op, target, opts)
}

// SubmitTerminateService submits this context's termination without waiting.
// Errors preserve the stage and any recovery handle in TerminateServiceError,
// except *pdp.ServiceAlreadyTerminatedError when the service already ended.
func (c *DataSetContext) SubmitTerminateService(ctx context.Context, opts *TerminateServiceOptions) (*TerminateServiceSubmission, error) {
	const op = "storage.DataSetContext.SubmitTerminateService"
	target, err := c.prepareTermination(ctx, opts)
	if err != nil {
		return nil, terminationError(op, TerminateServiceStagePrepare, nil, nil, err)
	}
	return submitPreparedTermination(ctx, op, target, opts)
}

// TerminateService submits once and waits for an arranged termination. Wait
// errors retain the submission and observations in TerminateServiceError.
func (s *Service) TerminateService(ctx context.Context, dataSetID types.BigInt, opts *TerminateServiceOptions) (*TerminateServiceResult, error) {
	const op = "storage.Service.TerminateService"
	target, err := s.prepareTermination(ctx, dataSetID, opts)
	if err != nil {
		return nil, terminationError(op, TerminateServiceStagePrepare, nil, nil, err)
	}
	return terminatePreparedService(ctx, op, target, opts)
}

// TerminateService submits once and waits. It does not wait until EndEpoch or
// remove remaining data-set state. Errors retain recovery facts without a callback.
func (c *DataSetContext) TerminateService(ctx context.Context, opts *TerminateServiceOptions) (*TerminateServiceResult, error) {
	const op = "storage.DataSetContext.TerminateService"
	target, err := c.prepareTermination(ctx, opts)
	if err != nil {
		return nil, terminationError(op, TerminateServiceStagePrepare, nil, nil, err)
	}
	return terminatePreparedService(ctx, op, target, opts)
}

// WaitForTerminateService only observes an existing submission. Timeout includes
// provider resolution and chain reconciliation. No signer is needed. Recovery
// must use the original chain and record keeper. A missing reader disables chain
// reconciliation; a missing data set does not establish successful termination.
func (s *Service) WaitForTerminateService(ctx context.Context, sub TerminateServiceSubmission, opts *TerminateServiceWaitOptions) (*TerminateServiceResult, error) {
	const op = "storage.Service.WaitForTerminateService"
	work := copyTerminationSubmission(sub)
	if err := s.checkInit(); err != nil {
		return nil, terminationError(op, TerminateServiceStageWait, &work, nil, err)
	}
	if err := validateTerminationSubmission(&work); err != nil {
		return nil, terminationError(op, TerminateServiceStageWait, &work, nil, err)
	}
	waitOpts := normalizedTerminationWaitOptions(opts)
	waitCtx, cancel := context.WithTimeout(ctx, waitOpts.Timeout)
	defer cancel()
	target := terminationTarget{dataSetID: work.DataSetID, providerID: work.ProviderID, direct: work.Direct, terminator: s.terminator, reader: s.dsReader, checkClosed: s.checkInit}
	if !work.Direct {
		client, err := s.terminationProviderClient(waitCtx, work.ProviderID)
		if err != nil {
			return reconcileTermination(waitCtx, op, &work, target, err)
		}
		target.client = client
	}
	return waitPreparedTermination(waitCtx, op, target, work, waitOpts)
}

// WaitForTerminateService observes a saved handle for this data set without
// sending a write request. Errors preserve the latest known handle and receipt.
func (c *DataSetContext) WaitForTerminateService(ctx context.Context, sub TerminateServiceSubmission, opts *TerminateServiceWaitOptions) (*TerminateServiceResult, error) {
	const op = "storage.DataSetContext.WaitForTerminateService"
	work := copyTerminationSubmission(sub)
	if c == nil || c.core == nil {
		return nil, terminationError(op, TerminateServiceStageWait, &work, nil, fmt.Errorf("%w: nil data-set context", ErrInvalidArgument))
	}
	if err := validateTerminationSubmission(&work); err != nil {
		return nil, terminationError(op, TerminateServiceStageWait, &work, nil, err)
	}
	if !work.DataSetID.Equal(c.ref.DataSetID()) || (!work.Direct && !work.ProviderID.Equal(c.core.provider.ID)) {
		return nil, terminationError(op, TerminateServiceStageWait, &work, nil, fmt.Errorf("%w: termination submission does not match context", ErrInvalidArgument))
	}
	waitOpts := normalizedTerminationWaitOptions(opts)
	waitCtx, cancel := context.WithTimeout(ctx, waitOpts.Timeout)
	defer cancel()
	target := terminationTarget{dataSetID: work.DataSetID, providerID: work.ProviderID, direct: work.Direct, client: c.core.client, terminator: c.core.fwssTerminator, reader: c.core.dataSetReader}
	return waitPreparedTermination(waitCtx, op, target, work, waitOpts)
}

func (s *Service) prepareTermination(ctx context.Context, dataSetID types.BigInt, opts *TerminateServiceOptions) (terminationTarget, error) {
	if err := s.checkInit(); err != nil {
		return terminationTarget{}, err
	}
	if err := ctx.Err(); err != nil {
		return terminationTarget{}, err
	}
	if dataSetID.IsZero() {
		return terminationTarget{}, fmt.Errorf("%w: zero dataSetID", ErrInvalidArgument)
	}
	target := terminationTarget{dataSetID: copyBigInt(dataSetID), terminator: s.terminator, reader: s.dsReader, checkClosed: s.checkInit}
	if opts != nil && opts.SkipProvider {
		target.direct = true
		if target.terminator == nil {
			return target, fmt.Errorf("%w: no FWSS terminator configured", ErrUninitialized)
		}
		return target, nil
	}
	if s.dsReader == nil {
		return target, fmt.Errorf("%w: no FWSSDataSetReader configured", ErrUninitialized)
	}
	if s.signer == nil {
		return target, fmt.Errorf("%w: nil signer", ErrInvalidArgument)
	}
	if !s.chainID.IsValid() {
		return target, fmt.Errorf("%w: invalid chainID", ErrInvalidArgument)
	}
	if s.recordKeeper == (common.Address{}) {
		return target, fmt.Errorf("%w: zero recordKeeper", ErrInvalidArgument)
	}
	dataSet, err := s.dsReader.GetDataSet(ctx, dataSetID)
	if err != nil {
		return target, fmt.Errorf("GetDataSet: %w", err)
	}
	if dataSet == nil {
		return target, errors.New("GetDataSet returned nil")
	}
	if dataSet.Payer != s.payerAddr {
		return target, fmt.Errorf("%w: data set payer %s does not match configured payer %s", ErrInvalidArgument, dataSet.Payer.Hex(), s.payerAddr.Hex())
	}
	if err := ensureTerminationAccountSettled(ctx, s.payments, s.epochs, s.paymentToken, dataSet.Payer); err != nil {
		return target, err
	}
	target.providerID = copyBigInt(dataSet.ProviderID)
	target.client, err = s.terminationProviderClient(ctx, target.providerID)
	if err != nil {
		return target, err
	}
	target.extraData, err = signTerminateServiceExtraData(s.signer.SignHash, s.chainID, s.recordKeeper, dataSetID)
	return target, err
}

func (c *DataSetContext) prepareTermination(ctx context.Context, opts *TerminateServiceOptions) (terminationTarget, error) {
	if c == nil || c.core == nil {
		return terminationTarget{}, fmt.Errorf("%w: nil data-set context", ErrInvalidArgument)
	}
	if err := ctx.Err(); err != nil {
		return terminationTarget{}, err
	}
	target := terminationTarget{dataSetID: c.ref.DataSetID(), providerID: c.core.provider.ID, client: c.core.client, terminator: c.core.fwssTerminator, reader: c.core.dataSetReader}
	if opts != nil && opts.SkipProvider {
		target.direct = true
		target.providerID = types.BigInt{}
		if target.terminator == nil {
			return target, fmt.Errorf("%w: no FWSS terminator configured", ErrUninitialized)
		}
		return target, nil
	}
	if c.core.signer == nil {
		return target, fmt.Errorf("%w: nil signer", ErrInvalidArgument)
	}
	if !c.core.chainID.IsValid() {
		return target, fmt.Errorf("%w: invalid chainID", ErrInvalidArgument)
	}
	if c.core.recordKeeper == (common.Address{}) {
		return target, fmt.Errorf("%w: zero recordKeeper", ErrInvalidArgument)
	}
	if target.client == nil {
		return target, fmt.Errorf("%w: no PDP client configured", ErrUninitialized)
	}
	if err := ensureTerminationAccountSettled(ctx, c.core.paymentReader, c.core.epochReader, c.core.paymentToken, c.core.payer); err != nil {
		return target, err
	}
	var err error
	target.extraData, err = signTerminateServiceExtraData(c.core.signHashFunc(), c.core.chainID, c.core.recordKeeper, target.dataSetID)
	return target, err
}

func (s *Service) terminationProviderClient(ctx context.Context, providerID types.BigInt) (PDPProviderClient, error) {
	if s.providers == nil {
		return nil, fmt.Errorf("%w: no ProviderResolver configured", ErrUninitialized)
	}
	provider, err := s.providers.ResolveProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("ResolveProvider: %w", err)
	}
	client, err := pdp.New(provider.ServiceURL, pdp.WithHTTPClient(s.providerHTTPClient), pdp.WithLogger(s.logger))
	if err != nil {
		return nil, fmt.Errorf("create PDP client: %w", err)
	}
	return client, nil
}

func submitPreparedTermination(ctx context.Context, op string, target terminationTarget, opts *TerminateServiceOptions) (*TerminateServiceSubmission, error) {
	if !target.direct {
		providerCtx, cancel := terminateProviderContext(ctx, opts)
		defer cancel()
		ctx = providerCtx
	}
	return target.submit(ctx, op, opts)
}

func terminatePreparedService(ctx context.Context, op string, target terminationTarget, opts *TerminateServiceOptions) (*TerminateServiceResult, error) {
	if !target.direct {
		providerCtx, cancel := terminateProviderContext(ctx, opts)
		defer cancel()
		ctx = providerCtx
	}
	sub, err := target.submit(ctx, op, opts)
	if err != nil {
		if already, ok := errors.AsType[*pdp.ServiceAlreadyTerminatedError](err); ok {
			return terminationResult(TerminateServiceSubmission{DataSetID: target.dataSetID}, already.ServiceTerminationEpoch), nil
		}
		return nil, err
	}
	waitOpts := TerminateServiceWaitOptions{Timeout: defaultTerminateWait}
	if opts != nil {
		waitOpts.PollInterval = opts.PollInterval
		waitOpts.WriteOptions = append([]warmstorage.WriteOption(nil), opts.WriteOptions...)
		if opts.DirectWaitTimeout > 0 {
			waitOpts.Timeout = opts.DirectWaitTimeout
		}
	}
	if target.direct {
		waitCtx, cancel := context.WithTimeout(ctx, waitOpts.Timeout)
		defer cancel()
		ctx = waitCtx
	}
	return waitPreparedTermination(ctx, op, target, *sub, waitOpts)
}

func (target terminationTarget) submit(ctx context.Context, op string, opts *TerminateServiceOptions) (*TerminateServiceSubmission, error) {
	if err := ctx.Err(); err != nil {
		return nil, terminationError(op, TerminateServiceStagePrepare, nil, nil, err)
	}
	sub := TerminateServiceSubmission{DataSetID: copyBigInt(target.dataSetID), Direct: target.direct, ProviderID: copyBigInt(target.providerID)}
	if target.direct {
		var writeOpts []warmstorage.WriteOption
		if opts != nil {
			writeOpts = append(writeOpts, opts.WriteOptions...)
		}
		res, err := target.terminator.TerminateDataSet(ctx, target.dataSetID, FWSSTerminationOptions{WriteOptions: writeOpts})
		if err != nil {
			if broadcast, ok := errors.AsType[*types.BroadcastError](err); ok {
				if broadcast.Transaction != nil {
					sub.TransactionID = broadcast.Transaction.Hash().Hex()
				}
				return nil, terminationError(op, TerminateServiceStageSubmit, &sub, nil, err)
			}
			return nil, terminationError(op, TerminateServiceStagePrepare, nil, nil, err)
		}
		if res == nil || res.Hash == (common.Hash{}) {
			return nil, terminationError(op, TerminateServiceStageSubmit, &sub, nil, errors.New("direct termination returned no transaction hash"))
		}
		sub.TransactionID = res.Hash.Hex()
	} else {
		res, err := target.client.TerminateService(ctx, pdp.TerminateServiceRequest{DataSetID: target.dataSetID, ExtraData: target.extraData})
		if err != nil {
			if _, ok := errors.AsType[*pdp.ServiceAlreadyTerminatedError](err); ok {
				return nil, fmt.Errorf("%s: %w", op, err)
			}
			if _, ok := errors.AsType[*pdp.TerminateServicePendingError](err); !ok {
				return nil, terminationError(op, TerminateServiceStageSubmit, &sub, nil, err)
			}
			sub.Queued = true
		} else if res == nil {
			return nil, terminationError(op, TerminateServiceStageSubmit, &sub, nil, errors.New("provider returned nil termination result"))
		}
	}
	sub.Accepted = true
	if opts != nil && opts.OnSubmitted != nil {
		opts.OnSubmitted(copyTerminationSubmission(sub))
	}
	out := copyTerminationSubmission(sub)
	return &out, nil
}

func waitPreparedTermination(ctx context.Context, op string, target terminationTarget, work TerminateServiceSubmission, opts TerminateServiceWaitOptions) (*TerminateServiceResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, terminationError(op, TerminateServiceStageWait, &work, nil, err)
	}
	if target.checkClosed != nil {
		if err := target.checkClosed(); err != nil {
			return nil, terminationError(op, TerminateServiceStageWait, &work, nil, err)
		}
	}
	if target.direct {
		if target.terminator == nil {
			return nil, terminationError(op, TerminateServiceStageWait, &work, nil, fmt.Errorf("%w: no FWSS terminator configured", ErrUninitialized))
		}
		hash := common.HexToHash(work.TransactionID)
		if work.ConfirmedTransactionID != "" {
			hash = common.HexToHash(work.ConfirmedTransactionID)
		}
		res, endEpoch, err := target.terminator.WaitForTermination(ctx, work.DataSetID, hash, opts.Timeout, opts.WriteOptions)
		var receipt *ethtypes.Receipt
		if res != nil {
			receipt = res.Receipt
			if receipt != nil {
				work.Accepted = true
				confirmed := receipt.TxHash
				if confirmed == (common.Hash{}) {
					confirmed = hash
				}
				work.ConfirmedTransactionID = confirmed.Hex()
			}
		}
		if err != nil {
			return nil, terminationError(op, TerminateServiceStageWait, &work, receipt, err)
		}
		if receipt == nil {
			return nil, terminationError(op, TerminateServiceStageWait, &work, nil, errors.New("direct termination returned no receipt"))
		}
		return terminationResult(work, endEpoch), nil
	}
	if target.client == nil {
		return reconcileTermination(ctx, op, &work, target, fmt.Errorf("%w: no PDP client configured", ErrUninitialized))
	}
	var pollOpts []pdp.WaitForTerminateServiceOption
	knownHash := work.TransactionID
	if knownHash == "" {
		knownHash = work.ConfirmedTransactionID
	}
	if knownHash != "" {
		pollOpts = append(pollOpts, pdp.WithKnownTerminationTxHash(common.HexToHash(knownHash)))
	}
	status, err := target.client.WaitForTerminateService(ctx, work.DataSetID, opts.PollInterval, nil, pollOpts...)
	if status != nil {
		applyTerminationStatus(&work, status)
	}
	if err == nil {
		if status != nil && status.FWSSTerminated {
			return terminationResult(work, status.ServiceTerminationEpoch), nil
		}
		err = errors.New("provider returned no confirmed termination status")
	}
	if work.Queued {
		if _, ok := errors.AsType[*pdp.WaitForTerminateServiceNotFoundError](err); ok {
			err = errors.Join(&pdp.TerminateServicePendingError{}, err)
		}
	}
	return reconcileTermination(ctx, op, &work, target, err)
}

func reconcileTermination(ctx context.Context, op string, work *TerminateServiceSubmission, target terminationTarget, cause error) (*TerminateServiceResult, error) {
	if errors.Is(cause, ErrClosed) {
		return nil, terminationError(op, TerminateServiceStageWait, work, nil, cause)
	}
	if err := ctx.Err(); err != nil {
		return nil, terminationError(op, TerminateServiceStageWait, work, nil, errors.Join(cause, err))
	}
	if target.checkClosed != nil {
		if err := target.checkClosed(); err != nil {
			return nil, terminationError(op, TerminateServiceStageWait, work, nil, errors.Join(cause, err))
		}
	}
	if target.reader != nil {
		info, err := target.reader.GetDataSet(ctx, work.DataSetID)
		switch {
		case err != nil:
			cause = errors.Join(cause, fmt.Errorf("GetDataSet: %w", err))
		case ctx.Err() != nil:
			return nil, terminationError(op, TerminateServiceStageWait, work, nil, errors.Join(cause, ctx.Err()))
		case info == nil:
			cause = errors.Join(cause, errors.New("GetDataSet returned nil"))
		case info.PDPEndEpoch != 0:
			return terminationResult(*work, info.PDPEndEpoch), nil
		}
	}
	if err := ctx.Err(); err != nil {
		cause = errors.Join(cause, err)
	}
	return nil, terminationError(op, TerminateServiceStageWait, work, nil, cause)
}

func validateTerminationSubmission(sub *TerminateServiceSubmission) error {
	if sub.DataSetID.IsZero() {
		return fmt.Errorf("%w: zero dataSetID", ErrInvalidArgument)
	}
	if sub.Direct {
		if !sub.ProviderID.IsZero() || sub.Queued {
			return fmt.Errorf("%w: direct submission contains provider relay fields", ErrInvalidArgument)
		}
		if sub.TransactionID == "" {
			return fmt.Errorf("%w: direct submission requires a transaction hash", ErrInvalidArgument)
		}
	} else if sub.ProviderID.IsZero() {
		return fmt.Errorf("%w: zero providerID", ErrInvalidArgument)
	}
	for _, value := range []*string{&sub.TransactionID, &sub.ConfirmedTransactionID} {
		if *value == "" {
			continue
		}
		if !common.IsHexHash(*value) || common.HexToHash(*value) == (common.Hash{}) {
			return fmt.Errorf("%w: invalid termination transaction hash", ErrInvalidArgument)
		}
		*value = common.HexToHash(*value).Hex()
	}
	return nil
}

// applyTerminationStatus records a provider status. A status reporting any hash
// replaces both hashes, so they always come from the same response.
func applyTerminationStatus(work *TerminateServiceSubmission, status *pdp.TerminateServiceStatus) {
	work.Accepted = true
	var original, confirmed string
	if status.TerminationTxHash != nil && *status.TerminationTxHash != (common.Hash{}) {
		original = status.TerminationTxHash.Hex()
	}
	if status.ConfirmedTxHash != nil && *status.ConfirmedTxHash != (common.Hash{}) {
		confirmed = status.ConfirmedTxHash.Hex()
	}
	if original != "" || confirmed != "" {
		work.TransactionID, work.ConfirmedTransactionID = original, confirmed
	}
}

func copyTerminationSubmission(sub TerminateServiceSubmission) TerminateServiceSubmission {
	sub.DataSetID = copyBigInt(sub.DataSetID)
	sub.ProviderID = copyBigInt(sub.ProviderID)
	return sub
}

func terminationError(op string, stage TerminateServiceStage, sub *TerminateServiceSubmission, receipt *ethtypes.Receipt, cause error) error {
	var saved *TerminateServiceSubmission
	if sub != nil {
		copied := copyTerminationSubmission(*sub)
		saved = &copied
	}
	return fmt.Errorf("%s: %w", op, &TerminateServiceError{Stage: stage, Submission: saved, Receipt: receipt, Cause: cause})
}

func terminationResult(sub TerminateServiceSubmission, endEpoch types.Epoch) *TerminateServiceResult {
	out := &TerminateServiceResult{DataSetID: copyBigInt(sub.DataSetID), EndEpoch: endEpoch}
	if sub.TransactionID != "" {
		hash := common.HexToHash(sub.TransactionID)
		out.TxHash = &hash
	}
	if sub.ConfirmedTransactionID != "" {
		hash := common.HexToHash(sub.ConfirmedTransactionID)
		out.ConfirmedTxHash = &hash
	}
	return out
}

func normalizedTerminationWaitOptions(opts *TerminateServiceWaitOptions) TerminateServiceWaitOptions {
	out := TerminateServiceWaitOptions{Timeout: defaultTerminateWait}
	if opts != nil {
		out = *opts
		out.WriteOptions = append([]warmstorage.WriteOption(nil), opts.WriteOptions...)
	}
	if out.Timeout <= 0 {
		out.Timeout = defaultTerminateWait
	}
	return out
}

func signTerminateServiceExtraData(signHash func([]byte) ([]byte, error), chainID types.ChainID, recordKeeper common.Address, dataSetID types.BigInt) ([]byte, error) {
	domain := ityped.NewDomain(chainID.BigInt(), recordKeeper)
	sig, err := ityped.SignTerminateService(signHash, domain, dataSetID.Big())
	if err != nil {
		return nil, fmt.Errorf("sign terminate service: %w", err)
	}
	extraData, err := encodeSignatureExtraData(signatureBytes(sig))
	if err != nil {
		return nil, fmt.Errorf("encode terminate service extraData: %w", err)
	}
	return extraData, nil
}

func ensureTerminationAccountSettled(ctx context.Context, pay PaymentStateReader, epochs EpochReader, token, payer common.Address) error {
	if pay == nil {
		return fmt.Errorf("%w: no PaymentStateReader configured", ErrUninitialized)
	}
	if epochs == nil {
		return fmt.Errorf("%w: no EpochReader configured", ErrUninitialized)
	}
	if token == (common.Address{}) {
		return fmt.Errorf("%w: zero payment token", ErrInvalidArgument)
	}
	if payer == (common.Address{}) {
		return fmt.Errorf("%w: zero payer", ErrInvalidArgument)
	}
	block, err := epochs.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("BlockNumber: %w", err)
	}
	account, err := pay.AccountInfo(ctx, token, payer)
	if err != nil {
		return fmt.Errorf("AccountInfo: %w", err)
	}
	if account == nil {
		return errors.New("AccountInfo returned nil")
	}
	debt := account.DebtAt(new(big.Int).SetUint64(block))
	if debt.Sign() > 0 {
		return &TerminateServiceDebtError{Shortfall: debt}
	}
	return nil
}

func terminateProviderContext(ctx context.Context, opts *TerminateServiceOptions) (context.Context, context.CancelFunc) {
	if opts != nil && opts.ProviderWaitTimeout < 0 {
		return ctx, func() {}
	}
	wait := defaultTerminateWait
	if opts != nil && opts.ProviderWaitTimeout > 0 {
		wait = opts.ProviderWaitTimeout
	}
	return context.WithTimeout(ctx, wait)
}
