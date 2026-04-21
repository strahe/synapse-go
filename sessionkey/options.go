package sessionkey

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
