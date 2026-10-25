package spregistry

import (
	"math/big"
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
	value         *big.Int
	valueSet      bool
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

// WithValue overrides the transaction's msg.value for payable calls. It is
// only consumed by [Service.RegisterProvider]; other writes ignore it. For
// RegisterProvider, an explicit value must equal the 5 FIL registration fee.
// When not provided, RegisterProvider reads the contract's REGISTRATION_FEE.
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
