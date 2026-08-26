package pdp

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

// HTTPError wraps a non-success response from the PDP provider API.
// Use errors.AsType[*pdp.HTTPError] to read StatusCode, Body, and RetryAfter
// from wrapped errors.
//
// The URL field is always pre-redacted: userinfo is stripped and sensitive
// query parameters (see sensitiveQueryKeys in redact.go) are masked as
// "***". This removes the footgun where a caller logs `%+v` or JSON
// marshals the struct and leaks credentials. The pre-redacted form is
// sufficient for debugging — path, scheme, host and non-sensitive query
// values are preserved.
type HTTPError struct {
	Method     string
	URL        string
	StatusCode int
	Body       string
	// RetryAfter is the server-requested wait duration from the Retry-After
	// header. Non-zero on HTTP 429 (Too Many Requests) and 503 (Service
	// Unavailable) responses when the server provides the header.
	RetryAfter time.Duration
}

func newHTTPError(req *http.Request, resp *http.Response, body []byte) *HTTPError {
	e := &HTTPError{
		Method:     req.Method,
		URL:        redactURL(req.URL),
		StatusCode: resp.StatusCode,
		Body:       strings.TrimSpace(string(body)),
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
		e.RetryAfter = parseRetryAfter(resp.Header.Get("Retry-After"))
	}
	return e
}

// parseRetryAfter parses a Retry-After header value. It accepts both
// delay-seconds (e.g. "30") and HTTP-date formats.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	// Numeric: delay in seconds.
	if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	// HTTP-date format.
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// RedactedURL returns the log-safe URL stored in e.URL. Retained for
// backwards compatibility; new code can simply read e.URL directly as it
// is always pre-redacted.
func (e *HTTPError) RedactedURL() string {
	return e.URL
}

// Error implements the error interface using the pre-redacted URL.
func (e *HTTPError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("pdp: %s %s: HTTP %d", e.Method, e.URL, e.StatusCode)
	}
	return fmt.Sprintf("pdp: %s %s: HTTP %d: %s", e.Method, e.URL, e.StatusCode, e.Body)
}

// ErrLocationHeader is returned when the server responds successfully but
// the expected Location header is missing or malformed.
var ErrLocationHeader = errors.New("pdp: missing or invalid Location header")

// ErrPieceNotFound is returned when GET /pdp/piece returns 404.
var ErrPieceNotFound = errors.New("pdp: piece not found")

// ErrPieceProcessing is returned when GET /pdp/piece returns 202, meaning
// the piece is known but not yet parked and queryable.
var ErrPieceProcessing = errors.New("pdp: piece still processing")

// ErrTooManyPieces is returned when an add-pieces style request exceeds
// MaxAddPiecesBatchSize.
var ErrTooManyPieces = errors.New("pdp: too many pieces")

// ErrTxRejected is returned when an on-chain operation posted by the SP was
// rejected, failed, or removed from the canonical chain by a reorganization.
var ErrTxRejected = errors.New("pdp: transaction rejected")

// ErrInvalidStatus is returned when a provider status response is unknown,
// incomplete, or internally inconsistent.
var ErrInvalidStatus = errors.New("pdp: invalid status response")

// ErrStatusURLOrigin is returned when a provider status URL does not use the
// same origin as the PDP client, or redirects to another origin.
var ErrStatusURLOrigin = errors.New("pdp: status URL origin mismatch")

// ServiceAlreadyTerminatedError is returned when the provider reports the
// service is already terminated on chain.
type ServiceAlreadyTerminatedError struct {
	ServiceTerminationEpoch types.Epoch
	Message                 string
}

func (e *ServiceAlreadyTerminatedError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("pdp: service already terminated at epoch %d: %s", e.ServiceTerminationEpoch, e.Message)
	}
	return fmt.Sprintf("pdp: service already terminated at epoch %d", e.ServiceTerminationEpoch)
}

// TerminateServicePendingError is returned when a termination request is
// already queued.
type TerminateServicePendingError struct {
	Message string
}

func (e *TerminateServicePendingError) Error() string {
	if e.Message != "" {
		return "pdp: terminate service already pending: " + e.Message
	}
	return "pdp: terminate service already pending"
}

// TerminateServiceNotSupportedError is returned when the provider does not
// support provider-relayed termination.
type TerminateServiceNotSupportedError struct {
	Body string
}

func (e *TerminateServiceNotSupportedError) Error() string {
	if e.Body != "" {
		return "pdp: terminate service not supported: " + e.Body
	}
	return "pdp: terminate service not supported"
}

// WaitForTerminateServiceNotFoundError is returned when no provider
// termination request is visible before a tx hash is observed.
type WaitForTerminateServiceNotFoundError struct{}

func (e *WaitForTerminateServiceNotFoundError) Error() string {
	return "pdp: terminate service status not found"
}

// WaitForTerminateServiceRejectedError is returned when a provider tx hash
// was observed and the status later disappears.
type WaitForTerminateServiceRejectedError struct {
	TxHash common.Hash
}

func (e *WaitForTerminateServiceRejectedError) Error() string {
	return "pdp: terminate service transaction rejected: " + e.TxHash.Hex()
}
