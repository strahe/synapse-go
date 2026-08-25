package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

// CreateDataSet creates an empty data set for this provider. The receiver
// remains unbound; use [ProviderContext.ForDataSet] with the returned ref to
// obtain a DataSetContext.
func (c *ProviderContext) CreateDataSet(ctx context.Context, opts *CreateDataSetOptions) (*CreateDataSetResult, error) {
	submission, err := c.submitCreateDataSet(ctx)
	if err != nil {
		return nil, err
	}
	if opts != nil && opts.OnSubmitted != nil {
		opts.OnSubmitted(copyCreateDataSetSubmission(submission))
	}
	return c.waitForDataSetCreated(ctx, "storage.ProviderContext.CreateDataSet", submission)
}

// WaitForDataSetCreated waits for a previously submitted create-dataset
// transaction. The receiver remains unbound.
func (c *ProviderContext) WaitForDataSetCreated(ctx context.Context, submission CreateDataSetSubmission) (*CreateDataSetResult, error) {
	return c.waitForDataSetCreated(ctx, "storage.ProviderContext.WaitForDataSetCreated", submission)
}

func (c *ProviderContext) submitCreateDataSet(ctx context.Context) (CreateDataSetSubmission, error) {
	const op = "storage.ProviderContext.CreateDataSet"
	extraData, clientDataSetID, err := c.signCreateDataSet(ctx, op)
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
	return CreateDataSetSubmission{
		ProviderID:      copyBigInt(c.core.provider.ID),
		TransactionID:   created.TxHash.Hex(),
		StatusURL:       created.StatusURL,
		ClientDataSetID: copyClientDataSetIDPtr(clientDataSetID),
	}, nil
}

func (c *ProviderContext) signCreateDataSet(ctx context.Context, op string) ([]byte, types.BigInt, error) {
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

	clientDataSetID, err := randomClientDataSetID()
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
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, types.BigInt{}, fmt.Errorf("%s: wrapped/decorated EVMSigner values are unsupported: %w", op, err)
		}
		return nil, types.BigInt{}, fmt.Errorf("%s: sign create dataset: %w", op, err)
	}
	extraData, err := encodeCreateDataSetExtraData(c.core.payer, clientDataSetID.Big(), dataSetMetadata, signatureBytes(createSig))
	if err != nil {
		return nil, types.BigInt{}, err
	}
	return extraData, clientDataSetID, nil
}

func (c *ProviderContext) waitForDataSetCreated(ctx context.Context, op string, submission CreateDataSetSubmission) (*CreateDataSetResult, error) {
	submission, err := validateCreateDataSetSubmission(op, c.core.provider.ID, submission)
	if err != nil {
		return nil, err
	}

	status, err := c.core.client.WaitForDataSetCreated(ctx, submission.StatusURL, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: wait dataset created: %w", op, err)
	}
	if status == nil {
		return nil, errors.New(op + ": wait dataset created returned nil status")
	}
	if status.DataSetID == nil || status.DataSetID.IsZero() {
		return nil, errors.New(op + ": server returned zero dataSetID")
	}
	wantTransactionID := common.HexToHash(submission.TransactionID)
	if got := status.CreateMessageHash; got != wantTransactionID {
		return nil, fmt.Errorf(
			"%s: %w: server returned mismatched transactionID: got %s want %s",
			op,
			ErrInvalidArgument,
			got.Hex(),
			wantTransactionID.Hex(),
		)
	}

	return &CreateDataSetResult{
		TransactionID: submission.TransactionID,
		DataSet: DataSetRef{
			ProviderID:      copyBigInt(c.core.provider.ID),
			DataSetID:       copyBigInt(*status.DataSetID),
			ClientDataSetID: copyClientDataSetIDFromPtr(submission.ClientDataSetID),
		},
	}, nil
}

func validateCreateDataSetSubmission(op string, providerID types.BigInt, submission CreateDataSetSubmission) (CreateDataSetSubmission, error) {
	submission = copyCreateDataSetSubmission(submission)
	if submission.ProviderID.IsZero() {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: zero providerID", op, ErrInvalidArgument)
	}
	if !submission.ProviderID.Equal(providerID) {
		return CreateDataSetSubmission{}, fmt.Errorf(
			"%s: %w: submission providerID %s does not match context providerID %s",
			op,
			ErrInvalidArgument,
			submission.ProviderID.String(),
			providerID.String(),
		)
	}
	if submission.TransactionID == "" {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: empty transactionID", op, ErrInvalidArgument)
	}
	if !common.IsHexHash(submission.TransactionID) {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: invalid transactionID %q", op, ErrInvalidArgument, submission.TransactionID)
	}
	wantTransactionID := common.HexToHash(submission.TransactionID)
	if wantTransactionID == (common.Hash{}) {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: invalid transactionID %q", op, ErrInvalidArgument, submission.TransactionID)
	}
	if submission.StatusURL == "" {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: empty statusURL", op, ErrInvalidArgument)
	}
	if submission.ClientDataSetID == nil {
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: missing clientDataSetID", op, ErrInvalidArgument)
	}
	return submission, nil
}

func copyCreateDataSetSubmission(in CreateDataSetSubmission) CreateDataSetSubmission {
	out := in
	out.ProviderID = copyBigInt(in.ProviderID)
	out.ClientDataSetID = copyBigIntPtr(in.ClientDataSetID)
	return out
}
