package storage

import (
	"context"
	"fmt"
	"io"
)

// Upload stores a single copy and commits it to a new data set. opts may be
// nil. When batching is configured, successful admission transfers ownership
// to the batcher; later caller cancellation stops waiting but not the commit.
func (c *ProviderContext) Upload(ctx context.Context, r io.Reader, opts *ContextUploadOptions) (*UploadResult, error) {
	return c.core.upload(ctx, "storage.ProviderContext.Upload", c, nil, r, opts)
}

// Upload stores a single copy and commits it to the bound data set. opts may be
// nil. When batching is configured, successful admission transfers ownership
// to the batcher; later caller cancellation stops waiting but not the commit.
func (c *DataSetContext) Upload(ctx context.Context, r io.Reader, opts *ContextUploadOptions) (*UploadResult, error) {
	return c.core.upload(ctx, "storage.DataSetContext.Upload", c, &c.ref, r, opts)
}

// upload stores a single copy of data on this context's provider and
// commits it on-chain. It is Store + Commit — no fan-out, no Pull —
// and returns the canonical UploadResult shape used elsewhere in the SDK.
//
// opts may be nil. PieceCID, OnProgress, PieceMetadata, OnStored,
// OnPiecesAdded, and OnPiecesConfirmed are honoured when present.
//
// Lifecycle callbacks fired (when opts provides them):
//   - OnProgress during the store upload stream
//   - OnStored after Store succeeds
//   - OnPiecesAdded when the commit transaction is submitted
//   - OnPiecesConfirmed after commit is confirmed
func (c *contextCore) upload(ctx context.Context, op string, target StorageContext, ref *DataSetRef, r io.Reader, opts *ContextUploadOptions) (*UploadResult, error) {
	if r == nil {
		return nil, fmt.Errorf("%s: %w: nil reader", op, ErrInvalidArgument)
	}
	uploadOpts := newUploadCallbackGuard(c.logger).wrapUploadOptions(uploadOptionsFromContext(opts))
	var reservation *uploadReservation
	if c.uploadBatcher != nil {
		if err := c.uploadBatcher.validateTarget(target); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		var err error
		reservation, err = c.uploadBatcher.reserve()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		defer reservation.release()
		var releaseContext func()
		ctx, releaseContext = c.uploadBatcher.bindContext(ctx)
		defer releaseContext()
	}

	if err := c.validateWritableDataSet(ctx, op, ref); err != nil {
		return nil, uploadBatchContextError(ctx, err)
	}

	storeOpts := &StoreOptions{}
	if uploadOpts != nil {
		storeOpts.PieceCID = uploadOpts.PieceCID
		storeOpts.OnProgress = uploadOpts.OnProgress
	}
	storeResult, err := c.store(ctx, op, r, storeOpts)
	if err != nil {
		return nil, &StoreError{
			ProviderID: copyBigInt(c.provider.ID),
			Endpoint:   c.provider.ServiceURL,
			Cause:      uploadBatchContextError(ctx, err),
		}
	}

	if uploadOpts != nil && uploadOpts.OnStored != nil {
		uploadOpts.OnStored(copyBigInt(c.provider.ID), storeResult.PieceCID)
	}

	pieceInputs := []PieceInput{{
		PieceCID:      storeResult.PieceCID,
		PieceMetadata: cloneMetadata(uploadOpts),
	}}

	var onSubmitted func(string)
	if uploadOpts != nil && uploadOpts.OnPiecesAdded != nil {
		pieceCID := storeResult.PieceCID
		providerID := copyBigInt(c.provider.ID)
		onSubmitted = func(txHash string) {
			uploadOpts.OnPiecesAdded(txHash, providerID, []SubmittedPiece{{PieceCID: pieceCID}})
		}
	}

	var commit *CommitResult
	batched := c.uploadBatcher != nil
	if !batched {
		commit, err = c.commit(ctx, op, ref, CommitRequest{Pieces: pieceInputs, OnSubmitted: onSubmitted})
	} else {
		task, enqueueErr := c.uploadBatcher.enqueue(ctx, reservation.seq, target, pieceInputs[0])
		reservation.release()
		if enqueueErr != nil {
			err = enqueueErr
		} else {
			commit, err = task.wait(ctx, onSubmitted)
		}
	}
	if err != nil {
		return nil, &CommitError{
			ProviderID: copyBigInt(c.provider.ID),
			Endpoint:   c.provider.ServiceURL,
			Cause:      uploadBatchContextError(ctx, err),
		}
	}

	if commit == nil {
		return nil, fmt.Errorf("%s: commit returned nil result", op)
	}
	if len(commit.PieceIDs) == 0 {
		return nil, fmt.Errorf("%s: commit returned no piece IDs", op)
	}

	if uploadOpts != nil && uploadOpts.OnPiecesConfirmed != nil && ctx.Err() == nil {
		confirmed := make([]ConfirmedPiece, len(commit.PieceIDs))
		for i, id := range commit.PieceIDs {
			confirmed[i] = ConfirmedPiece{PieceID: id, PieceCID: storeResult.PieceCID}
		}
		uploadOpts.OnPiecesConfirmed(commit.DataSet.DataSetID(), copyBigInt(c.provider.ID), confirmed)
	}

	copies := []CopyResult{{
		ProviderID:   copyBigInt(c.provider.ID),
		DataSetID:    commit.DataSet.DataSetID(),
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
