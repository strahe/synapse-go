// Package warmstorage provides the WarmStorage (FWSS) service for managing
// storage contracts, data sets, and service pricing.
//
// FWSS (Filecoin Warm Storage Service) is the root-of-trust contract.
// All other contract addresses (PDPVerifier, SPRegistry, Payments) are
// auto-discovered from FWSS using Multicall3.
//
// Key operations: data set management (including [Service.TerminateDataSet]
// for FWSS-initiated teardown), service price queries, approval management,
// and provider allocation.
//
// When multiple write-capable services share the same signer / EOA, pass the
// same txutil.NonceManager to each constructor so nonce allocation stays
// serialized across services.
//
// Errors are returned as wrapped sentinels. Use errors.Is to check:
//
//   - ErrNotFound: returned when a queried record (e.g. data set) does not
//     exist. Getter methods document which lookups can produce this.
//   - ErrInvalidArgument: returned when required arguments are nil, zero,
//     or otherwise malformed.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Mirrors the
// TS SDK package at synapse-sdk/packages/synapse-sdk/src/warm-storage.
package warmstorage
