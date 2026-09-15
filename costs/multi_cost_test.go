package costs

import (
	"context"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestCalculateMultiContextCosts_ReadyWhenFunded(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

	refs := []MultiContextRef{
		{IsNewDataSet: true},
		{IsNewDataSet: true, WithCDN: true},
	}
	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		[]uint64{1024},
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
		&mockWS{},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

	single, err := svc.GetUploadCosts(
		context.Background(),
		common.Address{},
		[]uint64{chain.MaxUploadSize},
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
		[]uint64{chain.MaxUploadSize},
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
		})

	opts := &UploadCostOptions{BufferEpochs: new(int64(0))}

	allExisting, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		[]uint64{1024},
		[]MultiContextRef{existingMultiContextRef(new(big.Int)), existingMultiContextRef(new(big.Int))},
		opts,
	)
	if err != nil {
		t.Fatalf("all existing CalculateMultiContextCosts: %v", err)
	}
	allNew, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		[]uint64{1024},
		[]MultiContextRef{{IsNewDataSet: true}, {IsNewDataSet: true}},
		opts,
	)
	if err != nil {
		t.Fatalf("all new CalculateMultiContextCosts: %v", err)
	}
	mixed, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		[]uint64{1024},
		[]MultiContextRef{{IsNewDataSet: true}, existingMultiContextRef(new(big.Int))},
		opts,
	)
	if err != nil {
		t.Fatalf("mixed CalculateMultiContextCosts: %v", err)
	}

	twoNewDelta := new(big.Int).Sub(allNew.DepositNeeded, allExisting.DepositNeeded)
	reserveFunding, err := CalculateLifecycleReserveFunding(LifecycleReserveCalculation{
		PriceList: priceList, PieceSizes: []uint64{1024}, IsNewDataSet: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantPerNew := reserveFunding.Total
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

func TestCalculateMultiContextCosts_PricesEveryPieceForEachContext(t *testing.T) {
	priceList := defaultPriceList()
	svc := buildSvc(t,
		&mockWS{priceList: priceList},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

	pieceSizes := slices.Repeat([]uint64{128}, 41)
	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		pieceSizes,
		[]MultiContextRef{{IsNewDataSet: true}, existingMultiContextRef(new(big.Int))},
		&UploadCostOptions{BufferEpochs: new(int64(0))},
	)
	if err != nil {
		t.Fatalf("CalculateMultiContextCosts: %v", err)
	}

	newFees, err := CalculateUploadFees(priceList, true, pieceSizes)
	if err != nil {
		t.Fatal(err)
	}
	existingFees, err := CalculateUploadFees(priceList, false, pieceSizes)
	if err != nil {
		t.Fatal(err)
	}
	wantAddPieces := new(big.Int).Add(newFees.AddPiecesFee, existingFees.AddPiecesFee)
	if got.Fees.AddPiecesFee.Cmp(wantAddPieces) != 0 {
		t.Fatalf("AddPiecesFee=%s want %s", got.Fees.AddPiecesFee, wantAddPieces)
	}
}

func TestCalculateMultiContextCosts_AggregatesReservePerContextAndAccountDebtOnce(t *testing.T) {
	priceList := &warmstorage.PriceList{
		Fees: warmstorage.PriceListFees{AddPiecesBaseFee: bi(7), AddPiecesPerPieceFee: bi(3)},
		Lockups: warmstorage.PriceListLockups{
			LifecycleReserveTarget: bi(100), ReplenishThreshold: bi(10), DefaultLockupPeriod: bi(DefaultLockupPeriod),
		},
	}
	svc := buildSvc(t, &mockWS{priceList: priceList}, &mockPay{
		account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: bi(5), LockupRate: new(big.Int)},
		approval: maxApproval(),
	})
	endEpoch := types.Epoch(0)
	refs := []MultiContextRef{
		{
			CurrentDataSetLeafCount:        new(big.Int),
			CurrentLifecycleReserveBalance: bi(19),
			PDPEndEpoch:                    &endEpoch,
		},
		{
			CurrentDataSetLeafCount:        new(big.Int),
			CurrentLifecycleReserveBalance: bi(20),
			PDPEndEpoch:                    &endEpoch,
		},
	}
	zeroBuffer := int64(0)

	got, err := svc.CalculateMultiContextCosts(
		context.Background(), common.Address{}, []uint64{1024}, refs,
		&UploadCostOptions{BufferEpochs: &zeroBuffer},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertBigIntEquals(t, "reserve replenishment", got.Lockup.ReserveReplenishment, bi(91))
	assertBigIntEquals(t, "total lockup", got.Lockup.Total, bi(91))
	assertBigIntEquals(t, "fees", got.Fees.Total, bi(20))
	assertBigIntEquals(t, "deposit", got.DepositNeeded, bi(96))
}

func TestCalculateMultiContextCosts_NilPriceListUsesZeroValue(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{nilPriceListResult: true},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		})

	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		[]uint64{1024},
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

func TestCalculateMultiContextCosts_BufferEpochOptions(t *testing.T) {
	account := &payments.AccountState{
		Funds:         new(big.Int),
		LockupCurrent: new(big.Int),
		LockupRate:    bi(100),
	}
	svc := buildSvc(t,
		&mockWS{priceList: defaultPriceList()},
		&mockPay{account: account, approval: maxApproval()})
	refs := []MultiContextRef{existingMultiContextRef(new(big.Int))}

	withoutBuffer, err := svc.CalculateMultiContextCosts(
		context.Background(), common.Address{}, []uint64{1024}, refs,
		&UploadCostOptions{BufferEpochs: new(int64(0))},
	)
	if err != nil {
		t.Fatalf("CalculateMultiContextCosts without buffer: %v", err)
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
			got, err := svc.CalculateMultiContextCosts(
				context.Background(), common.Address{}, []uint64{1024}, refs,
				&UploadCostOptions{BufferEpochs: tt.bufferEpochs},
			)
			if err != nil {
				t.Fatalf("CalculateMultiContextCosts: %v", err)
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

func TestCalculateMultiContextCosts_EmptyRefs(t *testing.T) {
	svc := buildSvc(t, &mockWS{}, &mockPay{})
	if _, err := svc.CalculateMultiContextCosts(
		context.Background(), common.Address{}, []uint64{1024}, nil, nil,
	); err == nil {
		t.Error("expected error for empty refs")
	}
}
