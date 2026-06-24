package synapse

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/testutil"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
)

// testKey returns a random ECDSA private key for testing.
func testKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

//nolint:staticcheck // test constructs D storage to verify zeroPrivateKey clears it.
func testPrivateKeyWithScalar(scalar *big.Int) *ecdsa.PrivateKey {
	return &ecdsa.PrivateKey{D: scalar}
}

// jsonRPCReq is a minimal JSON-RPC request.
type jsonRPCReq struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type contractCallerFunc func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)

func (fn contractCallerFunc) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return fn(ctx, call, blockNumber)
}

// fakeRPCServer creates an httptest.Server that responds to eth_chainId and
// the FWSS address-resolution multicall. Returns the server (caller must
// Close) and an ethclient connected to it.
func fakeRPCServer(t *testing.T, chainIDHex string) (*httptest.Server, *ethclient.Client) {
	t.Helper()
	c := fakeRPCAddressChain(chainIDHex)
	a := c.Addresses()
	return fakeRPCServerWithResolvedAddresses(t, chainIDHex, ResolvedAddresses{
		FWSS:               a.FWSS,
		PDPVerifier:        a.PDPVerifier,
		SPRegistry:         a.SPRegistry,
		USDFC:              a.USDFC,
		Payments:           a.Payments,
		ViewContract:       a.StateView,
		SessionKeyRegistry: a.SessionKeyRegistry,
	})
}

func fakeRPCServerWithResolvedAddresses(t *testing.T, chainIDHex string, addresses ResolvedAddresses) (*httptest.Server, *ethclient.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json-rpc", http.StatusBadRequest)
			return
		}
		var result string
		switch req.Method {
		case "eth_chainId":
			result = fmt.Sprintf(`"%s"`, chainIDHex)
		case "eth_call":
			result = fmt.Sprintf("%q", testutil.FWSSAddressResolutionResultHexFor(t, chain.ContractAddresses{
				PDPVerifier:        addresses.PDPVerifier,
				SPRegistry:         addresses.SPRegistry,
				USDFC:              addresses.USDFC,
				Payments:           addresses.Payments,
				StateView:          addresses.ViewContract,
				SessionKeyRegistry: addresses.SessionKeyRegistry,
			}, addresses.FilBeamBeneficiary))
		default:
			result = "null"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, req.ID, result)
	}))
	ec, err := ethclient.Dial(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("dial fake RPC: %v", err)
	}
	return srv, ec
}

func testResolvedAddresses(c chain.Chain) ResolvedAddresses {
	return ResolvedAddresses{
		FWSS:               c.Addresses().FWSS,
		PDPVerifier:        common.HexToAddress("0x1000000000000000000000000000000000000001"),
		SPRegistry:         common.HexToAddress("0x1000000000000000000000000000000000000002"),
		USDFC:              common.HexToAddress("0x1000000000000000000000000000000000000003"),
		Payments:           common.HexToAddress("0x1000000000000000000000000000000000000004"),
		ViewContract:       common.HexToAddress("0x1000000000000000000000000000000000000005"),
		FilBeamBeneficiary: common.HexToAddress("0x1000000000000000000000000000000000000006"),
		SessionKeyRegistry: common.HexToAddress("0x1000000000000000000000000000000000000007"),
	}
}

func fakeRPCAddressChain(chainIDHex string) chain.Chain {
	switch strings.ToLower(chainIDHex) {
	case "0x13a":
		return chain.Mainnet
	default:
		return chain.Calibration
	}
}

func TestResolveAddresses(t *testing.T) {
	want := testResolvedAddresses(chain.Calibration)
	srv, ec := fakeRPCServerWithResolvedAddresses(t, "0x4cb2f", want)
	defer srv.Close()
	defer ec.Close()

	got, err := ResolveAddresses(context.Background(), ec, want.FWSS)
	if err != nil {
		t.Fatalf("ResolveAddresses: %v", err)
	}
	if got != want {
		t.Fatalf("addresses = %+v, want %+v", got, want)
	}
}

func TestResolveAddresses_InvalidArguments(t *testing.T) {
	validCaller := contractCallerFunc(func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error) {
		return nil, errors.New("unexpected call")
	})
	tests := []struct {
		name   string
		caller ContractCaller
		fwss   common.Address
	}{
		{name: "nil caller", fwss: testResolvedAddresses(chain.Calibration).FWSS},
		{name: "zero FWSS", caller: validCaller},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveAddresses(context.Background(), tc.caller, tc.fwss)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error = %v, want ErrInvalidArgument", err)
			}
			if !strings.HasPrefix(err.Error(), "synapse.ResolveAddresses:") {
				t.Fatalf("error = %v, want synapse.ResolveAddresses prefix", err)
			}
		})
	}
}

func TestResolveAddresses_PreservesContextCancellation(t *testing.T) {
	caller := contractCallerFunc(func(ctx context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ResolveAddresses(ctx, caller, testResolvedAddresses(chain.Calibration).FWSS)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}

func TestResolveAddresses_ValidatesResolvedAddresses(t *testing.T) {
	tests := []struct {
		name  string
		clear func(*ResolvedAddresses)
	}{
		{name: "SPRegistry", clear: func(a *ResolvedAddresses) { a.SPRegistry = common.Address{} }},
		{name: "USDFC", clear: func(a *ResolvedAddresses) { a.USDFC = common.Address{} }},
		{name: "Payments", clear: func(a *ResolvedAddresses) { a.Payments = common.Address{} }},
		{name: "ViewContract", clear: func(a *ResolvedAddresses) { a.ViewContract = common.Address{} }},
		{name: "SessionKeyRegistry", clear: func(a *ResolvedAddresses) { a.SessionKeyRegistry = common.Address{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			addresses := testResolvedAddresses(chain.Calibration)
			tc.clear(&addresses)
			srv, ec := fakeRPCServerWithResolvedAddresses(t, "0x4cb2f", addresses)
			defer srv.Close()
			defer ec.Close()

			got, err := ResolveAddresses(context.Background(), ec, addresses.FWSS)
			if err == nil || !strings.Contains(err.Error(), tc.name+" returned zero address") {
				t.Fatalf("error = %v, want zero %s error", err, tc.name)
			}
			if got != (ResolvedAddresses{}) {
				t.Fatalf("addresses = %+v, want zero result", got)
			}
		})
	}

	addresses := testResolvedAddresses(chain.Calibration)
	addresses.PDPVerifier = common.Address{}
	addresses.FilBeamBeneficiary = common.Address{}
	srv, ec := fakeRPCServerWithResolvedAddresses(t, "0x4cb2f", addresses)
	defer srv.Close()
	defer ec.Close()
	if _, err := ResolveAddresses(context.Background(), ec, addresses.FWSS); err != nil {
		t.Fatalf("optional zero addresses: %v", err)
	}
}

func TestNew_WithEthClient_Calibration(t *testing.T) {
	wantAddresses := testResolvedAddresses(chain.Calibration)
	srv, ec := fakeRPCServerWithResolvedAddresses(t, "0x4cb2f", wantAddresses) // Calibration chain ID = 314159
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.Chain() != chain.Calibration {
		t.Errorf("chain = %v, want Calibration", client.Chain())
	}
	want := ethcrypto.PubkeyToAddress(key.PublicKey)
	if client.Address() != want {
		t.Errorf("address = %v, want %v", client.Address(), want)
	}
	if got := client.ResolvedAddresses(); got != wantAddresses {
		t.Fatalf("ResolvedAddresses = %+v, want %+v", got, wantAddresses)
	}
	if got := client.WarmStorage().FWSSAddress(); got != wantAddresses.FWSS {
		t.Errorf("WarmStorage FWSS = %s, want %s", got, wantAddresses.FWSS)
	}
	if got := client.WarmStorage().ViewAddress(); got != wantAddresses.ViewContract {
		t.Errorf("WarmStorage view = %s, want %s", got, wantAddresses.ViewContract)
	}
	if got := client.WarmStorage().PDPVerifierAddress(); got != wantAddresses.PDPVerifier {
		t.Errorf("WarmStorage PDPVerifier = %s, want %s", got, wantAddresses.PDPVerifier)
	}
	if got := client.SPRegistry().Address(); got != wantAddresses.SPRegistry {
		t.Errorf("SPRegistry = %s, want %s", got, wantAddresses.SPRegistry)
	}
	if got := client.Payments().Address(); got != wantAddresses.Payments {
		t.Errorf("Payments = %s, want %s", got, wantAddresses.Payments)
	}
	if got := client.SessionKey().RegistryAddress(); got != wantAddresses.SessionKeyRegistry {
		t.Errorf("SessionKeyRegistry = %s, want %s", got, wantAddresses.SessionKeyRegistry)
	}
}

func TestNew_WithRPCURL(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithRPCURL(srv.URL),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.Chain() != chain.Calibration {
		t.Errorf("chain = %v, want Calibration", client.Chain())
	}
}

func TestNew_WithChain_SkipsDetection(t *testing.T) {
	// Provide a chain explicitly — no RPC call for chain ID.
	srv, ec := fakeRPCServer(t, "0xdeadbeef") // bogus chain ID
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	cal := chain.Calibration
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithChain(cal),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.Chain() != chain.Calibration {
		t.Errorf("chain = %v, want Calibration", client.Chain())
	}
}

func TestNew_WithPrivateKeyHex(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	hexKey := fmt.Sprintf("0x%x", ethcrypto.FromECDSA(key))

	client, err := New(context.Background(),
		WithPrivateKeyHex(hexKey),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	want := ethcrypto.PubkeyToAddress(key.PublicKey)
	if client.Address() != want {
		t.Errorf("address = %v, want %v", client.Address(), want)
	}
}

func TestNew_MissingKey(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	_, err := New(context.Background(), WithEthClient(ec))
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
}

func TestNew_DefaultsToCalibrationRPC(t *testing.T) {
	key := testKey(t)
	client, err := New(context.Background(), WithPrivateKey(key))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.Chain() != chain.Calibration {
		t.Errorf("chain = %v, want Calibration", client.Chain())
	}
}

func TestNew_WithChainDefaultsToChainRPC(t *testing.T) {
	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithChain(chain.Mainnet),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.Chain() != chain.Mainnet {
		t.Errorf("chain = %v, want Mainnet", client.Chain())
	}
}

func TestNew_MissingRPCSourceForChainWithoutDefault(t *testing.T) {
	key := testKey(t)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithChain(chain.Chain(255)),
	)
	if err == nil {
		t.Fatal("expected error for missing RPC source")
	}
	if !strings.Contains(err.Error(), "missing RPC source") {
		t.Fatalf("error = %v, want missing RPC source", err)
	}
}

func TestNew_UnsupportedChain(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x1") // Ethereum mainnet — not supported
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err == nil {
		t.Fatal("expected error for unsupported chain")
	}
}

func TestClose_OwnedClient(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithRPCURL(srv.URL),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Close should not error for owned client.
	if err := client.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestClose_BorrowedClient(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Close must NOT close the borrowed ethclient.
	if err := client.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	// ec should still be usable — verify by calling ChainID.
	_, err = ec.ChainID(context.Background())
	if err != nil {
		t.Errorf("borrowed client unusable after Close: %v", err)
	}
	ec.Close()
}

func TestServiceGetters_Exist(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Verify getters return non-nil.
	// These are purely in-memory constructors, no RPC calls.
	if client.WarmStorage() == nil {
		t.Error("WarmStorage() returned nil")
	}
	if client.SPRegistry() == nil {
		t.Error("SPRegistry() returned nil")
	}
	if client.Payments() == nil {
		t.Error("Payments() returned nil")
	}
	if client.SessionKey() == nil {
		t.Error("SessionKey() returned nil")
	}
	if client.Costs() == nil {
		t.Error("Costs() returned nil")
	}
	if client.FilBeam() == nil {
		t.Error("FilBeam() returned nil")
	}
	if client.Storage() == nil {
		t.Error("Storage() returned nil")
	}
}

func TestClose_AllServicesReturnErrClosed(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	ctx := context.Background()
	addr := client.Address()

	// After Close every service method must fail with ErrClosed.
	// Use a read method that exercises lifecycle before any other I/O.
	if _, err := client.Payments().Balance(ctx, addr, addr); !errors.Is(err, ErrClosed) {
		t.Errorf("Payments.Balance: got %v, want ErrClosed", err)
	}
	if _, err := client.WarmStorage().GetServicePrice(ctx); !errors.Is(err, ErrClosed) { //nolint:staticcheck // Deprecated compatibility method must honor Close.
		t.Errorf("WarmStorage.GetServicePrice: got %v, want ErrClosed", err)
	}
	if _, err := client.WarmStorage().GetPriceList(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("WarmStorage.GetPriceList: got %v, want ErrClosed", err)
	}
	if _, err := client.SPRegistry().GetProviderIDByAddress(ctx, addr); !errors.Is(err, ErrClosed) {
		t.Errorf("SPRegistry.GetProviderIDByAddress: got %v, want ErrClosed", err)
	}
	if _, err := client.SessionKey().Login(ctx, addr, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("SessionKey.Login: got %v, want ErrClosed", err)
	}
	if _, err := client.Costs().GetServicePrice(ctx); !errors.Is(err, ErrClosed) { //nolint:staticcheck // Deprecated compatibility method must honor Close.
		t.Errorf("Costs.GetServicePrice: got %v, want ErrClosed", err)
	}
	if _, err := client.Costs().GetPriceList(ctx); !errors.Is(err, ErrClosed) {
		t.Errorf("Costs.GetPriceList: got %v, want ErrClosed", err)
	}
	if _, err := client.FilBeam().GetDataSetStats(ctx, types.NewBigInt(1)); !errors.Is(err, ErrClosed) {
		t.Errorf("FilBeam.GetDataSetStats: got %v, want ErrClosed", err)
	}
	if _, err := client.Storage().Download(ctx, cid.Undef, nil); !errors.Is(err, ErrClosed) {
		t.Errorf("Storage.Download: got %v, want ErrClosed", err)
	}
}

func TestServiceGetters_Idempotent(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Calling twice should return the same pointer (plain field read, idempotent by construction).
	ws1 := client.WarmStorage()
	ws2 := client.WarmStorage()
	if ws1 != ws2 {
		t.Error("WarmStorage() not idempotent")
	}

	fb1 := client.FilBeam()
	fb2 := client.FilBeam()
	if fb1 != fb2 {
		t.Error("FilBeam() not idempotent")
	}

	st1 := client.Storage()
	st2 := client.Storage()
	if st1 != st2 {
		t.Error("Storage() not idempotent")
	}
}

func TestNew_WithLogger(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	logger := newTestLogger()

	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithLogger(logger),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Logger gets passed to sub-services. Verify Payments gets it.
	_ = client.Payments()
}

func TestNew_WithHTTPClient(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	seenRequests := map[string]bool{}
	hc := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		seenRequests[req.URL.Host+req.URL.Path] = true
		var body io.ReadCloser = http.NoBody
		if req.URL.Host == "calibration.stats.filbeam.com" {
			body = io.NopCloser(strings.NewReader(`{"cdnEgressQuota":"1","cacheMissEgressQuota":"2"}`))
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
	})}

	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithHTTPClient(hc),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	providerClient, err := client.newPDPClient("https://provider.example")
	if err != nil {
		t.Fatalf("newPDPClient: %v", err)
	}
	if err := providerClient.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if !seenRequests["provider.example/pdp/ping"] {
		t.Fatalf("custom HTTP client did not see provider ping request: %v", seenRequests)
	}

	if _, err := client.FilBeam().GetDataSetStats(context.Background(), types.NewBigInt(123)); err != nil {
		t.Fatalf("GetDataSetStats: %v", err)
	}
	if !seenRequests["calibration.stats.filbeam.com/data-set/123"] {
		t.Fatalf("custom HTTP client did not see filbeam request: %v", seenRequests)
	}
	if client.Storage() == nil {
		t.Fatal("Storage() returned nil")
	}
}

func TestNew_WithFilBeamRetrievalDomain(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	data := bytes.Repeat([]byte("domain"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var gotHost string
	hc := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		gotHost = req.URL.Host
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(data)),
			Header:     make(http.Header),
			Request:    req,
		}, nil
	})}

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithHTTPClient(hc),
		WithFilBeamRetrievalDomain("staging.filbeam.example"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	retriever, err := client.FilBeam().NewRetriever(client.Address())
	if err != nil {
		t.Fatalf("NewRetriever: %v", err)
	}
	rc, err := retriever.DownloadPiece(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("DownloadPiece: %v", err)
	}
	_ = rc.Close()

	wantHost := strings.ToLower(client.Address().Hex()) + ".staging.filbeam.example"
	if gotHost != wantHost {
		t.Fatalf("host=%q want %q", gotHost, wantHost)
	}
}

func TestNew_Mainnet(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x13a") // Filecoin mainnet = 314
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.Chain() != chain.Mainnet {
		t.Errorf("chain = %v, want Mainnet", client.Chain())
	}
}

// newTestLogger returns a discard logger for tests.
func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWithSource(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithSource("my-app"),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	if client.source != "my-app" {
		t.Errorf("source = %q, want %q", client.source, "my-app")
	}
	// Storage() returns the manager; verify it works with source set.
	_ = client.Storage()
}

func TestNew_InvalidPrivateKeyHex(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	_, err := New(context.Background(),
		WithPrivateKeyHex("not-valid-hex"),
		WithEthClient(ec),
	)
	if err == nil {
		t.Fatal("expected error for invalid hex key")
	}
}

func TestNew_ShortPrivateKeyHex(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	_, err := New(context.Background(),
		WithPrivateKeyHex("0xabcdef"),
		WithEthClient(ec),
	)
	if err == nil {
		t.Fatal("expected error for too-short key")
	}
}

func TestNew_RPCDialError(t *testing.T) {
	key := testKey(t)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithRPCURL("http://127.0.0.1:1"), // refused port
	)
	if err == nil {
		t.Fatal("expected error for RPC dial failure")
	}
}

func TestNew_ChainDetectionFailure(t *testing.T) {
	// Server that returns an error for eth_chainId.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32000,"message":"boom"}}`, req.ID)
	}))
	defer srv.Close()
	ec, err := ethclient.Dial(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ec.Close()

	key := testKey(t)
	_, err = New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err == nil {
		t.Fatal("expected error for chain detection failure")
	}
}

func TestNew_AddressResolutionFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json-rpc", http.StatusBadRequest)
			return
		}
		var result string
		switch req.Method {
		case "eth_chainId":
			result = `"0x4cb2f"`
		default:
			result = "null"
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, req.ID, result)
	}))
	defer srv.Close()
	ec, err := ethclient.Dial(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer ec.Close()

	key := testKey(t)
	_, err = New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err == nil {
		t.Fatal("expected error for address resolution failure")
	}
	if !strings.Contains(err.Error(), "synapse.ResolveAddresses") {
		t.Fatalf("error = %v, want synapse.ResolveAddresses", err)
	}
}

func TestNew_ChainIDOverflow(t *testing.T) {
	// Return a chain ID that doesn't fit in int64.
	srv, ec := fakeRPCServer(t, "0xffffffffffffffffffffffffffffffff")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err == nil {
		t.Fatal("expected error for overflowing chain ID")
	}
	if !strings.Contains(err.Error(), "exceeds int64") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestNew_ChainIDOverflow_OwnedClient(t *testing.T) {
	// Same overflow test but with WithRPCURL (ownsClient=true).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0xffffffffffffffffffffffffffffffff"}`, req.ID)
	}))
	defer srv.Close()

	key := testKey(t)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithRPCURL(srv.URL),
	)
	if err == nil {
		t.Fatal("expected error for overflowing chain ID")
	}
}

func TestNew_UnsupportedChain_OwnedClient(t *testing.T) {
	// Unsupported chain ID with owned client (WithRPCURL) — tests ownsClient cleanup.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x1"}`, req.ID) // Ethereum mainnet
	}))
	defer srv.Close()

	key := testKey(t)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithRPCURL(srv.URL),
	)
	if err == nil {
		t.Fatal("expected error for unsupported chain")
	}
}

func TestNew_WithLoggerAndHTTPClient_FilBeam(t *testing.T) {
	// Exercise the FilBeam logger + httpClient option branches and verify
	// FilBeam requests go through the injected HTTP client.
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	logger := newTestLogger()
	var gotURL string
	hc := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotURL = req.URL.String()
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"cdnEgressQuota":"1","cacheMissEgressQuota":"2"}`)),
				Request:    req,
			}, nil
		}),
	}
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithLogger(logger),
		WithHTTPClient(hc),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	stats, err := client.FilBeam().GetDataSetStats(context.Background(), types.NewBigInt(123))
	if err != nil {
		t.Fatalf("GetDataSetStats: %v", err)
	}
	if gotURL != "https://calibration.stats.filbeam.com/data-set/123" {
		t.Fatalf("expected injected client to see filbeam request, got %q", gotURL)
	}
	if stats.CDNEgressQuota.Cmp(big.NewInt(1)) != 0 {
		t.Fatalf("unexpected CDN quota: got %s, want 1", stats.CDNEgressQuota)
	}
	if client.Storage() == nil {
		t.Error("Storage() returned nil")
	}
}

func TestNew_ZeroAddressChain(t *testing.T) {
	// Provide a chain with zero addresses via WithChain to trigger address validation error.
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	bogus := chain.Chain(255) // out of range → all zero addresses
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithChain(bogus),
	)
	if err == nil {
		t.Fatal("expected error for zero-address chain")
	}
	if !strings.Contains(err.Error(), "address") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestNew_ZeroAddressChain_OwnedClient(t *testing.T) {
	// Same as above but with owned client (WithRPCURL) to cover ownsClient cleanup.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x4cb2f"}`, req.ID)
	}))
	defer srv.Close()

	key := testKey(t)
	bogus := chain.Chain(255)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithRPCURL(srv.URL),
		WithChain(bogus),
	)
	if err == nil {
		t.Fatal("expected error for zero-address chain")
	}
}

func TestNew_ChainDetectionFailure_OwnedClient(t *testing.T) {
	// Chain detection failure when we own the client (WithRPCURL).
	// The client should be closed automatically.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":{"code":-32000,"message":"boom"}}`, req.ID)
	}))
	defer srv.Close()

	key := testKey(t)
	_, err := New(context.Background(),
		WithPrivateKey(key),
		WithRPCURL(srv.URL),
	)
	if err == nil {
		t.Fatal("expected error for chain detection failure")
	}
}

func TestParsePrivateKeyHex_Empty(t *testing.T) {
	_, err := parsePrivateKeyHex("")
	if err == nil {
		t.Fatal("expected error for empty string")
	}
}

func TestParsePrivateKeyHex_InvalidHex(t *testing.T) {
	_, err := parsePrivateKeyHex("0xzzzz")
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestParsePrivateKeyHex_TooShort(t *testing.T) {
	_, err := parsePrivateKeyHex("0xabcdef")
	if err == nil {
		t.Fatal("expected error for too-short key bytes")
	}
}

func TestParsePrivateKeyHex_Valid(t *testing.T) {
	key := testKey(t)
	hexStr := fmt.Sprintf("0x%x", ethcrypto.FromECDSA(key))
	got, err := parsePrivateKeyHex(hexStr)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ethcrypto.PubkeyToAddress(got.PublicKey) != ethcrypto.PubkeyToAddress(key.PublicKey) {
		t.Error("parsed key address mismatch")
	}
}

func TestZeroPrivateKeyClearsBackingArrayCapacity(t *testing.T) {
	backing := []big.Word{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
	scalar := new(big.Int).SetBits(backing[:2])
	key := testPrivateKeyWithScalar(scalar)

	zeroPrivateKey(key)

	for i, word := range backing {
		if word != 0 {
			t.Fatalf("backing word %d = %x, want 0", i, word)
		}
	}
	if scalar.Sign() != 0 {
		t.Fatalf("scalar.Sign() = %d, want 0", scalar.Sign())
	}
}

func TestGetters_ConcurrentAccess(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f")
	defer srv.Close()
	defer ec.Close()

	key := testKey(t)
	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	// Hammer all getters concurrently under the race detector.
	// Getters are plain field reads; this verifies no accidental data race
	// is introduced by future changes.
	const goroutines = 20
	done := make(chan struct{})
	for i := range goroutines {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			switch n % 7 {
			case 0:
				_ = client.WarmStorage()
			case 1:
				_ = client.SPRegistry()
			case 2:
				_ = client.Payments()
			case 3:
				_ = client.SessionKey()
			case 4:
				_ = client.Costs()
			case 5:
				_ = client.FilBeam()
			case 6:
				_ = client.Storage()
			}
		}(i)
	}
	for range goroutines {
		<-done
	}

	// Verify idempotency after concurrent access.
	ws1, ws2 := client.WarmStorage(), client.WarmStorage()
	if ws1 != ws2 {
		t.Error("WarmStorage() returned different instances")
	}
	st1, st2 := client.Storage(), client.Storage()
	if st1 != st2 {
		t.Error("Storage() returned different instances")
	}
}

// makeLoopbackDownloadClient creates a synapse.Client whose storage service
// will attempt to download from a loopback httptest.Server.
// It reuses fakeRPCServer (Calibration chain) defined elsewhere in this file.
// The returned cleanup closure (rather than a t.Cleanup registration) lets
// callers defer it inline, keeping the loopback download tests concise.
func makeLoopbackDownloadClient(t *testing.T, opts ...ClientOption) (*Client, func()) {
	t.Helper()
	srv, ec := fakeRPCServer(t, "0x4cb2f") // Calibration
	key := testKey(t)
	base := []ClientOption{
		WithPrivateKey(key),
		WithEthClient(ec),
	}
	base = append(base, opts...)
	client, err := New(context.Background(), base...)
	if err != nil {
		srv.Close()
		ec.Close()
		t.Fatalf("New: %v", err)
	}
	cleanup := func() {
		_ = client.Close()
		ec.Close()
		srv.Close()
	}
	return client, cleanup
}

// TestWithAllowPrivateNetworks_DefaultRejectsLoopback verifies that a root
// client built without WithAllowPrivateNetworks rejects downloads from
// loopback addresses with ErrPrivateNetwork.
func TestWithAllowPrivateNetworks_DefaultRejectsLoopback(t *testing.T) {
	data := bytes.Repeat([]byte("ssrf"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	loopback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer loopback.Close()

	client, cleanup := makeLoopbackDownloadClient(t) // default: AllowPrivateNetworks=false
	defer cleanup()

	_, dlErr := client.Storage().Download(context.Background(), info.CIDv2,
		&storage.DownloadOptions{URL: loopback.URL})
	if dlErr == nil {
		t.Fatal("expected ErrPrivateNetwork, got nil")
	}
	if !errors.Is(dlErr, storage.ErrPrivateNetwork) {
		t.Fatalf("expected ErrPrivateNetwork, got: %v", dlErr)
	}
}

// TestWithAllowPrivateNetworks_TrueAllowsLoopback verifies that
// WithAllowPrivateNetworks(true) allows downloading from a loopback server.
func TestWithAllowPrivateNetworks_TrueAllowsLoopback(t *testing.T) {
	data := bytes.Repeat([]byte("priv"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	loopback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer loopback.Close()

	client, cleanup := makeLoopbackDownloadClient(t, WithAllowPrivateNetworks(true))
	defer cleanup()

	reader, dlErr := client.Storage().Download(context.Background(), info.CIDv2,
		&storage.DownloadOptions{URL: loopback.URL})
	if dlErr != nil {
		t.Fatalf("Download: %v", dlErr)
	}
	defer func() { _ = reader.Close() }()
	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
}

// TestWithAllowPrivateNetworks_WithHTTPClientWins verifies that when
// WithHTTPClient is supplied, the custom client's transport governs SSRF
// policy regardless of the WithAllowPrivateNetworks value — the bool has
// no effect because storage.Options.AllowPrivateNetworks is only consulted
// when the SDK builds its own safe HTTP client (i.e. when HTTPClient is nil).
func TestWithAllowPrivateNetworks_WithHTTPClientWins(t *testing.T) {
	data := bytes.Repeat([]byte("cust"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	loopback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer loopback.Close()

	// Supply a custom HTTP client that permits loopback (uses DefaultTransport).
	custom := &http.Client{Transport: http.DefaultTransport}

	// Even without WithAllowPrivateNetworks the custom client's transport is used,
	// so the download must succeed — the bool has no effect when HTTPClient is set.
	client, cleanup := makeLoopbackDownloadClient(t,
		WithHTTPClient(custom),
		WithAllowPrivateNetworks(false), // explicit false — custom client still wins
	)
	defer cleanup()

	reader, dlErr := client.Storage().Download(context.Background(), info.CIDv2,
		&storage.DownloadOptions{URL: loopback.URL})
	if dlErr != nil {
		t.Fatalf("Download with custom HTTP client: %v", dlErr)
	}
	defer func() { _ = reader.Close() }()
	got, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
}
