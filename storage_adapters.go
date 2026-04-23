package synapse

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
	sdktypes "github.com/strahe/synapse-go/types"
)

// pdpVerifierAdapter adapts the abigen PDPVerifierCaller plus an
// ethclient into storage.PDPVerifierReader, converting between
// Go-friendly types (sdktypes.DataSetID / cid.Cid / uint64) and the
// abigen-native types (*big.Int / pdpverifier.CidsCid).
type pdpVerifierAdapter struct {
	caller  *pdpverifier.PDPVerifierCaller
	backend *ethclient.Client
}

func (a *pdpVerifierAdapter) FindPieceIdsByCid(ctx context.Context, dataSetID sdktypes.DataSetID, pieceCID cid.Cid, start, limit uint64) ([]uint64, error) {
	opts := &bind.CallOpts{Context: ctx}
	raw, err := a.caller.FindPieceIdsByCid(
		opts,
		idconv.Big(dataSetID),
		iabi.EncodePieceCID(pieceCID),
		new(big.Int).SetUint64(start),
		new(big.Int).SetUint64(limit),
	)
	if err != nil {
		return nil, fmt.Errorf("pdpVerifierAdapter.FindPieceIdsByCid: %w", err)
	}
	out, err := idconv.SafeSlice[uint64]("pieceID", raw)
	if err != nil {
		return nil, fmt.Errorf("pdpVerifierAdapter.FindPieceIdsByCid: %w", err)
	}
	return out, nil
}

func (a *pdpVerifierAdapter) GetScheduledRemovals(ctx context.Context, dataSetID sdktypes.DataSetID) ([]uint64, error) {
	raw, err := a.caller.GetScheduledRemovals(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
	if err != nil {
		if isDataSetNotLive(err) {
			return []uint64{}, nil
		}
		return nil, fmt.Errorf("pdpVerifierAdapter.GetScheduledRemovals: %w", err)
	}
	out, err := idconv.SafeSlice[uint64]("pieceID", raw)
	if err != nil {
		return nil, fmt.Errorf("pdpVerifierAdapter.GetScheduledRemovals: %w", err)
	}
	return dedupeUint64s(out), nil
}

func (a *pdpVerifierAdapter) GetNextChallengeEpoch(ctx context.Context, dataSetID sdktypes.DataSetID) (*big.Int, error) {
	v, err := a.caller.GetNextChallengeEpoch(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
	if err != nil {
		if isDataSetNotLive(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("pdpVerifierAdapter.GetNextChallengeEpoch: %w", err)
	}
	if v == nil || v.Sign() <= 0 {
		return nil, nil
	}
	return new(big.Int).Set(v), nil
}

func (a *pdpVerifierAdapter) BlockNumber(ctx context.Context) (uint64, error) {
	return a.backend.BlockNumber(ctx)
}

// GetDataSetSizeBytes returns the on-chain size in bytes of a data set
// by reading PDPVerifier.getDataSetLeafCount and multiplying by the
// fixed 32-byte leaf size. Satisfies storage.DataSetSizeReader.
//
// For TS parity with getDataSetSizes (synapse-core/pdp-verifier/
// get-dataset-size.ts:73-83), a "Data set not live" revert — returned
// by the contract when the data set has been terminated — is treated
// as size 0 rather than propagated, so Service.Prepare keeps producing
// floor-priced costs for recently-terminated contexts.
func (a *pdpVerifierAdapter) GetDataSetSizeBytes(ctx context.Context, dataSetID sdktypes.DataSetID) (*big.Int, error) {
	leafCount, err := a.caller.GetDataSetLeafCount(&bind.CallOpts{Context: ctx}, idconv.Big(dataSetID))
	if err != nil {
		if isDataSetNotLive(err) {
			return new(big.Int), nil
		}
		return nil, fmt.Errorf("pdpVerifierAdapter.GetDataSetSizeBytes: %w", err)
	}
	if leafCount == nil {
		return new(big.Int), nil
	}
	return new(big.Int).Mul(leafCount, big.NewInt(32)), nil
}

// isDataSetNotLive reports whether err is the PDPVerifier revert raised
// for terminated / non-live data sets. Matched against the literal
// string used by the solidity contract (see
// synapse-sdk/.../utils/contract-errors.ts:7).
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

// fwssTerminateAdapter is no longer needed: *warmstorage.Service directly
// satisfies storage.FWSSTerminator. Kept as a typed alias-free comment for
// clarity; see services.go where ws is injected directly.
