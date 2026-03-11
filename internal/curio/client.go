package curio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultUserAgent is set on every outgoing request unless overridden.
const DefaultUserAgent = "synapse-go/curio"

// DefaultHTTPTimeout applies to simple JSON operations. Streaming uploads
// use a separate, longer timeout (or none at all) negotiated per call.
const DefaultHTTPTimeout = 30 * time.Second

// Client is a thin HTTP client for a single Curio PDP service URL.
// Safe for concurrent use.
type Client struct {
	baseURL    *url.URL
	httpClient *http.Client
	userAgent  string
	logger     *slog.Logger
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
		c.logger.Debug("curio request", "method", req.Method, "url", req.URL.String())
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("curio: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return resp, nil, fmt.Errorf("curio: read body: %w", readErr)
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

// postJSON builds a POST request with a JSON-encoded body.
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

// getJSON performs GET and decodes the JSON response into dst (may be nil
// to ignore the body).
func (c *Client) getJSON(ctx context.Context, path string, dst any) error {
	u, err := c.resolve(path)
	if err != nil {
		return fmt.Errorf("curio: resolve %s: %w", path, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("curio: build GET %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/json")
	_, body, err := c.do(req)
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
// response into dst.
func (c *Client) deleteJSON(ctx context.Context, path string, payload any, dst any) error {
	u, err := c.resolve(path)
	if err != nil {
		return fmt.Errorf("curio: resolve %s: %w", path, err)
	}
	var body io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("curio: marshal %s: %w", path, err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u.String(), body)
	if err != nil {
		return fmt.Errorf("curio: build DELETE %s: %w", path, err)
	}
	if payload != nil {
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
