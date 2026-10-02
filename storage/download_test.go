package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestContextDownload_UsesPDPProviderClientAndValidatesPiece(t *testing.T) {
	data := bytes.Repeat([]byte("dl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	fake := &fakePDPProviderClient{
		downloadPieceFn: func(_ context.Context, pieceCID cid.Cid) (io.ReadCloser, int64, error) {
			if pieceCID != info.CIDv2 {
				t.Fatalf("pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
		},
	}
	providerCtx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	dataSetCtx, err := NewDataSetContext(testProvider(), fake, mustTestSigner(t), testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)))
	if err != nil {
		t.Fatalf("NewDataSetContext: %v", err)
	}

	for _, storageCtx := range []StorageContext{providerCtx, dataSetCtx} {
		reader, err := storageCtx.Download(context.Background(), info.CIDv2)
		if err != nil {
			t.Fatalf("%T.Download: %v", storageCtx, err)
		}
		got, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("%T downloaded bytes mismatch", storageCtx)
		}
	}
}

func TestContextDownload_EnforcesPieceCIDV2RawSize(t *testing.T) {
	data := bytes.Repeat([]byte("limit"), 64)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	overflow := append(bytes.Clone(data), 0xff)

	tests := []struct {
		name    string
		withCDN bool
		chunks  [][]byte
	}{
		{name: "provider overflow in same read", chunks: [][]byte{overflow}},
		{name: "provider overflow in later read", chunks: [][]byte{data, {0xff}}},
		{name: "CDN overflow in same read", withCDN: true, chunks: [][]byte{overflow}},
		{name: "CDN overflow in later read", withCDN: true, chunks: [][]byte{data, {0xff}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := io.NopCloser(&scriptedReader{chunks: append([][]byte(nil), tt.chunks...)})
			fake := &fakePDPProviderClient{
				downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
					if tt.withCDN {
						t.Fatal("PDP direct should not be called after CDN success")
					}
					return body, -1, nil
				},
			}
			options := []ContextOption(nil)
			if tt.withCDN {
				options = append(options,
					WithCDN(true),
					WithCDNRetriever(fakeCDNRetriever{
						downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, error) {
							return body, nil
						},
					}),
				)
			}
			ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t), options...)
			if err != nil {
				t.Fatalf("NewProviderContext: %v", err)
			}

			reader, err := ctx.Download(context.Background(), info.CIDv2)
			if err != nil {
				t.Fatalf("Download: %v", err)
			}
			got, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if !errors.Is(readErr, ErrMaxBytesExceeded) {
				t.Fatalf("ReadAll error = %v, want ErrMaxBytesExceeded", readErr)
			}
			if closeErr != nil {
				t.Fatalf("Close: %v", closeErr)
			}
			if !bytes.Equal(got, data) {
				t.Fatalf("downloaded %d bytes, want exactly RawSize %d", len(got), info.RawSize)
			}
		})
	}
}

func TestContextDownload_RejectsOversizedDeclaredLength(t *testing.T) {
	data := bytes.Repeat([]byte("length"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	body := &trackingReadCloser{Reader: bytes.NewReader(data)}
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
			return body, int64(info.RawSize) + 1, nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("Download error = %v, want ErrMaxBytesExceeded", err)
	}
	if reader != nil {
		t.Fatal("Download returned a reader for an oversized declared length")
	}
	if !body.closed {
		t.Fatal("oversized response body was not closed")
	}
}

func TestContextDownload_RejectsNilProviderBody(t *testing.T) {
	data := bytes.Repeat([]byte("nil-body"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
			return nil, int64(info.RawSize) + 1, nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if err == nil {
		t.Fatal("Download returned nil error for nil provider body")
	}
	if reader != nil {
		t.Fatal("Download returned a reader for nil provider body")
	}
}

func TestContextDownload_TruncatedStreamStillFailsIntegrityValidation(t *testing.T) {
	data := bytes.Repeat([]byte("truncated"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	truncated := data[:len(data)-1]
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader(truncated)), -1, nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	if !bytes.Equal(got, truncated) {
		t.Fatalf("downloaded %d bytes, want truncated length %d", len(got), len(truncated))
	}
	if errors.Is(readErr, ErrMaxBytesExceeded) {
		t.Fatalf("ReadAll error = %v, must remain an integrity error", readErr)
	}
	if _, ok := errors.AsType[*CIDMismatchError](readErr); !ok {
		t.Fatalf("ReadAll error = %T %v, want *CIDMismatchError", readErr, readErr)
	}
}

func TestContextDownload_ValidationFailureSurfacesAtEOF(t *testing.T) {
	good := bytes.Repeat([]byte("ok"), 128)
	bad := bytes.Repeat([]byte("no"), 128)
	info, err := piece.CalculateFromBytes(good)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	fake := &fakePDPProviderClient{
		downloadPieceFn: func(_ context.Context, _ cid.Cid) (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader(bad)), int64(len(bad)), nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = reader.Close() }()

	_, readErr := io.ReadAll(reader)
	if readErr == nil {
		t.Fatal("expected validation error")
	}
	cidMismatch, ok := errors.AsType[*CIDMismatchError](readErr)
	if !ok {
		t.Fatalf("want *CIDMismatchError, got %T: %v", readErr, readErr)
	}
	if cidMismatch.Expected != info.CIDv2 {
		t.Errorf("Expected CID = %s, want %s", cidMismatch.Expected, info.CIDv2)
	}
	if cidMismatch.ComputedV1 == (cid.Cid{}) {
		t.Error("ComputedV1 should not be zero")
	}
	if cidMismatch.ComputedV2 == (cid.Cid{}) {
		t.Error("ComputedV2 should not be zero")
	}
}

func TestContextDownload_WithCDNUsesRetrieverFirst(t *testing.T) {
	data := bytes.Repeat([]byte("cdn"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	cdnCalls := 0
	cdn := fakeCDNRetriever{
		downloadPieceFn: func(_ context.Context, pieceCID cid.Cid) (io.ReadCloser, error) {
			cdnCalls++
			if pieceCID != info.CIDv2 {
				t.Fatalf("cdn pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	}
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
			t.Fatal("PDP direct should not be called after CDN success")
			return nil, 0, nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t),
		WithCDN(true),
		WithCDNRetriever(cdn),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
	if cdnCalls != 1 {
		t.Fatalf("cdnCalls=%d want 1", cdnCalls)
	}
}

func TestContextDownload_WithCDNFallbacksToPDPOnRetrieverError(t *testing.T) {
	data := bytes.Repeat([]byte("fallback"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	cdnCalls := 0
	pdpCalls := 0
	cdn := fakeCDNRetriever{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, error) {
			cdnCalls++
			return nil, errors.New("cdn miss")
		},
	}
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(_ context.Context, pieceCID cid.Cid) (io.ReadCloser, int64, error) {
			pdpCalls++
			if pieceCID != info.CIDv2 {
				t.Fatalf("pdp pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t),
		WithCDN(true),
		WithCDNRetriever(cdn),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
	if cdnCalls != 1 || pdpCalls != 1 {
		t.Fatalf("cdnCalls=%d pdpCalls=%d want 1,1", cdnCalls, pdpCalls)
	}
}

func TestContextDownload_WithCDNContextCancellationDoesNotFallback(t *testing.T) {
	data := bytes.Repeat([]byte("cancel"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	cdn := fakeCDNRetriever{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, error) {
			return nil, context.Canceled
		},
	}
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
			t.Fatal("PDP direct should not be called after CDN cancellation")
			return nil, 0, nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t),
		WithCDN(true),
		WithCDNRetriever(cdn),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Download(context.Background(), info.CIDv2)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

func TestContextDownload_WithCDNFalseSkipsRetriever(t *testing.T) {
	data := bytes.Repeat([]byte("direct"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	cdnCalls := 0
	pdpCalls := 0
	cdn := fakeCDNRetriever{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, error) {
			cdnCalls++
			return nil, errors.New("should not be called")
		},
	}
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
			pdpCalls++
			return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
		},
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t),
		WithCDN(false),
		WithCDNRetriever(cdn),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
	if cdnCalls != 0 || pdpCalls != 1 {
		t.Fatalf("cdnCalls=%d pdpCalls=%d want 0,1", cdnCalls, pdpCalls)
	}
}

type fakeCDNRetriever struct {
	downloadPieceFn func(context.Context, cid.Cid) (io.ReadCloser, error)
}

func (f fakeCDNRetriever) DownloadPiece(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, error) {
	return f.downloadPieceFn(ctx, pieceCID)
}

type scriptedReader struct {
	chunks [][]byte
}

func (r *scriptedReader) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, errors.New("unexpected read beyond scripted overflow")
	}
	n := copy(p, r.chunks[0])
	r.chunks[0] = r.chunks[0][n:]
	if len(r.chunks[0]) == 0 {
		r.chunks = r.chunks[1:]
	}
	return n, nil
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestManagerDownload_URLValidatesPiece(t *testing.T) {
	data := bytes.Repeat([]byte("mg"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true})
	reader, err := mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = reader.Close() }()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
}

func TestManagerDownload_ContextAndURLConflict(t *testing.T) {
	data := bytes.Repeat([]byte("conflict"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	mgr := mustNewService(t, Options{})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{
		Context: fakeDownloadContext{},
		URL:     "https://example.com",
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !errors.Is(err, ErrInvalidDownloadOptions) {
		t.Fatalf("expected ErrInvalidDownloadOptions, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got: %v", err)
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected mutually-exclusive message, got: %v", err)
	}
}

type fakeDownloadContext struct{}

func (fakeDownloadContext) Download(context.Context, cid.Cid) (io.ReadCloser, error) {
	return nil, nil
}

// TestManagerDownload_WithHTTPClient proves that WithHTTPClient replaces the
// default transport so the manager's URL-based download uses the injected client.
func TestManagerDownload_WithHTTPClient(t *testing.T) {
	data := bytes.Repeat([]byte("inject"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		_, _ = w.Write(data)
	}))
	defer server.Close()

	// Inject a custom client with a transport that records whether it was used.
	customTransport := &recordingTransport{inner: http.DefaultTransport}
	custom := &http.Client{Transport: customTransport}

	mgr := mustNewService(t, Options{HTTPClient: custom})
	reader, err := mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = reader.Close() }()
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !called {
		t.Error("server was never called")
	}
	if !customTransport.used {
		t.Error("custom HTTP transport was not used")
	}
}

// TestManagerDownload_DefaultClientHasTimeout proves the default Manager has a
// finite HTTP timeout (not zero / no-timeout like http.DefaultClient).
func TestManagerDownload_DefaultClientHasTimeout(t *testing.T) {
	mgr := mustNewService(t, Options{})
	if mgr.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if mgr.httpClient.Timeout == 0 {
		t.Fatal("default httpClient.Timeout is 0 (no timeout), want a finite default")
	}
}

type recordingTransport struct {
	inner http.RoundTripper
	used  bool
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.used = true
	return rt.inner.RoundTrip(req)
}

// TestContextDownload_RejectsPieceCIDv1 proves that Context.Download requires
// PieceCIDv2 because the PDP provider only accepts v2. Raw size is unavailable
// here so transparent v1→v2 normalisation is not possible. The URL-based path in
// Manager.Download still accepts both forms.
func TestContextDownload_RejectsPieceCIDv1(t *testing.T) {
	data := bytes.Repeat([]byte("v1dl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	ctx, err := NewProviderContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Download(context.Background(), info.CIDv1)
	if err == nil {
		t.Fatal("expected error: Context.Download must reject PieceCIDv1")
	}
}

// TestManagerDownload_URLAcceptsPieceCIDv1 proves that Manager.Download with
// a raw URL accepts PieceCIDv1 and the post-download validator matches v1.
func TestManagerDownload_URLAcceptsPieceCIDv1(t *testing.T) {
	data := bytes.Repeat([]byte("urlv1"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true})
	reader, err := mgr.Download(context.Background(), info.CIDv1, &DownloadOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("Manager.Download with v1 CID: %v", err)
	}
	defer func() { _ = reader.Close() }()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
}

// TestManagerDownload_RejectsNonPieceCID proves that a CID that is neither
// PieceCIDv1 nor PieceCIDv2 is rejected immediately at the boundary.
func TestManagerDownload_RejectsNonPieceCID(t *testing.T) {
	// A well-known dag-pb CID from IPFS — definitely not a piece CID.
	nonPiece, err := cid.Parse("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	if err != nil {
		t.Fatalf("cid.Parse: %v", err)
	}
	mgr := mustNewService(t, Options{})
	_, err = mgr.Download(context.Background(), nonPiece, &DownloadOptions{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for non-piece CID, got nil")
	}
}

// TestContextDownload_RejectsNonPieceCID proves the same boundary check for
// the PDP-backed path.
func TestContextDownload_RejectsNonPieceCID(t *testing.T) {
	nonPiece, err := cid.Parse("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	if err != nil {
		t.Fatalf("cid.Parse: %v", err)
	}
	ctx, err := NewProviderContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Download(context.Background(), nonPiece)
	if err == nil {
		t.Fatal("expected error for non-piece CID, got nil")
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func assertNoURLSecrets(t *testing.T, label, got string) {
	t.Helper()
	for _, secret := range []string{"secretuser", "secretpass", "secretquery"} {
		if strings.Contains(got, secret) {
			t.Fatalf("%s leaked %q: %s", label, secret, got)
		}
	}
}

func TestDownloadAndValidate_StatusErrorRedactsURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	data := bytes.Repeat([]byte("rs"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	rawURL := strings.Replace(server.URL, "://", "://secretuser:secretpass@", 1) + "/piece?token=secretquery&part=1"

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: rawURL})
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	dlErr, ok := errors.AsType[*DownloadError](err)
	if !ok {
		t.Fatalf("want *DownloadError, got %T: %v", err, err)
	}
	if dlErr.StatusCode != http.StatusForbidden {
		t.Fatalf("StatusCode = %d, want %d", dlErr.StatusCode, http.StatusForbidden)
	}
	assertNoURLSecrets(t, "DownloadError.URL", dlErr.URL)
	assertNoURLSecrets(t, "Error()", err.Error())
	assertNoURLSecrets(t, "%#v", fmt.Sprintf("%#v", dlErr))
	if !strings.Contains(dlErr.URL, "part=1") {
		t.Fatalf("DownloadError.URL dropped non-sensitive query: %s", dlErr.URL)
	}
}

func TestDownloadAndValidate_TransportErrorRedactsURL(t *testing.T) {
	data := bytes.Repeat([]byte("rt"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	transportErr := errors.New("connection refused")
	httpClient := &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})}

	mgr := mustNewService(t, Options{HTTPClient: httpClient})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{
		URL: "https://secretuser:secretpass@provider.example/piece?token=secretquery&part=1",
	})
	if err == nil {
		t.Fatal("expected transport error")
	}
	dlErr, ok := errors.AsType[*DownloadError](err)
	if !ok {
		t.Fatalf("want *DownloadError, got %T: %v", err, err)
	}
	urlErr, ok := errors.AsType[*url.Error](err)
	if !ok {
		t.Fatalf("want wrapped *url.Error, got %T: %v", err, err)
	}
	if !errors.Is(err, transportErr) {
		t.Fatalf("error should wrap transport cause: %v", err)
	}
	assertNoURLSecrets(t, "DownloadError.URL", dlErr.URL)
	assertNoURLSecrets(t, "url.Error.URL", urlErr.URL)
	assertNoURLSecrets(t, "Error()", err.Error())
	assertNoURLSecrets(t, "%#v", fmt.Sprintf("%#v", dlErr))
	if !strings.Contains(dlErr.URL, "part=1") {
		t.Fatalf("DownloadError.URL dropped non-sensitive query: %s", dlErr.URL)
	}
}

func TestDownloadAndValidate_MalformedURLRedactsQuery(t *testing.T) {
	data := bytes.Repeat([]byte("mq"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	mgr := mustNewService(t, Options{})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{
		URL: "https://secretuser:secretpass@provider.example/%zz?token=secretquery&part=1",
	})
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
	dlErr, ok := errors.AsType[*DownloadError](err)
	if !ok {
		t.Fatalf("want *DownloadError, got %T: %v", err, err)
	}
	urlErr, ok := errors.AsType[*url.Error](err)
	if !ok {
		t.Fatalf("want wrapped *url.Error, got %T: %v", err, err)
	}
	assertNoURLSecrets(t, "DownloadError.URL", dlErr.URL)
	assertNoURLSecrets(t, "url.Error.URL", urlErr.URL)
	assertNoURLSecrets(t, "Error()", err.Error())
	assertNoURLSecrets(t, "%#v", fmt.Sprintf("%#v", dlErr))
	if !strings.Contains(dlErr.URL, "part=1") {
		t.Fatalf("DownloadError.URL dropped non-sensitive query: %s", dlErr.URL)
	}
}

func TestDownloadAndValidate_Non2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	data := bytes.Repeat([]byte("xx"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	dlErr, ok := errors.AsType[*DownloadError](err)
	if !ok {
		t.Fatalf("want *DownloadError, got %T: %v", err, err)
	}
	if dlErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("StatusCode = %d, want %d", dlErr.StatusCode, http.StatusInternalServerError)
	}
	if dlErr.URL != server.URL {
		t.Errorf("URL = %q, want %q", dlErr.URL, server.URL)
	}
}

func TestDownloadAndValidate_RequestCreationError(t *testing.T) {
	data := bytes.Repeat([]byte("rr"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	mgr := mustNewService(t, Options{})
	// A URL with an invalid control character triggers http.NewRequestWithContext failure
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: "http://example.com/\x7f"})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	dlErr, ok := errors.AsType[*DownloadError](err)
	if !ok {
		t.Fatalf("want *DownloadError, got %T: %v", err, err)
	}
	if dlErr.Cause == nil {
		t.Error("expected non-nil Cause")
	}
	const wantURL = "http://example.com/\x7f"
	if dlErr.URL != wantURL {
		t.Errorf("URL = %q, want %q", dlErr.URL, wantURL)
	}
}

func TestManagerDownload_NilOptions(t *testing.T) {
	data := bytes.Repeat([]byte("no"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	mgr := mustNewService(t, Options{})
	_, err = mgr.Download(context.Background(), info.CIDv2, nil)
	if err == nil {
		t.Fatal("expected error for nil options")
	}
	if !errors.Is(err, ErrInvalidDownloadOptions) {
		t.Fatalf("expected ErrInvalidDownloadOptions, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got: %v", err)
	}
}

func TestManagerDownload_NoSource(t *testing.T) {
	data := bytes.Repeat([]byte("ns"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	mgr := mustNewService(t, Options{})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{})
	if err == nil {
		t.Fatal("expected error for no download source")
	}
	if !errors.Is(err, ErrInvalidDownloadOptions) {
		t.Fatalf("expected ErrInvalidDownloadOptions, got: %v", err)
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got: %v", err)
	}
}

// TestManagerDownload_RejectsLoopbackByDefault verifies that the default
// Service refuses to dial loopback addresses, preventing SSRF via
// Service.Download URL-based calls.
func TestManagerDownload_RejectsLoopbackByDefault(t *testing.T) {
	data := bytes.Repeat([]byte("x"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	mgr := mustNewService(t, Options{})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err == nil {
		t.Fatal("expected ErrPrivateNetwork, got nil")
	}
	if !errors.Is(err, ErrPrivateNetwork) {
		t.Fatalf("expected ErrPrivateNetwork, got: %v", err)
	}
}

// TestManagerDownload_AllowPrivateNetworksOptOut verifies that opting in
// via AllowPrivateNetworks=true lets the same loopback download succeed.
func TestManagerDownload_AllowPrivateNetworksOptOut(t *testing.T) {
	data := bytes.Repeat([]byte("y"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer server.Close()

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true})
	reader, err := mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = reader.Close() }()
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
}

// TestManagerDownload_RejectsUnsupportedScheme verifies that only http and
// https schemes are accepted.
func TestManagerDownload_RejectsUnsupportedScheme(t *testing.T) {
	data := bytes.Repeat([]byte("z"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	mgr := mustNewService(t, Options{AllowPrivateNetworks: true})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: "file:///etc/passwd"})
	if err == nil {
		t.Fatal("expected ErrUnsupportedScheme, got nil")
	}
	if !errors.Is(err, ErrUnsupportedScheme) {
		t.Fatalf("expected ErrUnsupportedScheme, got: %v", err)
	}
}

// TestManagerDownload_MaxBytesContentLength verifies eager rejection when
// Content-Length reports a body larger than the cap.
func TestManagerDownload_MaxBytesContentLength(t *testing.T) {
	data := bytes.Repeat([]byte("q"), 4096)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "4096")
		_, _ = w.Write(data)
	}))
	defer server.Close()

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true, DownloadMaxBytes: 128})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err == nil {
		t.Fatal("expected ErrMaxBytesExceeded, got nil")
	}
	if !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("expected ErrMaxBytesExceeded, got: %v", err)
	}
}

// TestManagerDownload_MaxBytesStreaming verifies that a body exceeding the
// cap mid-stream surfaces ErrMaxBytesExceeded as the terminal Read error
// even when Content-Length is absent.
func TestManagerDownload_MaxBytesStreaming(t *testing.T) {
	data := bytes.Repeat([]byte("s"), 4096)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Transfer-Encoding", "chunked")
		_, _ = w.Write(data[:256])
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = w.Write(data[256:])
	}))
	defer server.Close()

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true, DownloadMaxBytes: 128})
	reader, err := mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = reader.Close() }()
	_, err = io.ReadAll(reader)
	if err == nil {
		t.Fatal("expected ErrMaxBytesExceeded, got nil")
	}
	if !errors.Is(err, ErrMaxBytesExceeded) {
		t.Fatalf("expected ErrMaxBytesExceeded, got: %v", err)
	}
}

// TestValidatingReadCloser_CloseBeforeEOF verifies that closing the reader
// before draining returns ErrClosedPipe on subsequent Reads rather than
// allowing the caller to mistake partial data for validated content.
func TestValidatingReadCloser_CloseBeforeEOF(t *testing.T) {
	data := bytes.Repeat([]byte("a"), 256)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	rc := newValidatingReadCloser(io.NopCloser(bytes.NewReader(data)), info.CIDv2, 0)
	buf := make([]byte, 8)
	if _, err := rc.Read(buf); err != nil {
		t.Fatalf("first Read: %v", err)
	}
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	n, err := rc.Read(buf)
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected io.ErrClosedPipe, got: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes after Close, got %d", n)
	}
}

type sleepyRaceReadCloser struct {
	data  []byte
	delay time.Duration
}

func (b *sleepyRaceReadCloser) Read(p []byte) (int, error) {
	time.Sleep(b.delay)
	n := copy(p, b.data)
	return n, io.EOF
}

func (*sleepyRaceReadCloser) Close() error { return nil }

// TestValidatingReadCloser_ConcurrentReadAndClose exercises the common pattern
// where one goroutine is blocked in Read while another closes the stream to
// abort it. Run with -race: the test should be free of Read/Close state races.
func TestValidatingReadCloser_ConcurrentReadAndClose(t *testing.T) {
	data := bytes.Repeat([]byte("b"), 256)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	base := &sleepyRaceReadCloser{data: data, delay: 20 * time.Millisecond}
	rc := newValidatingReadCloser(base, info.CIDv2, 0)

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, len(data))
		_, _ = rc.Read(buf)
	}()

	time.Sleep(5 * time.Millisecond)
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	<-done
}

func TestManagerDownload_DataSetID(t *testing.T) {
	data := bytes.Repeat([]byte("dataset-download"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	cdnMiss := errors.New("CDN cache miss")
	tests := []struct {
		name         string
		active       bool
		withCDN      bool
		noRetriever  bool
		wrongPayer   bool
		cdnErr       error
		cdnNilBody   bool
		oversized    bool
		wantErr      error
		wantReadErr  error
		wantCDNCalls int
		wantPDPCalls int
	}{
		{name: "inactive provider with CDN", withCDN: true, wantCDNCalls: 1},
		{name: "inactive provider without CDN", wantErr: spregistry.ErrNotFound},
		{name: "inactive provider with unavailable CDN", withCDN: true, cdnErr: cdnMiss, wantErr: spregistry.ErrNotFound, wantCDNCalls: 1},
		{name: "inactive provider without retriever", withCDN: true, noRetriever: true, wantErr: spregistry.ErrNotFound},
		{name: "active provider fallback", active: true, withCDN: true, cdnErr: cdnMiss, wantCDNCalls: 1, wantPDPCalls: 1},
		{name: "active provider without CDN", active: true, wantPDPCalls: 1},
		{name: "nil CDN body falls back", active: true, withCDN: true, cdnNilBody: true, wantCDNCalls: 1, wantPDPCalls: 1},
		{name: "CDN canceled", active: true, withCDN: true, cdnErr: context.Canceled, wantErr: context.Canceled, wantCDNCalls: 1},
		{name: "CDN deadline", active: true, withCDN: true, cdnErr: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantCDNCalls: 1},
		{name: "oversized CDN response", withCDN: true, oversized: true, wantCDNCalls: 1, wantReadErr: ErrMaxBytesExceeded},
		{name: "different payer", withCDN: true, wrongPayer: true, wantErr: ErrInvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payer := testPayer()
			if tt.wrongPayer {
				payer = common.HexToAddress("0x9999")
			}
			fixture := serviceResolverFixture{
				dataSetsByID:    map[string]*warmstorage.DataSetInfo{idconv.Key(types.NewBigInt(17)): {DataSetID: types.NewBigInt(17), ClientDataSetID: types.NewBigInt(3), ProviderID: types.NewBigInt(1), Payer: payer}},
				dataSetMetadata: map[string]map[string]string{idconv.Key(types.NewBigInt(17)): {}},
			}
			if tt.withCDN {
				fixture.dataSetMetadata[idconv.Key(types.NewBigInt(17))]["withCDN"] = ""
			}
			if tt.active {
				fixture.activeProviders = []spregistry.PDPProvider{testPDPProvider(types.NewBigInt(1), "https://provider.test")}
			}
			cdnCalls, pdpCalls, factoryCalls := 0, 0, 0
			var retriever CDNRetriever
			if !tt.noRetriever {
				retriever = fakeCDNRetriever{downloadPieceFn: func(_ context.Context, gotCID cid.Cid) (io.ReadCloser, error) {
					cdnCalls++
					if !gotCID.Equals(info.CIDv2) {
						t.Fatalf("CDN CID=%s want %s", gotCID, info.CIDv2)
					}
					if tt.cdnErr != nil || tt.cdnNilBody {
						return nil, tt.cdnErr
					}
					payload := bytes.Clone(data)
					if tt.oversized {
						payload = append(payload, 0)
					}
					return io.NopCloser(bytes.NewReader(payload)), nil
				}}
			}
			resolver, err := NewServiceResolver(ServiceResolverOptions{
				Payer: testPayer(), SPRegistry: &fakePDPProviderSource{fixture: fixture}, WarmStorage: &fakeDataSetCatalog{fixture: fixture}, CDNRetriever: retriever,
				NewContext: func(provider Provider, opts ContextFactoryOptions) (*ProviderContext, error) {
					factoryCalls++
					if opts.WithCDN {
						t.Fatal("provider fallback must not retry CDN")
					}
					return NewProviderContext(provider, &fakePDPProviderClient{downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) {
						pdpCalls++
						return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
					}}, nil, WithPayer(testPayer()))
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			svc := mustNewService(t, Options{DataSetDownloader: resolver})
			body, err := svc.Download(context.Background(), info.CIDv2, &DownloadOptions{DataSetID: new(types.NewBigInt(17))})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) || body != nil {
					t.Fatalf("body=%v err=%v want %v", body, err, tt.wantErr)
				}
				if tt.cdnErr != nil && !errors.Is(err, tt.cdnErr) {
					t.Fatalf("CDN cause lost: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				got, readErr := io.ReadAll(body)
				_ = body.Close()
				if tt.wantReadErr != nil {
					if !errors.Is(readErr, tt.wantReadErr) {
						t.Fatalf("read error=%v want %v", readErr, tt.wantReadErr)
					}
				} else if readErr != nil || !bytes.Equal(got, data) {
					t.Fatalf("download bytes=%d read error=%v", len(got), readErr)
				}
			}
			if cdnCalls != tt.wantCDNCalls || pdpCalls != tt.wantPDPCalls || factoryCalls != tt.wantPDPCalls {
				t.Fatalf("CDN/PDP/factory calls=%d/%d/%d want %d/%d/%d", cdnCalls, pdpCalls, factoryCalls, tt.wantCDNCalls, tt.wantPDPCalls, tt.wantPDPCalls)
			}
			if !tt.active && !tt.wrongPayer {
				if _, err := resolver.ResolveDataSetContext(context.Background(), types.NewBigInt(17), NewDataSetContextOptions{}); !errors.Is(err, spregistry.ErrNotFound) {
					t.Fatalf("inactive writable context error=%v want ErrNotFound", err)
				}
			}
		})
	}
}

func TestManagerDownload_DataSetIDOptions(t *testing.T) {
	info := mustPieceInfo(t)
	tests := []struct {
		name string
		opts DownloadOptions
		want error
	}{
		{name: "URL conflict", opts: DownloadOptions{DataSetID: new(types.NewBigInt(17)), URL: "https://example.test/piece"}, want: ErrInvalidDownloadOptions},
		{name: "context conflict", opts: DownloadOptions{DataSetID: new(types.NewBigInt(17)), Context: (*DataSetContext)(nil)}, want: ErrInvalidDownloadOptions},
		{name: "zero ID", opts: DownloadOptions{DataSetID: new(types.BigInt{})}, want: ErrInvalidDownloadOptions},
		{name: "unconfigured", opts: DownloadOptions{DataSetID: new(types.NewBigInt(17))}, want: ErrUninitialized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mustNewService(t, Options{}).Download(context.Background(), info.CIDv2, &tt.opts)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error=%v want %v", err, tt.want)
			}
		})
	}
}
