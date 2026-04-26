package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// Terminate schedules termination of this context's data set via the
// FWSS terminateService entry point. On success the provider stops
// proving the data set and all contained pieces will be removed
// on-chain.
//
// opts are forwarded to warmstorage.Service.TerminateDataSet (wait /
// confirmations / etc.).
func (c *Context) Terminate(ctx context.Context, opts ...warmstorage.WriteOption) (*types.WriteResult, error) {
	if c.fwssTerminator == nil {
		return nil, errors.New("storage.Context.Terminate: FWSS terminator not configured")
	}
	if c.dataSetID == nil {
		return nil, fmt.Errorf("storage.Context.Terminate: %w: dataSetID not set", ErrInvalidArgument)
	}
	return c.fwssTerminator.TerminateDataSet(ctx, *c.dataSetID, opts...)
}
