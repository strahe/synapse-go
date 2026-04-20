// Package warmstorage provides the WarmStorage (FWSS) service for managing
// storage contracts, data sets, and service pricing.
//
// FWSS (Filecoin Warm Storage Service) is the root-of-trust contract.
// All other contract addresses (PDPVerifier, SPRegistry, Payments) are
// auto-discovered from FWSS using Multicall3.
//
// Key operations: data set management, service price queries, approval
// management, and provider allocation.
//
// Errors are returned as wrapped sentinels. Use errors.Is to check:
//
//   - ErrNotFound: returned when a queried record (e.g. data set) does not
//     exist. Getter methods document which lookups can produce this.
//   - ErrInvalidArgument: returned when required arguments are nil, zero,
//     or otherwise malformed.
package warmstorage
