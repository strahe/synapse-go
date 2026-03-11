package curio

import (
	"bytes"
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
)

// UploadPieceResult describes the outcome of POST /pdp/piece.
type UploadPieceResult struct {
	// AlreadyExists is true when the server responds 200 OK, meaning the
	// piece is already parked and no PUT is required.
	AlreadyExists bool
	// UploadUUID is the server-generated upload identifier parsed out of
	// the Location header when the server responds 201.
	UploadUUID string
}

// UploadPiece calls POST /pdp/piece to register a piece. If the server
// responds 201, the caller must follow up with UploadPieceBytes(ctx,
// result.UploadUUID, data). If it responds 200, the piece is already
// stored server-side.
//
// Mirrors TS synapse-core sp/upload.ts::uploadPiece.
func (c *Client) UploadPiece(ctx context.Context, pieceCID cid.Cid) (*UploadPieceResult, error) {
	if !pieceCID.Defined() {
		return nil, errors.New("curio.UploadPiece: undefined pieceCID")
	}
	u, err := c.resolve("pdp/piece")
	if err != nil {
		return nil, err
	}
	payload := struct {
		PieceCID string `json:"pieceCid"`
	}{PieceCID: pieceCID.String()}

	req, err := buildJSONRequest(ctx, http.MethodPost, u.String(), payload)
	if err != nil {
		return nil, err
	}

	resp, body, err := c.do(req, http.StatusOK, http.StatusCreated)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusOK {
		return &UploadPieceResult{AlreadyExists: true}, nil
	}
	// 201 → Location header carries .../pdp/piece/upload/{uuid}
	loc := resp.Header.Get("Location")
	uuid := lastPathSegment(loc)
	if loc == "" || uuid == "" {
		return nil, fmt.Errorf("%w: Location=%q body=%q", ErrLocationHeader, loc, string(body))
	}
	return &UploadPieceResult{UploadUUID: uuid}, nil
}

// UploadPieceBytes streams the piece bytes to PUT /pdp/piece/upload/{uuid}.
// contentLength MUST match the byte count the server expects (and was
// committed to during the POST /pdp/piece pre-registration).
//
// Uploads bypass the Client's default request timeout so large streaming
// transfers are not capped at DefaultHTTPTimeout. Callers that need stricter
// limits can pass a context deadline/cancelation or a custom HTTP client.
func (c *Client) UploadPieceBytes(ctx context.Context, uploadUUID string, data io.Reader, contentLength int64) error {
	if uploadUUID == "" {
		return errors.New("curio.UploadPieceBytes: empty uploadUUID")
	}
	if data == nil {
		return errors.New("curio.UploadPieceBytes: nil data reader")
	}
	if contentLength <= 0 {
		return fmt.Errorf("curio.UploadPieceBytes: invalid contentLength %d", contentLength)
	}
	u, err := c.resolve(path.Join("pdp/piece/upload", uploadUUID))
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), data)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = contentLength
	client := c.httpClient
	if client != nil && client.Timeout == DefaultHTTPTimeout {
		cloned := *client
		cloned.Timeout = 0
		client = &cloned
	}
	_, _, err = c.doWithClient(client, req, http.StatusNoContent, http.StatusOK, http.StatusCreated)
	return err
}

// UploadPieceFromBytes is a convenience wrapper that handles both the
// POST and the (optional) PUT in one call. When the piece already exists
// server-side, the bytes are not re-uploaded.
func (c *Client) UploadPieceFromBytes(ctx context.Context, pieceCID cid.Cid, data []byte) (*UploadPieceResult, error) {
	res, err := c.UploadPiece(ctx, pieceCID)
	if err != nil {
		return nil, err
	}
	if res.AlreadyExists {
		return res, nil
	}
	if err := c.UploadPieceBytes(ctx, res.UploadUUID, bytes.NewReader(data), int64(len(data))); err != nil {
		return nil, err
	}
	return res, nil
}

// FindPieceResult mirrors the JSON body of GET /pdp/piece?pieceCid=...
type FindPieceResult struct {
	PieceCID string `json:"pieceCid"`
}

// FindPiece calls GET /pdp/piece?pieceCid=.... It returns ErrPieceNotFound
// for HTTP 404 and ErrPieceProcessing for HTTP 202 while the SP is still
// parking the piece.
func (c *Client) FindPiece(ctx context.Context, pieceCID cid.Cid) (*FindPieceResult, error) {
	if !pieceCID.Defined() {
		return nil, errors.New("curio.FindPiece: undefined pieceCID")
	}
	u, err := c.resolve("pdp/piece")
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("pieceCid", pieceCID.String())
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, body, err := c.do(req, http.StatusOK, http.StatusNotFound, http.StatusAccepted)
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
