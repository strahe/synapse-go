package typeddata

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
)

func testSignHash(t *testing.T) (func([]byte) ([]byte, error), common.Address) {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	addr := crypto.PubkeyToAddress(key.PublicKey)
	return func(hash []byte) ([]byte, error) {
		return crypto.Sign(hash, key)
	}, addr
}

func testDomain() apitypes.TypedDataDomain {
	return NewDomain(big.NewInt(314159), common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"))
}

// recoverAddress recovers the signer address from a digest and Signature.
func recoverAddress(t *testing.T, domain apitypes.TypedDataDomain, primaryType string, message apitypes.TypedDataMessage, sig *Signature) common.Address {
	t.Helper()

	typedData := apitypes.TypedData{
		Types:       Types,
		PrimaryType: primaryType,
		Domain:      domain,
		Message:     message,
	}

	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		t.Fatal(err)
	}
	messageHash, err := typedData.HashStruct(primaryType, message)
	if err != nil {
		t.Fatal(err)
	}

	rawData := []byte{0x19, 0x01}
	rawData = append(rawData, domainSeparator...)
	rawData = append(rawData, messageHash...)
	digest := crypto.Keccak256(rawData)

	// Reconstruct 65-byte signature with recovery-compatible V
	var sigBytes [65]byte
	copy(sigBytes[:32], sig.R[:])
	copy(sigBytes[32:64], sig.S[:])
	sigBytes[64] = sig.V - 27

	pubKey, err := crypto.Ecrecover(digest, sigBytes[:])
	if err != nil {
		t.Fatal(err)
	}

	pk, err := crypto.UnmarshalPubkey(pubKey)
	if err != nil {
		t.Fatal(err)
	}

	return crypto.PubkeyToAddress(*pk)
}

func testCID(t *testing.T, data []byte) cid.Cid {
	t.Helper()
	hash, err := mh.Sum(data, mh.SHA2_256, -1)
	if err != nil {
		t.Fatal(err)
	}
	return cid.NewCidV1(cid.Raw, hash)
}

func TestNewDomain(t *testing.T) {
	chainID := big.NewInt(314159)
	addr := common.HexToAddress("0xABCDEF0123456789abcdef0123456789ABCDEF01")

	d := NewDomain(chainID, addr)

	if d.Name != "FilecoinWarmStorageService" {
		t.Errorf("Name = %q, want %q", d.Name, "FilecoinWarmStorageService")
	}
	if d.Version != "1" {
		t.Errorf("Version = %q, want %q", d.Version, "1")
	}
	if (*big.Int)(d.ChainId).Cmp(chainID) != 0 {
		t.Errorf("ChainId = %v, want %v", d.ChainId, (*math.HexOrDecimal256)(chainID))
	}
	if d.VerifyingContract != addr.Hex() {
		t.Errorf("VerifyingContract = %q, want %q", d.VerifyingContract, addr.Hex())
	}
}

func TestSign_CreateDataSet(t *testing.T) {
	signHash, addr := testSignHash(t)
	domain := testDomain()

	metadata := []MetadataEntry{{Key: "title", Value: "TestDataSet"}}
	sig, err := SignCreateDataSet(signHash, domain, big.NewInt(42), addr, metadata)
	if err != nil {
		t.Fatal(err)
	}

	if sig.V < 27 {
		t.Errorf("V = %d, want >= 27", sig.V)
	}

	msg := CreateDataSetMessage(big.NewInt(42), addr, metadata)
	recovered := recoverAddress(t, domain, "CreateDataSet", msg, sig)
	if recovered != addr {
		t.Errorf("recovered address %s != expected %s", recovered.Hex(), addr.Hex())
	}
}

func TestSign_AddPieces(t *testing.T) {
	signHash, addr := testSignHash(t)
	domain := testDomain()

	c1 := testCID(t, []byte("piece-1"))
	c2 := testCID(t, []byte("piece-2"))
	pieceCIDs := []cid.Cid{c1, c2}
	metadata := [][]MetadataEntry{
		{{Key: "name", Value: "piece1"}},
		{{Key: "name", Value: "piece2"}},
	}

	sig, err := SignAddPieces(signHash, domain, big.NewInt(1), big.NewInt(100), pieceCIDs, metadata)
	if err != nil {
		t.Fatal(err)
	}

	if sig.V < 27 {
		t.Errorf("V = %d, want >= 27", sig.V)
	}

	msg, err := AddPiecesMessage(big.NewInt(1), big.NewInt(100), pieceCIDs, metadata)
	if err != nil {
		t.Fatal(err)
	}
	recovered := recoverAddress(t, domain, "AddPieces", msg, sig)
	if recovered != addr {
		t.Errorf("recovered address %s != expected %s", recovered.Hex(), addr.Hex())
	}
}

func TestSign_DeleteDataSet(t *testing.T) {
	signHash, addr := testSignHash(t)
	domain := testDomain()

	sig, err := SignDeleteDataSet(signHash, domain, big.NewInt(99))
	if err != nil {
		t.Fatal(err)
	}

	if sig.V < 27 {
		t.Errorf("V = %d, want >= 27", sig.V)
	}

	msg := DeleteDataSetMessage(big.NewInt(99))
	if len(msg) != 1 {
		t.Fatalf("DeleteDataSetMessage fields=%d want 1", len(msg))
	}
	if _, ok := msg["dataSetId"]; !ok {
		t.Fatal("DeleteDataSetMessage missing dataSetId field")
	}
	if _, ok := msg["clientDataSetId"]; ok {
		t.Fatal("DeleteDataSetMessage includes clientDataSetId field")
	}
	recovered := recoverAddress(t, domain, "DeleteDataSet", msg, sig)
	if recovered != addr {
		t.Errorf("recovered address %s != expected %s", recovered.Hex(), addr.Hex())
	}
}

func TestSign_SchedulePieceRemovals(t *testing.T) {
	signHash, addr := testSignHash(t)
	domain := testDomain()

	pieceIDs := []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)}
	sig, err := SignSchedulePieceRemovals(signHash, domain, big.NewInt(10), pieceIDs)
	if err != nil {
		t.Fatal(err)
	}

	if sig.V < 27 {
		t.Errorf("V = %d, want >= 27", sig.V)
	}

	msg := SchedulePieceRemovalsMessage(big.NewInt(10), pieceIDs)
	recovered := recoverAddress(t, domain, "SchedulePieceRemovals", msg, sig)
	if recovered != addr {
		t.Errorf("recovered address %s != expected %s", recovered.Hex(), addr.Hex())
	}
}

func TestSign_DeterministicSignature(t *testing.T) {
	signHash, _ := testSignHash(t)
	domain := testDomain()

	sig1, err := SignDeleteDataSet(signHash, domain, big.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}

	sig2, err := SignDeleteDataSet(signHash, domain, big.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}

	if sig1.V != sig2.V || sig1.R != sig2.R || sig1.S != sig2.S {
		t.Error("same inputs produced different signatures")
	}
}

func TestSign_DifferentMessages(t *testing.T) {
	signHash, _ := testSignHash(t)
	domain := testDomain()

	sig1, err := SignDeleteDataSet(signHash, domain, big.NewInt(1))
	if err != nil {
		t.Fatal(err)
	}

	sig2, err := SignDeleteDataSet(signHash, domain, big.NewInt(2))
	if err != nil {
		t.Fatal(err)
	}

	if sig1.R == sig2.R && sig1.S == sig2.S && sig1.V == sig2.V {
		t.Error("different inputs produced identical signatures")
	}
}

func TestSign_InvalidSignatureLength(t *testing.T) {
	domain := testDomain()

	_, err := SignDeleteDataSet(func([]byte) ([]byte, error) {
		return make([]byte, 64), nil
	}, domain, big.NewInt(1))
	if err == nil {
		t.Fatal("expected error for invalid signature length")
	}
}

func TestSign_InvalidRecoveryID(t *testing.T) {
	signHash, _ := testSignHash(t)
	domain := testDomain()

	_, err := SignDeleteDataSet(func(hash []byte) ([]byte, error) {
		sig, err := signHash(hash)
		if err != nil {
			return nil, err
		}
		sig[64] = 2
		return sig, nil
	}, domain, big.NewInt(1))
	if err == nil {
		t.Fatal("expected error for invalid recovery id")
	}
	if !strings.Contains(err.Error(), "recovery") {
		t.Fatalf("error = %v, want recovery-id validation error", err)
	}
}
