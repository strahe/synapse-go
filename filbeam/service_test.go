package filbeam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/types"
)

// newTestService builds a filbeam Service for tests. It panics on error.
func newTestService(t *testing.T, c chain.Chain) *Service {
	t.Helper()
	svc, err := New(Options{Chain: c})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return svc
}

// serveStats starts an httptest server that responds with the given quota strings.
func serveStats(t *testing.T, cdn, cacheMiss string) (*httptest.Server, *Service) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"cdnEgressQuota":       cdn,
			"cacheMissEgressQuota": cacheMiss,
		})
	}))
	t.Cleanup(srv.Close)

	svc, err := New(Options{
		Chain:      chain.Calibration,
		HTTPClient: srv.Client(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Override httpClient to rewrite requests to the test server.
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}
	return srv, svc
}

// rewriteHost is a test-only RoundTripper that rewrites requests to point at base.
type rewriteHost struct {
	base  string
	inner http.RoundTripper
}

func (rt *rewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	parsed, _ := url.Parse(rt.base)
	cloned.URL.Scheme = parsed.Scheme
	cloned.URL.Host = parsed.Host
	return rt.inner.RoundTrip(cloned)
}

func TestGetDataSetStats_Success(t *testing.T) {
	cdnVal := "1234567890123456789"
	cacheVal := "9876543210987654321"
	_, svc := serveStats(t, cdnVal, cacheVal)

	stats, err := svc.GetDataSetStats(context.Background(), types.NewBigInt(42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantCDN, _ := new(big.Int).SetString(cdnVal, 10)
	if stats.CDNEgressQuota.Cmp(wantCDN) != 0 {
		t.Errorf("CDNEgressQuota: got %s, want %s", stats.CDNEgressQuota, wantCDN)
	}
	wantCache, _ := new(big.Int).SetString(cacheVal, 10)
	if stats.CacheMissEgressQuota.Cmp(wantCache) != 0 {
		t.Errorf("CacheMissEgressQuota: got %s, want %s", stats.CacheMissEgressQuota, wantCache)
	}
}

func TestGetDataSetStats_TypedDataSetIDSignature(t *testing.T) {
	method, ok := reflect.TypeOf((*Service)(nil)).MethodByName("GetDataSetStats")
	if !ok {
		t.Fatal("GetDataSetStats method not found")
	}
	want := reflect.TypeOf(func(*Service, context.Context, types.BigInt) (*DataSetStats, error) { return nil, nil })
	if method.Type != want {
		t.Fatalf("GetDataSetStats signature = %v, want %v", method.Type, want)
	}
}

func TestGetDataSetStats_ZeroQuotas(t *testing.T) {
	_, svc := serveStats(t, "0", "0")
	stats, err := svc.GetDataSetStats(context.Background(), types.NewBigInt(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stats.CDNEgressQuota.Sign() != 0 {
		t.Errorf("expected CDNEgressQuota=0, got %s", stats.CDNEgressQuota)
	}
	if stats.CacheMissEgressQuota.Sign() != 0 {
		t.Errorf("expected CacheMissEgressQuota=0, got %s", stats.CacheMissEgressQuota)
	}
}

func TestGetDataSetStats_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), types.NewBigInt(99))
	if !errors.Is(err, ErrDataSetNotFound) {
		t.Fatalf("expected ErrDataSetNotFound, got %v", err)
	}
}

func TestGetDataSetStats_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), types.NewBigInt(1))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetDataSetStats_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "not valid json")
	}))
	defer srv.Close()

	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), types.NewBigInt(1))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetDataSetStats_InvalidBigIntField(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"cdnEgressQuota":       "not-a-number",
			"cacheMissEgressQuota": "0",
		})
	}))
	defer srv.Close()

	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), types.NewBigInt(1))
	if err == nil {
		t.Fatal("expected error for non-numeric quota string")
	}
}

func TestBaseURL_Mainnet(t *testing.T) {
	svc := newTestService(t, chain.Mainnet)
	if svc.baseURL != "https://stats.filbeam.com" {
		t.Errorf("unexpected mainnet baseURL: %s", svc.baseURL)
	}
}

func TestBaseURL_Calibration(t *testing.T) {
	svc := newTestService(t, chain.Calibration)
	if svc.baseURL != "https://calibration.stats.filbeam.com" {
		t.Errorf("unexpected calibration baseURL: %s", svc.baseURL)
	}
}

func TestNew_UnsupportedChain(t *testing.T) {
	_, err := New(Options{Chain: chain.Chain(255)})
	if err == nil {
		t.Fatal("expected error for unsupported chain")
	}
	if !errors.Is(err, chain.ErrUnknownChain) {
		t.Errorf("expected wrapped chain.ErrUnknownChain, got %v", err)
	}
}

func TestNew_HTTPClientNilDefaults(t *testing.T) {
	svc, err := New(Options{Chain: chain.Calibration})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.httpClient != http.DefaultClient {
		t.Errorf("expected httpClient to default to http.DefaultClient, got %v", svc.httpClient)
	}
}

func TestNew_LoggerViaOptions(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := New(Options{Chain: chain.Calibration, Logger: logger})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.logger != logger {
		t.Error("expected logger to be set")
	}
}

// TestNew_ChainZeroValueIsMainnet guards the documented contract that an
// omitted Options.Chain (zero value) is equivalent to chain.Mainnet. A
// future refactor that starts treating zero as "unset/invalid" would break
// this and should be caught here.
func TestNew_ChainZeroValueIsMainnet(t *testing.T) {
	svc, err := New(Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.baseURL != "https://stats.filbeam.com" {
		t.Errorf("expected mainnet baseURL for zero-value Chain, got %s", svc.baseURL)
	}
}
