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

// AccountStateResolution is an account state projected to a specific epoch.
type AccountStateResolution struct {
	// AvailableFunds is the balance available for withdrawal or new
	// commitments at the resolved epoch.
	AvailableFunds *big.Int
	// RunwayInEpochs is the number of epochs until the account enters deficit.
	// It is maxUint256 when LockupRate is zero.
	RunwayInEpochs *big.Int
	// GrossCoverageInEpochs is Funds / LockupRate, ignoring reserved lockup.
	// It is maxUint256 when LockupRate is zero.
	GrossCoverageInEpochs *big.Int
}

// AccountSummary is a payment account health snapshot for the configured
// USDFC token.
type AccountSummary struct {
	// Funds is the total deposited balance in base units of the configured
	// USDFC token.
	Funds *big.Int
	// AvailableFunds is the balance available for withdrawal or new
	// commitments at CurrentEpoch, in base units of the configured USDFC token.
	AvailableFunds *big.Int
	// Debt is the outstanding amount owed beyond Funds, in base units of the
	// configured USDFC token.
	Debt *big.Int
	// LockupRatePerEpoch is the aggregate payment lockup rate in base units of
	// the configured USDFC token per Filecoin epoch.
	LockupRatePerEpoch *big.Int
	// LockupRatePerMonth is LockupRatePerEpoch multiplied by the SDK's
	// Filecoin epochs-per-month constant.
	LockupRatePerMonth *big.Int
	// TotalLockup is the effective locked amount at CurrentEpoch in base units
	// of the configured USDFC token.
	TotalLockup *big.Int
	// TotalFixedLockup is the sum of fixed lockup across payer rails, including
	// terminated-but-not-finalized rails, in base units of the configured USDFC
	// token.
	TotalFixedLockup *big.Int
	// TotalRateBasedLockup is TotalLockup minus TotalFixedLockup, floored at
	// zero, in base units of the configured USDFC token.
	TotalRateBasedLockup *big.Int
	// FundedUntilEpoch is the legacy absolute epoch at which unreserved funds
	// are exhausted at the current lockup rate.
	//
	// Deprecated: Use RunwayInEpochs for account health decisions.
	FundedUntilEpoch *big.Int
	// RunwayInEpochs is the number of epochs from CurrentEpoch until the
	// account enters deficit. It is maxUint256 when LockupRatePerEpoch is zero.
	RunwayInEpochs *big.Int
	// GrossCoverageInEpochs is Funds / LockupRatePerEpoch, ignoring reserved
	// lockup. It is maxUint256 when LockupRatePerEpoch is zero.
	GrossCoverageInEpochs *big.Int
	// CurrentEpoch is the Filecoin epoch used for this snapshot's calculations.
	CurrentEpoch *big.Int
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

// ResolveAt projects the raw account fields to epoch. It ignores AccountInfo's
// cached available funds so each projection uses the same source fields.
func (a *AccountState) ResolveAt(epoch *big.Int) AccountStateResolution {
	funds, lockupCurrent, lockupRate, lockupLastSettledAt := accountStateParts(a)
	current := copyBigOrZero(epoch)

	fundedUntil := fundedUntilEpoch(funds, lockupCurrent, lockupRate, lockupLastSettledAt)
	elapsed := new(big.Int).Sub(current, lockupLastSettledAt)
	if elapsed.Sign() < 0 {
		elapsed.SetInt64(0)
	}

	simulatedLockup := new(big.Int).Mul(lockupRate, elapsed)
	simulatedLockup.Add(simulatedLockup, lockupCurrent)

	available := new(big.Int).Sub(funds, simulatedLockup)
	if available.Sign() < 0 {
		available.SetInt64(0)
	}

	runway := new(big.Int).Set(maxUint256)
	gross := new(big.Int).Set(maxUint256)
	if lockupRate.Sign() == 0 {
		if lockupCurrent.Cmp(funds) > 0 {
			runway.SetInt64(0)
		}
	} else {
		if fundedUntil.Cmp(current) > 0 {
			runway.Sub(fundedUntil, current)
		} else {
			runway.SetInt64(0)
		}
		gross.Quo(funds, lockupRate)
	}

	return AccountStateResolution{
		AvailableFunds:        available,
		RunwayInEpochs:        runway,
		GrossCoverageInEpochs: gross,
	}
}

// DebtAt returns the outstanding amount owed beyond Funds at epoch.
func (a *AccountState) DebtAt(epoch *big.Int) *big.Int {
	funds, lockupCurrent, lockupRate, lockupLastSettledAt := accountStateParts(a)
	current := copyBigOrZero(epoch)
	if current.Cmp(lockupLastSettledAt) < 0 {
		return new(big.Int)
	}
	elapsed := new(big.Int).Sub(current, lockupLastSettledAt)
	totalOwed := new(big.Int).Mul(lockupRate, elapsed)
	totalOwed.Add(totalOwed, lockupCurrent)
	if totalOwed.Cmp(funds) <= 0 {
		return new(big.Int)
	}
	return totalOwed.Sub(totalOwed, funds)
}

func accountStateParts(a *AccountState) (funds, lockupCurrent, lockupRate, lockupLastSettledAt *big.Int) {
	if a == nil {
		return new(big.Int), new(big.Int), new(big.Int), new(big.Int)
	}
	return copyBigOrZero(a.Funds),
		copyBigOrZero(a.LockupCurrent),
		copyBigOrZero(a.LockupRate),
		copyBigOrZero(a.LockupLastSettledAt)
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
