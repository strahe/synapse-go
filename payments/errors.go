package payments

import (
	"errors"

	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/types"
)

// ErrTxFailed reports that a transaction was mined but reverted on-chain.
// Use errors.Is to match errors returned by state-changing calls.
//
// This is an alias of types.ErrTxFailed kept for backwards compatibility;
// callers can match either interchangeably.
var ErrTxFailed = types.ErrTxFailed

// ErrUninitialized is returned when a method is invoked on a zero-value
// Service (one that was not constructed via [New]). Match with
// errors.Is(err, payments.ErrUninitialized).
var ErrUninitialized = errors.New("payments: service not initialized; use payments.New")

// ErrClosed is returned when a method is called after the owning Client
// has been closed. Alias of [internal/lifecycle.ErrClosed] so
// errors.Is(err, payments.ErrClosed) matches either sentinel.
var ErrClosed = lifecycle.ErrClosed

// ErrInvalidArgument is returned, wrapped via fmt.Errorf with %w, when a
// caller passes an argument that violates a precondition (nil pointer,
// zero address, invalid chain ID). Match with
// errors.Is(err, payments.ErrInvalidArgument).
//
// Business-domain sentinels such as ErrInsufficientBalance or
// ErrInsufficientAllowance live alongside this file and cover runtime
// preconditions that are independent of caller-supplied arguments.
var ErrInvalidArgument = errors.New("payments: invalid argument")
