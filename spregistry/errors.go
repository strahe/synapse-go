package spregistry

import (
	"errors"

	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/types"
)

// ErrNotFound is returned, wrapped via fmt.Errorf with %w, when a queried
// provider (by id or address) is not registered. Callers should use
// errors.Is(err, spregistry.ErrNotFound) rather than comparing for nil
// results.
var ErrNotFound = errors.New("spregistry: not found")

// ErrUninitialized is returned when a method is invoked on a zero-value
// Service (one that was not constructed via [New]).
var ErrUninitialized = errors.New("spregistry: service not initialized; use spregistry.New")

// ErrClosed is returned when a method is called after the owning Client
// has been closed. Alias of [internal/lifecycle.ErrClosed].
var ErrClosed = lifecycle.ErrClosed

// ErrInvalidArgument is returned, wrapped, when a caller passes an argument
// that fails local precondition checks (nil IDs, zero addresses, etc.).
// Use errors.Is to detect it.
var ErrInvalidArgument = errors.New("spregistry: invalid argument")

// ErrInvalidOffering is returned when a PDPOffering decoded from on-chain
// capabilities fails validation (missing required fields, non-positive
// numeric parameters).
var ErrInvalidOffering = errors.New("spregistry: invalid offering")

// ErrWriteNotConfigured is returned when a state-changing method is invoked
// on a Service that was constructed without a Signer / Backend. The SDK
// deliberately keeps writes opt-in: provider operations are an advanced
// path and most callers only need read access.
var ErrWriteNotConfigured = errors.New("spregistry: write operations not configured (missing signer or backend)")

// ErrTxFailed is returned when a broadcast transaction is mined with
// status == 0 (EVM revert). It is an alias of [types.ErrTxFailed]
// (itself an alias of [internal/txutil.ErrTxFailed]) so that
// errors.Is(err, types.ErrTxFailed) holds for every SDK-originated
// transaction failure, matching the convention used by payments and
// sessionkey. The [WriteResult.Receipt] is preserved alongside the
// error for callers that need the full on-chain status.
var ErrTxFailed = types.ErrTxFailed
