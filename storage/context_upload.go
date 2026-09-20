package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/strahe/synapse-go/internal/redact"
)

// Upload stores a single copy and commits it to a new data set. opts may be
// nil. With batching, it commits to the data set the batcher shares for this
// provider, data-set metadata, and CDN setting. Successful admission transfers
// ownership to the batcher; later caller cancellation stops waiting but not the
// commit.
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
	ctx, guard := newUploadCallbackGuard(ctx, op, c.logger)
	defer guard.rethrow()
	uploadOpts := guard.wrapUploadOptions(uploadOptionsFromContext(opts))
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
	var transfer *uploadBatchTransfer
	if reservation != nil {
		var err error
		transfer, err = reservation.beginTransfer(target)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
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
			Endpoint:   redact.URLString(c.provider.ServiceURL),
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

	var onBatchSubmitted func(string)
	var onCommitSubmitted func(CommitSubmission)
	if uploadOpts != nil && uploadOpts.OnPiecesAdded != nil {
		pieceCID := storeResult.PieceCID
		providerID := copyBigInt(c.provider.ID)
		onBatchSubmitted = func(txHash string) {
			uploadOpts.OnPiecesAdded(txHash, providerID, []SubmittedPiece{{PieceCID: pieceCID}})
		}
		onCommitSubmitted = func(submission CommitSubmission) {
			uploadOpts.OnPiecesAdded(submission.TransactionID, submission.ProviderID, []SubmittedPiece{{PieceCID: pieceCID}})
		}
	}

	var (
		commit     *CommitResult
		submission *CommitSubmission
	)
	batched := c.uploadBatcher != nil
	if !batched {
		submission, err = c.submitCommit(ctx, op, ref, commitRequest{CommitRequest: CommitRequest{
			Pieces:      pieceInputs,
			OnSubmitted: onCommitSubmitted,
		}})
		if err == nil {
			commit, err = c.waitForCommit(ctx, op, ref, *submission)
		}
	} else {
		task, enqueueErr := c.uploadBatcher.enqueue(ctx, reservation.seq, target, pieceInputs[0], transfer)
		reservation.release()
		if enqueueErr != nil {
			err = enqueueErr
		} else {
			commit, submission, err = task.wait(ctx, onBatchSubmitted)
		}
	}
	if err != nil {
		err = uploadBatchContextError(ctx, err)
		return nil, &CommitError{
			ProviderID: copyBigInt(c.provider.ID),
			Endpoint:   redact.URLString(c.provider.ServiceURL),
			Cause:      err,
			PieceCID:   storeResult.PieceCID,
			Size:       storeResult.Size,
			FailedAttempts: []FailedAttempt{{
				ProviderID: copyBigInt(c.provider.ID),
				Role:       CopyRolePrimary,
				Stage:      CopyStageCommit,
				Err:        err,
				Explicit:   true,
				Submission: submission,
			}},
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
