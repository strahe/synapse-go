package sessionkey

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
// zero address, negative/invalid amount, past ExpiresAt timestamp). Match
// it with errors.Is(err, sessionkey.ErrInvalidArgument).
var ErrInvalidArgument = errors.New("sessionkey: invalid argument")
