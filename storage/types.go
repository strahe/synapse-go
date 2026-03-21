package storage

import (
	"math/big"

	"github.com/ipfs/go-cid"
)

type CopyRole string

const (
	CopyRolePrimary   CopyRole = "primary"
	CopyRoleSecondary CopyRole = "secondary"
)

type CopyStage string

const (
	CopyStageStore   CopyStage = "store"
	CopyStagePull    CopyStage = "pull"
	CopyStagePresign CopyStage = "presign"
	CopyStageCommit  CopyStage = "commit"
)

type PullStatus string

const (
	PullStatusPending    PullStatus = "pending"
	PullStatusInProgress PullStatus = "inProgress"
	PullStatusComplete   PullStatus = "complete"
	PullStatusFailed     PullStatus = "failed"
)

type StoreOptions struct{}

type StoreResult struct {
	PieceCID cid.Cid
	Size     int64
}

type PieceInput struct {
	PieceCID      cid.Cid
	PieceMetadata map[string]string
}

type PullRequest struct {
	Pieces    []cid.Cid
	From      func(cid.Cid) string
	ExtraData []byte
}

type PullPieceResult struct {
	PieceCID cid.Cid
	Status   PullStatus
}

type PullResult struct {
	Status PullStatus
	Pieces []PullPieceResult
}

type CommitRequest struct {
	Pieces    []PieceInput
	ExtraData []byte
}

type CommitResult struct {
	TransactionID string
	DataSetID     *big.Int
	PieceIDs      []*big.Int
	IsNewDataSet  bool
}

type CopyResult struct {
	ProviderID   *big.Int
	DataSetID    *big.Int
	PieceID      *big.Int
	Role         CopyRole
	RetrievalURL string
	IsNewDataSet bool
}

type FailedAttempt struct {
	ProviderID *big.Int
	Role       CopyRole
	Stage      CopyStage
	Err        error
	Explicit   bool
}

type UploadResult struct {
	PieceCID        cid.Cid
	Size            int64
	RequestedCopies int
	Complete        bool
	Copies          []CopyResult
	FailedAttempts  []FailedAttempt
}

type UploadOptions struct {
	Copies             int
	PieceMetadata      map[string]string
	DataSetMetadata    map[string]string
	ProviderIDs        []*big.Int
	DataSetIDs         []*big.Int
	ExcludeProviderIDs []*big.Int
	WithCDN            bool
}
