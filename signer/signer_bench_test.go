package signer

import (
	"crypto/rand"
	"testing"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	blst "github.com/supranational/blst/bindings/go"
)

var benchMsg = []byte("benchmark message for signing hot path")

func BenchmarkSecp256k1Sign(b *testing.B) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		b.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := s.Sign(benchMsg); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSecp256k1SignHash(b *testing.B) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		b.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		b.Fatal(err)
	}
	var hash [32]byte
	if _, err := rand.Read(hash[:]); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := s.SignHash(hash[:]); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBLSSign(b *testing.B) {
	var ikm [32]byte
	if _, err := rand.Read(ikm[:]); err != nil {
		b.Fatal(err)
	}
	sk := blst.KeyGen(ikm[:])
	if sk == nil {
		b.Fatal("failed to generate BLS key")
	}
	raw := sk.Serialize()
	s, err := NewBLSSigner(raw)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := s.Sign(benchMsg); err != nil {
			b.Fatal(err)
		}
	}
}
