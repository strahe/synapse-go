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
// a shared signer. Standalone services create their own nonce coordinator
// when constructed with write dependencies.
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package payments
