package payments

import (
	"errors"

	"github.com/strahe/synapse-go/types"
)

// ErrTxFailed reports that a transaction was mined but reverted on-chain.
// Use errors.Is to match errors returned by state-changing calls.
//
// This is an alias of types.ErrTxFailed kept for backwards compatibility;
// callers can match either interchangeably.
var ErrTxFailed = types.ErrTxFailed

// ErrInvalidArgument is returned, wrapped via fmt.Errorf with %w, when a
// caller passes an argument that violates a precondition (nil pointer,
// zero address, invalid chain ID). Match with
// errors.Is(err, payments.ErrInvalidArgument).
//
// Business-domain sentinels such as ErrInsufficientBalance or
// ErrInsufficientAllowance live alongside this file and cover runtime
// preconditions that are independent of caller-supplied arguments.
var ErrInvalidArgument = errors.New("payments: invalid argument")
