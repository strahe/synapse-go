package piece

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	"github.com/multiformats/go-multihash"
)

const (
	// FilCommitmentUnsealed is the CID codec for Filecoin unsealed piece commitments (v1).
	FilCommitmentUnsealed = 0xf101

	// SHA2_256_Trunc254Padded is the multihash code for Filecoin v1 piece hashing.
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

	// ErrNotV2 is returned when ParseV2 receives a CID that is not PieceCIDv2.
	ErrNotV2 = errors.New("CID is not PieceCIDv2 (fr32-sha256-trunc254-padbintree)")

	// ErrZeroRawSize is returned when PieceInfoFromV1 is called with rawSize == 0.
	ErrZeroRawSize = errors.New("rawSize must be non-zero to construct PieceInfo from v1 CID")
)

// PieceInfo holds lossless piece information at the upload/download boundary.
//
// CIDv1 is the standard Filecoin piece commitment (codec=0xf101,
// multihash=sha2-256-trunc254-padded). It is the form used by smart contracts
// and on-chain storage.
//
// CIDv2 is the PieceCIDv2 (codec=0x55, multihash=fr32-sha256-trunc254-padbintree),
// which encodes the original raw (unpadded) byte count inside its multihash
// digest. It is the form used at the storage API boundary so that RawSize
// never needs to be recomputed from the original data.
//
// RawSize is the original unpadded byte count. It is encoded losslessly inside
// CIDv2 and is always equal to the number of bytes passed to Calculate.
//
// Use PaddedSize(int64(info.RawSize)) to obtain the corresponding padded piece size.
type PieceInfo struct {
	CIDv1   cid.Cid
	CIDv2   cid.Cid
	RawSize uint64
}

// minPayloadSizeForV2 is the minimum raw byte count for which PieceCIDv2 can
// be constructed. It corresponds to the minimum valid Filecoin piece size
// (128 bytes padded = 127 bytes unpadded).
const minPayloadSizeForV2 = 127

// Calculate reads all data from r, computes the piece commitment, and returns
// a fully-populated PieceInfo. Returns ErrEmptyInput if r is empty.
//
// CIDv1 is always populated. CIDv2 is populated when RawSize >= 127 (the
// minimum Filecoin piece payload size). For smaller inputs CIDv2 is cid.Undef
// because the fr32 encoding standard does not define pieces below 127 bytes.
//
// The returned PieceInfo carries both CIDv1 (for on-chain use / smart contracts)
// and CIDv2 (for the storage API boundary), plus the exact raw byte count.
func Calculate(r io.Reader) (PieceInfo, error) {
	w := NewWriter()
	if _, err := io.Copy(w, r); err != nil {
		return PieceInfo{}, fmt.Errorf("piece.Calculate: %w", err)
	}
	info, err := w.Sum()
	if err != nil {
		// Preserve the historical piece.Calculate error prefix.
		if errors.Is(err, ErrEmptyInput) {
			return PieceInfo{}, fmt.Errorf("piece.Calculate: %w", ErrEmptyInput)
		}
		return PieceInfo{}, fmt.Errorf("piece.Calculate: %w", err)
	}
	return info, nil
}

// CalculateFromBytes is a convenience wrapper around Calculate for byte slices.
func CalculateFromBytes(data []byte) (PieceInfo, error) {
	return Calculate(bytes.NewReader(data))
}

// ParseV2 decodes a PieceCIDv2 and returns the fully-populated PieceInfo.
// Returns ErrNotV2 if c is a v1 piece CID or any other non-v2 CID.
func ParseV2(c cid.Cid) (PieceInfo, error) {
	if c.Type() != uint64(multicodec.Raw) {
		return PieceInfo{}, fmt.Errorf("piece.ParseV2: %w", ErrNotV2)
	}
	if c.Prefix().MhType != uint64(multicodec.Fr32Sha256Trunc254Padbintree) {
		return PieceInfo{}, fmt.Errorf("piece.ParseV2: %w", ErrNotV2)
	}

	v1, rawSize, err := commcid.PieceCidV1FromV2(c)
	if err != nil {
		return PieceInfo{}, fmt.Errorf("piece.ParseV2: %w", err)
	}

	return PieceInfo{
		CIDv1:   v1,
		CIDv2:   c,
		RawSize: rawSize,
	}, nil
}

// PieceInfoFromV1 constructs a PieceInfo from a v1 piece CID and the known raw size.
// This is useful when reading from storage systems that persist v1 + size separately
// and need to reconstruct the full v2-aware PieceInfo.
//
// Returns ErrZeroRawSize if rawSize is 0. For raw sizes below 127 bytes, CIDv2 is
// returned as cid.Undef to mirror Calculate semantics.
func PieceInfoFromV1(v1 cid.Cid, rawSize uint64) (PieceInfo, error) {
	if rawSize == 0 {
		return PieceInfo{}, fmt.Errorf("piece.PieceInfoFromV1: %w", ErrZeroRawSize)
	}

	if err := Validate(v1); err != nil {
		return PieceInfo{}, fmt.Errorf("piece.PieceInfoFromV1: %w", err)
	}

	var v2 cid.Cid
	if rawSize >= minPayloadSizeForV2 {
		var err error
		v2, err = commcid.PieceCidV2FromV1(v1, rawSize)
		if err != nil {
			return PieceInfo{}, fmt.Errorf("piece.PieceInfoFromV1: %w", err)
		}
	}

	return PieceInfo{
		CIDv1:   v1,
		CIDv2:   v2,
		RawSize: rawSize,
	}, nil
}

// ExtractRoot extracts the 32-byte merkle root from a v1 PieceCID.
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

// Validate checks that a CID has the correct v1 PieceCID format:
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

var (
	// ErrNegativeRawSize is returned when PaddedSize is called with a
	// negative rawSize.
	ErrNegativeRawSize = errors.New("rawSize must be non-negative")

	// ErrRawSizeTooLarge is returned when PaddedSize is called with a rawSize
	// that would overflow int64 arithmetic during padded-size computation.
	ErrRawSizeTooLarge = errors.New("rawSize too large for padded size computation")
)

// PaddedSize returns the smallest power-of-two padded piece size (in bytes)
// that can hold rawSize bytes after FR32 padding (127 useful bytes per
// 128-byte leaf). Returns ErrNegativeRawSize for negative inputs and
// ErrRawSizeTooLarge for inputs near math.MaxInt64 where the computation
// would otherwise silently overflow.
func PaddedSize(rawSize int64) (int64, error) {
	if rawSize < 0 {
		return 0, fmt.Errorf("piece.PaddedSize: %w (%d)", ErrNegativeRawSize, rawSize)
	}
	if rawSize == 0 {
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
