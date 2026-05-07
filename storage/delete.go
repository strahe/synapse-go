package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	sdktypes "github.com/strahe/synapse-go/types"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/signer"
)

// DeletePiece schedules removal of the first piece matching pieceCID from
// this context's data set.
//
// A data set can contain multiple piece IDs with the same piece CID. This
// method resolves pieceCID with PDPVerifier.findPieceIdsByCid using limit=1
// and deletes the first returned piece ID. Prefer [Context.DeletePieceByID]
// when the on-chain piece ID is available.
//
// The implementation:
//  1. Resolves one on-chain pieceID via PDPVerifier.findPieceIdsByCid.
//  2. Signs an EIP-712 SchedulePieceRemovals message over (clientDataSetID,
//     pieceID) and ABI-encodes the signature as bytes.
//  3. Calls the provider's DELETE /pdp/data-sets/{id}/pieces/{pieceId}
//     endpoint via PDPProviderClient.SchedulePieceDeletion.
//
// The returned WriteResult carries only the transaction hash; there is no
// on-chain wait.
func (c *Context) DeletePiece(ctx context.Context, pieceCID cid.Cid) (*sdktypes.WriteResult, error) {
	const op = "storage.Context.DeletePiece"

	if c.pdpCaller == nil {
		return nil, errors.New(op + ": PDPVerifier reader not configured")
	}
	if !pieceCID.Defined() {
		return nil, fmt.Errorf("%s: %w: undefined pieceCID", op, ErrInvalidArgument)
	}
	target, err := c.snapshotDeletePieceTarget(op)
	if err != nil {
		return nil, err
	}

	pieceIDs, err := c.pdpCaller.FindPieceIdsByCid(ctx, target.dataSetID, pieceCID, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if len(pieceIDs) == 0 {
		return nil, fmt.Errorf("%s: %w: piece not found in data set", op, ErrInvalidArgument)
	}

	return c.schedulePieceDeletionByID(ctx, op, target, pieceIDs[0])
}

// DeletePieceByID schedules removal of the piece identified by its on-chain
// piece ID from this context's data set.
//
// Prefer this method when the piece ID is available, because piece CID is not
// guaranteed to be unique within a data set.
func (c *Context) DeletePieceByID(ctx context.Context, pieceID sdktypes.BigInt) (*sdktypes.WriteResult, error) {
	const op = "storage.Context.DeletePieceByID"

	target, err := c.snapshotDeletePieceTarget(op)
	if err != nil {
		return nil, err
	}

	return c.schedulePieceDeletionByID(ctx, op, target, pieceID)
}

type deletePieceTarget struct {
	dataSetID       sdktypes.BigInt
	clientDataSetID *big.Int
	chainID         sdktypes.ChainID
	recordKeeper    common.Address
}

func (c *Context) snapshotDeletePieceTarget(op string) (deletePieceTarget, error) {
	if c.client == nil {
		return deletePieceTarget{}, errors.New(op + ": PDP client not configured")
	}
	if c.signer == nil {
		return deletePieceTarget{}, fmt.Errorf("%s: %w: nil signer", op, ErrInvalidArgument)
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.dataSetID == nil {
		return deletePieceTarget{}, fmt.Errorf("%s: %w: dataSetID not set", op, ErrInvalidArgument)
	}
	if !c.chainID.IsValid() {
		return deletePieceTarget{}, fmt.Errorf("%s: %w: invalid chainID", op, ErrInvalidArgument)
	}
	if c.recordKeeper == (common.Address{}) {
		return deletePieceTarget{}, fmt.Errorf("%s: %w: zero recordKeeper", op, ErrInvalidArgument)
	}
	var clientDataSetID *big.Int
	if c.clientDataSetID != nil {
		clientDataSetID = c.clientDataSetID.Big()
	}

	return deletePieceTarget{
		dataSetID:       copyBigInt(*c.dataSetID),
		clientDataSetID: clientDataSetID,
		chainID:         c.chainID,
		recordKeeper:    c.recordKeeper,
	}, nil
}

func (c *Context) schedulePieceDeletionByID(ctx context.Context, op string, target deletePieceTarget, pieceID sdktypes.BigInt) (*sdktypes.WriteResult, error) {
	if target.clientDataSetID == nil {
		return nil, fmt.Errorf(
			"%s: %w: clientDataSetID is required when the context targets an existing data set; "+
				"supply it with WithClientDataSetID or construct the context via Service.CreateContext",
			op,
			ErrInvalidArgument,
		)
	}

	domain := ityped.NewDomain(target.chainID.BigInt(), target.recordKeeper)
	sig, err := ityped.SignSchedulePieceRemovals(
		c.signHashFunc(),
		domain,
		target.clientDataSetID,
		[]*big.Int{pieceID.Big()},
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

	txHash, err := c.client.SchedulePieceDeletion(ctx, target.dataSetID, pieceID, extraData)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	return &sdktypes.WriteResult{Hash: txHash}, nil
}

// encodeSignatureExtraData wraps a raw 65-byte signature as
// abi.encode(["bytes"], [sig]).
func encodeSignatureExtraData(sig []byte) ([]byte, error) {
	args := abi.Arguments{{Type: contextBytesType}}
	out, err := args.Pack(sig)
	if err != nil {
		return nil, fmt.Errorf("encode schedule-removal extraData: %w", err)
	}
	return out, nil
}
