package piece

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/filecoin-project/go-commp-utils/v2/writer"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"
)

const (
	// FilCommitmentUnsealed is the CID codec for Filecoin unsealed piece commitments.
	FilCommitmentUnsealed = 0xf101

	// SHA2_256_Trunc254Padded is the multihash code for Filecoin piece hashing.
	SHA2_256_Trunc254Padded = 0x1012
)

var (
	// ErrInvalidCodec is returned when a CID does not use the fil-commitment-unsealed codec.
	ErrInvalidCodec = errors.New("CID codec is not fil-commitment-unsealed (0xf101)")

	// ErrInvalidHashFunction is returned when a CID does not use sha2-256-trunc254-padded.
	ErrInvalidHashFunction = errors.New("multihash function is not sha2-256-trunc254-padded (0x1012)")

	// ErrInvalidDigestLength is returned when a PieceCID multihash digest is not 32 bytes.
	ErrInvalidDigestLength = errors.New("multihash digest length is not 32 bytes")

	// ErrEmptyInput is returned when Calculate receives an empty reader.
	ErrEmptyInput = errors.New("input data is empty")
)

// Calculate reads all data from r and computes the PieceCID (Filecoin piece commitment).
// Returns the PieceCID and the padded piece size. Returns error if r is empty.
func Calculate(r io.Reader) (cid.Cid, uint64, error) {
	// Read the first byte to ensure the reader is non-empty.
	var first [1]byte
	n, err := r.Read(first[:])
	firstReadErr := err
	if n == 0 {
		if err == nil || errors.Is(err, io.EOF) {
			return cid.Undef, 0, fmt.Errorf("piece.Calculate: %w", ErrEmptyInput)
		}
		return cid.Undef, 0, fmt.Errorf("piece.Calculate: %w", err)
	}

	w := &writer.Writer{}
	// Write the first byte we already read.
	if _, err := w.Write(first[:n]); err != nil {
		return cid.Undef, 0, fmt.Errorf("piece.Calculate: %w", err)
	}
	if firstReadErr != nil && !errors.Is(firstReadErr, io.EOF) {
		return cid.Undef, 0, fmt.Errorf("piece.Calculate: %w", firstReadErr)
	}

	// Copy remaining data.
	if _, err := io.Copy(w, r); err != nil {
		return cid.Undef, 0, fmt.Errorf("piece.Calculate: %w", err)
	}

	result, err := w.Sum()
	if err != nil {
		return cid.Undef, 0, fmt.Errorf("piece.Calculate: %w", err)
	}

	return result.PieceCID, uint64(result.PieceSize), nil
}

// ExtractRoot extracts the 32-byte merkle root from a PieceCID.
// Smart contracts use this root for piece verification.
func ExtractRoot(c cid.Cid) ([32]byte, error) {
	if err := Validate(c); err != nil {
		return [32]byte{}, fmt.Errorf("piece.ExtractRoot: %w", err)
	}

	decoded, err := multihash.Decode(c.Hash())
	if err != nil {
		return [32]byte{}, fmt.Errorf("piece.ExtractRoot: %w", err)
	}

	digest := decoded.Digest
	if len(digest) < 32 {
		return [32]byte{}, fmt.Errorf("piece.ExtractRoot: digest too short (%d bytes)", len(digest))
	}

	var root [32]byte
	copy(root[:], digest[len(digest)-32:])
	return root, nil
}

// Validate checks that a CID has the correct PieceCID format:
// codec=0xf101 (fil-commitment-unsealed), hash=0x1012 (sha2-256-trunc254-padded).
func Validate(c cid.Cid) error {
	if c.Type() != FilCommitmentUnsealed {
		return fmt.Errorf("piece.Validate: %w", ErrInvalidCodec)
	}

	decoded, err := multihash.Decode(c.Hash())
	if err != nil {
		return fmt.Errorf("piece.Validate: %w", err)
	}

	if decoded.Code != SHA2_256_Trunc254Padded {
		return fmt.Errorf("piece.Validate: %w", ErrInvalidHashFunction)
	}
	if len(decoded.Digest) != 32 {
		return fmt.Errorf("piece.Validate: %w", ErrInvalidDigestLength)
	}

	return nil
}

// ErrRawSizeTooLarge is returned when PaddedSize is called with a rawSize
// that would overflow int64 arithmetic during padded-size computation.
var ErrRawSizeTooLarge = errors.New("rawSize too large for padded size computation")

// PaddedSize returns the smallest power-of-two padded piece size (in bytes)
// that can hold rawSize bytes after FR32 padding (127 useful bytes per
// 128-byte leaf). Returns ErrRawSizeTooLarge for inputs near math.MaxInt64
// where the computation would otherwise silently overflow.
func PaddedSize(rawSize int64) (int64, error) {
	if rawSize <= 0 {
		return 128, nil
	}

	// Guard against rawSize+126 overflow.
	if rawSize > math.MaxInt64-126 {
		return 0, fmt.Errorf("piece.PaddedSize: %w (%d)", ErrRawSizeTooLarge, rawSize)
	}

	// Each 128-byte leaf holds 127 useful bytes due to FR32 padding.
	leavesNeeded := (rawSize + 126) / 127 // ceil(rawSize / 127)
	nextPow2 := nextPowerOfTwo(leavesNeeded)

	// Guard against nextPow2*128 overflow.
	if nextPow2 > math.MaxInt64/128 {
		return 0, fmt.Errorf("piece.PaddedSize: %w (%d)", ErrRawSizeTooLarge, rawSize)
	}

	return nextPow2 * 128, nil
}

// nextPowerOfTwo returns the smallest power of 2 >= n. Assumes n > 0.
func nextPowerOfTwo(n int64) int64 {
	if n <= 1 {
		return 1
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	return n + 1
}

// CalculateFromBytes is a convenience wrapper around Calculate for byte slices.
func CalculateFromBytes(data []byte) (cid.Cid, uint64, error) {
	return Calculate(bytes.NewReader(data))
}
