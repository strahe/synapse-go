package piece

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"
)

// zeroPieceCidFixtures maps raw input sizes to their expected PieceCID outcomes.
// V1PieceCID is the Filecoin CIDv1 (codec=0xf101, sha2-256-trunc254-padded).
// Multiple raw sizes can share the same padded bucket and therefore the same v1 CID.
var zeroPieceCidFixtures = []struct {
	RawSize    int
	PaddedSize int
	V1PieceCID string
}{
	{96, 128, "baga6ea4seaqdomn3tgwgrh3g532zopskstnbrd2n3sxfqbze7rxt7vqn7veigmy"},
	{126, 128, "baga6ea4seaqdomn3tgwgrh3g532zopskstnbrd2n3sxfqbze7rxt7vqn7veigmy"},
	{127, 128, "baga6ea4seaqdomn3tgwgrh3g532zopskstnbrd2n3sxfqbze7rxt7vqn7veigmy"},
	{192, 256, "baga6ea4seaqgiktap34inmaex4wbs6cghlq5i2j2yd2bb2zndn5ep7ralzphkdy"},
	{253, 256, "baga6ea4seaqgiktap34inmaex4wbs6cghlq5i2j2yd2bb2zndn5ep7ralzphkdy"},
	{254, 256, "baga6ea4seaqgiktap34inmaex4wbs6cghlq5i2j2yd2bb2zndn5ep7ralzphkdy"},
	{255, 512, "baga6ea4seaqfpirydiugkk7up5v666wkm6n6jlw6lby2wxht5mwaqekerdfykjq"},
	{256, 512, "baga6ea4seaqfpirydiugkk7up5v666wkm6n6jlw6lby2wxht5mwaqekerdfykjq"},
	{384, 512, "baga6ea4seaqfpirydiugkk7up5v666wkm6n6jlw6lby2wxht5mwaqekerdfykjq"},
	{507, 512, "baga6ea4seaqfpirydiugkk7up5v666wkm6n6jlw6lby2wxht5mwaqekerdfykjq"},
	{508, 512, "baga6ea4seaqfpirydiugkk7up5v666wkm6n6jlw6lby2wxht5mwaqekerdfykjq"},
	{509, 1024, "baga6ea4seaqb66wjlfkrbye6uqoemcyxmqylwmrm235uclwfpsyx3ge2imidoly"},
	{512, 1024, "baga6ea4seaqb66wjlfkrbye6uqoemcyxmqylwmrm235uclwfpsyx3ge2imidoly"},
	{768, 1024, "baga6ea4seaqb66wjlfkrbye6uqoemcyxmqylwmrm235uclwfpsyx3ge2imidoly"},
	{1015, 1024, "baga6ea4seaqb66wjlfkrbye6uqoemcyxmqylwmrm235uclwfpsyx3ge2imidoly"},
	{1016, 1024, "baga6ea4seaqb66wjlfkrbye6uqoemcyxmqylwmrm235uclwfpsyx3ge2imidoly"},
	{1017, 2048, "baga6ea4seaqpy7usqklokfx2vxuynmupslkeutzexe2uqurdg5vhtebhxqmpqmy"},
	{1024, 2048, "baga6ea4seaqpy7usqklokfx2vxuynmupslkeutzexe2uqurdg5vhtebhxqmpqmy"},
}

// --------------------------------------------------------------------------
// PieceInfo (v2 boundary) tests — TDD: these tests define the NEW contract.
// --------------------------------------------------------------------------

// TestCalculate_ReturnsPieceInfo verifies that Calculate returns a fully-populated
// PieceInfo with CIDv1, CIDv2, and RawSize all set for inputs >= minPayloadSizeForV2.
// For inputs below the minimum piece size, CIDv1 and RawSize are set but CIDv2 is Undef.
func TestCalculate_ReturnsPieceInfo(t *testing.T) {
	for _, tc := range zeroPieceCidFixtures {
		t.Run(formatSize(tc.RawSize), func(t *testing.T) {
			data := make([]byte, tc.RawSize)
			info, err := Calculate(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Calculate(%d zero bytes): %v", tc.RawSize, err)
			}

			// CIDv1 must match fixture.
			if info.CIDv1.String() != tc.V1PieceCID {
				t.Errorf("CIDv1 mismatch for %d bytes:\n  got  %s\n  want %s",
					tc.RawSize, info.CIDv1.String(), tc.V1PieceCID)
			}

			// RawSize must match the input size exactly.
			if info.RawSize != uint64(tc.RawSize) {
				t.Errorf("RawSize mismatch for %d bytes: got %d", tc.RawSize, info.RawSize)
			}

			// PaddedSize derived from RawSize must match fixture.
			padded, err := PaddedSize(int64(info.RawSize))
			if err != nil {
				t.Fatalf("PaddedSize(%d): %v", info.RawSize, err)
			}
			if padded != int64(tc.PaddedSize) {
				t.Errorf("derived PaddedSize mismatch: got %d, want %d", padded, tc.PaddedSize)
			}

			// CIDv2 is only expected when the input meets the minimum piece size.
			if tc.RawSize >= minPayloadSizeForV2 {
				if !info.CIDv2.Defined() {
					t.Error("CIDv2 is undefined for input >= minPayloadSizeForV2")
				}
				if info.CIDv1.Equals(info.CIDv2) {
					t.Error("CIDv1 and CIDv2 must differ")
				}
			}
		})
	}
}

// TestCalculate_V2RoundTrip verifies that the v2 CID returned by Calculate can
// be decoded back to the original v1 CID and RawSize without loss.
func TestCalculate_V2RoundTrip(t *testing.T) {
	data := make([]byte, 256)
	info, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	decoded, err := ParseV2(info.CIDv2)
	if err != nil {
		t.Fatalf("ParseV2: %v", err)
	}

	if !decoded.CIDv1.Equals(info.CIDv1) {
		t.Errorf("v1 round-trip mismatch: got %s, want %s", decoded.CIDv1, info.CIDv1)
	}
	if decoded.RawSize != info.RawSize {
		t.Errorf("RawSize round-trip mismatch: got %d, want %d", decoded.RawSize, info.RawSize)
	}
	if !decoded.CIDv2.Equals(info.CIDv2) {
		t.Errorf("v2 round-trip mismatch: got %s, want %s", decoded.CIDv2, info.CIDv2)
	}
}

// TestParseV2_Valid ensures ParseV2 accepts a valid v2 CID and populates all fields.
func TestParseV2_Valid(t *testing.T) {
	// Build a valid v2 CID via Calculate.
	data := make([]byte, 512)
	for i := range data {
		data[i] = byte(i)
	}
	original, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	info, err := ParseV2(original.CIDv2)
	if err != nil {
		t.Fatalf("ParseV2: %v", err)
	}

	if !info.CIDv1.Equals(original.CIDv1) {
		t.Errorf("CIDv1 mismatch: got %s, want %s", info.CIDv1, original.CIDv1)
	}
	if info.RawSize != original.RawSize {
		t.Errorf("RawSize mismatch: got %d, want %d", info.RawSize, original.RawSize)
	}
}

// TestParseV2_RejectsV1 ensures ParseV2 rejects a v1 piece CID.
func TestParseV2_RejectsV1(t *testing.T) {
	v1Str := "baga6ea4seaqpy7usqklokfx2vxuynmupslkeutzexe2uqurdg5vhtebhxqmpqmy"
	v1, err := cid.Decode(v1Str)
	if err != nil {
		t.Fatalf("cid.Decode: %v", err)
	}
	_, err = ParseV2(v1)
	if err == nil {
		t.Fatal("expected error for v1 CID, got nil")
	}
	if !strings.Contains(err.Error(), "v2") {
		t.Errorf("error should mention 'v2', got: %v", err)
	}
}

// TestParseV2_RejectsArbitraryCID ensures ParseV2 rejects a non-piece CID.
func TestParseV2_RejectsArbitraryCID(t *testing.T) {
	c, err := cid.Decode("bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenera6qau")
	if err != nil {
		t.Fatalf("cid.Decode: %v", err)
	}
	_, err = ParseV2(c)
	if err == nil {
		t.Fatal("expected error for non-piece CID, got nil")
	}
}

func TestParseV2_RejectsWrongCodec(t *testing.T) {
	data := make([]byte, 512)
	for i := range data {
		data[i] = byte(i)
	}
	info, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	wrongCodec := cid.NewCidV1(uint64(multicodec.DagCbor), info.CIDv2.Hash())
	_, err = ParseV2(wrongCodec)
	if !errors.Is(err, ErrNotV2) {
		t.Fatalf("want ErrNotV2 for wrong codec, got %v", err)
	}
}

// TestPieceInfoFromV1 verifies that PieceInfoFromV1 constructs a correct PieceInfo
// from a v1 CID and a raw size.
func TestPieceInfoFromV1(t *testing.T) {
	data := make([]byte, 768)
	original, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	reconstructed, err := PieceInfoFromV1(original.CIDv1, original.RawSize)
	if err != nil {
		t.Fatalf("PieceInfoFromV1: %v", err)
	}

	if !reconstructed.CIDv1.Equals(original.CIDv1) {
		t.Errorf("CIDv1 mismatch")
	}
	if !reconstructed.CIDv2.Equals(original.CIDv2) {
		t.Errorf("CIDv2 mismatch: got %s, want %s", reconstructed.CIDv2, original.CIDv2)
	}
	if reconstructed.RawSize != original.RawSize {
		t.Errorf("RawSize mismatch")
	}
}

// TestPieceInfoFromV1_ZeroSizeError ensures PieceInfoFromV1 rejects zero raw size.
func TestPieceInfoFromV1_ZeroSizeError(t *testing.T) {
	data := make([]byte, 128)
	original, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	_, err = PieceInfoFromV1(original.CIDv1, 0)
	if err == nil {
		t.Fatal("expected error for zero rawSize, got nil")
	}
}

// TestPieceInfoFromV1_SubMinimum is a regression test for the round-trip invariant:
// Calculate returns CIDv2=Undef for rawSize in [1,126]; PieceInfoFromV1 must mirror
// that behaviour rather than propagating the library error from PieceCidV2FromV1.
func TestPieceInfoFromV1_SubMinimum(t *testing.T) {
	subMinSizes := []int{1, 50, 96, 126}
	for _, size := range subMinSizes {
		t.Run(formatSize(size), func(t *testing.T) {
			original, err := Calculate(bytes.NewReader(make([]byte, size)))
			if err != nil {
				t.Fatalf("Calculate(%d): %v", size, err)
			}
			// Sanity-check that Calculate itself leaves CIDv2 undefined for sub-minimum sizes.
			if original.CIDv2.Defined() {
				t.Fatalf("expected CIDv2=Undef from Calculate(%d), got %s", size, original.CIDv2)
			}

			// PieceInfoFromV1 must succeed and mirror Calculate semantics.
			reconstructed, err := PieceInfoFromV1(original.CIDv1, original.RawSize)
			if err != nil {
				t.Fatalf("PieceInfoFromV1(%d): unexpected error: %v", size, err)
			}
			if !reconstructed.CIDv1.Equals(original.CIDv1) {
				t.Errorf("CIDv1 mismatch: got %s, want %s", reconstructed.CIDv1, original.CIDv1)
			}
			if reconstructed.CIDv2.Defined() {
				t.Errorf("CIDv2 should be Undef for rawSize=%d, got %s", size, reconstructed.CIDv2)
			}
			if reconstructed.RawSize != original.RawSize {
				t.Errorf("RawSize mismatch: got %d, want %d", reconstructed.RawSize, original.RawSize)
			}
		})
	}
}

// TestCalculate_DifferentInputsDifferentCIDs verifies content-addressing: two different
// inputs of the same size produce different CIDs.
func TestCalculate_DifferentInputsDifferentCIDs(t *testing.T) {
	size := 256
	data1 := make([]byte, size)
	data2 := make([]byte, size)
	for i := range data2 {
		data2[i] = 0xff
	}

	info1, err := Calculate(bytes.NewReader(data1))
	if err != nil {
		t.Fatalf("Calculate(data1): %v", err)
	}
	info2, err := Calculate(bytes.NewReader(data2))
	if err != nil {
		t.Fatalf("Calculate(data2): %v", err)
	}

	if info1.CIDv1.Equals(info2.CIDv1) {
		t.Error("different data produced the same v1 CID")
	}
	if info1.CIDv2.Equals(info2.CIDv2) {
		t.Error("different data produced the same v2 CID")
	}
	// Same raw size → same padded size.
	if info1.RawSize != info2.RawSize {
		t.Errorf("same-size inputs have different RawSize: %d vs %d", info1.RawSize, info2.RawSize)
	}
}

// TestCalculate_Deterministic ensures the same input always produces the same PieceInfo.
func TestCalculate_Deterministic(t *testing.T) {
	// Use >= minPayloadSizeForV2 to also verify deterministic v2.
	data := make([]byte, 256)
	copy(data, []byte("hello filecoin piece commitment"))

	info1, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("first Calculate: %v", err)
	}
	info2, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("second Calculate: %v", err)
	}

	if !info1.CIDv1.Equals(info2.CIDv1) {
		t.Errorf("same data produced different v1 CIDs: %s vs %s", info1.CIDv1, info2.CIDv1)
	}
	if !info1.CIDv2.Equals(info2.CIDv2) {
		t.Errorf("same data produced different v2 CIDs: %s vs %s", info1.CIDv2, info2.CIDv2)
	}
	if info1.RawSize != info2.RawSize {
		t.Errorf("same data produced different RawSize: %d vs %d", info1.RawSize, info2.RawSize)
	}
}

// TestCalculate_Empty verifies that Calculate rejects an empty reader.
func TestCalculate_Empty(t *testing.T) {
	_, err := Calculate(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty reader, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

// TestCalculate_FirstReadReturnsDataAndError exercises the error path when the
// first Read call returns both data and a non-EOF error.
func TestCalculate_FirstReadReturnsDataAndError(t *testing.T) {
	reader := &partialErrorReader{
		data: []byte("x"),
		err:  io.ErrUnexpectedEOF,
	}

	_, err := Calculate(reader)
	if err == nil {
		t.Fatal("expected partial-read error, got nil")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// ExtractRoot / Validate tests (unchanged v1-level utilities).
// --------------------------------------------------------------------------

func TestExtractRoot(t *testing.T) {
	data := make([]byte, 96)
	info, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	root, err := ExtractRoot(info.CIDv1)
	if err != nil {
		t.Fatalf("ExtractRoot: %v", err)
	}

	allZero := true
	for _, b := range root {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Error("extracted root is all zeros, expected non-zero for piece commitment")
	}
}

func TestExtractRoot_InvalidCID(t *testing.T) {
	c, err := cid.Decode("bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenera6qau")
	if err != nil {
		t.Fatalf("failed to decode test CID: %v", err)
	}

	_, err = ExtractRoot(c)
	if err == nil {
		t.Fatal("expected error for non-piece CID, got nil")
	}
}

func TestValidate(t *testing.T) {
	t.Run("valid piece CID", func(t *testing.T) {
		data := make([]byte, 128)
		info, err := Calculate(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Calculate: %v", err)
		}
		if err := Validate(info.CIDv1); err != nil {
			t.Errorf("Validate returned error for valid piece CID: %v", err)
		}
	})

	t.Run("invalid codec", func(t *testing.T) {
		c, err := cid.Decode("bafkreihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenera6qau")
		if err != nil {
			t.Fatalf("failed to decode test CID: %v", err)
		}
		err = Validate(c)
		if err == nil {
			t.Fatal("expected error for wrong codec")
		}
		if !strings.Contains(err.Error(), "codec") {
			t.Errorf("error should mention 'codec', got: %v", err)
		}
	})

	t.Run("invalid hash function", func(t *testing.T) {
		data := []byte("test")
		_, err := cid.PrefixFromBytes([]byte{0x01, 0x81, 0xe2, 0x03, 0x12, 0x20}) // v1 + fil-commitment-unsealed + sha2-256
		if err != nil {
			// Fallback: build manually with CID builder.
			pref := cid.Prefix{
				Version:  1,
				Codec:    FilCommitmentUnsealed,
				MhType:   0x12, // sha2-256 (wrong hash function)
				MhLength: 32,
			}
			c, err := pref.Sum(data)
			if err != nil {
				t.Fatalf("failed to build test CID: %v", err)
			}
			err = Validate(c)
			if err == nil {
				t.Fatal("expected error for wrong hash function")
			}
			if !strings.Contains(err.Error(), "hash") {
				t.Errorf("error should mention 'hash', got: %v", err)
			}
			return
		}
		pref := cid.Prefix{
			Version:  1,
			Codec:    FilCommitmentUnsealed,
			MhType:   0x12,
			MhLength: 32,
		}
		c, err := pref.Sum(data)
		if err != nil {
			t.Fatalf("failed to build CID: %v", err)
		}
		err = Validate(c)
		if err == nil {
			t.Fatal("expected error for wrong hash function")
		}
	})

	t.Run("invalid digest length", func(t *testing.T) {
		digest := make([]byte, 31)
		rawMH := binary.AppendUvarint(nil, SHA2_256_Trunc254Padded)
		rawMH = binary.AppendUvarint(rawMH, uint64(len(digest)))
		rawMH = append(rawMH, digest...)
		c := cid.NewCidV1(FilCommitmentUnsealed, mh.Multihash(rawMH))

		err := Validate(c)
		if err == nil {
			t.Fatal("expected error for wrong digest length")
		}
		if !strings.Contains(err.Error(), "digest") {
			t.Errorf("error should mention 'digest', got: %v", err)
		}
	})
}

// --------------------------------------------------------------------------
// PaddedSize tests (unchanged helper).
// --------------------------------------------------------------------------

func TestPaddedSize(t *testing.T) {
	for _, tc := range zeroPieceCidFixtures {
		got, err := PaddedSize(int64(tc.RawSize))
		if err != nil {
			t.Errorf("PaddedSize(%d) unexpected error: %v", tc.RawSize, err)
			continue
		}
		if got != int64(tc.PaddedSize) {
			t.Errorf("PaddedSize(%d) = %d, want %d", tc.RawSize, got, tc.PaddedSize)
		}
	}

	tests := []struct {
		rawSize int64
		want    int64
	}{
		{0, 128},
		{-1, 128},
		{1, 128},
		{127, 128},
		{128, 256},
		{254, 256},
		{255, 512},
	}
	for _, tc := range tests {
		got, err := PaddedSize(tc.rawSize)
		if err != nil {
			t.Errorf("PaddedSize(%d) unexpected error: %v", tc.rawSize, err)
			continue
		}
		if got != tc.want {
			t.Errorf("PaddedSize(%d) = %d, want %d", tc.rawSize, got, tc.want)
		}
	}

	overflowInputs := []int64{math.MaxInt64, math.MaxInt64 - 125, math.MaxInt64 / 2}
	for _, in := range overflowInputs {
		if _, err := PaddedSize(in); !errors.Is(err, ErrRawSizeTooLarge) {
			t.Errorf("PaddedSize(%d) expected ErrRawSizeTooLarge, got err=%v", in, err)
		}
	}
}

// --------------------------------------------------------------------------
// CalculateFromBytes convenience wrapper tests.
// --------------------------------------------------------------------------

func TestCalculateFromBytes_Basic(t *testing.T) {
	data := make([]byte, 128)
	info, err := CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	if !info.CIDv1.Defined() {
		t.Error("CIDv1 is undefined")
	}
	if !info.CIDv2.Defined() {
		t.Error("CIDv2 is undefined")
	}
	if info.RawSize != 128 {
		t.Errorf("RawSize = %d, want 128", info.RawSize)
	}
}

// --------------------------------------------------------------------------
// helpers
// --------------------------------------------------------------------------

func formatSize(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	return fmt.Sprintf("%dKB", n/1024)
}

type partialErrorReader struct {
	data []byte
	err  error
	read bool
}

func (r *partialErrorReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	copy(p, r.data)
	return len(r.data), r.err
}
