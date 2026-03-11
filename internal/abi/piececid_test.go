package abi

import (
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/piece"
)

func TestPieceCIDRoundTrip(t *testing.T) {
	c, _, err := piece.CalculateFromBytes([]byte("hello synapse go"))
	if err != nil {
		t.Fatal(err)
	}
	raw := EncodePieceCID(c)
	if len(raw.Data) == 0 {
		t.Fatal("empty encoded data")
	}
	// Mutate the encoded copy; a fresh encode should be untouched
	// (defensive copy contract).
	raw.Data[0] ^= 0xff
	raw2 := EncodePieceCID(c)
	if raw.Data[0] == raw2.Data[0] {
		t.Fatal("encode did not copy bytes")
	}
	got, err := DecodePieceCID(raw2)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equals(c) {
		t.Fatalf("cid mismatch: got %s want %s", got, c)
	}
}

func TestEncodePieceCID_Undef(t *testing.T) {
	raw := EncodePieceCID(cid.Undef)
	if raw.Data != nil {
		t.Fatalf("expected nil data for cid.Undef, got %x", raw.Data)
	}
}

func TestDecodePieceCID_Empty(t *testing.T) {
	if _, err := DecodePieceCID(pdpverifier.CidsCid{}); err == nil {
		t.Fatal("expected error for empty bytes")
	}
}

func TestDecodePieceCID_Invalid(t *testing.T) {
	if _, err := DecodePieceCID(pdpverifier.CidsCid{Data: []byte{0x01, 0x02}}); err == nil {
		t.Fatal("expected error for invalid CID bytes")
	}
}

func TestPieceCIDsBatch(t *testing.T) {
	c1, _, _ := piece.CalculateFromBytes([]byte("alpha"))
	c2, _, _ := piece.CalculateFromBytes([]byte("beta"))
	in := []cid.Cid{c1, c2}
	enc := EncodePieceCIDs(in)
	if len(enc) != 2 {
		t.Fatalf("len=%d", len(enc))
	}
	dec, err := DecodePieceCIDs(enc)
	if err != nil {
		t.Fatal(err)
	}
	if !dec[0].Equals(c1) || !dec[1].Equals(c2) {
		t.Fatal("batch mismatch")
	}
}

func TestDecodePieceCIDs_FirstInvalid(t *testing.T) {
	_, err := DecodePieceCIDs([]pdpverifier.CidsCid{{Data: []byte{0x01}}})
	if err == nil {
		t.Fatal("expected error")
	}
}
