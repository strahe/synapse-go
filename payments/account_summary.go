package payments

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/idconv"
)

// AccountSummary returns a payment health snapshot for owner using the
// Service's configured USDFC token.
func (s *Service) AccountSummary(ctx context.Context, owner common.Address) (*AccountSummary, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	token, err := s.defaultUSDFCToken("payments.AccountSummary")
	if err != nil {
		return nil, err
	}
	if (owner == common.Address{}) {
		return nil, invalidZeroAddressError("payments.AccountSummary", "owner")
	}

	currentBlock, err := s.backend.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.AccountSummary: block number: %w", err)
	}
	current := new(big.Int).SetUint64(currentBlock)
	account, err := s.accountStateAt(ctx, token, owner, current)
	if err != nil {
		return nil, fmt.Errorf("payments.AccountSummary: account info: %w", err)
	}
	fixed, err := s.totalAccountFixedLockupAt(ctx, token, owner, current)
	if err != nil {
		return nil, err
	}

	return summarizeAccount(account, fixed, current), nil
}

// TotalAccountFixedLockup returns the sum of fixed lockup across all payer
// rails for owner using the Service's configured USDFC token.
func (s *Service) TotalAccountFixedLockup(ctx context.Context, owner common.Address) (*big.Int, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	token, err := s.defaultUSDFCToken("payments.TotalAccountFixedLockup")
	if err != nil {
		return nil, err
	}
	if (owner == common.Address{}) {
		return nil, invalidZeroAddressError("payments.TotalAccountFixedLockup", "owner")
	}
	return s.totalAccountFixedLockup(ctx, token, owner)
}

func (s *Service) defaultUSDFCToken(op string) (common.Address, error) {
	if (s.usdfcToken == common.Address{}) {
		return common.Address{}, fmt.Errorf("%s: %w: USDFCTokenAddress not configured", op, ErrInvalidArgument)
	}
	return s.usdfcToken, nil
}

func (s *Service) totalAccountFixedLockup(ctx context.Context, token, owner common.Address) (*big.Int, error) {
	return s.totalAccountFixedLockupAt(ctx, token, owner, nil)
}

func (s *Service) totalAccountFixedLockupAt(ctx context.Context, token, owner common.Address, blockNumber *big.Int) (*big.Int, error) {
	call := &bind.CallOpts{Context: ctx, BlockNumber: copyBig(blockNumber)}
	out, err := s.filPayCall.GetRailsForPayerAndToken(call, owner, token, big.NewInt(0), big.NewInt(0))
	if err != nil {
		return nil, fmt.Errorf("payments.TotalAccountFixedLockup: list rails: %w", err)
	}

	total := new(big.Int)
	for _, item := range out.Results {
		railID, err := idconv.FromBig("railID", item.RailId)
		if err != nil {
			return nil, fmt.Errorf("payments.TotalAccountFixedLockup: %w", err)
		}
		rail, err := s.filPayCall.GetRail(call, railID.Big())
		if err != nil {
			return nil, fmt.Errorf("payments.TotalAccountFixedLockup: get rail %s: %w", railID.String(), err)
		}
		if rail.LockupFixed != nil {
			total.Add(total, rail.LockupFixed)
		}
	}
	return total, nil
}

func (s *Service) accountStateAt(ctx context.Context, token, owner common.Address, blockNumber *big.Int) (*AccountState, error) {
	v, err := s.filPayCall.Accounts(&bind.CallOpts{Context: ctx, BlockNumber: copyBig(blockNumber)}, token, owner)
	if err != nil {
		return nil, fmt.Errorf("accounts: %w", err)
	}
	return &AccountState{
		Funds:               copyBig(v.Funds),
		LockupCurrent:       copyBig(v.LockupCurrent),
		LockupRate:          copyBig(v.LockupRate),
		LockupLastSettledAt: copyBig(v.LockupLastSettledAt),
	}, nil
}

func summarizeAccount(account *AccountState, fixedLockup, currentEpoch *big.Int) *AccountSummary {
	funds, lockupCurrent, lockupRate, lockupLastSettledAt := accountStateParts(account)
	current := copyBigOrZero(currentEpoch)
	fixed := copyBigOrZero(fixedLockup)

	fundedUntil := fundedUntilEpoch(funds, lockupCurrent, lockupRate, lockupLastSettledAt)
	resolved := account.ResolveAt(current)
	debt := account.DebtAt(current)

	totalLockup := new(big.Int).Sub(funds, resolved.AvailableFunds)
	if totalLockup.Sign() < 0 {
		totalLockup.SetInt64(0)
	}
	rateBased := new(big.Int).Sub(totalLockup, fixed)
	if rateBased.Sign() < 0 {
		rateBased.SetInt64(0)
	}

	return &AccountSummary{
		Funds:                 funds,
		AvailableFunds:        resolved.AvailableFunds,
		Debt:                  debt,
		LockupRatePerEpoch:    new(big.Int).Set(lockupRate),
		LockupRatePerMonth:    new(big.Int).Mul(lockupRate, big.NewInt(chain.EpochsPerMonth)),
		TotalLockup:           totalLockup,
		TotalFixedLockup:      fixed,
		TotalRateBasedLockup:  rateBased,
		FundedUntilEpoch:      fundedUntil,
		RunwayInEpochs:        resolved.RunwayInEpochs,
		GrossCoverageInEpochs: resolved.GrossCoverageInEpochs,
		CurrentEpoch:          current,
	}
}

func fundedUntilEpoch(funds, lockupCurrent, lockupRate, lockupLastSettledAt *big.Int) *big.Int {
	if lockupRate.Sign() == 0 {
		return new(big.Int).Set(maxUint256)
	}
	remaining := new(big.Int).Sub(funds, lockupCurrent)
	epochs := new(big.Int).Quo(remaining, lockupRate)
	return new(big.Int).Add(lockupLastSettledAt, epochs)
}

func copyBigOrZero(v *big.Int) *big.Int {
	if v == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(v)
}
