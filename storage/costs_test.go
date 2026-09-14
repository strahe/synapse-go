package storage_test

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestServiceCalculateMultiContextCosts_EnableCDNPropagatesToRefs(t *testing.T) {
	costSvc, err := costs.New(costs.Options{
		Chain: chain.Calibration,
		WarmStorage: fixedWarmStorageReader{priceList: &warmstorage.PriceList{
			Token: common.HexToAddress("0xbeef"),
			Rates: warmstorage.PriceListRates{
				StoragePerTiBPerMonth: big.NewInt(288000),
			},
			Fees: warmstorage.PriceListFees{
				AddPiecesBaseFee:     big.NewInt(10),
				AddPiecesPerPieceFee: big.NewInt(1),
			},
			Lockups: warmstorage.PriceListLockups{
				DefaultLockupPeriod:   big.NewInt(costs.DefaultLockupPeriod),
				CDNLockupAmount:       big.NewInt(17),
				CacheMissLockupAmount: big.NewInt(23),
			},
		}},
		Payments: fixedPaymentsReader{
			account: &payments.AccountState{
				Funds:         new(big.Int),
				LockupCurrent: new(big.Int),
				LockupRate:    new(big.Int),
			},
			approval: &payments.OperatorApproval{
				IsApproved:      true,
				RateAllowance:   new(big.Int).SetUint64(^uint64(0)),
				LockupAllowance: new(big.Int).SetUint64(^uint64(0)),
				RateUsage:       new(big.Int),
				LockupUsage:     new(big.Int),
				MaxLockupPeriod: big.NewInt(0),
			},
		},
		Caller: noopContractCaller{},
	})
	if err != nil {
		t.Fatalf("costs.New: %v", err)
	}

	payer := common.HexToAddress("0x1001")
	svc, err := storage.New(storage.Options{CostCalculator: costSvc, PayerAddress: payer})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	actual, err := svc.CalculateMultiContextCosts(
		context.Background(),
		1024,
		[]storage.ContextCostRef{{CurrentDataSetSizeBytes: new(big.Int)}},
		storage.MultiCostOptions{EnableCDN: true, PieceCount: big.NewInt(41)},
		common.Address{},
	)
	if err != nil {
		t.Fatalf("storage.CalculateMultiContextCosts: %v", err)
	}

	expected, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		payer,
		big.NewInt(1024),
		[]costs.MultiContextRef{{IsNewDataSet: true, WithCDN: true}},
		&costs.UploadCostOptions{PieceCount: big.NewInt(41)},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts: %v", err)
	}

	assertMultiContextCostsEqual(t, actual, expected)
}

func TestServiceCalculateMultiContextCosts_IgnoresCurrentSizeForNewDataSets(t *testing.T) {
	costSvc, err := costs.New(costs.Options{
		Chain: chain.Calibration,
		WarmStorage: fixedWarmStorageReader{priceList: &warmstorage.PriceList{
			Token: common.HexToAddress("0xbeef"),
			Rates: warmstorage.PriceListRates{
				StoragePerTiBPerMonth: big.NewInt(1000),
			},
			Lockups: warmstorage.PriceListLockups{
				DefaultLockupPeriod: big.NewInt(costs.DefaultLockupPeriod),
			},
		}},
		Payments: fixedPaymentsReader{
			account: &payments.AccountState{
				Funds:         new(big.Int),
				LockupCurrent: new(big.Int),
				LockupRate:    new(big.Int),
			},
			approval: &payments.OperatorApproval{
				IsApproved:      true,
				RateAllowance:   new(big.Int).SetUint64(^uint64(0)),
				LockupAllowance: new(big.Int).SetUint64(^uint64(0)),
				RateUsage:       new(big.Int),
				LockupUsage:     new(big.Int),
				MaxLockupPeriod: big.NewInt(0),
			},
		},
		Caller: noopContractCaller{},
	})
	if err != nil {
		t.Fatalf("costs.New: %v", err)
	}

	payer := common.HexToAddress("0x1001")
	svc, err := storage.New(storage.Options{CostCalculator: costSvc, PayerAddress: payer})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	actual, err := svc.CalculateMultiContextCosts(
		context.Background(),
		uint64(chain.TiB),
		[]storage.ContextCostRef{{
			CurrentDataSetSizeBytes: big.NewInt(chain.TiB),
		}},
		storage.MultiCostOptions{},
		common.Address{},
	)
	if err != nil {
		t.Fatalf("storage.CalculateMultiContextCosts: %v", err)
	}

	expected, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		payer,
		big.NewInt(chain.TiB),
		[]costs.MultiContextRef{{IsNewDataSet: true, WithCDN: false}},
		&costs.UploadCostOptions{},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts: %v", err)
	}

	assertMultiContextCostsEqual(t, actual, expected)
}

func TestServiceCalculateMultiContextCosts_PreservesExplicitZeroBuffer(t *testing.T) {
	costSvc, err := costs.New(costs.Options{
		Chain: chain.Calibration,
		WarmStorage: fixedWarmStorageReader{priceList: &warmstorage.PriceList{
			Rates: warmstorage.PriceListRates{StoragePerTiBPerMonth: big.NewInt(288000)},
			Lockups: warmstorage.PriceListLockups{
				DefaultLockupPeriod: big.NewInt(costs.DefaultLockupPeriod),
			},
		}},
		Payments: fixedPaymentsReader{
			account: &payments.AccountState{
				Funds:         new(big.Int),
				LockupCurrent: new(big.Int),
				LockupRate:    big.NewInt(100),
			},
			approval: &payments.OperatorApproval{},
		},
		Caller: noopContractCaller{},
	})
	if err != nil {
		t.Fatalf("costs.New: %v", err)
	}

	zeroBuffer := int64(0)
	dataSetID := types.NewBigInt(1)
	payer := common.HexToAddress("0x1001")
	svc, err := storage.New(storage.Options{CostCalculator: costSvc, PayerAddress: payer})
	if err != nil {
		t.Fatalf("storage.New: %v", err)
	}
	actual, err := svc.CalculateMultiContextCosts(
		context.Background(),
		uint64(chain.TiB),
		[]storage.ContextCostRef{{DataSetID: &dataSetID, CurrentDataSetSizeBytes: new(big.Int)}},
		storage.MultiCostOptions{BufferEpochs: &zeroBuffer},
		common.Address{},
	)
	if err != nil {
		t.Fatalf("storage.CalculateMultiContextCosts: %v", err)
	}

	expected, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		payer,
		big.NewInt(chain.TiB),
		[]costs.MultiContextRef{{}},
		&costs.UploadCostOptions{BufferEpochs: &zeroBuffer},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts with zero buffer: %v", err)
	}
	withDefault, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		payer,
		big.NewInt(chain.TiB),
		[]costs.MultiContextRef{{}},
		&costs.UploadCostOptions{},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts with default buffer: %v", err)
	}

	assertMultiContextCostsEqual(t, actual, expected)
	if actual.DepositNeeded.Cmp(withDefault.DepositNeeded) >= 0 {
		t.Fatalf("explicit-zero deposit=%s want less than default %s", actual.DepositNeeded, withDefault.DepositNeeded)
	}
}

func assertMultiContextCostsEqual(t *testing.T, actual, expected *costs.MultiContextCosts) {
	t.Helper()
	if actual == nil || expected == nil {
		t.Fatalf("costs actual=%+v expected=%+v", actual, expected)
	}
	fields := []struct {
		name         string
		actual, want *big.Int
	}{
		{"RatePerEpoch", actual.RatePerEpoch, expected.RatePerEpoch},
		{"RatePerMonth", actual.RatePerMonth, expected.RatePerMonth},
		{"Fees.CreateDataSetFee", actual.Fees.CreateDataSetFee, expected.Fees.CreateDataSetFee},
		{"Fees.AddPiecesFee", actual.Fees.AddPiecesFee, expected.Fees.AddPiecesFee},
		{"Fees.Total", actual.Fees.Total, expected.Fees.Total},
		{"Lockup.RateDeltaPerEpoch", actual.Lockup.RateDeltaPerEpoch, expected.Lockup.RateDeltaPerEpoch},
		{"Lockup.StreamingLockup", actual.Lockup.StreamingLockup, expected.Lockup.StreamingLockup},
		{"Lockup.LifecycleLockup", actual.Lockup.LifecycleLockup, expected.Lockup.LifecycleLockup},
		{"Lockup.CDNLockup", actual.Lockup.CDNLockup, expected.Lockup.CDNLockup},
		{"Lockup.CacheMissLockup", actual.Lockup.CacheMissLockup, expected.Lockup.CacheMissLockup},
		{"Lockup.Total", actual.Lockup.Total, expected.Lockup.Total},
		{"DepositNeeded", actual.DepositNeeded, expected.DepositNeeded},
		{"RequiredLockupPeriod", actual.RequiredLockupPeriod, expected.RequiredLockupPeriod},
	}
	for _, field := range fields {
		if field.actual == nil || field.want == nil {
			if field.actual != field.want {
				t.Fatalf("%s=%v want %v", field.name, field.actual, field.want)
			}
			continue
		}
		if field.actual.Cmp(field.want) != 0 {
			t.Fatalf("%s=%s want %s", field.name, field.actual, field.want)
		}
	}
	if actual.NeedsFWSSMaxApproval != expected.NeedsFWSSMaxApproval || actual.Ready != expected.Ready {
		t.Fatalf("NeedsFWSSMaxApproval=%v Ready=%v want %v %v", actual.NeedsFWSSMaxApproval, actual.Ready, expected.NeedsFWSSMaxApproval, expected.Ready)
	}
}

type noopContractCaller struct{}

func (noopContractCaller) BlockNumber(context.Context) (uint64, error) {
	return 0, nil
}

type fixedWarmStorageReader struct {
	priceList *warmstorage.PriceList
}

func (r fixedWarmStorageReader) GetPriceList(context.Context) (*warmstorage.PriceList, error) {
	return r.priceList, nil
}

type fixedPaymentsReader struct {
	account  *payments.AccountState
	approval *payments.OperatorApproval
}

func (r fixedPaymentsReader) AccountInfo(context.Context, common.Address, common.Address) (*payments.AccountState, error) {
	return r.account, nil
}

func (r fixedPaymentsReader) ServiceApproval(context.Context, common.Address, common.Address, common.Address) (*payments.OperatorApproval, error) {
	return r.approval, nil
}
