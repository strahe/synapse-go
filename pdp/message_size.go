package pdp

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const addPiecesMethodSelectorSize = 4

var addPiecesCalldataArguments = mustAddPiecesCalldataArguments()

type addPiecesCalldataPiece struct {
	Data []byte
}

// EstimateAddPiecesMessageSize returns the encoded PDPVerifier.addPieces
// calldata size in bytes, including the four-byte method selector. It does not
// enforce the provider's piece-count or message-size limits.
func EstimateAddPiecesMessageSize(pieces []AddPieceInput, extraData []byte) (int, error) {
	pieceData := make([]addPiecesCalldataPiece, len(pieces))
	for i, piece := range pieces {
		if !piece.PieceCID.Defined() {
			return 0, fmt.Errorf("pdp.EstimateAddPiecesMessageSize: undefined pieceCID at index %d", i)
		}
		pieceData[i] = addPiecesCalldataPiece{Data: piece.PieceCID.Bytes()}
	}
	encoded, err := addPiecesCalldataArguments.Pack(new(big.Int), common.Address{}, pieceData, extraData)
	if err != nil {
		return 0, fmt.Errorf("pdp.EstimateAddPiecesMessageSize: encode addPieces calldata: %w", err)
	}
	return addPiecesMethodSelectorSize + len(encoded), nil
}

func validateAddPiecesMessageSize(op string, pieces []AddPieceInput, extraData []byte) error {
	size, err := EstimateAddPiecesMessageSize(pieces, extraData)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	if size > MaxAddPiecesMessageSize {
		return fmt.Errorf("%s: %w", op, &AddPiecesMessageTooLargeError{
			Size: size,
			Max:  MaxAddPiecesMessageSize,
		})
	}
	return nil
}

func mustAddPiecesCalldataArguments() abi.Arguments {
	types := make([]abi.Type, 4)
	var err error
	types[0], err = abi.NewType("uint256", "", nil)
	if err != nil {
		panic("pdp: parse addPieces setId ABI type: " + err.Error())
	}
	types[1], err = abi.NewType("address", "", nil)
	if err != nil {
		panic("pdp: parse addPieces listenerAddr ABI type: " + err.Error())
	}
	types[2], err = abi.NewType("tuple[]", "", []abi.ArgumentMarshaling{{Name: "data", Type: "bytes"}})
	if err != nil {
		panic("pdp: parse addPieces pieceData ABI type: " + err.Error())
	}
	types[3], err = abi.NewType("bytes", "", nil)
	if err != nil {
		panic("pdp: parse addPieces extraData ABI type: " + err.Error())
	}
	arguments := make(abi.Arguments, len(types))
	for i, typ := range types {
		arguments[i] = abi.Argument{Type: typ}
	}
	return arguments
}
