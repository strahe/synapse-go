package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/idconv"
	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

// CreateDataSet creates an empty data set for this context's provider and
// binds the confirmed DataSetID to the context.
func (c *Context) CreateDataSet(ctx context.Context, opts *CreateDataSetOptions) (*CreateDataSetResult, error) {
	submission, submitted, err := c.submitCreateDataSet(ctx)
	if err != nil {
		return nil, err
	}
	if submitted && opts != nil && opts.OnSubmitted != nil {
		opts.OnSubmitted(copyCreateDataSetSubmission(submission))
	}
	return c.waitForDataSetCreated(ctx, "storage.Context.CreateDataSet", submission)
}

// WaitForDataSetCreated waits for a previously submitted create-dataset
// transaction and binds the confirmed DataSetID to the context.
func (c *Context) WaitForDataSetCreated(ctx context.Context, submission CreateDataSetSubmission) (*CreateDataSetResult, error) {
	submission, err := c.prepareWaitForDataSetCreated("storage.Context.WaitForDataSetCreated", submission)
	if err != nil {
		return nil, err
	}
	return c.waitForDataSetCreated(ctx, "storage.Context.WaitForDataSetCreated", submission)
}

func (c *Context) submitCreateDataSet(ctx context.Context) (CreateDataSetSubmission, bool, error) {
	c.commitMu.Lock()
	defer c.commitMu.Unlock()

	c.mu.Lock()
	if c.dataSetID != nil {
		c.mu.Unlock()
		return CreateDataSetSubmission{}, false, fmt.Errorf("storage.Context.CreateDataSet: %w: context already has dataSetID %d", ErrInvalidArgument, *c.dataSetID)
	}
	if c.pendingCreate != nil {
		pending := copyCreateDataSetSubmission(*c.pendingCreate)
		c.mu.Unlock()
		return pending, false, nil
	}
	if c.createInFlight {
		c.mu.Unlock()
		return CreateDataSetSubmission{}, false, fmt.Errorf("storage.Context.CreateDataSet: %w: dataset creation is pending; complete CreateDataSet or WaitForDataSetCreated first", ErrInvalidArgument)
	}
	c.createInFlight = true
	c.mu.Unlock()
	inFlight := true
	defer func() {
		if inFlight {
			c.clearCreateInFlight()
		}
	}()

	extraData, clientDataSetID, recordKeeper, err := c.signCreateDataSet(ctx, "storage.Context.CreateDataSet")
	if err != nil {
		return CreateDataSetSubmission{}, false, err
	}
	created, err := c.client.CreateDataSet(ctx, recordKeeper, extraData)
	if err != nil {
		return CreateDataSetSubmission{}, false, fmt.Errorf("storage.Context.CreateDataSet: create dataset: %w", err)
	}
	if created == nil {
		return CreateDataSetSubmission{}, false, errors.New("storage.Context.CreateDataSet: create dataset returned nil result")
	}
	if created.TxHash == (common.Hash{}) {
		return CreateDataSetSubmission{}, false, errors.New("storage.Context.CreateDataSet: create dataset returned zero transactionID")
	}
	if created.StatusURL == "" {
		return CreateDataSetSubmission{}, false, errors.New("storage.Context.CreateDataSet: create dataset returned empty statusURL")
	}

	submission := CreateDataSetSubmission{
		TransactionID:   created.TxHash.Hex(),
		StatusURL:       created.StatusURL,
		ClientDataSetID: copyClientDataSetID(clientDataSetID),
	}
	c.mu.Lock()
	if c.dataSetID != nil {
		c.createInFlight = false
		inFlight = false
		c.mu.Unlock()
		return CreateDataSetSubmission{}, false, fmt.Errorf("storage.Context.CreateDataSet: %w: context already has dataSetID %d", ErrInvalidArgument, *c.dataSetID)
	}
	c.pendingCreate = cloneCreateDataSetSubmissionPtr(submission)
	c.createInFlight = false
	inFlight = false
	c.mu.Unlock()

	return submission, true, nil
}

func (c *Context) prepareWaitForDataSetCreated(op string, submission CreateDataSetSubmission) (CreateDataSetSubmission, error) {
	c.commitMu.Lock()
	defer c.commitMu.Unlock()

	return c.rememberPendingDataSetCreation(op, submission)
}

func (c *Context) signCreateDataSet(ctx context.Context, op string) ([]byte, types.ClientDataSetID, common.Address, error) {
	if c.signer == nil {
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}

	c.mu.Lock()
	if c.pendingCreate != nil {
		c.mu.Unlock()
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w: dataset creation is pending; complete CreateDataSet or WaitForDataSetCreated first", op, ErrInvalidArgument)
	}
	if c.dataSetID != nil {
		c.mu.Unlock()
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w: context already has dataSetID %d", op, ErrInvalidArgument, *c.dataSetID)
	}
	if c.clientDataSetID == nil {
		v, err := randomClientDataSetID()
		if err != nil {
			c.mu.Unlock()
			return nil, nil, common.Address{}, fmt.Errorf("%s: %w", op, err)
		}
		c.clientDataSetID = v
	}
	clientDataSetID := copyClientDataSetID(c.clientDataSetID)
	dataSetMetadataSnap := cloneStringMap(c.dataSetMetadata)
	payerSnap := c.payer
	payeeSnap := c.provider.Payee
	withCDNSnap := c.withCDN
	chainIDSnap := c.chainID
	recordKeeperSnap := c.recordKeeper
	c.mu.Unlock()

	if !chainIDSnap.IsValid() {
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w: invalid chainID", op, ErrInvalidArgument)
	}
	if recordKeeperSnap == (common.Address{}) {
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w: zero recordKeeper", op, ErrInvalidArgument)
	}
	if payerSnap == (common.Address{}) {
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w: zero payer", op, ErrInvalidArgument)
	}
	dataSetMetadata, err := dataSetMetadataEntries(dataSetMetadataSnap, withCDNSnap)
	if err != nil {
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w", op, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, common.Address{}, fmt.Errorf("%s: %w", op, err)
	}
	domain := ityped.NewDomain(chainIDSnap.BigInt(), recordKeeperSnap)
	createSig, err := ityped.SignCreateDataSet(c.signHashFunc(), domain, clientDataSetID, payeeSnap, dataSetMetadata)
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, nil, common.Address{}, fmt.Errorf("%s: wrapped/decorated EVMSigner values are unsupported: %w", op, err)
		}
		return nil, nil, common.Address{}, fmt.Errorf("%s: sign create dataset: %w", op, err)
	}
	extraData, err := encodeCreateDataSetExtraData(payerSnap, clientDataSetID, dataSetMetadata, signatureBytes(createSig))
	if err != nil {
		return nil, nil, common.Address{}, err
	}
	return extraData, clientDataSetID, recordKeeperSnap, nil
}

func (c *Context) waitForDataSetCreated(ctx context.Context, op string, submission CreateDataSetSubmission) (*CreateDataSetResult, error) {
	submission, err := c.rememberPendingDataSetCreation(op, submission)
	if err != nil {
		return nil, err
	}

	status, err := c.client.WaitForDataSetCreated(ctx, submission.StatusURL, 0)
	if err != nil {
		if errors.Is(err, pdp.ErrTxRejected) {
			c.forgetPendingDataSetCreation(submission)
		}
		return nil, fmt.Errorf("%s: wait dataset created: %w", op, err)
	}
	if status == nil {
		c.forgetPendingDataSetCreation(submission)
		return nil, errors.New(op + ": wait dataset created returned nil status")
	}
	dataSetID, err := idconv.Safe[types.DataSetID]("dataSetID", status.DataSetID)
	if err != nil {
		c.forgetPendingDataSetCreation(submission)
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if dataSetID == 0 {
		c.forgetPendingDataSetCreation(submission)
		return nil, errors.New(op + ": server returned zero dataSetID")
	}
	transactionID := submission.TransactionID
	wantTransactionID := common.HexToHash(submission.TransactionID)
	if got := status.CreateMessageHash; got != wantTransactionID {
		c.forgetPendingDataSetCreation(submission)
		return nil, fmt.Errorf("%s: %w: server returned mismatched transactionID: got %s want %s", op, ErrInvalidArgument, got.Hex(), wantTransactionID.Hex())
	}

	c.mu.Lock()
	if c.dataSetID != nil && *c.dataSetID != dataSetID {
		existingID := *c.dataSetID
		c.mu.Unlock()
		return nil, fmt.Errorf("%s: %w: server returned mismatched dataSetID: got %d want %d", op, ErrInvalidArgument, dataSetID, existingID)
	}
	newID := dataSetID
	c.dataSetID = &newID
	c.clientDataSetID = copyClientDataSetID(submission.ClientDataSetID)
	c.clientIDFromPending = false
	c.pendingCreate = nil
	c.mu.Unlock()

	return &CreateDataSetResult{
		TransactionID:   transactionID,
		DataSetID:       dataSetID,
		ClientDataSetID: copyClientDataSetID(submission.ClientDataSetID),
	}, nil
}

func (c *Context) rememberPendingDataSetCreation(op string, submission CreateDataSetSubmission) (CreateDataSetSubmission, error) {
	submission = copyCreateDataSetSubmission(submission)
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
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: nil clientDataSetID", op, ErrInvalidArgument)
	}

	c.mu.Lock()
	bound := c.dataSetID != nil
	if c.clientDataSetID != nil && c.clientDataSetID.Cmp(submission.ClientDataSetID) != 0 {
		c.mu.Unlock()
		return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: mismatched clientDataSetID", op, ErrInvalidArgument)
	}
	if !bound {
		if c.pendingCreate != nil && !sameCreateDataSetSubmission(*c.pendingCreate, submission) {
			c.mu.Unlock()
			return CreateDataSetSubmission{}, fmt.Errorf("%s: %w: different dataset creation is pending", op, ErrInvalidArgument)
		}
		c.pendingCreate = cloneCreateDataSetSubmissionPtr(submission)
		if c.clientDataSetID == nil {
			c.clientDataSetID = copyClientDataSetID(submission.ClientDataSetID)
			c.clientIDFromPending = true
		}
	}
	c.mu.Unlock()
	return submission, nil
}

func (c *Context) clearCreateInFlight() {
	c.mu.Lock()
	c.createInFlight = false
	c.mu.Unlock()
}

func (c *Context) forgetPendingDataSetCreation(submission CreateDataSetSubmission) {
	c.mu.Lock()
	if c.pendingCreate != nil && sameCreateDataSetSubmission(*c.pendingCreate, submission) {
		c.pendingCreate = nil
		if c.clientIDFromPending && sameClientDataSetID(c.clientDataSetID, submission.ClientDataSetID) {
			c.clientDataSetID = nil
			c.clientIDFromPending = false
		}
	}
	c.mu.Unlock()
}

func copyCreateDataSetSubmission(in CreateDataSetSubmission) CreateDataSetSubmission {
	out := in
	out.ClientDataSetID = copyClientDataSetID(in.ClientDataSetID)
	return out
}

func cloneCreateDataSetSubmissionPtr(in CreateDataSetSubmission) *CreateDataSetSubmission {
	out := copyCreateDataSetSubmission(in)
	return &out
}

func sameCreateDataSetSubmission(a, b CreateDataSetSubmission) bool {
	if a.TransactionID != b.TransactionID || a.StatusURL != b.StatusURL {
		return false
	}
	return sameClientDataSetID(a.ClientDataSetID, b.ClientDataSetID)
}

func sameClientDataSetID(a, b types.ClientDataSetID) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Cmp(b) == 0
	}
}
