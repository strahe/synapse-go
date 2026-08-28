package adapters

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

func TestCostCalculator_CalculateMultiContextCosts_EnableCDNPropagatesToRefs(t *testing.T) {
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

	adapter := &costCalculator{c: costSvc}
	actual, err := adapter.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(1024),
		[]storage.ContextCostRef{{CurrentDataSetSizeBytes: new(big.Int)}},
		storage.MultiCostOptions{EnableCDN: true, PieceCount: big.NewInt(41)},
	)
	if err != nil {
		t.Fatalf("adapter.CalculateMultiContextCosts: %v", err)
	}

	expected, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(1024),
		[]costs.MultiContextRef{{IsNewDataSet: true, WithCDN: true}},
		&costs.UploadCostOptions{PieceCount: big.NewInt(41)},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts: %v", err)
	}

	if actual.DepositNeeded.Cmp(expected.DepositNeeded) != 0 {
		t.Fatalf("DepositNeeded=%s want %s", actual.DepositNeeded, expected.DepositNeeded)
	}
	if actual.Ready != expected.Ready {
		t.Fatalf("Ready=%v want %v", actual.Ready, expected.Ready)
	}
}

func TestCostCalculator_CalculateMultiContextCosts_IgnoresCurrentSizeForNewDataSets(t *testing.T) {
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

	adapter := &costCalculator{c: costSvc}
	actual, err := adapter.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(chain.TiB),
		[]storage.ContextCostRef{{
			CurrentDataSetSizeBytes: big.NewInt(chain.TiB),
		}},
		storage.MultiCostOptions{},
	)
	if err != nil {
		t.Fatalf("adapter.CalculateMultiContextCosts: %v", err)
	}

	expected, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(chain.TiB),
		[]costs.MultiContextRef{{IsNewDataSet: true, WithCDN: false}},
		&costs.UploadCostOptions{},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts: %v", err)
	}

	if actual.DepositNeeded.Cmp(expected.DepositNeeded) != 0 {
		t.Fatalf("DepositNeeded=%s want %s", actual.DepositNeeded, expected.DepositNeeded)
	}
	if actual.RatePerEpoch.Cmp(expected.RatePerEpoch) != 0 {
		t.Fatalf("RatePerEpoch=%s want %s", actual.RatePerEpoch, expected.RatePerEpoch)
	}
	if actual.RatePerMonth.Cmp(expected.RatePerMonth) != 0 {
		t.Fatalf("RatePerMonth=%s want %s", actual.RatePerMonth, expected.RatePerMonth)
	}
}

func TestCostCalculator_CalculateMultiContextCosts_PreservesExplicitZeroBuffer(t *testing.T) {
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
	adapter := &costCalculator{c: costSvc}
	actual, err := adapter.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(chain.TiB),
		[]storage.ContextCostRef{{DataSetID: &dataSetID, CurrentDataSetSizeBytes: new(big.Int)}},
		storage.MultiCostOptions{BufferEpochs: &zeroBuffer},
	)
	if err != nil {
		t.Fatalf("adapter.CalculateMultiContextCosts: %v", err)
	}

	expected, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(chain.TiB),
		[]costs.MultiContextRef{{}},
		&costs.UploadCostOptions{BufferEpochs: &zeroBuffer},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts with zero buffer: %v", err)
	}
	withDefault, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(chain.TiB),
		[]costs.MultiContextRef{{}},
		&costs.UploadCostOptions{},
	)
	if err != nil {
		t.Fatalf("costSvc.CalculateMultiContextCosts with default buffer: %v", err)
	}

	if actual.DepositNeeded.Cmp(expected.DepositNeeded) != 0 {
		t.Fatalf("DepositNeeded=%s want explicit-zero result %s", actual.DepositNeeded, expected.DepositNeeded)
	}
	if actual.DepositNeeded.Cmp(withDefault.DepositNeeded) >= 0 {
		t.Fatalf("explicit-zero deposit=%s want less than default %s", actual.DepositNeeded, withDefault.DepositNeeded)
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
