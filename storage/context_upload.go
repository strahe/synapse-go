package storage

import (
	"context"
	"fmt"
	"io"
)

// Upload stores a single copy of data on this context's provider and
// commits it on-chain. It is Store + Commit — no fan-out, no Pull —
// and returns the canonical UploadResult shape used elsewhere in the SDK.
//
// opts may be nil. PieceCID, OnProgress, PieceMetadata, OnStored,
// OnPiecesAdded, and OnPiecesConfirmed are honoured when present; other
// UploadOptions fields related to provider selection are ignored because this
// path does not touch provider selection.
//
// Lifecycle callbacks fired (when opts provides them):
//   - OnStored after Store succeeds
//   - OnPiecesAdded when the commit transaction is submitted
//   - OnPiecesConfirmed after commit is confirmed
func (c *Context) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	if r == nil {
		return nil, fmt.Errorf("storage.Context.Upload: %w: nil reader", ErrInvalidArgument)
	}
	opts = newUploadCallbackGuard(c.logger).wrapUploadOptions(opts)

	storeOpts := &StoreOptions{}
	if opts != nil {
		storeOpts.PieceCID = opts.PieceCID
		storeOpts.OnProgress = opts.OnProgress
	}
	storeResult, err := c.Store(ctx, r, storeOpts)
	if err != nil {
		return nil, &StoreError{
			ProviderID: c.ProviderID(),
			Endpoint:   c.ServiceURL(),
			Cause:      err,
		}
	}

	if opts != nil && opts.OnStored != nil {
		opts.OnStored(c.ProviderID(), storeResult.PieceCID)
	}

	pieceInputs := []PieceInput{{
		PieceCID:      storeResult.PieceCID,
		PieceMetadata: cloneMetadata(opts),
	}}

	var onSubmitted func(string)
	if opts != nil && opts.OnPiecesAdded != nil {
		pieceCID := storeResult.PieceCID
		providerID := c.ProviderID()
		onSubmitted = func(txHash string) {
			opts.OnPiecesAdded(txHash, providerID, []SubmittedPiece{{PieceCID: pieceCID}})
		}
	}

	commit, err := c.Commit(ctx, CommitRequest{Pieces: pieceInputs, OnSubmitted: onSubmitted})
	if err != nil {
		return nil, &CommitError{
			ProviderID: c.ProviderID(),
			Endpoint:   c.ServiceURL(),
			Cause:      err,
		}
	}

	if len(commit.PieceIDs) == 0 {
		return nil, fmt.Errorf("storage.Context.Upload: commit returned no piece IDs")
	}

	if opts != nil && opts.OnPiecesConfirmed != nil {
		confirmed := make([]ConfirmedPiece, len(commit.PieceIDs))
		for i, id := range commit.PieceIDs {
			confirmed[i] = ConfirmedPiece{PieceID: id, PieceCID: storeResult.PieceCID}
		}
		opts.OnPiecesConfirmed(commit.DataSetID, c.ProviderID(), confirmed)
	}

	copies := []CopyResult{{
		ProviderID:   c.ProviderID(),
		DataSetID:    commit.DataSetID,
		PieceID:      commit.PieceIDs[0],
		Role:         CopyRolePrimary,
		RetrievalURL: c.PieceURL(storeResult.PieceCID),
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
