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
	mh "github.com/multiformats/go-multihash"
)

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

func TestCalculate_ZeroData(t *testing.T) {
	for _, tc := range zeroPieceCidFixtures {
		t.Run(formatSize(tc.RawSize), func(t *testing.T) {
			data := make([]byte, tc.RawSize)
			pieceCID, paddedSize, err := Calculate(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("Calculate(%d zero bytes): %v", tc.RawSize, err)
			}
			if pieceCID.String() != tc.V1PieceCID {
				t.Errorf("PieceCID mismatch for %d bytes:\n  got  %s\n  want %s", tc.RawSize, pieceCID.String(), tc.V1PieceCID)
			}
			if int(paddedSize) != tc.PaddedSize {
				t.Errorf("PaddedSize mismatch for %d bytes: got %d, want %d", tc.RawSize, paddedSize, tc.PaddedSize)
			}
		})
	}
}

func TestCalculate_NonZeroData(t *testing.T) {
	// Two different inputs of the same size must produce different CIDs.
	size := 256
	data1 := make([]byte, size)
	data2 := make([]byte, size)
	for i := range data2 {
		data2[i] = 0xff
	}

	cid1, ps1, err := Calculate(bytes.NewReader(data1))
	if err != nil {
		t.Fatalf("Calculate(data1): %v", err)
	}
	cid2, ps2, err := Calculate(bytes.NewReader(data2))
	if err != nil {
		t.Fatalf("Calculate(data2): %v", err)
	}

	if cid1.Equals(cid2) {
		t.Error("different data produced the same PieceCID")
	}
	if ps1 != ps2 {
		t.Errorf("same-size inputs got different padded sizes: %d vs %d", ps1, ps2)
	}
}

func TestCalculate_Deterministic(t *testing.T) {
	data := []byte("hello filecoin piece commitment")

	cid1, ps1, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("first Calculate: %v", err)
	}
	cid2, ps2, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("second Calculate: %v", err)
	}

	if !cid1.Equals(cid2) {
		t.Errorf("same data produced different CIDs: %s vs %s", cid1, cid2)
	}
	if ps1 != ps2 {
		t.Errorf("same data produced different padded sizes: %d vs %d", ps1, ps2)
	}
}

func TestCalculate_Empty(t *testing.T) {
	_, _, err := Calculate(bytes.NewReader(nil))
	if err == nil {
		t.Fatal("expected error for empty reader, got nil")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Errorf("error should mention 'empty', got: %v", err)
	}
}

func TestExtractRoot(t *testing.T) {
	// Use the first fixture to get a known PieceCID.
	data := make([]byte, 96)
	pieceCID, _, err := Calculate(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Calculate: %v", err)
	}

	root, err := ExtractRoot(pieceCID)
	if err != nil {
		t.Fatalf("ExtractRoot: %v", err)
	}

	// Root must be exactly 32 bytes (always true for [32]byte) and non-zero.
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
	// A raw CID (not a piece CID) should fail.
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
		pieceCID, _, err := Calculate(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("Calculate: %v", err)
		}
		if err := Validate(pieceCID); err != nil {
			t.Errorf("Validate returned error for valid piece CID: %v", err)
		}
	})

	t.Run("invalid codec", func(t *testing.T) {
		// Use a raw CID (codec 0x55).
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
		// Build a CID with the right codec but wrong hash.
		data := []byte("test")
		mh, err := cid.PrefixFromBytes([]byte{0x01, 0x81, 0xe2, 0x03, 0x12, 0x20}) // v1 + fil-commitment-unsealed + sha2-256
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
		c, err := mh.Sum(data)
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

func TestCalculate_FirstReadReturnsDataAndError(t *testing.T) {
	reader := &partialErrorReader{
		data: []byte("x"),
		err:  io.ErrUnexpectedEOF,
	}

	_, _, err := Calculate(reader)
	if err == nil {
		t.Fatal("expected partial-read error, got nil")
	}
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected io.ErrUnexpectedEOF, got: %v", err)
	}
}

func TestPaddedSize(t *testing.T) {
	// Verify against fixture padded sizes.
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

	// Edge cases.
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

	// Overflow guard: rawSize near math.MaxInt64 must return an error
	// rather than silently producing a tiny padded size.
	overflowInputs := []int64{math.MaxInt64, math.MaxInt64 - 125, math.MaxInt64 / 2}
	for _, in := range overflowInputs {
		if _, err := PaddedSize(in); !errors.Is(err, ErrRawSizeTooLarge) {
			t.Errorf("PaddedSize(%d) expected ErrRawSizeTooLarge, got err=%v", in, err)
		}
	}
}

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
