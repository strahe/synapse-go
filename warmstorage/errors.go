package warmstorage

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/types"
)

// ErrNotFound is returned, wrapped via fmt.Errorf with %w, when a queried
// record (e.g. a data set) does not exist on-chain. Callers should use
// errors.Is(err, warmstorage.ErrNotFound) rather than comparing for nil
// results.
var ErrNotFound = errors.New("warmstorage: not found")

// ErrUninitialized is returned when a method is invoked on a zero-value
// Service (one that was not constructed via [New]).
var ErrUninitialized = errors.New("warmstorage: service not initialized; use warmstorage.New")

// ErrClosed is returned when a method is called after the owning Client
// has been closed. It aliases the shared closed-client sentinel.
var ErrClosed = lifecycle.ErrClosed

// ErrInvalidArgument is returned, wrapped, when a caller passes an argument
// that fails local precondition checks (nil IDs, zero addresses, etc.).
// Use errors.Is to detect it.
var ErrInvalidArgument = errors.New("warmstorage: invalid argument")

// DataSetNotLiveError is returned when PDPVerifier reports that a data set is
// not live, which prevents adding pieces to it.
type DataSetNotLiveError struct {
	DataSetID types.BigInt
}

func (e *DataSetNotLiveError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("warmstorage: data set %s does not exist or is not live", e.DataSetID.String())
}

// DataSetNotManagedError is returned when a data set is live but managed by a
// listener other than this WarmStorage contract.
type DataSetNotManagedError struct {
	DataSetID        types.BigInt
	Listener         common.Address
	ExpectedListener common.Address
}

func (e *DataSetNotManagedError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf(
		"warmstorage: data set %s is managed by %s, not this WarmStorage contract (%s)",
		e.DataSetID.String(), e.Listener.Hex(), e.ExpectedListener.Hex(),
	)
}
