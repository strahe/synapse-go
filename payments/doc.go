// Package payments provides the payment management service for Filecoin
// Onchain Cloud.
//
// It handles USDFC token operations including deposits, withdrawals,
// balance queries, approval management, and payment rail creation
// via the Filecoin Pay contract.
// AccountSummary is the recommended entry point for payment account state;
// TotalAccountFixedLockup reports fixed lockup held across payer rails.
// Both methods pin every read to one block and include terminated rails awaiting
// final settlement. Rail details require the chain's Multicall3 contract.
// Options.MaxMulticallCalls bounds each detail batch (zero uses 64). Transient
// RPC failures receive bounded retries within the caller's context; contract
// and decoding errors are not retried. Any failure returns a nil result.
//
// The root synapse Client wires payments together with the other
// write-capable services so transaction nonce allocation is coordinated for
// a shared signer. It also wires Fund approval lockup periods from the
// warmstorage PriceList. Standalone services create their own nonce
// coordinator when constructed with write dependencies and use the legacy
// Fund lockup-period fallback unless an ApprovalLockupPeriod reader is
// supplied.
//
// Write methods return [BroadcastError] when sending a signed transaction
// returns an error. It preserves the original transaction and cause; a nil
// WriteResult does not establish that the transaction was rejected.
// Permit writes retain their token nonce lock while tracking the original
// transaction in the background, including after a broadcast error or caller
// cancellation. Tracking is bounded by Options.PermitReceiptWait (five minutes
// by default); expiry releases the lock even if the transaction is unresolved.
// Callers must resolve that transaction by hash before retrying after expiry.
//
// Amounts, operator allowances, and positive epochs or permit deadlines used in
// contract calls must fit uint256. Out-of-range caller arguments return
// [ErrInvalidArgument] before nonce acquisition or signing. Maximum allowances
// use 2^256-1; larger values are rejected. [WithSkipPrecheck] keeps these checks.
//
// Permit writes require token name, version, and nonces metadata.
// [ErrPermitUnsupported] identifies missing contract code or undecodable
// metadata; other call failures preserve their cause without that sentinel.
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
package payments
