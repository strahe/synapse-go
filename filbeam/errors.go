package filbeam

import "errors"

// ErrDataSetNotFound is returned by GetDataSetStats when the dataset does not
// exist on the FilBeam stats API (HTTP 404).
var ErrDataSetNotFound = errors.New("filbeam: data set not found")

// ErrInvalidArgument is returned when a caller passes a zero or otherwise
// invalid argument to a Service method.
var ErrInvalidArgument = errors.New("filbeam: invalid argument")
