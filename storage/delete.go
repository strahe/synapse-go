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

// DeletePiece schedules removal of the piece identified by pieceCID from
// this context's data set. It matches
// packages/synapse-sdk/src/storage/context.ts:1070 deletePiece().
//
// The implementation:
//  1. Resolves the on-chain pieceID via PDPVerifier.findPieceIdsByCid.
//  2. Signs an EIP-712 SchedulePieceRemovals message over (clientDataSetID,
//     [pieceID]) and ABI-encodes the signature as bytes (matching TS
//     signSchedulePieceRemovals).
//  3. Calls the provider's DELETE /pdp/data-sets/{id}/pieces/{pieceId}
//     endpoint via PDPClient.SchedulePieceDeletion.
//
// The returned WriteResult carries only the transaction hash; there is no
// on-chain wait (mirroring TS which returns a bare Hash).
func (c *Context) DeletePiece(ctx context.Context, pieceCID cid.Cid) (*sdktypes.WriteResult, error) {
	if c.pdpCaller == nil {
		return nil, errors.New("storage.Context.DeletePiece: PDPVerifier reader not configured")
	}
	if c.client == nil {
		return nil, errors.New("storage.Context.DeletePiece: PDP client not configured")
	}
	if c.signer == nil {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w: nil signer", ErrInvalidArgument)
	}
	if c.dataSetID == nil {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w: dataSetID not set", ErrInvalidArgument)
	}
	if !c.chainID.IsValid() {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w: invalid chainID", ErrInvalidArgument)
	}
	if !pieceCID.Defined() {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w: undefined pieceCID", ErrInvalidArgument)
	}
	if c.recordKeeper == (common.Address{}) {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w: zero recordKeeper", ErrInvalidArgument)
	}

	pieceIDs, err := c.pdpCaller.FindPieceIdsByCid(ctx, *c.dataSetID, pieceCID, 0, 1)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w", err)
	}
	if len(pieceIDs) == 0 {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w: piece not found in data set", ErrInvalidArgument)
	}
	pieceID := pieceIDs[0]

	clientDataSetID, err := c.ensureClientDataSetID()
	if err != nil {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w", err)
	}

	domain := ityped.NewDomain(c.chainID.BigInt(), c.recordKeeper)
	sig, err := ityped.SignSchedulePieceRemovals(
		c.signHashFunc(),
		domain,
		clientDataSetID,
		[]*big.Int{new(big.Int).SetUint64(pieceID)},
	)
	if err != nil {
		if errors.Is(err, signer.ErrUnsupportedSigner) {
			return nil, fmt.Errorf("storage.Context.DeletePiece: wrapped/decorated EVMSigner values are unsupported: %w", err)
		}
		return nil, fmt.Errorf("storage.Context.DeletePiece: sign schedule removals: %w", err)
	}

	extraData, err := encodeSignatureExtraData(signatureBytes(sig))
	if err != nil {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w", err)
	}

	txHash, err := c.client.SchedulePieceDeletion(ctx, uint64(*c.dataSetID), pieceID, extraData)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.DeletePiece: %w", err)
	}

	return &sdktypes.WriteResult{Hash: txHash}, nil
}

// ensureClientDataSetID returns the context's client data set id. For a
// context bound to an existing data set (c.dataSetID != nil) the caller
// must have supplied the on-chain clientDataSetId explicitly via
// [WithClientDataSetID] — FWSS reconstructs the EIP-712 hash with the
// value it stored at create time, so a random value would yield a
// signature that the contract rejects. For new-dataset contexts a
// random 256-bit value is generated on first use and reused thereafter,
// matching the TS SDK's `randU256()` behaviour at create time.
func (c *Context) ensureClientDataSetID() (*big.Int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.clientDataSetID == nil {
		if c.dataSetID != nil {
			return nil, fmt.Errorf(
				"%w: clientDataSetID is required when the context targets an existing data set; "+
					"supply it with WithClientDataSetID or construct the context via Service.CreateContext",
				ErrInvalidArgument,
			)
		}
		v, err := randomClientDataSetID()
		if err != nil {
			return nil, err
		}
		c.clientDataSetID = v
	}
	return new(big.Int).Set(c.clientDataSetID), nil
}

// encodeSignatureExtraData wraps a raw 65-byte signature as
// abi.encode(["bytes"], [sig]), mirroring the TS helper used by
// signSchedulePieceRemovals.
func encodeSignatureExtraData(sig []byte) ([]byte, error) {
	args := abi.Arguments{{Type: contextBytesType}}
	out, err := args.Pack(sig)
	if err != nil {
		return nil, fmt.Errorf("encode schedule-removal extraData: %w", err)
	}
	return out, nil
}
