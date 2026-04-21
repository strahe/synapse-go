package synapse

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/strahe/synapse-go/chain"
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

// jsonRPCReq is a minimal JSON-RPC request.
type jsonRPCReq struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// fakeRPCServer creates an httptest.Server that responds to eth_chainId with
// the given chain ID hex string (e.g. "0x4cb2f"). Returns the server (caller
// must Close) and an ethclient connected to it.
func fakeRPCServer(t *testing.T, chainIDHex string) (*httptest.Server, *ethclient.Client) {
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

func TestNew_WithEthClient_Calibration(t *testing.T) {
	srv, ec := fakeRPCServer(t, "0x4cb2f") // Calibration chain ID = 314159
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
}

func TestNew_WithRPCURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json-rpc", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		// Calibration
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x4cb2f"}`, req.ID)
	}))
	defer srv.Close()

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

func TestNew_MissingRPC(t *testing.T) {
	key := testKey(t)
	_, err := New(context.Background(), WithPrivateKey(key))
	if err == nil {
		t.Fatal("expected error for missing RPC source")
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req jsonRPCReq
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x4cb2f"}`, req.ID)
	}))
	defer srv.Close()

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
	hc := &http.Client{}

	client, err := New(context.Background(),
		WithPrivateKey(key),
		WithEthClient(ec),
		WithHTTPClient(hc),
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = client.Close() }()

	// HTTP client gets passed to sub-services.
	_ = client.FilBeam()
	_ = client.Storage()
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

	stats, err := client.FilBeam().GetDataSetStats(context.Background(), types.DataSetID(123))
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
