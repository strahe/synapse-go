package adapters

import (
	"context"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

// dataSetFinder adapts *warmstorage.Service to [storage.DataSetFinder].
// storage.DataSetInfo aliases warmstorage.EnhancedDataSetInfo, so no
// conversion is required.
type dataSetFinder struct {
	ws *warmstorage.Service
}

// NewDataSetFinder returns a [storage.DataSetFinder] backed by ws.
func NewDataSetFinder(ws *warmstorage.Service) storage.DataSetFinder {
	return &dataSetFinder{ws: ws}
}

func (a *dataSetFinder) FindDataSets(ctx context.Context, payer common.Address, onlyManaged bool) ([]*storage.DataSetInfo, error) {
	return a.ws.GetClientDataSetsWithDetails(ctx, payer, onlyManaged)
}
