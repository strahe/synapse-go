package signer

import (
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
	blake2b "github.com/minio/blake2b-simd"
	blst "github.com/supranational/blst/bindings/go"
)

func makeTestLotusExport(t *testing.T, keyType string, raw []byte) string {
	t.Helper()

	ki := lotusKeyInfo{Type: keyType, PrivateKey: raw}
	j, err := json.Marshal(ki)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return hex.EncodeToString(j)
}

//nolint:staticcheck // tests intentionally mutate secp256k1 D to verify defensive copies.
func testSecp256k1Scalar(key *ecdsa.PrivateKey) *big.Int {
	return key.D
}

func TestSecp256k1Signer_DualProtocol(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}

	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}

	// Filecoin address should be secp256k1 protocol
	if s.FilecoinAddress().Protocol() != address.SECP256K1 {
		t.Errorf("expected secp256k1 address, got protocol %d", s.FilecoinAddress().Protocol())
	}

	// EVM address should match go-ethereum derivation
	expectedEth := ethcrypto.PubkeyToAddress(key.PublicKey)
	if s.EVMAddress() != expectedEth {
		t.Errorf("EVMAddress() = %s, want %s", s.EVMAddress(), expectedEth)
	}

	// Sign a Filecoin message
	msg := []byte("test message")
	sig, err := s.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Type != crypto.SigTypeSecp256k1 {
		t.Errorf("signature type = %d, want %d", sig.Type, crypto.SigTypeSecp256k1)
	}
	if len(sig.Data) != 65 {
		t.Errorf("signature length = %d, want 65", len(sig.Data))
	}

	// Create an EVM transactor
	opts, err := s.Transactor(big.NewInt(314159))
	if err != nil {
		t.Fatal(err)
	}
	if opts.From != expectedEth {
		t.Errorf("Transactor.From = %s, want %s", opts.From, expectedEth)
	}
}

func TestSecp256k1Signer_SignatureRecovery(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("verify me")
	sig, err := s.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	// Recover the public key from the Filecoin signature.
	// Filecoin uses blake2b-256 hash, signature is R|S|V (65 bytes).
	hash := blake2b.Sum256(msg)
	expectedSig, err := ethcrypto.Sign(hash[:], key)
	if err != nil {
		t.Fatalf("crypto.Sign: %v", err)
	}
	if hex.EncodeToString(sig.Data) != hex.EncodeToString(expectedSig) {
		t.Fatalf("signature mismatch: got %x want %x", sig.Data, expectedSig)
	}

	recovered, err := ethcrypto.SigToPub(hash[:], sig.Data)
	if err != nil {
		t.Fatalf("SigToPub: %v", err)
	}

	// Recovered public key should derive the same Filecoin address
	recoveredAddr, err := address.NewSecp256k1Address(ethcrypto.FromECDSAPub(recovered))
	if err != nil {
		t.Fatal(err)
	}
	if recoveredAddr != s.FilecoinAddress() {
		t.Errorf("recovered address = %s, want %s", recoveredAddr, s.FilecoinAddress())
	}
}

func TestSecp256k1Signer_FromBytes(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	raw := ethcrypto.FromECDSA(key)

	s, err := NewSecp256k1SignerFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}

	expectedEth := ethcrypto.PubkeyToAddress(key.PublicKey)
	if s.EVMAddress() != expectedEth {
		t.Errorf("EVMAddress() = %s, want %s", s.EVMAddress(), expectedEth)
	}
}

func TestSecp256k1Signer_PadsShortKeys(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	raw := ethcrypto.FromECDSA(key)

	s1, err := NewSecp256k1SignerFromBytes(raw)
	if err != nil {
		t.Fatal(err)
	}

	// big.Int.Bytes() may drop leading zeros
	short := new(big.Int).SetBytes(raw).Bytes()
	s2, err := NewSecp256k1SignerFromBytes(short)
	if err != nil {
		t.Fatal(err)
	}

	if s1.EVMAddress() != s2.EVMAddress() {
		t.Errorf("EVM addresses differ: %s vs %s", s1.EVMAddress(), s2.EVMAddress())
	}
	if s1.FilecoinAddress() != s2.FilecoinAddress() {
		t.Errorf("Filecoin addresses differ: %s vs %s", s1.FilecoinAddress(), s2.FilecoinAddress())
	}
}

func TestSecp256k1Signer_InvalidInputs(t *testing.T) {
	if _, err := NewSecp256k1Signer(nil); err == nil {
		t.Error("expected error for nil key")
	}
	if _, err := NewSecp256k1Signer(&ecdsa.PrivateKey{}); err == nil {
		t.Error("expected error for nil private scalar")
	}
	if _, err := NewSecp256k1SignerFromBytes(nil); err == nil {
		t.Error("expected error for nil bytes")
	}
	if _, err := NewSecp256k1SignerFromBytes(make([]byte, 33)); err == nil {
		t.Error("expected error for 33-byte key")
	}
	if _, err := NewSecp256k1SignerFromBytes([]byte{}); err == nil {
		t.Error("expected error for empty bytes")
	}
}

func TestBLSSigner_Sign(t *testing.T) {
	var ikm [32]byte
	copy(ikm[:], []byte("test-bls-key-seed-for-unit-test!"))

	sk := blst.KeyGen(ikm[:])
	if sk == nil {
		t.Fatal("failed to generate BLS key")
	}
	raw := sk.Serialize()

	s, err := NewBLSSigner(raw)
	if err != nil {
		t.Fatal(err)
	}

	if s.FilecoinAddress().Protocol() != address.BLS {
		t.Errorf("expected BLS address, got protocol %d", s.FilecoinAddress().Protocol())
	}

	msg := []byte("test message")
	sig, err := s.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}
	if sig.Type != crypto.SigTypeBLS {
		t.Errorf("signature type = %d, want %d", sig.Type, crypto.SigTypeBLS)
	}
	if len(sig.Data) != 96 { // compressed G2 point
		t.Errorf("signature length = %d, want 96", len(sig.Data))
	}
}

func TestBLSSigner_SignatureVerify(t *testing.T) {
	var ikm [32]byte
	copy(ikm[:], []byte("test-bls-key-seed-for-unit-test!"))
	sk := blst.KeyGen(ikm[:])
	raw := sk.Serialize()

	s, err := NewBLSSigner(raw)
	if err != nil {
		t.Fatal(err)
	}

	msg := []byte("verify this BLS message")
	sig, err := s.Sign(msg)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the signature using blst
	pk := new(blst.P1Affine).From(sk)
	sigPoint := new(blst.P2Affine).Uncompress(sig.Data)
	if sigPoint == nil {
		t.Fatal("failed to uncompress BLS signature")
	}
	if !sigPoint.Verify(true, pk, false, msg, []byte(blsDST)) {
		t.Error("BLS signature verification failed")
	}

	// Verify with wrong message should fail
	if sigPoint.Verify(true, pk, false, []byte("wrong message"), []byte(blsDST)) {
		t.Error("BLS signature should not verify with wrong message")
	}
}

func TestBLSSigner_NotEVM(t *testing.T) {
	var ikm [32]byte
	copy(ikm[:], []byte("test-bls-key-seed-for-unit-test!"))
	sk := blst.KeyGen(ikm[:])
	raw := sk.Serialize()

	s, err := NewBLSSigner(raw)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := AsEVM(s)
	if ok {
		t.Error("BLS signer should not satisfy EVMSigner")
	}
}

func TestBLSSigner_InvalidKey(t *testing.T) {
	if _, err := NewBLSSigner([]byte("too-short")); err == nil {
		t.Error("expected error for invalid BLS key")
	}
}

func TestFromLotusExport_Secp256k1(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	exported := makeTestLotusExport(t, "secp256k1", ethcrypto.FromECDSA(key))

	s, err := FromLotusExport(exported)
	if err != nil {
		t.Fatal(err)
	}

	if s.FilecoinAddress().Protocol() != address.SECP256K1 {
		t.Error("expected secp256k1 signer")
	}

	evm, ok := AsEVM(s)
	if !ok {
		t.Fatal("secp256k1 signer should satisfy EVMSigner")
	}

	expectedEth := ethcrypto.PubkeyToAddress(key.PublicKey)
	if evm.EVMAddress() != expectedEth {
		t.Errorf("EVMAddress() = %s, want %s", evm.EVMAddress(), expectedEth)
	}
}

func TestFromLotusExport_BLS(t *testing.T) {
	var ikm [32]byte
	copy(ikm[:], []byte("test-bls-key-seed-for-unit-test!"))
	sk := blst.KeyGen(ikm[:])
	raw := sk.Serialize() // big-endian (blst native)

	// Simulate Lotus export: little-endian byte order
	lotusBytes := make([]byte, len(raw))
	for i, b := range raw {
		lotusBytes[len(lotusBytes)-1-i] = b
	}

	exported := makeTestLotusExport(t, "bls", lotusBytes)
	s, err := FromLotusExport(exported)
	if err != nil {
		t.Fatal(err)
	}

	if s.FilecoinAddress().Protocol() != address.BLS {
		t.Error("expected BLS signer")
	}

	_, ok := AsEVM(s)
	if ok {
		t.Error("BLS signer should not satisfy EVMSigner")
	}

	// The derived address should match what we'd get from the original key
	directSigner, err := NewBLSSigner(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.FilecoinAddress() != directSigner.FilecoinAddress() {
		t.Errorf("Lotus import address = %s, direct address = %s; should match",
			s.FilecoinAddress(), directSigner.FilecoinAddress())
	}
}

func TestFromLotusExport_UnsupportedType(t *testing.T) {
	exported := makeTestLotusExport(t, "ed25519", []byte("dummy"))
	_, err := FromLotusExport(exported)
	if err == nil {
		t.Error("expected error for unsupported key type")
	}
}

func TestFromLotusExport_InvalidHex(t *testing.T) {
	_, err := FromLotusExport("not-valid-hex!")
	if err == nil {
		t.Error("expected error for invalid hex")
	}
}

func TestFromLotusExport_InvalidJSON(t *testing.T) {
	exported := hex.EncodeToString([]byte("not json"))
	_, err := FromLotusExport(exported)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestAsEVM_Secp256k1(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}

	evm, ok := AsEVM(s)
	if !ok {
		t.Fatal("secp256k1 signer should be EVMSigner")
	}
	if evm.EVMAddress() == (common.Address{}) {
		t.Error("EVMAddress should not be zero")
	}
}

// ---------------------------------------------------------------------------
// SignHash tests
// ---------------------------------------------------------------------------

func TestSignHash_ValidHash(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}

	hash := ethcrypto.Keccak256([]byte("test data"))
	sig, err := s.SignHash(hash)
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 65 {
		t.Errorf("signature length = %d, want 65", len(sig))
	}

	// Verify recovery matches the signer's address.
	recovered, err := ethcrypto.SigToPub(hash, sig)
	if err != nil {
		t.Fatal(err)
	}
	recoveredAddr := ethcrypto.PubkeyToAddress(*recovered)
	if recoveredAddr != s.EVMAddress() {
		t.Errorf("recovered %s, want %s", recoveredAddr, s.EVMAddress())
	}
}

func TestSignHash_WrongLength(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		hash []byte
	}{
		{"31 bytes", make([]byte, 31)},
		{"33 bytes", make([]byte, 33)},
		{"empty", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.SignHash(tt.hash)
			if err == nil {
				t.Error("expected error for wrong hash length")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Key-lifecycle is now the caller's responsibility; SDK no longer offers Zero.
// ---------------------------------------------------------------------------

func TestNewSecp256k1Signer_DeepCopiesKey(t *testing.T) {
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	scalar := testSecp256k1Scalar(key)
	origD := new(big.Int).Set(scalar)

	s, err := NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}
	// Mutate the caller-owned key; the signer must remain functional.
	scalar.SetInt64(0)

	if _, err := s.Sign([]byte("msg")); err != nil {
		t.Errorf("Sign should still work after caller mutates original key: %v", err)
	}
	// Original key restored for clarity (no longer required by the signer).
	scalar.Set(origD)
}
