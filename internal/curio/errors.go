package curio

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// HTTPError wraps a non-success response from the Curio API.
type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
}

func newHTTPError(req *http.Request, resp *http.Response, body []byte) *HTTPError {
	return &HTTPError{
		Method:     req.Method,
		URL:        req.URL.String(),
		StatusCode: resp.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
}

// Error implements the error interface.
func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("curio: %s %s: HTTP %d", e.Method, e.URL, e.StatusCode)
	}
	return fmt.Sprintf("curio: %s %s: HTTP %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

// ErrLocationHeader is returned when the server responds successfully but
// the expected Location header is missing or malformed.
var ErrLocationHeader = errors.New("curio: missing or invalid Location header")

// ErrPieceNotFound is returned when GET /pdp/piece returns 404.
var ErrPieceNotFound = errors.New("curio: piece not found")

// ErrPieceProcessing is returned when GET /pdp/piece returns 202, meaning
// the piece is known but not yet parked and queryable.
var ErrPieceProcessing = errors.New("curio: piece still processing")

// ErrStillPending is returned by status-polling helpers when the server
// reports the transaction is still pending. It is the sentinel callers
// should loop on while waiting.
var ErrStillPending = errors.New("curio: still pending")

// ErrTxRejected is returned when an on-chain operation posted by the SP
// was rejected by the network.
var ErrTxRejected = errors.New("curio: transaction rejected")
