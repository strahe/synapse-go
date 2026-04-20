package warmstorage

import "errors"

// ErrNotFound is returned, wrapped via fmt.Errorf with %w, when a queried
// record (e.g. a data set) does not exist on-chain. Callers should use
// errors.Is(err, warmstorage.ErrNotFound) rather than comparing for nil
// results.
var ErrNotFound = errors.New("warmstorage: not found")

// ErrInvalidArgument is returned, wrapped, when a caller passes an argument
// that fails local precondition checks (nil IDs, zero addresses, etc.).
// Use errors.Is to detect it.
var ErrInvalidArgument = errors.New("warmstorage: invalid argument")
