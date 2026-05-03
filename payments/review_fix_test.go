package payments

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"
	filpaybind "github.com/strahe/synapse-go/internal/contracts/filpay"
	sdktypes "github.com/strahe/synapse-go/types"
)

func TestFund_NilAmountReturnsErrInvalidArgument(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.Fund(context.Background(), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Fund(nil) err=%v, want ErrInvalidArgument", err)
	}
}

func TestFund_WithFundNeedsFwssApprovalTrueSkipsApprovalCheck(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	mb.errs[filPayAddr.Hex()+":operatorApprovals"] = errors.New("rpc down")

	if _, err := s.Fund(context.Background(), big.NewInt(0), WithFundNeedsFwssApproval(true)); err != nil {
		t.Fatalf("Fund override=true: %v", err)
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(mb.sent))
	}
	if got, want := mb.sent[0].To(), &filPayAddr; got == nil || *got != *want {
		t.Fatalf("sent tx to=%v, want %v", got, want)
	}
}

func TestFund_WithFundNeedsFwssApprovalFalseSkipsApprovalCheck(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	mb.errs[filPayAddr.Hex()+":operatorApprovals"] = errors.New("rpc down")

	if _, err := s.Fund(context.Background(), big.NewInt(0), WithFundNeedsFwssApproval(false)); !errors.Is(err, ErrNothingToFund) {
		t.Fatalf("Fund override=false err=%v, want ErrNothingToFund", err)
	}
	if len(mb.sent) != 0 {
		t.Fatalf("sent tx count = %d, want 0", len(mb.sent))
	}
}

func TestFundSync_WithExplicitZeroWaitStillWaitsForReceipt(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	receiptCalls := 0
	mb.receiptFn = func(context.Context, common.Hash) (*coretypes.Receipt, error) {
		receiptCalls++
		return &coretypes.Receipt{Status: 1}, nil
	}

	got, err := s.FundSync(
		context.Background(),
		big.NewInt(0),
		WithWait(0),
		WithFundNeedsFwssApproval(true),
	)
	if err != nil {
		t.Fatalf("FundSync wait=0: %v", err)
	}
	if got == nil {
		t.Fatal("FundSync wait=0 returned nil result")
	}
	if got.Receipt == nil {
		t.Fatal("FundSync wait=0 should still include a receipt")
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(mb.sent))
	}
	if receiptCalls != 1 {
		t.Fatalf("receipt call count = %d, want 1", receiptCalls)
	}
}

func TestSettleAuto_NoSignerFailsBeforeFetchingRail(t *testing.T) {
	s, mb := newTestServiceWith(t, nil)
	mb.setFilPayReply(t, filPayAddr, "getRail", filpaybind.FilecoinPayV1RailView{
		Token:               tokenAddr,
		From:                otherAddr,
		To:                  operatorAddr,
		Operator:            operatorAddr,
		Validator:           common.HexToAddress("0x5555555555555555555555555555555555555555"),
		PaymentRate:         big.NewInt(1),
		LockupPeriod:        big.NewInt(1),
		LockupFixed:         big.NewInt(0),
		SettledUpTo:         big.NewInt(0),
		EndEpoch:            big.NewInt(0),
		CommissionRateBps:   big.NewInt(0),
		ServiceFeeRecipient: common.Address{},
	})

	_, err := s.SettleAuto(context.Background(), sdktypes.NewBigInt(1), nil)
	if err == nil || !strings.Contains(err.Error(), "signer required for write methods") {
		t.Fatalf("SettleAuto no signer err=%v, want signer-required error", err)
	}
	if _, ok := mb.lastIn[filPayAddr.Hex()+":getRail"]; ok {
		t.Fatal("SettleAuto fetched rail before checking signer")
	}
}

func TestGetRailsAsPayer_DefaultLimitUsesAllRemaining(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "getRailsForPayerAndToken", []filpaybind.FilecoinPayV1RailInfo{}, big.NewInt(0), big.NewInt(0))

	if _, err := s.GetRailsAsPayer(context.Background(), otherAddr, tokenAddr); err != nil {
		t.Fatalf("GetRailsAsPayer default: %v", err)
	}

	mth := mb.filPayABI.Methods["getRailsForPayerAndToken"]
	args, err := mth.Inputs.Unpack(mb.lastIn[filPayAddr.Hex()+":getRailsForPayerAndToken"][4:])
	if err != nil {
		t.Fatalf("unpack call args: %v", err)
	}
	limit := args[3].(*big.Int)
	if limit.Sign() != 0 {
		t.Fatalf("default limit = %s, want 0", limit)
	}
}

func TestGetRailsAsPayer_WithListLimitZeroPreservesZero(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "getRailsForPayerAndToken", []filpaybind.FilecoinPayV1RailInfo{}, big.NewInt(0), big.NewInt(0))

	if _, err := s.GetRailsAsPayer(context.Background(), otherAddr, tokenAddr, WithListLimit(big.NewInt(0))); err != nil {
		t.Fatalf("GetRailsAsPayer limit=0: %v", err)
	}

	mth := mb.filPayABI.Methods["getRailsForPayerAndToken"]
	args, err := mth.Inputs.Unpack(mb.lastIn[filPayAddr.Hex()+":getRailsForPayerAndToken"][4:])
	if err != nil {
		t.Fatalf("unpack call args: %v", err)
	}
	limit := args[3].(*big.Int)
	if limit.Sign() != 0 {
		t.Fatalf("explicit zero limit = %s, want 0", limit)
	}
}

func TestFetchPermitInputs_CallErrorDoesNotWrapErrPermitUnsupported(t *testing.T) {
	s, mb := newTestService(t)
	mb.errs[tokenAddr.Hex()+":name"] = context.DeadlineExceeded

	_, err := s.fetchPermitInputs(context.Background(), tokenAddr, s.signer.EVMAddress())
	if err == nil {
		t.Fatal("fetchPermitInputs err=nil, want call error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("fetchPermitInputs err=%v, want context.DeadlineExceeded", err)
	}
	if errors.Is(err, ErrPermitUnsupported) {
		t.Fatalf("fetchPermitInputs err=%v, should not wrap ErrPermitUnsupported for call failure", err)
	}
}
