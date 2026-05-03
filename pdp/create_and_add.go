package pdp

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type createAndAddPiecesRequest struct {
	RecordKeeper string                  `json:"recordKeeper"`
	ExtraData    string                  `json:"extraData"`
	Pieces       []addPiecesRequestPiece `json:"pieces"`
}

// CreateDataSetAndAddPieces calls POST /pdp/data-sets/create-and-add.
//
// The provider creates a dataset and immediately submits the add-pieces
// transaction using the caller-provided EIP-712 signed extraData for the
// combined create+add flow.
func (c *Client) CreateDataSetAndAddPieces(
	ctx context.Context,
	recordKeeper common.Address,
	pieces []AddPieceInput,
	extraData []byte,
) (*CreateDataSetResult, error) {
	if (recordKeeper == common.Address{}) {
		return nil, errors.New("pdp.CreateDataSetAndAddPieces: zero recordKeeper")
	}
	if len(pieces) == 0 {
		return nil, errors.New("pdp.CreateDataSetAndAddPieces: no pieces provided")
	}
	if len(extraData) == 0 {
		return nil, errors.New("pdp.CreateDataSetAndAddPieces: empty extraData")
	}

	wire := createAndAddPiecesRequest{
		RecordKeeper: recordKeeper.Hex(),
		ExtraData:    "0x" + hex.EncodeToString(extraData),
	}
	for _, p := range pieces {
		if !p.PieceCID.Defined() {
			return nil, errors.New("pdp.CreateDataSetAndAddPieces: undefined pieceCID in input")
		}
		s := p.PieceCID.String()
		wire.Pieces = append(wire.Pieces, addPiecesRequestPiece{
			PieceCID:  s,
			SubPieces: []addPiecesSubPiece{{SubPieceCID: s}},
		})
	}

	resp, body, err := c.postJSON(ctx, "pdp/data-sets/create-and-add", wire,
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
		return nil, fmt.Errorf("pdp.CreateDataSetAndAddPieces: resolve status URL: %w", err)
	}
	return &CreateDataSetResult{TxHash: tx, StatusURL: statusURL.String()}, nil
}

// WaitForCreateDataSetAndAddPieces first waits for dataset creation to confirm,
// then polls the add-pieces status endpoint for the same transaction hash.
func (c *Client) WaitForCreateDataSetAndAddPieces(
	ctx context.Context,
	statusURL string,
	pollInterval time.Duration,
) (*AddPiecesStatus, error) {
	createStatus, err := c.WaitForDataSetCreated(ctx, statusURL, pollInterval)
	if err != nil {
		return nil, err
	}
	if createStatus.DataSetID == nil {
		return nil, errors.New("pdp.WaitForCreateDataSetAndAddPieces: missing dataSetId in create status")
	}
	if createStatus.CreateMessageHash == (common.Hash{}) {
		return nil, errors.New("pdp.WaitForCreateDataSetAndAddPieces: server returned zero CreateMessageHash")
	}

	addStatusURL, err := c.resolve(path.Join(
		"pdp/data-sets",
		createStatus.DataSetID.String(),
		"pieces/added",
		createStatus.CreateMessageHash.Hex(),
	))
	if err != nil {
		return nil, fmt.Errorf("pdp.WaitForCreateDataSetAndAddPieces: resolve add status URL: %w", err)
	}
	return c.WaitForPiecesAdded(ctx, addStatusURL.String(), pollInterval)
}
