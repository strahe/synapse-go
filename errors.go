package synapse

import "github.com/strahe/synapse-go/internal/lifecycle"

// ErrClosed is returned by service methods invoked after the owning
// [Client] has been closed via [Client.Close]. It is an alias of
// [internal/lifecycle.ErrClosed] and matches the per-package ErrClosed
// sentinels (e.g. [payments.ErrClosed]) re-exported by every sub-service.
var ErrClosed = lifecycle.ErrClosed
