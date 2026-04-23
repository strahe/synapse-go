package adapters

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/storage"
)

// costCalculator adapts *costs.Service to [storage.MultiCostCalculator],
// translating storage.ContextCostRef -> costs.MultiContextRef.
type costCalculator struct {
	c *costs.Service
}

// NewCostCalculator returns a [storage.MultiCostCalculator] backed by c.
func NewCostCalculator(c *costs.Service) storage.MultiCostCalculator {
	return &costCalculator{c: c}
}

func (a *costCalculator) CalculateMultiContextCosts(
	ctx context.Context,
	payer common.Address,
	dataSizeBytes *big.Int,
	refs []storage.ContextCostRef,
	opts storage.MultiCostOptions,
) (*storage.MultiContextCosts, error) {
	out := make([]costs.MultiContextRef, 0, len(refs))
	for _, r := range refs {
		currentSize := r.CurrentDataSetSizeBytes
		if r.DataSetID == nil {
			currentSize = nil
		}
		// The manager-level EnableCDN flag force-enables CDN across the
		// batch. Per-ref WithCDN still matters for mixed batches when the
		// caller leaves EnableCDN false.
		out = append(out, costs.MultiContextRef{
			IsNewDataSet:            r.DataSetID == nil,
			CurrentDataSetSizeBytes: currentSize,
			WithCDN:                 r.WithCDN || opts.EnableCDN,
		})
	}
	got, err := a.c.CalculateMultiContextCosts(ctx, payer, dataSizeBytes, out, &costs.UploadCostOptions{
		ExtraRunwayEpochs: opts.ExtraRunwayEpochs,
		BufferEpochs:      opts.BufferEpochs,
	})
	if err != nil {
		return nil, err
	}
	return &storage.MultiContextCosts{
		RatePerEpoch:         got.RatePerEpoch,
		RatePerMonth:         got.RatePerMonth,
		DepositNeeded:        got.DepositNeeded,
		NeedsFWSSMaxApproval: got.NeedsFWSSMaxApproval,
		Ready:                got.Ready,
	}, nil
}
