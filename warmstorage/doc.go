// Package warmstorage provides the WarmStorage (FWSS) service for managing
// storage contracts, data sets, and service pricing.
//
// FWSS (Filecoin Warm Storage Service) is the canonical storage contract.
// The root synapse Client supplies the chain's known FWSS, StateView, and
// PDPVerifier addresses when constructing this service. Low-level callers
// that instantiate Service directly must provide those addresses explicitly.
//
// Key operations: data set management (including [Service.TerminateDataSet]
// for FWSS-initiated teardown), service price queries, approval management,
// and provider allocation.
//
// The root synapse Client wires WarmStorage together with the other
// write-capable services so transaction nonce allocation is coordinated for
// a shared signer. Standalone services create their own nonce coordinator
// when constructed with write dependencies.
//
// Errors are returned as wrapped sentinels. Use errors.Is to check:
//
//   - ErrNotFound: returned when a queried record (e.g. data set) does not
//     exist. Getter methods document which lookups can produce this.
//   - ErrInvalidArgument: returned when required arguments are nil, zero,
//     or otherwise malformed.
//   - ErrPDPVerifierNotConfigured: returned when a PDPVerifier-dependent read
//     is used without configuring a PDPVerifier address.
//   - ErrWriteNotConfigured: returned when a write method is used without
//     write dependencies.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package warmstorage
