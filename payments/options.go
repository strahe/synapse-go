package payments

import (
	"errors"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

// ErrInsufficientBalance is returned when the on-chain balance is lower
// than the requested amount for a withdrawal or transfer.
var ErrInsufficientBalance = errors.New("payments: insufficient balance")

// ErrInsufficientAllowance is returned when an ERC20 allowance is lower
// than the amount required for a deposit.
var ErrInsufficientAllowance = errors.New("payments: insufficient allowance")

// ErrZeroAddress is returned when a caller passes common.Address{} for an
// argument that must be a real token or account address. Public service
// methods wrap it together with ErrInvalidArgument so callers can match
// either the specific zero-address sentinel or the generic invalid-argument
// sentinel.
var ErrZeroAddress = errors.New("payments: zero address")

// WriteOption configures transaction submission or receipt waiting. Nil
// options are ignored.
type WriteOption func(*writeConfig)

type writeConfig struct {
	onSubmitted           func(common.Hash)
	waitTimeout           time.Duration
	confirmations         uint64
	skipPrecheck          bool
	fundNeedsFwssApproval *bool
	fundApprovalLockup    *big.Int
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

// WithSkipPrecheck disables client-side balance / allowance / funds checks
// before broadcasting. Useful when the caller has already validated state
// or wants to probe an on-chain revert for diagnostic purposes.
// Static argument validation, including uint256 bounds, still applies.
func WithSkipPrecheck() WriteOption {
	return func(c *writeConfig) { c.skipPrecheck = true }
}

// WithFundNeedsFwssApproval overrides Fund / FundSync's approval-state probe.
// When set, Fund skips isFwssMaxApproved and uses the supplied decision
// directly so prepare/execute flows can reuse a previously computed result.
func WithFundNeedsFwssApproval(needs bool) WriteOption {
	return func(c *writeConfig) { c.fundNeedsFwssApproval = &needs }
}

// WithFundApprovalLockupPeriod overrides the max lockup period used when
// Fund / FundSync writes or checks FWSS approval.
func WithFundApprovalLockupPeriod(period *big.Int) WriteOption {
	return func(c *writeConfig) {
		if period == nil {
			c.fundApprovalLockup = nil
			return
		}
		c.fundApprovalLockup = new(big.Int).Set(period)
	}
}
