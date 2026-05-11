package costs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"reflect"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/warmstorage"
)

// --- mocks ---

type mockWS struct{ price *warmstorage.ServicePrice }

func (m *mockWS) GetServicePrice(_ context.Context) (*warmstorage.ServicePrice, error) {
	return m.price, nil
}

type mockPay struct {
	account  *payments.AccountState
	approval *payments.OperatorApproval
}

func (m *mockPay) AccountInfo(_ context.Context, _, _ common.Address) (*payments.AccountState, error) {
	return m.account, nil
}

func (m *mockPay) ServiceApproval(_ context.Context, _, _, _ common.Address) (*payments.OperatorApproval, error) {
	return m.approval, nil
}

type mockCaller struct{ fee *big.Int }

func (m *mockCaller) CallContract(_ context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	return usdfcSybilFeeABI.Methods["USDFC_SYBIL_FEE"].Outputs.Pack(m.fee)
}

// mockPayErr is a PaymentsReader that returns errors on all calls.
type mockPayErr struct{ err error }

func (m *mockPayErr) AccountInfo(_ context.Context, _, _ common.Address) (*payments.AccountState, error) {
	return nil, m.err
}

func (m *mockPayErr) ServiceApproval(_ context.Context, _, _, _ common.Address) (*payments.OperatorApproval, error) {
	return nil, m.err
}

// --- helpers ---

func defaultPrice() *warmstorage.ServicePrice {
	return &warmstorage.ServicePrice{
		PricePerTiBPerMonthNoCDN: usdfcFrac(25),
		MinimumPricePerMonth:     usdfcFrac(1),
		EpochsPerMonth:           big.NewInt(chain.EpochsPerMonth),
	}
}

func maxApproval() *payments.OperatorApproval {
	return &payments.OperatorApproval{
		IsApproved:      true,
		RateAllowance:   new(big.Int).Set(maxUint256),
		LockupAllowance: new(big.Int).Set(maxUint256),
		MaxLockupPeriod: big.NewInt(DefaultLockupPeriod),
	}
}

func buildSvc(t *testing.T, ws WarmStorageReader, pay PaymentsReader, fee *big.Int) *Service {
	t.Helper()
	svc, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: ws,
		Payments:    pay,
		Caller:      &mockCaller{fee: fee},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// --- tests ---

func TestGetServicePrice(t *testing.T) {
	ws := &mockWS{price: defaultPrice()}
	svc := buildSvc(t, ws, &mockPay{
		account:  &payments.AccountState{},
		approval: &payments.OperatorApproval{},
	}, new(big.Int))

	price, err := svc.GetServicePrice(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if price.EpochsPerMonth.Int64() != chain.EpochsPerMonth {
		t.Errorf("EpochsPerMonth: got %d, want %d", price.EpochsPerMonth.Int64(), chain.EpochsPerMonth)
	}
}

func TestGetUploadCosts_NeedsApproval(t *testing.T) {
	notApproved := &payments.OperatorApproval{
		IsApproved:      false,
		RateAllowance:   big.NewInt(1_000_000),
		LockupAllowance: new(big.Int),
		MaxLockupPeriod: big.NewInt(DefaultLockupPeriod),
	}
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(100), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: notApproved,
		},
		usdfcFrac(1),
	)

	costs, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), &UploadCostOptions{IsNewDataSet: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !costs.NeedsFWSSMaxApproval {
		t.Error("expected NeedsFWSSMaxApproval=true")
	}
	if costs.Ready {
		t.Error("expected Ready=false when needs approval")
	}
}

func TestGetUploadCosts_ReadyWhenFundedAndApproved(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		usdfcFrac(1),
	)

	costs, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), &UploadCostOptions{IsNewDataSet: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if costs.NeedsFWSSMaxApproval {
		t.Error("expected NeedsFWSSMaxApproval=false")
	}
	if costs.DepositNeeded.Sign() != 0 {
		t.Errorf("expected zero deposit for well-funded account: got %s", costs.DepositNeeded)
	}
	if !costs.Ready {
		t.Error("expected Ready=true")
	}
}

func TestGetUploadCosts_DepositPositive_WhenUnderfunded(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		usdfcFrac(1),
	)

	costs, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(chain.TiB), &UploadCostOptions{IsNewDataSet: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if costs.DepositNeeded.Sign() <= 0 {
		t.Errorf("expected positive deposit: got %s", costs.DepositNeeded)
	}
	if costs.Ready {
		t.Error("expected Ready=false when deposit needed")
	}
}

func TestGetUploadCosts_NilOpts_UsesDefaults(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		usdfcFrac(1),
	)
	// nil opts must not panic
	costs, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if costs == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestUploadCostOptions_OnlyExposeCurrentFields(t *testing.T) {
	typ := reflect.TypeOf(UploadCostOptions{})
	got := make([]string, typ.NumField())
	for i := range typ.NumField() {
		got[i] = typ.Field(i).Name
	}
	want := []string{
		"ExtraRunwayEpochs",
		"BufferEpochs",
		"EnableCDN",
		"IsNewDataSet",
		"CurrentDataSetSizeBytes",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UploadCostOptions fields=%v want %v", got, want)
	}
}

func TestAccountSummary_OnlyExposeCompatibilityFields(t *testing.T) {
	typ := reflect.TypeOf(AccountSummary{})
	got := make([]string, typ.NumField())
	for i := range typ.NumField() {
		got[i] = typ.Field(i).Name
	}
	want := []string{
		"Funds",
		"AvailableFunds",
		"Debt",
		"LockupRatePerEpoch",
		"LockupRatePerMonth",
		"FundedUntilEpoch",
		"CurrentEpoch",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AccountSummary fields=%v want %v", got, want)
	}
}

func TestGetAccountSummary(t *testing.T) {
	rate := big.NewInt(500_000)
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account: &payments.AccountState{
				Funds:         usdfc(10),
				LockupCurrent: usdfc(3),
				LockupRate:    rate,
			},
			approval: &payments.OperatorApproval{},
		},
		usdfcFrac(1),
	)

	summary, err := svc.GetAccountSummary(context.Background(), common.Address{}) //nolint:staticcheck // compatibility entry point must stay covered
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Funds.Cmp(usdfc(10)) != 0 {
		t.Errorf("Funds: got %s, want %s", summary.Funds, usdfc(10))
	}
	want := new(big.Int).Sub(usdfc(10), usdfc(3))
	if summary.AvailableFunds.Cmp(want) != 0 {
		t.Errorf("AvailableFunds: got %s, want %s", summary.AvailableFunds, want)
	}
	if summary.Debt.Sign() != 0 {
		t.Errorf("Debt should be 0 when Funds > LockupCurrent: got %s", summary.Debt)
	}
	wantMonthlyRate := new(big.Int).Mul(rate, big.NewInt(chain.EpochsPerMonth))
	if summary.LockupRatePerMonth.Cmp(wantMonthlyRate) != 0 {
		t.Errorf("LockupRatePerMonth: got %s, want %s", summary.LockupRatePerMonth, wantMonthlyRate)
	}
}

func TestGetAccountSummary_Debt(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account: &payments.AccountState{
				Funds:         usdfc(1),
				LockupCurrent: usdfc(5), // locked > funds → debt
				LockupRate:    new(big.Int),
			},
			approval: &payments.OperatorApproval{},
		},
		new(big.Int),
	)

	summary, err := svc.GetAccountSummary(context.Background(), common.Address{}) //nolint:staticcheck // compatibility entry point must stay covered
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantDebt := new(big.Int).Sub(usdfc(5), usdfc(1))
	if summary.Debt.Cmp(wantDebt) != 0 {
		t.Errorf("Debt: got %s, want %s", summary.Debt, wantDebt)
	}
	if summary.AvailableFunds.Sign() != 0 {
		t.Errorf("AvailableFunds should be 0 when in debt: got %s", summary.AvailableFunds)
	}
}

func TestGetUploadCosts_PartialGoroutineFailure(t *testing.T) {
	// payments goroutines fail; GetServicePrice succeeds. Verify error is propagated.
	payErr := fmt.Errorf("rpc unavailable")
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPayErr{err: payErr},
		usdfcFrac(1),
	)

	_, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), nil)
	if err == nil {
		t.Fatal("expected error when payments RPC fails")
	}
	// Both AccountInfo and ServiceApproval fail — errors.Join wraps both.
	// Verify at least one is reachable via unwrap chain.
	if !errors.Is(err, payErr) {
		t.Errorf("expected wrapped payErr in error chain, got: %v", err)
	}
}

// --- New validation ---

func TestNew_NilWarmStorage(t *testing.T) {
	_, err := New(Options{
		Chain:    chain.Calibration,
		Payments: &mockPay{},
		Caller:   &mockCaller{fee: new(big.Int)},
	})
	if err == nil {
		t.Fatal("expected error for nil WarmStorage")
	}
}

func TestNew_NilPayments(t *testing.T) {
	_, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: &mockWS{},
		Caller:      &mockCaller{fee: new(big.Int)},
	})
	if err == nil {
		t.Fatal("expected error for nil Payments")
	}
}

func TestNew_NilCaller(t *testing.T) {
	_, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: &mockWS{},
		Payments:    &mockPay{},
	})
	if err == nil {
		t.Fatal("expected error for nil Caller")
	}
}

func TestNew_LoggerViaOptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: &mockWS{},
		Payments:    &mockPay{},
		Caller:      &mockCaller{fee: new(big.Int)},
		Logger:      logger,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.logger != logger {
		t.Error("expected logger to be set")
	}
}

// TestNew_ChainZeroValueIsMainnet guards the documented contract that an
// omitted Options.Chain (zero value) is equivalent to chain.Mainnet. A
// future refactor that starts treating zero as "unset/invalid" would break
// this and should be caught here.
func TestNew_ChainZeroValueIsMainnet(t *testing.T) {
	svc, err := New(Options{
		WarmStorage: &mockWS{},
		Payments:    &mockPay{},
		Caller:      &mockCaller{fee: new(big.Int)},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mainnetAddrs := chain.Mainnet.Addresses()
	if svc.fwss != mainnetAddrs.FWSS {
		t.Errorf("expected mainnet FWSS address for zero-value Chain, got %s", svc.fwss)
	}
}

func TestNew_UnsupportedChain(t *testing.T) {
	_, err := New(Options{
		Chain:       chain.Chain(255),
		WarmStorage: &mockWS{},
		Payments:    &mockPay{},
		Caller:      &mockCaller{fee: new(big.Int)},
	})
	if err == nil {
		t.Fatal("expected error for unsupported chain")
	}
	if !errors.Is(err, chain.ErrUnknownChain) {
		t.Fatalf("expected wrapped ErrUnknownChain, got %v", err)
	}
}

// --- readUsdfcSybilFee error paths ---

type mockCallerErr struct{ err error }

func (m *mockCallerErr) CallContract(_ context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	return nil, m.err
}

type mockCallerBadReturn struct{ data []byte }

func (m *mockCallerBadReturn) CallContract(_ context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	return m.data, nil
}

func TestReadUsdfcSybilFee_CallContractError(t *testing.T) {
	svc, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: &mockWS{},
		Payments:    &mockPay{},
		Caller:      &mockCallerErr{err: fmt.Errorf("rpc down")},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.readUsdfcSybilFee(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadUsdfcSybilFee_UnpackError(t *testing.T) {
	svc, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: &mockWS{},
		Payments:    &mockPay{},
		Caller:      &mockCallerBadReturn{data: []byte{0x01, 0x02}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.readUsdfcSybilFee(context.Background())
	if err == nil {
		t.Fatal("expected unpack error")
	}
}

// --- GetAccountSummary error ---

func TestGetAccountSummary_Error(t *testing.T) {
	payErr := fmt.Errorf("rpc down")
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPayErr{err: payErr},
		new(big.Int),
	)
	_, err := svc.GetAccountSummary(context.Background(), common.Address{}) //nolint:staticcheck // compatibility entry point must stay covered
	if !errors.Is(err, payErr) {
		t.Fatalf("want wrapped payErr, got %v", err)
	}
}

// --- GetAccountSummary with nil fields ---

func TestGetAccountSummary_NilFields(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account: &payments.AccountState{
				Funds:         nil,
				LockupCurrent: nil,
				LockupRate:    nil,
			},
			approval: &payments.OperatorApproval{},
		},
		new(big.Int),
	)
	summary, err := svc.GetAccountSummary(context.Background(), common.Address{}) //nolint:staticcheck // compatibility entry point must stay covered
	if err != nil {
		t.Fatal(err)
	}
	if summary.Funds.Sign() != 0 {
		t.Errorf("Funds should be 0, got %s", summary.Funds)
	}
	if summary.Debt.Sign() != 0 {
		t.Errorf("Debt should be 0, got %s", summary.Debt)
	}
}
