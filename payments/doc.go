// Package payments provides the payment management service for Filecoin
// Onchain Cloud.
//
// It handles USDFC token operations including deposits, withdrawals,
// balance queries, approval management, and payment rail creation
// via the Filecoin Pay contract.
// AccountSummary is the recommended entry point for payment account state;
// TotalAccountFixedLockup reports fixed lockup held across payer rails.
//
// The root synapse Client wires payments together with the other
// write-capable services so transaction nonce allocation is coordinated for
// a shared signer. It also wires Fund approval lockup periods from the
// warmstorage PriceList. Standalone services create their own nonce
// coordinator when constructed with write dependencies and use the legacy
// Fund lockup-period fallback unless an ApprovalLockupPeriod reader is
// supplied.
//
// # Rail pagination
//
// GetRailsAsPayer and GetRailsAsPayee require types.ListOptions with Limit > 0.
// A zero limit returns both ErrInvalidArgument and types.ErrInvalidListOptions.
// The limit bounds the slots examined; finalized slots are skipped, so a page
// can be short or empty even when NextOffset < Total indicates more slots.
// Total counts underlying rail slots, not just the returned rails.
//
// Use IterateAllRailsAsPayer or IterateAllRailsAsPayee to read every page
// without accumulating all rails in memory. Pages may observe different
// blocks; neither manual pagination nor iteration provides an atomic snapshot.
//
// Manual offsets in types.ListOptions are uint64. NextOffset retains the
// contract's uint256 width: check IsUint64 before converting it to a manual
// offset, and reject out-of-range values rather than truncating them.
// The IterateAllRails methods carry the full-width continuation automatically.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package payments
