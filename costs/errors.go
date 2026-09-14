package costs

import (
	"errors"
	"fmt"

	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/types"
)

// ErrUninitialized is returned when a method is invoked on a zero-value
// Service (one that was not constructed via [New]).
var ErrUninitialized = errors.New("costs: service not initialized; use costs.New")

// ErrClosed is returned when a method is called after the owning Client
// has been closed. It aliases the shared closed-client sentinel.
var ErrClosed = lifecycle.ErrClosed

// ErrInvalidArgument is returned, wrapped via fmt.Errorf with %w, when a
// caller passes an argument that violates a precondition. Match with
// errors.Is(err, costs.ErrInvalidArgument).
var ErrInvalidArgument = errors.New("costs: invalid argument")

// DataSetServiceTerminatedError is returned when an existing data set's PDP
// payment rail has an end epoch, so the data set cannot accept uploads.
type DataSetServiceTerminatedError struct {
	PDPEndEpoch types.Epoch
}

func (e *DataSetServiceTerminatedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("costs: data set cannot accept uploads: has PDP payment rail end epoch %d", e.PDPEndEpoch)
}
