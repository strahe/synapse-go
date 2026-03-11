package txutil

import "errors"

// Sentinel errors returned from the txutil package.
var (
	// ErrReceiptTimeout is returned when waiting for a transaction receipt
	// exceeds the configured timeout.
	ErrReceiptTimeout = errors.New("timeout waiting for transaction receipt")

	// ErrReceiptRPCFailure is returned when too many consecutive RPC errors
	// occur while polling for a receipt.
	ErrReceiptRPCFailure = errors.New("receipt fetch failed due to repeated RPC errors")

	// ErrTxFailed is returned when a transaction is mined but its receipt
	// status is not successful (reverted or out-of-gas on-chain).
	ErrTxFailed = errors.New("transaction failed on-chain")

	// ErrNonRetryable wraps errors that should not be retried by the caller.
	ErrNonRetryable = errors.New("non-retryable error")
)
