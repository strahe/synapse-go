package pdp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	pingResponseToken    = "curio-pdp"
	maxPingResponseBytes = 64
)

// Ping issues GET /pdp/ping and accepts only a 2xx response whose trimmed body
// is exactly "curio-pdp". Responses larger than 64 bytes are rejected.
func (c *Client) Ping(ctx context.Context) error {
	u, err := c.resolve("pdp/ping")
	if err != nil {
		return err
	}
	_, _, err = c.doRetryableWithExecutor(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	}, func(req *http.Request) (*http.Response, []byte, error) {
		return c.doPingWithClient(c.httpClient, req)
	})
	return err
}

func (c *Client) doPingWithClient(client *http.Client, req *http.Request) (*http.Response, []byte, error) {
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	if c.logger != nil {
		c.logger.Debug("pdp request", "method", req.Method, "url", redactURL(req.URL))
	}
	resp, err := client.Do(req)
	if err != nil {
		err = redactRequestError(err)
		return nil, nil, fmt.Errorf("pdp: %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxPingResponseBytes+1))
	if readErr != nil {
		return resp, nil, fmt.Errorf("pdp: read ping body: %w", errors.Join(ErrPingResponseMismatch, readErr))
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return resp, body, newHTTPError(req, resp, body)
	}
	if len(body) > maxPingResponseBytes {
		return resp, nil, fmt.Errorf("%w: response body exceeds %d bytes", ErrPingResponseMismatch, maxPingResponseBytes)
	}
	if strings.TrimSpace(string(body)) != pingResponseToken {
		return resp, body, ErrPingResponseMismatch
	}
	return resp, body, nil
}
