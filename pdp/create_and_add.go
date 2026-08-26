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
// combined create+add flow. Piece CIDs must be unique within the request after
// PieceCIDv2-to-v1 normalization.
func (c *Client) CreateDataSetAndAddPieces(
	ctx context.Context,
	recordKeeper common.Address,
	pieces []AddPieceInput,
	extraData []byte,
) (*CreateDataSetResult, error) {
	if (recordKeeper == common.Address{}) {
		return nil, errors.New("pdp.CreateDataSetAndAddPieces: zero recordKeeper")
	}
	if err := validateAddPieceInputs("pdp.CreateDataSetAndAddPieces", pieces); err != nil {
		return nil, err
	}
	if len(extraData) == 0 {
		return nil, errors.New("pdp.CreateDataSetAndAddPieces: empty extraData")
	}

	wire := createAndAddPiecesRequest{
		RecordKeeper: recordKeeper.Hex(),
		ExtraData:    "0x" + hex.EncodeToString(extraData),
	}
	for _, p := range pieces {
		s := p.PieceCID.String()
		wire.Pieces = append(wire.Pieces, addPiecesRequestPiece{
			PieceCID:  s,
			SubPieces: []addPiecesSubPiece{{SubPieceCID: s}},
		})
	}

	resp, _, err := c.postJSON(ctx, "pdp/data-sets/create-and-add", wire,
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
		return nil, fmt.Errorf("pdp.CreateDataSetAndAddPieces: resolve status URL: %w", err)
	}
	return &CreateDataSetResult{TxHash: tx, StatusURL: statusURL}, nil
}

// CreateAndAddPiecesStatus contains the available status snapshots for a
// combined create-and-add submission. Add is nil until data-set creation has
// completed and the add-pieces endpoint can be queried.
type CreateAndAddPiecesStatus struct {
	Create *CreateDataSetStatus
	Add    *AddPiecesStatus
}

// GetCreateDataSetAndAddPiecesStatus checks a combined create-and-add
// submission once. It does not wait for either stage to advance.
func (c *Client) GetCreateDataSetAndAddPiecesStatus(
	ctx context.Context,
	statusURL string,
) (*CreateAndAddPiecesStatus, error) {
	out := &CreateAndAddPiecesStatus{}
	createStatus, err := c.GetDataSetCreationStatus(ctx, statusURL)
	out.Create = createStatus
	if err != nil {
		return out, err
	}
	disposition, err := classifyCreateDataSetStatus("pdp.GetCreateDataSetAndAddPiecesStatus", createStatus)
	if err != nil {
		return out, err
	}
	if disposition != transactionConfirmed {
		return out, nil
	}

	addStatusURL, err := c.resolveStatusURL(path.Join(
		"pdp/data-sets",
		createStatus.DataSetID.String(),
		"pieces/added",
		createStatus.CreateMessageHash.Hex(),
	))
	if err != nil {
		return out, fmt.Errorf("pdp.GetCreateDataSetAndAddPiecesStatus: resolve add status URL: %w", err)
	}
	addStatus, err := c.GetAddPiecesStatus(ctx, addStatusURL)
	out.Add = addStatus
	if httpErr, ok := errors.AsType[*HTTPError](err); ok && httpErr.StatusCode == http.StatusNotFound {
		// Data-set creation and add-record publication are not atomic. Until
		// the add record appears, the combined submission is still pending.
		return out, nil
	}
	if addStatus != nil {
		if addStatus.TxHash != createStatus.CreateMessageHash {
			return out, invalidStatusf("pdp.GetCreateDataSetAndAddPiecesStatus", "create and add transaction hashes differ")
		}
		if !addStatus.DataSetID.Equal(*createStatus.DataSetID) {
			return out, invalidStatusf("pdp.GetCreateDataSetAndAddPiecesStatus", "create and add dataSetIds differ")
		}
		if createStatus.ConfirmedTxHash != (common.Hash{}) &&
			addStatus.ConfirmedTxHash != (common.Hash{}) &&
			createStatus.ConfirmedTxHash != addStatus.ConfirmedTxHash {
			return out, invalidStatusf("pdp.GetCreateDataSetAndAddPiecesStatus", "create and add confirmed transaction hashes differ")
		}
	}
	return out, err
}

// WaitForCreateDataSetAndAddPieces polls the one-shot combined status check
// until both stages are confirmed or the transaction is rejected.
func (c *Client) WaitForCreateDataSetAndAddPieces(
	ctx context.Context,
	statusURL string,
	pollInterval time.Duration,
) (*AddPiecesStatus, error) {
	if pollInterval <= 0 {
		pollInterval = 4 * time.Second
	}
	for {
		status, err := c.GetCreateDataSetAndAddPiecesStatus(ctx, statusURL)
		if err != nil {
			return status.Add, err
		}
		if status.Add != nil {
			disposition, err := classifyAddPiecesStatus("pdp.WaitForCreateDataSetAndAddPieces", status.Add)
			if err != nil {
				return status.Add, err
			}
			if disposition == transactionConfirmed {
				return status.Add, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
