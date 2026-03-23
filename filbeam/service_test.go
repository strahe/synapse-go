package filbeam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/strahe/synapse-go/chain"
)

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

	svc := NewService(chain.Calibration, WithHTTPClient(srv.Client()))
	// Override baseURL by creating a small wrapper we can inject via httpClient transport.
	// We use a custom RoundTripper to rewrite the host.
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

	stats, err := svc.GetDataSetStats(context.Background(), big.NewInt(42))
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

func TestGetDataSetStats_ZeroQuotas(t *testing.T) {
	_, svc := serveStats(t, "0", "0")
	stats, err := svc.GetDataSetStats(context.Background(), big.NewInt(1))
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

	svc := NewService(chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), big.NewInt(99))
	if !errors.Is(err, ErrDataSetNotFound) {
		t.Fatalf("expected ErrDataSetNotFound, got %v", err)
	}
}

func TestGetDataSetStats_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	svc := NewService(chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), big.NewInt(1))
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetDataSetStats_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, "not valid json")
	}))
	defer srv.Close()

	svc := NewService(chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), big.NewInt(1))
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

	svc := NewService(chain.Calibration)
	svc.httpClient = &http.Client{
		Transport: &rewriteHost{base: srv.URL, inner: http.DefaultTransport},
	}

	_, err := svc.GetDataSetStats(context.Background(), big.NewInt(1))
	if err == nil {
		t.Fatal("expected error for non-numeric quota string")
	}
}

func TestGetDataSetStats_NilDataSetID(t *testing.T) {
	svc := NewService(chain.Calibration)
	_, err := svc.GetDataSetStats(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil dataSetID")
	}
}

func TestBaseURL_Mainnet(t *testing.T) {
	svc := NewService(chain.Mainnet)
	if svc.baseURL() != "https://stats.filbeam.com" {
		t.Errorf("unexpected mainnet baseURL: %s", svc.baseURL())
	}
}

func TestBaseURL_Calibration(t *testing.T) {
	svc := NewService(chain.Calibration)
	if svc.baseURL() != "https://calibration.stats.filbeam.com" {
		t.Errorf("unexpected calibration baseURL: %s", svc.baseURL())
	}
}
