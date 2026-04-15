package curio

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// CreateDataSetRequest is the body of POST /pdp/data-sets.
type CreateDataSetRequest struct {
	RecordKeeper common.Address `json:"recordKeeper"`
	ExtraData    string         `json:"extraData"` // 0x-prefixed hex
}

// CreateDataSetResult carries what the server returns from POST
// /pdp/data-sets: the transaction hash (parsed out of the Location header)
// and the absolute status URL the caller can poll.
type CreateDataSetResult struct {
	TxHash    common.Hash
	StatusURL string
}

// CreateDataSet calls POST /pdp/data-sets. extraData must be a hex-encoded
// EIP-712 signed blob produced by the internal/typeddata signer (the same
// format viem's encodeAbiParameters emits on the TS side).
func (c *Client) CreateDataSet(ctx context.Context, recordKeeper common.Address, extraData []byte) (*CreateDataSetResult, error) {
	if (recordKeeper == common.Address{}) {
		return nil, errors.New("curio.CreateDataSet: zero recordKeeper")
	}
	if len(extraData) == 0 {
		return nil, errors.New("curio.CreateDataSet: empty extraData")
	}
	payload := CreateDataSetRequest{
		RecordKeeper: recordKeeper,
		ExtraData:    "0x" + hex.EncodeToString(extraData),
	}
	resp, body, err := c.postJSON(ctx, "pdp/data-sets", payload,
		http.StatusCreated, http.StatusOK, http.StatusAccepted)
	if err != nil {
		return nil, err
	}
	loc := resp.Header.Get("Location")
	hashHex := lastPathSegment(loc)
	if loc == "" || hashHex == "" {
		return nil, fmt.Errorf("%w: Location=%q body=%q", ErrLocationHeader, loc, string(body))
	}
	if !strings.HasPrefix(hashHex, "0x") {
		hashHex = "0x" + hashHex
	}
	tx := common.HexToHash(hashHex)
	if tx == (common.Hash{}) {
		return nil, fmt.Errorf("%w: parsed zero tx hash from %q", ErrLocationHeader, loc)
	}
	statusURL, err := c.resolve(loc)
	if err != nil {
		return nil, fmt.Errorf("curio.CreateDataSet: resolve status URL: %w", err)
	}
	return &CreateDataSetResult{TxHash: tx, StatusURL: statusURL.String()}, nil
}

// CreateDataSetStatus mirrors GET /pdp/data-sets/created/{txHash}. Fields
// match the discriminated-union variants on the TS side:
// pending/confirmed/rejected (see CreateDataSet{Pending,Rejected,Success}Schema
// in synapse-core/src/sp/create-dataset.ts).
type CreateDataSetStatus struct {
	CreateMessageHash common.Hash `json:"createMessageHash"`
	Service           string      `json:"service"`
	TxStatus          string      `json:"txStatus"` // pending | confirmed | rejected
	DataSetCreated    bool        `json:"dataSetCreated"`
	OK                *bool       `json:"ok"`
	DataSetID         *big.Int    `json:"-"`
}

// rawCreateDataSetStatus mirrors the JSON wire format; DataSetID comes as
// a JSON number, which doesn't fit in *big.Int directly.
type rawCreateDataSetStatus struct {
	CreateMessageHash string      `json:"createMessageHash"`
	Service           string      `json:"service"`
	TxStatus          string      `json:"txStatus"`
	DataSetCreated    bool        `json:"dataSetCreated"`
	OK                *bool       `json:"ok"`
	DataSetID         json.Number `json:"dataSetId,omitempty"`
}

// GetDataSetCreationStatus polls the status URL once.
func (c *Client) GetDataSetCreationStatus(ctx context.Context, statusURL string) (*CreateDataSetStatus, error) {
	if statusURL == "" {
		return nil, errors.New("curio.GetDataSetCreationStatus: empty statusURL")
	}
	_, body, err := c.doRetryable(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	}, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var raw rawCreateDataSetStatus
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("curio.GetDataSetCreationStatus: decode: %w", err)
	}
	out := &CreateDataSetStatus{
		CreateMessageHash: common.HexToHash(raw.CreateMessageHash),
		Service:           raw.Service,
		TxStatus:          raw.TxStatus,
		DataSetCreated:    raw.DataSetCreated,
		OK:                raw.OK,
	}
	if raw.DataSetID != "" {
		id, ok := new(big.Int).SetString(raw.DataSetID.String(), 10)
		if !ok {
			return nil, fmt.Errorf("curio.GetDataSetCreationStatus: bad dataSetId %q", raw.DataSetID)
		}
		out.DataSetID = id
	}
	return out, nil
}

// WaitForDataSetCreated polls until the server reports txStatus=confirmed
// with dataSetCreated=true (success) or txStatus=rejected (ErrTxRejected).
// Transport errors, including HTTP 404s from the status URL, are returned
// immediately rather than retried. pollInterval defaults to 4s (matching
// the TS SDK's DELAY_TIME).
func (c *Client) WaitForDataSetCreated(ctx context.Context, statusURL string, pollInterval time.Duration) (*CreateDataSetStatus, error) {
	if pollInterval <= 0 {
		pollInterval = 4 * time.Second
	}
	for {
		status, err := c.GetDataSetCreationStatus(ctx, statusURL)
		if err != nil {
			return nil, err
		}
		switch status.TxStatus {
		case "confirmed":
			if !status.DataSetCreated {
				break
			}
			return status, nil
		case "rejected":
			return status, ErrTxRejected
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// DataSetPiece mirrors a piece entry returned by GET /pdp/data-sets/{id}.
type DataSetPiece struct {
	PieceCID       string `json:"pieceCid"`
	PieceID        uint64 `json:"pieceId"`
	SubPieceCID    string `json:"subPieceCid"`
	SubPieceOffset uint64 `json:"subPieceOffset"`
}

// DataSet mirrors the JSON returned by GET /pdp/data-sets/{id}.
type DataSet struct {
	ID                 uint64         `json:"id"`
	NextChallengeEpoch int64          `json:"nextChallengeEpoch"`
	Pieces             []DataSetPiece `json:"pieces"`
}

// GetDataSet calls GET /pdp/data-sets/{dataSetId}.
func (c *Client) GetDataSet(ctx context.Context, dataSetID uint64) (*DataSet, error) {
	var out DataSet
	if err := c.getJSON(ctx, path.Join("pdp/data-sets", fmt.Sprint(dataSetID)), &out); err != nil {
		return nil, err
	}
	return &out, nil
}
