package costs

import (
	"fmt"
	"math/big"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/types"
)

const leavesPerFR32Block = (chain.MinUploadSize + 1) / chain.BytesPerLeaf

// PieceSizesToLeafCount returns the PDP leaf count added by pieces with the
// given raw sizes. Pass each piece's size: leaves are rounded per piece, so a
// total size gives a different result. Every size must be between
// chain.MinUploadSize and chain.MaxUploadSize; otherwise it returns
// ErrInvalidArgument.
func PieceSizesToLeafCount(pieceSizes []uint64) (*big.Int, error) {
	if err := validatePieceSizes(pieceSizes); err != nil {
		return nil, fmt.Errorf("costs.PieceSizesToLeafCount: %w", err)
	}
	return pieceSizesToLeafCount(pieceSizes), nil
}

// LeafCountToBillableBytes returns the billable size in bytes for a data set's
// total leaf count, as expected by [CalculateEffectiveRate]. Convert the total,
// current plus added, rather than individual pieces. A nil count is zero; a
// negative count returns ErrInvalidArgument.
func LeafCountToBillableBytes(leaves *big.Int) (*big.Int, error) {
	if leaves == nil {
		return new(big.Int), nil
	}
	if leaves.Sign() < 0 {
		return nil, fmt.Errorf("costs.LeafCountToBillableBytes: %w: leaf count must be non-negative", ErrInvalidArgument)
	}
	return leafCountToBillableBytes(leaves), nil
}

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

type resolvedDataSetCostState struct {
	leaves          *big.Int
	reserveBalance  *big.Int
	pendingPayments *big.Int
	pdpEndEpoch     types.Epoch
}

func resolveDataSetCostState(
	isNew bool,
	leaves *big.Int,
	reserveBalance *big.Int,
	pendingPayments *big.Int,
	pdpEndEpoch *types.Epoch,
) (resolvedDataSetCostState, error) {
	if isNew {
		return resolvedDataSetCostState{
			leaves:          new(big.Int),
			reserveBalance:  new(big.Int),
			pendingPayments: new(big.Int),
		}, nil
	}

	resolvedLeaves, err := resolveCurrentLeafCount(false, leaves)
	if err != nil {
		return resolvedDataSetCostState{}, err
	}
	if reserveBalance == nil {
		return resolvedDataSetCostState{}, fmt.Errorf("%w: CurrentLifecycleReserveBalance is required for an existing dataset", ErrInvalidArgument)
	}
	if reserveBalance.Sign() < 0 {
		return resolvedDataSetCostState{}, fmt.Errorf("%w: CurrentLifecycleReserveBalance must be non-negative", ErrInvalidArgument)
	}
	if pdpEndEpoch == nil {
		return resolvedDataSetCostState{}, fmt.Errorf("%w: PDPEndEpoch is required for an existing dataset", ErrInvalidArgument)
	}
	if *pdpEndEpoch != 0 {
		return resolvedDataSetCostState{}, &DataSetServiceTerminatedError{PDPEndEpoch: *pdpEndEpoch}
	}
	resolvedPending := copyBigOrDefault(pendingPayments, nil)
	if resolvedPending.Sign() < 0 {
		return resolvedDataSetCostState{}, fmt.Errorf("%w: PendingOneTimePayments must be non-negative", ErrInvalidArgument)
	}

	return resolvedDataSetCostState{
		leaves:          resolvedLeaves,
		reserveBalance:  new(big.Int).Set(reserveBalance),
		pendingPayments: resolvedPending,
		pdpEndEpoch:     *pdpEndEpoch,
	}, nil
}
