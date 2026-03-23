package filbeam

import "errors"

// ErrDataSetNotFound is returned by GetDataSetStats when the dataset does not
// exist on the FilBeam stats API (HTTP 404).
var ErrDataSetNotFound = errors.New("filbeam: data set not found")
