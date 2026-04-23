// Package spregistry provides the Storage Provider Registry service.
//
// It queries the ServiceProviderRegistry contract to discover storage
// providers, their capabilities, endpoints, and endorsement status, and
// (when constructed with a Signer + Backend) exposes the state-changing
// surface needed to register, update, or remove a provider and manage
// its PDP product.
//
// Provider types:
//   - Endorsed: curated, high-quality providers (used as primary).
//   - Approved: automated QA-checked providers (used as secondary).
//
// # Read surface
//
// The read surface is usable with any bind.ContractCaller (including a
// plain *ethclient.Client) and returns decoded Go types for providers,
// PDP offerings, endorsement lists, and ID ↔ address mappings.
//
// # Write surface
//
// When Options.Backend, Options.Signer, and Options.ChainID are supplied,
// the following state-changing methods become available:
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
//     GetProviderIDByAddress is an exception: it returns id.Sign() == 0
//     (no error) for unknown addresses, mirroring the contract convention.
//   - ErrInvalidArgument: returned when required arguments are nil, zero,
//     or otherwise malformed.
//   - ErrInvalidOffering: returned by ValidatePDPOffering / write methods
//     when a PDP offering fails structural validation.
//   - ErrWriteNotConfigured: returned by write methods when Service was
//     constructed without a Signer / Backend / NonceManager.
//   - ErrTxFailed: returned by write methods when the broadcast
//     transaction reverts on-chain.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Mirrors the
// TS SDK package at synapse-sdk/packages/synapse-sdk/src/sp-registry.
package spregistry
