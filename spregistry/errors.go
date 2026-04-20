package spregistry

import "errors"

// ErrNotFound is returned, wrapped via fmt.Errorf with %w, when a queried
// provider (by id or address) is not registered. Callers should use
// errors.Is(err, spregistry.ErrNotFound) rather than comparing for nil
// results.
var ErrNotFound = errors.New("spregistry: not found")

// ErrInvalidArgument is returned, wrapped, when a caller passes an argument
// that fails local precondition checks (nil IDs, zero addresses, etc.).
// Use errors.Is to detect it.
var ErrInvalidArgument = errors.New("spregistry: invalid argument")

// ErrInvalidOffering is returned when a PDPOffering decoded from on-chain
// capabilities fails validation (missing required fields, non-positive
// numeric parameters).
var ErrInvalidOffering = errors.New("spregistry: invalid offering")
