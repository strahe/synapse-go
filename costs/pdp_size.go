package costs

import (
	"fmt"
	"math/big"
)

// Partial leaves are charged per piece; byte conversion happens after summing
// leaves so that rounding is applied only once to the complete dataset.
func pieceSizesToLeafCount(pieceSizes []uint64) *big.Int {
	leaves := new(big.Int)
	for _, size := range pieceSizes {
		pieceLeaves := new(big.Int).SetUint64(size)
		pieceLeaves.Mul(pieceLeaves, big.NewInt(4))
		pieceLeaves.Add(pieceLeaves, big.NewInt(126))
		pieceLeaves.Div(pieceLeaves, big.NewInt(127))
		leaves.Add(leaves, pieceLeaves)
	}
	return leaves
}

func leafCountToBillableBytes(leaves *big.Int) *big.Int {
	bytes := new(big.Int).Mul(leaves, big.NewInt(127))
	return bytes.Div(bytes, big.NewInt(4))
}

func validatePieceSizes(pieceSizes []uint64) error {
	if len(pieceSizes) == 0 {
		return fmt.Errorf("%w: pieceSizes must not be empty", ErrInvalidArgument)
	}
	for i, size := range pieceSizes {
		if size == 0 {
			return fmt.Errorf("%w: pieceSizes[%d] must be greater than zero", ErrInvalidArgument, i)
		}
	}
	return nil
}

func resolveCurrentLeafCount(isNew bool, leaves *big.Int) (*big.Int, error) {
	if isNew {
		return new(big.Int), nil
	}
	if leaves == nil {
		return nil, fmt.Errorf("%w: CurrentDataSetLeafCount is required for an existing dataset", ErrInvalidArgument)
	}
	if leaves.Sign() < 0 {
		return nil, fmt.Errorf("%w: CurrentDataSetLeafCount must be non-negative", ErrInvalidArgument)
	}
	return new(big.Int).Set(leaves), nil
}
