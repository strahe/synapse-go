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
// (WriteResult, BigInt, ...) without cross-importing each other's service
// package.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Contract uint256
// identifiers are exposed as BigInt; field and parameter names carry the
// domain meaning.
package types
