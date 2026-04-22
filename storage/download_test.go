package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	_, readErr := io.ReadAll(reader)
	if readErr == nil {
		t.Fatal("expected validation error")
	}
	var cidMismatch *CIDMismatchError
	if !errors.As(readErr, &cidMismatch) {
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

	mgr := mustNewService(t, Options{AllowPrivateNetworks: true})
	_, err = mgr.Download(context.Background(), info.CIDv2, &DownloadOptions{URL: server.URL})
	if err == nil {
		t.Fatal("expected error for non-2xx status")
	}
	var dlErr *DownloadError
	if !errors.As(err, &dlErr) {
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
	var dlErr *DownloadError
	if !errors.As(err, &dlErr) {
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
