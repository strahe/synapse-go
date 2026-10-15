package pdp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
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

func TestDownloadPiece_TransportErrorRedactsServiceURLUserinfo(t *testing.T) {
	rt := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	c, err := New("https://secretuser@provider.example", WithHTTPClient(&http.Client{Transport: rt}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, _, err = c.DownloadPiece(context.Background(), testPieceInfoV2(t).CIDv2)
	if err == nil {
		t.Fatal("expected transport error")
	}
	if _, ok := errors.AsType[*url.Error](err); !ok {
		t.Fatalf("error=%T %v, want wrapped *url.Error", err, err)
	}
	if strings.Contains(err.Error(), "secretuser") {
		t.Fatalf("error leaked service URL userinfo: %v", err)
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

func TestDownloadPiece_RejectsV1(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	pc := testPieceInfoV2(t).CIDv1
	if _, _, err := c.DownloadPiece(context.Background(), pc); err == nil {
		t.Fatal("expected PieceCIDv2 validation error")
	}
}
