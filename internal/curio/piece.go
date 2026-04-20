package curio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/piece"
)

// UploadPieceStreamingOptions configures a single streaming upload.
type UploadPieceStreamingOptions struct {
	// Size is the payload byte count. When > 0 it is used to set the
	// Content-Length request header. When 0 the request is sent with
	// Transfer-Encoding: chunked.
	Size int64
	// PieceCID, when defined, is a pre-computed PieceCIDv2 of the payload.
	// If set, the client skips incremental commP calculation during the
	// streaming PUT. The server still verifies the uploaded bytes against
	// this value during finalize; a mismatch yields an HTTP error.
	PieceCID cid.Cid
	// OnProgress is invoked after each non-empty Read from the data reader,
	// with the cumulative byte count sent so far. It may be nil.
	OnProgress func(bytesUploaded int64)
}

// UploadStreamingResult is returned by UploadPieceStreaming on success.
type UploadStreamingResult struct {
	// PieceCID is the PieceCIDv2 of the uploaded piece — either the caller-
	// provided value (when opts.PieceCID was set) or the value computed
	// client-side during the streaming PUT.
	PieceCID cid.Cid
	// Size is the total byte count consumed from the data reader.
	Size int64
}

// UploadPieceStreaming uploads a piece using Curio's CommP-last 3-step
// streaming protocol. This is the preferred upload path: data is streamed
// to the server in a single pass while the PieceCID is computed inline
// (either by the client via TeeReader or by the caller in advance).
//
// Protocol:
//
//  1. POST /pdp/piece/uploads         — create upload session, get UUID
//  2. PUT  /pdp/piece/uploads/{uuid}  — stream body; commP is computed in
//     this pass (unless opts.PieceCID is pre-filled)
//  3. POST /pdp/piece/uploads/{uuid}  — finalize with the PieceCID;
//     server validates that the uploaded bytes hash to the same value
//
// The PUT clears the default Client timeout so large transfers are not
// capped at DefaultHTTPTimeout. Callers needing stricter limits should
// pass a context deadline or use a custom HTTP client.
//
// Mirrors TS synapse-core sp/upload-streaming.ts::uploadPieceStreaming.
func (c *Client) UploadPieceStreaming(
	ctx context.Context,
	data io.Reader,
	opts UploadPieceStreamingOptions,
) (*UploadStreamingResult, error) {
	if data == nil {
		return nil, errors.New("curio.UploadPieceStreaming: nil data reader")
	}
	if opts.Size > 0 && opts.Size > chain.MaxUploadSize {
		return nil, fmt.Errorf("curio.UploadPieceStreaming: payload size %d exceeds maximum %d bytes", opts.Size, chain.MaxUploadSize)
	}
	if opts.PieceCID.Defined() {
		if err := validatePieceCIDV2("curio.UploadPieceStreaming", opts.PieceCID); err != nil {
			return nil, err
		}
	}

	// Step 1: create upload session.
	createURL, err := c.resolve("pdp/piece/uploads")
	if err != nil {
		return nil, err
	}
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL.String(), nil)
	if err != nil {
		return nil, err
	}
	createResp, createBody, err := c.do(createReq, http.StatusCreated)
	if err != nil {
		return nil, err
	}
	loc := createResp.Header.Get("Location")
	uploadUUID := lastPathSegment(loc)
	if loc == "" || uploadUUID == "" {
		return nil, fmt.Errorf("%w: Location=%q body=%q", ErrLocationHeader, loc, string(createBody))
	}

	// Step 2: streaming PUT with optional inline commP computation.
	counted := &countingReader{r: data, max: chain.MaxUploadSize, onProgress: opts.OnProgress}
	var body io.Reader = counted
	var pw *piece.Writer
	if !opts.PieceCID.Defined() {
		pw = piece.NewWriter()
		body = io.TeeReader(counted, pw)
	}

	putURL, err := c.resolve(path.Join("pdp/piece/uploads", uploadUUID))
	if err != nil {
		return nil, err
	}
	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, putURL.String(), body)
	if err != nil {
		return nil, err
	}
	putReq.Header.Set("Content-Type", "application/octet-stream")
	if opts.Size > 0 {
		putReq.ContentLength = opts.Size
	}
	// Always clone and clear the timeout for the streaming PUT so large
	// transfers are not capped by the default client timeout. This mirrors
	// the pattern used in DownloadPiece; the context deadline is the sole
	// lifecycle authority for the duration of the transfer.
	putClient := &http.Client{Timeout: 0}
	if c.httpClient != nil {
		cloned := *c.httpClient
		cloned.Timeout = 0
		putClient = &cloned
	}
	if _, _, err := c.doWithClient(putClient, putReq, http.StatusNoContent); err != nil {
		return nil, fmt.Errorf("curio.UploadPieceStreaming: PUT: %w", err)
	}
	// Note: if the PUT fails, the upload session is left on the server.
	// Curio has no HTTP DELETE endpoint for sessions; orphaned sessions are
	// cleaned up by the server's background maintenance tasks.

	// Step 3: finalize with the PieceCID.
	var pieceCID cid.Cid
	if opts.PieceCID.Defined() {
		pieceCID = opts.PieceCID
	} else {
		info, err := pw.Sum()
		if err != nil {
			return nil, fmt.Errorf("curio.UploadPieceStreaming: compute piece: %w", err)
		}
		if !info.CIDv2.Defined() {
			return nil, fmt.Errorf("curio.UploadPieceStreaming: payload too small for PieceCIDv2 (%d bytes)", info.RawSize)
		}
		pieceCID = info.CIDv2
	}

	finalizeURL, err := c.resolve(path.Join("pdp/piece/uploads", uploadUUID))
	if err != nil {
		return nil, err
	}
	finalizeReq, err := buildJSONRequest(ctx, http.MethodPost, finalizeURL.String(), struct {
		PieceCID string `json:"pieceCid"`
	}{PieceCID: pieceCID.String()})
	if err != nil {
		return nil, err
	}
	if _, _, err := c.do(finalizeReq, http.StatusOK, http.StatusNoContent); err != nil {
		return nil, fmt.Errorf("curio.UploadPieceStreaming: finalize: %w", err)
	}

	return &UploadStreamingResult{PieceCID: pieceCID, Size: counted.n}, nil
}

// countingReader wraps an io.Reader to track the number of bytes read and
// optionally report progress via a callback. When max > 0, reads that push
// the cumulative total past max return an error.
type countingReader struct {
	r          io.Reader
	n          int64
	max        int64 // 0 = no limit
	onProgress func(int64)
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if n > 0 {
		cr.n += int64(n)
		if cr.max > 0 && cr.n > cr.max {
			return 0, fmt.Errorf("curio.UploadPieceStreaming: payload exceeds maximum size %d bytes", cr.max)
		}
		if cr.onProgress != nil {
			cr.onProgress(cr.n)
		}
	}
	return n, err
}

// FindPieceResult mirrors the JSON body of GET /pdp/piece?pieceCid=...
type FindPieceResult struct {
	PieceCID string `json:"pieceCid"`
}

// FindPiece calls GET /pdp/piece?pieceCid=.... It returns ErrPieceNotFound
// for HTTP 404 and ErrPieceProcessing for HTTP 202 while the SP is still
// parking the piece.
func (c *Client) FindPiece(ctx context.Context, pieceCID cid.Cid) (*FindPieceResult, error) {
	if err := validatePieceCIDV2("curio.FindPiece", pieceCID); err != nil {
		return nil, err
	}
	u, err := c.resolve("pdp/piece")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("pieceCid", pieceCID.String())
	u.RawQuery = q.Encode()

	resp, body, err := c.doRetryable(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	}, http.StatusOK, http.StatusNotFound, http.StatusAccepted)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrPieceNotFound
	}
	if resp.StatusCode == http.StatusAccepted {
		return nil, ErrPieceProcessing
	}
	var out FindPieceResult
	if err := jsonUnmarshal(body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// WaitForPieceParked polls FindPiece until the piece is found or the
// context is cancelled / timeout is reached. A zero pollInterval defaults
// to one second.
func (c *Client) WaitForPieceParked(ctx context.Context, pieceCID cid.Cid, pollInterval time.Duration) error {
	if err := validatePieceCIDV2("curio.WaitForPieceParked", pieceCID); err != nil {
		return err
	}
	if pollInterval <= 0 {
		pollInterval = time.Second
	}
	for {
		if _, err := c.FindPiece(ctx, pieceCID); err == nil {
			return nil
		} else if !errors.Is(err, ErrPieceNotFound) && !errors.Is(err, ErrPieceProcessing) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func validatePieceCIDV2(op string, pieceCID cid.Cid) error {
	if !pieceCID.Defined() {
		return fmt.Errorf("%s: undefined pieceCID", op)
	}
	if _, err := piece.ParseV2(pieceCID); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

// lastPathSegment returns the last non-empty path segment of a URL or
// path-like string. Returns "" if none is found.
func lastPathSegment(s string) string {
	s = strings.TrimRight(s, "/")
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Path != "" {
		s = strings.TrimRight(u.Path, "/")
	}
	idx := strings.LastIndex(s, "/")
	if idx < 0 {
		return s
	}
	return s[idx+1:]
}
