package typeddata

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// TestSign_NormalizesHighSToLowS verifies that when a signer returns a
// non-canonical high-S signature, [Sign] rewrites it into the low-S form
// (s' = n - s) and flips the recovery bit so the resulting signature still
// recovers the original signer.
func TestSign_NormalizesHighSToLowS(t *testing.T) {
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	expectedAddr := crypto.PubkeyToAddress(key.PublicKey)
	curveN := secp256k1.S256().Params().N
	halfN := new(big.Int).Rsh(curveN, 1)

	// Wrap crypto.Sign so it returns a high-S signature: any time the
	// underlying signer produces a low-S, flip it to high-S, so we can be
	// sure typeddata.Sign exercises the normalization branch.
	highSSign := func(hash []byte) ([]byte, error) {
		sig, err := crypto.Sign(hash, key)
		if err != nil {
			return nil, err
		}
		s := new(big.Int).SetBytes(sig[32:64])
		if s.Cmp(halfN) <= 0 {
			s.Sub(curveN, s)
			sBytes := s.Bytes()
			for i := 32; i < 64; i++ {
				sig[i] = 0
			}
			copy(sig[64-len(sBytes):64], sBytes)
			sig[64] ^= 0x01 // flip recovery bit
		}
		return sig, nil
	}

	domain := testDomain()
	metadata := []MetadataEntry{{Key: "k", Value: "v"}}
	clientDataSetID := big.NewInt(1)

	sig, err := SignCreateDataSet(highSSign, domain, clientDataSetID, expectedAddr, metadata)
	if err != nil {
		t.Fatalf("SignCreateDataSet: %v", err)
	}

	// Output S must be low-S.
	sBig := new(big.Int).SetBytes(sig.S[:])
	if sBig.Cmp(halfN) > 0 {
		t.Fatalf("output S is not low-S: %x (halfN=%x)", sig.S, halfN.Bytes())
	}
	if sBig.Sign() == 0 {
		t.Fatal("output S is zero")
	}

	// Signature must still recover the original address (proving v was flipped correctly).
	recovered := recoverAddress(t, domain, "CreateDataSet",
		CreateDataSetMessage(clientDataSetID, expectedAddr, metadata), sig)
	if recovered != expectedAddr {
		t.Errorf("recovered = %s, want %s", recovered.Hex(), expectedAddr.Hex())
	}
}

// TestSign_LowSPassesThrough verifies that a signer returning a canonical
// low-S signature is left untouched (no spurious flip).
func TestSign_LowSPassesThrough(t *testing.T) {
	signHash, addr := testSignHash(t)
	domain := testDomain()
	metadata := []MetadataEntry{{Key: "k", Value: "v"}}
	clientDataSetID := big.NewInt(1)

	sig, err := SignCreateDataSet(signHash, domain, clientDataSetID, addr, metadata)
	if err != nil {
		t.Fatalf("SignCreateDataSet: %v", err)
	}
	halfN := new(big.Int).Rsh(secp256k1.S256().Params().N, 1)
	sBig := new(big.Int).SetBytes(sig.S[:])
	if sBig.Cmp(halfN) > 0 {
		t.Fatalf("output S is not low-S: %x", sig.S)
	}
	recovered := recoverAddress(t, domain, "CreateDataSet",
		CreateDataSetMessage(clientDataSetID, addr, metadata), sig)
	if recovered != addr {
		t.Errorf("recovered = %s, want %s", recovered.Hex(), addr.Hex())
	}
}
