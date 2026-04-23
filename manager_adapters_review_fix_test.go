package synapse

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/costs"
	fwssbind "github.com/strahe/synapse-go/internal/contracts/fwss"
	fwssviewbind "github.com/strahe/synapse-go/internal/contracts/fwssview"
	pdpverifierbind "github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestStorageInfoAdapter_GetStorageInfo_UsesValidApprovedProviderPagination(t *testing.T) {
	ws, mc := newStorageInfoTestWarmStorage(t)
	mc.setFWSSReply(t, "getServicePrice", fwssbind.FilecoinWarmStorageServiceServicePricing{
		PricePerTiBPerMonthNoCDN:   big.NewInt(1000),
		PricePerTiBCdnEgress:       big.NewInt(20),
		PricePerTiBCacheMissEgress: big.NewInt(30),
		TokenAddress:               common.HexToAddress("0xabc"),
		EpochsPerMonth:             big.NewInt(2880),
		MinimumPricePerMonth:       big.NewInt(0),
	})
	mc.setViewReply(t, "getApprovedProviders", []*big.Int{})

	got, err := (&storageInfoAdapter{ws: ws}).GetStorageInfo(context.Background(), common.Address{})
	if err != nil {
		t.Fatalf("GetStorageInfo: %v", err)
	}
	if got == nil {
		t.Fatal("GetStorageInfo returned nil result")
	}
	callData, ok := mc.lastIn["getApprovedProviders"]
	if !ok {
		t.Fatal("GetStorageInfo did not call getApprovedProviders")
	}
	args, err := mc.viewABI.Methods["getApprovedProviders"].Inputs.Unpack(callData[4:])
	if err != nil {
		t.Fatalf("unpack getApprovedProviders args: %v", err)
	}
	limit, ok := args[1].(*big.Int)
	if !ok {
		t.Fatalf("limit arg has type %T, want *big.Int", args[1])
	}
	if limit.Sign() <= 0 {
		t.Fatalf("getApprovedProviders limit=%s, want > 0", limit)
	}
}

func TestCostsAdapter_CalculateMultiContextCosts_EnableCDNPropagatesToRefs(t *testing.T) {
	costSvc, err := costs.New(costs.Options{
		Chain: chain.Calibration,
		WarmStorage: fixedWarmStorageReader{price: &warmstorage.ServicePrice{
			PricePerTiBPerMonthNoCDN: big.NewInt(288000),
			TokenAddress:             common.HexToAddress("0xbeef"),
			EpochsPerMonth:           big.NewInt(2880),
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

	adapter := &costsAdapter{c: costSvc}
	actual, err := adapter.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(1024),
		[]storage.ContextCostRef{{CurrentDataSetSizeBytes: new(big.Int)}},
		storage.MultiCostOptions{EnableCDN: true},
	)
	if err != nil {
		t.Fatalf("adapter.CalculateMultiContextCosts: %v", err)
	}

	expected, err := costSvc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		big.NewInt(1024),
		[]costs.MultiContextRef{{IsNewDataSet: true, CurrentDataSetSizeBytes: new(big.Int), WithCDN: true}},
		&costs.UploadCostOptions{},
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

func TestCostsAdapter_CalculateMultiContextCosts_IgnoresCurrentSizeForNewDataSets(t *testing.T) {
	costSvc, err := costs.New(costs.Options{
		Chain: chain.Calibration,
		WarmStorage: fixedWarmStorageReader{price: &warmstorage.ServicePrice{
			PricePerTiBPerMonthNoCDN: big.NewInt(1000),
			TokenAddress:             common.HexToAddress("0xbeef"),
			EpochsPerMonth:           big.NewInt(2880),
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

	adapter := &costsAdapter{c: costSvc}
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

func TestPDPVerifierAdapter_GetScheduledRemovals_Dedupes(t *testing.T) {
	mc := newStorageInfoTestCaller(t)
	mc.setPDPReply(t, "getScheduledRemovals", []*big.Int{big.NewInt(2), big.NewInt(2), big.NewInt(5)})

	caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}

	got, err := (&pdpVerifierAdapter{caller: caller}).GetScheduledRemovals(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetScheduledRemovals: %v", err)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 5 {
		t.Fatalf("scheduled removals=%v want [2 5]", got)
	}
}

func TestPDPVerifierAdapter_GetScheduledRemovals_DataSetNotLiveReturnsEmpty(t *testing.T) {
	mc := newStorageInfoTestCaller(t)
	mc.setPDPError("getScheduledRemovals", errors.New("execution reverted: Data set not live"))

	caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}

	got, err := (&pdpVerifierAdapter{caller: caller}).GetScheduledRemovals(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetScheduledRemovals: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("scheduled removals=%v want empty", got)
	}
}

func TestPDPVerifierAdapter_GetNextChallengeEpoch_ReturnsNilForUnavailableEpoch(t *testing.T) {
	t.Run("not live", func(t *testing.T) {
		mc := newStorageInfoTestCaller(t)
		mc.setPDPError("getNextChallengeEpoch", errors.New("execution reverted: Data set not live"))

		caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
		if err != nil {
			t.Fatalf("NewPDPVerifierCaller: %v", err)
		}

		got, err := (&pdpVerifierAdapter{caller: caller}).GetNextChallengeEpoch(context.Background(), 42)
		if err != nil {
			t.Fatalf("GetNextChallengeEpoch: %v", err)
		}
		if got != nil {
			t.Fatalf("next challenge epoch=%v want nil", got)
		}
	})

	t.Run("non positive", func(t *testing.T) {
		mc := newStorageInfoTestCaller(t)
		mc.setPDPReply(t, "getNextChallengeEpoch", big.NewInt(0))

		caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
		if err != nil {
			t.Fatalf("NewPDPVerifierCaller: %v", err)
		}

		got, err := (&pdpVerifierAdapter{caller: caller}).GetNextChallengeEpoch(context.Background(), 42)
		if err != nil {
			t.Fatalf("GetNextChallengeEpoch: %v", err)
		}
		if got != nil {
			t.Fatalf("next challenge epoch=%v want nil", got)
		}
	})
}

type noopContractCaller struct{}

func (noopContractCaller) CallContract(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
	return make([]byte, 32), nil
}

type fixedWarmStorageReader struct {
	price *warmstorage.ServicePrice
}

func (r fixedWarmStorageReader) GetServicePrice(context.Context) (*warmstorage.ServicePrice, error) {
	return r.price, nil
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

type storageInfoTestCaller struct {
	fwssABI abi.ABI
	viewABI abi.ABI
	pdpABI  abi.ABI
	mu      sync.Mutex
	replies map[string][]byte
	errs    map[string]error
	lastIn  map[string][]byte
}

func newStorageInfoTestCaller(t *testing.T) *storageInfoTestCaller {
	t.Helper()
	fABI, err := fwssbind.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	vABI, err := fwssviewbind.FWSSViewMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	pABI, err := pdpverifierbind.PDPVerifierMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	return &storageInfoTestCaller{
		fwssABI: *fABI,
		viewABI: *vABI,
		pdpABI:  *pABI,
		replies: map[string][]byte{},
		errs:    map[string]error{},
		lastIn:  map[string][]byte{},
	}
}

func (m *storageInfoTestCaller) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *storageInfoTestCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	data := call.Data
	if len(data) < 4 {
		return nil, errors.New("calldata too short")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	selector := [4]byte{data[0], data[1], data[2], data[3]}
	for name, method := range m.fwssABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			if err, ok := m.errs[name]; ok {
				return nil, err
			}
			return m.replies[name], nil
		}
	}
	for name, method := range m.viewABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			if err, ok := m.errs[name]; ok {
				return nil, err
			}
			return m.replies[name], nil
		}
	}
	for name, method := range m.pdpABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			if err, ok := m.errs[name]; ok {
				return nil, err
			}
			return m.replies[name], nil
		}
	}
	return nil, errors.New("no method matches selector")
}

func (m *storageInfoTestCaller) setFWSSReply(t *testing.T, method string, values ...any) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	mth, ok := m.fwssABI.Methods[method]
	if !ok {
		t.Fatalf("fwss method %q not found", method)
	}
	b, err := mth.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	m.replies[method] = b
}

func (m *storageInfoTestCaller) setViewReply(t *testing.T, method string, values ...any) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	mth, ok := m.viewABI.Methods[method]
	if !ok {
		t.Fatalf("view method %q not found", method)
	}
	b, err := mth.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	m.replies[method] = b
}

func (m *storageInfoTestCaller) setPDPReply(t *testing.T, method string, values ...any) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	mth, ok := m.pdpABI.Methods[method]
	if !ok {
		t.Fatalf("pdp method %q not found", method)
	}
	b, err := mth.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	m.replies[method] = b
}

func (m *storageInfoTestCaller) setPDPError(method string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.errs[method] = err
}

func newStorageInfoTestWarmStorage(t *testing.T) (*warmstorage.Service, *storageInfoTestCaller) {
	t.Helper()
	mc := newStorageInfoTestCaller(t)
	ws, err := warmstorage.New(warmstorage.Options{
		Client:       mc,
		FWSS:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ViewContract: common.HexToAddress("0x2222222222222222222222222222222222222222"),
	})
	if err != nil {
		t.Fatalf("warmstorage.New: %v", err)
	}
	return ws, mc
}
