package typeddata

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/ipfs/go-cid"
)

// MetadataEntry represents a key-value metadata pair.
type MetadataEntry struct {
	Key   string
	Value string
}

// Signature holds the components of an EIP-712 signature.
type Signature struct {
	V uint8
	R [32]byte
	S [32]byte
}

// Types contains all EIP-712 type definitions for the FWSS protocol.
var Types = apitypes.Types{
	"EIP712Domain": {
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	},
	"MetadataEntry": {
		{Name: "key", Type: "string"},
		{Name: "value", Type: "string"},
	},
	"CreateDataSet": {
		{Name: "clientDataSetId", Type: "uint256"},
		{Name: "payee", Type: "address"},
		{Name: "metadata", Type: "MetadataEntry[]"},
	},
	"Cid": {
		{Name: "data", Type: "bytes"},
	},
	"PieceMetadata": {
		{Name: "pieceIndex", Type: "uint256"},
		{Name: "metadata", Type: "MetadataEntry[]"},
	},
	"AddPieces": {
		{Name: "clientDataSetId", Type: "uint256"},
		{Name: "nonce", Type: "uint256"},
		{Name: "pieceData", Type: "Cid[]"},
		{Name: "pieceMetadata", Type: "PieceMetadata[]"},
	},
	"SchedulePieceRemovals": {
		{Name: "clientDataSetId", Type: "uint256"},
		{Name: "pieceIds", Type: "uint256[]"},
	},
	"DeleteDataSet": {
		{Name: "clientDataSetId", Type: "uint256"},
	},
}

// CreateDataSetMessage builds the EIP-712 message for dataset creation.
func CreateDataSetMessage(clientDataSetID *big.Int, payee common.Address, metadata []MetadataEntry) apitypes.TypedDataMessage {
	metadataArray := make([]interface{}, len(metadata))
	for i, m := range metadata {
		metadataArray[i] = map[string]interface{}{
			"key":   m.Key,
			"value": m.Value,
		}
	}

	return apitypes.TypedDataMessage{
		"clientDataSetId": (*math.HexOrDecimal256)(clientDataSetID),
		"payee":           payee.Hex(),
		"metadata":        metadataArray,
	}
}

// AddPiecesMessage builds the EIP-712 message for adding pieces.
func AddPiecesMessage(clientDataSetID, nonce *big.Int, pieceCIDs []cid.Cid, metadata [][]MetadataEntry) (apitypes.TypedDataMessage, error) {
	if len(metadata) == 0 {
		metadata = make([][]MetadataEntry, len(pieceCIDs))
		for i := range metadata {
			metadata[i] = []MetadataEntry{}
		}
	}
	if len(metadata) != len(pieceCIDs) {
		return nil, fmt.Errorf("typeddata.AddPiecesMessage: metadata length (%d) must match pieceCIDs length (%d)", len(metadata), len(pieceCIDs))
	}

	pieceData := make([]interface{}, len(pieceCIDs))
	for i, c := range pieceCIDs {
		pieceData[i] = map[string]interface{}{
			"data": c.Bytes(),
		}
	}

	pieceMetadata := make([]interface{}, len(pieceCIDs))
	for i, meta := range metadata {
		metadataArray := make([]interface{}, len(meta))
		for j, m := range meta {
			metadataArray[j] = map[string]interface{}{
				"key":   m.Key,
				"value": m.Value,
			}
		}
		pieceMetadata[i] = map[string]interface{}{
			"pieceIndex": (*math.HexOrDecimal256)(big.NewInt(int64(i))),
			"metadata":   metadataArray,
		}
	}

	return apitypes.TypedDataMessage{
		"clientDataSetId": (*math.HexOrDecimal256)(clientDataSetID),
		"nonce":           (*math.HexOrDecimal256)(nonce),
		"pieceData":       pieceData,
		"pieceMetadata":   pieceMetadata,
	}, nil
}

// DeleteDataSetMessage builds the EIP-712 message for dataset deletion.
func DeleteDataSetMessage(clientDataSetID *big.Int) apitypes.TypedDataMessage {
	return apitypes.TypedDataMessage{
		"clientDataSetId": (*math.HexOrDecimal256)(clientDataSetID),
	}
}

// SchedulePieceRemovalsMessage builds the EIP-712 message for scheduling piece removals.
func SchedulePieceRemovalsMessage(clientDataSetID *big.Int, pieceIDs []*big.Int) apitypes.TypedDataMessage {
	pieceIDsArray := make([]interface{}, len(pieceIDs))
	for i, id := range pieceIDs {
		pieceIDsArray[i] = (*math.HexOrDecimal256)(id)
	}

	return apitypes.TypedDataMessage{
		"clientDataSetId": (*math.HexOrDecimal256)(clientDataSetID),
		"pieceIds":        pieceIDsArray,
	}
}
