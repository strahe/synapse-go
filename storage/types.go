package storage

import (
	"github.com/ethereum/go-ethereum/common"
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

// StoreOptions configures a single provider or data-set context Store call.
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

// DataSetRef identifies one data set and the provider that owns it.
// ProviderID and DataSetID must be non-zero. ClientDataSetID is the
// caller-chosen uint256 used by EIP-712 authorization and may be zero.
type DataSetRef struct {
	providerID      types.BigInt
	dataSetID       types.BigInt
	clientDataSetID types.BigInt
}

// ContextIdentity identifies the account and chain configuration used by a
// storage context for signing and on-chain operations. Its JSON form uses
// strict lowerCamel field names.
type ContextIdentity struct {
	Payer        common.Address `json:"payer"`
	ChainID      types.ChainID  `json:"chainId"`
	RecordKeeper common.Address `json:"recordKeeper"`
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
	// OnSubmitted is invoked with the original transaction hash immediately
	// after the provider returns a valid submission handle, before confirmation.
	// It may be nil. Direct Commit calls do not recover callback panics.
	OnSubmitted func(txHash string)
}

// CommitKind identifies whether a submission creates a data set or adds to an
// existing one.
type CommitKind string

const (
	// CommitKindCreateAndAdd creates a new data set and adds pieces to it.
	CommitKindCreateAndAdd CommitKind = "create-and-add"
	// CommitKindAddPieces adds pieces to an existing data set.
	CommitKindAddPieces CommitKind = "add-pieces"
)

// CommitState is the durable, provider-independent state of a submission.
type CommitState string

const (
	// CommitStatePending means the provider has not reported a terminal result.
	CommitStatePending CommitState = "pending"
	// CommitStateConfirmed means all submitted pieces were added successfully.
	CommitStateConfirmed CommitState = "confirmed"
	// CommitStateRejected means the transaction failed or was rejected.
	CommitStateRejected CommitState = "rejected"
)

// CommitSubmission is a persistable handle returned after one successful
// provider submission. Persist all fields together and resume it with the same
// concrete context type. Its JSON form uses strict lowerCamel field names and
// rejects alternate capitalization.
type CommitSubmission struct {
	Kind            CommitKind      `json:"kind"`
	TransactionID   string          `json:"transactionId"`
	StatusURL       string          `json:"statusUrl"`
	ProviderID      types.BigInt    `json:"providerId"`
	Identity        ContextIdentity `json:"identity"`
	DataSet         *DataSetRef     `json:"dataSet"`
	ClientDataSetID *types.BigInt   `json:"clientDataSetId"`
	PieceCIDs       []cid.Cid       `json:"pieceCids"`
}

// CommitStatus is one logical status snapshot for a submission.
type CommitStatus struct {
	Kind                   CommitKind     `json:"kind"`
	State                  CommitState    `json:"state"`
	TransactionID          string         `json:"transactionId"`
	ConfirmedTransactionID string         `json:"confirmedTransactionId"`
	DataSet                *DataSetRef    `json:"dataSet"`
	PieceIDs               []types.BigInt `json:"pieceIds"`
}

// CommitResult is returned by a successful Commit call.
type CommitResult struct {
	TransactionID          string         `json:"transactionId"`          // transaction hash from the provider submission
	ConfirmedTransactionID string         `json:"confirmedTransactionId"` // actual on-chain hash, when reported by the provider
	DataSet                DataSetRef     `json:"dataSet"`
	PieceIDs               []types.BigInt `json:"pieceIds"`
	IsNewDataSet           bool           `json:"isNewDataSet"` // true when a new data set was created by this commit
}

// CreateDataSetOptions configures [ProviderContext.CreateDataSet].
type CreateDataSetOptions struct {
	// OnSubmitted is invoked after the create transaction is submitted and
	// before waiting for confirmation. It may be nil.
	OnSubmitted func(CreateDataSetSubmission)
}

// CreateDataSetSubmission identifies a submitted create-dataset transaction.
// Persist and restore all fields together using the strict lowerCamel JSON
// form; incomplete submissions and alternate capitalization are rejected. A
// zero ProviderID is filled from the ProviderContext used to wait.
type CreateDataSetSubmission struct {
	ProviderID    types.BigInt `json:"providerId"`
	TransactionID string       `json:"transactionId"`
	StatusURL     string       `json:"statusUrl"`
	// ClientDataSetID must be non-nil when resuming a submitted create.
	ClientDataSetID *types.BigInt `json:"clientDataSetId"`
}

// CreateDataSetResult is returned after standalone dataset creation confirms.
type CreateDataSetResult struct {
	TransactionID          string     `json:"transactionId"`
	ConfirmedTransactionID string     `json:"confirmedTransactionId"`
	DataSet                DataSetRef `json:"dataSet"`
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

// UploadOptions configures upload operations. Service.Upload requires Copies
// and accepts target-selection fields. Service.UploadToContexts rejects target
// selection fields. ProviderContext.Upload and DataSetContext.Upload also
// reject callbacks that only apply to secondary copies.
//
// Some lifecycle callbacks may be invoked from internal orchestration
// goroutines. Callers that share mutable state across callbacks must keep their
// handlers concurrency-safe. Service.Upload, ProviderContext.Upload, and
// DataSetContext.Upload recover and ignore callback panics; when a logger is
// configured, the first panic per callback name in an upload logs a warning.
// This recovery does not apply to direct StoreOptions, PullRequest, or
// CommitRequest hooks.
type UploadOptions struct {
	// Copies is the number of provider copies to store. Service.Upload requires
	// a positive value. Explicit-context upload methods do not accept it.
	Copies int
	// PieceMetadata is stored with each piece on-chain.
	PieceMetadata map[string]string
	// DataSetMetadata is stored with the data set on first creation.
	DataSetMetadata map[string]string
	// ExcludeProviderIDs skips these providers only during auto-selection.
	ExcludeProviderIDs []types.BigInt
	// RequireEndorsedPrimary controls automatic primary selection. nil and true
	// require the primary to be in the configured endorsement set; false uses
	// the full approved-provider pool and does not query endorsements. Explicit-
	// context upload methods reject this field.
	RequireEndorsedPrimary *bool
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
