package synapse

import (
	"errors"

	"github.com/strahe/synapse-go/internal/lifecycle"
)

// ErrClosed is returned by service methods invoked after the owning
// [Client] has been closed via [Client.Close]. It is an alias of
// the shared closed-client sentinel and matches the per-package ErrClosed
// sentinels (for example, [payments.ErrClosed]) re-exported by every sub-service.
var ErrClosed = lifecycle.ErrClosed

// ErrInvalidArgument is returned when a public root-package function receives
// a nil, zero, or otherwise invalid caller-supplied argument.
var ErrInvalidArgument = errors.New("synapse: invalid argument")
