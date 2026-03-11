package payments

import (
	"errors"
	"time"
)

// ErrInsufficientBalance is returned when the on-chain balance is lower
// than the requested amount for a withdrawal or transfer.
var ErrInsufficientBalance = errors.New("payments: insufficient balance")

// ErrInsufficientAllowance is returned when an ERC20 allowance is lower
// than the amount required for a deposit.
var ErrInsufficientAllowance = errors.New("payments: insufficient allowance")

// ErrZeroAddress is returned when a caller passes common.Address{} for an
// argument that must be a real token or account address.
var ErrZeroAddress = errors.New("payments: zero address")

// WriteOption tunes the behaviour of a single state-changing call.
type WriteOption func(*writeConfig)

type writeConfig struct {
	waitTimeout   time.Duration
	confirmations uint64
	skipPrecheck  bool
}

func newWriteConfig(opts []WriteOption) writeConfig {
	cfg := writeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithWait makes the call block until the transaction is mined, or the
// given timeout elapses. Use zero or a negative duration to return as soon
// as the tx is broadcast (the default).
func WithWait(timeout time.Duration) WriteOption {
	return func(c *writeConfig) { c.waitTimeout = timeout }
}

// WithConfirmations requires N block confirmations in addition to WithWait.
// Has no effect unless WithWait is also passed with a positive timeout.
func WithConfirmations(n uint64) WriteOption {
	return func(c *writeConfig) { c.confirmations = n }
}

// WithSkipPrecheck disables client-side balance / allowance / funds checks
// before broadcasting. Useful when the caller has already validated state
// or wants to probe an on-chain revert for diagnostic purposes.
func WithSkipPrecheck() WriteOption {
	return func(c *writeConfig) { c.skipPrecheck = true }
}
