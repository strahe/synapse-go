// Package lifecycle tracks whether a [synapse.Client] has been closed so
// that services derived from it can refuse new work with a clear error
// rather than making RPC calls on a closed ethclient.
//
// Lifecycle is shared between the root Client and all of its services:
// Client.Close marks the Lifecycle closed; each service's public methods
// fail fast with ErrClosed while still being safe for concurrent use.
package lifecycle

import (
	"errors"
	"sync/atomic"
)

// ErrClosed is returned from any service method called after the owning
// Client's Close has executed.
var ErrClosed = errors.New("synapse: client closed")

// Lifecycle is safe for concurrent use. The zero value is a usable,
// unclosed lifecycle. A nil receiver is treated as a permanently-open
// lifecycle so services used without a root Client (e.g. in tests) skip
// the closed check entirely.
type Lifecycle struct {
	closed atomic.Bool
}

// New returns a fresh, unclosed Lifecycle.
func New() *Lifecycle { return &Lifecycle{} }

// CheckClosed returns ErrClosed if the lifecycle has been closed.
// Nil-safe.
func (l *Lifecycle) CheckClosed() error {
	if l == nil {
		return nil
	}
	if l.closed.Load() {
		return ErrClosed
	}
	return nil
}

// IsClosed reports whether Close has been called. Nil-safe.
func (l *Lifecycle) IsClosed() bool {
	if l == nil {
		return false
	}
	return l.closed.Load()
}

// Close marks the lifecycle closed. Idempotent and nil-safe.
func (l *Lifecycle) Close() {
	if l == nil {
		return
	}
	l.closed.Store(true)
}
