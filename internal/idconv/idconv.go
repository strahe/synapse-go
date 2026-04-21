// Package idconv converts between the SDK's typed uint64 identifiers
// (types.DataSetID, types.ProviderID, ...) and the *big.Int values used by
// generated contract bindings.
//
// All conversions go through this package so we have a single, audited
// location that:
//
//   - Uses *big.Int.SetUint64 / IsUint64 / Uint64 (never int64, which would
//     silently wrap for IDs above math.MaxInt64).
//   - Rejects nil and out-of-range values with clear errors instead of
//     panicking or truncating.
package idconv

import (
	"fmt"
	"math/big"
)

// Big returns a fresh *big.Int holding id, using SetUint64 so the full
// uint64 range round-trips without relying on int64.
func Big[T ~uint64](id T) *big.Int {
	return new(big.Int).SetUint64(uint64(id))
}

// BigSlice applies Big to every element of ids.
func BigSlice[T ~uint64](ids []T) []*big.Int {
	if ids == nil {
		return nil
	}
	out := make([]*big.Int, len(ids))
	for i, id := range ids {
		out[i] = Big(id)
	}
	return out
}

// Safe converts v into a typed identifier, returning an error when v is
// nil, negative, or exceeds math.MaxUint64. name is used as the error
// prefix so the caller site is always recoverable.
func Safe[T ~uint64](name string, v *big.Int) (T, error) {
	var zero T
	if v == nil {
		return zero, fmt.Errorf("%s: nil", name)
	}
	if v.Sign() < 0 {
		return zero, fmt.Errorf("%s: negative: %s", name, v.String())
	}
	if !v.IsUint64() {
		return zero, fmt.Errorf("%s: exceeds uint64: %s", name, v.String())
	}
	return T(v.Uint64()), nil
}

// SafeSlice applies Safe to every element of values; the returned slice
// has the same length as values. name is used as the element error
// prefix; each element's index is appended for context.
func SafeSlice[T ~uint64](name string, values []*big.Int) ([]T, error) {
	if values == nil {
		return nil, nil
	}
	out := make([]T, len(values))
	for i, v := range values {
		got, err := Safe[T](fmt.Sprintf("%s[%d]", name, i), v)
		if err != nil {
			return nil, err
		}
		out[i] = got
	}
	return out, nil
}
