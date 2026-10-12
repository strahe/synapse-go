package txutil

import (
	"errors"
	"math/big"
)

// ValidateUint256 checks that v is non-nil and within [0, 2^256-1].
// It does not modify v.
func ValidateUint256(v *big.Int) error {
	if v == nil {
		return errors.New("nil")
	}
	if v.Sign() < 0 {
		return errors.New("negative")
	}
	if v.BitLen() > 256 {
		return errors.New("exceeds uint256")
	}
	return nil
}
