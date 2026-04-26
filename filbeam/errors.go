package filbeam

import (
	"errors"

	"github.com/strahe/synapse-go/internal/lifecycle"
)

// ErrDataSetNotFound is returned by GetDataSetStats when the dataset does not
// exist on the FilBeam stats API (HTTP 404).
var ErrDataSetNotFound = errors.New("filbeam: data set not found")

// ErrUninitialized is returned when a method is invoked on a zero-value
// Service (one that was not constructed via [New]).
var ErrUninitialized = errors.New("filbeam: service not initialized; use filbeam.New")

// ErrClosed is returned when a method is called after the owning Client
// has been closed. It aliases the shared closed-client sentinel.
var ErrClosed = lifecycle.ErrClosed

// ErrInvalidArgument is returned when a caller passes a zero or otherwise
// invalid argument to a Service method.
var ErrInvalidArgument = errors.New("filbeam: invalid argument")
