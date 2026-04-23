package signer

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
)

// FuzzSignHash ensures SignHash is total over arbitrary hash byte slices:
// the underlying secp256k1.Sign rejects non-32-byte hashes, but the wrapper
// must surface that as an error rather than panicking.
func FuzzSignHash(f *testing.F) {
	priv, err := crypto.GenerateKey()
	if err != nil {
		f.Fatalf("generate key: %v", err)
	}
	s, err := newSecp256k1(priv)
	if err != nil {
		f.Fatalf("newSecp256k1: %v", err)
	}

	f.Add([]byte{})
	f.Add([]byte("short"))
	f.Add(make([]byte, 31))
	f.Add(make([]byte, 32))
	f.Add(make([]byte, 33))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, hash []byte) {
		sig, err := SignHash(s, hash)
		if err != nil {
			return
		}
		if len(sig) != 65 {
			t.Fatalf("SignHash produced %d-byte signature; want 65 (input %d bytes)", len(sig), len(hash))
		}
	})
}
