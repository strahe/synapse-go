package sessionkey

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// WriteOption configures transaction submission or receipt waiting. Nil
// options are ignored.
type WriteOption func(*writeConfig)

type writeConfig struct {
	onSubmitted   func(common.Hash)
	waitTimeout   time.Duration
	confirmations uint64
}

func newWriteConfig(opts []WriteOption) writeConfig {
	cfg := writeConfig{}
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	return cfg
}

// WithWait makes the call block until the transaction is mined, or the
// given timeout elapses. Use zero or a negative duration to return as soon
// as the tx is broadcast (the default).
// For [Service.WaitForTransaction], non-positive values select the default timeout.
func WithWait(timeout time.Duration) WriteOption {
	return func(c *writeConfig) { c.waitTimeout = timeout }
}

// WithOnSubmitted calls fn synchronously after successful broadcast, before
// receipt waiting. A nil fn disables notification. Callback panics propagate.
func WithOnSubmitted(fn func(common.Hash)) WriteOption {
	return func(c *writeConfig) { c.onSubmitted = fn }
}

// WithConfirmations requires n confirmations for successful and failed receipts,
// counting the inclusion block as the first. Zero waits only for inclusion.
// For writes it requires a positive WithWait timeout.
// [Service.WaitForTransaction] applies it with the selected wait timeout.
func WithConfirmations(n uint64) WriteOption {
	return func(c *writeConfig) { c.confirmations = n }
}
