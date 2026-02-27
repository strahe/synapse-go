package signer

import (
	"fmt"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	blst "github.com/supranational/blst/bindings/go"
)

const blsDST = "BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_"

// BLSSigner implements [Signer] backed by a BLS12-381 private key.
// BLS keys can only sign Filecoin messages; EVM operations are not supported.
type BLSSigner struct {
	sk      *blst.SecretKey
	filAddr address.Address
}

var _ Signer = (*BLSSigner)(nil)

// NewBLSSigner creates a Filecoin-only signer from raw BLS secret key bytes
// in big-endian order (the native blst serialization format, as produced by
// blst.SecretKey.Serialize). For Lotus-exported keys (little-endian), use
// [FromLotusExport] which handles the byte-order conversion automatically.
func NewBLSSigner(raw []byte) (*BLSSigner, error) {
	sk := new(blst.SecretKey).Deserialize(raw)
	if sk == nil {
		return nil, fmt.Errorf("signer: invalid BLS secret key")
	}

	pk := new(blst.P1Affine).From(sk).Compress()
	filAddr, err := address.NewBLSAddress(pk)
	if err != nil {
		return nil, fmt.Errorf("signer: deriving BLS address: %w", err)
	}

	return &BLSSigner{
		sk:      sk,
		filAddr: filAddr,
	}, nil
}

// FilecoinAddress returns the Filecoin BLS protocol address.
func (s *BLSSigner) FilecoinAddress() address.Address { return s.filAddr }

// Sign produces a BLS signature over the raw message bytes.
// Unlike secp256k1, BLS signing does not pre-hash the message.
func (s *BLSSigner) Sign(msg []byte) (*crypto.Signature, error) {
	sig := new(blst.P2Affine).Sign(s.sk, msg, []byte(blsDST))
	return &crypto.Signature{
		Type: crypto.SigTypeBLS,
		Data: sig.Compress(),
	}, nil
}
