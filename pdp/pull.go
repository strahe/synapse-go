package pdp

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"
	"github.com/strahe/synapse-go/types"
)

// PullStatus mirrors the status values used by the PDP pull endpoint.
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
var ErrPullFailed = errors.New("pdp: pull failed")

// PullPieceInput is one entry in a pull request.
type PullPieceInput struct {
	// PieceCID is the piece to pull: a PieceCIDv2 whose raw size is between
	// chain.MinUploadSize and chain.MaxUploadSize.
	PieceCID cid.Cid
	// SourceURL is an HTTPS URL ending in /piece/{pieceCid} on the source SP.
	SourceURL string
}

// PullRequest carries all parameters for POST /pdp/piece/pull.
type PullRequest struct {
	// RecordKeeper is the record-keeper contract address (e.g. FWSS). The
	// pull request body always carries it, even when reusing an existing dataset.
	RecordKeeper common.Address
	// ExtraData is caller-provided EIP-712 signed data encoded as the provider expects.
	ExtraData []byte
	// DataSetID is the target dataset. Nil means create a new dataset.
	DataSetID *types.BigInt
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
	DataSetID    *json.Number        `json:"dataSetId,omitempty"`
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
// This makes it safe to poll for status using repeated calls. Transient
// failures are retried. When the provider's pull queue is full, the error
// matches ErrPullQueueFull and is returned without retrying; send the same
// request again after the wrapped *HTTPError's RetryAfter. Requests
// exceeding MaxAddPiecesMessageSize are rejected before submission. Existing
// legacy data sets also have a MaxLegacyAddPiecesBatchSize count limit when
// WithLegacyPieceStorageIDLimit configures their deployment's cutoff.
func (c *Client) PullPieces(ctx context.Context, req PullRequest) (*PullResult, error) {
	if err := c.validateLegacyAddPiecesBatch("pdp.PullPieces", req.DataSetID, len(req.Pieces)); err != nil {
		return nil, err
	}
	if err := validateAddPiecesBatch("pdp.PullPieces", len(req.Pieces)); err != nil {
		return nil, err
	}
	if len(req.ExtraData) == 0 {
		return nil, errors.New("pdp.PullPieces: empty extraData")
	}

	if req.RecordKeeper == (common.Address{}) {
		return nil, errors.New("pdp.PullPieces: recordKeeper is required")
	}

	wire := pullPiecesWire{
		ExtraData:    "0x" + hex.EncodeToString(req.ExtraData),
		RecordKeeper: req.RecordKeeper.Hex(),
		Pieces:       make([]pullPieceWireItem, 0, len(req.Pieces)),
	}
	addPieces := make([]AddPieceInput, 0, len(req.Pieces))

	if req.DataSetID != nil {
		if req.DataSetID.IsZero() {
			return nil, errors.New("pdp.PullPieces: zero dataSetID")
		}
		ds := json.Number(req.DataSetID.String())
		wire.DataSetID = &ds
	}

	for i, p := range req.Pieces {
		if err := validateUploadPieceCID("pdp.PullPieces", i, p.PieceCID); err != nil {
			return nil, err
		}
		if p.SourceURL == "" {
			return nil, errors.New("pdp.PullPieces: empty sourceURL in input")
		}
		wire.Pieces = append(wire.Pieces, pullPieceWireItem{
			PieceCid:  p.PieceCID.String(),
			SourceURL: p.SourceURL,
		})
		addPieces = append(addPieces, AddPieceInput{PieceCID: p.PieceCID})
	}
	if err := validateAddPiecesMessageSize("pdp.PullPieces", addPieces, req.ExtraData); err != nil {
		return nil, err
	}

	body, err := c.postPull(ctx, wire)
	if err != nil {
		return nil, err
	}

	var out PullResult
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("pdp.PullPieces: decode response: %w", err)
	}
	return &out, nil
}

// postPull sends one pull request and retries transient failures. A full pull
// queue is returned at once as ErrPullQueueFull so the caller decides when to
// send it again.
func (c *Client) postPull(ctx context.Context, wire pullPiecesWire) ([]byte, error) {
	const path = "pdp/piece/pull"
	u, err := c.resolve(path)
	if err != nil {
		return nil, fmt.Errorf("pdp: resolve %s: %w", path, err)
	}
	buf, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("pdp: marshal %s: %w", path, err)
	}
	_, body, err := c.doRetryableWithExecutor(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
		if err != nil {
			return nil, fmt.Errorf("pdp: build POST %s: %w", path, err)
		}
		req.Header.Set("Content-Type", "application/json")
		return req, nil
	}, func(req *http.Request) (*http.Response, []byte, error) {
		resp, body, err := c.do(req, http.StatusOK, http.StatusCreated, http.StatusAccepted)
		if err != nil && resp != nil && resp.StatusCode == http.StatusTooManyRequests {
			// The status and Retry-After arrive before the body, so a body
			// read failure still reports a full queue.
			if _, ok := errors.AsType[*HTTPError](err); !ok {
				err = errors.Join(newHTTPError(req, resp, body), err)
			}
			err = fmt.Errorf("pdp.PullPieces: %w: %w", ErrPullQueueFull, err)
		}
		return resp, body, err
	})
	return body, err
}

const (
	// defaultPullQueueFullDelay applies when a full pull queue gives no
	// usable Retry-After.
	defaultPullQueueFullDelay = time.Minute
	// minPullQueueFullDelay is the shortest Retry-After honored, so a malformed
	// header cannot turn the wait into back-to-back requests.
	minPullQueueFullDelay = time.Second
	// maxPullQueueFullDelay bounds the provider's Retry-After so one response
	// cannot stall a wait that has no deadline.
	maxPullQueueFullDelay = 5 * time.Minute
)

// pullQueueFullDelay returns how long to wait before resending a pull that the
// provider declined because its queue was full.
func pullQueueFullDelay(err error) time.Duration {
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.RetryAfter < minPullQueueFullDelay {
		return defaultPullQueueFullDelay
	}
	return min(httpErr.RetryAfter, maxPullQueueFullDelay)
}

// WaitForPullComplete polls PullPieces until the overall pull status is
// "complete" or "failed". On failure it returns (result, ErrPullFailed) so
// callers can inspect the per-piece statuses. When the provider's pull queue
// is full, it waits for the provider's Retry-After (one minute when absent or
// under a second, at most five minutes) and sends the request again until ctx
// ends.
//
// onStatus is invoked after each poll (may be nil). A zero pollInterval
// defaults to 4 seconds.
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
		if errors.Is(err, ErrPullQueueFull) {
			delayFn := c.pullQueueFullDelayFn
			if delayFn == nil {
				delayFn = pullQueueFullDelay
			}
			delay := delayFn(err)
			if c.logger != nil {
				c.logger.Debug("pdp pull queue full", "wait", delay)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}
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
