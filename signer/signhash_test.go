package signer

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
)

// stubEVMSigner satisfies EVMSigner but not the internal hashSigner.
type stubEVMSigner struct{ EVMSigner }

func TestSignHash_OnSecp256k1(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}
	hash := make([]byte, 32)
	for i := range hash {
		hash[i] = byte(i)
	}
	sig, err := SignHash(s, hash)
	if err != nil {
		t.Fatalf("SignHash: %v", err)
	}
	if len(sig) != 65 {
		t.Errorf("expected 65-byte signature, got %d", len(sig))
	}

	recovered, err := ethcrypto.SigToPub(hash, sig)
	if err != nil {
		t.Fatalf("SigToPub: %v", err)
	}
	expectedEth := ethcrypto.PubkeyToAddress(key.PublicKey)
	recoveredAddr := ethcrypto.PubkeyToAddress(*recovered)
	if recoveredAddr != expectedEth {
		t.Errorf("recovered address = %s, want %s", recoveredAddr.Hex(), expectedEth.Hex())
	}
}

func TestSignHash_UnsupportedSigner(t *testing.T) {
	stub := &stubEVMSigner{}
	_, err := SignHash(stub, make([]byte, 32))
	if !errors.Is(err, ErrUnsupportedSigner) {
		t.Errorf("expected ErrUnsupportedSigner, got %v", err)
	}
}

// wrappedEVMSigner wraps an EVMSigner but does not implement hashSigner.
type wrappedEVMSigner struct {
	inner EVMSigner
}

func (w *wrappedEVMSigner) FilecoinAddress() address.Address           { return w.inner.FilecoinAddress() }
func (w *wrappedEVMSigner) Sign(msg []byte) (*crypto.Signature, error) { return w.inner.Sign(msg) }
func (w *wrappedEVMSigner) EVMAddress() common.Address                 { return w.inner.EVMAddress() }
func (w *wrappedEVMSigner) Transactor(chainID *big.Int) (*bind.TransactOpts, error) {
	return w.inner.Transactor(chainID)
}

func TestSignHash_WrappedSignerUnsupported(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}
	wrapped := &wrappedEVMSigner{inner: s}
	_, err = SignHash(wrapped, make([]byte, 32))
	if !errors.Is(err, ErrUnsupportedSigner) {
		t.Errorf("expected ErrUnsupportedSigner for wrapped signer, got %v", err)
	}
}
