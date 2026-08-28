package adapters

import (
	"context"
	"fmt"
	"math/big"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/storage"
	sdktypes "github.com/strahe/synapse-go/types"
)

// PDPReader is the union of [storage.PDPVerifierReader] and
// [storage.DataSetSizeReader] satisfied by a single adapter around the
// abigen PDPVerifierCaller plus an RPC backend. Root synapse holds a
// single field of this type and fans it out to both storage options.
type PDPReader interface {
	storage.PDPVerifierReader
	storage.DataSetSizeReader
}

// pdpVerifierReader adapts the abigen PDPVerifierCaller plus an RPC
// backend into [PDPReader], converting between Go-friendly types
// (sdktypes.BigInt / cid.Cid) and the abigen-native types
// (*big.Int / pdpverifier.CidsCid).
type pdpVerifierReader struct {
	caller            *pdpverifier.PDPVerifierCaller
	backend           pdpReaderBackend
	address           common.Address
	maxMulticallCalls int
}

type pdpReaderBackend interface {
	iabi.ContractCaller
	BlockNumber(context.Context) (uint64, error)
}

// NewPDPVerifierReader returns a [PDPReader] wrapping caller and backend.
// When caller is nil it returns a nil interface value, letting callers
// keep the plain `if r != nil` check without hitting Go's typed-nil
// interface trap.
func NewPDPVerifierReader(
	caller *pdpverifier.PDPVerifierCaller,
	backend pdpReaderBackend,
	address common.Address,
	maxMulticallCalls int,
) PDPReader {
	if caller == nil {
		return nil
	}
	if maxMulticallCalls == 0 {
		maxMulticallCalls = iabi.DefaultMaxMulticallCalls
	}
	return &pdpVerifierReader{
		caller:            caller,
		backend:           backend,
		address:           address,
		maxMulticallCalls: maxMulticallCalls,
	}
}

func (a *pdpVerifierReader) FindPieceIdsByCid(ctx context.Context, dataSetID sdktypes.BigInt, pieceCID cid.Cid, start, limit uint64) ([]sdktypes.BigInt, error) {
	opts := &bind.CallOpts{Context: ctx}
	raw, err := a.caller.FindPieceIdsByCid(
		opts,
		dataSetID.Big(),
		iabi.EncodePieceCID(pieceCID),
		new(big.Int).SetUint64(start),
		new(big.Int).SetUint64(limit),
	)
	if err != nil {
		if pdpverifier.IsDataSetUnavailable(err) {
			return []sdktypes.BigInt{}, nil
		}
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIdsByCid: %w", err)
	}
	out, err := idconv.FromBigSlice("pieceID", raw)
	if err != nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIdsByCid: %w", err)
	}
	return out, nil
}

func (a *pdpVerifierReader) FindPieceIDsByCIDs(ctx context.Context, dataSetID sdktypes.BigInt, pieceCIDs []cid.Cid) ([][]sdktypes.BigInt, error) {
	if len(pieceCIDs) == 0 {
		return [][]sdktypes.BigInt{}, nil
	}
	if a.backend == nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: backend not configured")
	}
	contractABI, err := pdpverifier.PDPVerifierMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: parse ABI: %w", err)
	}
	calls := make([]iabi.Call3, len(pieceCIDs))
	for i, pieceCID := range pieceCIDs {
		callData, err := contractABI.Pack(
			"findPieceIdsByCid",
			dataSetID.Big(),
			iabi.EncodePieceCID(pieceCID),
			new(big.Int),
			big.NewInt(1),
		)
		if err != nil {
			return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: pack pieceCID at index %d: %w", i, err)
		}
		calls[i] = iabi.Call3{Target: a.address, AllowFailure: true, CallData: callData}
	}
	results, err := iabi.BatchCallChunked(ctx, a.backend, calls, a.maxMulticallCalls)
	if err != nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: %w", err)
	}
	if len(results) != len(pieceCIDs) {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: expected %d results, got %d", len(pieceCIDs), len(results))
	}

	method := contractABI.Methods["findPieceIdsByCid"]
	out := make([][]sdktypes.BigInt, len(results))
	for i, result := range results {
		if !result.Success {
			return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: pieceCID at index %d returned unsuccessful result", i)
		}
		values, err := method.Outputs.Unpack(result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: unpack pieceCID at index %d: %w", i, err)
		}
		if len(values) != 1 {
			return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: pieceCID at index %d returned %d values, want 1", i, len(values))
		}
		raw := *gethabi.ConvertType(values[0], new([]*big.Int)).(*[]*big.Int)
		out[i], err = idconv.FromBigSlice("pieceID", raw)
		if err != nil {
			return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIDsByCIDs: pieceCID at index %d: %w", i, err)
		}
	}
	return out, nil
}

func (a *pdpVerifierReader) GetScheduledRemovals(ctx context.Context, dataSetID sdktypes.BigInt) ([]sdktypes.BigInt, error) {
	raw, err := a.caller.GetScheduledRemovals(&bind.CallOpts{Context: ctx}, dataSetID.Big())
	if err != nil {
		if pdpverifier.IsDataSetUnavailable(err) {
			return []sdktypes.BigInt{}, nil
		}
		return nil, fmt.Errorf("adapters.pdpVerifierReader.GetScheduledRemovals: %w", err)
	}
	out, err := idconv.FromBigSlice("pieceID", raw)
	if err != nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.GetScheduledRemovals: %w", err)
	}
	return dedupeBigInts(out), nil
}

func (a *pdpVerifierReader) GetNextChallengeEpoch(ctx context.Context, dataSetID sdktypes.BigInt) (*big.Int, error) {
	v, err := a.caller.GetNextChallengeEpoch(&bind.CallOpts{Context: ctx}, dataSetID.Big())
	if err != nil {
		if pdpverifier.IsDataSetUnavailable(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("adapters.pdpVerifierReader.GetNextChallengeEpoch: %w", err)
	}
	if v == nil || v.Sign() <= 0 {
		return nil, nil
	}
	return new(big.Int).Set(v), nil
}

func (a *pdpVerifierReader) BlockNumber(ctx context.Context) (uint64, error) {
	return a.backend.BlockNumber(ctx)
}

// GetDataSetSizeBytes returns the on-chain size in bytes of a data set
// by reading PDPVerifier.getDataSetLeafCount and multiplying by the
// fixed 32-byte leaf size. Satisfies storage.DataSetSizeReader.
//
// A "Data set not live" revert from the contract means the data set has
// already been terminated. Treat it as size 0 instead of propagating the
// error so Service.Prepare can still compute floor-priced costs for
// recently-terminated contexts.
func (a *pdpVerifierReader) GetDataSetSizeBytes(ctx context.Context, dataSetID sdktypes.BigInt) (*big.Int, error) {
	leafCount, err := a.caller.GetDataSetLeafCount(&bind.CallOpts{Context: ctx}, dataSetID.Big())
	if err != nil {
		if pdpverifier.IsDataSetUnavailable(err) {
			return new(big.Int), nil
		}
		return nil, fmt.Errorf("adapters.pdpVerifierReader.GetDataSetSizeBytes: %w", err)
	}
	if leafCount == nil {
		return new(big.Int), nil
	}
	return new(big.Int).Mul(leafCount, big.NewInt(32)), nil
}

func dedupeBigInts(values []sdktypes.BigInt) []sdktypes.BigInt {
	if len(values) == 0 {
		return values
	}
	out := make([]sdktypes.BigInt, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := idconv.Key(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
