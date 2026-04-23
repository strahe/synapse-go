package spregistry

import (
	"math/big"
	"time"
)

// WriteOption tunes the behaviour of a single state-changing call.
type WriteOption func(*writeConfig)

type writeConfig struct {
	waitTimeout   time.Duration
	confirmations uint64
	value         *big.Int
	valueSet      bool
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

// WithValue overrides the transaction's msg.value for payable calls. It is
// only consumed by [Service.RegisterProvider]; other writes ignore it. When
// not provided, RegisterProvider reads the contract's REGISTRATION_FEE.
// A non-nil *big.Int is copied defensively before being stored on the
// transaction options.
func WithValue(v *big.Int) WriteOption {
	return func(c *writeConfig) {
		if v == nil {
			c.value = nil
		} else {
			c.value = new(big.Int).Set(v)
		}
		c.valueSet = true
	}
}
