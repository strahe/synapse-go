package pdp

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/redact"
)

// DownloadPiece calls GET /piece/{pieceCid} and returns a streaming
// io.ReadCloser plus the Content-Length (-1 when unknown). The caller must
// close the reader when done.
//
// Downloads have no total HTTP timeout by default. A custom streaming client's
// Timeout and the caller's context both bound the download lifetime.
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

	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.logger != nil {
		c.logger.Debug("pdp request", "method", req.Method, "url", redact.URL(req.URL))
	}

	resp, err := c.streamingHTTPClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("pdp: GET %s: %w", req.URL.Path, redact.URLError(err))
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
