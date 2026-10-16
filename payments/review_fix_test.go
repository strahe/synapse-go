package payments

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/strahe/synapse-go/chain"
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

func TestFund_UsesInjectedApprovalLockupPeriod(t *testing.T) {
	for _, tt := range []struct {
		name   string
		period *big.Int
	}{
		{name: "ordinary period", period: big.NewInt(123)},
		{name: "maximum period", period: new(big.Int).Set(maxUint256)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, mb := newTestService(t)
			s.warmStorage = operatorAddr
			s.usdfcToken = tokenAddr
			s.lockups = &fakeApprovalLockupReader{period: tt.period}

			if _, err := s.Fund(context.Background(), big.NewInt(0), WithFundNeedsFwssApproval(true)); err != nil {
				t.Fatalf("Fund: %v", err)
			}
			if got := sentApprovalMaxLockupPeriod(t, mb); got.Cmp(tt.period) != 0 {
				t.Fatalf("maxLockupPeriod=%s want %s", got, tt.period)
			}
		})
	}
}

func TestFund_ExplicitApprovalLockupPeriodOverridesReader(t *testing.T) {
	reader := &fakeApprovalLockupReader{period: big.NewInt(123)}
	s, mb := newTestService(t)
	s.warmStorage = operatorAddr
	s.usdfcToken = tokenAddr
	s.lockups = reader

	if _, err := s.Fund(
		context.Background(),
		big.NewInt(0),
		WithFundNeedsFwssApproval(true),
		WithFundApprovalLockupPeriod(big.NewInt(456)),
	); err != nil {
		t.Fatalf("Fund: %v", err)
	}
	if reader.calls != 0 {
		t.Fatalf("lockup reader calls=%d want 0", reader.calls)
	}
	if got := sentApprovalMaxLockupPeriod(t, mb); got.Cmp(big.NewInt(456)) != 0 {
		t.Fatalf("maxLockupPeriod=%s want 456", got)
	}
}

func TestFund_StandaloneApprovalLockupPeriodFallback(t *testing.T) {
	var typedNil *fakeApprovalLockupReader
	for _, tt := range []struct {
		name   string
		reader ApprovalLockupPeriodReader
	}{
		{"nil reader", nil},
		{"typed-nil reader", typedNil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mb := newMockBackend(t)
			s, err := New(Options{
				Backend:              mb,
				ChainID:              sdktypes.ChainID(314159),
				FilPayAddress:        filPayAddr,
				WarmStorageAddress:   operatorAddr,
				USDFCTokenAddress:    tokenAddr,
				Signer:               newTestSigner(t),
				ApprovalLockupPeriod: tt.reader,
			})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if _, err := s.Fund(context.Background(), big.NewInt(0), WithFundNeedsFwssApproval(true)); err != nil {
				t.Fatalf("Fund: %v", err)
			}
			want := big.NewInt(chain.EpochsPerMonth)
			if got := sentApprovalMaxLockupPeriod(t, mb); got.Cmp(want) != 0 {
				t.Fatalf("maxLockupPeriod=%s want %s", got, want)
			}
		})
	}
}

func TestFundSync_WaitTimeout(t *testing.T) {
	for _, tt := range []struct {
		name string
		opts []WriteOption
		wait time.Duration
	}{
		{"omitted", nil, 5 * time.Minute},
		{"zero", []WriteOption{WithWait(0)}, 5 * time.Minute},
		{"negative", []WriteOption{WithWait(-time.Second)}, 5 * time.Minute},
		{"positive", []WriteOption{WithWait(time.Minute)}, time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, mb := newTestService(t)
			s.warmStorage = operatorAddr
			s.usdfcToken = tokenAddr
			receiptCalls := 0
			started := time.Now()
			mb.receiptFn = func(ctx context.Context, _ common.Hash) (*coretypes.Receipt, error) {
				receiptCalls++
				deadline, ok := ctx.Deadline()
				if !ok || deadline.Before(started.Add(tt.wait)) || deadline.After(time.Now().Add(tt.wait)) {
					t.Fatalf("receipt deadline = %s, want timeout %s", deadline, tt.wait)
				}
				return &coretypes.Receipt{Status: 1}, nil
			}

			opts := append([]WriteOption{WithFundNeedsFwssApproval(true)}, tt.opts...)
			got, err := s.FundSync(context.Background(), big.NewInt(0), opts...)
			if err != nil {
				t.Fatalf("FundSync: %v", err)
			}
			if got == nil || got.Receipt == nil {
				t.Fatalf("FundSync result = %+v, want receipt", got)
			}
			if len(mb.sent) != 1 || receiptCalls != 1 {
				t.Fatalf("sent = %d, receipt calls = %d, want one each", len(mb.sent), receiptCalls)
			}
		})
	}
}

type fakeApprovalLockupReader struct {
	period *big.Int
	calls  int
}

func (f *fakeApprovalLockupReader) ApprovalLockupPeriod(context.Context) (*big.Int, error) {
	f.calls++
	return f.period, nil
}

func sentApprovalMaxLockupPeriod(t *testing.T, mb *mockBackend) *big.Int {
	t.Helper()
	if len(mb.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(mb.sent))
	}
	method := mb.filPayABI.Methods["setOperatorApproval"]
	args, err := method.Inputs.Unpack(mb.sent[0].Data()[4:])
	if err != nil {
		t.Fatalf("unpack setOperatorApproval: %v", err)
	}
	return args[5].(*big.Int)
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

func TestGetRailsAsPayer_ExplicitPageOptions(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "getRailsForPayerAndToken", []filpaybind.FilecoinPayV1RailInfo{}, big.NewInt(0), big.NewInt(0))

	opts := sdktypes.ListOptions{Offset: ^uint64(0), Limit: ^uint64(0)}
	if _, err := s.GetRailsAsPayer(context.Background(), otherAddr, tokenAddr, opts); err != nil {
		t.Fatalf("GetRailsAsPayer: %v", err)
	}

	mth := mb.filPayABI.Methods["getRailsForPayerAndToken"]
	args, err := mth.Inputs.Unpack(mb.lastIn[filPayAddr.Hex()+":getRailsForPayerAndToken"][4:])
	if err != nil {
		t.Fatalf("unpack call args: %v", err)
	}
	for i, want := range []uint64{opts.Offset, opts.Limit} {
		if got := args[i+2].(*big.Int); got.Cmp(new(big.Int).SetUint64(want)) != 0 {
			t.Fatalf("offset/limit arg %d = %s, want %d", i, got, want)
		}
	}
}

func TestGetRails_ZeroLimitRejectedBeforeRPC(t *testing.T) {
	for _, asPayer := range []bool{true, false} {
		name := "payee"
		if asPayer {
			name = "payer"
		}
		t.Run(name, func(t *testing.T) {
			s, mb := newTestService(t)
			list := s.GetRailsAsPayee
			if asPayer {
				list = s.GetRailsAsPayer
			}
			_, err := list(context.Background(), otherAddr, tokenAddr, sdktypes.ListOptions{})
			if !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, sdktypes.ErrInvalidListOptions) {
				t.Fatalf("zero limit err=%v, want ErrInvalidArgument and ErrInvalidListOptions", err)
			}
			if len(mb.lastIn) != 0 {
				t.Fatalf("zero limit made RPC calls: %v", mb.lastIn)
			}
		})
	}
}
