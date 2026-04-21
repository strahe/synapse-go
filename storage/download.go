package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	commpwriter "github.com/filecoin-project/go-commp-utils/v2/writer"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/piece"
)

// DownloadContext provides piece retrieval from a known storage provider.
type DownloadContext interface {
	Download(context.Context, cid.Cid) (io.ReadCloser, error)
}

// DownloadOptions configures a Service.Download call. Exactly one of Context
// or URL must be set; supplying both, or neither, returns an error wrapping
// [ErrInvalidDownloadOptions].
type DownloadOptions struct {
	Context DownloadContext // when set, delegates to DownloadContext.Download; mutually exclusive with URL
	URL     string          // direct HTTPS URL; validated against pieceCID on read completion
}

// ErrInvalidDownloadOptions is returned when [DownloadOptions] is nil, empty,
// or specifies more than one download source. Wrap with errors.Is to detect.
var ErrInvalidDownloadOptions = errors.New("storage: invalid download options")

// validatePieceCID returns nil if c is a valid PieceCIDv1 or PieceCIDv2, or
// an error that describes why c is not a piece CID.  Arbitrary non-piece CIDs
// (e.g. dag-pb, raw sha2-256) are rejected here so callers get a clear error
// instead of a confusing mismatch at the end of the download stream.
func validatePieceCID(c cid.Cid) error {
	if !c.Defined() {
		return errors.New("undefined pieceCID")
	}
	if piece.Validate(c) == nil {
		return nil // valid PieceCIDv1
	}
	if _, err := piece.ParseV2(c); err == nil {
		return nil // valid PieceCIDv2
	}
	return fmt.Errorf("not a piece CID (v1 or v2): %s", c)
}

// Download retrieves a piece by URL or via a DownloadContext.  When URL is
// used, the response body is streamed through a validating reader; the
// terminal read error from io.ReadAll (or any last Read call that returns
// io.EOF) carries the integrity check result — callers must not discard it.
func (s *Service) Download(ctx context.Context, pieceCID cid.Cid, opts *DownloadOptions) (io.ReadCloser, error) {
	if err := validatePieceCID(pieceCID); err != nil {
		return nil, fmt.Errorf("storage.Service.Download: %w", err)
	}
	if opts == nil {
		return nil, fmt.Errorf("storage.Service.Download: %w: options must not be nil", ErrInvalidDownloadOptions)
	}
	if opts.Context != nil && opts.URL != "" {
		return nil, fmt.Errorf("storage.Service.Download: %w: Context and URL are mutually exclusive", ErrInvalidDownloadOptions)
	}
	if opts.Context != nil {
		return opts.Context.Download(ctx, pieceCID)
	}
	if opts.URL == "" {
		return nil, fmt.Errorf("storage.Service.Download: %w: either Context or URL must be set", ErrInvalidDownloadOptions)
	}
	return s.downloadAndValidate(ctx, opts.URL, pieceCID)
}

// Download retrieves a piece from the storage provider.  Validation is
// streaming: the integrity check runs at EOF, so callers must inspect the
// terminal error returned by the last Read (or io.ReadAll).
//
// pieceCID must be a PieceCIDv2.  PieceCIDv1 is not accepted on this path
// because curio only accepts v2 and the raw size needed to normalise v1→v2 is
// not available here.  Use Service.Download with a URL if you only have v1.
func (c *Context) Download(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, error) {
	if _, err := piece.ParseV2(pieceCID); err != nil {
		return nil, fmt.Errorf("storage.Context.Download: PieceCIDv2 required: %w", err)
	}
	body, _, err := c.client.DownloadPiece(ctx, pieceCID)
	if err != nil {
		return nil, fmt.Errorf("storage.Context.Download: %w", err)
	}
	return newValidatingReadCloser(body, pieceCID), nil
}

func (s *Service) downloadAndValidate(ctx context.Context, rawURL string, pieceCID cid.Cid) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &DownloadError{URL: rawURL, Cause: err}
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, &DownloadError{URL: rawURL, Cause: err}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, &DownloadError{URL: rawURL, StatusCode: resp.StatusCode}
	}
	return newValidatingReadCloser(resp.Body, pieceCID), nil
}

type validatingReadCloser struct {
	reader   io.ReadCloser
	hasher   *commpwriter.Writer
	expected cid.Cid
	finished bool
	finalErr error
}

func newValidatingReadCloser(reader io.ReadCloser, expected cid.Cid) io.ReadCloser {
	return &validatingReadCloser{
		reader:   reader,
		hasher:   &commpwriter.Writer{},
		expected: expected,
	}
}

func (r *validatingReadCloser) Read(p []byte) (int, error) {
	if r.finished {
		return 0, r.finalErr
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		if _, writeErr := r.hasher.Write(p[:n]); writeErr != nil {
			r.finished = true
			r.finalErr = writeErr
			return n, writeErr
		}
	}
	switch {
	case errors.Is(err, io.EOF):
		r.finished = true
		r.finalErr = r.validate()
		if r.finalErr == nil {
			r.finalErr = io.EOF
		}
		if n > 0 {
			return n, nil
		}
		return 0, r.finalErr
	case err != nil:
		r.finished = true
		r.finalErr = err
	}
	return n, err
}

func (r *validatingReadCloser) Close() error {
	return r.reader.Close()
}

func (r *validatingReadCloser) validate() error {
	sum, err := r.hasher.Sum()
	if err != nil {
		return fmt.Errorf("storage: validate download piece: %w", err)
	}
	info, err := piece.PieceInfoFromV1(sum.PieceCID, uint64(sum.PayloadSize))
	if err != nil {
		return fmt.Errorf("storage: validate download piece: %w", err)
	}
	// Accept the caller-supplied CID in either v1 or v2 form.
	if r.expected == info.CIDv1 {
		return nil
	}
	if info.CIDv2.Defined() && r.expected == info.CIDv2 {
		return nil
	}
	return &CIDMismatchError{
		Expected:   r.expected,
		ComputedV1: info.CIDv1,
		ComputedV2: info.CIDv2,
	}
}
