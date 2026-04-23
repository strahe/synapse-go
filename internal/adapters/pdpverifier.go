package adapters

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ipfs/go-cid"

	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/storage"
	sdktypes "github.com/strahe/synapse-go/types"
)

// PDPReader is the union of [storage.PDPVerifierReader] and
// [storage.DataSetSizeReader] satisfied by a single adapter around the
// abigen PDPVerifierCaller plus an ethclient. Root synapse holds a
// single field of this type and fans it out to both storage options.
type PDPReader interface {
	storage.PDPVerifierReader
	storage.DataSetSizeReader
}

// pdpVerifierReader adapts the abigen PDPVerifierCaller plus an
// ethclient into [PDPReader], converting between Go-friendly types
// (sdktypes.DataSetID / cid.Cid / uint64) and the abigen-native types
// (*big.Int / pdpverifier.CidsCid).
type pdpVerifierReader struct {
	caller  *pdpverifier.PDPVerifierCaller
	backend *ethclient.Client
}

// NewPDPVerifierReader returns a [PDPReader] wrapping caller and backend.
// When caller is nil it returns a nil interface value, letting callers
// keep the plain `if r != nil` check without hitting Go's typed-nil
// interface trap.
func NewPDPVerifierReader(caller *pdpverifier.PDPVerifierCaller, backend *ethclient.Client) PDPReader {
	if caller == nil {
		return nil
	}
	return &pdpVerifierReader{caller: caller, backend: backend}
}

func (a *pdpVerifierReader) FindPieceIdsByCid(ctx context.Context, dataSetID sdktypes.DataSetID, pieceCID cid.Cid, start, limit uint64) ([]uint64, error) {
	opts := &bind.CallOpts{Context: ctx}
	raw, err := a.caller.FindPieceIdsByCid(
		opts,
		idconv.Big(dataSetID),
		iabi.EncodePieceCID(pieceCID),
		new(big.Int).SetUint64(start),
		new(big.Int).SetUint64(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIdsByCid: %w", err)
	}
	out, err := idconv.SafeSlice[uint64]("pieceID", raw)
	if err != nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.FindPieceIdsByCid: %w", err)
	}
	return out, nil
}

func (a *pdpVerifierReader) GetScheduledRemovals(ctx context.Context, dataSetID sdktypes.DataSetID) ([]uint64, error) {
	raw, err := a.caller.GetScheduledRemovals(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
	if err != nil {
		if isDataSetNotLive(err) {
			return []uint64{}, nil
		}
		return nil, fmt.Errorf("adapters.pdpVerifierReader.GetScheduledRemovals: %w", err)
	}
	out, err := idconv.SafeSlice[uint64]("pieceID", raw)
	if err != nil {
		return nil, fmt.Errorf("adapters.pdpVerifierReader.GetScheduledRemovals: %w", err)
	}
	return dedupeUint64s(out), nil
}

func (a *pdpVerifierReader) GetNextChallengeEpoch(ctx context.Context, dataSetID sdktypes.DataSetID) (*big.Int, error) {
	v, err := a.caller.GetNextChallengeEpoch(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
	if err != nil {
		if isDataSetNotLive(err) {
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
func (a *pdpVerifierReader) GetDataSetSizeBytes(ctx context.Context, dataSetID sdktypes.DataSetID) (*big.Int, error) {
	leafCount, err := a.caller.GetDataSetLeafCount(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
	if err != nil {
		if isDataSetNotLive(err) {
			return new(big.Int), nil
		}
		return nil, fmt.Errorf("adapters.pdpVerifierReader.GetDataSetSizeBytes: %w", err)
	}
	if leafCount == nil {
		return new(big.Int), nil
	}
	return new(big.Int).Mul(leafCount, big.NewInt(32)), nil
}

// isDataSetNotLive reports whether err is the PDPVerifier revert raised
// for terminated / non-live data sets. Match against the contract's
// literal revert string so callers can collapse that state to size 0.
func isDataSetNotLive(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "Data set not live")
}

func dedupeUint64s(values []uint64) []uint64 {
	if len(values) == 0 {
		return values
	}
	out := make([]uint64, 0, len(values))
	seen := make(map[uint64]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
