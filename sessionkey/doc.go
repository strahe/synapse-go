// Package sessionkey provides SessionKeyRegistry authorization management on
// Filecoin EVM.
//
// Session keys are authorized by a root account with specific EIP-712
// permissions (e.g., CreateDataSet, AddPieces) and a time-bounded expiry. This
// package manages those on-chain authorizations and expiry checks; root Client
// storage operations use the signer configured on the Client.
//
// This is separate from the signer package because session keys represent
// a higher-level authorization concept, not just a signing primitive.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package sessionkey
