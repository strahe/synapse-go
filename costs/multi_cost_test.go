package costs

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
)

func TestCalculateMultiContextCosts_ReadyWhenFunded(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		usdfcFrac(1),
	)

	refs := []MultiContextRef{
		{IsNewDataSet: true},
		{IsNewDataSet: true, WithCDN: true},
	}
	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(1024),
		refs,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.NeedsFWSSMaxApproval {
		t.Error("expected approval satisfied")
	}
	if got.DepositNeeded.Sign() != 0 {
		t.Errorf("expected zero deposit: got %s", got.DepositNeeded)
	}
	if !got.Ready {
		t.Error("expected Ready=true")
	}
}

func TestCalculateMultiContextCosts_AggregatesRates(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		usdfcFrac(1),
	)

	single, err := svc.GetUploadCosts(
		context.Background(),
		common.Address{},
		bi(chain.TiB),
		&UploadCostOptions{IsNewDataSet: true},
	)
	if err != nil {
		t.Fatalf("GetUploadCosts: %v", err)
	}

	refs := []MultiContextRef{
		{IsNewDataSet: true},
		{IsNewDataSet: true},
	}
	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(chain.TiB),
		refs,
		nil,
	)
	if err != nil {
		t.Fatalf("CalculateMultiContextCosts: %v", err)
	}

	wantRate := new(big.Int).Mul(single.Rate.RatePerEpoch, big.NewInt(2))
	if got.RatePerEpoch.Cmp(wantRate) != 0 {
		t.Errorf("RatePerEpoch: want %s got %s", wantRate, got.RatePerEpoch)
	}
	if got.DepositNeeded.Sign() <= 0 {
		t.Errorf("expected positive deposit: got %s", got.DepositNeeded)
	}
}

func TestCalculateMultiContextCosts_AggregatesNewDataSetFeesAndLifecycleLockup(t *testing.T) {
	priceList := defaultPriceList()
	svc := buildSvc(t,
		&mockWS{priceList: priceList},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		new(big.Int),
	)
	opts := &UploadCostOptions{BufferEpochs: -1}

	allExisting, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(1024),
		[]MultiContextRef{{}, {}},
		opts,
	)
	if err != nil {
		t.Fatalf("all existing CalculateMultiContextCosts: %v", err)
	}
	allNew, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(1024),
		[]MultiContextRef{{IsNewDataSet: true}, {IsNewDataSet: true}},
		opts,
	)
	if err != nil {
		t.Fatalf("all new CalculateMultiContextCosts: %v", err)
	}
	mixed, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(1024),
		[]MultiContextRef{{IsNewDataSet: true}, {}},
		opts,
	)
	if err != nil {
		t.Fatalf("mixed CalculateMultiContextCosts: %v", err)
	}

	twoNewDelta := new(big.Int).Sub(allNew.DepositNeeded, allExisting.DepositNeeded)
	wantPerNew := new(big.Int).Add(priceList.Fees.CreateDataSetFee, priceList.Lockups.LifecycleReserveTarget)
	wantTwoNew := new(big.Int).Mul(wantPerNew, big.NewInt(2))
	if twoNewDelta.Cmp(wantTwoNew) != 0 {
		t.Errorf("two new dataset delta: got %s, want %s", twoNewDelta, wantTwoNew)
	}
	oneNewDelta := new(big.Int).Sub(mixed.DepositNeeded, allExisting.DepositNeeded)
	if oneNewDelta.Cmp(wantPerNew) != 0 {
		t.Errorf("one new dataset delta: got %s, want %s", oneNewDelta, wantPerNew)
	}

	wantCreateFees := new(big.Int).Mul(priceList.Fees.CreateDataSetFee, big.NewInt(2))
	if allNew.Fees.CreateDataSetFee.Cmp(wantCreateFees) != 0 {
		t.Errorf("create dataset fees: got %s, want %s", allNew.Fees.CreateDataSetFee, wantCreateFees)
	}
	wantLifecycle := new(big.Int).Mul(priceList.Lockups.LifecycleReserveTarget, big.NewInt(2))
	if allNew.Lockup.LifecycleLockup.Cmp(wantLifecycle) != 0 {
		t.Errorf("lifecycle lockup: got %s, want %s", allNew.Lockup.LifecycleLockup, wantLifecycle)
	}
}

func TestCalculateMultiContextCosts_UsesPieceCountForAddPiecesFees(t *testing.T) {
	priceList := defaultPriceList()
	svc := buildSvc(t,
		&mockWS{priceList: priceList},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		new(big.Int),
	)

	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(1024),
		[]MultiContextRef{{IsNewDataSet: true}, {}},
		&UploadCostOptions{BufferEpochs: -1, PieceCount: bi(41)},
	)
	if err != nil {
		t.Fatalf("CalculateMultiContextCosts: %v", err)
	}

	newFees := CalculateUploadFees(priceList, true, bi(41))
	existingFees := CalculateUploadFees(priceList, false, bi(41))
	wantAddPieces := new(big.Int).Add(newFees.AddPiecesFee, existingFees.AddPiecesFee)
	if got.Fees.AddPiecesFee.Cmp(wantAddPieces) != 0 {
		t.Fatalf("AddPiecesFee=%s want %s", got.Fees.AddPiecesFee, wantAddPieces)
	}
}

func TestCalculateMultiContextCosts_NilPriceListUsesZeroValue(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{nilPriceListResult: true},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		new(big.Int),
	)

	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(1024),
		[]MultiContextRef{{IsNewDataSet: true}},
		nil,
	)
	if err != nil {
		t.Fatalf("CalculateMultiContextCosts: %v", err)
	}
	if got.RatePerMonth.Sign() != 0 || got.Fees.Total.Sign() != 0 {
		t.Fatalf("got non-zero price-derived values: rate=%s fees=%s", got.RatePerMonth, got.Fees.Total)
	}
}

func TestCalculateMultiContextCosts_EmptyRefs(t *testing.T) {
	svc := buildSvc(t, &mockWS{price: defaultPrice()}, &mockPay{}, usdfcFrac(1))
	if _, err := svc.CalculateMultiContextCosts(
		context.Background(), common.Address{}, bi(1024), nil, nil,
	); err == nil {
		t.Error("expected error for empty refs")
	}
}
