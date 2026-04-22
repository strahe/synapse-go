package spregistry

import (
	"errors"

	"github.com/strahe/synapse-go/internal/lifecycle"
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
