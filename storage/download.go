package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sync"

	commpwriter "github.com/filecoin-project/go-commp-utils/v2/writer"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/redact"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

// DownloadContext retrieves pieces from a known storage provider.
type DownloadContext interface {
	Download(context.Context, cid.Cid) (io.ReadCloser, error)
}

// CDNRetriever retrieves pieces through an optional CDN for storage contexts.
type CDNRetriever interface {
	DownloadPiece(context.Context, cid.Cid) (io.ReadCloser, error)
}

// DataSetDownloader downloads a piece from a data set identified by ID. The
// root Client supplies [ServiceResolver], which checks ownership and CDN
// metadata before retrieving a piece, with an active provider as the fallback.
type DataSetDownloader interface {
	DownloadDataSet(context.Context, types.BigInt, cid.Cid) (io.ReadCloser, error)
}

// DownloadOptions configures a Service.Download call. Exactly one of Context,
// URL, or DataSetID must be set; other combinations return an error matching
// both [ErrInvalidDownloadOptions] and [ErrInvalidArgument].
type DownloadOptions struct {
	Context DownloadContext // delegates to DownloadContext.Download
	URL     string          // direct HTTP or HTTPS URL; validated against pieceCID on read completion
	// DataSetID downloads an owned data set's piece, using its CDN setting.
	// CDN retrieval can succeed even when the provider is inactive.
	DataSetID *types.BigInt
}

// ErrInvalidDownloadOptions is returned when [DownloadOptions] is nil, empty,
// specifies more than one download source, or sets a zero DataSetID.
// Service.Download wraps it together with [ErrInvalidArgument], so callers may
// match either.
var ErrInvalidDownloadOptions = errors.New("storage: invalid download options")

func invalidDownloadOptionsError(msg string) error {
	return fmt.Errorf("storage.Service.Download: %w: %w: %s", ErrInvalidArgument, ErrInvalidDownloadOptions, msg)
}

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

// Download retrieves a piece by URL, through a DownloadContext, or from a data
// set by ID. When URL is used, the response body is streamed through a
// validating reader; the terminal read error from io.ReadAll (or any last Read
// call that returns io.EOF) carries the integrity check result — callers must
// not discard it.
// URL downloads are capped by non-zero [Options.DownloadMaxBytes]. Exceeding
// the cap returns [ErrMaxBytesExceeded] either before streaming starts, when
// Content-Length is too large, or as the terminal Read error.
func (s *Service) Download(ctx context.Context, pieceCID cid.Cid, opts *DownloadOptions) (io.ReadCloser, error) {
	if err := s.checkInit(); err != nil {
		return io.ReadCloser(nil), err
	}
	if err := validatePieceCID(pieceCID); err != nil {
		return nil, fmt.Errorf("storage.Service.Download: %w: %w", ErrInvalidArgument, err)
	}
	if opts == nil {
		return nil, invalidDownloadOptionsError("options must not be nil")
	}
	sources := 0
	if opts.Context != nil {
		sources++
	}
	if opts.URL != "" {
		sources++
	}
	if opts.DataSetID != nil {
		sources++
	}
	if sources > 1 {
		return nil, invalidDownloadOptionsError("Context, URL, and DataSetID are mutually exclusive")
	}
	if sources == 0 {
		return nil, invalidDownloadOptionsError("exactly one of Context, URL, or DataSetID must be set")
	}
	if opts.DataSetID != nil {
		if opts.DataSetID.IsZero() {
			return nil, invalidDownloadOptionsError("DataSetID must not be zero")
		}
		if s.dataSetDownloader == nil {
			return nil, fmt.Errorf("storage.Service.Download: %w: DataSetDownloader not configured", ErrUninitialized)
		}
		return s.dataSetDownloader.DownloadDataSet(ctx, opts.DataSetID.Copy(), pieceCID)
	}
	if opts.Context != nil {
		return opts.Context.Download(ctx, pieceCID)
	}
	return s.downloadAndValidate(ctx, opts.URL, pieceCID)
}

// DownloadDataSet retrieves a PieceCIDv2 from a data set owned by the resolver's
// payer. A CDN-enabled data set is tried through CDNRetriever first, regardless
// of provider activity. Provider retrieval requires an active PDP product.
func (r *ServiceResolver) DownloadDataSet(ctx context.Context, dataSetID types.BigInt, pieceCID cid.Cid) (io.ReadCloser, error) {
	const op = "storage.ServiceResolver.DownloadDataSet"
	if r == nil || r.warmStorage == nil || r.spRegistry == nil || r.newContext == nil {
		return nil, fmt.Errorf("%s: %w", op, ErrUninitialized)
	}
	info, err := piece.ParseV2(pieceCID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: PieceCIDv2 required: %w", op, ErrInvalidArgument, err)
	}
	dataSetID = dataSetID.Copy()
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("%s: %w: zero dataSetID", op, ErrInvalidArgument)
	}
	dataSet, err := r.warmStorage.GetDataSet(ctx, dataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: get data set: %w", op, err)
	}
	if dataSet == nil {
		return nil, fmt.Errorf("%s: %w", op, ErrDataSetUnavailable)
	}
	if dataSet.Payer != r.payer {
		return nil, fmt.Errorf("%s: %w: data set is not owned by payer", op, ErrInvalidArgument)
	}
	metadata, err := r.warmStorage.GetAllDataSetMetadata(ctx, dataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: get metadata: %w", op, err)
	}
	var cdnErr error
	if _, withCDN := metadata["withCDN"]; withCDN && r.cdnRetriever != nil {
		var body io.ReadCloser
		body, cdnErr = r.cdnRetriever.DownloadPiece(ctx, pieceCID)
		if cdnErr == nil && body != nil {
			return newValidatingReadCloser(body, pieceCID, info.RawSize), nil
		}
		if body != nil {
			_ = body.Close()
		}
		if cdnErr == nil {
			cdnErr = errors.New("CDN retriever returned nil body")
		}
		if errors.Is(cdnErr, context.Canceled) || errors.Is(cdnErr, context.DeadlineExceeded) || ctx.Err() != nil {
			return nil, fmt.Errorf("%s: CDN: %w", op, errors.Join(cdnErr, ctx.Err()))
		}
	}
	provider, err := r.ResolveProvider(ctx, dataSet.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, errors.Join(cdnErr, err))
	}
	providerCtx, err := r.buildProviderContext(op, provider, ContextFactoryOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, errors.Join(cdnErr, err))
	}
	ref, err := NewDataSetRef(dataSet.ProviderID, dataSetID, dataSet.ClientDataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	target, err := providerCtx.ForDataSet(ref)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	body, err := target.Download(ctx, pieceCID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, errors.Join(cdnErr, err))
	}
	return body, nil
}

// Download retrieves a piece from CDN when enabled and available, otherwise
// from the storage provider. It returns [ErrMaxBytesExceeded], either
// immediately or from the reader, if the response exceeds the raw payload size
// encoded in pieceCID.
func (c *ProviderContext) Download(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, error) {
	return c.core.download(ctx, "storage.ProviderContext.Download", pieceCID)
}

// Download retrieves a piece from CDN when enabled and available, otherwise
// from the storage provider. It returns [ErrMaxBytesExceeded], either
// immediately or from the reader, if the response exceeds the raw payload size
// encoded in pieceCID.
func (c *DataSetContext) Download(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, error) {
	return c.core.download(ctx, "storage.DataSetContext.Download", pieceCID)
}

// download retrieves a piece from CDN when enabled and available, otherwise
// from the storage provider. Validation is streaming: the integrity check runs
// at EOF, so callers must inspect the terminal error returned by the last Read
// (or io.ReadAll).
//
// pieceCID must be a PieceCIDv2.  PieceCIDv1 is not accepted on this path
// because PDP provider only accepts v2 and the raw size needed to normalise v1→v2 is
// not available here.  Use Service.Download with a URL if you only have v1.
func (c *contextCore) download(ctx context.Context, op string, pieceCID cid.Cid) (io.ReadCloser, error) {
	info, err := piece.ParseV2(pieceCID)
	if err != nil {
		return nil, fmt.Errorf("%s: PieceCIDv2 required: %w", op, err)
	}
	if c.withCDN && c.cdnRetriever != nil {
		body, err := c.cdnRetriever.DownloadPiece(ctx, pieceCID)
		if err == nil && body != nil {
			return newValidatingReadCloser(body, pieceCID, info.RawSize), nil
		}
		if err == nil {
			err = errors.New("CDN retriever returned nil body")
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, fmt.Errorf("%s: CDN: %w", op, err)
		}
	}
	body, contentLength, err := c.client.DownloadPiece(ctx, pieceCID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if body == nil {
		return nil, fmt.Errorf("%s: PDP provider returned nil body", op)
	}
	if contentLength >= 0 && uint64(contentLength) > info.RawSize {
		_ = body.Close()
		return nil, fmt.Errorf("%s: %w: Content-Length %d > PieceCIDv2 raw size %d", op, ErrMaxBytesExceeded, contentLength, info.RawSize)
	}
	return newValidatingReadCloser(body, pieceCID, info.RawSize), nil
}

func (s *Service) downloadAndValidate(ctx context.Context, rawURL string, pieceCID cid.Cid) (io.ReadCloser, error) {
	safeURL := redact.URLString(rawURL)
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, &DownloadError{URL: safeURL, Cause: redact.URLError(err)}
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return nil, &DownloadError{URL: safeURL, Cause: fmt.Errorf("%w: %q", ErrUnsupportedScheme, parsed.Scheme)}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, &DownloadError{URL: safeURL, Cause: redact.URLError(err)}
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, &DownloadError{URL: safeURL, Cause: redact.URLError(err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, &DownloadError{URL: safeURL, StatusCode: resp.StatusCode}
	}
	if s.downloadMaxBytes > 0 && resp.ContentLength > s.downloadMaxBytes {
		_ = resp.Body.Close()
		return nil, &DownloadError{URL: safeURL, Cause: fmt.Errorf("%w: Content-Length %d > %d", ErrMaxBytesExceeded, resp.ContentLength, s.downloadMaxBytes)}
	}
	maxBytes := uint64(0)
	if s.downloadMaxBytes > 0 {
		maxBytes = uint64(s.downloadMaxBytes)
	}
	return newValidatingReadCloser(resp.Body, pieceCID, maxBytes), nil
}

type validatingReadCloser struct {
	mu       sync.Mutex
	reader   io.ReadCloser
	hasher   *commpwriter.Writer
	expected cid.Cid
	maxBytes uint64
	read     uint64
	finished bool
	finalErr error
}

func newValidatingReadCloser(reader io.ReadCloser, expected cid.Cid, maxBytes uint64) io.ReadCloser {
	return &validatingReadCloser{
		reader:   reader,
		hasher:   &commpwriter.Writer{},
		expected: expected,
		maxBytes: maxBytes,
	}
}

func (r *validatingReadCloser) Read(p []byte) (int, error) {
	r.mu.Lock()
	if r.finished {
		err := r.finalErr
		r.mu.Unlock()
		return 0, err
	}
	readBuffer := p
	if r.maxBytes > 0 {
		remaining := r.maxBytes - r.read
		if uint64(len(readBuffer)) > remaining {
			// Probe only the first byte beyond the cap. Because remaining is
			// smaller than len(readBuffer), this conversion cannot overflow.
			readBuffer = readBuffer[:int(remaining)+1]
		}
	}
	r.mu.Unlock()
	n, err := r.reader.Read(readBuffer)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished {
		if n > 0 {
			if r.maxBytes > 0 && uint64(n) > r.maxBytes-r.read {
				n = int(r.maxBytes - r.read)
			}
			return n, r.finalErr
		}
		return 0, r.finalErr
	}
	if n > 0 {
		if r.maxBytes > 0 && uint64(n) > r.maxBytes-r.read {
			// Trim written bytes so the hasher and the caller never see
			// the overflow: the hasher must be consistent with what the
			// caller observes, and the caller must stop at the cap.
			observed := uint64(n)
			observedOverflow := r.read > math.MaxUint64-observed
			if !observedOverflow {
				observed += r.read
			}
			n = int(r.maxBytes - r.read)
			r.read = r.maxBytes
			r.finished = true
			if observedOverflow {
				r.finalErr = fmt.Errorf("%w: read beyond cap %d", ErrMaxBytesExceeded, r.maxBytes)
			} else {
				r.finalErr = fmt.Errorf("%w: read %d bytes (cap %d)", ErrMaxBytesExceeded, observed, r.maxBytes)
			}
			if n > 0 {
				if _, writeErr := r.hasher.Write(p[:n]); writeErr != nil {
					r.finalErr = errors.Join(r.finalErr, writeErr)
				}
			}
			return n, r.finalErr
		}
		r.read += uint64(n)
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

// Close closes the underlying reader and marks the stream finished. If the
// caller closes before EOF, subsequent Reads return [io.ErrClosedPipe] so
// that partial data cannot silently masquerade as validated content.
func (r *validatingReadCloser) Close() error {
	closeErr := r.reader.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.finished {
		r.finished = true
		if r.finalErr == nil {
			r.finalErr = io.ErrClosedPipe
		}
	}
	return closeErr
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
