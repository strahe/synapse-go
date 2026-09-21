package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// CreateDataSet creates an empty data set for this provider. The receiver
// remains unbound; use [ProviderContext.ForDataSet] with the returned ref to
// obtain a DataSetContext.
func (c *ProviderContext) CreateDataSet(ctx context.Context, opts *CreateDataSetOptions) (*CreateDataSetResult, error) {
	var clientDataSetID *types.BigInt
	if opts != nil {
		clientDataSetID = opts.ClientDataSetID
	}
	submission, err := c.submitCreateDataSet(ctx, clientDataSetID)
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.OnSubmitted != nil {
		opts.OnSubmitted(copyCreateDataSetSubmission(submission))
	}
	return c.waitForDataSetCreated(
		ctx,
		"storage.ProviderContext.CreateDataSet",
		submission.StatusURL,
		submission.ClientDataSetID,
	)
}

// WaitForDataSetCreated waits for a previously submitted create-dataset status
// URL. ClientDataSetID must be the value used for the original submission; zero
// is valid only when that original value was zero. The receiver remains unbound.
func (c *ProviderContext) WaitForDataSetCreated(
	ctx context.Context,
	statusURL string,
	clientDataSetID types.BigInt,
) (*CreateDataSetResult, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.ProviderContext.WaitForDataSetCreated: %w: nil context", ErrInvalidArgument)
	}
	return c.waitForDataSetCreated(
		ctx,
		"storage.ProviderContext.WaitForDataSetCreated",
		statusURL,
		clientDataSetID,
	)
}

// FindDataSetByClientDataSetID checks whether a data set created with the
// caller-owned clientDataSetID is visible on-chain for this provider. A false
// found value means no matching data set is visible yet; it does not mean the
// provider rejected the request. Callers choose their own polling policy.
//
// Persist clientDataSetID together with [ProviderContext.ProviderID] and
// [ProviderContext.ContextIdentity] before submitting the create request, then
// recreate the same context before using this method after a restart. A found
// create-and-add data set does not prove that its pieces were added.
func (c *ProviderContext) FindDataSetByClientDataSetID(
	ctx context.Context,
	clientDataSetID types.BigInt,
) (DataSetRef, bool, error) {
	const op = "storage.ProviderContext.FindDataSetByClientDataSetID"
	if c == nil || c.core == nil {
		return DataSetRef{}, false, fmt.Errorf("%s: %w: nil context", op, ErrInvalidArgument)
	}
	if !identityComplete(c.core.identity()) {
		return DataSetRef{}, false, fmt.Errorf("%s: %w: incomplete context identity", op, ErrInvalidArgument)
	}
	if c.core.dataSetReader == nil {
		return DataSetRef{}, false, fmt.Errorf("%s: %w: no FWSSDataSetReader configured", op, ErrUninitialized)
	}
	if err := ctx.Err(); err != nil {
		return DataSetRef{}, false, fmt.Errorf("%s: %w", op, err)
	}

	info, err := c.core.dataSetReader.FindDataSetByClientDataSetID(ctx, c.core.payer, clientDataSetID)
	if errors.Is(err, warmstorage.ErrNotFound) {
		return DataSetRef{}, false, nil
	}
	if err != nil {
		return DataSetRef{}, false, fmt.Errorf("%s: %w", op, err)
	}
	if info == nil {
		return c.confirmRecoveryCorrelation(ctx, clientDataSetID, nil, "reader returned no data-set record")
	}

	var mismatch string
	switch {
	case info.DataSetID.IsZero():
		mismatch = "record returned zero dataSetID"
	case info.Payer != c.core.payer:
		mismatch = fmt.Sprintf("record payer %s does not match context payer %s", info.Payer.Hex(), c.core.payer.Hex())
	case !info.ClientDataSetID.Equal(clientDataSetID):
		mismatch = fmt.Sprintf(
			"record clientDataSetID %s does not match requested ID %s",
			info.ClientDataSetID.String(),
			clientDataSetID.String(),
		)
	case !info.ProviderID.Equal(c.core.provider.ID):
		mismatch = fmt.Sprintf(
			"record providerID %s does not match context providerID %s",
			info.ProviderID.String(),
			c.core.provider.ID.String(),
		)
	case c.core.provider.ServiceProvider != (common.Address{}) && info.ServiceProvider != c.core.provider.ServiceProvider:
		mismatch = fmt.Sprintf(
			"record service provider %s does not match context service provider %s",
			info.ServiceProvider.Hex(),
			c.core.provider.ServiceProvider.Hex(),
		)
	}
	if mismatch != "" {
		return c.confirmRecoveryCorrelation(ctx, clientDataSetID, info, mismatch)
	}

	ref, err := NewDataSetRef(c.core.provider.ID, info.DataSetID, clientDataSetID)
	if err != nil {
		return DataSetRef{}, false, fmt.Errorf("%s: %w: invalid recovered data set", op, ErrDataSetCorrelationConflict)
	}
	return ref, true, nil
}

func (c *ProviderContext) confirmRecoveryCorrelation(
	ctx context.Context,
	clientDataSetID types.BigInt,
	first *warmstorage.DataSetInfo,
	detail string,
) (DataSetRef, bool, error) {
	const op = "storage.ProviderContext.FindDataSetByClientDataSetID"
	current, err := c.core.dataSetReader.FindDataSetByClientDataSetID(ctx, c.core.payer, clientDataSetID)
	if errors.Is(err, warmstorage.ErrNotFound) {
		return DataSetRef{}, false, nil
	}
	if err != nil {
		return DataSetRef{}, false, fmt.Errorf("%s: confirm correlation: %w", op, err)
	}
	if !sameRecoveryCorrelation(first, current) {
		return DataSetRef{}, false, nil
	}
	return DataSetRef{}, false, fmt.Errorf("%s: %w: %s", op, ErrDataSetCorrelationConflict, detail)
}

func sameRecoveryCorrelation(left, right *warmstorage.DataSetInfo) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.DataSetID.Equal(right.DataSetID) &&
		left.Payer == right.Payer &&
		left.ClientDataSetID.Equal(right.ClientDataSetID) &&
		left.ProviderID.Equal(right.ProviderID) &&
		left.ServiceProvider == right.ServiceProvider
}

func (c *ProviderContext) submitCreateDataSet(ctx context.Context, requestedClientDataSetID *types.BigInt) (CreateDataSetSubmission, error) {
	const op = "storage.ProviderContext.CreateDataSet"
	extraData, clientDataSetID, err := c.signCreateDataSet(ctx, op, requestedClientDataSetID)
	if err != nil {
		return CreateDataSetSubmission{}, err
	}
	created, err := c.core.client.CreateDataSet(ctx, c.core.recordKeeper, extraData)
	if err != nil {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: create dataset: %w", op, err)
	}
	if created == nil {
		return CreateDataSetSubmission{}, errors.New(op + ": create dataset returned nil result")
	}
	if created.TxHash == (common.Hash{}) {
		return CreateDataSetSubmission{}, errors.New(op + ": create dataset returned zero transactionID")
	}
	if created.StatusURL == "" {
		return CreateDataSetSubmission{}, errors.New(op + ": create dataset returned empty statusURL")
	}
	if err := validateProviderStatusURL(c.core.provider.ServiceURL, created.StatusURL); err != nil {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w", op, err)
	}
	return CreateDataSetSubmission{
		ProviderID:      copyBigInt(c.core.provider.ID),
		TransactionID:   created.TxHash.Hex(),
		StatusURL:       created.StatusURL,
		ClientDataSetID: copyBigInt(clientDataSetID),
	}, nil
}

func (c *ProviderContext) signCreateDataSet(ctx context.Context, op string, requestedClientDataSetID *types.BigInt) ([]byte, types.BigInt, error) {
	if c.core.signer == nil {
		return nil, types.BigInt{}, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}
	if !c.core.chainID.IsValid() {
		return nil, types.BigInt{}, fmt.Errorf("%s: %w: invalid chainID", op, ErrInvalidArgument)
	}
	if c.core.recordKeeper == (common.Address{}) {
		return nil, types.BigInt{}, fmt.Errorf("%s: %w: zero recordKeeper", op, ErrInvalidArgument)
	}
	if c.core.payer == (common.Address{}) {
		return nil, types.BigInt{}, fmt.Errorf("%s: %w: zero payer", op, ErrInvalidArgument)
	}

	clientDataSetID, err := clientDataSetIDOrRandom(requestedClientDataSetID)
	if err != nil {
		return nil, types.BigInt{}, fmt.Errorf("%s: %w", op, err)
	}
	dataSetMetadata, err := dataSetMetadataEntries(c.core.dataSetMetadata, c.core.withCDN)
	if err != nil {
		return nil, types.BigInt{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, types.BigInt{}, fmt.Errorf("%s: %w", op, err)
	}
	domain := ityped.NewDomain(c.core.chainID.BigInt(), c.core.recordKeeper)
	createSig, err := ityped.SignCreateDataSet(
		c.core.signHashFunc(),
		domain,
		clientDataSetID.Big(),
		c.core.provider.Payee,
		dataSetMetadata,
	)
	if err != nil {
		return nil, types.BigInt{}, fmt.Errorf("%s: sign create dataset: %w", op, err)
	}
	extraData, err := encodeCreateDataSetExtraData(c.core.payer, clientDataSetID.Big(), dataSetMetadata, signatureBytes(createSig))
	if err != nil {
		return nil, types.BigInt{}, err
	}
	return extraData, clientDataSetID, nil
}

func (c *ProviderContext) waitForDataSetCreated(
	ctx context.Context,
	op string,
	statusURL string,
	clientDataSetID types.BigInt,
) (*CreateDataSetResult, error) {
	if err := validateRecoveryStatusURL(op, c.core.provider.ServiceURL, statusURL); err != nil {
		return nil, err
	}

	status, err := c.core.client.WaitForDataSetCreated(ctx, statusURL, 0)
	if err != nil {
		return nil, wrapRecoveryStatusError(op, "wait dataset created", err)
	}
	if status == nil {
		return nil, errors.New(op + ": wait dataset created returned nil status")
	}
	if status.DataSetID == nil || status.DataSetID.IsZero() {
		return nil, errors.New(op + ": server returned zero dataSetID")
	}
	if status.CreateMessageHash == (common.Hash{}) {
		return nil, fmt.Errorf("%s: %w: server returned zero transactionID", op, pdp.ErrInvalidStatus)
	}

	ref, err := NewDataSetRef(c.core.provider.ID, *status.DataSetID, clientDataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return &CreateDataSetResult{
		TransactionID:          status.CreateMessageHash.Hex(),
		ConfirmedTransactionID: optionalHashString(status.ConfirmedTxHash),
		DataSet:                ref,
	}, nil
}

func copyCreateDataSetSubmission(in CreateDataSetSubmission) CreateDataSetSubmission {
	out := in
	out.ProviderID = copyBigInt(in.ProviderID)
	out.ClientDataSetID = copyBigInt(in.ClientDataSetID)
	return out
}
