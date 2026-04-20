package payments

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// AccountState mirrors FilPay.accounts(token, owner).
// All values are in base units of the payment token.
type AccountState struct {
	Funds               *big.Int
	LockupCurrent       *big.Int
	LockupRate          *big.Int
	LockupLastSettledAt *big.Int
	// FundedUntilEpoch is the forward-looking epoch at which the account's
	// available funds will be exhausted at the current lockup rate.
	// Sourced from getAccountInfoIfSettled(); zero when LockupRate is zero.
	FundedUntilEpoch *big.Int
	availableFunds   *big.Int
}

// AvailableFunds returns Funds - LockupCurrent (never negative). A nil
// AccountState returns nil.
func (a *AccountState) AvailableFunds() *big.Int {
	if a == nil {
		return nil
	}
	if a.availableFunds != nil {
		return new(big.Int).Set(a.availableFunds)
	}
	if a.Funds == nil || a.LockupCurrent == nil {
		return nil
	}
	v := new(big.Int).Sub(a.Funds, a.LockupCurrent)
	if v.Sign() < 0 {
		return big.NewInt(0)
	}
	return v
}

// OperatorApproval mirrors FilPay.operatorApprovals(token, client, operator).
type OperatorApproval struct {
	IsApproved      bool
	RateAllowance   *big.Int // maximum per-epoch rate the operator may charge
	LockupAllowance *big.Int // maximum lockup the operator may hold
	RateUsage       *big.Int // current per-epoch rate in use by the operator
	LockupUsage     *big.Int // current lockup held by the operator
	MaxLockupPeriod *big.Int // maximum lockup period allowed (in epochs)
}

// WriteResult is returned by every state-changing call.
//
// Hash is always populated when WriteResult is non-nil — it is set as soon as
// the transaction is broadcast. Receipt is populated only when the call was
// made with WithWait(timeout) and the transaction was mined (successfully or
// reverted) before the timeout elapsed.
//
// Error semantics (observe WriteResult and err together):
//
//   - WriteResult == nil, err != nil: pre-broadcast failure (validation,
//     signing, or broadcast itself failed). The transaction was never
//     submitted to the chain and it is safe to retry with fresh state.
//   - WriteResult != nil, err == nil: transaction was broadcast successfully.
//     Without WithWait, Receipt is nil. With WithWait, Receipt is non-nil and
//     Status == 1 (successful execution).
//   - WriteResult != nil, err != nil, Receipt == nil: WithWait timed out
//     before a terminal receipt was returned, or receipt lookup failed. This
//     includes both "not mined yet" and "already mined, but
//     WithConfirmations(...) has not been satisfied yet". Hash is valid;
//     callers can keep polling by Hash.
//   - WriteResult != nil, err != nil, Receipt != nil: transaction was mined
//     but execution reverted. err wraps ErrTxFailed (use errors.Is). Receipt
//     carries the failed status and any logs emitted before revert.
type WriteResult struct {
	Hash    common.Hash
	Receipt *types.Receipt
}

// ZeroAddress is a convenience alias for common.Address{} used to indicate
// native FIL in WalletBalance queries.
var ZeroAddress = common.Address{}
