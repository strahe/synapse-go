package payments

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	filpaybind "github.com/strahe/synapse-go/internal/contracts/filpay"
	sdktypes "github.com/strahe/synapse-go/types"
)

// ------------------------------------------------------------------
// GetRail / GetRailsAsPayee / WithListOffset
// ------------------------------------------------------------------

func TestGetRail_HappyPath(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "getRail", filpaybind.FilecoinPayV1RailView{
		Token:               tokenAddr,
		From:                otherAddr,
		To:                  operatorAddr,
		Operator:            operatorAddr,
		Validator:           common.Address{},
		PaymentRate:         big.NewInt(1),
		LockupPeriod:        big.NewInt(10),
		LockupFixed:         big.NewInt(0),
		SettledUpTo:         big.NewInt(0),
		EndEpoch:            big.NewInt(0),
		CommissionRateBps:   big.NewInt(0),
		ServiceFeeRecipient: common.Address{},
	})

	view, err := s.GetRail(context.Background(), sdktypes.NewBigInt(7))
	if err != nil {
		t.Fatalf("GetRail: %v", err)
	}
	if view.Token != tokenAddr || view.From != otherAddr {
		t.Fatalf("decoded rail mismatch: %+v", view)
	}
	if view.PaymentRate.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("PaymentRate = %s, want 1", view.PaymentRate)
	}
}

func TestGetRail_InvalidRailID(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.GetRail(context.Background(), sdktypes.NewBigInt(0)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("GetRail(0) err=%v, want ErrInvalidArgument", err)
	}
}

func TestGetRailsAsPayee_PaginatesAndUnpacks(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "getRailsForPayeeAndToken", []filpaybind.FilecoinPayV1RailInfo{
		{RailId: big.NewInt(1), IsTerminated: false, EndEpoch: big.NewInt(0)},
		{RailId: big.NewInt(2), IsTerminated: true, EndEpoch: big.NewInt(99)},
	}, big.NewInt(5), big.NewInt(20))

	page, err := s.GetRailsAsPayee(context.Background(), operatorAddr, tokenAddr,
		WithListOffset(big.NewInt(2)), WithListLimit(big.NewInt(10)))
	if err != nil {
		t.Fatalf("GetRailsAsPayee: %v", err)
	}
	if len(page.Rails) != 2 || page.Total.Cmp(big.NewInt(20)) != 0 {
		t.Fatalf("page decode: %+v", page)
	}
	if !page.Rails[1].IsTerminated || page.Rails[1].EndEpoch.Cmp(big.NewInt(99)) != 0 {
		t.Fatalf("rail[1] = %+v", page.Rails[1])
	}

	mth := mb.filPayABI.Methods["getRailsForPayeeAndToken"]
	args, err := mth.Inputs.Unpack(mb.lastIn[filPayAddr.Hex()+":getRailsForPayeeAndToken"][4:])
	if err != nil {
		t.Fatalf("unpack args: %v", err)
	}
	if args[2].(*big.Int).Cmp(big.NewInt(2)) != 0 || args[3].(*big.Int).Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("offset/limit args = %v", args[2:])
	}
}

func TestGetRailsAsPayee_ZeroAccountRejected(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.GetRailsAsPayee(context.Background(), common.Address{}, tokenAddr); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("GetRailsAsPayee zero acct err=%v, want ErrInvalidArgument", err)
	}
	if _, err := s.GetRailsAsPayee(context.Background(), operatorAddr, common.Address{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("GetRailsAsPayee zero token err=%v, want ErrInvalidArgument", err)
	}
}

// ------------------------------------------------------------------
// Settle / SettleTerminatedRail
// ------------------------------------------------------------------

func TestSettle_HappyPath(t *testing.T) {
	s, mb := newTestService(t)
	if _, err := s.Settle(context.Background(), sdktypes.NewBigInt(7), big.NewInt(100)); err != nil {
		t.Fatalf("Settle: %v", err)
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(mb.sent))
	}
	args, err := mb.filPayABI.Methods["settleRail"].Inputs.Unpack(mb.sent[0].Data()[4:])
	if err != nil {
		t.Fatalf("unpack settleRail: %v", err)
	}
	if got := args[0].(*big.Int); got.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("railId arg = %s, want 7", got)
	}
	if got := args[1].(*big.Int); got.Cmp(big.NewInt(100)) != 0 {
		t.Fatalf("untilEpoch arg = %s, want 100", got)
	}
}

func TestSettle_InvalidRailID(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.Settle(context.Background(), sdktypes.NewBigInt(0), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Settle(0) err=%v, want ErrInvalidArgument", err)
	}
}

func TestSettle_DefaultUntilEpochUsesBlockNumber(t *testing.T) {
	s, mb := newTestService(t)
	// blockFn default in mockBackend returns 10. resolveUntilEpoch should
	// substitute that when untilEpoch is nil or zero.
	if _, err := s.Settle(context.Background(), sdktypes.NewBigInt(1), nil); err != nil {
		t.Fatalf("Settle default epoch: %v", err)
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(mb.sent))
	}
	args, err := mb.filPayABI.Methods["settleRail"].Inputs.Unpack(mb.sent[0].Data()[4:])
	if err != nil {
		t.Fatalf("unpack settleRail: %v", err)
	}
	if got := args[0].(*big.Int); got.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("railId arg = %s, want 1", got)
	}
	// blockFn returns 10; that value must have been substituted into the
	// outgoing calldata — the core invariant this test is named after.
	if got := args[1].(*big.Int); got.Cmp(big.NewInt(10)) != 0 {
		t.Fatalf("untilEpoch arg = %s, want 10 (mock block number)", got)
	}
}

func TestSettleTerminatedRail_HappyAndInvalid(t *testing.T) {
	s, mb := newTestService(t)
	if _, err := s.SettleTerminatedRail(context.Background(), sdktypes.NewBigInt(7)); err != nil {
		t.Fatalf("SettleTerminatedRail: %v", err)
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(mb.sent))
	}
	args, err := mb.filPayABI.Methods["settleTerminatedRailWithoutValidation"].Inputs.Unpack(mb.sent[0].Data()[4:])
	if err != nil {
		t.Fatalf("unpack settleTerminatedRailWithoutValidation: %v", err)
	}
	if got := args[0].(*big.Int); got.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("railId arg = %s, want 7", got)
	}
	if _, err := s.SettleTerminatedRail(context.Background(), sdktypes.NewBigInt(0)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("SettleTerminatedRail(0) err=%v, want ErrInvalidArgument", err)
	}
}

// ------------------------------------------------------------------
// GetSettlementAmounts (eth_call simulating settleRail)
// ------------------------------------------------------------------

func TestGetSettlementAmounts_DecodesTuple(t *testing.T) {
	s, mb := newTestService(t)
	// settleRail is a state-changing method but the SDK simulates it via
	// eth_call + static ABI; we must seed a reply under that method name.
	mb.setFilPayReply(t, filPayAddr, "settleRail",
		big.NewInt(100), big.NewInt(90), big.NewInt(5), big.NewInt(5),
		big.NewInt(50), "ok",
	)
	res, err := s.GetSettlementAmounts(context.Background(), sdktypes.NewBigInt(3), big.NewInt(50))
	if err != nil {
		t.Fatalf("GetSettlementAmounts: %v", err)
	}
	if res.TotalSettledAmount.Cmp(big.NewInt(100)) != 0 || res.Note != "ok" {
		t.Fatalf("decoded = %+v", res)
	}
	if res.TotalNetPayeeAmount.Cmp(big.NewInt(90)) != 0 {
		t.Fatalf("netPayee = %s", res.TotalNetPayeeAmount)
	}
}

func TestGetSettlementAmounts_InvalidRailID(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.GetSettlementAmounts(context.Background(), sdktypes.NewBigInt(0), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("GetSettlementAmounts(0) err=%v, want ErrInvalidArgument", err)
	}
}

// ------------------------------------------------------------------
// isFwssMaxApproved
// ------------------------------------------------------------------

func TestIsFwssMaxApproved_AllApproved(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	mb.setFilPayReply(t, filPayAddr, "operatorApprovals",
		true, maxUint256, maxUint256, big.NewInt(0), big.NewInt(0), LockupPeriodEpochs,
	)
	ok, err := s.isFwssMaxApproved(context.Background())
	if err != nil {
		t.Fatalf("isFwssMaxApproved: %v", err)
	}
	if !ok {
		t.Fatal("expected true for fully-max approval")
	}
}

func TestIsFwssMaxApproved_Unapproved(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	mb.setFilPayReply(t, filPayAddr, "operatorApprovals",
		false, maxUint256, maxUint256, big.NewInt(0), big.NewInt(0), LockupPeriodEpochs,
	)
	ok, err := s.isFwssMaxApproved(context.Background())
	if err != nil {
		t.Fatalf("isFwssMaxApproved: %v", err)
	}
	if ok {
		t.Fatal("expected false when IsApproved=false")
	}
}

func TestIsFwssMaxApproved_RateTooLow(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	mb.setFilPayReply(t, filPayAddr, "operatorApprovals",
		true, big.NewInt(1), maxUint256, big.NewInt(0), big.NewInt(0), LockupPeriodEpochs,
	)
	ok, err := s.isFwssMaxApproved(context.Background())
	if err != nil {
		t.Fatalf("isFwssMaxApproved: %v", err)
	}
	if ok {
		t.Fatal("expected false when RateAllowance < maxUint256")
	}
}

// TestIsFwssMaxApproved_LockupTooLow covers the LockupAllowance < halfMax
// branch in isFwssMaxApproved (payments/fund.go:146).
func TestIsFwssMaxApproved_LockupTooLow(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	mb.setFilPayReply(t, filPayAddr, "operatorApprovals",
		true, maxUint256, big.NewInt(1), big.NewInt(0), big.NewInt(0), LockupPeriodEpochs,
	)
	ok, err := s.isFwssMaxApproved(context.Background())
	if err != nil {
		t.Fatalf("isFwssMaxApproved: %v", err)
	}
	if ok {
		t.Fatal("expected false when LockupAllowance < halfMax")
	}
}

// TestIsFwssMaxApproved_MaxLockupPeriodTooShort covers the
// MaxLockupPeriod < LockupPeriodEpochs branch (payments/fund.go:149).
func TestIsFwssMaxApproved_MaxLockupPeriodTooShort(t *testing.T) {
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	mb.setFilPayReply(t, filPayAddr, "operatorApprovals",
		true, maxUint256, maxUint256, big.NewInt(0), big.NewInt(0), big.NewInt(0),
	)
	ok, err := s.isFwssMaxApproved(context.Background())
	if err != nil {
		t.Fatalf("isFwssMaxApproved: %v", err)
	}
	if ok {
		t.Fatal("expected false when MaxLockupPeriod < LockupPeriodEpochs")
	}
}

// ------------------------------------------------------------------
// DepositWithPermit (full ERC-2612 signing path)
// ------------------------------------------------------------------

func TestDepositWithPermit_HappyPath(t *testing.T) {
	s, mb := newTestService(t)
	// seed token metadata for permit signing + wallet precheck.
	owner := s.signer.EVMAddress()
	mb.balances[owner] = new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	mb.setERC20Reply(t, tokenAddr, "balanceOf", new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18)))
	// name / version / nonces are read via permitERC20ABI on the token
	// contract; we must pack using that ABI's Outputs, not erc20ABI, since
	// erc20 bindings don't include name/version/nonces explicitly.
	for _, m := range []struct {
		name string
		v    any
	}{
		{"name", "USDFC"},
		{"version", "1"},
		{"nonces", big.NewInt(0)},
	} {
		mth, ok := permitERC20ABI.Methods[m.name]
		if !ok {
			t.Fatalf("permit ABI missing %s", m.name)
		}
		b, err := mth.Outputs.Pack(m.v)
		if err != nil {
			t.Fatalf("pack %s: %v", m.name, err)
		}
		mb.replies[tokenAddr.Hex()+":"+m.name] = b
	}

	_, err := s.DepositWithPermit(
		context.Background(),
		tokenAddr,
		common.Address{}, // credits signer
		big.NewInt(1_000),
		nil, // default deadline
		WithSkipPrecheck(),
	)
	if err != nil {
		t.Fatalf("DepositWithPermit: %v", err)
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(mb.sent))
	}
}

func TestDepositWithPermit_InvalidInputs(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.DepositWithPermit(context.Background(), common.Address{}, common.Address{}, big.NewInt(1), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("zero token err=%v, want ErrInvalidArgument", err)
	}
	if _, err := s.DepositWithPermit(context.Background(), tokenAddr, common.Address{}, big.NewInt(0), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("zero amount err=%v, want ErrInvalidArgument", err)
	}
	if _, err := s.DepositWithPermit(context.Background(), tokenAddr, common.Address{}, nil, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil amount err=%v, want ErrInvalidArgument", err)
	}
}

func TestDepositWithPermitAndApproveOperator_ZeroOperatorRejected(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.DepositWithPermitAndApproveOperator(
		context.Background(), tokenAddr, common.Address{},
		big.NewInt(1), nil,
		common.Address{},
		big.NewInt(0), big.NewInt(0), big.NewInt(0),
	)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("zero operator err=%v, want ErrInvalidArgument", err)
	}
}

// TestDepositWithPermitAndApproveOperator_HappyPath exercises the full
// combined-tx path (deposit + operator approval in a single on-chain call)
// so that any regression in argument ordering or composition is caught at
// the calldata layer, not just at the surface-validation layer.
func TestDepositWithPermitAndApproveOperator_HappyPath(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.signer.EVMAddress()
	mb.balances[owner] = new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18))
	mb.setERC20Reply(t, tokenAddr, "balanceOf", new(big.Int).Mul(big.NewInt(1_000_000), big.NewInt(1e18)))
	for _, m := range []struct {
		name string
		v    any
	}{
		{"name", "USDFC"},
		{"version", "1"},
		{"nonces", big.NewInt(0)},
	} {
		mth, ok := permitERC20ABI.Methods[m.name]
		if !ok {
			t.Fatalf("permit ABI missing %s", m.name)
		}
		b, err := mth.Outputs.Pack(m.v)
		if err != nil {
			t.Fatalf("pack %s: %v", m.name, err)
		}
		mb.replies[tokenAddr.Hex()+":"+m.name] = b
	}

	amount := big.NewInt(1_000)
	rate := big.NewInt(42)
	lockup := big.NewInt(43)
	maxPeriod := big.NewInt(44)
	_, err := s.DepositWithPermitAndApproveOperator(
		context.Background(),
		tokenAddr,
		common.Address{}, // credits signer
		amount,
		nil, // default deadline
		operatorAddr,
		rate, lockup, maxPeriod,
		WithSkipPrecheck(),
	)
	if err != nil {
		t.Fatalf("DepositWithPermitAndApproveOperator: %v", err)
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent = %d, want 1 (combined deposit+approve tx)", len(mb.sent))
	}
	args, err := mb.filPayABI.Methods["depositWithPermitAndApproveOperator"].Inputs.Unpack(mb.sent[0].Data()[4:])
	if err != nil {
		t.Fatalf("unpack depositWithPermitAndApproveOperator: %v", err)
	}
	if args[0].(common.Address) != tokenAddr {
		t.Errorf("token arg = %s, want %s", args[0], tokenAddr)
	}
	if args[1].(common.Address) != owner {
		t.Errorf("recipient arg = %s, want %s (zero→signer)", args[1], owner)
	}
	if got := args[2].(*big.Int); got.Cmp(amount) != 0 {
		t.Errorf("amount arg = %s, want %s", got, amount)
	}
	if args[7].(common.Address) != operatorAddr {
		t.Errorf("operator arg = %s, want %s", args[7], operatorAddr)
	}
	if got := args[8].(*big.Int); got.Cmp(rate) != 0 {
		t.Errorf("rateAllowance arg = %s, want %s", got, rate)
	}
	if got := args[9].(*big.Int); got.Cmp(lockup) != 0 {
		t.Errorf("lockupAllowance arg = %s, want %s", got, lockup)
	}
	if got := args[10].(*big.Int); got.Cmp(maxPeriod) != 0 {
		t.Errorf("maxLockupPeriod arg = %s, want %s", got, maxPeriod)
	}
}

func TestValidatePositive(t *testing.T) {
	if err := validatePositive("x", nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil err=%v, want ErrInvalidArgument", err)
	}
	if err := validatePositive("x", big.NewInt(0)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("zero err=%v, want ErrInvalidArgument", err)
	}
	if err := validatePositive("x", big.NewInt(-1)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("negative err=%v, want ErrInvalidArgument", err)
	}
	if err := validatePositive("x", big.NewInt(1)); err != nil {
		t.Fatalf("positive err=%v, want nil", err)
	}
}
