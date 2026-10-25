package types

import (
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// WriteResult reports a submitted transaction or a read-only transaction wait.
//
// Write methods populate Hash after successful broadcast. Receipt is populated
// when the call was made with a wait option (e.g. WithWait(timeout)) and the transaction
// was mined and reached any requested confirmation count before the wait ended.
// Explicit confirmations apply to both successful and failed receipts, counting
// the inclusion block as the first confirmation.
//
// Write-method error semantics (observe WriteResult and err together):
//
//   - WriteResult == nil, err != nil: no successful submission result is
//     available. When sending returned an error, err wraps [BroadcastError],
//     which preserves the original signed transaction. A nil result does not
//     establish that the node rejected the transaction; do not blindly retry
//     the business operation with a new transaction.
//   - WriteResult != nil, err == nil: transaction was broadcast
//     successfully. Without a wait option, Receipt is nil. With a wait
//     option, Receipt is non-nil and Status == 1 (successful execution).
//   - WriteResult != nil, err != nil, Receipt == nil: the wait timed out
//     before a terminal receipt was returned, was canceled, or receipt lookup failed.
//     This includes both "not mined yet" and "already mined, but the
//     configured confirmation count has not been satisfied yet". Hash is
//     valid; callers can keep polling by Hash.
//     Use errors.Is with ErrReceiptTimeout, ErrReceiptRPCFailure, or
//     context.Canceled to distinguish these outcomes; RPC causes are preserved.
//   - WriteResult != nil, err != nil, Receipt != nil: transaction was
//     mined but execution reverted. err wraps ErrTxFailed (use errors.Is).
//     Receipt carries the failed status.
//
// For wallet writes in payments, sessionkey, spregistry, and warmstorage, a nil
// result without BroadcastError establishes that this invocation did not send a
// transaction. This guarantee does not apply to provider-relayed storage writes.
//
// WaitForTransaction is read-only: Hash is the supplied lookup key and does not
// prove broadcast. Its Receipt follows the waiting cases above.
type WriteResult struct {
	Hash    common.Hash
	Receipt *ethtypes.Receipt
}
