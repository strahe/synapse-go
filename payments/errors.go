package payments

import "github.com/strahe/synapse-go/internal/txutil"

// ErrTxFailed reports that a transaction was mined but reverted on-chain.
// Use errors.Is to match errors returned by state-changing calls.
var ErrTxFailed = txutil.ErrTxFailed
