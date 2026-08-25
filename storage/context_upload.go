package storage

import (
	"context"
	"fmt"
	"io"
)

// Upload stores a single copy and commits it to a new data set.
func (c *ProviderContext) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	return c.core.upload(ctx, "storage.ProviderContext.Upload", nil, r, opts)
}

// Upload stores a single copy and commits it to the bound data set.
func (c *DataSetContext) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	return c.core.upload(ctx, "storage.DataSetContext.Upload", &c.ref, r, opts)
}

// upload stores a single copy of data on this context's provider and
// commits it on-chain. It is Store + Commit — no fan-out, no Pull —
// and returns the canonical UploadResult shape used elsewhere in the SDK.
//
// opts may be nil. PieceCID, OnProgress, PieceMetadata, OnStored,
// OnPiecesAdded, and OnPiecesConfirmed are honoured when present; other
// UploadOptions fields related to provider selection are ignored because this
// path does not touch provider selection.
//
// Lifecycle callbacks fired (when opts provides them):
//   - OnProgress during the store upload stream
//   - OnStored after Store succeeds
//   - OnPiecesAdded when the commit transaction is submitted
//   - OnPiecesConfirmed after commit is confirmed
func (c *contextCore) upload(ctx context.Context, op string, ref *DataSetRef, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	if r == nil {
		return nil, fmt.Errorf("%s: %w: nil reader", op, ErrInvalidArgument)
	}
	opts = newUploadCallbackGuard(c.logger).wrapUploadOptions(opts)

	if err := c.validateWritableDataSet(ctx, op, ref); err != nil {
		return nil, err
	}

	storeOpts := &StoreOptions{}
	if opts != nil {
		storeOpts.PieceCID = opts.PieceCID
		storeOpts.OnProgress = opts.OnProgress
	}
	storeResult, err := c.store(ctx, op, r, storeOpts)
	if err != nil {
		return nil, &StoreError{
			ProviderID: copyBigInt(c.provider.ID),
			Endpoint:   c.provider.ServiceURL,
			Cause:      err,
		}
	}

	if opts != nil && opts.OnStored != nil {
		opts.OnStored(copyBigInt(c.provider.ID), storeResult.PieceCID)
	}

	pieceInputs := []PieceInput{{
		PieceCID:      storeResult.PieceCID,
		PieceMetadata: cloneMetadata(opts),
	}}

	var onSubmitted func(string)
	if opts != nil && opts.OnPiecesAdded != nil {
		pieceCID := storeResult.PieceCID
		providerID := copyBigInt(c.provider.ID)
		onSubmitted = func(txHash string) {
			opts.OnPiecesAdded(txHash, providerID, []SubmittedPiece{{PieceCID: pieceCID}})
		}
	}

	commit, err := c.commit(ctx, op, ref, CommitRequest{Pieces: pieceInputs, OnSubmitted: onSubmitted})
	if err != nil {
		return nil, &CommitError{
			ProviderID: copyBigInt(c.provider.ID),
			Endpoint:   c.provider.ServiceURL,
			Cause:      err,
		}
	}

	if len(commit.PieceIDs) == 0 {
		return nil, fmt.Errorf("%s: commit returned no piece IDs", op)
	}

	if opts != nil && opts.OnPiecesConfirmed != nil {
		confirmed := make([]ConfirmedPiece, len(commit.PieceIDs))
		for i, id := range commit.PieceIDs {
			confirmed[i] = ConfirmedPiece{PieceID: id, PieceCID: storeResult.PieceCID}
		}
		opts.OnPiecesConfirmed(commit.DataSetID, copyBigInt(c.provider.ID), confirmed)
	}

	copies := []CopyResult{{
		ProviderID:   copyBigInt(c.provider.ID),
		DataSetID:    commit.DataSetID,
		PieceID:      commit.PieceIDs[0],
		Role:         CopyRolePrimary,
		RetrievalURL: c.pieceURLFor(storeResult.PieceCID),
		IsNewDataSet: commit.IsNewDataSet,
	}}

	return &UploadResult{
		PieceCID:        storeResult.PieceCID,
		Size:            storeResult.Size,
		RequestedCopies: 1,
		Complete:        true,
		Copies:          copies,
	}, nil
}
