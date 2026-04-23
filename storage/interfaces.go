package storage

import (
	"context"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/payments"
	sdktypes "github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// PDPVerifierReader is the read-only PDPVerifier surface required by
// [Context] for piece lifecycle queries (scheduled removals, id lookup,
// next challenge epoch) and by [PieceStatus] for proving-window
// calculations. Implementations convert between [sdktypes.DataSetID]
// / [cid.Cid] / uint64 and the abigen-native types (`*big.Int`,
// `pdpverifier.CidsCid`).
type PDPVerifierReader interface {
	FindPieceIdsByCid(ctx context.Context, dataSetID sdktypes.DataSetID, pieceCID cid.Cid, start, limit uint64) ([]uint64, error)
	GetScheduledRemovals(ctx context.Context, dataSetID sdktypes.DataSetID) ([]uint64, error)
	GetNextChallengeEpoch(ctx context.Context, dataSetID sdktypes.DataSetID) (*big.Int, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// PDPConfigReader returns the proving-period configuration from the
// FWSSView contract. Satisfied by *warmstorage.Service.
type PDPConfigReader interface {
	GetPDPConfig(ctx context.Context) (*warmstorage.PDPConfig, error)
}

// FWSSTerminator terminates an on-chain data set via FWSS.TerminateService.
// Satisfied by *warmstorage.Service (see TerminateDataSet).
type FWSSTerminator interface {
	TerminateDataSet(ctx context.Context, dataSetID sdktypes.DataSetID, opts ...warmstorage.WriteOption) (*sdktypes.WriteResult, error)
}

// DataSetFinder lists the enriched data sets owned by `payer`. Satisfied
// by *warmstorage.Service via GetClientDataSetsWithDetails. Mirrors TS
// StorageManager.findDataSets.
type DataSetFinder interface {
	FindDataSets(ctx context.Context, payer common.Address, onlyManaged bool) ([]*DataSetInfo, error)
}

// StorageInfoReader returns the chain-wide StorageInfo view for the
// given client. Mirrors TS StorageManager.getStorageInfo.
type StorageInfoReader interface {
	GetStorageInfo(ctx context.Context, client common.Address) (*StorageInfo, error)
}

// MultiCostOptions customises the multi-context cost calculation.
// Mirrors the TS SDK `{extraRunwayEpochs, bufferEpochs, withCDN}`
// knobs passed through from StorageManager.calculateMultiContextCosts.
type MultiCostOptions struct {
	// EnableCDN forces CDN pricing on every ref (in addition to any
	// per-ref `WithCDN` flag) and governs whether the CDN-fixed lockup
	// is added for new datasets.
	EnableCDN bool
	// ExtraRunwayEpochs is additional runway (epochs) on top of the
	// minimum lockup period. Defaults to 0 when unset.
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit cushion above current lockup usage to
	// cover transaction latency. Zero uses the cost service default
	// (5 epochs).
	BufferEpochs int64
}

// MultiCostCalculator computes an upload-cost summary for a fan-out
// across multiple prospective contexts. Mirrors TS
// StorageManager.calculateMultiContextCosts.
type MultiCostCalculator interface {
	CalculateMultiContextCosts(ctx context.Context, payer common.Address, dataSizeBytes *big.Int, refs []ContextCostRef, opts MultiCostOptions) (*MultiContextCosts, error)
}

// DataSetSizeReader returns the current on-chain size (bytes) of an
// existing data set, used by [Service.Prepare] to price lockup
// accurately for add-pieces scenarios. Satisfied by an adapter around
// PDPVerifier.getDataSetLeafCount (leafCount * 32).
type DataSetSizeReader interface {
	GetDataSetSizeBytes(ctx context.Context, dataSetID sdktypes.DataSetID) (*big.Int, error)
}

// PaymentsFunder tops up the Payments contract for an upload. Narrow
// view of payments.Service used by PrepareTransaction.Execute.
type PaymentsFunder interface {
	FundSync(ctx context.Context, amount *big.Int, opts ...payments.WriteOption) (*sdktypes.WriteResult, error)
}

// MultiContextCosts is the aggregate cost view across N upload targets.
// Shape matches TS calculateMultiContextCosts return value — a flat
// summary rather than per-context breakdown (TS computes per-context
// locally but only exposes the aggregate).
type MultiContextCosts struct {
	RatePerEpoch         *big.Int
	RatePerMonth         *big.Int
	DepositNeeded        *big.Int
	NeedsFWSSMaxApproval bool
	Ready                bool
}
