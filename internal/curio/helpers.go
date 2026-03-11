package curio

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
)

// buildJSONRequest marshals payload and returns a ready-to-send request.
func buildJSONRequest(ctx context.Context, method, url string, payload any) (*http.Request, error) {
	buf, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return req, nil
}

// jsonUnmarshal forwards to json.Unmarshal.
func jsonUnmarshal(body []byte, dst any) error {
	return json.Unmarshal(body, dst)
}
