package storage

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
	sdktypes "github.com/strahe/synapse-go/types"
)

// DeletePiece schedules removal of the first piece matching pieceCID from
// this context's data set.
//
// A data set can contain multiple piece IDs with the same piece CID. This
// method resolves pieceCID with PDPVerifier.findPieceIdsByCid using limit=1
// and deletes the first returned piece ID. Prefer [DataSetContext.DeletePieceByID]
// when the on-chain piece ID is available.
//
// Missing or non-live data sets return [ErrDataSetUnavailable].
//
// The returned WriteResult carries only the transaction hash; there is no
// on-chain wait.
func (c *DataSetContext) DeletePiece(ctx context.Context, pieceCID cid.Cid) (*sdktypes.WriteResult, error) {
	const op = "storage.DataSetContext.DeletePiece"
	return c.deletePieces(ctx, op, []cid.Cid{pieceCID})
}

// DeletePieces schedules removal of the first piece matching each CID in one
// provider transaction. Duplicate resolved piece IDs are removed while
// preserving their first occurrence.
//
// A data set can contain multiple piece IDs with the same piece CID. Use
// [DataSetContext.DeletePiecesByID] to remove specific duplicate instances.
// Missing or non-live data sets return [ErrDataSetUnavailable].
func (c *DataSetContext) DeletePieces(ctx context.Context, pieceCIDs []cid.Cid) (*sdktypes.WriteResult, error) {
	const op = "storage.DataSetContext.DeletePieces"
	return c.deletePieces(ctx, op, pieceCIDs)
}

func (c *DataSetContext) deletePieces(ctx context.Context, op string, pieceCIDs []cid.Cid) (*sdktypes.WriteResult, error) {
	normalizedCIDs, err := normalizeDeletePieceCIDs(op, pieceCIDs)
	if err != nil {
		return nil, err
	}
	if len(normalizedCIDs) > pdp.MaxDeletePiecesBatchSize {
		return nil, fmt.Errorf("%s: %w: %w: got %d, max %d", op, ErrInvalidArgument, pdp.ErrTooManyPieces, len(normalizedCIDs), pdp.MaxDeletePiecesBatchSize)
	}
	if c.core.pdpCaller == nil {
		return nil, errors.New(op + ": PDPVerifier reader not configured")
	}
	target, err := c.snapshotDeletePieceTarget(op)
	if err != nil {
		return nil, err
	}

	pieceIDs, err := c.resolveDeletePieceIDs(ctx, op, target.dataSetID, normalizedCIDs)
	if err != nil {
		return nil, err
	}
	pieceIDs, err = normalizeDeletePieceIDs(op, pieceIDs)
	if err != nil {
		return nil, err
	}
	return c.schedulePieceDeletionsByID(ctx, op, target, pieceIDs)
}

type batchPieceIDResolver interface {
	FindPieceIDsByCIDs(context.Context, sdktypes.BigInt, []cid.Cid) ([][]sdktypes.BigInt, error)
}

func (c *DataSetContext) resolveDeletePieceIDs(
	ctx context.Context,
	op string,
	dataSetID sdktypes.BigInt,
	normalizedCIDs []normalizedDeletePieceCID,
) ([]sdktypes.BigInt, error) {
	if resolver, ok := c.core.pdpCaller.(batchPieceIDResolver); ok && len(normalizedCIDs) > 1 {
		pieceCIDs := make([]cid.Cid, len(normalizedCIDs))
		for i, item := range normalizedCIDs {
			pieceCIDs[i] = item.pieceCID
		}
		matches, err := resolver.FindPieceIDsByCIDs(ctx, dataSetID, pieceCIDs)
		if err != nil {
			return nil, fmt.Errorf("%s: resolve piece CIDs: %w", op, err)
		}
		if len(matches) != len(normalizedCIDs) {
			return nil, fmt.Errorf("%s: resolve piece CIDs: got %d results, want %d", op, len(matches), len(normalizedCIDs))
		}
		return firstDeletePieceIDMatches(op, normalizedCIDs, matches)
	}

	matches := make([][]sdktypes.BigInt, len(normalizedCIDs))
	for i, item := range normalizedCIDs {
		resolved, err := c.core.pdpCaller.FindPieceIdsByCid(ctx, dataSetID, item.pieceCID, 0, 1)
		if err != nil {
			return nil, fmt.Errorf("%s: resolve pieceCID at index %d: %w", op, item.originalIndex, err)
		}
		matches[i] = resolved
	}
	return firstDeletePieceIDMatches(op, normalizedCIDs, matches)
}

func firstDeletePieceIDMatches(
	op string,
	normalizedCIDs []normalizedDeletePieceCID,
	matches [][]sdktypes.BigInt,
) ([]sdktypes.BigInt, error) {
	pieceIDs := make([]sdktypes.BigInt, 0, len(normalizedCIDs))
	for i, item := range normalizedCIDs {
		if len(matches[i]) == 0 {
			return nil, fmt.Errorf("%s: %w: pieceCID at index %d not found in data set", op, ErrInvalidArgument, item.originalIndex)
		}
		pieceIDs = append(pieceIDs, matches[i][0])
	}
	return pieceIDs, nil
}

type normalizedDeletePieceCID struct {
	pieceCID      cid.Cid
	originalIndex int
}

func normalizeDeletePieceCIDs(op string, pieceCIDs []cid.Cid) ([]normalizedDeletePieceCID, error) {
	if len(pieceCIDs) == 0 {
		return nil, fmt.Errorf("%s: %w: no piece CIDs provided", op, ErrInvalidArgument)
	}
	normalized := make([]normalizedDeletePieceCID, 0, len(pieceCIDs))
	seen := make(map[string]struct{}, len(pieceCIDs))
	for i, pieceCID := range pieceCIDs {
		if !pieceCID.Defined() {
			return nil, fmt.Errorf("%s: %w: undefined pieceCID at index %d", op, ErrInvalidArgument, i)
		}
		key := pieceCID.KeyString()
		if info, err := piece.ParseV2(pieceCID); err == nil {
			key = info.CIDv1.KeyString()
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, normalizedDeletePieceCID{pieceCID: pieceCID, originalIndex: i})
	}
	return normalized, nil
}

// DeletePieceByID schedules removal of the piece identified by its on-chain
// piece ID from this context's data set.
//
// Prefer this method when the piece ID is available, because piece CID is not
// guaranteed to be unique within a data set.
func (c *DataSetContext) DeletePieceByID(ctx context.Context, pieceID sdktypes.BigInt) (*sdktypes.WriteResult, error) {
	const op = "storage.DataSetContext.DeletePieceByID"
	return c.deletePiecesByID(ctx, op, []sdktypes.BigInt{pieceID})
}

// DeletePiecesByID schedules removal of the identified on-chain piece IDs in
// one provider transaction. Duplicate IDs are removed while preserving their
// first occurrence.
func (c *DataSetContext) DeletePiecesByID(ctx context.Context, pieceIDs []sdktypes.BigInt) (*sdktypes.WriteResult, error) {
	const op = "storage.DataSetContext.DeletePiecesByID"
	return c.deletePiecesByID(ctx, op, pieceIDs)
}

func (c *DataSetContext) deletePiecesByID(ctx context.Context, op string, pieceIDs []sdktypes.BigInt) (*sdktypes.WriteResult, error) {
	pieceIDs, err := normalizeDeletePieceIDs(op, pieceIDs)
	if err != nil {
		return nil, err
	}
	target, err := c.snapshotDeletePieceTarget(op)
	if err != nil {
		return nil, err
	}

	return c.schedulePieceDeletionsByID(ctx, op, target, pieceIDs)
}

type deletePieceTarget struct {
	dataSetID       sdktypes.BigInt
	clientDataSetID *big.Int
	chainID         sdktypes.ChainID
	recordKeeper    common.Address
}

func (c *DataSetContext) snapshotDeletePieceTarget(op string) (deletePieceTarget, error) {
	if c.core.client == nil {
		return deletePieceTarget{}, errors.New(op + ": PDP client not configured")
	}
	if c.core.signer == nil {
		return deletePieceTarget{}, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}
	if !c.core.chainID.IsValid() {
		return deletePieceTarget{}, fmt.Errorf("%s: %w: invalid chainID", op, ErrInvalidArgument)
	}
	if c.core.recordKeeper == (common.Address{}) {
		return deletePieceTarget{}, fmt.Errorf("%s: %w: zero recordKeeper", op, ErrInvalidArgument)
	}
	return deletePieceTarget{
		dataSetID:       c.ref.DataSetID(),
		clientDataSetID: c.ref.ClientDataSetID().Big(),
		chainID:         c.core.chainID,
		recordKeeper:    c.core.recordKeeper,
	}, nil
}

type batchPieceDeletionClient interface {
	SchedulePieceDeletions(context.Context, sdktypes.BigInt, []sdktypes.BigInt, []byte) (common.Hash, error)
}

func (c *DataSetContext) schedulePieceDeletionsByID(ctx context.Context, op string, target deletePieceTarget, pieceIDs []sdktypes.BigInt) (*sdktypes.WriteResult, error) {
	batchClient, supportsBatch := c.core.client.(batchPieceDeletionClient)
	if len(pieceIDs) > 1 && !supportsBatch {
		return nil, fmt.Errorf("%s: %w", op, ErrBatchPieceDeletionNotSupported)
	}
	typedPieceIDs := make([]*big.Int, len(pieceIDs))
	for i, pieceID := range pieceIDs {
		typedPieceIDs[i] = pieceID.Big()
	}
	domain := ityped.NewDomain(target.chainID.BigInt(), target.recordKeeper)
	sig, err := ityped.SignSchedulePieceRemovals(
		c.core.signHashFunc(),
		domain,
		target.clientDataSetID,
		typedPieceIDs,
	)
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, fmt.Errorf("%s: wrapped/decorated EVMSigner values are unsupported: %w", op, err)
		}
		return nil, fmt.Errorf("%s: sign schedule removals: %w", op, err)
	}

	extraData, err := encodeSignatureExtraData(signatureBytes(sig))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	var txHash common.Hash
	if supportsBatch {
		txHash, err = batchClient.SchedulePieceDeletions(ctx, target.dataSetID, pieceIDs, extraData)
	} else if len(pieceIDs) == 1 {
		txHash, err = c.core.client.SchedulePieceDeletion(ctx, target.dataSetID, pieceIDs[0], extraData)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &sdktypes.WriteResult{Hash: txHash}, nil
}

func normalizeDeletePieceIDs(op string, pieceIDs []sdktypes.BigInt) ([]sdktypes.BigInt, error) {
	if len(pieceIDs) == 0 {
		return nil, fmt.Errorf("%s: %w: no piece IDs provided", op, ErrInvalidArgument)
	}
	normalized := make([]sdktypes.BigInt, 0, len(pieceIDs))
	seen := make(map[uint64]struct{}, len(pieceIDs))
	for i, pieceID := range pieceIDs {
		id, ok := pieceID.Uint64()
		if !ok || id > math.MaxInt64 {
			return nil, fmt.Errorf("%s: %w: pieceID at index %d is outside Curio's supported range 0..%d", op, ErrInvalidArgument, i, int64(math.MaxInt64))
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, pieceID.Copy())
	}
	if len(normalized) > pdp.MaxDeletePiecesBatchSize {
		return nil, fmt.Errorf("%s: %w: %w: got %d, max %d", op, ErrInvalidArgument, pdp.ErrTooManyPieces, len(normalized), pdp.MaxDeletePiecesBatchSize)
	}
	return normalized, nil
}

// encodeSignatureExtraData wraps a raw 65-byte signature as
// abi.encode(["bytes"], [sig]).
func encodeSignatureExtraData(sig []byte) ([]byte, error) {
	out, err := bytesArgs.Pack(sig)
	if err != nil {
		return nil, fmt.Errorf("encode signature extraData: %w", err)
	}
	return out, nil
}
