package warmstorage

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	iabi "github.com/strahe/synapse-go/internal/abi"
	fwssviewbind "github.com/strahe/synapse-go/internal/contracts/fwssview"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

func TestGetClientDataSetsWithDetails_PropagatesEnrichmentFailure(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		{
			PdpRailId:               big.NewInt(11),
			CacheMissRailId:         big.NewInt(0),
			CdnRailId:               big.NewInt(0),
			Payer:                   payer,
			Payee:                   common.HexToAddress("0x5555555555555555555555555555555555555555"),
			ServiceProvider:         common.HexToAddress("0x6666666666666666666666666666666666666666"),
			CommissionBps:           big.NewInt(100),
			ClientDataSetId:         big.NewInt(1),
			PdpEndEpoch:             big.NewInt(0),
			ProviderId:              big.NewInt(7),
			DataSetId:               big.NewInt(1),
			PendingOneTimePayments:  big.NewInt(0),
			LifecycleReserveBalance: big.NewInt(0),
		},
		{
			PdpRailId:               big.NewInt(12),
			CacheMissRailId:         big.NewInt(0),
			CdnRailId:               big.NewInt(0),
			Payer:                   payer,
			Payee:                   common.HexToAddress("0x7777777777777777777777777777777777777777"),
			ServiceProvider:         common.HexToAddress("0x8888888888888888888888888888888888888888"),
			CommissionBps:           big.NewInt(200),
			ClientDataSetId:         big.NewInt(2),
			PdpEndEpoch:             big.NewInt(0),
			ProviderId:              big.NewInt(8),
			DataSetId:               big.NewInt(2),
			PendingOneTimePayments:  big.NewInt(0),
			LifecycleReserveBalance: big.NewInt(0),
		},
	})
	mc.setPDPReply(t, "getDataSetListener", s.fwssAddr)
	mc.handlers["dataSetLive"] = func(data []byte) ([]byte, error) {
		args, err := mc.pdpABI.Methods["dataSetLive"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if args[0].(*big.Int).Cmp(big.NewInt(1)) == 0 {
			return nil, errors.New("boom")
		}
		return mc.pdpABI.Methods["dataSetLive"].Outputs.Pack(false)
	}

	got, err := s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err == nil {
		t.Fatalf("GetClientDataSetsWithDetails err=nil, got=%+v want enrichment failure", got)
	}
	if !strings.Contains(err.Error(), "dataSetLive") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("GetClientDataSetsWithDetails err=%v, want dataSetLive context and revert reason", err)
	}
}

func TestUnpackDataSetDetailsResult_PreservesUnknownFailureData(t *testing.T) {
	_, err := unpackDataSetDetailsResult(iabi.Result3{Success: false, ReturnData: []byte{0xde, 0xad}}, nil)
	if err == nil || !strings.Contains(err.Error(), "0xdead") {
		t.Fatalf("error = %v, want raw return data", err)
	}
}

func TestTopUpCDNPaymentRails_RejectsDoubleZeroTopUp(t *testing.T) {
	s, backend := newWriteTestService(t)

	got, err := s.TopUpCDNPaymentRails(context.Background(), types.NewBigInt(1), big.NewInt(0), big.NewInt(0))
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("TopUpCDNPaymentRails double zero err=%v result=%+v, want ErrInvalidArgument", err, got)
	}
	if len(backend.sent) != 0 {
		t.Fatalf("sent tx count = %d, want 0", len(backend.sent))
	}
}

func TestGetClientDataSetsWithDetails_IncludesParityMetadata(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		{
			PdpRailId:               big.NewInt(11),
			CacheMissRailId:         big.NewInt(0),
			CdnRailId:               big.NewInt(99),
			Payer:                   payer,
			Payee:                   common.HexToAddress("0x5555555555555555555555555555555555555555"),
			ServiceProvider:         common.HexToAddress("0x6666666666666666666666666666666666666666"),
			CommissionBps:           big.NewInt(100),
			ClientDataSetId:         big.NewInt(1),
			PdpEndEpoch:             big.NewInt(0),
			ProviderId:              big.NewInt(7),
			DataSetId:               big.NewInt(42),
			PendingOneTimePayments:  big.NewInt(0),
			LifecycleReserveBalance: big.NewInt(0),
		},
	})
	mc.setPDPReply(t, "getDataSetListener", s.fwssAddr)
	mc.setPDPReply(t, "dataSetLive", true)
	mc.setPDPReply(t, "getActivePieceCount", big.NewInt(3))
	mc.setViewReply(t, "getAllDataSetMetadata", []string{"withCDN", "source"}, []string{"", "app"})

	got, err := s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err != nil {
		t.Fatalf("GetClientDataSetsWithDetails: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("result len = %d, want 1", len(got))
	}

	val := reflect.ValueOf(*got[0])
	pdpID := val.FieldByName("PDPVerifierDataSetID")
	if !pdpID.IsValid() {
		t.Fatalf("PDPVerifierDataSetID missing or wrong: %v", pdpID)
	}
	gotPDPID, ok := pdpID.Interface().(types.BigInt)
	if !ok || !gotPDPID.Equal(types.NewBigInt(42)) {
		t.Fatalf("PDPVerifierDataSetID missing or wrong: %v", pdpID)
	}
	withCDN := val.FieldByName("WithCDN")
	if !withCDN.IsValid() || !withCDN.Bool() {
		t.Fatalf("WithCDN missing or false: %v", withCDN)
	}
	metadata := val.FieldByName("Metadata")
	if !metadata.IsValid() {
		t.Fatal("Metadata field missing")
	}
	meta, ok := metadata.Interface().(map[string]string)
	if !ok {
		t.Fatalf("Metadata has wrong type: %T", metadata.Interface())
	}
	if meta["source"] != "app" {
		t.Fatalf("Metadata[source]=%q, want app", meta["source"])
	}
	if _, ok := meta["withCDN"]; !ok {
		t.Fatalf("Metadata missing withCDN key: %v", meta)
	}
}

func TestGetClientDataSetsWithDetails_ReportsMetadataMappingError(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		dataSetInfoView(1),
	})
	mc.setPDPReply(t, "getDataSetListener", s.fwssAddr)
	mc.setPDPReply(t, "dataSetLive", false)
	mc.setViewReply(t, "getAllDataSetMetadata", []string{"source"}, []string{})

	got, err := s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err == nil {
		t.Fatalf("GetClientDataSetsWithDetails err=nil, got=%+v want metadata mapping error", got)
	}
	if !strings.Contains(err.Error(), "getAllDataSetMetadata dataSetID 1") || !strings.Contains(err.Error(), "mismatched keys") {
		t.Fatalf("GetClientDataSetsWithDetails err=%v, want metadata operation, dataset, and mapping context", err)
	}
}

func TestGetClientDataSetsWithDetails_DefaultBatchLimit(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x1000000000000000000000000000000000000001")
	infos := make([]fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView, 100)
	for i := range infos {
		infos[i] = dataSetInfoView(int64(i + 1))
	}
	listCalls := 0
	mc.handlers["getClientDataSets"] = func(data []byte) ([]byte, error) {
		listCalls++
		args, err := mc.viewABI.Methods["getClientDataSets"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if args[1].(*big.Int).Sign() == 0 {
			return mc.viewABI.Methods["getClientDataSets"].Outputs.Pack(infos)
		}
		return mc.viewABI.Methods["getClientDataSets"].Outputs.Pack([]fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{})
	}
	mc.setPDPReply(t, "getDataSetListener", s.fwssAddr)
	mc.setPDPReply(t, "dataSetLive", true)
	mc.setPDPReply(t, "getActivePieceCount", big.NewInt(1))
	mc.setViewReply(t, "getAllDataSetMetadata", []string{}, []string{})

	got, err := s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(infos) {
		t.Fatalf("len(details) = %d, want %d", len(got), len(infos))
	}
	for i := range got {
		if got[i].DataSetID.String() != big.NewInt(int64(i+1)).String() {
			t.Fatalf("details[%d].DataSetID = %s, want %d", i, got[i].DataSetID.String(), i+1)
		}
	}
	wantSizes := []int{64, 36, 64, 64, 64, 8, 64, 36}
	if !slices.Equal(mc.multicallSizes, wantSizes) {
		t.Fatalf("multicall sizes = %v, want %v", mc.multicallSizes, wantSizes)
	}
	if listCalls != 2 {
		t.Fatalf("getClientDataSets calls = %d, want 2", listCalls)
	}
}

func TestGetClientDataSetsWithDetails_FiltersBeforeDependentCalls(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x1000000000000000000000000000000000000001")
	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		dataSetInfoView(1),
		dataSetInfoView(2),
		dataSetInfoView(3),
	})
	mc.handlers["getDataSetListener"] = func(data []byte) ([]byte, error) {
		args, err := mc.pdpABI.Methods["getDataSetListener"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		listener := s.fwssAddr
		if args[0].(*big.Int).Cmp(big.NewInt(1)) == 0 {
			listener = common.HexToAddress("0x9999999999999999999999999999999999999999")
		}
		return mc.pdpABI.Methods["getDataSetListener"].Outputs.Pack(listener)
	}
	mc.handlers["dataSetLive"] = func(data []byte) ([]byte, error) {
		args, err := mc.pdpABI.Methods["dataSetLive"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		return mc.pdpABI.Methods["dataSetLive"].Outputs.Pack(args[0].(*big.Int).Cmp(big.NewInt(3)) == 0)
	}
	activeCalls := 0
	mc.handlers["getActivePieceCount"] = func(_ []byte) ([]byte, error) {
		activeCalls++
		return mc.pdpABI.Methods["getActivePieceCount"].Outputs.Pack(big.NewInt(7))
	}
	mc.setViewReply(t, "getAllDataSetMetadata", []string{}, []string{})

	got, err := s.GetClientDataSetsWithDetails(context.Background(), payer, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].DataSetID.String() != "2" || got[1].DataSetID.String() != "3" {
		t.Fatalf("details = %+v, want data sets 2 and 3 in order", got)
	}
	if got[0].IsLive || got[0].ActivePieceCount.Sign() != 0 {
		t.Fatalf("data set 2 = %+v, want non-live with zero active pieces", got[0])
	}
	if !got[1].IsLive || got[1].ActivePieceCount.Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("data set 3 = %+v, want live with 7 active pieces", got[1])
	}
	if activeCalls != 1 {
		t.Fatalf("getActivePieceCount calls = %d, want 1", activeCalls)
	}
	if want := []int{3, 4, 1}; !slices.Equal(mc.multicallSizes, want) {
		t.Fatalf("multicall sizes = %v, want %v", mc.multicallSizes, want)
	}
}

func TestGetClientDataSetsWithDetails_UsesDataSetErrorOrder(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x1000000000000000000000000000000000000001")
	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		dataSetInfoView(1),
		dataSetInfoView(2),
	})
	mc.handlers["getDataSetListener"] = func(data []byte) ([]byte, error) {
		args, err := mc.pdpABI.Methods["getDataSetListener"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if args[0].(*big.Int).Cmp(big.NewInt(2)) == 0 {
			return nil, errors.New("listener failed")
		}
		return mc.pdpABI.Methods["getDataSetListener"].Outputs.Pack(s.fwssAddr)
	}
	mc.setPDPReply(t, "dataSetLive", true)
	mc.handlers["getActivePieceCount"] = func(_ []byte) ([]byte, error) {
		return nil, errors.New("active count failed")
	}
	mc.setViewReply(t, "getAllDataSetMetadata", []string{}, []string{})

	_, err := s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err == nil || !strings.Contains(err.Error(), "getActivePieceCount dataSetID 1") {
		t.Fatalf("error = %v, want data set 1 active-count failure", err)
	}
}

func TestGetClientDataSetsWithDetails_StopsAtResolvedErrorFrontier(t *testing.T) {
	mc := newMockCaller(t)
	s, err := New(Options{
		Client:            mc,
		FWSS:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ViewContract:      common.HexToAddress("0x2222222222222222222222222222222222222222"),
		PDPVerifier:       common.HexToAddress("0x3333333333333333333333333333333333333333"),
		MaxMulticallCalls: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payer := common.HexToAddress("0x1000000000000000000000000000000000000001")
	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		dataSetInfoView(1),
		dataSetInfoView(2),
	})
	mc.handlers["getDataSetListener"] = func(data []byte) ([]byte, error) {
		args, err := mc.pdpABI.Methods["getDataSetListener"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if args[0].(*big.Int).Cmp(big.NewInt(1)) == 0 {
			return nil, errors.New("listener failed")
		}
		return mc.pdpABI.Methods["getDataSetListener"].Outputs.Pack(s.fwssAddr)
	}
	batchCalls := 0
	mc.multicallFn = func(data []byte) ([]byte, error) {
		batchCalls++
		if batchCalls > 1 {
			return nil, errors.New("later transport failure")
		}
		return mc.handleMulticall(data)
	}

	_, err = s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err == nil || !strings.Contains(err.Error(), "getDataSetListener dataSetID 1") ||
		!strings.Contains(err.Error(), "listener failed") {
		t.Fatalf("error = %v, want first listener revert", err)
	}
	if batchCalls != 1 {
		t.Fatalf("multicall count = %d, want 1", batchCalls)
	}
}

func TestGetClientDataSetsWithDetails_StopsAfterTopLevelBatchFailure(t *testing.T) {
	mc := newMockCaller(t)
	s, err := New(Options{
		Client:            mc,
		FWSS:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ViewContract:      common.HexToAddress("0x2222222222222222222222222222222222222222"),
		PDPVerifier:       common.HexToAddress("0x3333333333333333333333333333333333333333"),
		MaxMulticallCalls: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payer := common.HexToAddress("0x1000000000000000000000000000000000000001")
	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		dataSetInfoView(1),
		dataSetInfoView(2),
		dataSetInfoView(3),
	})
	batchCalls := 0
	mc.multicallFn = func(data []byte) ([]byte, error) {
		batchCalls++
		if batchCalls == 2 {
			return nil, errors.New("RPC unavailable")
		}
		return mc.handleMulticall(data)
	}
	mc.setPDPReply(t, "getDataSetListener", s.fwssAddr)

	_, err = s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err == nil || !strings.Contains(err.Error(), "getDataSetListener batch") ||
		!strings.Contains(err.Error(), "calls [1:2)") {
		t.Fatalf("error = %v, want listener second-batch context", err)
	}
	if batchCalls != 2 {
		t.Fatalf("multicall count = %d, want 2", batchCalls)
	}
}

type mockWriteBackend struct {
	*mockCaller

	mu          sync.Mutex
	sent        []*coretypes.Transaction
	nonces      map[common.Address]uint64
	receiptFn   func(context.Context, common.Hash) (*coretypes.Receipt, error)
	blockNumber uint64
	nonceErr    error
	sendErr     error
}

func newMockWriteBackend(t *testing.T) *mockWriteBackend {
	t.Helper()
	return &mockWriteBackend{
		mockCaller: newMockCaller(t),
		nonces:     map[common.Address]uint64{},
	}
}

func (m *mockWriteBackend) HeaderByNumber(_ context.Context, _ *big.Int) (*coretypes.Header, error) {
	return &coretypes.Header{BaseFee: big.NewInt(1_000_000_000), Number: big.NewInt(1)}, nil
}

func (m *mockWriteBackend) PendingCodeAt(_ context.Context, _ common.Address) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockWriteBackend) PendingNonceAt(_ context.Context, account common.Address) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nonceErr != nil {
		return 0, m.nonceErr
	}
	return m.nonces[account], nil
}

func (m *mockWriteBackend) SuggestGasPrice(_ context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (m *mockWriteBackend) SuggestGasTipCap(_ context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (m *mockWriteBackend) EstimateGas(_ context.Context, _ ethereum.CallMsg) (uint64, error) {
	return 100_000, nil
}

func (m *mockWriteBackend) SendTransaction(_ context.Context, tx *coretypes.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, tx)
	return nil
}

func (m *mockWriteBackend) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]coretypes.Log, error) {
	return nil, nil
}

func (m *mockWriteBackend) SubscribeFilterLogs(_ context.Context, _ ethereum.FilterQuery, _ chan<- coretypes.Log) (ethereum.Subscription, error) {
	return nil, errors.New("subscription not supported")
}

func (m *mockWriteBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*coretypes.Receipt, error) {
	if m.receiptFn != nil {
		return m.receiptFn(ctx, hash)
	}
	return nil, ethereum.NotFound
}

func (m *mockWriteBackend) BlockNumber(context.Context) (uint64, error) {
	if m.blockNumber != 0 {
		return m.blockNumber, nil
	}
	return 10, nil
}

func newWriteTestSigner(t *testing.T) signer.EVMSigner {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := signer.NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newWriteTestService(t *testing.T) (*Service, *mockWriteBackend) {
	t.Helper()
	backend := newMockWriteBackend(t)
	s, err := New(Options{
		Client:       backend,
		Backend:      backend,
		Signer:       newWriteTestSigner(t),
		ChainID:      314159,
		FWSS:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ViewContract: common.HexToAddress("0x2222222222222222222222222222222222222222"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, backend
}

func TestReadDoesNotUseSignerAddress(t *testing.T) {
	s, backend := newWriteTestService(t)
	backend.rejectNonZeroCallFrom = true
	want := common.HexToAddress("0x4444444444444444444444444444444444444444")
	backend.setFWSSReply(t, "owner", want)

	got, err := s.GetOwner(context.Background())
	if err != nil {
		t.Fatalf("GetOwner: %v", err)
	}
	if got != want {
		t.Fatalf("owner=%s want %s", got, want)
	}
}
