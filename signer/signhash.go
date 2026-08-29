package signer

import "errors"

// ErrUnsupportedSigner is returned when an EVMSigner does not implement
// [HashSigner].
var ErrUnsupportedSigner = errors.New("signer: raw hash signing not supported by this signer")

// HashSigner signs a pre-computed 32-byte digest and returns a 65-byte
// Ethereum R‖S‖V signature. V may be encoded as 0/1 or 27/28. Implementations
// must return an error for any other digest length.
//
// This is a high-trust capability because the signer receives only a digest
// and cannot verify the original message or its domain separation. External
// implementations should dedicate the key to the intended SDK authorization
// flows and restrict access instead of exposing a general-purpose signing
// oracle.
type HashSigner interface {
	SignHash(hash []byte) ([]byte, error)
}

// SignHash signs a pre-computed 32-byte hash using the underlying secp256k1
// key, returning a 65-byte R‖S‖V signature. User code should prefer one of the
// higher-level APIs that constructs and domain-separates the message.
//
// SignHash returns [ErrUnsupportedSigner] when s does not implement
// [HashSigner]. Wrappers and external signers are supported when they expose
// that capability.
func SignHash(s EVMSigner, hash []byte) ([]byte, error) {
	hs, ok := s.(HashSigner)
	if !ok {
		return nil, ErrUnsupportedSigner
	}
	return hs.SignHash(hash)
}
