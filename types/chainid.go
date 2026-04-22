package types

import "math/big"

// ChainID is an EIP-155 chain identifier exposed at the SDK public boundary
// instead of the bare *big.Int used by go-ethereum. The newtype keeps callers
// honest about units (raw uint256 chain ids fit easily in int64 for every
// deployed EVM chain) and avoids silently accepting mutable *big.Int pointers
// across package boundaries.
//
// The zero value is not a valid chain id; use IsValid to check.
type ChainID int64

// BigInt returns a freshly-allocated *big.Int for consumers (contract
// bindings, EIP-712 domain separators) that still require the legacy
// representation. The returned pointer is owned by the caller.
func (c ChainID) BigInt() *big.Int {
	return big.NewInt(int64(c))
}

// Int64 returns the chain id as an int64.
func (c ChainID) Int64() int64 {
	return int64(c)
}

// IsValid reports whether the chain id is a positive EIP-155 identifier.
func (c ChainID) IsValid() bool {
	return c > 0
}
