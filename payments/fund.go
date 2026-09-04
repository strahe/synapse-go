package payments

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	sdktypes "github.com/strahe/synapse-go/types"
)

const defaultApprovalLockupPeriodEpochs int64 = chain.EpochsPerMonth

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
	var approvalLockupPeriod *big.Int
	if cfg.fundNeedsFwssApproval != nil {
		needsApproval = *cfg.fundNeedsFwssApproval
	} else {
		var err error
		approvalLockupPeriod, err = s.resolveFundApprovalLockupPeriod(ctx, cfg)
		if err != nil {
			return nil, fmt.Errorf("payments.Fund: %w", err)
		}
		approved, err := s.isFwssMaxApproved(ctx, approvalLockupPeriod)
		if err != nil {
			return nil, fmt.Errorf("payments.Fund: check approval: %w", err)
		}
		needsApproval = !approved
	}

	if needsApproval {
		if approvalLockupPeriod == nil {
			var err error
			approvalLockupPeriod, err = s.resolveFundApprovalLockupPeriod(ctx, cfg)
			if err != nil {
				return nil, fmt.Errorf("payments.Fund: %w", err)
			}
		}
		if amount.Sign() > 0 {
			return s.DepositWithPermitAndApproveOperator(
				ctx,
				s.usdfcToken,
				common.Address{},
				amount,
				nil,
				s.warmStorage,
				maxUint256, maxUint256, approvalLockupPeriod,
				opts...,
			)
		}
		return s.ApproveService(ctx, s.usdfcToken, s.warmStorage, maxUint256, maxUint256, approvalLockupPeriod, opts...)
	}
	if amount.Sign() > 0 {
		return s.DepositWithPermit(ctx, s.usdfcToken, common.Address{}, amount, nil, opts...)
	}
	return nil, ErrNothingToFund
}

// FundSync runs Fund and waits for the transaction to be mined. It is
// equivalent to Fund(..., WithWait(timeout)) with a sensible default
// timeout when the caller did not supply one.
func (s *Service) FundSync(ctx context.Context, amount *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	// Append a default-wait fallback; runs last so explicit user-supplied
	// WithWait continues to take precedence.
	opts = append(opts, waitIfUnset)
	return s.Fund(ctx, amount, opts...)
}

// waitIfUnset sets a 5-minute wait timeout when the caller omitted WithWait
// or supplied a non-positive timeout. FundSync always waits for a receipt, so
// zero/negative values fall back to the default wait instead of disabling
// waiting.
var waitIfUnset WriteOption = func(c *writeConfig) {
	if c.waitTimeout <= 0 {
		c.waitTimeout = 5 * time.Minute
	}
}

// isFwssMaxApproved reports whether WarmStorage holds sufficient operator
// allowances to skip the approve-step of Fund.
func (s *Service) isFwssMaxApproved(ctx context.Context, requiredLockupPeriod *big.Int) (bool, error) {
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
	required := defaultApprovalLockupPeriod()
	if requiredLockupPeriod != nil {
		required.Set(requiredLockupPeriod)
	}
	if approval.MaxLockupPeriod == nil || approval.MaxLockupPeriod.Cmp(required) < 0 {
		return false, nil
	}
	return true, nil
}

func (s *Service) resolveFundApprovalLockupPeriod(ctx context.Context, cfg writeConfig) (*big.Int, error) {
	if cfg.fundApprovalLockup != nil {
		if cfg.fundApprovalLockup.Sign() <= 0 {
			return nil, fmt.Errorf("%w: approval lockup period must be > 0", ErrInvalidArgument)
		}
		return new(big.Int).Set(cfg.fundApprovalLockup), nil
	}
	if s.lockups != nil {
		period, err := s.lockups.ApprovalLockupPeriod(ctx)
		if err != nil {
			return nil, err
		}
		if period == nil || period.Sign() <= 0 {
			return nil, fmt.Errorf("%w: approval lockup period must be > 0", ErrInvalidArgument)
		}
		return new(big.Int).Set(period), nil
	}
	return defaultApprovalLockupPeriod(), nil
}

func defaultApprovalLockupPeriod() *big.Int {
	return big.NewInt(defaultApprovalLockupPeriodEpochs)
}
