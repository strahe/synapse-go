package storage

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/piece"
)

func TestContextDownload_UsesCurioClientAndValidatesPiece(t *testing.T) {
	data := bytes.Repeat([]byte("dl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	fake := &fakeCurioClient{
		downloadPieceFn: func(_ context.Context, pieceCID cid.Cid) (io.ReadCloser, int64, error) {
			if pieceCID != info.CIDv2 {
				t.Fatalf("pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			return io.NopCloser(bytes.NewReader(data)), int64(len(data)), nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
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

func TestContextDownload_ValidationFailureSurfacesAtEOF(t *testing.T) {
	good := bytes.Repeat([]byte("ok"), 128)
	bad := bytes.Repeat([]byte("no"), 128)
	info, err := piece.CalculateFromBytes(good)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	fake := &fakeCurioClient{
		downloadPieceFn: func(_ context.Context, _ cid.Cid) (io.ReadCloser, int64, error) {
			return io.NopCloser(bytes.NewReader(bad)), int64(len(bad)), nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader, err := ctx.Download(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = reader.Close() }()

	if _, err := io.ReadAll(reader); err == nil {
		t.Fatal("expected validation error")
	}
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

	mgr := NewManager()
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
	mgr := NewManager()
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{
		Context: fakeDownloadContext{},
		URL:     "https://example.com",
	})
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !strings.Contains(err.Error(), "cannot specify both") {
		t.Fatalf("unexpected error: %v", err)
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

	mgr := NewManager(WithHTTPClient(custom))
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
	mgr := NewManager()
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
// PieceCIDv2 because curio only accepts v2.  Raw size is unavailable here so
// transparent v1→v2 normalisation is not possible.  The URL-based path in
// Manager.Download still accepts both forms.
func TestContextDownload_RejectsPieceCIDv1(t *testing.T) {
	data := bytes.Repeat([]byte("v1dl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	ctx, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t))
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

	mgr := NewManager()
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
	mgr := NewManager()
	_, err = mgr.Download(context.Background(), nonPiece, &DownloadOptions{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for non-piece CID, got nil")
	}
}

// TestContextDownload_RejectsNonPieceCID proves the same boundary check for
// the curio-backed path.
func TestContextDownload_RejectsNonPieceCID(t *testing.T) {
	nonPiece, err := cid.Parse("QmYwAPJzv5CZsnA625s3Xf2nemtYgPpHdWEz79ojWnPbdG")
	if err != nil {
		t.Fatalf("cid.Parse: %v", err)
	}
	ctx, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Download(context.Background(), nonPiece)
	if err == nil {
		t.Fatal("expected error for non-piece CID, got nil")
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

	mgr := NewManager()
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDownloadAndValidate_RequestCreationError(t *testing.T) {
	data := bytes.Repeat([]byte("rr"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	mgr := NewManager()
	// A URL with an invalid control character triggers http.NewRequestWithContext failure
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: "http://example.com/\x7f"})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
	if !strings.Contains(err.Error(), "build download request") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestManagerDownload_NilOptions(t *testing.T) {
	data := bytes.Repeat([]byte("no"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	mgr := NewManager()
	_, err = mgr.Download(context.Background(), info.CIDv2, nil)
	if err == nil {
		t.Fatal("expected error for nil options")
	}
}

func TestManagerDownload_NoSource(t *testing.T) {
	data := bytes.Repeat([]byte("ns"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	mgr := NewManager()
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{})
	if err == nil {
		t.Fatal("expected error for no download source")
	}
}
