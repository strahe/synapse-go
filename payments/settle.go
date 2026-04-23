package payments

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"

	"github.com/strahe/synapse-go/internal/contracts/filpay"
	sdktypes "github.com/strahe/synapse-go/types"
)

// SettlementResult is the decoded return tuple of FilPay.settleRail.
//
// Mirrors synapse-sdk/src/payments/service.ts:620 (SettlementResult).
type SettlementResult struct {
	TotalSettledAmount      *big.Int
	TotalNetPayeeAmount     *big.Int
	TotalOperatorCommission *big.Int
	TotalNetworkFee         *big.Int
	FinalSettledEpoch       *big.Int
	Note                    string
}

// Settle triggers a rail settlement up to `untilEpoch`. When `untilEpoch`
// is nil or zero, the current block number is used.
//
// Mirrors synapse-core/src/pay/settle-rail.ts and
// synapse-sdk/src/payments/service.ts:589.
func (s *Service) Settle(ctx context.Context, railID, untilEpoch *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner("Settle"); err != nil {
		return nil, err
	}
	if railID == nil || railID.Sign() <= 0 {
		return nil, fmt.Errorf("payments.Settle: %w: railID must be > 0", ErrInvalidArgument)
	}
	resolved, err := s.resolveUntilEpoch(ctx, untilEpoch)
	if err != nil {
		return nil, fmt.Errorf("payments.Settle: %w", err)
	}
	txOpts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.Settle: %w", err)
	}
	defer release()
	tx, err := s.filPayWrite.SettleRail(txOpts, railID, resolved)
	release()
	if err != nil {
		return nil, fmt.Errorf("payments.Settle: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// SettleTerminatedRail triggers the emergency-settlement path for a
// terminated rail. This bypasses the operator validator and pays in full;
// it can only be called by the client after the max settlement epoch has
// passed.
//
// Mirrors synapse-sdk/src/payments/service.ts:647 (settleTerminatedRail).
func (s *Service) SettleTerminatedRail(ctx context.Context, railID *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner("SettleTerminatedRail"); err != nil {
		return nil, err
	}
	if railID == nil || railID.Sign() <= 0 {
		return nil, fmt.Errorf("payments.SettleTerminatedRail: %w: railID must be > 0", ErrInvalidArgument)
	}
	txOpts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.SettleTerminatedRail: %w", err)
	}
	defer release()
	tx, err := s.filPayWrite.SettleTerminatedRailWithoutValidation(txOpts, railID)
	release()
	if err != nil {
		return nil, fmt.Errorf("payments.SettleTerminatedRail: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// SettleAuto inspects the rail state and routes to Settle or
// SettleTerminatedRail automatically — terminated rails (endEpoch > 0) go
// through the emergency path, active rails through the standard path.
//
// Mirrors synapse-sdk/src/payments/service.ts:697 (settleAuto).
func (s *Service) SettleAuto(ctx context.Context, railID, untilEpoch *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner("SettleAuto"); err != nil {
		return nil, err
	}
	rail, err := s.GetRail(ctx, railID)
	if err != nil {
		return nil, fmt.Errorf("payments.SettleAuto: %w", err)
	}
	if rail.EndEpoch != nil && rail.EndEpoch.Sign() > 0 {
		return s.SettleTerminatedRail(ctx, railID, opts...)
	}
	return s.Settle(ctx, railID, untilEpoch, opts...)
}

// filPayStaticABI is lazily parsed from the FilPay metadata string so we
// can execute read-only static calls against state-changing methods like
// settleRail via eth_call and decode the return tuple.
var filPayStaticABI = func() abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(filpay.FilPayMetaData.ABI))
	if err != nil {
		panic(fmt.Sprintf("payments: parse filpay ABI: %v", err)) //nolint:forbidigo // init-time ABI parse: error implies a build/codegen bug, not a runtime condition
	}
	return parsed
}()

// GetSettlementAmounts simulates FilPay.settleRail via eth_call and decodes
// the resulting amounts without broadcasting a transaction. Use it to
// preview how much would be settled to the payee / operator / network.
//
// Mirrors synapse-sdk/src/payments/service.ts:601 (getSettlementAmounts),
// which uses simulateContract for the same effect on viem.
func (s *Service) GetSettlementAmounts(ctx context.Context, railID, untilEpoch *big.Int) (*SettlementResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if railID == nil || railID.Sign() <= 0 {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: %w: railID must be > 0", ErrInvalidArgument)
	}
	resolved, err := s.resolveUntilEpoch(ctx, untilEpoch)
	if err != nil {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: %w", err)
	}
	bound := bind.NewBoundContract(s.filPayAddr, filPayStaticABI, s.backend, nil, nil)
	var out []interface{}
	if err := bound.Call(&bind.CallOpts{Context: ctx}, &out, "settleRail", railID, resolved); err != nil {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: static call: %w", err)
	}
	if len(out) < 6 {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: unexpected return length %d", len(out))
	}
	settled, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: unexpected type for totalSettledAmount: %T", out[0])
	}
	netPayee, ok := out[1].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: unexpected type for totalNetPayeeAmount: %T", out[1])
	}
	commission, ok := out[2].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: unexpected type for totalOperatorCommission: %T", out[2])
	}
	networkFee, ok := out[3].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: unexpected type for totalNetworkFee: %T", out[3])
	}
	finalEpoch, ok := out[4].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: unexpected type for finalSettledEpoch: %T", out[4])
	}
	note, ok := out[5].(string)
	if !ok {
		return nil, fmt.Errorf("payments.GetSettlementAmounts: unexpected type for note: %T", out[5])
	}
	return &SettlementResult{
		TotalSettledAmount:      copyBig(settled),
		TotalNetPayeeAmount:     copyBig(netPayee),
		TotalOperatorCommission: copyBig(commission),
		TotalNetworkFee:         copyBig(networkFee),
		FinalSettledEpoch:       copyBig(finalEpoch),
		Note:                    note,
	}, nil
}

// resolveUntilEpoch returns untilEpoch when non-nil and positive; otherwise
// it queries the current block number to match the TS default.
func (s *Service) resolveUntilEpoch(ctx context.Context, untilEpoch *big.Int) (*big.Int, error) {
	if untilEpoch != nil && untilEpoch.Sign() > 0 {
		return new(big.Int).Set(untilEpoch), nil
	}
	bn, err := s.backend.BlockNumber(ctx)
	if err != nil {
		return nil, fmt.Errorf("block number: %w", err)
	}
	return new(big.Int).SetUint64(bn), nil
}
