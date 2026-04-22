package types

import (
	"errors"
	"fmt"
)

// ErrInvalidListOptions is returned when ListOptions values are out of
// range. It is exposed so callers can assert on the class of error without
// depending on a specific service package.
var ErrInvalidListOptions = errors.New("invalid list options")

// ListOptions configures a paginated list call.
//
// Limit must be > 0. The zero value is rejected as a programming error:
// different services/contracts disagree on what "Limit == 0" means (some
// treat it as "all remaining", others as a default page size), so callers
// must be explicit.
//
// When you want to iterate every record regardless of page size, use the
// service's IterateAll* method instead of trying to encode "no cap" via
// ListOptions.
type ListOptions struct {
	Offset uint64
	Limit  uint64
}

// Validate returns ErrInvalidListOptions when Limit is not > 0.
func (o ListOptions) Validate() error {
	if o.Limit == 0 {
		return fmt.Errorf("%w: Limit must be > 0", ErrInvalidListOptions)
	}
	return nil
}
