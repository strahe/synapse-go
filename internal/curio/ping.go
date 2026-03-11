package curio

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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	_, _, err = c.do(req)
	return err
}
