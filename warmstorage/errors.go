package warmstorage

import (
	"errors"

	"github.com/strahe/synapse-go/internal/lifecycle"
)

// ErrNotFound is returned, wrapped via fmt.Errorf with %w, when a queried
// record (e.g. a data set) does not exist on-chain. Callers should use
// errors.Is(err, warmstorage.ErrNotFound) rather than comparing for nil
// results.
var ErrNotFound = errors.New("warmstorage: not found")

// ErrUninitialized is returned when a method is invoked on a zero-value
// Service (one that was not constructed via [New]).
var ErrUninitialized = errors.New("warmstorage: service not initialized; use warmstorage.New")

// ErrClosed is returned when a method is called after the owning Client
// has been closed. It aliases the shared closed-client sentinel.
var ErrClosed = lifecycle.ErrClosed

// ErrInvalidArgument is returned, wrapped, when a caller passes an argument
// that fails local precondition checks (nil IDs, zero addresses, etc.).
// Use errors.Is to detect it.
var ErrInvalidArgument = errors.New("warmstorage: invalid argument")
