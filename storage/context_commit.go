package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

const commitPollInterval = 4 * time.Second

// SubmitCommit submits one create-and-add transaction and returns without
// waiting for confirmation.
func (c *ProviderContext) SubmitCommit(ctx context.Context, req CommitRequest) (*CommitSubmission, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.ProviderContext.SubmitCommit: %w: nil context", ErrInvalidArgument)
	}
	return c.core.submitCommit(ctx, "storage.ProviderContext.SubmitCommit", nil, req)
}

// GetCommitStatus checks a create-and-add submission once. A caller-supplied
// status URL outside the provider origin returns an error matching both
// [ErrInvalidArgument] and [pdp.ErrStatusURLOrigin].
func (c *ProviderContext) GetCommitStatus(ctx context.Context, submission CommitSubmission) (*CommitStatus, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.ProviderContext.GetCommitStatus: %w: nil context", ErrInvalidArgument)
	}
	return c.core.getCommitStatus(ctx, "storage.ProviderContext.GetCommitStatus", nil, submission)
}

// WaitForCommit waits for a create-and-add submission to confirm or reject. A
// caller-supplied status URL outside the provider origin returns an error
// matching both [ErrInvalidArgument] and [pdp.ErrStatusURLOrigin].
func (c *ProviderContext) WaitForCommit(ctx context.Context, submission CommitSubmission) (*CommitResult, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.ProviderContext.WaitForCommit: %w: nil context", ErrInvalidArgument)
	}
	return c.core.waitForCommit(ctx, "storage.ProviderContext.WaitForCommit", nil, submission)
}

// SubmitCommit submits one add-pieces transaction and returns without waiting
// for confirmation.
func (c *DataSetContext) SubmitCommit(ctx context.Context, req CommitRequest) (*CommitSubmission, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.DataSetContext.SubmitCommit: %w: nil context", ErrInvalidArgument)
	}
	return c.core.submitCommit(ctx, "storage.DataSetContext.SubmitCommit", &c.ref, req)
}

// GetCommitStatus checks an add-pieces submission once. A caller-supplied
// status URL outside the provider origin returns an error matching both
// [ErrInvalidArgument] and [pdp.ErrStatusURLOrigin].
func (c *DataSetContext) GetCommitStatus(ctx context.Context, submission CommitSubmission) (*CommitStatus, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.DataSetContext.GetCommitStatus: %w: nil context", ErrInvalidArgument)
	}
	return c.core.getCommitStatus(ctx, "storage.DataSetContext.GetCommitStatus", &c.ref, submission)
}

// WaitForCommit waits for an add-pieces submission to confirm or reject. A
// caller-supplied status URL outside the provider origin returns an error
// matching both [ErrInvalidArgument] and [pdp.ErrStatusURLOrigin].
func (c *DataSetContext) WaitForCommit(ctx context.Context, submission CommitSubmission) (*CommitResult, error) {
	if c == nil || c.core == nil {
		return nil, fmt.Errorf("storage.DataSetContext.WaitForCommit: %w: nil context", ErrInvalidArgument)
	}
	return c.core.waitForCommit(ctx, "storage.DataSetContext.WaitForCommit", &c.ref, submission)
}

func (c *contextCore) commit(ctx context.Context, op string, ref *DataSetRef, req CommitRequest) (*CommitResult, error) {
	submission, err := c.submitCommit(ctx, op, ref, req)
	if err != nil {
		return nil, err
	}
	return c.waitForCommit(ctx, op, ref, *submission)
}

func (c *contextCore) submitCommit(
	ctx context.Context,
	op string,
	ref *DataSetRef,
	req CommitRequest,
) (*CommitSubmission, error) {
	pieceCIDs, err := validateCommitRequest(op, req)
	if err != nil {
		return nil, err
	}
	if err := c.validateWritableDataSet(ctx, op, ref); err != nil {
		return nil, err
	}

	extraData := append([]byte(nil), req.ExtraData...)
	var clientDataSetID *types.BigInt
	if len(extraData) == 0 {
		extraData, clientDataSetID, err = c.presignForCommit(ctx, op, ref, req.Pieces)
		if err != nil {
			return nil, err
		}
	} else if ref == nil {
		if !identityComplete(c.identity()) {
			return nil, fmt.Errorf("%s: %w: create-and-add requires a complete context identity", op, ErrInvalidArgument)
		}
		clientDataSetID, err = decodeCreateAndAddIdentity(op, extraData, c.payer)
		if err != nil {
			return nil, err
		}
	}

	pieces := make([]pdp.AddPieceInput, len(pieceCIDs))
	for i, pieceCID := range pieceCIDs {
		pieces[i] = pdp.AddPieceInput{PieceCID: pieceCID}
	}

	submission := CommitSubmission{
		ProviderID: copyBigInt(c.provider.ID),
		Identity:   c.identity(),
		PieceCIDs:  append([]cid.Cid(nil), pieceCIDs...),
	}
	if ref != nil {
		submission.Kind = CommitKindAddPieces
		dataSet := copyDataSetRef(*ref)
		submission.DataSet = &dataSet

		added, err := c.client.AddPieces(ctx, ref.dataSetID, pieces, extraData)
		if err != nil {
			return nil, fmt.Errorf("%s: add pieces: %w", op, err)
		}
		if added == nil {
			return nil, errors.New(op + ": add pieces returned nil result")
		}
		submission.TransactionID = added.TxHash.Hex()
		submission.StatusURL = added.StatusURL
	} else {
		submission.Kind = CommitKindCreateAndAdd
		submission.ClientDataSetID = copyBigIntPtr(clientDataSetID)

		created, err := c.client.CreateDataSetAndAddPieces(ctx, c.recordKeeper, pieces, extraData)
		if err != nil {
			return nil, fmt.Errorf("%s: create and add pieces: %w", op, err)
		}
		if created == nil {
			return nil, errors.New(op + ": create and add pieces returned nil result")
		}
		submission.TransactionID = created.TxHash.Hex()
		submission.StatusURL = created.StatusURL
	}

	validated, err := c.validateCommitSubmission(op, ref, submission, false)
	if err != nil {
		return nil, err
	}
	if req.OnSubmitted != nil {
		req.OnSubmitted(validated.TransactionID)
	}
	out := copyCommitSubmission(validated)
	return &out, nil
}

func validateCommitRequest(op string, req CommitRequest) ([]cid.Cid, error) {
	return validateCommitPieces(op, req.Pieces)
}

func validateCommitPieces(op string, pieces []PieceInput) ([]cid.Cid, error) {
	if len(pieces) == 0 {
		return nil, fmt.Errorf("%s: %w: no pieces provided", op, ErrInvalidArgument)
	}
	if err := validateAddPiecesBatch(op, len(pieces)); err != nil {
		return nil, err
	}
	pieceCIDs := make([]cid.Cid, len(pieces))
	for i, piece := range pieces {
		pieceCIDs[i] = piece.PieceCID
	}
	if err := validateCommitPieceCIDs(op, pieceCIDs); err != nil {
		return nil, err
	}
	return pieceCIDs, nil
}

func validateCommitPieceCIDs(op string, pieceCIDs []cid.Cid) error {
	seen := make(map[string]int, len(pieceCIDs))
	for i, pieceCID := range pieceCIDs {
		if !pieceCID.Defined() {
			return fmt.Errorf("%s: %w: undefined pieceCID at index %d", op, ErrInvalidArgument, i)
		}
		key := canonicalCommitPieceCIDKey(pieceCID)
		if first, ok := seen[key]; ok {
			return fmt.Errorf("%s: %w: duplicate pieceCID at indexes %d and %d", op, ErrInvalidArgument, first, i)
		}
		seen[key] = i
	}
	return nil
}

func decodeCreateAndAddIdentity(op string, extraData []byte, payer common.Address) (*types.BigInt, error) {
	outer, err := createAndAddArgs.Unpack(extraData)
	if err != nil || len(outer) != 2 {
		return nil, fmt.Errorf("%s: %w: invalid create-and-add extraData", op, ErrInvalidArgument)
	}
	createPayload, createOK := outer[0].([]byte)
	addPayload, addOK := outer[1].([]byte)
	if !createOK || !addOK {
		return nil, fmt.Errorf("%s: %w: invalid create-and-add extraData", op, ErrInvalidArgument)
	}
	createValues, err := createDataSetArgs.Unpack(createPayload)
	if err != nil || len(createValues) != 5 {
		return nil, fmt.Errorf("%s: %w: invalid create-dataset extraData", op, ErrInvalidArgument)
	}
	if _, err := addPiecesArgs.Unpack(addPayload); err != nil {
		return nil, fmt.Errorf("%s: %w: invalid add-pieces extraData", op, ErrInvalidArgument)
	}
	signedPayer, ok := createValues[0].(common.Address)
	if !ok || signedPayer != payer {
		return nil, fmt.Errorf("%s: %w: create-and-add payer does not match context identity", op, ErrInvalidArgument)
	}
	rawClientDataSetID, ok := createValues[1].(*big.Int)
	if !ok || rawClientDataSetID == nil {
		return nil, fmt.Errorf("%s: %w: invalid clientDataSetID in extraData", op, ErrInvalidArgument)
	}
	clientDataSetID, err := types.BigIntFromBig(rawClientDataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: invalid clientDataSetID in extraData", op, ErrInvalidArgument)
	}
	return copyBigIntPtr(&clientDataSetID), nil
}

func (c *contextCore) getCommitStatus(
	ctx context.Context,
	op string,
	ref *DataSetRef,
	submission CommitSubmission,
) (*CommitStatus, error) {
	validated, err := c.validateCommitSubmission(op, ref, submission, true)
	if err != nil {
		return nil, err
	}
	return c.getValidatedCommitStatus(ctx, op, ref, validated)
}

func (c *contextCore) getValidatedCommitStatus(
	ctx context.Context,
	op string,
	ref *DataSetRef,
	submission CommitSubmission,
) (*CommitStatus, error) {
	if ref != nil {
		return c.getAddPiecesCommitStatus(ctx, op, submission)
	}
	return c.getCreateAndAddCommitStatus(ctx, op, submission)
}

func (c *contextCore) getAddPiecesCommitStatus(
	ctx context.Context,
	op string,
	submission CommitSubmission,
) (*CommitStatus, error) {
	snapshot, err := c.client.GetAddPiecesStatus(ctx, submission.StatusURL)
	rejected := errors.Is(err, pdp.ErrTxRejected)
	if err != nil && !rejected {
		return nil, fmt.Errorf("%s: get add-pieces status: %w", op, err)
	}
	if snapshot == nil {
		return nil, invalidCommitStatusf(op, "nil add-pieces status")
	}
	if err := validateAddCommitSnapshot(op, submission, snapshot, rejected); err != nil {
		return nil, err
	}

	state := CommitStatePending
	if rejected {
		state = CommitStateRejected
	} else if snapshot.PiecesAdded {
		state = CommitStateConfirmed
	}
	status := &CommitStatus{
		Kind:                   submission.Kind,
		State:                  state,
		TransactionID:          submission.TransactionID,
		ConfirmedTransactionID: optionalHashString(snapshot.ConfirmedTxHash),
		DataSet:                copyDataSetRefPtr(submission.DataSet),
	}
	if state == CommitStateConfirmed {
		status.PieceIDs = copyBigInts(snapshot.ConfirmedPieceIDs)
	}
	return status, nil
}

func (c *contextCore) getCreateAndAddCommitStatus(
	ctx context.Context,
	op string,
	submission CommitSubmission,
) (*CommitStatus, error) {
	snapshot, err := c.client.GetCreateDataSetAndAddPiecesStatus(ctx, submission.StatusURL)
	rejected := errors.Is(err, pdp.ErrTxRejected)
	if err != nil && !rejected {
		return nil, fmt.Errorf("%s: get create-and-add status: %w", op, err)
	}
	if snapshot == nil || snapshot.Create == nil {
		return nil, invalidCommitStatusf(op, "nil create status")
	}
	if snapshot.Create.CreateMessageHash != common.HexToHash(submission.TransactionID) {
		return nil, invalidCommitStatusf(op, "create transactionID does not match submission")
	}

	confirmedHash := snapshot.Create.ConfirmedTxHash
	if snapshot.Add != nil {
		if snapshot.Add.TxHash != common.HexToHash(submission.TransactionID) {
			return nil, invalidCommitStatusf(op, "add transactionID does not match submission")
		}
		if snapshot.Create.DataSetID == nil || !snapshot.Add.DataSetID.Equal(*snapshot.Create.DataSetID) {
			return nil, invalidCommitStatusf(op, "create and add dataSetIds differ")
		}
		if confirmedHash != (common.Hash{}) && snapshot.Add.ConfirmedTxHash != (common.Hash{}) && confirmedHash != snapshot.Add.ConfirmedTxHash {
			return nil, invalidCommitStatusf(op, "create and add confirmed transactionIDs differ")
		}
		if snapshot.Add.ConfirmedTxHash != (common.Hash{}) {
			confirmedHash = snapshot.Add.ConfirmedTxHash
		}
		if err := validateCommitPieceCount(op, snapshot.Add, len(submission.PieceCIDs), rejected); err != nil {
			return nil, err
		}
	}

	state := CommitStatePending
	if rejected {
		state = CommitStateRejected
	} else if snapshot.Add != nil && snapshot.Add.PiecesAdded {
		state = CommitStateConfirmed
	}
	status := &CommitStatus{
		Kind:                   submission.Kind,
		State:                  state,
		TransactionID:          submission.TransactionID,
		ConfirmedTransactionID: optionalHashString(confirmedHash),
	}
	if state == CommitStateConfirmed {
		dataSet, err := NewDataSetRef(
			submission.ProviderID,
			*snapshot.Create.DataSetID,
			*submission.ClientDataSetID,
		)
		if err != nil {
			return nil, invalidCommitStatusf(op, "invalid confirmed data-set identity")
		}
		status.DataSet = &dataSet
		status.PieceIDs = copyBigInts(snapshot.Add.ConfirmedPieceIDs)
	}
	return status, nil
}

func validateAddCommitSnapshot(op string, submission CommitSubmission, snapshot *pdp.AddPiecesStatus, rejected bool) error {
	if snapshot.TxHash != common.HexToHash(submission.TransactionID) {
		return invalidCommitStatusf(op, "transactionID does not match submission")
	}
	if !snapshot.DataSetID.Equal(submission.DataSet.DataSetID()) {
		return invalidCommitStatusf(op, "dataSetID does not match submission")
	}
	return validateCommitPieceCount(op, snapshot, len(submission.PieceCIDs), rejected)
}

func validateCommitPieceCount(op string, snapshot *pdp.AddPiecesStatus, expected int, rejected bool) error {
	if snapshot.PiecesAdded {
		if snapshot.PieceCount != expected {
			return invalidCommitStatusf(op, "pieceCount %d does not match submission count %d", snapshot.PieceCount, expected)
		}
	} else if snapshot.PieceCount != 0 && snapshot.PieceCount != expected {
		state := "pending"
		if rejected {
			state = "rejected"
		}
		return invalidCommitStatusf(op, "%s pieceCount %d does not match submission count %d", state, snapshot.PieceCount, expected)
	}
	if snapshot.PiecesAdded && len(snapshot.ConfirmedPieceIDs) != expected {
		return invalidCommitStatusf(op, "confirmed piece ID count %d does not match submission count %d", len(snapshot.ConfirmedPieceIDs), expected)
	}
	return nil
}

func (c *contextCore) waitForCommit(
	ctx context.Context,
	op string,
	ref *DataSetRef,
	submission CommitSubmission,
) (*CommitResult, error) {
	validated, err := c.validateCommitSubmission(op, ref, submission, true)
	if err != nil {
		return nil, err
	}
	for {
		status, err := c.getValidatedCommitStatus(ctx, op, ref, validated)
		if err != nil {
			return nil, err
		}
		switch status.State {
		case CommitStateConfirmed:
			if status.DataSet == nil {
				return nil, invalidCommitStatusf(op, "confirmed commit is missing data-set identity")
			}
			return &CommitResult{
				TransactionID:          status.TransactionID,
				ConfirmedTransactionID: status.ConfirmedTransactionID,
				DataSet:                copyDataSetRef(*status.DataSet),
				PieceIDs:               copyBigInts(status.PieceIDs),
				IsNewDataSet:           status.Kind == CommitKindCreateAndAdd,
			}, nil
		case CommitStateRejected:
			return nil, newCommitRejectedError(validated, *status)
		case CommitStatePending:
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(commitPollInterval):
			}
		default:
			return nil, invalidCommitStatusf(op, "unknown commit state %q", status.State)
		}
	}
}

func (c *contextCore) validateCommitSubmission(
	op string,
	ref *DataSetRef,
	submission CommitSubmission,
	callerSupplied bool,
) (CommitSubmission, error) {
	submission = copyCommitSubmission(submission)
	invalid := func(format string, args ...any) (CommitSubmission, error) {
		message := fmt.Sprintf(format, args...)
		if callerSupplied {
			return CommitSubmission{}, fmt.Errorf("%s: %w: %s", op, ErrInvalidArgument, message)
		}
		return CommitSubmission{}, fmt.Errorf("%s: invalid provider submission: %s", op, message)
	}

	expectedKind := CommitKindCreateAndAdd
	if ref != nil {
		expectedKind = CommitKindAddPieces
	}
	if submission.Kind != expectedKind {
		return invalid("submission kind %q does not match context", submission.Kind)
	}
	if submission.ProviderID.IsZero() || !submission.ProviderID.Equal(c.provider.ID) {
		return invalid("submission provider does not match context")
	}
	if !common.IsHexHash(submission.TransactionID) || common.HexToHash(submission.TransactionID) == (common.Hash{}) {
		return invalid("invalid transactionID")
	}
	submission.TransactionID = common.HexToHash(submission.TransactionID).Hex()
	if err := validateProviderStatusURL(c.provider.ServiceURL, submission.StatusURL); err != nil {
		if callerSupplied {
			return CommitSubmission{}, fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
		}
		return CommitSubmission{}, fmt.Errorf("%s: %w", op, err)
	}
	if len(submission.PieceCIDs) == 0 || len(submission.PieceCIDs) > pdp.MaxAddPiecesBatchSize {
		return invalid("pieceCIDs count must be between 1 and %d", pdp.MaxAddPiecesBatchSize)
	}
	if err := validateCommitPieceCIDs(op, submission.PieceCIDs); err != nil {
		return CommitSubmission{}, err
	}

	if ref != nil {
		if submission.DataSet == nil || !submission.DataSet.valid() || !submission.DataSet.Equal(*ref) {
			return invalid("submission data set does not match context")
		}
		if submission.ClientDataSetID != nil {
			return invalid("add-pieces submission must not contain clientDataSetID")
		}
		if submission.Identity != c.identity() {
			return invalid("submission identity does not match context")
		}
	} else {
		if submission.DataSet != nil {
			return invalid("create-and-add submission must not contain a data set")
		}
		if submission.ClientDataSetID == nil {
			return invalid("create-and-add submission is missing clientDataSetID")
		}
		if !identityComplete(submission.Identity) || submission.Identity != c.identity() {
			return invalid("submission identity does not match complete context identity")
		}
	}
	return submission, nil
}

func canonicalCommitPieceCIDKey(pieceCID cid.Cid) string {
	if info, err := piece.ParseV2(pieceCID); err == nil {
		return info.CIDv1.KeyString()
	}
	return pieceCID.KeyString()
}

func identityComplete(identity ContextIdentity) bool {
	return identity.Payer != (common.Address{}) &&
		identity.ChainID.IsValid() &&
		identity.RecordKeeper != (common.Address{})
}

func validateProviderStatusURL(serviceURL, statusURL string) error {
	base, baseErr := url.Parse(serviceURL)
	status, statusErr := url.Parse(statusURL)
	if baseErr != nil || statusErr != nil ||
		base == nil || status == nil ||
		!base.IsAbs() || !status.IsAbs() ||
		!validStatusScheme(base.Scheme) || !validStatusScheme(status.Scheme) ||
		base.Hostname() == "" || status.Hostname() == "" ||
		!strings.EqualFold(base.Scheme, status.Scheme) ||
		!strings.EqualFold(base.Hostname(), status.Hostname()) ||
		effectiveStatusPort(base) != effectiveStatusPort(status) {
		return fmt.Errorf("%w: provider status URL origin does not match service URL", pdp.ErrStatusURLOrigin)
	}
	return nil
}

func validStatusScheme(scheme string) bool {
	return strings.EqualFold(scheme, "http") || strings.EqualFold(scheme, "https")
}

func effectiveStatusPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "http") {
		return "80"
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return ""
}

func invalidCommitStatusf(op, format string, args ...any) error {
	return fmt.Errorf("%s: %w: %s", op, pdp.ErrInvalidStatus, fmt.Sprintf(format, args...))
}

func optionalHashString(hash common.Hash) string {
	if hash == (common.Hash{}) {
		return ""
	}
	return hash.Hex()
}

func copyCommitSubmission(in CommitSubmission) CommitSubmission {
	out := in
	out.ProviderID = copyBigInt(in.ProviderID)
	out.DataSet = copyDataSetRefPtr(in.DataSet)
	out.ClientDataSetID = copyBigIntPtr(in.ClientDataSetID)
	out.PieceCIDs = append([]cid.Cid(nil), in.PieceCIDs...)
	return out
}

func copyCommitStatus(in CommitStatus) CommitStatus {
	out := in
	out.DataSet = copyDataSetRefPtr(in.DataSet)
	out.PieceIDs = copyBigInts(in.PieceIDs)
	return out
}

func copyDataSetRefPtr(in *DataSetRef) *DataSetRef {
	if in == nil {
		return nil
	}
	out := copyDataSetRef(*in)
	return &out
}

func copyBigInts(in []types.BigInt) []types.BigInt {
	if len(in) == 0 {
		return nil
	}
	out := make([]types.BigInt, len(in))
	for i := range in {
		out[i] = copyBigInt(in[i])
	}
	return out
}

func newCommitRejectedError(submission CommitSubmission, status CommitStatus) *CommitRejectedError {
	return &CommitRejectedError{
		Submission: copyCommitSubmission(submission),
		Status:     copyCommitStatus(status),
	}
}
