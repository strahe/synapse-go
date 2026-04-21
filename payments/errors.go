package payments

import "github.com/strahe/synapse-go/types"

// ErrTxFailed reports that a transaction was mined but reverted on-chain.
// Use errors.Is to match errors returned by state-changing calls.
//
// This is an alias of types.ErrTxFailed kept for backwards compatibility;
// callers can match either interchangeably.
var ErrTxFailed = types.ErrTxFailed
