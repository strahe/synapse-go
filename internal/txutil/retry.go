package txutil

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/rpc"
)

// IsRetryableRPCError reports whether err is a transient RPC condition that
// should be retried (connection drops, timeouts). Context errors and nil are
// never retryable.
func IsRetryableRPCError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if httpErr, ok := errors.AsType[rpc.HTTPError](err); ok {
		return IsRetryableHTTPStatus(httpErr.StatusCode)
	}
	if httpErr, ok := errors.AsType[*rpc.HTTPError](err); ok && httpErr != nil {
		return IsRetryableHTTPStatus(httpErr.StatusCode)
	}
	s := strings.ToLower(err.Error())
	for _, m := range retryableMarkers {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// IsRetryableHTTPStatus reports whether an RPC HTTP response indicates a
// transient request timeout, rate limit, or gateway failure.
func IsRetryableHTTPStatus(status int) bool {
	return status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status == http.StatusBadGateway ||
		status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

var retryableMarkers = []string{
	"timeout",
	"timed out",
	"connection refused",
	"connection reset",
	"broken pipe",
	"i/o timeout",
	"eof",
	"temporary failure",
	"try again",
}

// IsNonceError reports whether err relates to nonce sequencing (too low/high/invalid).
func IsNonceError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "nonce too low") ||
		strings.Contains(s, "nonce too high") ||
		strings.Contains(s, "invalid nonce") ||
		strings.Contains(s, "already known")
}

// IsGasError reports whether err relates to gas or fee pricing.
func IsGasError(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "gas required exceeds") ||
		strings.Contains(s, "insufficient funds for gas") ||
		strings.Contains(s, "underpriced") ||
		strings.Contains(s, "fee cap")
}
