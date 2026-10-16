package payments

import (
	"errors"

	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/types"
)

// ErrTxFailed reports that a transaction was mined but reverted on-chain.
// Use errors.Is to match errors returned by state-changing calls.
//
// This is a convenience re-export of [types.ErrTxFailed]; the two values
// are the same sentinel and can be matched interchangeably.
var ErrTxFailed = types.ErrTxFailed

// ErrReceiptTimeout is a convenience re-export of [types.ErrReceiptTimeout].
// The same value can be matched interchangeably with errors.Is.
var ErrReceiptTimeout = types.ErrReceiptTimeout

// ErrReceiptRPCFailure is a convenience re-export of [types.ErrReceiptRPCFailure].
// The same value can be matched interchangeably with errors.Is.
var ErrReceiptRPCFailure = types.ErrReceiptRPCFailure

// BroadcastError is a convenience re-export of the same [types.BroadcastError]
// type. Either type can be used to match a transaction send error.
type BroadcastError = types.BroadcastError

// ErrUninitialized is returned when a method is invoked on a zero-value
// Service (one that was not constructed via [New]). Match with
// errors.Is(err, payments.ErrUninitialized).
var ErrUninitialized = errors.New("payments: service not initialized; use payments.New")

// ErrClosed is returned when a method is called after the owning Client
// has been closed. It aliases the shared closed-client sentinel, so
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

// ErrInvalidRailPage is returned by AccountSummary, TotalAccountFixedLockup,
// and IterateAllRailsAsPayer/Payee when a rail page cannot be safely traversed.
// Match with errors.Is.
var ErrInvalidRailPage = errors.New("payments: invalid rail page")

// ErrPermitUnsupported reports a token with no contract code or undecodable
// name, version, or nonces results required for permit signing. The underlying
// cause is preserved. RPC errors, cancellation, timeouts, and contract reverts
// do not match this sentinel; its absence does not establish permit support.
var ErrPermitUnsupported = errors.New("payments: token contract does not support EIP-2612 permit")
