package payments

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	sdktypes "github.com/strahe/synapse-go/types"
)

// LockupPeriodEpochs is the default client-operator max lockup period
// granted when Fund auto-approves the WarmStorage operator. Mirrors
// synapse-core `LOCKUP_PERIOD` = 30 days * 2880 epochs/day.
var LockupPeriodEpochs = big.NewInt(30 * 2880)

// maxUint256 is the ERC-2612-style "unlimited" allowance used by Fund.
var maxUint256 = func() *big.Int {
	x, _ := new(big.Int).SetString(
		"115792089237316195423570985008687907853269984665640564039457584007913129639935", 10,
	)
	return x
}()

// ErrNothingToFund is returned by Fund / FundSync when the account is
// already fully approved for WarmStorage and the caller requested a zero
// deposit amount, meaning there is no work to do.
var ErrNothingToFund = errors.New("payments: nothing to fund (already approved and amount is 0)")

// Fund is a smart deposit that auto-detects whether WarmStorage is already
// approved with sufficient allowances and routes to the correct on-chain
// call:
//
//   - needs approval + amount > 0 → DepositWithPermitAndApproveOperator
//   - needs approval + amount == 0 → ApproveService
//   - already approved + amount > 0 → DepositWithPermit
//   - already approved + amount == 0 → ErrNothingToFund
//
// Fund requires WarmStorageAddress and USDFCTokenAddress to be set on the
// [Options]. Returns ErrInvalidArgument when either is zero.
//
// amount must be non-nil. Callers that want an approval-only flow must pass
// [big.NewInt](0) explicitly so an omitted amount cannot silently broadcast an
// approval transaction.
//
// Pass [WithFundNeedsFwssApproval] to reuse a previously computed approval
// decision instead of re-reading on-chain state.
//
// Mirrors synapse-core/src/pay/fund.ts:72.
func (s *Service) Fund(ctx context.Context, amount *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner("Fund"); err != nil {
		return nil, err
	}
	if amount == nil {
		return nil, fmt.Errorf("payments.Fund: %w: amount must not be nil", ErrInvalidArgument)
	}
	if amount.Sign() < 0 {
		return nil, fmt.Errorf("payments.Fund: %w: amount must be >= 0", ErrInvalidArgument)
	}
	if (s.warmStorage == common.Address{}) {
		return nil, fmt.Errorf("payments.Fund: %w: WarmStorageAddress not configured", ErrInvalidArgument)
	}
	if (s.usdfcToken == common.Address{}) {
		return nil, fmt.Errorf("payments.Fund: %w: USDFCTokenAddress not configured", ErrInvalidArgument)
	}

	cfg := newWriteConfig(opts)
	needsApproval := false
	if cfg.fundNeedsFwssApproval != nil {
		needsApproval = *cfg.fundNeedsFwssApproval
	} else {
		approved, err := s.isFwssMaxApproved(ctx)
		if err != nil {
			return nil, fmt.Errorf("payments.Fund: check approval: %w", err)
		}
		needsApproval = !approved
	}

	if needsApproval {
		if amount.Sign() > 0 {
			return s.DepositWithPermitAndApproveOperator(
				ctx,
				s.usdfcToken,
				common.Address{},
				amount,
				nil,
				s.warmStorage,
				maxUint256, maxUint256, LockupPeriodEpochs,
				opts...,
			)
		}
		return s.ApproveService(ctx, s.usdfcToken, s.warmStorage, maxUint256, maxUint256, LockupPeriodEpochs, opts...)
	}
	if amount.Sign() > 0 {
		return s.DepositWithPermit(ctx, s.usdfcToken, common.Address{}, amount, nil, opts...)
	}
	return nil, ErrNothingToFund
}

// FundSync runs Fund and waits for the transaction to be mined. It is
// equivalent to Fund(..., WithWait(timeout)) with a sensible default
// timeout when the caller did not supply one.
//
// Mirrors synapse-core/src/pay/fund.ts:144 (fundSync).
func (s *Service) FundSync(ctx context.Context, amount *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	// Append a default-wait fallback; runs last so explicit user-supplied
	// WithWait continues to take precedence.
	opts = append(opts, waitIfUnset)
	return s.Fund(ctx, amount, opts...)
}

// waitIfUnset sets a 5-minute wait timeout when the caller omitted WithWait
// or supplied a non-positive timeout. FundSync mirrors the TS sync helper and
// always waits for a receipt, so zero/negative values fall back to the default
// wait instead of disabling waiting.
var waitIfUnset WriteOption = func(c *writeConfig) {
	if c.waitTimeout <= 0 {
		c.waitTimeout = 5 * time.Minute
	}
}

// isFwssMaxApproved reports whether WarmStorage holds sufficient operator
// allowances to skip the approve-step of Fund. Matches the TS logic which
// checks isApproved + rateAllowance == maxUint256 +
// lockupAllowance >= maxUint256/2 + maxLockupPeriod >= LOCKUP_PERIOD.
//
// Mirrors synapse-core/src/pay/is-fwss-max-approved.ts.
func (s *Service) isFwssMaxApproved(ctx context.Context) (bool, error) {
	approval, err := s.ServiceApproval(ctx, s.usdfcToken, s.signer.EVMAddress(), s.warmStorage)
	if err != nil {
		return false, err
	}
	if !approval.IsApproved {
		return false, nil
	}
	if approval.RateAllowance == nil || approval.RateAllowance.Cmp(maxUint256) != 0 {
		return false, nil
	}
	halfMax := new(big.Int).Rsh(maxUint256, 1)
	if approval.LockupAllowance == nil || approval.LockupAllowance.Cmp(halfMax) < 0 {
		return false, nil
	}
	if approval.MaxLockupPeriod == nil || approval.MaxLockupPeriod.Cmp(LockupPeriodEpochs) < 0 {
		return false, nil
	}
	return true, nil
}
