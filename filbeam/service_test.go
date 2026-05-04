package filbeam

import (
	"bytes"
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

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/piece"
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

type recordedRequest struct {
	method string
	host   string
	path   string
}

type recordingRewriteHost struct {
	base     string
	inner    http.RoundTripper
	requests *[]recordedRequest
}

func (rt *recordingRewriteHost) RoundTrip(req *http.Request) (*http.Response, error) {
	*rt.requests = append(*rt.requests, recordedRequest{
		method: req.Method,
		host:   req.URL.Host,
		path:   req.URL.Path,
	})
	cloned := req.Clone(req.Context())
	parsed, _ := url.Parse(rt.base)
	cloned.URL.Scheme = parsed.Scheme
	cloned.URL.Host = parsed.Host
	return rt.inner.RoundTrip(cloned)
}

type closeTrackingBody struct {
	closed *bool
}

func (b closeTrackingBody) Read(_ []byte) (int, error) { return 0, io.EOF }

func (b closeTrackingBody) Close() error {
	*b.closed = true
	return nil
}

func TestNewRetrieverRejectsZeroOwner(t *testing.T) {
	svc := newTestService(t, chain.Calibration)
	_, err := svc.NewRetriever(common.Address{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("want ErrInvalidArgument, got %v", err)
	}
}

func TestRetrieverDownloadPieceValidatesPieceCID(t *testing.T) {
	svc := newTestService(t, chain.Calibration)
	retriever, err := svc.NewRetriever(common.HexToAddress("0x1234567890123456789012345678901234567890"))
	if err != nil {
		t.Fatalf("NewRetriever: %v", err)
	}

	if _, err := retriever.DownloadPiece(context.Background(), cid.Undef); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("undefined CID: want ErrInvalidArgument, got %v", err)
	}

	nonPiece, err := cid.Parse("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	if err != nil {
		t.Fatalf("cid.Parse: %v", err)
	}
	if _, err := retriever.DownloadPiece(context.Background(), nonPiece); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("non-piece CID: want ErrInvalidArgument, got %v", err)
	}
}

func TestRetrieverDownloadPieceAcceptsV1AndV2PieceCID(t *testing.T) {
	data := bytes.Repeat([]byte("fb"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	}))
	defer srv.Close()

	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{Transport: &recordingRewriteHost{
		base:     srv.URL,
		inner:    http.DefaultTransport,
		requests: &requests,
	}}
	retriever, err := svc.NewRetriever(common.HexToAddress("0x1234567890123456789012345678901234567890"))
	if err != nil {
		t.Fatalf("NewRetriever: %v", err)
	}

	for _, pc := range []cid.Cid{info.CIDv1, info.CIDv2} {
		rc, err := retriever.DownloadPiece(context.Background(), pc)
		if err != nil {
			t.Fatalf("DownloadPiece(%s): %v", pc, err)
		}
		_ = rc.Close()
	}
}

func TestRetrieverDownloadPieceUsesChainRetrievalDomain(t *testing.T) {
	data := bytes.Repeat([]byte("domain"), 64)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	tests := []struct {
		name string
		c    chain.Chain
		host string
	}{
		{"mainnet", chain.Mainnet, "0x1234567890123456789012345678901234567890.filbeam.io"},
		{"calibration", chain.Calibration, "0x1234567890123456789012345678901234567890.calibration.filbeam.io"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var requests []recordedRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					_, _ = w.Write(data)
				}
			}))
			defer srv.Close()

			svc := newTestService(t, tc.c)
			svc.httpClient = &http.Client{Transport: &recordingRewriteHost{
				base:     srv.URL,
				inner:    http.DefaultTransport,
				requests: &requests,
			}}
			retriever, err := svc.NewRetriever(common.HexToAddress("0x1234567890123456789012345678901234567890"))
			if err != nil {
				t.Fatalf("NewRetriever: %v", err)
			}
			rc, err := retriever.DownloadPiece(context.Background(), info.CIDv2)
			if err != nil {
				t.Fatalf("DownloadPiece: %v", err)
			}
			_ = rc.Close()

			if len(requests) != 2 {
				t.Fatalf("requests=%d want 2", len(requests))
			}
			if requests[0].host != tc.host || requests[1].host != tc.host {
				t.Fatalf("hosts=%q,%q want %q", requests[0].host, requests[1].host, tc.host)
			}
			if requests[0].path != "/"+info.CIDv2.String() || requests[1].path != "/"+info.CIDv2.String() {
				t.Fatalf("paths=%q,%q want /%s", requests[0].path, requests[1].path, info.CIDv2)
			}
		})
	}
}

func TestRetrieverDownloadPieceHeadsBeforeGet(t *testing.T) {
	data := bytes.Repeat([]byte("head-get"), 64)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_, _ = w.Write(data)
		}
	}))
	defer srv.Close()

	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{Transport: &recordingRewriteHost{
		base:     srv.URL,
		inner:    http.DefaultTransport,
		requests: &requests,
	}}
	retriever, err := svc.NewRetriever(common.HexToAddress("0x1234567890123456789012345678901234567890"))
	if err != nil {
		t.Fatalf("NewRetriever: %v", err)
	}

	rc, err := retriever.DownloadPiece(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("DownloadPiece: %v", err)
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
	if len(requests) != 2 || requests[0].method != http.MethodHead || requests[1].method != http.MethodGet {
		t.Fatalf("methods=%v want HEAD then GET", requests)
	}
}

func TestRetrieverDownloadPieceHeadFailureSkipsGet(t *testing.T) {
	data := bytes.Repeat([]byte("head-fail"), 64)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var requests []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			http.Error(w, "missing", http.StatusNotFound)
			return
		}
		t.Fatalf("unexpected GET after failed HEAD")
	}))
	defer srv.Close()

	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{Transport: &recordingRewriteHost{
		base:     srv.URL,
		inner:    http.DefaultTransport,
		requests: &requests,
	}}
	retriever, err := svc.NewRetriever(common.HexToAddress("0x1234567890123456789012345678901234567890"))
	if err != nil {
		t.Fatalf("NewRetriever: %v", err)
	}

	if _, err := retriever.DownloadPiece(context.Background(), info.CIDv2); err == nil {
		t.Fatal("expected error")
	}
	if len(requests) != 1 || requests[0].method != http.MethodHead {
		t.Fatalf("requests=%v want one HEAD", requests)
	}
}

func TestRetrieverDownloadPieceGetFailureClosesBody(t *testing.T) {
	data := bytes.Repeat([]byte("get-fail"), 64)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var getBodyClosed bool
	svc := newTestService(t, chain.Calibration)
	svc.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.Method {
		case http.MethodHead:
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Header:     make(http.Header),
				Request:    req,
			}, nil
		case http.MethodGet:
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       closeTrackingBody{closed: &getBodyClosed},
				Header:     make(http.Header),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected method %s", req.Method)
			return nil, nil
		}
	})}
	retriever, err := svc.NewRetriever(common.HexToAddress("0x1234567890123456789012345678901234567890"))
	if err != nil {
		t.Fatalf("NewRetriever: %v", err)
	}

	if _, err := retriever.DownloadPiece(context.Background(), info.CIDv2); err == nil {
		t.Fatal("expected error")
	}
	if !getBodyClosed {
		t.Fatal("GET failure body was not closed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
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

func TestNew_RetrievalDomainOverride(t *testing.T) {
	svc, err := New(Options{
		Chain:           chain.Calibration,
		RetrievalDomain: "staging.filbeam.example",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if svc.retrievalDomain != "staging.filbeam.example" {
		t.Fatalf("retrievalDomain=%q want staging.filbeam.example", svc.retrievalDomain)
	}
}

func TestNew_RetrievalDomainOverrideRejectsURL(t *testing.T) {
	tests := []string{
		"https://calibration.filbeam.io",
		"staging.filbeam.example@attacker.example",
	}
	for _, domain := range tests {
		t.Run(domain, func(t *testing.T) {
			_, err := New(Options{
				Chain:           chain.Calibration,
				RetrievalDomain: domain,
			})
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("want ErrInvalidArgument, got %v", err)
			}
		})
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
