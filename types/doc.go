// Package types holds shared SDK domain types used across service packages.
//
// Boundary discipline:
//
//   - Only pure data types (structs, named primitives) and sentinel errors
//     belong here.
//   - No business logic, no helpers, no service glue.
//   - Imports stay minimal and must not include service packages, because
//     service imports from types must remain acyclic.
//
// This package exists so every service can share the same vocabulary
// (WriteResult, DataSetID, ProviderID, ...) without cross-importing each
// other's service package.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Identifier
// widths follow the on-chain ABI; bounded SDK identifiers use uint64,
// while protocol-width identifiers (e.g. ClientDataSetID) keep the full
// uint256 *big.Int width for contract interoperability.
package types
