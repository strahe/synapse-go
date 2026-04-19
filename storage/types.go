package storage

import (
	"math/big"

	"github.com/ipfs/go-cid"
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
	PullStatusComplete   PullStatus = "complete"
	PullStatusFailed     PullStatus = "failed"
)

// StoreOptions configures a single StoreBytes call. Reserved for future use.
type StoreOptions struct{}

// StoreResult is returned by a successful StoreBytes call.
type StoreResult struct {
	PieceCID cid.Cid // PieceCIDv2 of the stored data
	Size     int64   // raw (unpadded) byte count
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
}

// CommitResult is returned by a successful Commit call.
type CommitResult struct {
	TransactionID string   // on-chain transaction hash
	DataSetID     *big.Int // data set that now holds the piece
	PieceIDs      []*big.Int
	IsNewDataSet  bool // true when a new data set was created by this commit
}

// CopyResult describes one successfully committed copy.
type CopyResult struct {
	ProviderID   *big.Int
	DataSetID    *big.Int
	PieceID      *big.Int
	Role         CopyRole
	RetrievalURL string // HTTPS retrieval URL for this piece on the provider.
	IsNewDataSet bool
}

// FailedAttempt records a provider attempt that did not produce a copy.
type FailedAttempt struct {
	ProviderID *big.Int
	Role       CopyRole
	Stage      CopyStage // pipeline stage where the failure occurred
	Err        error
	Explicit   bool // true when the provider was caller-specified (no auto-retry)
}

// UploadResult is returned by a successful Upload or UploadBytes call.
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
	PieceCID        cid.Cid
	Size            int64 // raw (unpadded) byte count
	RequestedCopies int
	// Complete is true when all RequestedCopies were committed on-chain.
	// Equivalent to len(Copies) >= RequestedCopies.
	Complete        bool
	Copies          []CopyResult
	FailedAttempts  []FailedAttempt
}

// SuccessCount returns the number of copies that were successfully committed
// on-chain. Equivalent to len(Copies).
func (r *UploadResult) SuccessCount() int {
	if r == nil {
		return 0
	}
	return len(r.Copies)
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

// UploadOptions configures an Upload or UploadBytes call.
type UploadOptions struct {
	// Copies is the number of provider copies to store. Zero means the resolver
	// default: len(DataSetIDs) or len(ProviderIDs) when those are set, otherwise 2.
	Copies int
	// PieceMetadata is stored with each piece on-chain.
	PieceMetadata map[string]string
	// DataSetMetadata is stored with the data set on first creation.
	DataSetMetadata map[string]string
	// ProviderIDs pins the upload to specific providers by ID.
	ProviderIDs []*big.Int
	// DataSetIDs pins the upload to specific existing data sets.
	DataSetIDs []*big.Int
	// ExcludeProviderIDs skips these providers during auto-selection.
	ExcludeProviderIDs []*big.Int
	// WithCDN enables CDN services for this upload.
	WithCDN bool
}
