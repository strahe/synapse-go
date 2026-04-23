package curio

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/http2"
)

// DefaultUserAgent is set on every outgoing request unless overridden.
const DefaultUserAgent = "synapse-go/curio"

// DefaultHTTPTimeout applies to simple JSON operations. Streaming uploads
// use a separate, longer timeout (or none at all) negotiated per call.
const DefaultHTTPTimeout = 30 * time.Second

// DefaultMaxRetries is the number of retries attempted for transient failures
// (5xx, 429, network errors) when no explicit value is set.
const DefaultMaxRetries = 3

// MaxControlResponseBytes caps the size of control-plane JSON response bodies
// read fully into memory. Curio's control-plane endpoints return small JSON
// documents (dataset status, piece IDs, etc.); anything larger indicates a
// buggy or hostile server and should not be allowed to exhaust client memory.
// Streaming endpoints (piece download) bypass this limit.
const MaxControlResponseBytes = 16 << 20 // 16 MiB

// Client is a thin HTTP client for a single Curio PDP service URL.
// Safe for concurrent use.
type Client struct {
	baseURL      *url.URL
	httpClient   *http.Client
	userAgent    string
	logger       *slog.Logger
	maxRetries   int                                        // 0 = disabled; set to DefaultMaxRetries in New()
	retryDelayFn func(err error, attempt int) time.Duration // nil = httpRetryDelay
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies a custom *http.Client. Useful to inject timeouts,
// custom transports, or test doubles.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithUserAgent sets the User-Agent header on every request.
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithMaxRetries sets the maximum number of retry attempts for transient
// failures (5xx, 429, network errors). A value of 0 disables retries.
// Negative values are treated as 0. Defaults to DefaultMaxRetries (3).
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n < 0 {
			n = 0
		}
		c.maxRetries = n
	}
}

// WithLogger attaches a structured logger. A nil logger disables logging.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.logger = l }
}

// New constructs a Client pointing at the given service URL (e.g.
// https://pdp.example.com).
func New(serviceURL string, opts ...Option) (*Client, error) {
	if serviceURL == "" {
		return nil, errors.New("curio.New: empty serviceURL")
	}
	u, err := url.Parse(serviceURL)
	if err != nil {
		return nil, fmt.Errorf("curio.New: parse serviceURL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("curio.New: unsupported scheme %q", u.Scheme)
	}
	// Ensure path ends with "/" so relative resolutions append cleanly.
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	c := &Client{
		baseURL:    u,
		httpClient: &http.Client{Timeout: DefaultHTTPTimeout},
		userAgent:  DefaultUserAgent,
		maxRetries: DefaultMaxRetries,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// BaseURL returns a copy of the configured base URL.
func (c *Client) BaseURL() *url.URL {
	u := *c.baseURL
	return &u
}

// resolve joins ref against the base URL. ref is interpreted as a path
// relative to /pdp/... (no leading slash expected).
func (c *Client) resolve(ref string) (*url.URL, error) {
	r, err := url.Parse(ref)
	if err != nil {
		return nil, err
	}
	return c.baseURL.ResolveReference(r), nil
}

// do executes the HTTP request, checks for protocol-level success, and
// returns the response body for the caller to decode. Non-2xx responses
// are returned as *HTTPError. The body is always drained and closed.
func (c *Client) do(req *http.Request, expectStatuses ...int) (*http.Response, []byte, error) {
	return c.doWithClient(c.httpClient, req, expectStatuses...)
}

func (c *Client) doWithClient(client *http.Client, req *http.Request, expectStatuses ...int) (*http.Response, []byte, error) {
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.logger != nil {
		c.logger.Debug("curio request", "method", req.Method, "url", redactURL(req.URL))
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("curio: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Cap response body size to protect against unbounded or hostile responses.
	// Read MaxControlResponseBytes+1 so we can detect overflow distinctly from
	// an exactly-sized reply.
	limited := io.LimitReader(resp.Body, MaxControlResponseBytes+1)
	body, readErr := io.ReadAll(limited)
	if readErr != nil {
		return resp, nil, fmt.Errorf("curio: read body: %w", readErr)
	}
	if int64(len(body)) > MaxControlResponseBytes {
		return resp, nil, fmt.Errorf("curio: %s %s: response body exceeds %d bytes (MaxControlResponseBytes)", req.Method, req.URL.Path, MaxControlResponseBytes)
	}

	if len(expectStatuses) == 0 {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return resp, body, newHTTPError(req, resp, body)
		}
		return resp, body, nil
	}
	for _, s := range expectStatuses {
		if resp.StatusCode == s {
			return resp, body, nil
		}
	}
	return resp, body, newHTTPError(req, resp, body)
}

// isRetryable reports whether the error warrants a retry attempt.
//
// Non-retryable (permanent):
//   - context.Canceled / context.DeadlineExceeded
//   - HTTP 4xx except 429
//   - HTTP 501 Not Implemented
//   - TLS alert errors (bad cert, expired cert, protocol violations)
//
// Retryable (transient):
//   - HTTP 5xx (except 501) and 429
//   - Connection-level failures: ECONNREFUSED, ECONNRESET, EPIPE
//   - Transient DNS errors (IsTemporary || IsTimeout)
//   - I/O cut short: io.ErrUnexpectedEOF
//   - url.Error with Timeout() == true
//
// Unknown error types are NOT retried. Older releases retried optimistically,
// but that masked permanent misconfigurations (bad URL, invalid signer).
// Callers that need broader retries should do so at the business layer.
func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusTooManyRequests {
			return true
		}
		return httpErr.StatusCode >= 500 && httpErr.StatusCode != http.StatusNotImplemented
	}
	// TLS handshake rejections are permanent — bad/expired cert, protocol
	// mismatch. Check before url.Error since these wrap tls.AlertError inside
	// net.OpError inside url.Error.
	var tlsAlert tls.AlertError
	if errors.As(err, &tlsAlert) {
		return false
	}
	// Connection-level failures are transient.
	if errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) {
		return true
	}
	// Server cut the stream mid-response; worth retrying an idempotent call.
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	// HTTP/2: retry on GOAWAY and stream-level recoverable errors.
	var goaway http2.GoAwayError
	if errors.As(err, &goaway) {
		return true
	}
	var streamErr http2.StreamError
	if errors.As(err, &streamErr) {
		switch streamErr.Code {
		case http2.ErrCodeRefusedStream, http2.ErrCodeCancel:
			return true
		}
	}
	// DNS: retry only explicitly temporary or timeout failures.
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return dnsErr.IsTemporary || dnsErr.IsTimeout
	}
	// url.Error surfaces timeouts (request timeout, idle timeout).
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Timeout()
	}
	// Unknown error type: do not retry. Safer than optimistic retry.
	return false
}

// httpRetryDelay returns the delay to wait before the next retry attempt.
// For responses with a Retry-After header (429, 503) the server's value takes
// precedence, capped at maxRetryDelay. Otherwise an exponential backoff
// (1s, 2s, 4s … capped at maxRetryDelay) is used.
func httpRetryDelay(err error, attempt int) time.Duration {
	const maxRetryDelay = 30 * time.Second
	var httpErr *HTTPError
	if errors.As(err, &httpErr) && httpErr.RetryAfter > 0 {
		if httpErr.RetryAfter > maxRetryDelay {
			return maxRetryDelay
		}
		return httpErr.RetryAfter
	}
	const maxShift = 5 // caps at 32s before the maxRetryDelay clamp
	if attempt > maxShift {
		attempt = maxShift
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > maxRetryDelay {
		d = maxRetryDelay
	}
	return d
}

// doRetryable calls makeReq to build a fresh request for each attempt and
// executes it with c.do. Transient errors (5xx, 429, network) are retried up
// to c.maxRetries times with exponential back-off (or Retry-After delay for
// 429). Non-retryable errors (4xx except 429, context errors) are returned
// immediately.
//
// Only safe for GET/HEAD or other idempotent operations. POST/DELETE that
// mutate server state must not be retried here — see postJSON/deleteJSON.
// Long-running and streaming calls should use c.do directly.
func (c *Client) doRetryable(ctx context.Context, makeReq func() (*http.Request, error), expectStatuses ...int) (*http.Response, []byte, error) {
	maxRetries := c.maxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		req, err := makeReq()
		if err != nil {
			return nil, nil, err
		}
		resp, body, err := c.do(req, expectStatuses...)
		if err == nil {
			return resp, body, nil
		}
		if !isRetryable(err) || attempt == maxRetries {
			return resp, body, err
		}
		if c.logger != nil {
			c.logger.Debug("curio retry", "attempt", attempt+1, "maxRetries", maxRetries, "error", err)
		}
		delayFn := c.retryDelayFn
		if delayFn == nil {
			delayFn = httpRetryDelay
		}
		delay := delayFn(err, attempt)
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	// Defensive: the loop's `attempt == maxRetries` branch always returns on
	// the last iteration, and the `maxRetries < 0 → 0` guard ensures the
	// loop runs at least once. Returning a sentinel error here keeps the
	// function total without using panic in library code.
	return nil, nil, fmt.Errorf("curio: doRetryable: retry loop exited without result")
}

// postJSON builds a POST request with a JSON-encoded body and executes it
// exactly once. POST is not idempotent in general (a transient failure after
// the server has processed the request may cause duplicate submissions), so
// callers that need retry behavior must implement it at the business layer
// with appropriate deduplication.
func (c *Client) postJSON(ctx context.Context, path string, payload any, expect ...int) (*http.Response, []byte, error) {
	u, err := c.resolve(path)
	if err != nil {
		return nil, nil, fmt.Errorf("curio: resolve %s: %w", path, err)
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("curio: marshal %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return nil, nil, fmt.Errorf("curio: build POST %s: %w", path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, expect...)
}

// longClient returns a clone of c.httpClient with Timeout=0 so that
// the request lifetime is controlled solely by the context. Use this
// for operations that may wait for blockchain confirmation.
func (c *Client) longClient() *http.Client {
	if c.httpClient != nil {
		cloned := *c.httpClient
		cloned.Timeout = 0
		return &cloned
	}
	return &http.Client{Timeout: 0}
}

// postJSONLong is like postJSON but uses a no-timeout HTTP client.
// Suitable for endpoints that block until a blockchain transaction confirms.
func (c *Client) postJSONLong(ctx context.Context, pth string, payload any, expect ...int) (*http.Response, []byte, error) {
	u, err := c.resolve(pth)
	if err != nil {
		return nil, nil, fmt.Errorf("curio: resolve %s: %w", pth, err)
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("curio: marshal %s: %w", pth, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(buf))
	if err != nil {
		return nil, nil, fmt.Errorf("curio: build POST %s: %w", pth, err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doWithClient(c.longClient(), req, expect...)
}

// getJSON performs GET and decodes the JSON response into dst (may be nil
// to ignore the body).
func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	u, err := c.resolve(path)
	if err != nil {
		return fmt.Errorf("curio: resolve %s: %w", path, err)
	}
	_, body, err := c.doRetryable(ctx, func() (*http.Request, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("curio: build GET %s: %w", path, err)
		}
		req.Header.Set("Accept", "application/json")
		return req, nil
	})
	if err != nil {
		return err
	}
	if dst == nil {
		return nil
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("curio: decode %s: %w", path, err)
	}
	return nil
}

// deleteJSON performs DELETE with an optional JSON body and decodes the
// response into dst. Executes exactly once. DELETE is not idempotent here
// for retry purposes because callers (e.g. dataset/piece removal) may depend
// on distinguishing "already gone" from "just removed" via HTTP status.
// Business-layer retry is the caller's responsibility.
func (c *Client) deleteJSON(ctx context.Context, path string, payload, dst any) error {
	u, err := c.resolve(path)
	if err != nil {
		return fmt.Errorf("curio: resolve %s: %w", path, err)
	}
	var buf []byte
	if payload != nil {
		var err error
		buf, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("curio: marshal %s: %w", path, err)
		}
	}
	var body io.Reader
	if buf != nil {
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), body)
	if err != nil {
		return fmt.Errorf("curio: build DELETE %s: %w", path, err)
	}
	if buf != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	_, respBody, err := c.do(req)
	if err != nil {
		return err
	}
	if dst == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, dst); err != nil {
		return fmt.Errorf("curio: decode %s: %w", path, err)
	}
	return nil
}
