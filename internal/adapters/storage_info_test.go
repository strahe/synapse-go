package adapters

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	fwssbind "github.com/strahe/synapse-go/internal/contracts/fwss"
	fwssviewbind "github.com/strahe/synapse-go/internal/contracts/fwssview"
	pdpverifierbind "github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	sprbind "github.com/strahe/synapse-go/internal/contracts/spregistry"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestStorageInfoReader_GetStorageInfo_UsesValidApprovedProviderPagination(t *testing.T) {
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

	got, err := (&storageInfoReader{ws: ws}).GetStorageInfo(context.Background(), common.Address{})
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

func TestStorageInfoReader_GetStorageInfo_ReturnsPartialProvidersAndError(t *testing.T) {
	ws, mc := newStorageInfoTestWarmStorage(t)
	sp := newStorageInfoTestSPRegistry(t, mc)

	mc.setFWSSReply(t, "getServicePrice", fwssbind.FilecoinWarmStorageServiceServicePricing{
		PricePerTiBPerMonthNoCDN:   big.NewInt(1000),
		PricePerTiBCdnEgress:       big.NewInt(20),
		PricePerTiBCacheMissEgress: big.NewInt(30),
		TokenAddress:               common.HexToAddress("0xabc"),
		EpochsPerMonth:             big.NewInt(2880),
		MinimumPricePerMonth:       big.NewInt(0),
	})
	mc.setViewReply(t, "getApprovedProviders", []*big.Int{big.NewInt(1), big.NewInt(2)})
	mc.setSPResponder(t, "getProviderWithProduct", func(args []any) ([]byte, error) {
		providerID, ok := args[0].(*big.Int)
		if !ok {
			t.Fatalf("providerID arg type = %T, want *big.Int", args[0])
		}
		switch providerID.Uint64() {
		case 1:
			return mc.packSP(t, "getProviderWithProduct", storageInfoPDPProviderView(1, common.HexToAddress("0x1111")))
		case 2:
			return nil, errors.New("boom")
		default:
			t.Fatalf("unexpected providerID %d", providerID.Uint64())
			return nil, nil
		}
	})

	got, err := (&storageInfoReader{ws: ws, sp: sp}).GetStorageInfo(context.Background(), common.Address{})
	if err == nil {
		t.Fatal("GetStorageInfo error = nil, want joined partial error")
	}
	if got == nil {
		t.Fatal("GetStorageInfo result = nil, want partial result")
	}
	if len(got.Providers) != 1 || got.Providers[0].Info.ID != 1 {
		t.Fatalf("Providers = %+v, want only provider 1", got.Providers)
	}
	if got.Pricing.NoCDN.PerMonth == nil || got.Pricing.NoCDN.PerMonth.Cmp(big.NewInt(1000)) != 0 {
		t.Fatalf("Pricing.NoCDN.PerMonth = %v, want 1000", got.Pricing.NoCDN.PerMonth)
	}
	if !strings.Contains(err.Error(), "GetPDPProvider(2)") {
		t.Fatalf("err = %v, want GetPDPProvider(2)", err)
	}
}

type storageInfoTestCaller struct {
	fwssABI      abi.ABI
	viewABI      abi.ABI
	pdpABI       abi.ABI
	spABI        abi.ABI
	mu           sync.Mutex
	replies      map[string][]byte
	errs         map[string]error
	lastIn       map[string][]byte
	spResponders map[string]func([]any) ([]byte, error)
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
	sABI, err := sprbind.SPRegistryMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	return &storageInfoTestCaller{
		fwssABI:      *fABI,
		viewABI:      *vABI,
		pdpABI:       *pABI,
		spABI:        *sABI,
		replies:      map[string][]byte{},
		errs:         map[string]error{},
		lastIn:       map[string][]byte{},
		spResponders: map[string]func([]any) ([]byte, error){},
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
	selector := [4]byte{data[0], data[1], data[2], data[3]}
	for name, method := range m.fwssABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			reply := m.replies[name]
			err := m.errs[name]
			m.mu.Unlock()
			if err != nil {
				return nil, err
			}
			return reply, nil
		}
	}
	for name, method := range m.viewABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			reply := m.replies[name]
			err := m.errs[name]
			m.mu.Unlock()
			if err != nil {
				return nil, err
			}
			return reply, nil
		}
	}
	for name, method := range m.pdpABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			reply := m.replies[name]
			err := m.errs[name]
			m.mu.Unlock()
			if err != nil {
				return nil, err
			}
			return reply, nil
		}
	}
	for name, method := range m.spABI.Methods {
		if [4]byte(method.ID) == selector {
			m.lastIn[name] = data
			args, err := method.Inputs.Unpack(data[4:])
			if err != nil {
				m.mu.Unlock()
				return nil, err
			}
			responder := m.spResponders[name]
			reply := m.replies[name]
			callErr := m.errs[name]
			m.mu.Unlock()
			if responder != nil {
				return responder(args)
			}
			if callErr != nil {
				return nil, callErr
			}
			return reply, nil
		}
	}
	m.mu.Unlock()
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

func (m *storageInfoTestCaller) setSPResponder(t *testing.T, method string, responder func([]any) ([]byte, error)) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.spABI.Methods[method]; !ok {
		t.Fatalf("sp method %q not found", method)
	}
	m.spResponders[method] = responder
}

func (m *storageInfoTestCaller) packSP(t *testing.T, method string, values ...any) ([]byte, error) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	mth, ok := m.spABI.Methods[method]
	if !ok {
		t.Fatalf("sp method %q not found", method)
	}
	return mth.Outputs.Pack(values...)
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

func newStorageInfoTestSPRegistry(t *testing.T, mc *storageInfoTestCaller) *spregistry.Service {
	t.Helper()
	sp, err := spregistry.New(spregistry.Options{
		Client:  mc,
		Address: common.HexToAddress("0x3333333333333333333333333333333333333333"),
	})
	if err != nil {
		t.Fatalf("spregistry.New: %v", err)
	}
	return sp
}

func storageInfoPDPProviderView(id int64, serviceProvider common.Address) sprbind.ServiceProviderRegistryStorageProviderWithProduct {
	return sprbind.ServiceProviderRegistryStorageProviderWithProduct{
		ProviderId: big.NewInt(id),
		ProviderInfo: sprbind.ServiceProviderRegistryStorageServiceProviderInfo{
			ServiceProvider: serviceProvider,
			Payee:           common.HexToAddress("0xaaaa"),
			Name:            "provider",
			IsActive:        true,
		},
		Product: sprbind.ServiceProviderRegistryStorageServiceProduct{
			ProductType: uint8(spregistry.ProductTypePDP),
			CapabilityKeys: []string{
				spregistry.CapServiceURL,
				spregistry.CapMinPieceSize,
				spregistry.CapMaxPieceSize,
				spregistry.CapStoragePrice,
				spregistry.CapMinProvingPeriod,
				spregistry.CapLocation,
				spregistry.CapPaymentToken,
			},
			IsActive: true,
		},
		ProductCapabilityValues: [][]byte{
			[]byte("https://pdp.example.com"),
			big.NewInt(1024).Bytes(),
			big.NewInt(1 << 30).Bytes(),
			big.NewInt(1_000_000).Bytes(),
			big.NewInt(2880).Bytes(),
			[]byte("US-EAST"),
			common.HexToAddress("0xb3042734b608a1B16e9e86B374A3f3e389B4cDf0").Bytes(),
		},
	}
}
