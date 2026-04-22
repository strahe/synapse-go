package storage

import (
	"errors"
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
)

// ErrInvalidArgument is returned, wrapped via fmt.Errorf with %w, when a
// caller passes an argument that violates a precondition (nil pointer,
// zero ID, empty required URL, undefined pieceCID, etc.). Match with
// errors.Is(err, storage.ErrInvalidArgument).
//
// Business / server-invariant errors (no approved providers, duplicate
// dataset IDs, server returned zero dataSetID, etc.) are intentionally
// left as plain errors so that errors.Is against ErrInvalidArgument
// only matches genuine caller-supplied validation failures.
var ErrInvalidArgument = errors.New("storage: invalid argument")

// ErrPrivateNetwork is returned by Service.Download when the target URL
// resolves to a loopback / link-local / RFC1918 / ULA / multicast /
// unspecified address and the Service was constructed without
// Options.AllowPrivateNetworks. It prevents SDK callers from being used as
// SSRF egress against internal networks.
var ErrPrivateNetwork = errors.New("storage: private / local network address disallowed")

// ErrUnsupportedScheme is returned when the URL passed to Service.Download
// uses a scheme other than http or https.
var ErrUnsupportedScheme = errors.New("storage: unsupported URL scheme")

// ErrMaxBytesExceeded is returned by Service.Download when the response body
// exceeds Options.DownloadMaxBytes. The error is surfaced either eagerly
// (via Content-Length when supplied by the server) or via the terminal Read
// on the returned reader.
var ErrMaxBytesExceeded = errors.New("storage: download exceeded MaxBytes")

// StoreError is returned when the primary store operation fails.
type StoreError struct {
	ProviderID types.ProviderID
	Endpoint   string
	Cause      error
}

func (e *StoreError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("storage.StoreError: provider %d (%s)", uint64(e.ProviderID), e.Endpoint)
	}
	return fmt.Sprintf("storage.StoreError: provider %d (%s): %v", uint64(e.ProviderID), e.Endpoint, e.Cause)
}

func (e *StoreError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// CommitError is returned when all on-chain commit attempts fail and no copies
// are stored. Individual per-provider failures are reported in UploadResult.FailedAttempts.
type CommitError struct {
	ProviderID types.ProviderID
	Endpoint   string
	Cause      error
}

func (e *CommitError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("storage.CommitError: provider %d (%s)", uint64(e.ProviderID), e.Endpoint)
	}
	return fmt.Sprintf("storage.CommitError: provider %d (%s): %v", uint64(e.ProviderID), e.Endpoint, e.Cause)
}

func (e *CommitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// DownloadError is returned when an HTTP download request fails, either due to
// a network error or a non-2xx HTTP status code. Use errors.As to access the
// URL and status code.
type DownloadError struct {
	URL        string
	StatusCode int   // zero when the failure occurred before receiving a response
	Cause      error // non-nil when the failure has an underlying error (e.g. a network or transport-level error)
}

func (e *DownloadError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause != nil && e.StatusCode != 0 {
		return fmt.Sprintf("storage.DownloadError: GET %s: status %d: %v", e.URL, e.StatusCode, e.Cause)
	}
	if e.StatusCode != 0 {
		return fmt.Sprintf("storage.DownloadError: GET %s: status %d", e.URL, e.StatusCode)
	}
	if e.Cause != nil {
		return fmt.Sprintf("storage.DownloadError: GET %s: %v", e.URL, e.Cause)
	}
	return fmt.Sprintf("storage.DownloadError: GET %s", e.URL)
}

func (e *DownloadError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// CIDMismatchError is returned when the piece CID computed from the downloaded
// bytes does not match the expected CID. Use errors.As to retrieve the computed
// and expected CIDs for diagnostic purposes.
type CIDMismatchError struct {
	Expected   cid.Cid
	ComputedV1 cid.Cid
	ComputedV2 cid.Cid
}

func (e *CIDMismatchError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return fmt.Sprintf("storage.CIDMismatchError: pieceCID mismatch (computed v1=%s v2=%s, want %s)",
		e.ComputedV1, e.ComputedV2, e.Expected)
}
