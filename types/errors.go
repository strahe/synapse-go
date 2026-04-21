package types

import "github.com/strahe/synapse-go/internal/txutil"

// ErrTxFailed is the canonical sentinel reported when a transaction is
// mined but its receipt status is not successful (reverted or out-of-gas
// on-chain). Use errors.Is to match errors returned by any state-changing
// call in the SDK.
//
// Service packages (payments, sessionkey, ...) expose aliases to this
// sentinel for backward compatibility; they are all the same error value.
var ErrTxFailed = txutil.ErrTxFailed
