package costs

import (
	"fmt"
	"math/big"

	"github.com/strahe/synapse-go/chain"
)

const leavesPerFR32Block = (chain.MinUploadSize + 1) / chain.BytesPerLeaf

// Partial leaves are charged per piece; byte conversion happens after summing
// leaves so that rounding is applied only once to the complete dataset.
func pieceSizesToLeafCount(pieceSizes []uint64) *big.Int {
	leaves := new(big.Int)
	var pieceLeaves big.Int
	for _, size := range pieceSizes {
		fullBlocks := size / chain.MinUploadSize
		partialBlock := size % chain.MinUploadSize
		partialLeaves := (partialBlock*leavesPerFR32Block + chain.MinUploadSize - 1) / chain.MinUploadSize
		pieceLeafCount := fullBlocks*leavesPerFR32Block + partialLeaves
		leaves.Add(leaves, pieceLeaves.SetUint64(pieceLeafCount))
	}
	return leaves
}

func leafCountToBillableBytes(leaves *big.Int) *big.Int {
	bytes := new(big.Int).Mul(leaves, big.NewInt(chain.MinUploadSize))
	return bytes.Div(bytes, big.NewInt(leavesPerFR32Block))
}

func validatePieceSizes(pieceSizes []uint64) error {
	if len(pieceSizes) == 0 {
		return fmt.Errorf("%w: pieceSizes must not be empty", ErrInvalidArgument)
	}
	for i, size := range pieceSizes {
		if size < chain.MinUploadSize || size > chain.MaxUploadSize {
			return fmt.Errorf(
				"%w: pieceSizes[%d] must be between %d and %d bytes",
				ErrInvalidArgument, i, chain.MinUploadSize, chain.MaxUploadSize,
			)
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
