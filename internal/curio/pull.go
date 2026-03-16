package curio

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"
)

// PullStatus mirrors the status values used by the Curio pull endpoint.
type PullStatus string

const (
	PullStatusPending    PullStatus = "pending"
	PullStatusInProgress PullStatus = "inProgress"
	PullStatusRetrying   PullStatus = "retrying"
	PullStatusComplete   PullStatus = "complete"
	PullStatusFailed     PullStatus = "failed"
)

// ErrPullFailed is returned by WaitForPullComplete when the server reports
// that the overall pull status is "failed".
var ErrPullFailed = errors.New("curio: pull failed")

// PullPieceInput is one entry in a pull request.
type PullPieceInput struct {
	// PieceCID is the piece to pull.
	PieceCID cid.Cid
	// SourceURL is an HTTPS URL ending in /piece/{pieceCid} on the source SP.
	SourceURL string
}

// PullRequest carries all parameters for POST /pdp/piece/pull.
type PullRequest struct {
	// RecordKeeper is the record-keeper contract address (e.g. FWSS). Curio's
	// pull request body always carries it, even when reusing an existing dataset.
	RecordKeeper common.Address
	// ExtraData is the EIP-712 signed blob produced by the typeddata signer.
	ExtraData []byte
	// DataSetID is the target dataset.  0 (or unset) means create a new dataset.
	DataSetID uint64
	// Pieces are the pieces to pull with their source URLs.
	Pieces []PullPieceInput
}

// PullPieceStatus is the per-piece status returned by POST /pdp/piece/pull.
type PullPieceStatus struct {
	PieceCID string     `json:"pieceCid"`
	Status   PullStatus `json:"status"`
}

// PullResult is what the server returns from POST /pdp/piece/pull.
type PullResult struct {
	Status PullStatus        `json:"status"`
	Pieces []PullPieceStatus `json:"pieces"`
}

// pullPiecesWire is the JSON body sent to POST /pdp/piece/pull.
type pullPiecesWire struct {
	ExtraData    string              `json:"extraData"`
	RecordKeeper string              `json:"recordKeeper,omitempty"`
	DataSetID    *uint64             `json:"dataSetId,omitempty"`
	Pieces       []pullPieceWireItem `json:"pieces"`
}

type pullPieceWireItem struct {
	PieceCid  string `json:"pieceCid"`
	SourceURL string `json:"sourceUrl"`
}

// PullPieces calls POST /pdp/piece/pull to request that this SP pull the
// given pieces from the source URLs.
//
// The endpoint is idempotent: calling again with the same extraData returns
// the status of the existing pull request rather than creating a duplicate.
// This makes it safe to poll for status using repeated calls.
//
// Mirrors TS synapse-core sp/pull-pieces.ts::pullPiecesApiRequest.
func (c *Client) PullPieces(ctx context.Context, req PullRequest) (*PullResult, error) {
	if len(req.Pieces) == 0 {
		return nil, errors.New("curio.PullPieces: no pieces provided")
	}
	if len(req.ExtraData) == 0 {
		return nil, errors.New("curio.PullPieces: empty extraData")
	}

	if req.RecordKeeper == (common.Address{}) {
		return nil, errors.New("curio.PullPieces: recordKeeper is required")
	}

	wire := pullPiecesWire{
		ExtraData:    "0x" + hex.EncodeToString(req.ExtraData),
		RecordKeeper: req.RecordKeeper.Hex(),
		Pieces:       make([]pullPieceWireItem, 0, len(req.Pieces)),
	}

	if req.DataSetID != 0 {
		ds := req.DataSetID
		wire.DataSetID = &ds
	}

	for _, p := range req.Pieces {
		if err := validatePieceCIDV2("curio.PullPieces", p.PieceCID); err != nil {
			return nil, err
		}
		if p.SourceURL == "" {
			return nil, errors.New("curio.PullPieces: empty sourceURL in input")
		}
		wire.Pieces = append(wire.Pieces, pullPieceWireItem{
			PieceCid:  p.PieceCID.String(),
			SourceURL: p.SourceURL,
		})
	}

	_, body, err := c.postJSON(ctx, "pdp/piece/pull", wire, http.StatusOK, http.StatusCreated, http.StatusAccepted)
	if err != nil {
		return nil, err
	}

	var out PullResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("curio.PullPieces: decode response: %w", err)
	}
	return &out, nil
}

// WaitForPullComplete polls PullPieces until the overall pull status is
// "complete" or "failed". On failure it returns (result, ErrPullFailed) so
// callers can inspect the per-piece statuses.
//
// onStatus is invoked after each poll (may be nil). A zero pollInterval
// defaults to 4 s (matching the TS SDK DELAY_TIME constant).
//
// Mirrors TS synapse-core sp/pull-pieces.ts::waitForPullPiecesApiRequest.
func (c *Client) WaitForPullComplete(
	ctx context.Context,
	req PullRequest,
	pollInterval time.Duration,
	onStatus func(*PullResult),
) (*PullResult, error) {
	if pollInterval <= 0 {
		pollInterval = 4 * time.Second
	}

	for {
		res, err := c.PullPieces(ctx, req)
		if err != nil {
			return nil, err
		}

		if onStatus != nil {
			onStatus(res)
		}

		switch res.Status {
		case PullStatusComplete:
			return res, nil
		case PullStatusFailed:
			return res, ErrPullFailed
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
