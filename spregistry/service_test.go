package spregistry

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	sprbind "github.com/strahe/synapse-go/internal/contracts/spregistry"
	"github.com/strahe/synapse-go/types"
)

type mockCaller struct {
	sprABI       abi.ABI
	multicallABI abi.ABI
	replies      map[string][]byte
	errs         map[string]error
	handlers     map[string]func([]any) ([]byte, error)
	multicallFn  func([]byte) ([]byte, error)
	argCheck     func(string, []any)
}

func newMockCaller(t *testing.T) *mockCaller {
	t.Helper()
	a, err := sprbind.SPRegistryMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	mcABI, err := abi.JSON(strings.NewReader(`[{"inputs":[{"components":[{"internalType":"address","name":"target","type":"address"},{"internalType":"bool","name":"allowFailure","type":"bool"},{"internalType":"bytes","name":"callData","type":"bytes"}],"internalType":"struct Multicall3.Call3[]","name":"calls","type":"tuple[]"}],"name":"aggregate3","outputs":[{"components":[{"internalType":"bool","name":"success","type":"bool"},{"internalType":"bytes","name":"returnData","type":"bytes"}],"internalType":"struct Multicall3.Result[]","name":"returnData","type":"tuple[]"}],"stateMutability":"payable","type":"function"}]`))
	if err != nil {
		t.Fatal(err)
	}
	return &mockCaller{
		sprABI:       *a,
		multicallABI: mcABI,
		replies:      map[string][]byte{},
		errs:         map[string]error{},
		handlers:     map[string]func([]any) ([]byte, error){},
	}
}

func (m *mockCaller) CodeAt(_ context.Context, _ common.Address, _ *big.Int) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if len(call.Data) < 4 {
		return nil, errors.New("calldata too short")
	}
	if call.To != nil && *call.To == chain.Mainnet.Addresses().Multicall3 {
		if m.multicallFn != nil {
			return m.multicallFn(call.Data)
		}
		return m.handleMulticall(call.Data)
	}
	return m.handleRegistryCall(call.Data)
}

func (m *mockCaller) handleRegistryCall(data []byte) ([]byte, error) {
	if len(data) < 4 {
		return nil, errors.New("calldata too short")
	}
	selector := [4]byte{data[0], data[1], data[2], data[3]}
	for name, method := range m.sprABI.Methods {
		if [4]byte(method.ID) == selector {
			args, err := method.Inputs.Unpack(data[4:])
			if err != nil {
				return nil, err
			}
			if m.argCheck != nil {
				m.argCheck(name, args)
			}
			if err, ok := m.errs[name]; ok {
				return nil, err
			}
			if handler, ok := m.handlers[name]; ok {
				return handler(args)
			}
			return m.replies[name], nil
		}
	}
	return nil, errors.New("no method matches selector")
}

func (m *mockCaller) handleMulticall(data []byte) ([]byte, error) {
	vals, err := m.multicallABI.Methods["aggregate3"].Inputs.Unpack(data[4:])
	if err != nil {
		return nil, err
	}
	rawCalls, ok := vals[0].([]struct {
		Target       common.Address `json:"target"`
		AllowFailure bool           `json:"allowFailure"`
		CallData     []byte         `json:"callData"`
	})
	if !ok {
		return nil, errors.New("unexpected aggregate3 input type")
	}
	type result3 struct {
		Success    bool
		ReturnData []byte
	}
	results := make([]result3, len(rawCalls))
	for i, c := range rawCalls {
		reply, err := m.handleRegistryCall(c.CallData)
		if err != nil {
			if c.AllowFailure {
				results[i] = result3{Success: false, ReturnData: []byte(err.Error())}
				continue
			}
			return nil, err
		}
		results[i] = result3{Success: true, ReturnData: reply}
	}
	return m.multicallABI.Methods["aggregate3"].Outputs.Pack(results)
}

func (m *mockCaller) set(t *testing.T, method string, values ...any) {
	t.Helper()
	mth, ok := m.sprABI.Methods[method]
	if !ok {
		t.Fatalf("method %q not found", method)
	}
	b, err := mth.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	m.replies[method] = b
}

func (m *mockCaller) setHandler(t *testing.T, method string, handler func([]any) ([]byte, error)) {
	t.Helper()
	if _, ok := m.sprABI.Methods[method]; !ok {
		t.Fatalf("method %q not found", method)
	}
	m.handlers[method] = handler
}

func (m *mockCaller) pack(t *testing.T, method string, values ...any) []byte {
	t.Helper()
	mth, ok := m.sprABI.Methods[method]
	if !ok {
		t.Fatalf("method %q not found", method)
	}
	b, err := mth.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	return b
}

func newTestService(t *testing.T) (*Service, *mockCaller) {
	t.Helper()
	mc := newMockCaller(t)
	s, err := New(Options{Client: mc, Address: common.HexToAddress("0xabcd")})
	if err != nil {
		t.Fatal(err)
	}
	return s, mc
}

func TestNew_Validation(t *testing.T) {
	_, err := New(Options{Address: common.HexToAddress("0x01")})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for nil Client, got %v", err)
	}
	mc := newMockCaller(t)
	_, err = New(Options{Client: mc})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for zero Address, got %v", err)
	}
}

func TestGetProvider_FoundAndMissing(t *testing.T) {
	s, mc := newTestService(t)

	mc.set(t, "getProvider", sprbind.ServiceProviderRegistryServiceProviderInfoView{
		ProviderId: big.NewInt(3),
		Info: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
			ServiceProvider: common.HexToAddress("0x11"),
			Payee:           common.HexToAddress("0x22"),
			Name:            "alice",
			Description:     "",
			IsActive:        true,
		},
	})
	got, err := s.GetProvider(context.Background(), types.NewBigInt(3))
	if err != nil || got == nil || got.Name != "alice" || !got.ID.Equal(types.NewBigInt(3)) {
		t.Fatalf("got=%+v err=%v", got, err)
	}

	mc.set(t, "getProvider", sprbind.ServiceProviderRegistryServiceProviderInfoView{
		ProviderId: big.NewInt(0),
		Info:       sprbind.ServiceProviderRegistryStorageServiceProviderInfo{},
	})
	got, err = s.GetProvider(context.Background(), types.NewBigInt(99))
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got err=%v result=%+v", err, got)
	}
	if got != nil {
		t.Errorf("expected nil result with ErrNotFound, got %+v", got)
	}
}

func TestGetProvider_ZeroProviderID(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.GetProvider(context.Background(), types.NewBigInt(0))
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero provider ID, got %v", err)
	}
}

func TestGetProviderIDByAddress(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderIdByAddress", big.NewInt(7))
	got, err := s.GetProviderIDByAddress(context.Background(), common.HexToAddress("0x55"))
	if err != nil || !got.Equal(types.NewBigInt(7)) {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err := s.GetProviderIDByAddress(context.Background(), common.Address{}); err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for zero addr, got %v", err)
	}
}

// Unknown addresses return (0, nil) from the contract; this
// asymmetric behaviour (vs. GetProviderByAddress returning ErrNotFound) is
// intentional and documented in spregistry/doc.go. Callers must check
// id.IsZero().
func TestGetProviderIDByAddress_UnknownReturnsZero(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderIdByAddress", big.NewInt(0))
	got, err := s.GetProviderIDByAddress(context.Background(), common.HexToAddress("0xdeadbeef"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Fatalf("expected zero ProviderID for unknown addr, got %v", got)
	}
}

func TestIsProviderActive(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "isProviderActive", true)
	ok, err := s.IsProviderActive(context.Background(), types.NewBigInt(1))
	if err != nil || !ok {
		t.Fatal(err)
	}
}

func TestIsProviderActive_ZeroProviderID(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.IsProviderActive(context.Background(), types.NewBigInt(0))
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero provider ID, got %v", err)
	}
}

func TestIsRegisteredProvider(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "isRegisteredProvider", true)
	ok, err := s.IsRegisteredProvider(context.Background(), common.HexToAddress("0x55"))
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}

	mc.set(t, "isRegisteredProvider", false)
	ok, err = s.IsRegisteredProvider(context.Background(), common.HexToAddress("0x66"))
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestIsRegisteredProvider_ZeroAddress(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.IsRegisteredProvider(context.Background(), common.Address{})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero address, got %v", err)
	}
}

func TestIsRegisteredProvider_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["isRegisteredProvider"] = errors.New("rpc error")
	_, err := s.IsRegisteredProvider(context.Background(), common.HexToAddress("0x55"))
	if err == nil {
		t.Fatal("expected RPC error")
	}
}

func TestGetProviderCountAndActive(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderCount", big.NewInt(50))
	mc.set(t, "activeProviderCount", big.NewInt(33))
	total, _ := s.GetProviderCount(context.Background())
	active, _ := s.GetActiveProviderCount(context.Background())
	if total.Int64() != 50 || active.Int64() != 33 {
		t.Fatalf("total=%d active=%d", total, active)
	}
}

func pdpCapsFixture() (keys []string, values [][]byte) {
	keys = []string{
		CapServiceURL,
		CapMinPieceSize,
		CapMaxPieceSize,
		CapStoragePrice,
		CapMinProvingPeriod,
		CapLocation,
		CapPaymentToken,
		CapIPNIPiece,
		CapIPNIIPFS,
	}
	values = [][]byte{
		[]byte("https://pdp.example.com"),
		big.NewInt(1024).Bytes(),
		big.NewInt(1 << 30).Bytes(),
		big.NewInt(1_000_000).Bytes(),
		big.NewInt(2880).Bytes(),
		[]byte("US-EAST"),
		common.HexToAddress("0xb3042734b608a1B16e9e86B374A3f3e389B4cDf0").Bytes(),
		{0x01},
		{0x00}, // NOT enabled (must be 0x01 to count)
	}
	return
}

func pdpCapsFixtureWithToken(token common.Address) (keys []string, values [][]byte) {
	keys, values = pdpCapsFixture()
	values[6] = token.Bytes()
	return
}

func pdpProviderFixture(providerID int64, serviceProvider common.Address, name string) sprbind.ServiceProviderRegistryStorageProviderWithProduct {
	keys, values := pdpCapsFixture()
	return sprbind.ServiceProviderRegistryStorageProviderWithProduct{
		ProviderId: big.NewInt(providerID),
		ProviderInfo: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
			ServiceProvider: serviceProvider,
			Payee:           common.HexToAddress("0x66"),
			Name:            name,
			Description:     "test",
			IsActive:        true,
		},
		Product: sprbind.ServiceProviderRegistryStorageServiceProduct{
			ProductType:    uint8(ProductTypePDP),
			CapabilityKeys: keys,
			IsActive:       true,
		},
		ProductCapabilityValues: values,
	}
}

func addrPtr(addr common.Address) *common.Address {
	return &addr
}

func TestGetPDPProvider(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	mc.set(t, "getProviderWithProduct", sprbind.ServiceProviderRegistryStorageProviderWithProduct{
		ProviderId: big.NewInt(4),
		ProviderInfo: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
			ServiceProvider: common.HexToAddress("0x99"),
			Payee:           common.HexToAddress("0xaa"),
			Name:            "pdp-sp",
			IsActive:        true,
		},
		Product: sprbind.ServiceProviderRegistryStorageServiceProduct{
			ProductType:    uint8(ProductTypePDP),
			CapabilityKeys: keys,
			IsActive:       true,
		},
		ProductCapabilityValues: vals,
	})
	p, err := s.GetPDPProvider(context.Background(), types.NewBigInt(4))
	if err != nil {
		t.Fatal(err)
	}
	if p == nil || p.Offering.ServiceURL != "https://pdp.example.com" {
		t.Fatalf("p=%+v", p)
	}
	if !p.Offering.IPNIPiece || p.Offering.IPNIIPFS {
		t.Errorf("flags wrong: piece=%v ipfs=%v", p.Offering.IPNIPiece, p.Offering.IPNIIPFS)
	}
	if p.Offering.MinPieceSizeInBytes.Int64() != 1024 {
		t.Errorf("minPiece=%d", p.Offering.MinPieceSizeInBytes)
	}
	if p.Offering.PaymentTokenAddress != common.HexToAddress("0xb3042734b608a1B16e9e86B374A3f3e389B4cDf0") {
		t.Errorf("token=%s", p.Offering.PaymentTokenAddress)
	}
}

func TestGetPDPProvider_MissingReturnsNotFound(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderWithProduct", sprbind.ServiceProviderRegistryStorageProviderWithProduct{
		ProviderId:              big.NewInt(0),
		ProviderInfo:            sprbind.ServiceProviderRegistryStorageServiceProviderInfo{},
		Product:                 sprbind.ServiceProviderRegistryStorageServiceProduct{ProductType: 0, CapabilityKeys: []string{}, IsActive: false},
		ProductCapabilityValues: [][]byte{},
	})
	p, err := s.GetPDPProvider(context.Background(), types.NewBigInt(77))
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got err=%v result=%+v", err, p)
	}
	if p != nil {
		t.Errorf("expected nil result with ErrNotFound, got %+v", p)
	}
}

func TestGetPDPProvider_ZeroProviderID(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.GetPDPProvider(context.Background(), types.NewBigInt(0))
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero provider ID, got %v", err)
	}
}

func TestGetPDPProviderByAddress_ZeroAddress(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.GetPDPProviderByAddress(context.Background(), common.Address{})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument for zero address, got %v", err)
	}
}

func TestGetPDPProviderByAddress_UnknownReturnsNotFound(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderIdByAddress", big.NewInt(0))
	got, err := s.GetPDPProviderByAddress(context.Background(), common.HexToAddress("0x99"))
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got err=%v result=%+v", err, got)
	}
	if got != nil {
		t.Fatalf("expected nil provider, got %+v", got)
	}
}

func TestGetPDPProviderByAddress_ValidProvider(t *testing.T) {
	s, mc := newTestService(t)
	addr := common.HexToAddress("0x55")
	mc.set(t, "getProviderIdByAddress", big.NewInt(4))
	mc.set(t, "getProviderWithProduct", pdpProviderFixture(4, addr, "alice"))
	got, err := s.GetPDPProviderByAddress(context.Background(), addr)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || !got.Info.ID.Equal(types.NewBigInt(4)) || got.Info.ServiceProvider != addr || got.Offering.ServiceURL == "" {
		t.Fatalf("got=%+v", got)
	}
}

func TestGetPDPProviderByAddress_PDPError(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderIdByAddress", big.NewInt(4))
	mc.errs["getProviderWithProduct"] = errors.New("rpc error")
	got, err := s.GetPDPProviderByAddress(context.Background(), common.HexToAddress("0x55"))
	if err == nil {
		t.Fatalf("expected error, got provider %+v", got)
	}
}

func TestGetPDPProviders(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	mc.argCheck = func(method string, args []any) {
		if method != "getProvidersByProductType" {
			return
		}
		if got := args[3].(*big.Int); got.Int64() != 50 {
			t.Fatalf("limit = %s, want 50", got)
		}
	}
	raw := sprbind.ServiceProviderRegistryStoragePaginatedProviders{
		Providers: []sprbind.ServiceProviderRegistryStorageProviderWithProduct{
			{
				ProviderId: big.NewInt(1),
				ProviderInfo: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
					ServiceProvider: common.HexToAddress("0x01"), Name: "a", IsActive: true,
				},
				Product: sprbind.ServiceProviderRegistryStorageServiceProduct{
					ProductType: 0, CapabilityKeys: keys, IsActive: true,
				},
				ProductCapabilityValues: vals,
			},
		},
		HasMore: true,
	}
	mc.set(t, "getProvidersByProductType", raw)
	out, err := s.GetPDPProviders(context.Background(), true, types.ListOptions{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasMore || len(out.Providers) != 1 {
		t.Fatalf("out=%+v", out)
	}
}

func TestGetProvidersByIDs(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProvidersByIds",
		[]sprbind.ServiceProviderRegistryServiceProviderInfoView{
			{ProviderId: big.NewInt(1), Info: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{ServiceProvider: common.HexToAddress("0x01"), Name: "a", IsActive: true}},
			{ProviderId: big.NewInt(0), Info: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{}},
		},
		[]bool{true, false},
	)
	out, err := s.GetProvidersByIDs(context.Background(), []types.BigInt{types.NewBigInt(1), types.NewBigInt(999)})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] == nil || out[1] != nil {
		t.Fatalf("out=%+v", out)
	}
	if out[0].Name != "a" {
		t.Errorf("name=%s", out[0].Name)
	}
}

func TestGetPDPProvidersByIDs_EmptyInput(t *testing.T) {
	s, _ := newTestService(t)
	out, err := s.GetPDPProvidersByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty non-nil slice, got %v (nil=%v)", out, out == nil)
	}
}

func TestGetPDPProvidersByIDs_AllSuccess(t *testing.T) {
	s, mc := newTestService(t)
	mc.setHandler(t, "getProviderWithProduct", func(args []any) ([]byte, error) {
		providerID := args[0].(*big.Int)
		switch providerID.Int64() {
		case 1:
			return mc.pack(t, "getProviderWithProduct", pdpProviderFixture(1, common.HexToAddress("0x01"), "alpha")), nil
		case 2:
			return mc.pack(t, "getProviderWithProduct", pdpProviderFixture(2, common.HexToAddress("0x02"), "beta")), nil
		default:
			return nil, errors.New("unexpected provider id")
		}
	})
	out, err := s.GetPDPProvidersByIDs(context.Background(), []types.BigInt{types.NewBigInt(1), types.NewBigInt(2)})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d providers=%+v", len(out), out)
	}
	if out[0].Info.Name != "alpha" || out[1].Info.Name != "beta" {
		t.Fatalf("providers out of order: %+v", out)
	}
}

func TestGetPDPProvidersByIDs_SkipsFailedAndEmptyProviders(t *testing.T) {
	s, mc := newTestService(t)
	mc.setHandler(t, "getProviderWithProduct", func(args []any) ([]byte, error) {
		providerID := args[0].(*big.Int)
		switch providerID.Int64() {
		case 1:
			return mc.pack(t, "getProviderWithProduct", pdpProviderFixture(1, common.HexToAddress("0x01"), "alpha")), nil
		case 2:
			return nil, errors.New("subcall failed")
		case 3:
			return mc.pack(t, "getProviderWithProduct", sprbind.ServiceProviderRegistryStorageProviderWithProduct{
				ProviderId: big.NewInt(3),
			}), nil
		case 4:
			return mc.pack(t, "getProviderWithProduct", pdpProviderFixture(4, common.HexToAddress("0x04"), "delta")), nil
		default:
			return nil, errors.New("unexpected provider id")
		}
	})
	out, err := s.GetPDPProvidersByIDs(context.Background(), []types.BigInt{
		types.NewBigInt(1),
		types.NewBigInt(2),
		types.NewBigInt(3),
		types.NewBigInt(4),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 {
		t.Fatalf("len=%d providers=%+v", len(out), out)
	}
	if out[0].Info.Name != "alpha" || out[1].Info.Name != "delta" {
		t.Fatalf("providers out of order: %+v", out)
	}
}

func TestGetPDPProvidersByIDs_DecodeError(t *testing.T) {
	s, mc := newTestService(t)
	mc.setHandler(t, "getProviderWithProduct", func(_ []any) ([]byte, error) {
		return []byte{0x01}, nil
	})
	_, err := s.GetPDPProvidersByIDs(context.Background(), []types.BigInt{types.NewBigInt(1)})
	if err == nil {
		t.Fatal("expected decode error")
	}
}

func TestGetPDPProvidersByIDs_EmptySuccessfulReturnData(t *testing.T) {
	s, mc := newTestService(t)
	mc.multicallFn = func(_ []byte) ([]byte, error) {
		type result3 struct {
			Success    bool
			ReturnData []byte
		}
		return mc.multicallABI.Methods["aggregate3"].Outputs.Pack([]result3{{Success: true}})
	}
	_, err := s.GetPDPProvidersByIDs(context.Background(), []types.BigInt{types.NewBigInt(1)})
	if err == nil || !strings.Contains(err.Error(), "empty return data") {
		t.Fatalf("expected empty return data error, got %v", err)
	}
}

func TestGetPDPProvidersByIDs_MalformedAggregateLength(t *testing.T) {
	s, mc := newTestService(t)
	mc.multicallFn = func(_ []byte) ([]byte, error) {
		type result3 struct {
			Success    bool
			ReturnData []byte
		}
		return mc.multicallABI.Methods["aggregate3"].Outputs.Pack([]result3{})
	}
	_, err := s.GetPDPProvidersByIDs(context.Background(), []types.BigInt{types.NewBigInt(1)})
	if err == nil || !strings.Contains(err.Error(), "expected 1 results") {
		t.Fatalf("expected malformed length error, got %v", err)
	}
}

func TestCapabilitiesListToMap(t *testing.T) {
	m := CapabilitiesListToMap([]string{"a", "b", "c"}, [][]byte{{0x01}, {0x02}})
	if len(m) != 2 {
		t.Fatalf("len=%d", len(m))
	}
}

func TestDecodePDPOffering_IPNIPeerID(t *testing.T) {
	peerBytes := []byte{0xde, 0xad, 0xbe, 0xef}
	caps := map[string][]byte{
		CapServiceURL:       []byte("https://x"),
		CapMinProvingPeriod: big.NewInt(1).Bytes(),
		CapStoragePrice:     big.NewInt(1).Bytes(),
		CapIPNIPeerID:       peerBytes,
		"extraKey":          []byte("extraVal"),
	}
	off, err := DecodePDPOffering(caps)
	if err != nil {
		t.Fatal(err)
	}
	if off.IPNIPeerID == "" {
		t.Error("peerID should be set")
	}
	if off.IPNIPeerID[0] != 'z' {
		t.Fatalf("peerID should keep multibase prefix, got %q", off.IPNIPeerID)
	}
	if off.ExtraCapabilities["extraKey"] == nil {
		t.Error("extra capability should be preserved")
	}
	if err := ValidatePDPOffering(off); err != nil {
		t.Errorf("unexpected validation error: %v", err)
	}
}

func TestValidatePDPOffering_Errors(t *testing.T) {
	err := ValidatePDPOffering(PDPOffering{})
	if err == nil || !errors.Is(err, ErrInvalidOffering) {
		t.Errorf("expected ErrInvalidOffering on empty, got %v", err)
	}
}

func TestGetProvidersByIDs_EmptyInput(t *testing.T) {
	s, _ := newTestService(t)
	out, err := s.GetProvidersByIDs(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Fatalf("expected empty non-nil slice, got %v (nil=%v)", out, out == nil)
	}
}

func TestGetProvidersByIDs_MalformedResponse(t *testing.T) {
	s, mc := newTestService(t)
	// Contract is supposed to return arrays of equal length to the request,
	// but we simulate a truncated response (len(ValidIds)=1 for 2 requested
	// ids) to verify the service rejects it instead of silently dropping
	// entries.
	mc.set(t, "getProvidersByIds",
		[]sprbind.ServiceProviderRegistryServiceProviderInfoView{
			{ProviderId: big.NewInt(1), Info: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{ServiceProvider: common.HexToAddress("0x01"), Name: "a", IsActive: true}},
		},
		[]bool{true},
	)
	_, err := s.GetProvidersByIDs(context.Background(), []types.BigInt{types.NewBigInt(1), types.NewBigInt(2)})
	if err == nil {
		t.Fatal("expected error for malformed response, got nil")
	}
}

// --- SelectActivePDPProviders tests ---

// buildPaginatedRaw builds a mock getProvidersByProductType response with n providers
// starting from startID, each using the given keys/values.
func buildPaginatedRaw(startID int64, n int, keys []string, vals [][]byte, hasMore bool) sprbind.ServiceProviderRegistryStoragePaginatedProviders {
	providers := make([]sprbind.ServiceProviderRegistryStorageProviderWithProduct, n)
	for i := 0; i < n; i++ {
		providers[i] = sprbind.ServiceProviderRegistryStorageProviderWithProduct{
			ProviderId: big.NewInt(startID + int64(i)),
			ProviderInfo: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
				ServiceProvider: common.HexToAddress("0x01"),
				Name:            "sp",
				IsActive:        true,
			},
			Product: sprbind.ServiceProviderRegistryStorageServiceProduct{
				ProductType:    uint8(ProductTypePDP),
				CapabilityKeys: keys,
				IsActive:       true,
			},
			ProductCapabilityValues: vals,
		}
	}
	return sprbind.ServiceProviderRegistryStoragePaginatedProviders{Providers: providers, HasMore: hasMore}
}

func TestSelectActivePDPProviders_NoFilter(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	mc.set(t, "getProvidersByProductType", buildPaginatedRaw(3, 2, keys, vals, false))

	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(got))
	}
	// Results must be sorted by ID ascending.
	if got[0].Info.ID.Cmp(got[1].Info.ID) >= 0 {
		t.Errorf("expected ascending ID order, got %v %v", got[0].Info.ID, got[1].Info.ID)
	}
}

func TestSelectActivePDPProviders_SortedByID(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	// Build providers with IDs in non-sorted order: 5, 2, 8.
	raw := sprbind.ServiceProviderRegistryStoragePaginatedProviders{
		Providers: []sprbind.ServiceProviderRegistryStorageProviderWithProduct{
			{
				ProviderId:              big.NewInt(5),
				ProviderInfo:            sprbind.ServiceProviderRegistryStorageServiceProviderInfo{ServiceProvider: common.HexToAddress("0x05"), IsActive: true},
				Product:                 sprbind.ServiceProviderRegistryStorageServiceProduct{ProductType: 0, CapabilityKeys: keys, IsActive: true},
				ProductCapabilityValues: vals,
			},
			{
				ProviderId:              big.NewInt(2),
				ProviderInfo:            sprbind.ServiceProviderRegistryStorageServiceProviderInfo{ServiceProvider: common.HexToAddress("0x02"), IsActive: true},
				Product:                 sprbind.ServiceProviderRegistryStorageServiceProduct{ProductType: 0, CapabilityKeys: keys, IsActive: true},
				ProductCapabilityValues: vals,
			},
			{
				ProviderId:              big.NewInt(8),
				ProviderInfo:            sprbind.ServiceProviderRegistryStorageServiceProviderInfo{ServiceProvider: common.HexToAddress("0x08"), IsActive: true},
				Product:                 sprbind.ServiceProviderRegistryStorageServiceProduct{ProductType: 0, CapabilityKeys: keys, IsActive: true},
				ProductCapabilityValues: vals,
			},
		},
		HasMore: false,
	}
	mc.set(t, "getProvidersByProductType", raw)

	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	ids := []types.BigInt{got[0].Info.ID, got[1].Info.ID, got[2].Info.ID}
	if !ids[0].Equal(types.NewBigInt(2)) || !ids[1].Equal(types.NewBigInt(5)) || !ids[2].Equal(types.NewBigInt(8)) {
		t.Errorf("expected [2 5 8], got %v", ids)
	}
}

func TestSelectActivePDPProviders_FilterByPieceSize(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	// pdpCapsFixture: minPieceSize=1024, maxPieceSize=1<<30.
	mc.set(t, "getProvidersByProductType", buildPaginatedRaw(1, 2, keys, vals, false))

	// Request a piece size that fits within the provider range.
	fit := big.NewInt(4096)
	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{PieceSizeBytes: fit})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 (fit), got %d", len(got))
	}

	// Request a piece size that is too small (below minPieceSize).
	tooSmall := big.NewInt(512)
	got, err = s.SelectActivePDPProviders(context.Background(), ProviderFilter{PieceSizeBytes: tooSmall})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 (too small), got %d", len(got))
	}

	// Request a piece size that is too large (above maxPieceSize).
	tooBig := new(big.Int).Add(big.NewInt(1<<30), big.NewInt(1))
	got, err = s.SelectActivePDPProviders(context.Background(), ProviderFilter{PieceSizeBytes: tooBig})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 (too big), got %d", len(got))
	}
}

func TestSelectActivePDPProviders_FilterByPaymentToken(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	// pdpCapsFixture uses token 0xb3042734b608a1B16e9e86B374A3f3e389B4cDf0.
	mc.set(t, "getProvidersByProductType", buildPaginatedRaw(1, 1, keys, vals, false))

	wantToken := common.HexToAddress("0xb3042734b608a1B16e9e86B374A3f3e389B4cDf0")
	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{PaymentToken: addrPtr(wantToken)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 (matching token), got %d", len(got))
	}

	otherToken := common.HexToAddress("0xdead")
	got, err = s.SelectActivePDPProviders(context.Background(), ProviderFilter{PaymentToken: addrPtr(otherToken)})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 (non-matching token), got %d", len(got))
	}
}

func TestSelectActivePDPProviders_FilterByFILToken(t *testing.T) {
	s, mc := newTestService(t)
	usdfcKeys, usdfcVals := pdpCapsFixture()
	filKeys, filVals := pdpCapsFixtureWithToken(common.Address{})
	raw := sprbind.ServiceProviderRegistryStoragePaginatedProviders{
		Providers: []sprbind.ServiceProviderRegistryStorageProviderWithProduct{
			{
				ProviderId:              big.NewInt(1),
				ProviderInfo:            sprbind.ServiceProviderRegistryStorageServiceProviderInfo{ServiceProvider: common.HexToAddress("0x01"), Name: "fil", IsActive: true},
				Product:                 sprbind.ServiceProviderRegistryStorageServiceProduct{ProductType: 0, CapabilityKeys: filKeys, IsActive: true},
				ProductCapabilityValues: filVals,
			},
			{
				ProviderId:              big.NewInt(2),
				ProviderInfo:            sprbind.ServiceProviderRegistryStorageServiceProviderInfo{ServiceProvider: common.HexToAddress("0x02"), Name: "usdfc", IsActive: true},
				Product:                 sprbind.ServiceProviderRegistryStorageServiceProduct{ProductType: 0, CapabilityKeys: usdfcKeys, IsActive: true},
				ProductCapabilityValues: usdfcVals,
			},
		},
		HasMore: false,
	}
	mc.set(t, "getProvidersByProductType", raw)

	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{PaymentToken: addrPtr(common.Address{})})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Info.ID.Equal(types.NewBigInt(1)) {
		t.Fatalf("expected only FIL provider, got %+v", got)
	}
}

func TestSelectActivePDPProviders_ExcludeIDs(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	// Providers with IDs 1, 2, 3.
	mc.set(t, "getProvidersByProductType", buildPaginatedRaw(1, 3, keys, vals, false))

	excluded := []types.BigInt{types.NewBigInt(2)}
	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{ExcludeIDs: excluded})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 after exclusion, got %d", len(got))
	}
	for _, p := range got {
		if p.Info.ID.Equal(types.NewBigInt(2)) {
			t.Errorf("excluded provider ID 2 still present")
		}
	}
}

func TestSelectActivePDPProviders_Pagination(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()

	// The mock always returns the same reply regardless of offset; we simulate
	// pagination by having page 1 with hasMore=true and page 2 with hasMore=false.
	// We achieve this by using a custom reply sequence via a counter.
	callCount := 0
	mc.argCheck = func(method string, args []any) {
		if method == "getProvidersByProductType" {
			callCount++
		}
	}

	page1 := buildPaginatedRaw(1, 2, keys, vals, true)
	page2 := buildPaginatedRaw(3, 2, keys, vals, false)

	// We swap the reply after the first call by hooking into argCheck.
	// Set page1 first; argCheck will switch to page2 on second call.
	mc.set(t, "getProvidersByProductType", page1)
	origArgCheck := mc.argCheck
	mc.argCheck = func(method string, args []any) {
		origArgCheck(method, args)
		if method == "getProvidersByProductType" && callCount >= 2 {
			mc.set(t, "getProvidersByProductType", page2)
		}
	}

	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	// Should have fetched both pages (4 total), sorted by ID.
	if len(got) != 4 {
		t.Fatalf("expected 4 providers across 2 pages, got %d", len(got))
	}
}

func TestSelectActivePDPProviders_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getProvidersByProductType"] = errors.New("rpc timeout")

	_, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{})
	if err == nil {
		t.Fatal("expected RPC error to propagate")
	}
}

func TestSelectActivePDPProviders_EmptyResult(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProvidersByProductType", sprbind.ServiceProviderRegistryStoragePaginatedProviders{
		Providers: []sprbind.ServiceProviderRegistryStorageProviderWithProduct{},
		HasMore:   false,
	})
	got, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d", len(got))
	}
}

func TestSelectActivePDPProviders_PaginationCapExceeded(t *testing.T) {
	s, mc := newTestService(t)
	keys, vals := pdpCapsFixture()
	// Always return hasMore=true so the loop would run forever without the cap.
	mc.set(t, "getProvidersByProductType", buildPaginatedRaw(1, 1, keys, vals, true))

	_, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{})
	if err == nil {
		t.Fatal("expected error when pagination cap is exceeded")
	}
	// Verify the error message mentions pagination.
	if !containsString(err.Error(), "pagination exceeded") {
		t.Errorf("error should mention pagination cap, got: %v", err)
	}
}

func TestSelectActivePDPProviders_ZeroIDFailsHard(t *testing.T) {
	keys, vals := pdpCapsFixture()
	s, mc := newTestService(t)
	mc.set(t, "getProvidersByProductType", sprbind.ServiceProviderRegistryStoragePaginatedProviders{
		Providers: []sprbind.ServiceProviderRegistryStorageProviderWithProduct{
			{
				ProviderId: big.NewInt(0),
				ProviderInfo: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
					ServiceProvider: common.HexToAddress("0x01"),
					IsActive:        true,
				},
				Product: sprbind.ServiceProviderRegistryStorageServiceProduct{
					ProductType:    uint8(ProductTypePDP),
					CapabilityKeys: keys,
					IsActive:       true,
				},
				ProductCapabilityValues: vals,
			},
		},
		HasMore: false,
	})

	_, err := s.SelectActivePDPProviders(context.Background(), ProviderFilter{})
	if err == nil {
		t.Fatal("expected error for zero-ID provider entry")
	}
}

// containsString is a simple substring helper to avoid importing strings in test file.
func containsString(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}

// ---------------------------------------------------------------------------
// Address getter test
// ---------------------------------------------------------------------------

func TestAddress(t *testing.T) {
	s, _ := newTestService(t)
	want := common.HexToAddress("0xabcd")
	if got := s.Address(); got != want {
		t.Errorf("Address() = %s, want %s", got, want)
	}
}

// ---------------------------------------------------------------------------
// GetProviderByAddress tests
// ---------------------------------------------------------------------------

func TestGetProviderByAddress_ZeroAddress(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.GetProviderByAddress(context.Background(), common.Address{})
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument for zero address, got %v", err)
	}
}

func TestGetProviderByAddress_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getProviderByAddress"] = errors.New("rpc error")
	_, err := s.GetProviderByAddress(context.Background(), common.HexToAddress("0x11"))
	if err == nil {
		t.Error("expected RPC error")
	}
}

func TestGetProviderByAddress_EmptyReturnsNotFound(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderByAddress", sprbind.ServiceProviderRegistryServiceProviderInfoView{
		ProviderId: big.NewInt(0),
		Info:       sprbind.ServiceProviderRegistryStorageServiceProviderInfo{},
	})
	got, err := s.GetProviderByAddress(context.Background(), common.HexToAddress("0x99"))
	if err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got err=%v result=%+v", err, got)
	}
	if got != nil {
		t.Errorf("expected nil result with ErrNotFound, got %+v", got)
	}
}

func TestGetProviderByAddress_ValidProvider(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "getProviderByAddress", sprbind.ServiceProviderRegistryServiceProviderInfoView{
		ProviderId: big.NewInt(5),
		Info: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
			ServiceProvider: common.HexToAddress("0x55"),
			Payee:           common.HexToAddress("0x66"),
			Name:            "bob",
			Description:     "test",
			IsActive:        true,
		},
	})
	got, err := s.GetProviderByAddress(context.Background(), common.HexToAddress("0x55"))
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "bob" || !got.ID.Equal(types.NewBigInt(5)) {
		t.Errorf("got=%+v", got)
	}
}

// ---------------------------------------------------------------------------
// IsProviderActive edge cases
// ---------------------------------------------------------------------------

func TestIsProviderActive_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["isProviderActive"] = errors.New("rpc error")
	_, err := s.IsProviderActive(context.Background(), types.NewBigInt(1))
	if err == nil {
		t.Error("expected RPC error")
	}
}

func TestIsProviderActive_ReturnsFalse(t *testing.T) {
	s, mc := newTestService(t)
	mc.set(t, "isProviderActive", false)
	ok, err := s.IsProviderActive(context.Background(), types.NewBigInt(1))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("expected false")
	}
}

// ---------------------------------------------------------------------------
// GetProviderCount / GetActiveProviderCount error paths
// ---------------------------------------------------------------------------

func TestGetProviderCount_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["getProviderCount"] = errors.New("rpc error")
	_, err := s.GetProviderCount(context.Background())
	if err == nil {
		t.Error("expected RPC error")
	}
}

func TestGetActiveProviderCount_RPCError(t *testing.T) {
	s, mc := newTestService(t)
	mc.errs["activeProviderCount"] = errors.New("rpc error")
	_, err := s.GetActiveProviderCount(context.Background())
	if err == nil {
		t.Error("expected RPC error")
	}
}
