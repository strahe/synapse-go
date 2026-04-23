// Package sessionkey provides session key management for delegated signing
// via the SessionKeyRegistry contract on Filecoin EVM.
//
// Session keys allow applications to sign FWSS (Filecoin Warm Storage Service)
// operations without exposing the main wallet private key. A session key is
// authorized by the root account with specific EIP-712 permissions (e.g.,
// CreateDataSet, AddPieces) and a time-bounded expiry.
//
// This is separate from the signer package because session keys represent
// a higher-level authorization concept, not just a signing primitive.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Mirrors the
// TS SDK session-key package at
// synapse-sdk/packages/synapse-sdk/src/session-key.
package sessionkey
