package adapters

import (
	"context"
	"time"

	"github.com/ethereum/go-ethereum/common"

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
	writeOpts = append(writeOpts, warmstorage.WithWait(0), warmstorage.WithOnSubmitted(nil))
	return a.w.TerminateDataSet(ctx, id, writeOpts...)
}

func (a *fwssTerminator) WaitForTermination(ctx context.Context, id sdktypes.BigInt, hash common.Hash, timeout time.Duration, opts []warmstorage.WriteOption) (*sdktypes.WriteResult, sdktypes.Epoch, error) {
	writeOpts := append([]warmstorage.WriteOption(nil), opts...)
	writeOpts = append(writeOpts, warmstorage.WithWait(timeout), warmstorage.WithOnSubmitted(nil))
	res, err := a.w.WaitForTransaction(ctx, hash, writeOpts...)
	if err != nil {
		return res, 0, err
	}
	event, err := a.w.PDPPaymentTerminatedEvent(res.Receipt, id)
	if err != nil {
		return res, 0, err
	}
	return res, event.EndEpoch, nil
}
