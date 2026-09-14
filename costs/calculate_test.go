package costs

import (
	"errors"
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

func bi(v int64) *big.Int { return big.NewInt(v) }

// usdfc returns n whole USDFC as attoUSDFC.
func usdfc(n int64) *big.Int {
	return new(big.Int).Mul(bi(n), bi(1e18))
}

// usdfcFrac returns n/10 USDFC.
func usdfcFrac(tenths int64) *big.Int {
	return new(big.Int).Mul(bi(tenths), big.NewInt(1e17))
}

func defaultPriceList() *warmstorage.PriceList {
	return &warmstorage.PriceList{
		Rates: warmstorage.PriceListRates{
			StoragePerTiBPerMonth: usdfcFrac(25),
			DatasetFeePerMonth:    usdfcFrac(1),
		},
		Fees: warmstorage.PriceListFees{
			CreateDataSetFee:     usdfcFrac(2),
			AddPiecesBaseFee:     usdfcFrac(3),
			AddPiecesPerPieceFee: usdfcFrac(1),
		},
		Lockups: warmstorage.PriceListLockups{
			LifecycleReserveTarget: usdfcFrac(4),
			DefaultLockupPeriod:    bi(DefaultLockupPeriod),
			CDNLockupAmount:        usdfcFrac(5),
			CacheMissLockupAmount:  usdfcFrac(6),
		},
	}
}

func TestCalculateEffectiveRate_AddsDatasetFeeForNonEmptyDataSets(t *testing.T) {
	priceList := defaultPriceList()
	rate := CalculateEffectiveRate(
		bi(chain.TiB),
		priceList.Rates.StoragePerTiBPerMonth,
		priceList.Rates.DatasetFeePerMonth,
		chain.EpochsPerMonth,
	)

	wantMonth := new(big.Int).Add(priceList.Rates.StoragePerTiBPerMonth, priceList.Rates.DatasetFeePerMonth)
	if rate.RatePerMonth.Cmp(wantMonth) != 0 {
		t.Fatalf("RatePerMonth=%s want %s", rate.RatePerMonth, wantMonth)
	}
	wantEpoch := new(big.Int).Div(priceList.Rates.StoragePerTiBPerMonth, bi(chain.EpochsPerMonth))
	wantEpoch.Add(wantEpoch, new(big.Int).Div(priceList.Rates.DatasetFeePerMonth, bi(chain.EpochsPerMonth)))
	if rate.RatePerEpoch.Cmp(wantEpoch) != 0 {
		t.Fatalf("RatePerEpoch=%s want %s", rate.RatePerEpoch, wantEpoch)
	}
}

func TestCalculateEffectiveRate_EmptyDataSetHasNoRecurringRate(t *testing.T) {
	priceList := defaultPriceList()
	rate := CalculateEffectiveRate(
		bi(0),
		priceList.Rates.StoragePerTiBPerMonth,
		priceList.Rates.DatasetFeePerMonth,
		chain.EpochsPerMonth,
	)
	if rate.RatePerEpoch.Sign() != 0 || rate.RatePerMonth.Sign() != 0 {
		t.Fatalf("rate=%+v want zero recurring rate", rate)
	}
}

func TestCalculateEffectiveRate_NilInputsUseZeroValues(t *testing.T) {
	rate := CalculateEffectiveRate(nil, nil, nil, 0)
	if rate.RatePerEpoch.Sign() != 0 || rate.RatePerMonth.Sign() != 0 {
		t.Fatalf("rate=%+v want zero recurring rate", rate)
	}
}

func TestCalculateUploadFees_UsesCreateFeeAndAddPiecesBatchBoundary(t *testing.T) {
	priceList := defaultPriceList()
	within := CalculateUploadFees(priceList, true, bi(40))
	spill := CalculateUploadFees(priceList, true, bi(41))

	wantWithin := new(big.Int).Set(priceList.Fees.CreateDataSetFee)
	wantWithin.Add(wantWithin, priceList.Fees.AddPiecesBaseFee)
	wantWithin.Add(wantWithin, new(big.Int).Mul(priceList.Fees.AddPiecesPerPieceFee, bi(40)))
	if within.Total.Cmp(wantWithin) != 0 {
		t.Fatalf("within.Total=%s want %s", within.Total, wantWithin)
	}

	wantSpill := new(big.Int).Set(priceList.Fees.CreateDataSetFee)
	wantSpill.Add(wantSpill, new(big.Int).Mul(priceList.Fees.AddPiecesBaseFee, bi(2)))
	wantSpill.Add(wantSpill, new(big.Int).Mul(priceList.Fees.AddPiecesPerPieceFee, bi(41)))
	if spill.Total.Cmp(wantSpill) != 0 {
		t.Fatalf("spill.Total=%s want %s", spill.Total, wantSpill)
	}
}

func TestCalculateAdditionalLockupRequired_NilInputsUseZeroValues(t *testing.T) {
	lockup, err := CalculateAdditionalLockupRequired(nil, nil, nil, nil, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if lockup.RateDeltaPerEpoch.Sign() != 0 ||
		lockup.StreamingLockup.Sign() != 0 ||
		lockup.LifecycleLockup.Sign() != 0 ||
		lockup.CDNLockup.Sign() != 0 ||
		lockup.CacheMissLockup.Sign() != 0 ||
		lockup.Total.Sign() != 0 {
		t.Fatalf("lockup=%+v want zero values", lockup)
	}
}

func TestCalculateAdditionalLockupRequired_NewCDNDataSetBreakdown(t *testing.T) {
	priceList := defaultPriceList()
	lockup, err := CalculateAdditionalLockupRequired(
		[]uint64{chain.TiB},
		nil,
		priceList,
		priceList.Lockups.DefaultLockupPeriod,
		true,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}

	wantStreaming := new(big.Int).Mul(lockup.RateDeltaPerEpoch, priceList.Lockups.DefaultLockupPeriod)
	if lockup.StreamingLockup.Cmp(wantStreaming) != 0 {
		t.Fatalf("StreamingLockup=%s want %s", lockup.StreamingLockup, wantStreaming)
	}
	if lockup.LifecycleLockup.Cmp(priceList.Lockups.LifecycleReserveTarget) != 0 {
		t.Fatalf("LifecycleLockup=%s want %s", lockup.LifecycleLockup, priceList.Lockups.LifecycleReserveTarget)
	}
	if lockup.CDNLockup.Cmp(priceList.Lockups.CDNLockupAmount) != 0 {
		t.Fatalf("CDNLockup=%s want %s", lockup.CDNLockup, priceList.Lockups.CDNLockupAmount)
	}
	if lockup.CacheMissLockup.Cmp(priceList.Lockups.CacheMissLockupAmount) != 0 {
		t.Fatalf("CacheMissLockup=%s want %s", lockup.CacheMissLockup, priceList.Lockups.CacheMissLockupAmount)
	}
	wantTotal := new(big.Int).Add(lockup.StreamingLockup, lockup.LifecycleLockup)
	wantTotal.Add(wantTotal, lockup.CDNLockup)
	wantTotal.Add(wantTotal, lockup.CacheMissLockup)
	if lockup.Total.Cmp(wantTotal) != 0 {
		t.Fatalf("Total=%s want %s", lockup.Total, wantTotal)
	}
}

func TestCalculateAdditionalLockupRequired_ExistingDataSetUsesRateDeltaOnly(t *testing.T) {
	priceList := defaultPriceList()
	lockup, err := CalculateAdditionalLockupRequired(
		[]uint64{chain.TiB},
		bi(chain.TiB),
		priceList,
		priceList.Lockups.DefaultLockupPeriod,
		false,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	if lockup.LifecycleLockup.Sign() != 0 || lockup.CDNLockup.Sign() != 0 || lockup.CacheMissLockup.Sign() != 0 {
		t.Fatalf("existing lockup=%+v want only streaming lockup", lockup)
	}
	if lockup.Total.Cmp(lockup.StreamingLockup) != 0 {
		t.Fatalf("Total=%s want StreamingLockup=%s", lockup.Total, lockup.StreamingLockup)
	}
}

func TestCalculateAdditionalLockupRequired_RejectsNegativeExistingLeafCount(t *testing.T) {
	lockup, err := CalculateAdditionalLockupRequired(
		[]uint64{128},
		bi(-1),
		defaultPriceList(),
		nil,
		false,
		false,
	)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error=%v want ErrInvalidArgument", err)
	}
	if lockup != (AdditionalLockup{}) {
		t.Fatalf("lockup=%+v want zero value", lockup)
	}

	if _, err := CalculateAdditionalLockupRequired(
		[]uint64{128},
		bi(-1),
		defaultPriceList(),
		nil,
		true,
		false,
	); err != nil {
		t.Fatalf("new dataset should ignore current leaf count: %v", err)
	}
}

func TestCalculateDepositNeeded_IncludesFees(t *testing.T) {
	deposit := CalculateDepositNeeded(DepositCalculation{
		AdditionalLockup:  bi(10),
		Fees:              bi(7),
		RateDelta:         bi(0),
		CurrentLockupRate: bi(0),
		AvailableFunds:    bi(0),
		IsNewDataSet:      true,
	})
	if deposit.Cmp(bi(17)) != 0 {
		t.Fatalf("deposit=%s want 17", deposit)
	}
}

func TestCalculateDepositNeeded_DoesNotAliasOrModifyInputs(t *testing.T) {
	inputs := []*big.Int{bi(10), bi(7), bi(2), bi(3), bi(4), bi(5), bi(6)}
	want := make([]*big.Int, len(inputs))
	for i := range inputs {
		want[i] = new(big.Int).Set(inputs[i])
	}

	deposit := CalculateDepositNeeded(DepositCalculation{
		AdditionalLockup:  inputs[0],
		Fees:              inputs[1],
		RateDelta:         inputs[2],
		CurrentLockupRate: inputs[3],
		Debt:              inputs[4],
		AvailableFunds:    inputs[5],
		RunwayInEpochs:    inputs[6],
		ExtraRunwayEpochs: 8,
		BufferEpochs:      9,
	})
	deposit.SetInt64(0)

	for i := range inputs {
		if inputs[i].Cmp(want[i]) != 0 {
			t.Fatalf("input[%d]=%s want unchanged %s", i, inputs[i], want[i])
		}
	}
}

func TestCalculateDepositNeeded_BufferUsesRunwayWindow(t *testing.T) {
	deposit := CalculateDepositNeeded(DepositCalculation{
		AdditionalLockup:  bi(0),
		Fees:              bi(0),
		RateDelta:         bi(1),
		CurrentLockupRate: bi(4),
		AvailableFunds:    bi(10),
		RunwayInEpochs:    bi(3),
		BufferEpochs:      5,
	})
	if deposit.Cmp(bi(15)) != 0 {
		t.Fatalf("deposit=%s want 15", deposit)
	}
}

func TestIsFWSSMaxApproved_UsesRequiredLockupPeriod(t *testing.T) {
	required := bi(DefaultLockupPeriod + 10)
	if isFWSSMaxApproved(true, maxUint256, maxUint256, bi(DefaultLockupPeriod), required) {
		t.Fatal("approval should fail when max lockup period is below required period")
	}
	if !isFWSSMaxApproved(true, maxUint256, maxUint256, required, required) {
		t.Fatal("approval should pass at required period")
	}
}
