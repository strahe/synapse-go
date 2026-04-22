package warmstorage

import "time"

// WriteOption tunes the behaviour of a single state-changing call.
type WriteOption func(*writeConfig)

type writeConfig struct {
	waitTimeout   time.Duration
	confirmations uint64
}

func newWriteConfig(opts []WriteOption) writeConfig {
	cfg := writeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithWait makes the call block until the transaction is mined, or the given
// timeout elapses. Zero / negative returns immediately after broadcast.
func WithWait(timeout time.Duration) WriteOption {
	return func(c *writeConfig) { c.waitTimeout = timeout }
}

// WithConfirmations requires N block confirmations in addition to WithWait.
func WithConfirmations(n uint64) WriteOption {
	return func(c *writeConfig) { c.confirmations = n }
}
