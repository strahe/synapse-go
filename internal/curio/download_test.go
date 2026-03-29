package curio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestDownloadPiece_OK(t *testing.T) {
	payload := []byte("hello piece data")
	pc := testPieceInfoV2(t).CIDv2

	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %s", r.Method)
		}
		wantPath := "/piece/" + pc.String()
		if r.URL.Path != wantPath {
			t.Errorf("path=%q want %q", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))

	rc, cl, err := c.DownloadPiece(context.Background(), pc)
	if err != nil {
		t.Fatalf("DownloadPiece: %v", err)
	}
	defer func() { _ = rc.Close() }()

	if cl != int64(len(payload)) {
		t.Errorf("contentLength=%d want %d", cl, len(payload))
	}
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("body=%q want %q", got, payload)
	}
}

func TestDownloadPiece_NotFound(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2

	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	_, _, err := c.DownloadPiece(context.Background(), pc)
	if !errors.Is(err, ErrPieceNotFound) {
		t.Fatalf("want ErrPieceNotFound, got %v", err)
	}
}

func TestDownloadPiece_UndefinedCID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server")
	}))
	_, _, err := c.DownloadPiece(context.Background(), emptyCID())
	if err == nil {
		t.Error("expected error for undefined CID")
	}
}

func TestDownloadPiece_UnknownContentLength(t *testing.T) {
	payload := []byte("no length header")
	pc := testPieceInfoV2(t).CIDv2

	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force chunked transfer encoding so no Content-Length is emitted.
		w.Header().Set("Transfer-Encoding", "chunked")
		// Explicitly set Content-Length to -1 is not possible; instead use
		// http.Flusher to force chunked mode which suppresses the header.
		w.WriteHeader(http.StatusOK)
		if flusher, ok := w.(http.Flusher); ok {
			_, _ = w.Write(payload)
			flusher.Flush()
		} else {
			_, _ = w.Write(payload)
		}
	}))

	rc, cl, err := c.DownloadPiece(context.Background(), pc)
	if err != nil {
		t.Fatalf("DownloadPiece: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if cl != -1 {
		t.Errorf("contentLength=%d want -1 for chunked/unknown length", cl)
	}
}

// Issue 2: DownloadPiece must clear timeout even for custom clients whose
// Timeout != DefaultHTTPTimeout (e.g. any nonzero custom timeout).
func TestDownloadPiece_CustomClientTimeoutCleared(t *testing.T) {
	payload := []byte("custom client payload")
	pc := testPieceInfoV2(t).CIDv2

	// Slow server: sleeps briefly before responding.  A client with a 1ms
	// timeout would fail here unless DownloadPiece zeros it out.
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))

	// Inject a custom client with a very short timeout (clearly not DefaultHTTPTimeout
	// and small enough to fire before the server replies).
	c.httpClient = &http.Client{Timeout: 1 * time.Millisecond}

	// Before the fix, DownloadPiece would only zero the timeout when it equals
	// DefaultHTTPTimeout, so the 1ms timeout would cut off the request.
	rc, _, err := c.DownloadPiece(context.Background(), pc)
	if err != nil {
		t.Fatalf("DownloadPiece with short custom-timeout client should succeed (timeout must be cleared): %v", err)
	}
	defer func() { _ = rc.Close() }()

	// Also verify the original client struct was not mutated.
	if c.httpClient.Timeout != 1*time.Millisecond {
		t.Errorf("original client Timeout mutated: got %v want 1ms", c.httpClient.Timeout)
	}
}

func TestDownloadPiece_RejectsV1(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	pc := testPieceInfoV2(t).CIDv1
	if _, _, err := c.DownloadPiece(context.Background(), pc); err == nil {
		t.Fatal("expected PieceCIDv2 validation error")
	}
}
