// Package signer provides signing abstractions for Filecoin and Ethereum
// transactions.
//
// This is a leaf package. It defines separate capabilities for native
// Filecoin messages, Ethereum transactions, and Storage EIP-712
// authorizations:
//
//   - [Signer] signs native Filecoin messages.
//   - [EVMSigner] adds Ethereum transaction signing.
//   - [HashSigner] signs pre-computed 32-byte digests.
//   - [StorageSigner] combines an EVM address with HashSigner without requiring
//     native message or transaction signing.
//
// The package provides concrete implementations:
//
//   - [Secp256k1Signer]: dual-protocol signer using a single secp256k1 key
//     for both Filecoin (blake2b) and Ethereum (keccak256) signing.
//   - [BLSSigner]: Filecoin-only BLS signature support.
//
// Consumers should accept the interface types; this package returns concrete
// types per Go convention.
//
// # Raw hash signing
//
// HashSigner and StorageSigner are extension points for KMS/HSM-backed keys,
// remote signers, and decorators. A HashSigner receives only a digest, so it
// cannot reconstruct or validate the original EIP-712 domain and message.
// Implementations should use a dedicated authorization key and restrict access
// to trusted callers. Do not expose HashSigner as a general-purpose signing
// oracle.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package signer
