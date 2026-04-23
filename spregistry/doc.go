// Package spregistry provides the Storage Provider Registry service.
//
// It queries the ServiceProviderRegistry contract to discover storage
// providers, their capabilities, endpoints, and endorsement status.
//
// Provider types:
//   - Endorsed: curated, high-quality providers (used as primary).
//   - Approved: automated QA-checked providers (used as secondary).
//
// Errors are returned as wrapped sentinels. Use errors.Is to check:
//
//   - ErrNotFound: returned when the queried provider does not exist.
//     GetProviderIDByAddress is an exception: it returns id.Sign() == 0
//     (no error) for unknown addresses, mirroring the contract convention.
//   - ErrInvalidArgument: returned when required arguments are nil, zero,
//     or otherwise malformed.
//   - ErrInvalidOffering: returned by ValidatePDPOffering when a PDP
//     offering fails structural validation.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Mirrors the
// TS SDK package at synapse-sdk/packages/synapse-sdk/src/sp-registry.
package spregistry
