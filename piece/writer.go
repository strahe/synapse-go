package piece

import (
	"errors"
	"fmt"
	"io"

	"github.com/filecoin-project/go-commp-utils/v2/writer"
	commcid "github.com/filecoin-project/go-fil-commcid"
)

// Writer is a streaming PieceCID calculator. Data written to a Writer is
// hashed incrementally; call Sum once after the final Write to obtain the
// fully-populated PieceInfo.
//
// A Writer is not safe for concurrent use. Do not reuse a Writer after Sum
// has been called.
//
// Typical use is wrapping an upload body with io.TeeReader so the data is
// hashed in the same pass that sends it to the network:
//
//	w := piece.NewWriter()
//	body := io.TeeReader(src, w)
//	// ... stream body to HTTP PUT ...
//	info, err := w.Sum()
type Writer struct {
	w       *writer.Writer
	written int64
	done    bool
}

// NewWriter returns a new, ready-to-use streaming PieceCID calculator.
func NewWriter() *Writer {
	return &Writer{w: &writer.Writer{}}
}

// Write implements io.Writer. It is an error to Write after Sum has been called.
func (pw *Writer) Write(p []byte) (int, error) {
	if pw.done {
		return 0, errors.New("piece.Writer: Write after Sum")
	}
	n, err := pw.w.Write(p)
	pw.written += int64(n)
	return n, err
}

// Written returns the number of bytes written so far.
func (pw *Writer) Written() int64 { return pw.written }

// Sum finalizes the piece commitment and returns the corresponding
// PieceInfo. It may only be called once; subsequent calls return an error.
// Returns ErrEmptyInput if no bytes were written.
//
// CIDv1 is always populated. CIDv2 is populated when the written byte count
// is at least 127 (the minimum Filecoin piece payload size); for smaller
// inputs CIDv2 is cid.Undef, mirroring Calculate semantics.
func (pw *Writer) Sum() (PieceInfo, error) {
	if pw.done {
		return PieceInfo{}, errors.New("piece.Writer: Sum called more than once")
	}
	pw.done = true

	if pw.written == 0 {
		return PieceInfo{}, fmt.Errorf("piece.Writer.Sum: %w", ErrEmptyInput)
	}

	result, err := pw.w.Sum()
	if err != nil {
		return PieceInfo{}, fmt.Errorf("piece.Writer.Sum: %w", err)
	}

	v1 := result.PieceCID
	rawSize := uint64(result.PayloadSize)

	info := PieceInfo{CIDv1: v1, RawSize: rawSize}
	if rawSize >= minPayloadSizeForV2 {
		v2, err := commcid.PieceCidV2FromV1(v1, rawSize)
		if err != nil {
			return PieceInfo{}, fmt.Errorf("piece.Writer.Sum: build v2: %w", err)
		}
		info.CIDv2 = v2
	}
	return info, nil
}

// ensure Writer satisfies io.Writer.
var _ io.Writer = (*Writer)(nil)
