package warmstorage

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// WriteOption tunes the behaviour of a single state-changing call. Nil
// options are ignored.
type WriteOption func(*writeConfig)

type writeConfig struct {
	waitTimeout   time.Duration
	confirmations uint64
	onSubmitted   func(common.Hash)
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

// WithWait makes the call block until the transaction is mined, or the given
// timeout elapses. Zero / negative returns immediately after broadcast.
func WithWait(timeout time.Duration) WriteOption {
	return func(c *writeConfig) { c.waitTimeout = timeout }
}

// WithOnSubmitted calls fn synchronously once a transaction is successfully
// broadcast, before any receipt polling. A nil fn disables notification.
// Callback panics propagate to the caller.
func WithOnSubmitted(fn func(common.Hash)) WriteOption {
	return func(c *writeConfig) { c.onSubmitted = fn }
}

// WithConfirmations requires N block confirmations in addition to WithWait.
func WithConfirmations(n uint64) WriteOption {
	return func(c *writeConfig) { c.confirmations = n }
}
