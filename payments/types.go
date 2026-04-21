package payments

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	sdktypes "github.com/strahe/synapse-go/types"
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

// WriteResult is kept as an alias for backwards compatibility.
type WriteResult = sdktypes.WriteResult

// ZeroAddress is a convenience alias for common.Address{} used to indicate
// native FIL in WalletBalance queries.
var ZeroAddress = common.Address{}
