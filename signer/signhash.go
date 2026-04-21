package signer

import "errors"

// ErrUnsupportedSigner is returned when an EVMSigner does not implement
// raw 32-byte hash signing. Only secp256k1-backed signers do.
//
// Note: Wrappers, decorators, or any type that does not directly implement
// the internal hashSigner interface are also unsupported. This boundary is
// intentional and enforced.
var ErrUnsupportedSigner = errors.New("signer: raw hash signing not supported by this signer")

// hashSigner is the internal interface satisfied by signers that can sign a
// pre-computed 32-byte hash. It is intentionally not exported and not part
// of EVMSigner: hash signing is dangerous (allows signing arbitrary
// messages, bypasses EIP-712 domain separation) and is reserved for
// internal SDK use such as EIP-712 typed-data signing.
//
// Only direct, concrete types implementing hashSigner are supported. Wrappers
// or decorators are not recognized and will be rejected.
type hashSigner interface {
	SignHash(hash []byte) ([]byte, error)
}

// SignHash signs a pre-computed 32-byte hash using the underlying secp256k1
// key, returning a 65-byte R‖S‖V signature. It is intended for internal SDK
// use (EIP-712 typed-data signing); user code should prefer Sign or one of
// the higher-level APIs that domain-separate the message.
//
// Returns ErrUnsupportedSigner if s does not back its key with secp256k1,
// or if s is a wrapper/decorator that does not directly implement hashSigner.
func SignHash(s EVMSigner, hash []byte) ([]byte, error) {
	hs, ok := s.(hashSigner)
	if !ok {
		return nil, ErrUnsupportedSigner
	}
	return hs.SignHash(hash)
}
