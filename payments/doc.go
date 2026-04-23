// Package payments provides the payment management service for Filecoin
// Onchain Cloud.
//
// It handles USDFC token operations including deposits, withdrawals,
// balance queries, approval management, and payment rail creation
// via the Filecoin Pay contract.
//
// When multiple write-capable services share the same signer / EOA, pass the
// same txutil.NonceManager to each constructor so nonce allocation stays
// serialized across services.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Mirrors
// the TS SDK package at synapse-sdk/packages/synapse-sdk/src/payments.
package payments
