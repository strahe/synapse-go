// Package signer provides signing abstractions for Filecoin and Ethereum
// transactions.
//
// This is a leaf package. It defines the [Signer] and [EVMSigner] interfaces
// and provides concrete implementations:
//
//   - [Secp256k1Signer]: dual-protocol signer using a single secp256k1 key
//     for both Filecoin (blake2b) and Ethereum (keccak256) signing.
//   - [BLSSigner]: Filecoin-only BLS signature support.
//
// Consumers should accept the interface types; this package returns concrete
// types per Go convention.
package signer
