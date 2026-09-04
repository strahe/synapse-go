package costs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"reflect"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/warmstorage"
)

// --- mocks ---

type mockWS struct {
	priceList          *warmstorage.PriceList
	priceListErr       error
	nilPriceListResult bool
}

func (m *mockWS) GetPriceList(_ context.Context) (*warmstorage.PriceList, error) {
	if m.priceListErr != nil {
		return nil, m.priceListErr
	}
	if m.nilPriceListResult {
		return nil, nil
	}
	if m.priceList == nil {
		return defaultPriceList(), nil
	}
	return m.priceList, nil
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

type strictPay struct {
	mu           sync.Mutex
	wantToken    common.Address
	wantOwner    common.Address
	wantOperator common.Address
	account      *payments.AccountState
	approval     *payments.OperatorApproval
	accountCalls int
	approvalCall int
}

func (m *strictPay) AccountInfo(_ context.Context, token, owner common.Address) (*payments.AccountState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if token != m.wantToken {
		return nil, fmt.Errorf("AccountInfo token=%s want %s", token, m.wantToken)
	}
	if owner != m.wantOwner {
		return nil, fmt.Errorf("AccountInfo owner=%s want %s", owner, m.wantOwner)
	}
	m.accountCalls++
	return m.account, nil
}

func (m *strictPay) ServiceApproval(_ context.Context, token, client, operator common.Address) (*payments.OperatorApproval, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if token != m.wantToken {
		return nil, fmt.Errorf("ServiceApproval token=%s want %s", token, m.wantToken)
	}
	if client != m.wantOwner {
		return nil, fmt.Errorf("ServiceApproval client=%s want %s", client, m.wantOwner)
	}
	if operator != m.wantOperator {
		return nil, fmt.Errorf("ServiceApproval operator=%s want %s", operator, m.wantOperator)
	}
	m.approvalCall++
	return m.approval, nil
}

func (m *strictPay) calls() (account, approval int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.accountCalls, m.approvalCall
}

type mockCaller struct {
	blockNumber uint64
	blockErr    error
}

func (m *mockCaller) BlockNumber(_ context.Context) (uint64, error) {
	if m.blockErr != nil {
		return 0, m.blockErr
	}
	return m.blockNumber, nil
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

func maxApproval() *payments.OperatorApproval {
	return &payments.OperatorApproval{
		IsApproved:      true,
		RateAllowance:   new(big.Int).Set(maxUint256),
		LockupAllowance: new(big.Int).Set(maxUint256),
		MaxLockupPeriod: big.NewInt(DefaultLockupPeriod),
	}
}

func buildSvc(t *testing.T, ws WarmStorageReader, pay PaymentsReader) *Service {
	t.Helper()
	svc, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: ws,
		Payments:    pay,
		Caller:      &mockCaller{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// --- tests ---

func TestGetPriceList(t *testing.T) {
	want := defaultPriceList()
	svc := buildSvc(t, &mockWS{priceList: want}, &mockPay{
		account:  &payments.AccountState{},
		approval: &payments.OperatorApproval{},
	})

	got, err := svc.GetPriceList(context.Background())
	if err != nil {
		t.Fatalf("GetPriceList: %v", err)
	}
	if got != want {
		t.Fatalf("GetPriceList returned %p, want delegated value %p", got, want)
	}
}

func TestGetPriceList_PropagatesReaderError(t *testing.T) {
	wantErr := errors.New("price list unavailable")
	svc := buildSvc(t, &mockWS{priceListErr: wantErr}, &mockPay{
		account:  &payments.AccountState{},
		approval: &payments.OperatorApproval{},
	})

	if _, err := svc.GetPriceList(context.Background()); !errors.Is(err, wantErr) {
		t.Fatalf("GetPriceList error = %v, want %v", err, wantErr)
	}
}

func TestService_UsesExplicitPaymentAddresses(t *testing.T) {
	token := common.HexToAddress("0x0000000000000000000000000000000000000aAa")
	operator := common.HexToAddress("0x0000000000000000000000000000000000000bBb")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000cCc")
	pay := &strictPay{
		wantToken:    token,
		wantOwner:    owner,
		wantOperator: operator,
		account: &payments.AccountState{
			Funds:         usdfc(1_000_000),
			LockupCurrent: new(big.Int),
			LockupRate:    new(big.Int),
		},
		approval: maxApproval(),
	}
	svc, err := New(Options{
		Chain:              chain.Calibration,
		USDFCTokenAddress:  token,
		WarmStorageAddress: operator,
		WarmStorage:        &mockWS{priceList: defaultPriceList()},
		Payments:           pay,
		Caller:             &mockCaller{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.usdfc != token {
		t.Fatalf("service USDFC=%s want %s", svc.usdfc, token)
	}
	if svc.fwss != operator {
		t.Fatalf("service FWSS=%s want %s", svc.fwss, operator)
	}

	if _, err := svc.GetUploadCosts(context.Background(), owner, bi(1024), &UploadCostOptions{IsNewDataSet: true}); err != nil {
		t.Fatalf("GetUploadCosts: %v", err)
	}
	if _, err := svc.CalculateMultiContextCosts(
		context.Background(),
		owner,
		bi(1024),
		[]MultiContextRef{{IsNewDataSet: true}},
		&UploadCostOptions{},
	); err != nil {
		t.Fatalf("CalculateMultiContextCosts: %v", err)
	}
	accountCalls, approvalCalls := pay.calls()
	if accountCalls != 2 {
		t.Fatalf("AccountInfo calls=%d want 2", accountCalls)
	}
	if approvalCalls != 2 {
		t.Fatalf("ServiceApproval calls=%d want 2", approvalCalls)
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
		&mockWS{},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(100), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: notApproved,
		})

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
		&mockWS{},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

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
		&mockWS{},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

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

func TestGetUploadCosts_UsesPriceListFeesAndLifecycleLockup(t *testing.T) {
	priceList := defaultPriceList()
	svc := buildSvc(t,
		&mockWS{priceList: priceList},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

	newDataSet, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), &UploadCostOptions{
		IsNewDataSet: true,
		PieceCount:   bi(41),
	})
	if err != nil {
		t.Fatalf("new dataset GetUploadCosts: %v", err)
	}
	wantFees := CalculateUploadFees(priceList, true, bi(41))
	if newDataSet.Fees.Total.Cmp(wantFees.Total) != 0 {
		t.Errorf("new dataset fees: got %s, want %s", newDataSet.Fees.Total, wantFees.Total)
	}
	if newDataSet.Lockup.LifecycleLockup.Cmp(priceList.Lockups.LifecycleReserveTarget) != 0 {
		t.Errorf("new dataset lifecycle lockup: got %s, want %s", newDataSet.Lockup.LifecycleLockup, priceList.Lockups.LifecycleReserveTarget)
	}

	existingDataSet, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), &UploadCostOptions{
		CurrentDataSetSizeBytes: bi(chain.TiB),
	})
	if err != nil {
		t.Fatalf("existing dataset GetUploadCosts: %v", err)
	}
	wantExistingFees := CalculateUploadFees(priceList, false, nil)
	if existingDataSet.Fees.Total.Cmp(wantExistingFees.Total) != 0 {
		t.Errorf("existing dataset fees: got %s, want %s", existingDataSet.Fees.Total, wantExistingFees.Total)
	}
	if existingDataSet.Lockup.LifecycleLockup.Sign() != 0 {
		t.Errorf("existing dataset lifecycle lockup: got %s, want 0", existingDataSet.Lockup.LifecycleLockup)
	}
}

func TestGetUploadCosts_NilOpts_UsesDefaults(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

	// nil opts must not panic
	costs, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if costs == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGetUploadCosts_BufferEpochOptions(t *testing.T) {
	account := &payments.AccountState{
		Funds:         new(big.Int),
		LockupCurrent: new(big.Int),
		LockupRate:    bi(100),
	}
	svc := buildSvc(t,
		&mockWS{priceList: defaultPriceList()},
		&mockPay{account: account, approval: maxApproval()})

	withoutBuffer, err := svc.GetUploadCosts(
		context.Background(), common.Address{}, bi(1024),
		&UploadCostOptions{BufferEpochs: new(int64(0))},
	)
	if err != nil {
		t.Fatalf("GetUploadCosts without buffer: %v", err)
	}

	tests := []struct {
		name         string
		bufferEpochs *int64
		wantEpochs   int64
	}{
		{name: "default", wantEpochs: DefaultBufferEpochs},
		{name: "explicit zero", bufferEpochs: new(int64(0))},
		{name: "positive", bufferEpochs: new(int64(9)), wantEpochs: 9},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.GetUploadCosts(
				context.Background(), common.Address{}, bi(1024),
				&UploadCostOptions{BufferEpochs: tt.bufferEpochs},
			)
			if err != nil {
				t.Fatalf("GetUploadCosts: %v", err)
			}

			combinedRate := new(big.Int).Add(account.LockupRate, got.Lockup.RateDeltaPerEpoch)
			wantDelta := new(big.Int).Mul(combinedRate, big.NewInt(tt.wantEpochs))
			gotDelta := new(big.Int).Sub(got.DepositNeeded, withoutBuffer.DepositNeeded)
			if gotDelta.Cmp(wantDelta) != 0 {
				t.Fatalf("buffer deposit delta=%s want %s", gotDelta, wantDelta)
			}
		})
	}
}

func TestCostServicesRejectNegativeBufferEpochsBeforeBackendReads(t *testing.T) {
	backendErr := errors.New("backend must not be called")
	svc := buildSvc(t,
		&mockWS{priceListErr: backendErr},
		&mockPayErr{err: backendErr})
	svc.caller = &mockCaller{blockErr: backendErr}
	negative := new(int64(-1))

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "GetUploadCosts",
			call: func() error {
				_, err := svc.GetUploadCosts(
					context.Background(), common.Address{}, bi(1024),
					&UploadCostOptions{BufferEpochs: negative},
				)
				return err
			},
		},
		{
			name: "CalculateMultiContextCosts",
			call: func() error {
				_, err := svc.CalculateMultiContextCosts(
					context.Background(), common.Address{}, bi(1024),
					[]MultiContextRef{{}}, &UploadCostOptions{BufferEpochs: negative},
				)
				return err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v want ErrInvalidArgument", err)
			}
			if errors.Is(err, backendErr) {
				t.Fatalf("error=%v unexpectedly includes backend error", err)
			}
		})
	}
}

func TestGetUploadCosts_NilPriceListUsesZeroValue(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{nilPriceListResult: true},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

	got, err := svc.GetUploadCosts(context.Background(), common.Address{}, bi(1024), nil)
	if err != nil {
		t.Fatalf("GetUploadCosts: %v", err)
	}
	if got.Rate.RatePerMonth.Sign() != 0 || got.Fees.Total.Sign() != 0 {
		t.Fatalf("got non-zero price-derived values: rate=%s fees=%s", got.Rate.RatePerMonth, got.Fees.Total)
	}
}

func TestAdditionalLockup_OnlyExposesCurrentFields(t *testing.T) {
	typ := reflect.TypeFor[AdditionalLockup]()
	got := make([]string, typ.NumField())
	for i := range typ.NumField() {
		got[i] = typ.Field(i).Name
	}
	want := []string{
		"RateDeltaPerEpoch",
		"StreamingLockup",
		"LifecycleLockup",
		"CDNLockup",
		"CacheMissLockup",
		"Total",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AdditionalLockup fields=%v want %v", got, want)
	}
}

func TestUploadCostOptions_OnlyExposeCurrentFields(t *testing.T) {
	typ := reflect.TypeFor[UploadCostOptions]()
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
		"PieceCount",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UploadCostOptions fields=%v want %v", got, want)
	}
}

func TestCurrentEpochPrefersCallerBlockNumber(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{},
		&mockPay{account: &payments.AccountState{}, approval: &payments.OperatorApproval{}})

	svc.caller = &mockCaller{blockNumber: 1234}

	got, err := svc.currentEpoch(context.Background())
	if err != nil {
		t.Fatalf("currentEpoch: %v", err)
	}

	if got.Cmp(big.NewInt(1234)) != 0 {
		t.Fatalf("currentEpoch = %s, want 1234", got)
	}
}

func TestCurrentEpochErrorsWhenBlockNumberFails(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{},
		&mockPay{account: &payments.AccountState{}, approval: &payments.OperatorApproval{}})

	svc.caller = &mockCaller{blockErr: errors.New("rpc down")}

	_, err := svc.currentEpoch(context.Background())
	if err == nil {
		t.Fatal("expected BlockNumber error")
	}
}

func TestGetUploadCosts_PartialGoroutineFailure(t *testing.T) {
	// payments goroutines fail; GetPriceList succeeds. Verify error is propagated.
	payErr := fmt.Errorf("rpc unavailable")
	svc := buildSvc(t,
		&mockWS{},
		&mockPayErr{err: payErr})

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
		Caller:   &mockCaller{},
	})
	if err == nil {
		t.Fatal("expected error for nil WarmStorage")
	}
}

func TestNew_NilPayments(t *testing.T) {
	_, err := New(Options{
		Chain:       chain.Calibration,
		WarmStorage: &mockWS{},
		Caller:      &mockCaller{},
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
		Caller:      &mockCaller{},
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
		Caller:      &mockCaller{},
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
		Caller:      &mockCaller{},
	})
	if err == nil {
		t.Fatal("expected error for unsupported chain")
	}
	if !errors.Is(err, chain.ErrUnknownChain) {
		t.Fatalf("expected wrapped ErrUnknownChain, got %v", err)
	}
}
