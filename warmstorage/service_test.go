package warmstorage

import (
	"context"
	"errors"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	fwssbind "github.com/strahe/synapse-go/internal/contracts/fwss"
	fwssviewbind "github.com/strahe/synapse-go/internal/contracts/fwssview"
	pdpbind "github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/types"
)

// mockCaller implements bind.ContractCaller by routing calls to the
// matching ABI method based on the 4-byte selector, and returning a
// pre-packed reply.
type mockCaller struct {
	fwssABI abi.ABI
	viewABI abi.ABI
	pdpABI  abi.ABI
	// method name → reply bytes or error
	replies  map[string][]byte
	errs     map[string]error
	lastIn   map[string][]byte
	handlers map[string]func([]byte) ([]byte, error)
}

func newMockCaller(t *testing.T) *mockCaller {
	t.Helper()
	fABI, err := fwssbind.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	vABI, err := fwssviewbind.FWSSViewMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	pABI, err := pdpbind.PDPVerifierMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	return &mockCaller{
		fwssABI:  *fABI,
		viewABI:  *vABI,
		pdpABI:   *pABI,
		replies:  map[string][]byte{},
		errs:     map[string]error{},
		lastIn:   map[string][]byte{},
		handlers: map[string]func([]byte) ([]byte, error){},
	}
}

func (m *mockCaller) CodeAt(_ context.Context, _ common.Address, _ *big.Int) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	data := call.Data
	if len(data) < 4 {
		return nil, errors.New("calldata too short")
	}
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
			if handler, ok := m.handlers[name]; ok {
				return handler(data)
			}
			if err, ok := m.errs[name]; ok {
				return nil, err
			}
			return m.replies[name], nil
		}
	}
	for name, method := range m.pdpABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			if handler, ok := m.handlers[name]; ok {
				return handler(data)
			}
			if err, ok := m.errs[name]; ok {
				return nil, err
			}
			return m.replies[name], nil
		}
	}
	return nil, errors.New("no method matches selector")
}

func TestToDataSetInfo_ClientDataSetIDAllowsUint256(t *testing.T) {
	large := new(big.Int).Lsh(big.NewInt(1), 200)
	got, err := toDataSetInfo(fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		DataSetId:       big.NewInt(1),
		PdpRailId:       big.NewInt(2),
		CacheMissRailId: big.NewInt(3),
		CdnRailId:       big.NewInt(4),
		Payer:           common.HexToAddress("0x1000000000000000000000000000000000000001"),
		Payee:           common.HexToAddress("0x2000000000000000000000000000000000000002"),
		ServiceProvider: common.HexToAddress("0x3000000000000000000000000000000000000003"),
		CommissionBps:   big.NewInt(100),
		ClientDataSetId: large,
		PdpEndEpoch:     big.NewInt(0),
		ProviderId:      big.NewInt(5),
	})
	if err != nil {
		t.Fatalf("toDataSetInfo: %v", err)
	}
	if got.ClientDataSetID == nil {
		t.Fatal("ClientDataSetID should be preserved as uint256")
	}
	if got.ClientDataSetID.Cmp(large) != 0 {
		t.Fatalf("ClientDataSetID = %s, want %s", got.ClientDataSetID.String(), large.String())
	}
}

func (m *mockCaller) setFWSSReply(t *testing.T, method string, values ...any) {
	t.Helper()
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

func (m *mockCaller) setViewReply(t *testing.T, method string, values ...any) {
	t.Helper()
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

func (m *mockCaller) setPDPReply(t *testing.T, method string, values ...any) {
	t.Helper()
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

func newTestService(t *testing.T) (*Service, *mockCaller) {
	t.Helper()
	mc := newMockCaller(t)
	s, err := New(Options{
		Client:       mc,
		FWSS:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ViewContract: common.HexToAddress("0x2222222222222222222222222222222222222222"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, mc
}

func newTestServiceWithPDP(t *testing.T) (*Service, *mockCaller) {
	t.Helper()
	mc := newMockCaller(t)
	s, err := New(Options{
		Client:       mc,
		FWSS:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ViewContract: common.HexToAddress("0x2222222222222222222222222222222222222222"),
		PDPVerifier:  common.HexToAddress("0x3333333333333333333333333333333333333333"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, mc
}

func TestNew_Validation(t *testing.T) {
	mc := newMockCaller(t)
	_, err := New(Options{FWSS: common.HexToAddress("0x01"), ViewContract: common.HexToAddress("0x02")})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for nil client, got %v", err)
	}
	_, err = New(Options{Client: mc, ViewContract: common.HexToAddress("0x02")})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for zero FWSS, got %v", err)
	}
	_, err = New(Options{Client: mc, FWSS: common.HexToAddress("0x01")})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for zero View, got %v", err)
	}
}

func TestGetServicePrice(t *testing.T) {
	s, mc := newTestService(t)
	mc.setFWSSReply(t, "getServicePrice", fwssbind.FilecoinWarmStorageServiceServicePricing{
		PricePerTiBPerMonthNoCDN:   big.NewInt(1000),
		PricePerTiBCdnEgress:       big.NewInt(20),
		PricePerTiBCacheMissEgress: big.NewInt(30),
		TokenAddress:               common.HexToAddress("0xabcd"),
		EpochsPerMonth:             big.NewInt(86400),
		MinimumPricePerMonth:       big.NewInt(5),
	})
	p, err := s.GetServicePrice(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.PricePerTiBPerMonthNoCDN.Int64() != 1000 || p.MinimumPricePerMonth.Int64() != 5 {
		t.Errorf("bad: %+v", p)
	}
}

func TestGetDataSet_FoundAndMissing(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getDataSet", fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		PdpRailId:       big.NewInt(7),
		CacheMissRailId: big.NewInt(0),
		CdnRailId:       big.NewInt(0),
		Payer:           common.HexToAddress("0x33"),
		Payee:           common.HexToAddress("0x44"),
		ServiceProvider: common.HexToAddress("0x55"),
		CommissionBps:   big.NewInt(100),
		ClientDataSetId: big.NewInt(1),
		PdpEndEpoch:     big.NewInt(0),
		ProviderId:      big.NewInt(9),
		DataSetId:       big.NewInt(42),
	})
	got, err := s.GetDataSet(context.Background(), types.DataSetID(42))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.DataSetID != 42 || got.ProviderID != 9 {
		t.Fatalf("got=%+v", got)
	}

	// not found: pdpRailId=0 → ErrNotFound
	mc.setViewReply(t, "getDataSet", fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		PdpRailId:       big.NewInt(0),
		CacheMissRailId: big.NewInt(0),
		CdnRailId:       big.NewInt(0),
		CommissionBps:   big.NewInt(0),
		ClientDataSetId: big.NewInt(0),
		PdpEndEpoch:     big.NewInt(0),
		ProviderId:      big.NewInt(0),
		DataSetId:       big.NewInt(0),
	})
	got, err = s.GetDataSet(context.Background(), types.DataSetID(99))
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got err=%v result=%+v", err, got)
	}
	if got != nil {
		t.Errorf("expected nil result with ErrNotFound, got %+v", got)
	}
}

func TestGetDataSet_ZeroDataSetID(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getDataSet", fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		PdpRailId:       big.NewInt(0),
		CacheMissRailId: big.NewInt(0),
		CdnRailId:       big.NewInt(0),
		CommissionBps:   big.NewInt(0),
		ClientDataSetId: big.NewInt(0),
		PdpEndEpoch:     big.NewInt(0),
		ProviderId:      big.NewInt(0),
		DataSetId:       big.NewInt(0),
	})
	_, err := s.GetDataSet(context.Background(), 0)
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero data set ID, got %v", err)
	}
}

func TestGetClientDataSets(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getClientDataSets0", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		{PdpRailId: big.NewInt(1), CacheMissRailId: big.NewInt(0), CdnRailId: big.NewInt(0), CommissionBps: big.NewInt(0), ClientDataSetId: big.NewInt(0), PdpEndEpoch: big.NewInt(0), ProviderId: big.NewInt(0), DataSetId: big.NewInt(1)},
		{PdpRailId: big.NewInt(2), CacheMissRailId: big.NewInt(0), CdnRailId: big.NewInt(0), CommissionBps: big.NewInt(0), ClientDataSetId: big.NewInt(0), PdpEndEpoch: big.NewInt(0), ProviderId: big.NewInt(0), DataSetId: big.NewInt(2)},
	})
	// The overloaded getClientDataSets has two variants. We call the
	// (address,offset,limit) one, which abigen exposes as GetClientDataSets
	// (first overload -> "getClientDataSets", second -> "getClientDataSets0").
	// Pack correctly:
	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		{PdpRailId: big.NewInt(1), CacheMissRailId: big.NewInt(0), CdnRailId: big.NewInt(0), CommissionBps: big.NewInt(0), ClientDataSetId: big.NewInt(0), PdpEndEpoch: big.NewInt(0), ProviderId: big.NewInt(0), DataSetId: big.NewInt(1)},
		{PdpRailId: big.NewInt(2), CacheMissRailId: big.NewInt(0), CdnRailId: big.NewInt(0), CommissionBps: big.NewInt(0), ClientDataSetId: big.NewInt(0), PdpEndEpoch: big.NewInt(0), ProviderId: big.NewInt(0), DataSetId: big.NewInt(2)},
	})
	list, err := s.GetClientDataSets(context.Background(), common.HexToAddress("0xaa"), types.ListOptions{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	if _, err := s.GetClientDataSets(context.Background(), common.Address{}, types.ListOptions{Limit: 50}); err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for zero payer, got %v", err)
	}
}

func TestGetAllDataSetMetadata(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getAllDataSetMetadata", []string{"source", "withCDN"}, []string{"app", ""})

	got, err := s.GetAllDataSetMetadata(context.Background(), types.DataSetID(42))
	if err != nil {
		t.Fatal(err)
	}
	if got["source"] != "app" || got["withCDN"] != "" {
		t.Fatalf("metadata=%v", got)
	}
}

func TestGetAllDataSetMetadata_EmptyReturnsEmptyMap(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getAllDataSetMetadata", []string{}, []string{})
	got, err := s.GetAllDataSetMetadata(context.Background(), types.DataSetID(42))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected non-nil map, got nil")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestGetAllDataSetMetadata_ZeroDataSetID(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getAllDataSetMetadata", []string{}, []string{})
	got, err := s.GetAllDataSetMetadata(context.Background(), 0)
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero data set ID, got map=%v err=%v", got, err)
	}
}

func TestGetApprovedProviderIDs(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getApprovedProviders", []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)})
	ids, err := s.GetApprovedProviderIDs(context.Background(), types.ListOptions{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 {
		t.Fatalf("len=%d", len(ids))
	}
}

func TestIsProviderApproved(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "isProviderApproved", true)
	ok, err := s.IsProviderApproved(context.Background(), types.ProviderID(5))
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("want true")
	}
}

func TestIsProviderApproved_ZeroProviderID(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "isProviderApproved", false)
	_, err := s.IsProviderApproved(context.Background(), 0)
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero provider ID, got %v", err)
	}
}

func TestGetClientDataSetsLength(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getClientDataSetsLength", big.NewInt(42))
	n, err := s.GetClientDataSetsLength(context.Background(), common.HexToAddress("0xab"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Int64() != 42 {
		t.Errorf("n=%d", n.Int64())
	}
}

// ---------------------------------------------------------------------------
// Getter tests
// ---------------------------------------------------------------------------

func TestFWSSAddress(t *testing.T) {
	s, _ := newTestService(t)
	want := common.HexToAddress("0x1111111111111111111111111111111111111111")
	if got := s.FWSSAddress(); got != want {
		t.Errorf("FWSSAddress() = %s, want %s", got, want)
	}
}

func TestViewAddress(t *testing.T) {
	s, _ := newTestService(t)
	want := common.HexToAddress("0x2222222222222222222222222222222222222222")
	if got := s.ViewAddress(); got != want {
		t.Errorf("ViewAddress() = %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// GetApprovedProvidersLength tests
// ---------------------------------------------------------------------------

func TestGetApprovedProvidersLength_Success(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getApprovedProvidersLength", big.NewInt(17))
	n, err := s.GetApprovedProvidersLength(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n.Int64() != 17 {
		t.Errorf("n=%d, want 17", n.Int64())
	}
}

func TestGetApprovedProvidersLength_Error(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getApprovedProvidersLength"] = errors.New("rpc error")
	_, err := s.GetApprovedProvidersLength(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// IsProviderApproved edge cases
// ---------------------------------------------------------------------------

func TestIsProviderApproved_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["isProviderApproved"] = errors.New("rpc error")
	_, err := s.IsProviderApproved(context.Background(), types.ProviderID(1))
	if err == nil {
		t.Error("expected RPC error")
	}
}

func TestIsProviderApproved_ReturnsFalse(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "isProviderApproved", false)
	ok, err := s.IsProviderApproved(context.Background(), types.ProviderID(5))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected false")
	}
}

// ---------------------------------------------------------------------------
// RPC error paths for low-coverage methods
// ---------------------------------------------------------------------------

func TestGetClientDataSetsLength_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getClientDataSetsLength"] = errors.New("rpc error")
	_, err := s.GetClientDataSetsLength(context.Background(), common.HexToAddress("0xab"))
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetServicePrice_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getServicePrice"] = errors.New("rpc error")
	_, err := s.GetServicePrice(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetDataSet_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getDataSet"] = errors.New("rpc error")
	_, err := s.GetDataSet(context.Background(), types.DataSetID(1))
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// Coverage additions: methods.go / iterate.go / terminate.go
// ---------------------------------------------------------------------------

func TestValidateDataSet_ZeroID(t *testing.T) {
	s, _ := newTestServiceWithPDP(t)
	err := s.ValidateDataSet(context.Background(), 0)
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestValidateDataSet_NoPDP(t *testing.T) {
	s, _ := newTestService(t)
	err := s.ValidateDataSet(context.Background(), 1)
	if err == nil || !errors.Is(err, ErrPDPVerifierNotConfigured) {
		t.Errorf("expected ErrPDPVerifierNotConfigured, got %v", err)
	}
}

func TestValidateDataSet_NotLive(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	mc.setPDPReply(t, "dataSetLive", false)
	err := s.ValidateDataSet(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrInvalidArgument) || errors.Is(err, ErrPDPVerifierNotConfigured) {
		t.Errorf("unexpected sentinel: %v", err)
	}
}

func TestValidateDataSet_WrongListener(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	mc.setPDPReply(t, "dataSetLive", true)
	mc.setPDPReply(t, "getDataSetListener", common.HexToAddress("0xdead"))
	err := s.ValidateDataSet(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateDataSet_OK(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	mc.setPDPReply(t, "dataSetLive", true)
	mc.setPDPReply(t, "getDataSetListener", common.HexToAddress("0x1111111111111111111111111111111111111111"))
	if err := s.ValidateDataSet(context.Background(), 1); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestGetActivePieceCount(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	mc.setPDPReply(t, "getActivePieceCount", big.NewInt(42))
	n, err := s.GetActivePieceCount(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n.Int64() != 42 {
		t.Errorf("got %d", n.Int64())
	}
}

func TestGetActivePieceCount_ZeroID(t *testing.T) {
	s, _ := newTestServiceWithPDP(t)
	_, err := s.GetActivePieceCount(context.Background(), 0)
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestGetActivePieceCount_NoPDP(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.GetActivePieceCount(context.Background(), 1)
	if err == nil || !errors.Is(err, ErrPDPVerifierNotConfigured) {
		t.Errorf("expected ErrPDPVerifierNotConfigured, got %v", err)
	}
}

func TestGetPieceMetadata(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getPieceMetadata", true, "value-1")
	ok, v, err := s.GetPieceMetadata(context.Background(), 1, 2, "k")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || v != "value-1" {
		t.Errorf("got (%v, %q)", ok, v)
	}
}

func TestGetPieceMetadata_ZeroID(t *testing.T) {
	s, _ := newTestService(t)
	_, _, err := s.GetPieceMetadata(context.Background(), 0, 1, "k")
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestGetAllPieceMetadata(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getAllPieceMetadata", []string{"a", "b"}, []string{"1", "2"})
	got, err := s.GetAllPieceMetadata(context.Background(), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got["a"] != "1" || got["b"] != "2" {
		t.Errorf("got %+v", got)
	}
}

func TestGetAllPieceMetadata_ZeroID(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.GetAllPieceMetadata(context.Background(), 0, 1)
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestGetOwner(t *testing.T) {
	s, mc := newTestService(t)
	want := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	mc.setFWSSReply(t, "owner", want)
	got, err := s.GetOwner(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %s", got.Hex())
	}
}

func TestIsOwner(t *testing.T) {
	s, mc := newTestService(t)
	want := common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd")
	mc.setFWSSReply(t, "owner", want)
	ok, err := s.IsOwner(context.Background(), want)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("expected true")
	}
	mc.setFWSSReply(t, "owner", want)
	ok, err = s.IsOwner(context.Background(), common.HexToAddress("0xdead"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected false")
	}
}

func TestGetOwner_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["owner"] = errors.New("rpc error")
	_, err := s.GetOwner(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetPDPConfig(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "getPDPConfig",
		uint64(7200),
		big.NewInt(60),
		big.NewInt(5),
		big.NewInt(100),
	)
	cfg, err := s.GetPDPConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxProvingPeriod != 7200 || cfg.ChallengeWindowSize.Int64() != 60 || cfg.ChallengesPerProof.Int64() != 5 || cfg.InitChallengeWindowStart.Int64() != 100 {
		t.Errorf("got %+v", cfg)
	}
}

func TestGetPDPConfig_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getPDPConfig"] = errors.New("rpc error")
	_, err := s.GetPDPConfig(context.Background())
	if err == nil {
		t.Error("expected error")
	}
}

func TestGetClientDataSetIds(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "clientDataSets", []*big.Int{big.NewInt(1), big.NewInt(2), big.NewInt(3)})
	ids, err := s.GetClientDataSetIds(context.Background(), common.HexToAddress("0xabcd"), types.ListOptions{Offset: 0, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 1 || ids[2] != 3 {
		t.Errorf("got %+v", ids)
	}
}

func TestGetClientDataSetIds_ZeroPayer(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.GetClientDataSetIds(context.Background(), common.Address{}, types.ListOptions{Limit: 10})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestTerminateDataSet_WriteNotConfigured(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.TerminateDataSet(context.Background(), 1)
	if err == nil || !errors.Is(err, ErrWriteNotConfigured) {
		t.Errorf("expected ErrWriteNotConfigured, got %v", err)
	}
}

func TestIterateAllApprovedProviderIDs_Success(t *testing.T) {
	s, mc := newTestService(t)
	// Single page under defaultIteratePageSize terminates after one call.
	mc.setViewReply(t, "getApprovedProviders", []*big.Int{big.NewInt(1), big.NewInt(2)})
	var got []types.ProviderID
	for id, err := range s.IterateAllApprovedProviderIDs(context.Background()) {
		if err != nil {
			t.Fatalf("iter err: %v", err)
		}
		got = append(got, id)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("got %+v", got)
	}
}

func TestIterateAllApprovedProviderIDs_CtxCancelled(t *testing.T) {
	s, _ := newTestService(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	sawErr := false
	for _, err := range s.IterateAllApprovedProviderIDs(ctx) {
		if err != nil {
			sawErr = true
			break
		}
	}
	if !sawErr {
		t.Error("expected cancelled ctx to yield error")
	}
}

func TestIterateAllClientDataSetIds_Success(t *testing.T) {
	s, mc := newTestService(t)
	mc.setViewReply(t, "clientDataSets", []*big.Int{big.NewInt(10), big.NewInt(11)})
	var ids []types.DataSetID
	for id, err := range s.IterateAllClientDataSetIds(context.Background(), common.HexToAddress("0xabcd")) {
		if err != nil {
			t.Fatalf("iter err: %v", err)
		}
		ids = append(ids, id)
	}
	if len(ids) != 2 || ids[0] != 10 || ids[1] != 11 {
		t.Errorf("got %+v", ids)
	}
}

func TestIterateAllClientDataSets_Error(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getClientDataSets"] = errors.New("rpc error")
	sawErr := false
	for _, err := range s.IterateAllClientDataSets(context.Background(), common.HexToAddress("0xabcd")) {
		if err != nil {
			sawErr = true
			break
		}
	}
	if !sawErr {
		t.Error("expected rpc error to propagate via iterator")
	}
}
