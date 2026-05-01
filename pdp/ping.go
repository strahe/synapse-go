package pdp

import (
	"context"
	"net/http"
)

// Ping issues GET /pdp/ping to verify the service is reachable.
func (c *Client) Ping(ctx context.Context) error {
	u, err := c.resolve("pdp/ping")
	if err != nil {
		return err
	}
	_, _, err = c.doRetryable(ctx, func() (*http.Request, error) {
		return http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	})
	return err
}
