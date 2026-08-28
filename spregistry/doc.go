// Package spregistry provides the Storage Provider Registry service.
//
// It queries the ServiceProviderRegistry contract to discover storage
// providers and their capabilities and endpoints. Endorsement membership is
// read from an independently deployed ProviderIdSet when configured. When
// constructed with a Signer and Backend, the service also exposes the
// state-changing surface needed to register, update, or remove a provider and
// manage its PDP product.
//
// Endorsed and approved are separate on-chain sets. Endorsed membership comes
// from ProviderIdSet and is used by storage's default primary-selection policy;
// approved membership comes from FWSS and defines the broader automatic
// selection pool. These labels describe membership, not an SDK quality rating.
//
// # Read surface
//
// The read surface is usable with any bind.ContractCaller (including a
// plain *ethclient.Client) and returns decoded Go types for providers,
// PDP offerings, endorsement lists, and ID ↔ address mappings.
//
// # Write surface
//
// When Options.Backend, Options.Signer, and Options.ChainID are supplied, the
// following state-changing methods become available. A nonce coordinator is
// created automatically for standalone use; the root synapse Client injects a
// shared coordinator across all write-capable services.
//
//   - RegisterProvider: declares the caller as a provider and, in the
//     same transaction, registers a PDP product. Reads REGISTRATION_FEE
//     from the contract unless WithValue overrides.
//   - UpdateProviderInfo: changes the caller's display name / description.
//   - RemoveProvider: deregisters the caller.
//   - AddPDPProduct / UpdatePDPProduct: registers or replaces the PDP
//     product's capability set.
//   - RemoveProduct: removes the caller's product for the given type.
//
// All write methods validate and encode the PDP offering before broadcast,
// return a WriteResult carrying the transaction hash, and (under WithWait)
// block for the receipt; a reverted transaction surfaces as ErrTxFailed
// with the receipt preserved on the WriteResult for inspection.
//
// Callers that build Service read-only receive ErrWriteNotConfigured for
// any write method.
//
// Errors are returned as wrapped sentinels. Use errors.Is to check:
//
//   - ErrNotFound: returned when the queried provider does not exist.
//     GetProviderIDByAddress is an exception: it returns a zero ID (check
//     with id.IsZero()) for unknown addresses, mirroring the contract convention.
//   - ErrInvalidArgument: returned when required arguments are nil, zero,
//     or otherwise malformed.
//   - ErrInvalidOffering: returned by ValidatePDPOffering / write methods
//     when a PDP offering fails structural validation.
//   - ErrEndorsementsNotConfigured: returned by GetEndorsedProviderIDs when
//     Options.EndorsementsAddress was not configured.
//   - ErrWriteNotConfigured: returned by write methods when Service was
//     constructed without write dependencies.
//   - ErrTxFailed: returned by write methods when the broadcast
//     transaction reverts on-chain.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package spregistry
