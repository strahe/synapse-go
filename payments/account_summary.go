package payments

import (
	"context"
	"fmt"
	"math/big"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/idconv"
	sdktypes "github.com/strahe/synapse-go/types"
)

// AccountSummary returns a payment health snapshot for owner using the
// Service's configured USDFC token. All reads use CurrentEpoch as their block
// number. Rail details require Multicall3; any read failure returns a nil result.
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

	reader, err := s.newAccountSummaryReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.AccountSummary: %w", err)
	}
	account, err := reader.accountState(ctx, token, owner)
	if err != nil {
		return nil, fmt.Errorf("payments.AccountSummary: account info: %w", err)
	}
	fixed, err := s.totalAccountFixedLockup(ctx, token, owner, reader)
	if err != nil {
		return nil, err
	}

	return summarizeAccount(account, fixed, reader.block), nil
}

// TotalAccountFixedLockup returns the sum of fixed lockup across all payer
// rails for owner using the Service's configured USDFC token. It includes
// terminated rails awaiting final settlement. Reads use one block number and
// Multicall3; any read failure returns a nil result.
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
	reader, err := s.newAccountSummaryReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.TotalAccountFixedLockup: %w", err)
	}
	return s.totalAccountFixedLockup(ctx, token, owner, reader)
}

func (s *Service) defaultUSDFCToken(op string) (common.Address, error) {
	if (s.usdfcToken == common.Address{}) {
		return common.Address{}, fmt.Errorf("%s: %w: USDFCTokenAddress not configured", op, ErrInvalidArgument)
	}
	return s.usdfcToken, nil
}

func (s *Service) totalAccountFixedLockup(ctx context.Context, token, owner common.Address, reader *accountSummaryReader) (*big.Int, error) {
	contractABI, err := filpay.FilPayMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("payments.TotalAccountFixedLockup: ABI: %w", err)
	}
	total := new(big.Int)
	ids := make([]sdktypes.BigInt, 0, min(s.maxMulticallCalls, int(defaultIteratePageSize)))
	flush := func() error {
		amount, err := s.railFixedLockupBatch(ctx, reader, contractABI, ids)
		if err != nil {
			return err
		}
		total.Add(total, amount)
		ids = ids[:0]
		return nil
	}
	var totalSlots *big.Int
	for offset := new(big.Int); ; {
		page, err := reader.filPay.GetRailsForPayerAndToken(&bind.CallOpts{Context: ctx}, owner, token, offset, new(big.Int).SetUint64(defaultIteratePageSize))
		if err != nil {
			return nil, fmt.Errorf("payments.TotalAccountFixedLockup: list rails at %s: %w", offset, err)
		}
		if !validAccountRailPage(offset, page.NextOffset, page.Total, len(page.Results)) ||
			(totalSlots != nil && page.Total.Cmp(totalSlots) != 0) {
			return nil, fmt.Errorf("payments.TotalAccountFixedLockup: %w: offset=%s next=%v total=%v", ErrInvalidRailPage, offset, page.NextOffset, page.Total)
		}
		totalSlots = page.Total
		for _, item := range page.Results {
			railID, err := idconv.FromBig("railID", item.RailId)
			if err != nil {
				return nil, fmt.Errorf("payments.TotalAccountFixedLockup: %w", err)
			}
			ids = append(ids, railID)
			if len(ids) == s.maxMulticallCalls {
				if err := flush(); err != nil {
					return nil, err
				}
			}
		}
		if page.NextOffset.Cmp(page.Total) == 0 {
			break
		}
		offset = page.NextOffset
	}
	if len(ids) > 0 {
		if err := flush(); err != nil {
			return nil, err
		}
	}
	return total, nil
}

func validAccountRailPage(offset, next, total *big.Int, resultCount int) bool {
	if next == nil || total == nil || next.Sign() < 0 || total.Sign() < 0 || offset.Cmp(total) > 0 {
		return false
	}
	// Finalized rails shorten the result list, but the cursor still advances
	// over every historical slot in the requested range.
	expectedNext := new(big.Int).Add(offset, new(big.Int).SetUint64(defaultIteratePageSize))
	if expectedNext.Cmp(total) > 0 {
		expectedNext.Set(total)
	}
	scanned := new(big.Int).Sub(expectedNext, offset)
	return next.Cmp(expectedNext) == 0 && scanned.Cmp(big.NewInt(int64(resultCount))) >= 0
}

func (s *Service) railFixedLockupBatch(ctx context.Context, reader *accountSummaryReader, contractABI *gethabi.ABI, ids []sdktypes.BigInt) (*big.Int, error) {
	calls := make([]iabi.Call3, len(ids))
	for i, id := range ids {
		data, err := contractABI.Pack("getRail", id.Big())
		if err != nil {
			return nil, fmt.Errorf("payments.TotalAccountFixedLockup: pack rail %s: %w", id, err)
		}
		// Individual status preserves the failed rail's identity and revert data.
		calls[i] = iabi.Call3{Target: s.filPayAddr, AllowFailure: true, CallData: data}
	}
	results, err := iabi.BatchCall(ctx, reader, calls)
	if err != nil {
		return nil, fmt.Errorf("payments.TotalAccountFixedLockup: rail batch: %w", err)
	}
	if len(results) != len(ids) {
		return nil, fmt.Errorf("payments.TotalAccountFixedLockup: expected %d rail results, got %d", len(ids), len(results))
	}
	total := new(big.Int)
	for i, result := range results {
		if !result.Success {
			return nil, fmt.Errorf("payments.TotalAccountFixedLockup: get rail %s: %s", ids[i], railRevertReason(contractABI, result.ReturnData))
		}
		values, err := contractABI.Unpack("getRail", result.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("payments.TotalAccountFixedLockup: decode rail %s: %w", ids[i], err)
		}
		rail := *gethabi.ConvertType(values[0], new(filpay.FilecoinPayV1RailView)).(*filpay.FilecoinPayV1RailView)
		total.Add(total, rail.LockupFixed)
	}
	return total, nil
}

func railRevertReason(contractABI *gethabi.ABI, data []byte) string {
	if reason, err := gethabi.UnpackRevert(data); err == nil {
		return "execution reverted: " + reason
	}
	if len(data) >= 4 {
		if custom, err := contractABI.ErrorByID([4]byte(data[:4])); err == nil {
			if args, err := custom.Unpack(data); err == nil {
				return fmt.Sprintf("execution reverted: %s%v", custom.Name, args)
			}
		}
	}
	return "call failed"
}

func (r *accountSummaryReader) accountState(ctx context.Context, token, owner common.Address) (*AccountState, error) {
	v, err := r.filPay.Accounts(&bind.CallOpts{Context: ctx}, token, owner)
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
	funds, _, lockupRate, _ := accountStateParts(account)
	current := copyBigOrZero(currentEpoch)
	fixed := copyBigOrZero(fixedLockup)

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
