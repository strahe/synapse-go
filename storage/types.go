package storage

import (
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
)

// CopyRole identifies the role a provider plays in a multi-copy upload.
type CopyRole string

const (
	// CopyRolePrimary is the provider that received the original store.
	CopyRolePrimary CopyRole = "primary"
	// CopyRoleSecondary is a provider that pulled data from the primary.
	CopyRoleSecondary CopyRole = "secondary"
)

// CopyStage identifies the pipeline stage at which a provider attempt failed.
type CopyStage string

const (
	CopyStageStore   CopyStage = "store"
	CopyStagePull    CopyStage = "pull"
	CopyStagePresign CopyStage = "presign"
	CopyStageCommit  CopyStage = "commit"
)

// PullStatus is the per-piece or overall status returned by a pull operation.
type PullStatus string

const (
	PullStatusPending    PullStatus = "pending"
	PullStatusInProgress PullStatus = "inProgress"
	PullStatusRetrying   PullStatus = "retrying"
	PullStatusComplete   PullStatus = "complete"
	PullStatusFailed     PullStatus = "failed"
)

// StoreOptions configures a single Context.Store call.
type StoreOptions struct {
	// PieceCID, when defined, is a pre-computed PieceCIDv2 of the payload.
	// When set, the client skips inline commP calculation; the server still
	// verifies the uploaded bytes match this value.
	PieceCID cid.Cid
	// OnProgress is invoked after each non-empty Read from the reader, with
	// the cumulative bytes sent so far. It may be nil. Direct Store calls do
	// not recover callback panics.
	OnProgress func(bytesUploaded int64)
}

// StoreResult is returned by a successful Store call.
type StoreResult struct {
	PieceCID cid.Cid // PieceCIDv2 of the stored data
	Size     int64   // raw (unpadded) byte count
}

// SubmittedPiece carries the piece identity reported by an OnPiecesAdded callback.
type SubmittedPiece struct {
	PieceCID cid.Cid
}

// ConfirmedPiece carries the on-chain identity reported by an OnPiecesConfirmed callback.
type ConfirmedPiece struct {
	PieceID  types.BigInt
	PieceCID cid.Cid
}

// PieceInput describes a single piece being committed on-chain.
type PieceInput struct {
	PieceCID      cid.Cid
	PieceMetadata map[string]string // optional key-value metadata stored with the piece
}

// PullRequest asks a secondary provider to pull pieces from a primary.
type PullRequest struct {
	Pieces    []cid.Cid
	From      func(cid.Cid) string // returns the HTTPS URL for a given piece CID
	ExtraData []byte               // EIP-712 signed payload authorising the pull
	// OnProgress is invoked after each piece status update during the pull.
	// It may be nil. Direct Pull calls do not recover callback panics.
	OnProgress func(pieceCID cid.Cid, status PullStatus)
}

// PullPieceResult is the per-piece outcome within a PullResult.
type PullPieceResult struct {
	PieceCID cid.Cid
	Status   PullStatus
}

// PullResult is the aggregate outcome of a pull operation.
type PullResult struct {
	Status PullStatus
	Pieces []PullPieceResult
}

// CommitRequest triggers on-chain registration of pieces for one provider.
type CommitRequest struct {
	Pieces    []PieceInput
	ExtraData []byte // EIP-712 signed payload; nil for the primary (create-or-add path)
	// OnSubmitted is invoked with the transaction hash immediately after the
	// on-chain AddPieces transaction is submitted, before confirmation. It may
	// be nil. Direct Commit calls do not recover callback panics.
	OnSubmitted func(txHash string)
}

// CommitResult is returned by a successful Commit call.
type CommitResult struct {
	TransactionID string       // on-chain transaction hash
	DataSetID     types.BigInt // data set that now holds the piece
	PieceIDs      []types.BigInt
	IsNewDataSet  bool // true when a new data set was created by this commit
}

// CreateDataSetOptions configures [Context.CreateDataSet].
type CreateDataSetOptions struct {
	// OnSubmitted is invoked after the create transaction is submitted and
	// before waiting for confirmation. It may be nil.
	OnSubmitted func(CreateDataSetSubmission)
}

// CreateDataSetSubmission identifies a submitted create-dataset transaction.
// Persist and restore all fields together; incomplete submissions are rejected.
type CreateDataSetSubmission struct {
	TransactionID string
	StatusURL     string
	// ClientDataSetID must be non-nil when resuming a submitted create.
	ClientDataSetID *types.BigInt
}

// CreateDataSetResult is returned after standalone dataset creation confirms.
type CreateDataSetResult struct {
	TransactionID   string
	DataSetID       types.BigInt
	ClientDataSetID types.BigInt
}

// CopyResult describes one successfully committed copy.
type CopyResult struct {
	ProviderID   types.BigInt
	DataSetID    types.BigInt
	PieceID      types.BigInt
	Role         CopyRole
	RetrievalURL string // HTTPS retrieval URL for this piece on the provider.
	IsNewDataSet bool
}

// FailedAttempt records a provider attempt that did not produce a copy.
type FailedAttempt struct {
	ProviderID types.BigInt
	Role       CopyRole
	Stage      CopyStage // pipeline stage where the failure occurred
	Err        error
	Explicit   bool // true when the provider was caller-specified (no auto-retry)
}

// UploadResult is returned by a successful Upload call.
//
// Use Complete to determine overall success: it is true when every requested
// copy was committed on-chain (equivalent to len(Copies) >= RequestedCopies).
// A non-empty FailedAttempts slice does NOT indicate overall failure — failed
// attempts may have been resolved by successful retries on other providers.
//
// Example:
//
//	result, err := m.Upload(ctx, r, opts)
//	if err != nil { ... }
//	if !result.Complete {
//	    log.Printf("partial upload: %d/%d copies", result.SuccessCount(), result.RequestedCopies)
//	}
type UploadResult struct {
	PieceCID        cid.Cid // PieceCIDv2 of the stored data
	Size            int64   // raw (unpadded) byte count
	RequestedCopies int
	// Complete is true when all RequestedCopies were committed on-chain.
	// Equivalent to len(Copies) >= RequestedCopies.
	Complete       bool
	Copies         []CopyResult
	FailedAttempts []FailedAttempt
}

// SuccessCount returns the number of copies that were successfully committed
// on-chain. Equivalent to len(Copies).
func (r *UploadResult) SuccessCount() int {
	if r == nil {
		return 0
	}
	return len(r.Copies)
}

// PrimaryDataSetID returns the DataSetID of the primary copy.
//
// ok is false when no primary copy committed on-chain (even if secondaries
// did). Callers that need precise provenance should inspect
// [UploadResult.Copies] directly.
func (r *UploadResult) PrimaryDataSetID() (types.BigInt, bool) {
	if r == nil {
		return types.BigInt{}, false
	}
	for i := range r.Copies {
		c := &r.Copies[i]
		if c.Role != CopyRolePrimary {
			continue
		}
		return c.DataSetID, true
	}
	return types.BigInt{}, false
}

// SuccessfulProviderIDs returns the ProviderID of every copy that committed
// on-chain, in the order the copies appear in [UploadResult.Copies].
func (r *UploadResult) SuccessfulProviderIDs() []types.BigInt {
	if r == nil || len(r.Copies) == 0 {
		return nil
	}
	out := make([]types.BigInt, 0, len(r.Copies))
	for i := range r.Copies {
		out = append(out, r.Copies[i].ProviderID)
	}
	return out
}

// PartialSuccess reports whether at least one copy was committed on-chain but
// fewer than the requested number were obtained. Returns false when Complete is
// true or when no copies succeeded at all.
func (r *UploadResult) PartialSuccess() bool {
	if r == nil {
		return false
	}
	return !r.Complete && len(r.Copies) > 0
}

// UploadOptions configures an Upload call.
//
// Some lifecycle callbacks may be invoked from internal orchestration
// goroutines. Callers that share mutable state across callbacks must keep their
// handlers concurrency-safe. Service.Upload and Context.Upload recover and
// ignore callback panics; when a logger is configured, the first panic per
// callback name in an upload logs a warning. This recovery does not apply to
// direct StoreOptions, PullRequest, or CommitRequest hooks.
type UploadOptions struct {
	// Copies is the number of provider copies to store. Zero means the resolver
	// default: len(DataSetIDs) or len(ProviderIDs) when those are set, otherwise 2.
	Copies int
	// PieceMetadata is stored with each piece on-chain.
	PieceMetadata map[string]string
	// DataSetMetadata is stored with the data set on first creation.
	DataSetMetadata map[string]string
	// ProviderIDs pins the upload to specific providers by ID. Mutually
	// exclusive with DataSetIDs.
	ProviderIDs []types.BigInt
	// DataSetIDs pins the upload to specific existing data sets. Mutually
	// exclusive with ProviderIDs.
	DataSetIDs []types.BigInt
	// ExcludeProviderIDs skips these providers during auto-selection.
	ExcludeProviderIDs []types.BigInt
	// WithCDN is tri-state: nil inherits the Client-level default
	// configured via synapse.WithCDN; non-nil explicitly overrides
	// for this upload. Declare a local variable to take its address:
	//
	//	b := true
	//	opts := &storage.UploadOptions{WithCDN: &b}
	WithCDN *bool
	// PieceCID, when defined, is a pre-computed PieceCIDv2 of the payload.
	// When set, the primary provider client skips inline commP calculation;
	// the server still verifies the uploaded bytes match this value.
	PieceCID cid.Cid
	// OnProgress is invoked after each non-empty Read from the upload
	// reader, with the cumulative bytes sent to the primary provider so
	// far. It may be nil.
	OnProgress func(bytesUploaded int64)
	// OnStored is invoked once the primary provider has confirmed storage of
	// the piece. It may be nil.
	OnStored func(providerID types.BigInt, pieceCID cid.Cid)
	// OnPiecesAdded is invoked after the on-chain AddPieces transaction is
	// submitted for a provider (primary or secondary), carrying the transaction
	// hash and the batch of pieces included in that transaction. During
	// Service.Upload, different providers may invoke this callback
	// concurrently when commitConcurrency > 1. It may be nil.
	OnPiecesAdded func(txHash string, providerID types.BigInt, pieces []SubmittedPiece)
	// OnPiecesConfirmed is invoked after the on-chain AddPieces transaction is
	// confirmed (CommitResult received) for a provider, carrying the assigned
	// on-chain IDs for each piece. During Service.Upload, this callback is
	// invoked sequentially after all commit workers finish. It may be nil.
	OnPiecesConfirmed func(dataSetID, providerID types.BigInt, pieces []ConfirmedPiece)
	// OnCopyComplete is invoked once a secondary provider's SP-to-SP pull
	// completes successfully. It is not fired for the primary (which stores
	// directly). It may be nil.
	OnCopyComplete func(providerID types.BigInt, pieceCID cid.Cid)
	// OnCopyFailed is invoked when a secondary provider's SP-to-SP copy
	// attempt fails. Presign failures are not copy attempts and still surface
	// only through FailedAttempts with CopyStagePresign. Primary store/commit
	// failures likewise surface through the Upload return value and
	// FailedAttempts. It may be nil.
	OnCopyFailed func(providerID types.BigInt, pieceCID cid.Cid, err error)
	// OnPullProgress is invoked for each piece status update during a
	// secondary-provider pull. It may be nil.
	OnPullProgress func(providerID types.BigInt, pieceCID cid.Cid, status PullStatus)
}
