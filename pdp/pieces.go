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
	"github.com/ipfs/go-cid"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

// MaxAddPiecesBatchSize is the maximum number of pieces accepted by
// add-pieces style PDP requests.
const MaxAddPiecesBatchSize = 40

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
// Piece CIDs must be unique within one request after PieceCIDv2-to-v1
// normalization; the same CID may be used again in a later request.
func (c *Client) AddPieces(ctx context.Context, dataSetID types.BigInt, pieces []AddPieceInput, extraData []byte) (*AddPiecesResult, error) {
	if err := validateAddPieceInputs("pdp.AddPieces", pieces); err != nil {
		return nil, err
	}
	if len(extraData) == 0 {
		return nil, errors.New("pdp.AddPieces: empty extraData")
	}
	wire := addPiecesRequest{
		ExtraData: "0x" + hex.EncodeToString(extraData),
	}
	for _, p := range pieces {
		s := p.PieceCID.String()
		wire.Pieces = append(wire.Pieces, addPiecesRequestPiece{
			PieceCID:  s,
			SubPieces: []addPiecesSubPiece{{SubPieceCID: s}},
		})
	}
	urlPath := path.Join("pdp/data-sets", dataSetID.String(), "pieces")
	resp, _, err := c.postJSON(ctx, urlPath, wire,
		http.StatusCreated, http.StatusOK, http.StatusAccepted)
	if err != nil {
		return nil, err
	}
	loc := resp.Header.Get("Location")
	hashHex := lastPathSegment(loc)
	if loc == "" || hashHex == "" {
		return nil, fmt.Errorf("%w: missing transaction status location", ErrLocationHeader)
	}
	if !strings.HasPrefix(hashHex, "0x") {
		hashHex = "0x" + hashHex
	}
	if !common.IsHexHash(hashHex) {
		return nil, fmt.Errorf("%w: invalid transaction hash in Location", ErrLocationHeader)
	}
	tx := common.HexToHash(hashHex)
	if tx == (common.Hash{}) {
		return nil, fmt.Errorf("%w: zero transaction hash in Location", ErrLocationHeader)
	}
	statusURL, err := c.resolveStatusURL(loc)
	if err != nil {
		return nil, fmt.Errorf("pdp.AddPieces: resolve status URL: %w", err)
	}
	return &AddPiecesResult{TxHash: tx, StatusURL: statusURL}, nil
}

// AddPiecesStatus mirrors GET /pdp/data-sets/{id}/pieces/added/{txHash}.
// TxStatus reports the provider's wire status. Callers should use the error
// returned by GetAddPiecesStatus instead of interpreting it directly.
// Providers report pending, confirmed, failed, or reorged; rejected is also
// accepted for compatibility.
type AddPiecesStatus struct {
	TxHash          common.Hash  `json:"-"`
	ConfirmedTxHash common.Hash  `json:"-"`
	TxStatus        string       `json:"txStatus"`
	DataSetID       types.BigInt `json:"-"`
	// PieceCount may be zero while processing is pending or after rejection. A
	// successful response reports the full number of add entries.
	PieceCount        int            `json:"pieceCount"`
	AddMessageOK      *bool          `json:"addMessageOk"`
	PiecesAdded       bool           `json:"piecesAdded"`
	ConfirmedPieceIDs []types.BigInt `json:"-"`
}

type rawAddPiecesStatus struct {
	TxHash            string        `json:"txHash"`
	ConfirmedTxHash   string        `json:"confirmedTxHash,omitempty"`
	TxStatus          string        `json:"txStatus"`
	DataSetID         json.Number   `json:"dataSetId"`
	PieceCount        int           `json:"pieceCount"`
	AddMessageOK      *bool         `json:"addMessageOk"`
	PiecesAdded       bool          `json:"piecesAdded"`
	ConfirmedPieceIDs []json.Number `json:"confirmedPieceIds,omitempty"`
}

// GetAddPiecesStatus polls the status URL once. Providers may return either
// HTTP 200 or 202 with the same JSON body shape.
func (c *Client) GetAddPiecesStatus(ctx context.Context, statusURL string) (*AddPiecesStatus, error) {
	if statusURL == "" {
		return nil, fmt.Errorf("pdp.GetAddPiecesStatus: %w: empty statusURL", ErrStatusURLOrigin)
	}
	body, err := c.getStatusBody(ctx, "pdp.GetAddPiecesStatus", statusURL, http.StatusOK, http.StatusAccepted)
	if err != nil {
		return nil, err
	}
	var raw rawAddPiecesStatus
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("pdp.GetAddPiecesStatus: decode: %w", err)
	}
	txHash, err := parseRequiredStatusHash("pdp.GetAddPiecesStatus", "txHash", raw.TxHash)
	if err != nil {
		return nil, err
	}
	confirmedTxHash, err := parseOptionalStatusHash("pdp.GetAddPiecesStatus", "confirmedTxHash", raw.ConfirmedTxHash)
	if err != nil {
		return nil, err
	}
	out := &AddPiecesStatus{
		TxHash:          txHash,
		ConfirmedTxHash: confirmedTxHash,
		TxStatus:        raw.TxStatus,
		PieceCount:      raw.PieceCount,
		AddMessageOK:    raw.AddMessageOK,
		PiecesAdded:     raw.PiecesAdded,
	}
	if raw.DataSetID != "" {
		id, err := parseBigIntNumber("pdp.GetAddPiecesStatus", "dataSetId", raw.DataSetID)
		if err != nil {
			return nil, err
		}
		out.DataSetID = id
	}
	for _, n := range raw.ConfirmedPieceIDs {
		id, err := parseBigIntNumber("pdp.GetAddPiecesStatus", "confirmedPieceId", n)
		if err != nil {
			return nil, err
		}
		out.ConfirmedPieceIDs = append(out.ConfirmedPieceIDs, id)
	}
	disposition, err := classifyAddPiecesStatus("pdp.GetAddPiecesStatus", out)
	if err != nil {
		return out, err
	}
	if disposition == transactionRejected {
		return out, ErrTxRejected
	}
	return out, nil
}

// WaitForPiecesAdded polls until the add-pieces tx is confirmed with
// piecesAdded=true or rejected. pollInterval defaults to 4s.
func (c *Client) WaitForPiecesAdded(ctx context.Context, statusURL string, pollInterval time.Duration) (*AddPiecesStatus, error) {
	if pollInterval <= 0 {
		pollInterval = 4 * time.Second
	}
	for {
		status, err := c.GetAddPiecesStatus(ctx, statusURL)
		if err != nil {
			return status, err
		}
		disposition, err := classifyAddPiecesStatus("pdp.WaitForPiecesAdded", status)
		if err != nil {
			return status, err
		}
		if disposition == transactionConfirmed {
			return status, nil
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
func (c *Client) SchedulePieceDeletion(ctx context.Context, dataSetID, pieceID types.BigInt, extraData []byte) (common.Hash, error) {
	if len(extraData) == 0 {
		return common.Hash{}, errors.New("pdp.SchedulePieceDeletion: empty extraData")
	}
	payload := struct {
		ExtraData string `json:"extraData"`
	}{ExtraData: "0x" + hex.EncodeToString(extraData)}

	var out struct {
		TxHash string `json:"txHash"`
	}
	urlPath := path.Join("pdp/data-sets", dataSetID.String(), "pieces", pieceID.String())
	if err := c.deleteJSON(ctx, urlPath, payload, &out); err != nil {
		return common.Hash{}, err
	}
	h := common.HexToHash(out.TxHash)
	if h == (common.Hash{}) {
		return common.Hash{}, fmt.Errorf("pdp.SchedulePieceDeletion: empty txHash in response")
	}
	return h, nil
}

func validateAddPiecesBatch(op string, count int) error {
	if count == 0 {
		return fmt.Errorf("%s: no pieces provided", op)
	}
	if count > MaxAddPiecesBatchSize {
		return fmt.Errorf("%s: %w: got %d, max %d", op, ErrTooManyPieces, count, MaxAddPiecesBatchSize)
	}
	return nil
}

func validateAddPieceInputs(op string, pieces []AddPieceInput) error {
	if err := validateAddPiecesBatch(op, len(pieces)); err != nil {
		return err
	}
	seen := make(map[string]int, len(pieces))
	for i, input := range pieces {
		if !input.PieceCID.Defined() {
			return fmt.Errorf("%s: undefined pieceCID at index %d", op, i)
		}
		key := canonicalPieceCIDKey(input.PieceCID)
		if first, ok := seen[key]; ok {
			return fmt.Errorf("%s: duplicate pieceCID at indexes %d and %d", op, first, i)
		}
		seen[key] = i
	}
	return nil
}

func canonicalPieceCIDKey(pieceCID cid.Cid) string {
	if info, err := piece.ParseV2(pieceCID); err == nil {
		return info.CIDv1.KeyString()
	}
	return pieceCID.KeyString()
}
