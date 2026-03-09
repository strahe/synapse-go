package typeddata

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/ipfs/go-cid"
)

var (
	ErrInvalidSignatureLength = errors.New("signature must be 65 bytes")
	ErrInvalidRecoveryID      = errors.New("signature recovery id must be one of 0, 1, 27, 28")
)

// Sign signs an EIP-712 typed data message using the provided hash-signing
// function (typically signer.EVMSigner.SignHash).
func Sign(signHash func([]byte) ([]byte, error), domain apitypes.TypedDataDomain, primaryType string, message apitypes.TypedDataMessage) (*Signature, error) {
	typedData := apitypes.TypedData{
		Types:       Types,
		PrimaryType: primaryType,
		Domain:      domain,
		Message:     message,
	}

	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, fmt.Errorf("typeddata.Sign: hash domain: %w", err)
	}

	messageHash, err := typedData.HashStruct(primaryType, message)
	if err != nil {
		return nil, fmt.Errorf("typeddata.Sign: hash message: %w", err)
	}

	rawData := []byte{0x19, 0x01}
	rawData = append(rawData, domainSeparator...)
	rawData = append(rawData, messageHash...)
	digest := crypto.Keccak256(rawData)

	sig, err := signHash(digest)
	if err != nil {
		return nil, fmt.Errorf("typeddata.Sign: %w", err)
	}

	if len(sig) != 65 {
		return nil, fmt.Errorf("typeddata.Sign: %w", ErrInvalidSignatureLength)
	}

	// Do not mutate the caller-owned buffer; normalize V into a local.
	v := sig[64]
	if v < 27 {
		v += 27
	}
	if v != 27 && v != 28 {
		return nil, fmt.Errorf("typeddata.Sign: %w", ErrInvalidRecoveryID)
	}

	var r, s [32]byte
	copy(r[:], sig[:32])
	copy(s[:], sig[32:64])

	return &Signature{
		V: v,
		R: r,
		S: s,
	}, nil
}

// SignCreateDataSet signs a CreateDataSet EIP-712 message.
func SignCreateDataSet(signHash func([]byte) ([]byte, error), domain apitypes.TypedDataDomain, clientDataSetID *big.Int, payee common.Address, metadata []MetadataEntry) (*Signature, error) {
	msg := CreateDataSetMessage(clientDataSetID, payee, metadata)
	return Sign(signHash, domain, "CreateDataSet", msg)
}

// SignAddPieces signs an AddPieces EIP-712 message.
func SignAddPieces(signHash func([]byte) ([]byte, error), domain apitypes.TypedDataDomain, clientDataSetID, nonce *big.Int, pieceCIDs []cid.Cid, metadata [][]MetadataEntry) (*Signature, error) {
	msg, err := AddPiecesMessage(clientDataSetID, nonce, pieceCIDs, metadata)
	if err != nil {
		return nil, err
	}
	return Sign(signHash, domain, "AddPieces", msg)
}

// SignDeleteDataSet signs a DeleteDataSet EIP-712 message.
func SignDeleteDataSet(signHash func([]byte) ([]byte, error), domain apitypes.TypedDataDomain, clientDataSetID *big.Int) (*Signature, error) {
	msg := DeleteDataSetMessage(clientDataSetID)
	return Sign(signHash, domain, "DeleteDataSet", msg)
}

// SignSchedulePieceRemovals signs a SchedulePieceRemovals EIP-712 message.
func SignSchedulePieceRemovals(signHash func([]byte) ([]byte, error), domain apitypes.TypedDataDomain, clientDataSetID *big.Int, pieceIDs []*big.Int) (*Signature, error) {
	msg := SchedulePieceRemovalsMessage(clientDataSetID, pieceIDs)
	return Sign(signHash, domain, "SchedulePieceRemovals", msg)
}
