package costs

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// The synthetic price makes each billable byte contribute one atto/epoch,
// with three additional atto/epoch for a non-empty dataset.
func leafAccountingPriceList() *warmstorage.PriceList {
	return &warmstorage.PriceList{
		Rates: warmstorage.PriceListRates{
			StoragePerTiBPerMonth: bi(chain.TiB * chain.EpochsPerMonth),
			DatasetFeePerMonth:    bi(3 * chain.EpochsPerMonth),
		},
		Fees: warmstorage.PriceListFees{
			CreateDataSetFee: bi(10), AddPiecesBaseFee: bi(7), AddPiecesPerPieceFee: bi(2),
		},
		Lockups: warmstorage.PriceListLockups{
			DefaultLockupPeriod: bi(10), LifecycleReserveTarget: bi(100),
			ReplenishThreshold: bi(10),
			CDNLockupAmount:    bi(40), CacheMissLockupAmount: bi(30),
		},
	}
}

func TestCostServices_LeafAccounting(t *testing.T) {
	svc := buildSvc(t, &mockWS{priceList: leafAccountingPriceList()}, &mockPay{
		account: &payments.AccountState{}, approval: maxApproval(),
	})
	for _, tc := range []struct {
		name                   string
		sizes                  []uint64
		current                *big.Int
		isNew                  bool
		rate, delta, lifecycle int64
	}{
		{"different rounding A", []uint64{128, 190}, nil, true, 352, 352, 100},
		{"different rounding B", []uint64{159, 159}, nil, true, 384, 384, 100},
		{"existing nonempty", []uint64{128}, bi(5), false, 320, 159, 0},
		{"existing empty", []uint64{128}, bi(0), false, 161, 161, 0},
		{"new ignores positive state", []uint64{128}, bi(5), true, 161, 161, 100},
		{"new ignores negative state", []uint64{128}, bi(-1), true, 161, 161, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			originalSizes := slices.Clone(tc.sizes)
			var originalLeaves *big.Int
			if tc.current != nil {
				originalLeaves = new(big.Int).Set(tc.current)
			}
			opts := &UploadCostOptions{IsNewDataSet: true, CurrentDataSetLeafCount: tc.current}
			ref := MultiContextRef{IsNewDataSet: true, CurrentDataSetLeafCount: tc.current}
			if tc.isNew {
				terminatedEpoch := types.Epoch(9)
				opts.CurrentLifecycleReserveBalance = bi(-1)
				opts.PendingOneTimePayments = bi(-1)
				opts.PDPEndEpoch = &terminatedEpoch
				ref.CurrentLifecycleReserveBalance = bi(-1)
				ref.PendingOneTimePayments = bi(-1)
				ref.PDPEndEpoch = &terminatedEpoch
			} else {
				opts = existingUploadCostOptions(tc.current)
				ref = existingMultiContextRef(tc.current)
			}
			single, err := svc.GetUploadCosts(context.Background(), common.Address{}, tc.sizes, opts)
			if err != nil {
				t.Fatal(err)
			}
			multi, err := svc.CalculateMultiContextCosts(context.Background(), common.Address{}, tc.sizes,
				[]MultiContextRef{ref}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, got := range []struct {
				rate   *big.Int
				lockup AdditionalLockup
			}{
				{single.Rate.RatePerEpoch, single.Lockup}, {multi.RatePerEpoch, multi.Lockup},
			} {
				if got.rate.Cmp(bi(tc.rate)) != 0 || got.lockup.RateDeltaPerEpoch.Cmp(bi(tc.delta)) != 0 ||
					got.lockup.StreamingLockup.Cmp(bi(tc.delta*10)) != 0 || got.lockup.LifecycleLockup.Cmp(bi(tc.lifecycle)) != 0 {
					t.Fatalf("rate=%s lockup=%+v want rate=%d delta=%d lifecycle=%d", got.rate, got.lockup, tc.rate, tc.delta, tc.lifecycle)
				}
			}
			if !slices.Equal(tc.sizes, originalSizes) || (tc.current != nil && tc.current.Cmp(originalLeaves) != 0) {
				t.Fatal("cost calculation modified its inputs")
			}
		})
	}
}

func TestCalculateMultiContextCosts_MixedLeafAccountingAndIgnoredOptions(t *testing.T) {
	svc := buildSvc(t, &mockWS{priceList: leafAccountingPriceList()}, &mockPay{
		account: &payments.AccountState{}, approval: maxApproval(),
	})
	got, err := svc.CalculateMultiContextCosts(context.Background(), common.Address{}, []uint64{128},
		[]MultiContextRef{
			{IsNewDataSet: true, CurrentDataSetLeafCount: bi(-9), WithCDN: true},
			func() MultiContextRef {
				ref := existingMultiContextRef(bi(5))
				ref.WithCDN = true
				return ref
			}(),
			existingMultiContextRef(bi(0)),
		}, &UploadCostOptions{IsNewDataSet: true, CurrentDataSetLeafCount: bi(-9), EnableCDN: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.RatePerEpoch.Cmp(bi(642)) != 0 || got.RatePerMonth.Cmp(bi(642*chain.EpochsPerMonth)) != 0 ||
		got.Lockup.RateDeltaPerEpoch.Cmp(bi(481)) != 0 || got.Lockup.StreamingLockup.Cmp(bi(4810)) != 0 ||
		got.Lockup.LifecycleLockup.Cmp(bi(100)) != 0 || got.Lockup.CDNLockup.Cmp(bi(40)) != 0 ||
		got.Lockup.CacheMissLockup.Cmp(bi(30)) != 0 {
		t.Fatalf("incorrect mixed accounting: %+v lockup=%+v", got, got.Lockup)
	}
}

func TestGetUploadCosts_PricesEveryPieceConservatively(t *testing.T) {
	svc := buildSvc(t, &mockWS{priceList: leafAccountingPriceList()}, &mockPay{
		account: &payments.AccountState{}, approval: maxApproval(),
	})
	for _, count := range []int{40, 41} {
		got, err := svc.GetUploadCosts(context.Background(), common.Address{}, slices.Repeat([]uint64{128}, count), &UploadCostOptions{IsNewDataSet: true})
		if err != nil {
			t.Fatal(err)
		}
		want := int64(10 + (7+2)*count)
		if got.Fees.Total.Cmp(bi(want)) != 0 {
			t.Fatalf("count=%d: fees=%s want %d", count, got.Fees.Total, want)
		}
	}
}

func TestCalculateAdditionalLockupRequired_RejectsInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sizes   []uint64
		current *big.Int
	}{
		{name: "nil sizes", current: bi(5)},
		{name: "empty sizes", sizes: []uint64{}, current: bi(5)},
		{name: "zero size", sizes: []uint64{0}, current: bi(5)},
		{name: "below minimum", sizes: []uint64{chain.MinUploadSize - 1}, current: bi(5)},
		{name: "above maximum", sizes: []uint64{chain.MaxUploadSize + 1}, current: bi(5)},
		{name: "missing existing leaf count", sizes: []uint64{chain.MinUploadSize}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CalculateAdditionalLockupRequired(tc.sizes, tc.current, leafAccountingPriceList(), nil, false, true)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v want ErrInvalidArgument", err)
			}
			if got != (AdditionalLockup{}) {
				t.Fatalf("lockup=%+v want zero value", got)
			}
		})
	}
}

func TestLeafConversionHelpersMatchUploadCosts(t *testing.T) {
	sizes := []uint64{127, 128, 159, 190, chain.MaxUploadSize}
	leaves, err := PieceSizesToLeafCount(sizes)
	if err != nil {
		t.Fatal(err)
	}
	if want := pieceSizesToLeafCount(sizes); leaves.Cmp(want) != 0 {
		t.Fatalf("leaves=%s want %s", leaves, want)
	}
	current := bi(5)
	billable, err := LeafCountToBillableBytes(new(big.Int).Add(current, leaves))
	if err != nil {
		t.Fatal(err)
	}
	priceList := leafAccountingPriceList()
	rate := CalculateEffectiveRate(billable, priceList.Rates.StoragePerTiBPerMonth, priceList.Rates.DatasetFeePerMonth, 0)
	currentBillable, err := LeafCountToBillableBytes(current)
	if err != nil {
		t.Fatal(err)
	}
	currentRate := CalculateEffectiveRate(currentBillable, priceList.Rates.StoragePerTiBPerMonth, priceList.Rates.DatasetFeePerMonth, 0)
	lockup, err := CalculateAdditionalLockupRequired(sizes, current, priceList, nil, false, false)
	if err != nil {
		t.Fatal(err)
	}
	wantDelta := new(big.Int).Sub(rate.RatePerEpoch, currentRate.RatePerEpoch)
	if lockup.RateDeltaPerEpoch.Cmp(wantDelta) != 0 {
		t.Fatalf("rate delta=%s want %s from exported helpers", lockup.RateDeltaPerEpoch, wantDelta)
	}
}

func TestLeafConversionHelpersValidateInputs(t *testing.T) {
	for _, sizes := range [][]uint64{nil, {}, {chain.MinUploadSize - 1}, {chain.MaxUploadSize + 1}} {
		if got, err := PieceSizesToLeafCount(sizes); !errors.Is(err, ErrInvalidArgument) || got != nil {
			t.Fatalf("PieceSizesToLeafCount(%v)=(%v, %v), want ErrInvalidArgument", sizes, got, err)
		}
	}
	if got, err := LeafCountToBillableBytes(bi(-1)); !errors.Is(err, ErrInvalidArgument) || got != nil {
		t.Fatalf("LeafCountToBillableBytes(-1)=(%v, %v), want ErrInvalidArgument", got, err)
	}
	got, err := LeafCountToBillableBytes(nil)
	if err != nil || got.Sign() != 0 {
		t.Fatalf("LeafCountToBillableBytes(nil)=(%v, %v), want zero", got, err)
	}
}

type forbiddenCostBackend struct{ t *testing.T }

func (b forbiddenCostBackend) GetPriceList(context.Context) (*warmstorage.PriceList, error) {
	b.t.Error("invalid arguments reached price reader")
	return nil, errors.New("unexpected read")
}

func (b forbiddenCostBackend) AccountInfo(context.Context, common.Address, common.Address) (*payments.AccountState, error) {
	b.t.Error("invalid arguments reached account reader")
	return nil, errors.New("unexpected read")
}

func (b forbiddenCostBackend) ServiceApproval(context.Context, common.Address, common.Address, common.Address) (*payments.OperatorApproval, error) {
	b.t.Error("invalid arguments reached approval reader")
	return nil, errors.New("unexpected read")
}

func (b forbiddenCostBackend) BlockNumber(context.Context) (uint64, error) {
	b.t.Error("invalid arguments reached epoch reader")
	return 0, errors.New("unexpected read")
}

func TestCostServices_RejectInvalidArgumentsBeforeReads(t *testing.T) {
	for _, tc := range []struct {
		name  string
		sizes []uint64
		opts  *UploadCostOptions
	}{
		{"nil sizes", nil, &UploadCostOptions{IsNewDataSet: true}},
		{"empty sizes", []uint64{}, &UploadCostOptions{IsNewDataSet: true}},
		{"zero element", []uint64{128, 0}, &UploadCostOptions{IsNewDataSet: true}},
		{"below minimum", []uint64{chain.MinUploadSize - 1}, &UploadCostOptions{IsNewDataSet: true}},
		{"above maximum", []uint64{chain.MaxUploadSize + 1}, &UploadCostOptions{IsNewDataSet: true}},
		{"nil options", []uint64{128}, nil},
		{"empty options", []uint64{128}, &UploadCostOptions{}},
		{"negative leaves", []uint64{128}, &UploadCostOptions{CurrentDataSetLeafCount: bi(-1)}},
		{"missing reserve", []uint64{128}, &UploadCostOptions{CurrentDataSetLeafCount: new(big.Int), PDPEndEpoch: new(types.Epoch)}},
		{"negative reserve", []uint64{128}, &UploadCostOptions{CurrentDataSetLeafCount: new(big.Int), CurrentLifecycleReserveBalance: bi(-1), PDPEndEpoch: new(types.Epoch)}},
		{"negative pending", []uint64{128}, &UploadCostOptions{CurrentDataSetLeafCount: new(big.Int), CurrentLifecycleReserveBalance: new(big.Int), PendingOneTimePayments: bi(-1), PDPEndEpoch: new(types.Epoch)}},
		{"missing end epoch", []uint64{128}, &UploadCostOptions{CurrentDataSetLeafCount: new(big.Int), CurrentLifecycleReserveBalance: new(big.Int)}},
		{"negative runway", []uint64{128}, &UploadCostOptions{IsNewDataSet: true, ExtraRunwayEpochs: -1}},
		{"negative buffer", []uint64{128}, &UploadCostOptions{IsNewDataSet: true, BufferEpochs: new(int64(-1))}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := forbiddenCostBackend{t}
			svc := buildSvc(t, backend, backend)
			svc.caller = backend
			if got, err := svc.GetUploadCosts(context.Background(), common.Address{}, tc.sizes, tc.opts); got != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("GetUploadCosts = (%v, %v), want ErrInvalidArgument", got, err)
			}
			ref := MultiContextRef{}
			if tc.opts != nil {
				ref.IsNewDataSet = tc.opts.IsNewDataSet
				ref.CurrentDataSetLeafCount = tc.opts.CurrentDataSetLeafCount
				ref.CurrentLifecycleReserveBalance = tc.opts.CurrentLifecycleReserveBalance
				ref.PendingOneTimePayments = tc.opts.PendingOneTimePayments
				ref.PDPEndEpoch = tc.opts.PDPEndEpoch
			}
			if got, err := svc.CalculateMultiContextCosts(context.Background(), common.Address{}, tc.sizes, []MultiContextRef{ref}, tc.opts); got != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("CalculateMultiContextCosts = (%v, %v), want ErrInvalidArgument", got, err)
			}
		})
	}
	backend := forbiddenCostBackend{t}
	svc := buildSvc(t, backend, backend)
	if _, err := svc.CalculateMultiContextCosts(context.Background(), common.Address{}, []uint64{128}, nil, nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty refs: %v", err)
	}
}

func TestCostServices_AcceptUploadSizeBounds(t *testing.T) {
	svc := buildSvc(t, &mockWS{priceList: leafAccountingPriceList()}, &mockPay{
		account: &payments.AccountState{}, approval: maxApproval(),
	})
	for _, size := range []uint64{chain.MinUploadSize, chain.MaxUploadSize} {
		if _, err := svc.GetUploadCosts(
			context.Background(), common.Address{}, []uint64{size}, &UploadCostOptions{IsNewDataSet: true},
		); err != nil {
			t.Fatalf("GetUploadCosts(%d): %v", size, err)
		}
		if _, err := svc.CalculateMultiContextCosts(
			context.Background(), common.Address{}, []uint64{size}, []MultiContextRef{{IsNewDataSet: true}}, nil,
		); err != nil {
			t.Fatalf("CalculateMultiContextCosts(%d): %v", size, err)
		}
	}
}
