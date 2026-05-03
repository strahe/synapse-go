// Package idconv converts between SDK uint256 identifiers and generated
// contract binding *big.Int values.
package idconv

import (
	"fmt"
	"math/big"

	"github.com/strahe/synapse-go/types"
)

// FromBig validates v as uint256 and returns it as a types.BigInt.
func FromBig(name string, v *big.Int) (types.BigInt, error) {
	id, err := types.BigIntFromBig(v)
	if err != nil {
		return types.BigInt{}, fmt.Errorf("%s: %w", name, err)
	}
	return id, nil
}

// FromBigSlice applies FromBig to every value.
func FromBigSlice(name string, values []*big.Int) ([]types.BigInt, error) {
	if values == nil {
		return nil, nil
	}
	out := make([]types.BigInt, len(values))
	for i, v := range values {
		got, err := FromBig(fmt.Sprintf("%s[%d]", name, i), v)
		if err != nil {
			return nil, err
		}
		out[i] = got
	}
	return out, nil
}

// BigSlice returns defensive *big.Int copies for contract calls.
func BigSlice(values []types.BigInt) []*big.Int {
	if values == nil {
		return nil
	}
	out := make([]*big.Int, len(values))
	for i, id := range values {
		out[i] = id.Big()
	}
	return out
}

// Key returns a stable binary map key for an identifier.
func Key(id types.BigInt) string {
	buf := id.Bytes32()
	return string(buf[:])
}
