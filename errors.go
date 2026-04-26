package synapse

import "github.com/strahe/synapse-go/internal/lifecycle"

// ErrClosed is returned by service methods invoked after the owning
// [Client] has been closed via [Client.Close]. It is an alias of
// the shared closed-client sentinel and matches the per-package ErrClosed
// sentinels (for example, [payments.ErrClosed]) re-exported by every sub-service.
var ErrClosed = lifecycle.ErrClosed
