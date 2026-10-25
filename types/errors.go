package types

import (
	"fmt"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/txutil"
)

// ErrTxFailed is the canonical sentinel reported when a transaction is
// mined but its receipt status is not successful (reverted or out-of-gas
// on-chain). Use errors.Is to match errors returned by any state-changing
// call in the SDK.
//
// Service packages (payments, sessionkey, ...) provide convenience aliases
// to this sentinel; they are all the same error value.
var ErrTxFailed = txutil.ErrTxFailed

// ErrReceiptTimeout reports that the receipt wait or its parent context reached
// its deadline. Returned wait errors also match context.DeadlineExceeded.
var ErrReceiptTimeout = txutil.ErrReceiptTimeout

// ErrReceiptRPCFailure reports an RPC failure that cannot be retried or exhausted
// the receipt wait's consecutive error budget. The error preserves the RPC cause.
var ErrReceiptRPCFailure = txutil.ErrReceiptRPCFailure

// BroadcastError reports an error returned while sending a signed transaction.
// It does not establish whether the node rejected or accepted the transaction.
// SDK-originated errors always contain Transaction and Cause. Use errors.AsType
// to recover the original transaction for lookup or resubmission; do not retry
// the business operation merely because its WriteResult is nil.
// For payments, sessionkey, spregistry, and warmstorage wallet writes, sending
// errors wrap BroadcastError; earlier failures do not. Read-only waits and
// provider-relayed writes do not share that submission boundary.
type BroadcastError struct {
	Transaction *ethtypes.Transaction
	Cause       error
}

func (e *BroadcastError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Transaction == nil {
		return fmt.Sprintf("transaction broadcast: %v", e.Cause)
	}
	return fmt.Sprintf("transaction broadcast %s: %v", e.Transaction.Hash().Hex(), e.Cause)
}

// Unwrap preserves the original send error for errors.Is and errors.AsType.
func (e *BroadcastError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
