package pdp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/ipfs/go-cid"
)

// DownloadPiece calls GET /piece/{pieceCid} and returns a streaming
// io.ReadCloser plus the Content-Length (-1 when unknown). The caller must
// close the reader when done.
//
// The download bypasses the client's default JSON timeout so large pieces
// are not cut off; callers can enforce a deadline via the context.
func (c *Client) DownloadPiece(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, int64, error) {
	if err := validatePieceCIDV2("pdp.DownloadPiece", pieceCID); err != nil {
		return nil, 0, err
	}

	u, err := c.resolve(path.Join("piece", pieceCID.String()))
	if err != nil {
		return nil, 0, fmt.Errorf("pdp.DownloadPiece: resolve: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("pdp.DownloadPiece: build request: %w", err)
	}

	// Build a no-timeout HTTP client for streaming so the transport-level
	// deadline never fires — the context is the sole lifecycle authority.
	// Always clone to avoid mutating the caller's client.
	httpClient := &http.Client{Timeout: 0}
	if c.httpClient != nil {
		cloned := *c.httpClient
		cloned.Timeout = 0
		httpClient = &cloned
	}

	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.logger != nil {
		c.logger.Debug("pdp request", "method", req.Method, "url", redactURL(req.URL))
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("pdp: GET %s: %w", req.URL.Path, err)
	}

	if resp.StatusCode == http.StatusNotFound {
		_ = resp.Body.Close()
		return nil, 0, ErrPieceNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
		return nil, 0, newHTTPError(req, resp, body)
	}

	return resp.Body, resp.ContentLength, nil
}
