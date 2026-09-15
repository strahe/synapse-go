package typeddata

import (
	"encoding/hex"
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

func TestAddPiecesMessageCompactsOnlyFullyEmptyMetadata(t *testing.T) {
	pieceCIDs := []cid.Cid{
		testCID(t, []byte("piece-1")),
		testCID(t, []byte("piece-2")),
	}
	tests := []struct {
		name         string
		metadata     [][]MetadataEntry
		wantCount    int
		wantMetadata []int
		wantError    bool
	}{
		{name: "omitted", wantCount: 0},
		{name: "all empty", metadata: [][]MetadataEntry{{}, {}}, wantCount: 0},
		{
			name:         "mixed",
			metadata:     [][]MetadataEntry{{}, {{Key: "name", Value: "piece-2"}}},
			wantCount:    2,
			wantMetadata: []int{0, 1},
		},
		{
			name:         "all present",
			metadata:     [][]MetadataEntry{{{Key: "name", Value: "piece-1"}}, {{Key: "name", Value: "piece-2"}}},
			wantCount:    2,
			wantMetadata: []int{1, 1},
		},
		{name: "wrong length", metadata: [][]MetadataEntry{{}}, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, err := AddPiecesMessage(big.NewInt(1), big.NewInt(2), pieceCIDs, test.metadata)
			if test.wantError {
				if err == nil {
					t.Fatal("AddPiecesMessage error=nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("AddPiecesMessage: %v", err)
			}
			pieceMetadata, ok := message["pieceMetadata"].([]any)
			if !ok {
				t.Fatalf("pieceMetadata type=%T", message["pieceMetadata"])
			}
			if len(pieceMetadata) != test.wantCount {
				t.Fatalf("pieceMetadata len=%d want %d", len(pieceMetadata), test.wantCount)
			}
			for i, wantCount := range test.wantMetadata {
				entry, ok := pieceMetadata[i].(map[string]any)
				if !ok {
					t.Fatalf("pieceMetadata[%d] type=%T", i, pieceMetadata[i])
				}
				pieceIndex := (*big.Int)(entry["pieceIndex"].(*math.HexOrDecimal256))
				if pieceIndex.Cmp(big.NewInt(int64(i))) != 0 {
					t.Fatalf("pieceMetadata[%d].pieceIndex=%s", i, pieceIndex)
				}
				metadata, ok := entry["metadata"].([]any)
				if !ok || len(metadata) != wantCount {
					t.Fatalf("pieceMetadata[%d].metadata=%T len=%d want %d", i, entry["metadata"], len(metadata), wantCount)
				}
			}
		})
	}
}

func TestSignAddPiecesMetadataFreeUpstreamFixture(t *testing.T) {
	key, err := crypto.HexToECDSA("1234567890123456789012345678901234567890123456789012345678901234")
	if err != nil {
		t.Fatalf("HexToECDSA: %v", err)
	}
	pieceCIDs := make([]cid.Cid, 0, 2)
	for _, encoded := range []string{
		"bafkzcibcaac542av3szurbbscwuu3zjssvfwbpsvbjf6y3tukvlgl2nf5rha6pa",
		"bafkzcibcpybwiktap34inmaex4wbs6cghlq5i2j2yd2bb2zndn5ep7ralzphkdy",
	} {
		pieceCID, err := cid.Decode(encoded)
		if err != nil {
			t.Fatalf("Decode(%q): %v", encoded, err)
		}
		pieceCIDs = append(pieceCIDs, pieceCID)
	}
	domain := NewDomain(
		big.NewInt(314159),
		common.HexToAddress("0x02925630df557F957f70E112bA06e50965417CA0"),
	)
	signature, err := SignAddPieces(
		func(hash []byte) ([]byte, error) { return crypto.Sign(hash, key) },
		domain,
		big.NewInt(12345),
		big.NewInt(1),
		pieceCIDs,
		nil,
	)
	if err != nil {
		t.Fatalf("SignAddPieces: %v", err)
	}
	actual := make([]byte, 65)
	copy(actual[:32], signature.R[:])
	copy(actual[32:64], signature.S[:])
	actual[64] = signature.V
	const expected = "7b5f69b921e8b7b39652d384277ce73954a3154d70b0a64d309147a34bb9ce135690c1927e042e64bab7ffe3946084b81d807efc978ff47425da148a709cdf761b"
	if got := hex.EncodeToString(actual); got != expected {
		t.Fatalf("signature=%s want %s", got, expected)
	}
}

func TestSign_TerminateService(t *testing.T) {
	signHash, addr := testSignHash(t)
	domain := testDomain()

	sig, err := SignTerminateService(signHash, domain, big.NewInt(99))
	if err != nil {
		t.Fatal(err)
	}

	if sig.V < 27 {
		t.Errorf("V = %d, want >= 27", sig.V)
	}

	msg := TerminateServiceMessage(big.NewInt(99))
	if len(msg) != 1 {
		t.Fatalf("TerminateServiceMessage fields=%d want 1", len(msg))
	}
	if _, ok := msg["dataSetId"]; !ok {
		t.Fatal("TerminateServiceMessage missing dataSetId field")
	}
	if _, ok := msg["clientDataSetId"]; ok {
		t.Fatal("TerminateServiceMessage includes clientDataSetId field")
	}
	recovered := recoverAddress(t, domain, "TerminateService", msg, sig)
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

	sig1, err := SignTerminateService(signHash, domain, big.NewInt(7))
	if err != nil {
		t.Fatal(err)
	}

	sig2, err := SignTerminateService(signHash, domain, big.NewInt(7))
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

	sig1, err := SignTerminateService(signHash, domain, big.NewInt(1))
	if err != nil {
		t.Fatal(err)
	}

	sig2, err := SignTerminateService(signHash, domain, big.NewInt(2))
	if err != nil {
		t.Fatal(err)
	}

	if sig1.R == sig2.R && sig1.S == sig2.S && sig1.V == sig2.V {
		t.Error("different inputs produced identical signatures")
	}
}

func TestSign_InvalidSignatureLength(t *testing.T) {
	domain := testDomain()

	_, err := SignTerminateService(func([]byte) ([]byte, error) {
		return make([]byte, 64), nil
	}, domain, big.NewInt(1))
	if err == nil {
		t.Fatal("expected error for invalid signature length")
	}
}

func TestSign_InvalidRecoveryID(t *testing.T) {
	signHash, _ := testSignHash(t)
	domain := testDomain()

	_, err := SignTerminateService(func(hash []byte) ([]byte, error) {
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
