package storage

import (
	"context"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/payments"
	sdktypes "github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// PDPVerifierReader is the read-only PDPVerifier surface required by
// [DataSetContext] for piece lifecycle queries (scheduled removals, ID lookup,
// next challenge epoch) and proving-window calculations. The root Client
// supplies a PDPVerifier adapter.
type PDPVerifierReader interface {
	FindPieceIdsByCid(ctx context.Context, dataSetID sdktypes.BigInt, pieceCID cid.Cid, start, limit uint64) ([]sdktypes.BigInt, error)
	FindPieceIDsByCIDs(ctx context.Context, dataSetID sdktypes.BigInt, pieceCIDs []cid.Cid) ([][]sdktypes.BigInt, error)
	GetScheduledRemovals(ctx context.Context, dataSetID sdktypes.BigInt) ([]sdktypes.BigInt, error)
	GetNextChallengeEpoch(ctx context.Context, dataSetID sdktypes.BigInt) (*big.Int, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// PDPConfigReader reads proving-period configuration. The root Client
// supplies [warmstorage.Service].
type PDPConfigReader interface {
	GetPDPConfig(ctx context.Context) (*warmstorage.PDPConfig, error)
}

// FWSSTerminationOptions configures a data-set termination request.
type FWSSTerminationOptions struct {
	// WaitTimeout is positive and requires waiting for a receipt.
	WaitTimeout time.Duration
	// OnSubmitted must be called synchronously once after successful broadcast,
	// before receipt waiting, even if that wait later fails. Setup or broadcast
	// failures must not notify. Nil leaves notification to WriteOptions.
	OnSubmitted func(common.Hash)
	// WriteOptions carry additional WarmStorage write settings. WaitTimeout
	// overrides WithWait; a non-nil OnSubmitted overrides WithOnSubmitted.
	WriteOptions []warmstorage.WriteOption
}

// FWSSTerminator terminates data sets through FWSS. The root Client supplies a
// WarmStorage adapter.
type FWSSTerminator interface {
	TerminateDataSet(ctx context.Context, dataSetID sdktypes.BigInt, opts FWSSTerminationOptions) (*sdktypes.WriteResult, error)
}

// DataSetValidator checks that a data set is live in PDPVerifier and managed
// by the current FWSS listener. The root Client supplies
// [warmstorage.Service].
type DataSetValidator interface {
	ValidateDataSet(ctx context.Context, dataSetID sdktypes.BigInt) error
}

// DataSetDetailsCatalog lists data sets with PDP liveness, FWSS-listener
// ownership, and metadata. The root Client supplies [warmstorage.Service].
type DataSetDetailsCatalog interface {
	GetClientDataSetsWithDetails(ctx context.Context, payer common.Address, onlyManaged bool) ([]*warmstorage.EnhancedDataSetInfo, error)
}

// FWSSDataSetReader reads data-set records from the FWSSView contract.
// Resolvers use it before construction to obtain complete immutable targets;
// upload paths use it to reject ended existing data sets before sending bytes
// to a provider; ProviderContext uses it to recover creation by caller-owned
// client data-set ID. The implementation must target the same chain and record
// keeper configured on the context. Satisfied by *warmstorage.Service.
type FWSSDataSetReader interface {
	GetDataSet(ctx context.Context, dataSetID sdktypes.BigInt) (*warmstorage.DataSetInfo, error)
	FindDataSetByClientDataSetID(ctx context.Context, payer common.Address, clientDataSetID sdktypes.BigInt) (*warmstorage.DataSetInfo, error)
}

// ProviderResolver resolves storage providers by provider ID. The root Client
// supplies [ServiceResolver].
type ProviderResolver interface {
	ResolveProvider(ctx context.Context, providerID sdktypes.BigInt) (Provider, error)
}

// PaymentStateReader reads payment account state for termination pre-checks.
// The root Client supplies [payments.Service].
type PaymentStateReader interface {
	AccountInfo(ctx context.Context, token, owner common.Address) (*payments.AccountState, error)
}

// EpochReader returns the current chain epoch. The root Client supplies its
// configured Ethereum client.
type EpochReader interface {
	BlockNumber(ctx context.Context) (uint64, error)
}

// DataSetFinder lists enriched data sets owned by payer. The root Client
// supplies a WarmStorage adapter.
type DataSetFinder interface {
	FindDataSets(ctx context.Context, payer common.Address, onlyManaged bool) ([]*DataSetDetails, error)
}

// StorageInfoReader reads the chain-wide StorageInfo view. The root Client
// supplies an adapter.
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
	// minimum lockup period. Defaults to 0. Negative values return ErrInvalidArgument.
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit cushion above current lockup usage to
	// cover transaction latency. Nil uses the cost service default; a pointer
	// to zero disables the buffer. Negative values return ErrInvalidArgument.
	BufferEpochs *int64
}

// MultiCostCalculator calculates aggregate upload costs. The root Client
// supplies [costs.Service]. Its input contract is defined by
// [costs.Service.CalculateMultiContextCosts]: leaf count, lifecycle reserve
// state, PDP end epoch, and CDN are supplied through refs, not opts.
type MultiCostCalculator interface {
	CalculateMultiContextCosts(ctx context.Context, payer common.Address, pieceSizes []uint64, refs []costs.MultiContextRef, opts *costs.UploadCostOptions) (*costs.MultiContextCosts, error)
}

// DataSetLeafCountReader reads existing data-set leaf counts. The root Client
// supplies a PDPVerifier adapter. A successful result must be non-nil and
// non-negative; zero means known empty.
// Missing or non-live data sets return [ErrDataSetUnavailable].
type DataSetLeafCountReader interface {
	GetDataSetLeafCount(ctx context.Context, dataSetID sdktypes.BigInt) (*big.Int, error)
}

// PaymentsFunder tops up the Payments contract for an upload. The root Client
// supplies a Payments adapter.
type PaymentsFunder interface {
	FundSync(ctx context.Context, amount *big.Int, opts ...payments.WriteOption) (*sdktypes.WriteResult, error)
}
