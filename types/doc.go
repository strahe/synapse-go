// Package types holds shared SDK domain types used across service packages.
//
// Boundary discipline:
//
//   - Only pure data types (structs, named primitives) and sentinel errors
//     belong here.
//   - No business logic, no helpers, no service glue. Helpers that are used
//     across packages live under internal/ (for example, internal/idconv
//     holds *big.Int <-> named ID conversion helpers).
//   - Imports are restricted to the Go standard library, go-ethereum
//     packages (common, core/types) and internal/txutil (for ErrTxFailed).
//     Importing any service package from types creates cycle risk and is
//     forbidden.
//
// This package exists so every service can share the same vocabulary
// (WriteResult, DataSetID, ProviderID, ...) without cross-importing each
// other's service package.
package types
