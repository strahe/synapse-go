package signer

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// lotusKeyInfo mirrors the JSON structure of a Lotus wallet export.
type lotusKeyInfo struct {
	Type       string `json:"Type"`
	PrivateKey []byte `json:"PrivateKey"`
}

func decodeLotusKey(exported string) (*lotusKeyInfo, error) {
	raw, err := hex.DecodeString(exported)
	defer func() {
		for i := range raw {
			raw[i] = 0
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("signer: decoding hex: %w", err)
	}
	var ki lotusKeyInfo
	if err := json.Unmarshal(raw, &ki); err != nil {
		return nil, fmt.Errorf("signer: unmarshaling key: %w", err)
	}
	return &ki, nil
}

// FromLotusExport creates a Signer from a Lotus wallet export string.
// The export format is a hex-encoded JSON object with Type and PrivateKey
// fields, as produced by `lotus wallet export`.
//
// BLS private keys from Lotus are little-endian (filecoin-ffi convention),
// while blst expects big-endian. This function handles the byte-order
// conversion automatically.
func FromLotusExport(exported string) (Signer, error) {
	ki, err := decodeLotusKey(exported)
	if err != nil {
		return nil, err
	}
	// Zero the decoded private key bytes after the signer constructor
	// deep-copies them. decodeLotusKey already zeros the raw JSON buffer,
	// but ki.PrivateKey is a separate allocation made by json.Unmarshal.
	defer func() {
		for i := range ki.PrivateKey {
			ki.PrivateKey[i] = 0
		}
	}()
	switch ki.Type {
	case "secp256k1":
		return NewSecp256k1SignerFromBytes(ki.PrivateKey)
	case "bls":
		// Lotus/filecoin-ffi stores BLS secret keys in little-endian order,
		// but blst.SecretKey.Deserialize expects big-endian. Reverse the bytes.
		reversed := make([]byte, len(ki.PrivateKey))
		defer func() {
			for i := range reversed {
				reversed[i] = 0
			}
		}()
		for i, b := range ki.PrivateKey {
			reversed[len(reversed)-1-i] = b
		}
		return NewBLSSigner(reversed)
	default:
		return nil, fmt.Errorf("signer: unsupported key type: %s", ki.Type)
	}
}
