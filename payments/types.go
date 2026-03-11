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
	availableFunds      *big.Int
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
	RateAllowance   *big.Int
	LockupAllowance *big.Int
	RateUsage       *big.Int
	LockupUsage     *big.Int
	MaxLockupPeriod *big.Int
}

// WriteResult is returned by every state-changing call.
//
// Hash is populated as soon as the transaction is broadcast. Receipt is
// populated only when the call was made with WithWait(timeout) and the
// transaction was mined before the timeout elapsed.
type WriteResult struct {
	Hash    common.Hash
	Receipt *types.Receipt
}

// ZeroAddress is a convenience alias for common.Address{} used to indicate
// native FIL in WalletBalance queries.
var ZeroAddress = common.Address{}
