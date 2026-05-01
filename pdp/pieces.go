package pdp

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
	"github.com/ipfs/go-cid"
)

// AddPieceInput mirrors one entry of the pieces array for
// POST /pdp/data-sets/{id}/pieces. The wire format uses the piece CID
// as its own single sub-piece.
type AddPieceInput struct {
	PieceCID cid.Cid
}

// addPiecesRequest is the wire body for POST /pdp/data-sets/{id}/pieces.
type addPiecesRequest struct {
	Pieces    []addPiecesRequestPiece `json:"pieces"`
	ExtraData string                  `json:"extraData"`
}

type addPiecesRequestPiece struct {
	PieceCID  string              `json:"pieceCid"`
	SubPieces []addPiecesSubPiece `json:"subPieces"`
}

type addPiecesSubPiece struct {
	SubPieceCID string `json:"subPieceCid"`
}

// AddPiecesResult is what the client gets back from a successful POST.
type AddPiecesResult struct {
	TxHash    common.Hash
	StatusURL string
}

// AddPieces calls POST /pdp/data-sets/{dataSetId}/pieces. extraData must be
// caller-provided EIP-712 signed data encoded as the PDP provider expects.
func (c *Client) AddPieces(ctx context.Context, dataSetID uint64, pieces []AddPieceInput, extraData []byte) (*AddPiecesResult, error) {
	if len(pieces) == 0 {
		return nil, errors.New("pdp.AddPieces: no pieces provided")
	}
	if len(extraData) == 0 {
		return nil, errors.New("pdp.AddPieces: empty extraData")
	}
	wire := addPiecesRequest{
		ExtraData: "0x" + hex.EncodeToString(extraData),
	}
	for _, p := range pieces {
		if !p.PieceCID.Defined() {
			return nil, errors.New("pdp.AddPieces: undefined pieceCID in input")
		}
		s := p.PieceCID.String()
		wire.Pieces = append(wire.Pieces, addPiecesRequestPiece{
			PieceCID:  s,
			SubPieces: []addPiecesSubPiece{{SubPieceCID: s}},
		})
	}
	urlPath := path.Join("pdp/data-sets", fmt.Sprint(dataSetID), "pieces")
	resp, body, err := c.postJSON(ctx, urlPath, wire,
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
		return nil, fmt.Errorf("%w: zero tx hash from %q", ErrLocationHeader, loc)
	}
	statusURL, err := c.resolve(loc)
	if err != nil {
		return nil, fmt.Errorf("pdp.AddPieces: resolve status URL: %w", err)
	}
	return &AddPiecesResult{TxHash: tx, StatusURL: statusURL.String()}, nil
}

// AddPiecesStatus mirrors GET /pdp/data-sets/{id}/pieces/added/{txHash}.
// TxStatus reports pending, confirmed, or rejected.
type AddPiecesStatus struct {
	TxHash            common.Hash `json:"-"`
	TxStatus          string      `json:"txStatus"` // pending | confirmed | rejected
	DataSetID         uint64      `json:"-"`
	PieceCount        int         `json:"pieceCount"`
	AddMessageOK      *bool       `json:"addMessageOk"`
	PiecesAdded       bool        `json:"piecesAdded"`
	ConfirmedPieceIDs []*big.Int  `json:"-"`
}

type rawAddPiecesStatus struct {
	TxHash            string        `json:"txHash"`
	TxStatus          string        `json:"txStatus"`
	DataSetID         json.Number   `json:"dataSetId"`
	PieceCount        int           `json:"pieceCount"`
	AddMessageOK      *bool         `json:"addMessageOk"`
	PiecesAdded       bool          `json:"piecesAdded"`
	ConfirmedPieceIDs []json.Number `json:"confirmedPieceIds,omitempty"`
}

// GetAddPiecesStatus polls the status URL once.
func (c *Client) GetAddPiecesStatus(ctx context.Context, statusURL string) (*AddPiecesStatus, error) {
	if statusURL == "" {
		return nil, errors.New("pdp.GetAddPiecesStatus: empty statusURL")
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
	var raw rawAddPiecesStatus
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("pdp.GetAddPiecesStatus: decode: %w", err)
	}
	out := &AddPiecesStatus{
		TxHash:       common.HexToHash(raw.TxHash),
		TxStatus:     raw.TxStatus,
		PieceCount:   raw.PieceCount,
		AddMessageOK: raw.AddMessageOK,
		PiecesAdded:  raw.PiecesAdded,
	}
	if raw.DataSetID != "" {
		id, ok := new(big.Int).SetString(raw.DataSetID.String(), 10)
		if !ok || !id.IsUint64() {
			return nil, fmt.Errorf("pdp.GetAddPiecesStatus: bad dataSetId %q", raw.DataSetID)
		}
		out.DataSetID = id.Uint64()
	}
	for _, n := range raw.ConfirmedPieceIDs {
		b, ok := new(big.Int).SetString(n.String(), 10)
		if !ok {
			return nil, fmt.Errorf("pdp.GetAddPiecesStatus: bad confirmedPieceId %q", n)
		}
		out.ConfirmedPieceIDs = append(out.ConfirmedPieceIDs, b)
	}
	return out, nil
}

// WaitForPiecesAdded polls until the add-pieces tx is confirmed with
// piecesAdded=true or rejected. Transport errors, including HTTP 404s from
// the status URL, are returned immediately rather than retried. pollInterval
// defaults to 4s.
func (c *Client) WaitForPiecesAdded(ctx context.Context, statusURL string, pollInterval time.Duration) (*AddPiecesStatus, error) {
	if pollInterval <= 0 {
		pollInterval = 4 * time.Second
	}
	for {
		status, err := c.GetAddPiecesStatus(ctx, statusURL)
		if err != nil {
			return nil, err
		}
		switch status.TxStatus {
		case "confirmed":
			if !status.PiecesAdded {
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

// SchedulePieceDeletion issues DELETE /pdp/data-sets/{id}/pieces/{pieceId}
// with the provided EIP-712 signed extraData.
func (c *Client) SchedulePieceDeletion(ctx context.Context, dataSetID, pieceID uint64, extraData []byte) (common.Hash, error) {
	if len(extraData) == 0 {
		return common.Hash{}, errors.New("pdp.SchedulePieceDeletion: empty extraData")
	}
	payload := struct {
		ExtraData string `json:"extraData"`
	}{ExtraData: "0x" + hex.EncodeToString(extraData)}

	var out struct {
		TxHash string `json:"txHash"`
	}
	urlPath := path.Join("pdp/data-sets", fmt.Sprint(dataSetID), "pieces", fmt.Sprint(pieceID))
	if err := c.deleteJSON(ctx, urlPath, payload, &out); err != nil {
		return common.Hash{}, err
	}
	h := common.HexToHash(out.TxHash)
	if h == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("pdp.SchedulePieceDeletion: empty txHash in response")
	}
	return h, nil
}
