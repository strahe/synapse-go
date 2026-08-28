package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

const defaultTerminateWait = 5 * time.Minute

// TerminateServiceOptions configures provider-relayed service termination.
type TerminateServiceOptions struct {
	// SkipProvider uses the direct FWSS transaction path. The new
	// TerminateService helper still waits for the receipt so it can return
	// EndEpoch. Use DataSetContext.Terminate or Service.TerminateDataSet for the
	// legacy hash-only direct write surface.
	SkipProvider bool
	// WriteOptions are used only when SkipProvider is true.
	WriteOptions []warmstorage.WriteOption
	// DirectWaitTimeout is used only when SkipProvider is true. Zero selects
	// the default wait timeout.
	DirectWaitTimeout time.Duration
	// ProviderWaitTimeout bounds provider relay submission plus status polling.
	// Zero selects the default wait timeout. Negative disables the added timeout
	// and leaves cancellation to ctx.
	ProviderWaitTimeout time.Duration
	// PollInterval is used only for provider-relayed status polling.
	PollInterval time.Duration
	// OnSubmitted is called when the provider's original transaction hash
	// becomes known. A replacement-by-fee hash is returned as ConfirmedTxHash.
	OnSubmitted func(common.Hash)
}

// TerminateServiceResult is the high-level service termination result.
type TerminateServiceResult struct {
	// TxHash is the provider's original submission hash when available.
	TxHash *common.Hash
	// ConfirmedTxHash is the hash included on chain. It may differ from TxHash
	// after replacement-by-fee. For explorers and receipt lookups, prefer this
	// field when non-nil and otherwise use TxHash.
	ConfirmedTxHash *common.Hash
	DataSetID       types.BigInt
	EndEpoch        types.Epoch
}

// Terminate schedules termination of this context's data set via the
// FWSS terminateService entry point. On success the provider stops
// proving the data set and all contained pieces will be removed
// on-chain.
//
// opts are forwarded to warmstorage.Service.TerminateDataSet (wait /
// confirmations / etc.).
func (c *DataSetContext) Terminate(ctx context.Context, opts ...warmstorage.WriteOption) (*types.WriteResult, error) {
	if c.core.fwssTerminator == nil {
		return nil, errors.New("storage.DataSetContext.Terminate: FWSS terminator not configured")
	}
	return c.core.fwssTerminator.TerminateDataSet(ctx, c.ref.DataSetID(), opts...)
}

// TerminateService terminates this context's data set. By default it asks the
// provider to relay the termination; pass SkipProvider to use the direct FWSS
// path.
func (c *DataSetContext) TerminateService(ctx context.Context, opts *TerminateServiceOptions) (*TerminateServiceResult, error) {
	const op = "storage.DataSetContext.TerminateService"
	if opts != nil && opts.SkipProvider {
		return terminateServiceDirect(ctx, op, c.core.fwssTerminator, c.ref.DataSetID(), opts)
	}
	target, err := c.snapshotProviderTerminateTarget(op)
	if err != nil {
		return nil, err
	}
	if err := ensureTerminationAccountSettled(ctx, op, target.paymentReader, target.epochReader, target.paymentToken, target.payer); err != nil {
		return nil, err
	}
	extraData, err := signTerminateServiceExtraData(target.signHash, target.chainID, target.recordKeeper, target.dataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	providerCtx, cancel := terminateProviderContext(ctx, opts)
	defer cancel()
	res, err := target.client.TerminateService(providerCtx, pdp.TerminateServiceRequest{
		DataSetID: target.dataSetID,
		ExtraData: extraData,
	})
	var pendingErr error
	if err != nil {
		if already, ok := errors.AsType[*pdp.ServiceAlreadyTerminatedError](err); ok {
			return &TerminateServiceResult{DataSetID: target.dataSetID, EndEpoch: already.ServiceTerminationEpoch}, nil
		}
		if _, ok := errors.AsType[*pdp.TerminateServicePendingError](err); !ok {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		pendingErr = err
	} else if res == nil {
		return nil, fmt.Errorf("%s: provider returned nil termination result", op)
	}
	status, err := target.client.WaitForTerminateService(providerCtx, target.dataSetID, terminatePollInterval(opts), terminateHashCallback(opts))
	if err != nil {
		if pendingErr != nil {
			if _, ok := errors.AsType[*pdp.WaitForTerminateServiceNotFoundError](err); ok {
				return nil, fmt.Errorf("%s: %w", op, pendingErr)
			}
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return terminateResultFromStatus(target.dataSetID, status), nil
}

// TerminateService terminates an FWSS-managed data set by ID. By default it
// resolves the provider and asks it to relay the termination; pass
// SkipProvider to use the direct FWSS transaction path.
func (s *Service) TerminateService(ctx context.Context, dataSetID types.BigInt, opts *TerminateServiceOptions) (*TerminateServiceResult, error) {
	const op = "storage.Service.TerminateService"
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("%s: %w: zero dataSetID", op, ErrInvalidArgument)
	}
	if opts != nil && opts.SkipProvider {
		return terminateServiceDirect(ctx, op, s.terminator, dataSetID, opts)
	}
	if s.dsReader == nil {
		return nil, fmt.Errorf("%s: %w: no FWSSDataSetReader configured", op, ErrUninitialized)
	}
	if s.providers == nil {
		return nil, fmt.Errorf("%s: %w: no ProviderResolver configured", op, ErrUninitialized)
	}
	if s.signer == nil {
		return nil, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}
	if !s.chainID.IsValid() {
		return nil, fmt.Errorf("%s: %w: invalid chainID", op, ErrInvalidArgument)
	}
	if s.recordKeeper == (common.Address{}) {
		return nil, fmt.Errorf("%s: %w: zero recordKeeper", op, ErrInvalidArgument)
	}
	dataSet, err := s.dsReader.GetDataSet(ctx, dataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: GetDataSet: %w", op, err)
	}
	if dataSet == nil {
		return nil, fmt.Errorf("%s: GetDataSet returned nil", op)
	}
	if dataSet.Payer != s.payerAddr {
		return nil, fmt.Errorf("%s: %w: data set payer %s does not match configured payer %s", op, ErrInvalidArgument, dataSet.Payer.Hex(), s.payerAddr.Hex())
	}
	if err := ensureTerminationAccountSettled(ctx, op, s.payments, s.epochs, s.paymentToken, dataSet.Payer); err != nil {
		return nil, err
	}
	provider, err := s.providers.ResolveProvider(ctx, dataSet.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("%s: ResolveProvider: %w", op, err)
	}
	client, err := pdp.New(provider.ServiceURL, pdp.WithHTTPClient(s.httpClient), pdp.WithLogger(s.logger))
	if err != nil {
		return nil, fmt.Errorf("%s: create PDP client: %w", op, err)
	}
	extraData, err := signTerminateServiceExtraData(func(hash []byte) ([]byte, error) {
		return signer.SignHash(s.signer, hash)
	}, s.chainID, s.recordKeeper, dataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	providerCtx, cancel := terminateProviderContext(ctx, opts)
	defer cancel()
	res, err := client.TerminateService(providerCtx, pdp.TerminateServiceRequest{
		DataSetID: dataSetID,
		ExtraData: extraData,
	})
	var pendingErr error
	if err != nil {
		if already, ok := errors.AsType[*pdp.ServiceAlreadyTerminatedError](err); ok {
			return &TerminateServiceResult{DataSetID: dataSetID, EndEpoch: already.ServiceTerminationEpoch}, nil
		}
		if _, ok := errors.AsType[*pdp.TerminateServicePendingError](err); !ok {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		pendingErr = err
	} else if res == nil {
		return nil, fmt.Errorf("%s: provider returned nil termination result", op)
	}
	status, err := client.WaitForTerminateService(providerCtx, dataSetID, terminatePollInterval(opts), terminateHashCallback(opts))
	if err != nil {
		if pendingErr != nil {
			if _, ok := errors.AsType[*pdp.WaitForTerminateServiceNotFoundError](err); ok {
				return nil, fmt.Errorf("%s: %w", op, pendingErr)
			}
		}
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return terminateResultFromStatus(dataSetID, status), nil
}

type providerTerminateClient interface {
	TerminateService(context.Context, pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error)
	WaitForTerminateService(context.Context, types.BigInt, time.Duration, func(common.Hash)) (*pdp.TerminateServiceStatus, error)
}

type providerTerminateTarget struct {
	dataSetID     types.BigInt
	client        providerTerminateClient
	signHash      func([]byte) ([]byte, error)
	chainID       types.ChainID
	recordKeeper  common.Address
	payer         common.Address
	paymentReader PaymentStateReader
	epochReader   EpochReader
	paymentToken  common.Address
}

func (c *DataSetContext) snapshotProviderTerminateTarget(op string) (providerTerminateTarget, error) {
	if c.core.signer == nil {
		return providerTerminateTarget{}, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}
	client, ok := c.core.client.(providerTerminateClient)
	if !ok {
		return providerTerminateTarget{}, fmt.Errorf("%s: %w: PDP client does not support termination", op, ErrUninitialized)
	}
	if !c.core.chainID.IsValid() {
		return providerTerminateTarget{}, fmt.Errorf("%s: %w: invalid chainID", op, ErrInvalidArgument)
	}
	if c.core.recordKeeper == (common.Address{}) {
		return providerTerminateTarget{}, fmt.Errorf("%s: %w: zero recordKeeper", op, ErrInvalidArgument)
	}
	return providerTerminateTarget{
		dataSetID:     c.ref.DataSetID(),
		client:        client,
		signHash:      c.core.signHashFunc(),
		chainID:       c.core.chainID,
		recordKeeper:  c.core.recordKeeper,
		payer:         c.core.payer,
		paymentReader: c.core.paymentReader,
		epochReader:   c.core.epochReader,
		paymentToken:  c.core.paymentToken,
	}, nil
}

func terminateServiceDirect(ctx context.Context, op string, terminator FWSSTerminator, dataSetID types.BigInt, opts *TerminateServiceOptions) (*TerminateServiceResult, error) {
	if terminator == nil {
		return nil, fmt.Errorf("%s: %w: no FWSS terminator configured", op, ErrUninitialized)
	}
	writeOpts := []warmstorage.WriteOption(nil)
	if opts != nil {
		writeOpts = append(writeOpts, opts.WriteOptions...)
	}
	wait := defaultTerminateWait
	if opts != nil && opts.DirectWaitTimeout > 0 {
		wait = opts.DirectWaitTimeout
	}
	writeOpts = append(writeOpts, warmstorage.WithWait(wait))
	res, err := terminator.TerminateDataSet(ctx, dataSetID, writeOpts...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if res == nil || res.Receipt == nil {
		return nil, fmt.Errorf("%s: direct termination did not return a receipt", op)
	}
	if opts != nil && opts.OnSubmitted != nil {
		opts.OnSubmitted(res.Hash)
	}
	confirmedHash := res.Receipt.TxHash
	if confirmedHash == (common.Hash{}) {
		return nil, fmt.Errorf("%s: direct termination receipt has zero transaction hash", op)
	}
	ev, err := warmstorage.ExtractPDPPaymentTerminatedEvent(res.Receipt)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &TerminateServiceResult{
		TxHash:          &confirmedHash,
		ConfirmedTxHash: &confirmedHash,
		DataSetID:       dataSetID,
		EndEpoch:        ev.EndEpoch,
	}, nil
}

func signTerminateServiceExtraData(signHash func([]byte) ([]byte, error), chainID types.ChainID, recordKeeper common.Address, dataSetID types.BigInt) ([]byte, error) {
	domain := ityped.NewDomain(chainID.BigInt(), recordKeeper)
	sig, err := ityped.SignTerminateService(signHash, domain, dataSetID.Big())
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, fmt.Errorf("wrapped/decorated EVMSigner values are unsupported: %w", err)
		}
		return nil, fmt.Errorf("sign terminate service: %w", err)
	}
	extraData, err := encodeSignatureExtraData(signatureBytes(sig))
	if err != nil {
		return nil, fmt.Errorf("encode terminate service extraData: %w", err)
	}
	return extraData, nil
}

func ensureTerminationAccountSettled(ctx context.Context, op string, pay PaymentStateReader, epochs EpochReader, token, payer common.Address) error {
	if pay == nil {
		return fmt.Errorf("%s: %w: no PaymentStateReader configured", op, ErrUninitialized)
	}
	if epochs == nil {
		return fmt.Errorf("%s: %w: no EpochReader configured", op, ErrUninitialized)
	}
	if token == (common.Address{}) {
		return fmt.Errorf("%s: %w: zero payment token", op, ErrInvalidArgument)
	}
	if payer == (common.Address{}) {
		return fmt.Errorf("%s: %w: zero payer", op, ErrInvalidArgument)
	}
	block, err := epochs.BlockNumber(ctx)
	if err != nil {
		return fmt.Errorf("%s: BlockNumber: %w", op, err)
	}
	account, err := pay.AccountInfo(ctx, token, payer)
	if err != nil {
		return fmt.Errorf("%s: AccountInfo: %w", op, err)
	}
	if account == nil {
		return fmt.Errorf("%s: AccountInfo returned nil", op)
	}
	debt := account.DebtAt(new(big.Int).SetUint64(block))
	if debt.Sign() > 0 {
		return &TerminateServiceDebtError{Shortfall: debt}
	}
	return nil
}

func terminatePollInterval(opts *TerminateServiceOptions) time.Duration {
	if opts != nil && opts.PollInterval > 0 {
		return opts.PollInterval
	}
	return 0
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

func terminateHashCallback(opts *TerminateServiceOptions) func(common.Hash) {
	if opts == nil {
		return nil
	}
	return opts.OnSubmitted
}

func terminateResultFromStatus(dataSetID types.BigInt, status *pdp.TerminateServiceStatus) *TerminateServiceResult {
	out := &TerminateServiceResult{DataSetID: dataSetID}
	if status == nil {
		return out
	}
	if status.TerminationTxHash != nil {
		h := *status.TerminationTxHash
		out.TxHash = &h
	}
	if status.ConfirmedTxHash != nil {
		h := *status.ConfirmedTxHash
		out.ConfirmedTxHash = &h
	}
	out.EndEpoch = status.ServiceTerminationEpoch
	return out
}
