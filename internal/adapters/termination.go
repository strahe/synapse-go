package adapters

import (
	"context"

	"github.com/strahe/synapse-go/storage"
	sdktypes "github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type fwssTerminator struct {
	w *warmstorage.Service
}

// NewFWSSTerminator returns a storage.FWSSTerminator backed by w.
func NewFWSSTerminator(w *warmstorage.Service) storage.FWSSTerminator {
	return &fwssTerminator{w: w}
}

func (a *fwssTerminator) TerminateDataSet(ctx context.Context, id sdktypes.BigInt, opts storage.FWSSTerminationOptions) (*sdktypes.WriteResult, error) {
	writeOpts := append([]warmstorage.WriteOption(nil), opts.WriteOptions...)
	writeOpts = append(writeOpts, warmstorage.WithWait(opts.WaitTimeout))
	if opts.OnSubmitted != nil {
		writeOpts = append(writeOpts, warmstorage.WithOnSubmitted(opts.OnSubmitted))
	}
	return a.w.TerminateDataSet(ctx, id, writeOpts...)
}
