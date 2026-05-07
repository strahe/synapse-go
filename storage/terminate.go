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
	c.mu.RLock()
	if c.dataSetID == nil {
		c.mu.RUnlock()
		return nil, fmt.Errorf("storage.Context.Terminate: %w: dataSetID not set", ErrInvalidArgument)
	}
	dataSetID := copyBigInt(*c.dataSetID)
	c.mu.RUnlock()
	return c.fwssTerminator.TerminateDataSet(ctx, dataSetID, opts...)
}
