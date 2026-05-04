package typeddata

import (
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/ipfs/go-cid"
)

var (
	ErrInvalidSignatureLength = errors.New("signature must be 65 bytes")
	ErrInvalidRecoveryID      = errors.New("signature recovery id must be one of 0, 1, 27, 28")
)

// secp256k1HalfN is N/2 of the secp256k1 curve order. ECDSA signatures whose
// s component lies above this threshold are considered non-canonical
// (high-S) and may be rejected by EIP-2 / EIP-712 verifiers as malleable.
var secp256k1HalfN = new(big.Int).Rsh(secp256k1.S256().Params().N, 1)

// Sign signs an EIP-712 typed data message using the provided hash-signing
// function (typically obtained via signer.SignHash).
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

	// Enforce low-S form (EIP-2). Some signers may return a signature whose
	// s value is above secp256k1n/2; verifiers conformant with EIP-2 reject
	// such signatures as malleable. Substitute s' = n - s and flip the
	// recovery bit so the resulting signature still recovers the same
	// public key but is in canonical form.
	sBig := new(big.Int).SetBytes(s[:])
	if sBig.Cmp(secp256k1HalfN) > 0 {
		sBig.Sub(secp256k1.S256().Params().N, sBig)
		sBytes := sBig.Bytes()
		// Re-pad to 32 bytes; SetBytes drops leading zeros.
		s = [32]byte{}
		copy(s[32-len(sBytes):], sBytes)
		if v == 27 {
			v = 28
		} else {
			v = 27
		}
	}

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
func SignDeleteDataSet(signHash func([]byte) ([]byte, error), domain apitypes.TypedDataDomain, dataSetID *big.Int) (*Signature, error) {
	msg := DeleteDataSetMessage(dataSetID)
	return Sign(signHash, domain, "DeleteDataSet", msg)
}

// SignSchedulePieceRemovals signs a SchedulePieceRemovals EIP-712 message.
func SignSchedulePieceRemovals(signHash func([]byte) ([]byte, error), domain apitypes.TypedDataDomain, clientDataSetID *big.Int, pieceIDs []*big.Int) (*Signature, error) {
	msg := SchedulePieceRemovalsMessage(clientDataSetID, pieceIDs)
	return Sign(signHash, domain, "SchedulePieceRemovals", msg)
}
