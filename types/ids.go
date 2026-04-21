package types

import "math/big"

// DataSetID identifies a warm-storage data set. On-chain it is stored as a
// uint256, but the Warm Storage contract bounds it to the uint64-safe
// range; SDK callers work with the typed value directly.
type DataSetID uint64

// ProviderID identifies a storage provider registered with the SP
// registry.
type ProviderID uint64

// RailID identifies a payment rail in the payments contract.
type RailID uint64

// PieceID identifies a piece within a data set.
type PieceID uint64

// ClientDataSetID is a caller-assigned identifier that scopes EIP-712
// signatures to a specific client/data-set pair. It is a full uint256 in the
// protocol and reference SDK, so the Go SDK keeps the exact on-chain value.
//
// The pointed-to value must be treated as immutable once shared.
type ClientDataSetID = *big.Int

// Epoch is a Filecoin chain epoch number. Zero is the valid "indefinite"
// sentinel in several warm-storage fields.
type Epoch uint64
