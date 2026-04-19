package storage

import (
	"fmt"
	"math/big"

	"github.com/ipfs/go-cid"
)

// StoreError is returned when the primary store operation fails.
type StoreError struct {
	ProviderID *big.Int
	Endpoint   string
	Cause      error
}

func (e *StoreError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("storage.StoreError: provider %s (%s)", bigIntString(e.ProviderID), e.Endpoint)
	}
	return fmt.Sprintf("storage.StoreError: provider %s (%s): %v", bigIntString(e.ProviderID), e.Endpoint, e.Cause)
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
	ProviderID *big.Int
	Endpoint   string
	Cause      error
}

func (e *CommitError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("storage.CommitError: provider %s (%s)", bigIntString(e.ProviderID), e.Endpoint)
	}
	return fmt.Sprintf("storage.CommitError: provider %s (%s): %v", bigIntString(e.ProviderID), e.Endpoint, e.Cause)
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

func bigIntString(v *big.Int) string {
	if v == nil {
		return "<nil>"
	}
	return v.String()
}
