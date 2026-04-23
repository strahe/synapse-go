package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/strahe/synapse-go/types"
)

// Upload stores a single copy of data on this context's provider and
// commits it on-chain. Mirrors TS StorageContext.upload
// (synapse-sdk/.../storage/context.ts:915-954): it is Store + Commit —
// no fan-out, no Pull — and returns the canonical UploadResult shape
// used elsewhere in the SDK.
//
// opts may be nil. PieceCID / OnProgress / PieceMetadata are honoured when
// present; other UploadOptions fields are ignored
// because this path does not touch provider selection.
func (c *Context) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	if r == nil {
		return nil, fmt.Errorf("storage.Context.Upload: %w: nil reader", ErrInvalidArgument)
	}

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

	pieceInputs := []PieceInput{{
		PieceCID:      storeResult.PieceCID,
		PieceMetadata: cloneMetadata(opts),
	}}

	commit, err := c.Commit(ctx, CommitRequest{Pieces: pieceInputs})
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

// ensure types import is used even if future refactors drop it.
var _ = types.DataSetID(0)
