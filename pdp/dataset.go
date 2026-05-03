package pdp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/types"
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

// CreateDataSet calls POST /pdp/data-sets. extraData must be an EIP-712
// signed blob encoded as the PDP provider expects.
func (c *Client) CreateDataSet(ctx context.Context, recordKeeper common.Address, extraData []byte) (*CreateDataSetResult, error) {
	if (recordKeeper == common.Address{}) {
		return nil, errors.New("pdp.CreateDataSet: zero recordKeeper")
	}
	if len(extraData) == 0 {
		return nil, errors.New("pdp.CreateDataSet: empty extraData")
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
		return nil, fmt.Errorf("pdp.CreateDataSet: resolve status URL: %w", err)
	}
	return &CreateDataSetResult{TxHash: tx, StatusURL: statusURL.String()}, nil
}

// CreateDataSetStatus mirrors GET /pdp/data-sets/created/{txHash}.
// TxStatus reports pending, confirmed, or rejected.
type CreateDataSetStatus struct {
	CreateMessageHash common.Hash   `json:"createMessageHash"`
	Service           string        `json:"service"`
	TxStatus          string        `json:"txStatus"` // pending | confirmed | rejected
	DataSetCreated    bool          `json:"dataSetCreated"`
	OK                *bool         `json:"ok"`
	DataSetID         *types.BigInt `json:"-"`
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
		return nil, errors.New("pdp.GetDataSetCreationStatus: empty statusURL")
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
		return nil, fmt.Errorf("pdp.GetDataSetCreationStatus: decode: %w", err)
	}
	out := &CreateDataSetStatus{
		CreateMessageHash: common.HexToHash(raw.CreateMessageHash),
		Service:           raw.Service,
		TxStatus:          raw.TxStatus,
		DataSetCreated:    raw.DataSetCreated,
		OK:                raw.OK,
	}
	if raw.DataSetID != "" {
		id, err := parseBigIntNumber("pdp.GetDataSetCreationStatus", "dataSetId", raw.DataSetID)
		if err != nil {
			return nil, err
		}
		out.DataSetID = &id
	}
	return out, nil
}

// WaitForDataSetCreated polls until the server reports txStatus=confirmed
// with dataSetCreated=true (success) or txStatus=rejected (ErrTxRejected).
// Transport errors, including HTTP 404s from the status URL, are returned
// immediately rather than retried. pollInterval defaults to 4 seconds.
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
	PieceCID       string       `json:"pieceCid"`
	PieceID        types.BigInt `json:"-"`
	SubPieceCID    string       `json:"subPieceCid"`
	SubPieceOffset uint64       `json:"subPieceOffset"`
}

// DataSet mirrors the JSON returned by GET /pdp/data-sets/{id}.
type DataSet struct {
	ID                 types.BigInt   `json:"-"`
	NextChallengeEpoch int64          `json:"nextChallengeEpoch"`
	Pieces             []DataSetPiece `json:"pieces"`
}

type rawDataSetPiece struct {
	PieceCID       string      `json:"pieceCid"`
	PieceID        json.Number `json:"pieceId"`
	SubPieceCID    string      `json:"subPieceCid"`
	SubPieceOffset uint64      `json:"subPieceOffset"`
}

type rawDataSet struct {
	ID                 json.Number       `json:"id"`
	NextChallengeEpoch int64             `json:"nextChallengeEpoch"`
	Pieces             []rawDataSetPiece `json:"pieces"`
}

func (p DataSetPiece) MarshalJSON() ([]byte, error) {
	return json.Marshal(rawDataSetPiece{
		PieceCID:       p.PieceCID,
		PieceID:        json.Number(p.PieceID.String()),
		SubPieceCID:    p.SubPieceCID,
		SubPieceOffset: p.SubPieceOffset,
	})
}

func (p *DataSetPiece) UnmarshalJSON(data []byte) error {
	var raw rawDataSetPiece
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	piece, err := dataSetPieceFromRaw("pdp.DataSetPiece.UnmarshalJSON", raw)
	if err != nil {
		return err
	}
	*p = piece
	return nil
}

func (d DataSet) MarshalJSON() ([]byte, error) {
	raw := rawDataSet{
		ID:                 json.Number(d.ID.String()),
		NextChallengeEpoch: d.NextChallengeEpoch,
		Pieces:             make([]rawDataSetPiece, len(d.Pieces)),
	}
	for i, p := range d.Pieces {
		raw.Pieces[i] = rawDataSetPiece{
			PieceCID:       p.PieceCID,
			PieceID:        json.Number(p.PieceID.String()),
			SubPieceCID:    p.SubPieceCID,
			SubPieceOffset: p.SubPieceOffset,
		}
	}
	return json.Marshal(raw)
}

func (d *DataSet) UnmarshalJSON(data []byte) error {
	var raw rawDataSet
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	out, err := dataSetFromRaw("pdp.DataSet.UnmarshalJSON", raw)
	if err != nil {
		return err
	}
	*d = out
	return nil
}

// GetDataSet calls GET /pdp/data-sets/{dataSetId}.
func (c *Client) GetDataSet(ctx context.Context, dataSetID types.BigInt) (*DataSet, error) {
	var raw rawDataSet
	if err := c.getJSON(ctx, path.Join("pdp/data-sets", dataSetID.String()), &raw); err != nil {
		return nil, err
	}
	out, err := dataSetFromRaw("pdp.GetDataSet", raw)
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func dataSetFromRaw(op string, raw rawDataSet) (DataSet, error) {
	id, err := parseBigIntNumber(op, "id", raw.ID)
	if err != nil {
		return DataSet{}, err
	}
	out := DataSet{
		ID:                 id,
		NextChallengeEpoch: raw.NextChallengeEpoch,
		Pieces:             make([]DataSetPiece, 0, len(raw.Pieces)),
	}
	for _, p := range raw.Pieces {
		piece, err := dataSetPieceFromRaw(op, p)
		if err != nil {
			return DataSet{}, err
		}
		out.Pieces = append(out.Pieces, piece)
	}
	return out, nil
}

func dataSetPieceFromRaw(op string, raw rawDataSetPiece) (DataSetPiece, error) {
	pieceID, err := parseBigIntNumber(op, "pieceId", raw.PieceID)
	if err != nil {
		return DataSetPiece{}, err
	}
	return DataSetPiece{
		PieceCID:       raw.PieceCID,
		PieceID:        pieceID,
		SubPieceCID:    raw.SubPieceCID,
		SubPieceOffset: raw.SubPieceOffset,
	}, nil
}
