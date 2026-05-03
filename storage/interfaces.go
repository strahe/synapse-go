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
// calculations. Implementations convert between [sdktypes.BigInt]
// / [cid.Cid] and the abigen-native types (`*big.Int`,
// `pdpverifier.CidsCid`).
type PDPVerifierReader interface {
	FindPieceIdsByCid(ctx context.Context, dataSetID sdktypes.BigInt, pieceCID cid.Cid, start, limit uint64) ([]sdktypes.BigInt, error)
	GetScheduledRemovals(ctx context.Context, dataSetID sdktypes.BigInt) ([]sdktypes.BigInt, error)
	GetNextChallengeEpoch(ctx context.Context, dataSetID sdktypes.BigInt) (*big.Int, error)
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
	TerminateDataSet(ctx context.Context, dataSetID sdktypes.BigInt, opts ...warmstorage.WriteOption) (*sdktypes.WriteResult, error)
}

// FWSSDataSetReader reads an existing data set's on-chain record from the
// FWSSView contract. Used by Service.CreateContext / Service.CreateContexts
// to auto-fetch the on-chain ClientDataSetID when the resolver path did not
// already supply one. Satisfied by *warmstorage.Service (see GetDataSet).
type FWSSDataSetReader interface {
	GetDataSet(ctx context.Context, dataSetID sdktypes.BigInt) (*warmstorage.DataSetInfo, error)
}

// DataSetFinder lists the enriched data sets owned by `payer`. Satisfied
// by *warmstorage.Service via GetClientDataSetsWithDetails.
type DataSetFinder interface {
	FindDataSets(ctx context.Context, payer common.Address, onlyManaged bool) ([]*DataSetInfo, error)
}

// StorageInfoReader returns the chain-wide StorageInfo view for the given client.
type StorageInfoReader interface {
	GetStorageInfo(ctx context.Context, client common.Address) (*StorageInfo, error)
}

// MultiCostOptions customises the multi-context cost calculation.
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
// across multiple prospective contexts.
type MultiCostCalculator interface {
	CalculateMultiContextCosts(ctx context.Context, payer common.Address, dataSizeBytes *big.Int, refs []ContextCostRef, opts MultiCostOptions) (*MultiContextCosts, error)
}

// DataSetSizeReader returns the current on-chain size (bytes) of an
// existing data set, used by Service.Prepare to price lockup
// accurately for add-pieces scenarios. Satisfied by an adapter around
// PDPVerifier.getDataSetLeafCount (leafCount * 32).
type DataSetSizeReader interface {
	GetDataSetSizeBytes(ctx context.Context, dataSetID sdktypes.BigInt) (*big.Int, error)
}

// PaymentsFunder tops up the Payments contract for an upload. Narrow
// view of payments.Service used by PrepareTransaction.Execute.
type PaymentsFunder interface {
	FundSync(ctx context.Context, amount *big.Int, opts ...payments.WriteOption) (*sdktypes.WriteResult, error)
}

// MultiContextCosts is the aggregate cost view across N upload targets.
// It is a flat summary rather than a per-context breakdown.
type MultiContextCosts struct {
	RatePerEpoch         *big.Int
	RatePerMonth         *big.Int
	DepositNeeded        *big.Int
	NeedsFWSSMaxApproval bool
	Ready                bool
}
