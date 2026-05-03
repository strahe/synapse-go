package pdp

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/net/http2"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func newTestClient(t *testing.T, handler http.Handler) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := New(srv.URL)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Eliminate retry backoff delays in tests. Retry logic itself is exercised
	// by the dedicated TestDoRetryable_* tests.
	c.retryDelayFn = noRetryDelay
	return c, srv
}

func testPieceInfoV2(t *testing.T) piece.PieceInfo {
	t.Helper()
	info, err := piece.CalculateFromBytes(bytes.Repeat([]byte{0xab}, 512))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	if !info.CIDv2.Defined() {
		t.Fatal("expected PieceCIDv2 fixture")
	}
	return info
}

func TestNew_Validation(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Error("expected error for empty url")
	}
	if _, err := New("://bad"); err == nil {
		t.Error("expected error for bad url")
	}
	if _, err := New("file:///x"); err == nil {
		t.Error("expected error for non-http scheme")
	}
	if c, err := New("https://example.com/prefix"); err != nil {
		t.Fatalf("New: %v", err)
	} else if !strings.HasSuffix(c.BaseURL().Path, "/") {
		t.Errorf("base path should end with slash, got %q", c.BaseURL().Path)
	}
}

func TestPing_OK(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pdp/ping" {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPing_Error(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusServiceUnavailable)
	}))
	err := c.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want HTTPError, got %T", err)
	}
	if he.StatusCode != 503 {
		t.Errorf("status=%d", he.StatusCode)
	}
}

// streamingUploadHandler is a reusable httptest handler implementing the
// CommP-last 3-step streaming upload protocol for client-side unit tests.
//
// The three endpoints are:
//
//	POST /pdp/piece/uploads         — returns 201 + Location: /pdp/piece/uploads/{uuid}
//	PUT  /pdp/piece/uploads/{uuid}  — stores the body; returns 204
//	POST /pdp/piece/uploads/{uuid}  — reads {"pieceCid": "..."} finalize; returns 200
type streamingUploadHandler struct {
	t *testing.T
	// uuid is the identifier minted on the create request.
	uuid string
	// receivedBody is populated on the PUT.
	receivedBody []byte
	// contentLength is the value of Content-Length on the PUT (-1 if absent).
	contentLength int64
	// transferEncoding is the value of Transfer-Encoding on the PUT.
	transferEncoding string
	// finalizePieceCID is the pieceCid value posted in the finalize body.
	finalizePieceCID string
	// putStatus and finalizeStatus let tests override per-step responses.
	putStatus      int
	finalizeStatus int
}

func (h *streamingUploadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/pdp/piece/uploads":
		if h.uuid == "" {
			h.uuid = "upload-uuid-1"
		}
		w.Header().Set("Location", "/pdp/piece/uploads/"+h.uuid)
		w.WriteHeader(http.StatusCreated)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/pdp/piece/uploads/"):
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			h.t.Errorf("PUT Content-Type=%q", r.Header.Get("Content-Type"))
		}
		h.contentLength = r.ContentLength
		if te := r.TransferEncoding; len(te) > 0 {
			h.transferEncoding = te[0]
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			h.t.Fatalf("read PUT body: %v", err)
		}
		h.receivedBody = body
		status := h.putStatus
		if status == 0 {
			status = http.StatusNoContent
		}
		w.WriteHeader(status)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/pdp/piece/uploads/"):
		var body struct {
			PieceCID string `json:"pieceCid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			h.t.Fatalf("decode finalize body: %v", err)
		}
		h.finalizePieceCID = body.PieceCID
		status := h.finalizeStatus
		if status == 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
	default:
		h.t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}
}

func TestUploadPieceStreaming_ComputesPieceCID(t *testing.T) {
	payload := bytes.Repeat([]byte{0xab}, 512)
	want := testPieceInfoV2(t).CIDv2

	h := &streamingUploadHandler{t: t}
	c, _ := newTestClient(t, h)
	res, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader(payload), UploadPieceStreamingOptions{
		Size: int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("UploadPieceStreaming: %v", err)
	}
	if res.PieceCID != want {
		t.Errorf("PieceCID=%s want %s", res.PieceCID, want)
	}
	if res.Size != int64(len(payload)) {
		t.Errorf("Size=%d want %d", res.Size, len(payload))
	}
	if h.contentLength != int64(len(payload)) {
		t.Errorf("Content-Length=%d want %d", h.contentLength, len(payload))
	}
	if !bytes.Equal(h.receivedBody, payload) {
		t.Error("body mismatch")
	}
	if h.finalizePieceCID != want.String() {
		t.Errorf("finalize pieceCid=%q want %s", h.finalizePieceCID, want)
	}
}

func TestUploadPieceStreaming_UnknownSizeUsesChunked(t *testing.T) {
	payload := bytes.Repeat([]byte{0xcd}, 512)
	// Wrap reader so *bytes.Reader length hint isn't used by net/http.
	wrapped := struct{ io.Reader }{bytes.NewReader(payload)}

	h := &streamingUploadHandler{t: t}
	c, _ := newTestClient(t, h)
	if _, err := c.UploadPieceStreaming(context.Background(), wrapped, UploadPieceStreamingOptions{}); err != nil {
		t.Fatalf("UploadPieceStreaming: %v", err)
	}
	if h.contentLength > 0 {
		t.Errorf("expected chunked transfer, got Content-Length=%d", h.contentLength)
	}
	if h.transferEncoding != "chunked" {
		t.Errorf("Transfer-Encoding=%q want chunked", h.transferEncoding)
	}
	if !bytes.Equal(h.receivedBody, payload) {
		t.Error("body mismatch")
	}
}

func TestUploadPieceStreaming_PrefilledPieceCIDSkipsTee(t *testing.T) {
	payload := bytes.Repeat([]byte{0xab}, 512)
	want := testPieceInfoV2(t).CIDv2

	h := &streamingUploadHandler{t: t}
	c, _ := newTestClient(t, h)
	res, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader(payload), UploadPieceStreamingOptions{
		Size:     int64(len(payload)),
		PieceCID: want,
	})
	if err != nil {
		t.Fatalf("UploadPieceStreaming: %v", err)
	}
	if res.PieceCID != want {
		t.Errorf("PieceCID=%s want %s", res.PieceCID, want)
	}
	if !bytes.Equal(h.receivedBody, payload) {
		t.Error("body mismatch (tee branch corruption?)")
	}
	if h.finalizePieceCID != want.String() {
		t.Errorf("finalize pieceCid=%q want %s", h.finalizePieceCID, want)
	}
}

func TestUploadPieceStreaming_OnProgressMonotonic(t *testing.T) {
	payload := bytes.Repeat([]byte{0x11}, 4096)
	h := &streamingUploadHandler{t: t}
	c, _ := newTestClient(t, h)

	var seen []int64
	_, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader(payload), UploadPieceStreamingOptions{
		Size: int64(len(payload)),
		OnProgress: func(n int64) {
			seen = append(seen, n)
		},
	})
	if err != nil {
		t.Fatalf("UploadPieceStreaming: %v", err)
	}
	if len(seen) == 0 {
		t.Fatal("OnProgress never called")
	}
	for i := 1; i < len(seen); i++ {
		if seen[i] < seen[i-1] {
			t.Fatalf("OnProgress not monotonic: %v", seen)
		}
	}
	if seen[len(seen)-1] != int64(len(payload)) {
		t.Errorf("final progress=%d want %d", seen[len(seen)-1], len(payload))
	}
}

func TestUploadPieceStreaming_PutFailure(t *testing.T) {
	h := &streamingUploadHandler{t: t, putStatus: http.StatusInternalServerError}
	c, _ := newTestClient(t, h)
	_, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader([]byte("data")), UploadPieceStreamingOptions{
		Size: 4,
	})
	if err == nil || !strings.Contains(err.Error(), "PUT") {
		t.Fatalf("want PUT error, got %v", err)
	}
}

func TestUploadPieceStreaming_FinalizeFailure(t *testing.T) {
	h := &streamingUploadHandler{t: t, finalizeStatus: http.StatusBadRequest}
	c, _ := newTestClient(t, h)
	_, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader(bytes.Repeat([]byte{0xab}, 512)), UploadPieceStreamingOptions{
		Size: 512,
	})
	if err == nil || !strings.Contains(err.Error(), "finalize") {
		t.Fatalf("want finalize error, got %v", err)
	}
}

func TestUploadPieceStreaming_CreateMissingLocation(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/pdp/piece/uploads" {
			w.WriteHeader(http.StatusCreated)
			return
		}
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	_, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader([]byte("x")), UploadPieceStreamingOptions{})
	if !errors.Is(err, ErrLocationHeader) {
		t.Fatalf("want ErrLocationHeader, got %v", err)
	}
}

func TestUploadPieceStreaming_NilReader(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	_, err := c.UploadPieceStreaming(context.Background(), nil, UploadPieceStreamingOptions{})
	if err == nil || !strings.Contains(err.Error(), "nil data reader") {
		t.Fatalf("want nil reader error, got %v", err)
	}
}

func TestUploadPieceStreaming_SizeExceedsMax(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	_, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader([]byte("x")), UploadPieceStreamingOptions{
		Size: chain.MaxUploadSize + 1,
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("want size-exceeded error, got %v", err)
	}
}

func TestUploadPieceStreaming_RejectsPieceCIDv1(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
	}))
	v1 := testPieceInfoV2(t).CIDv1
	_, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader([]byte("x")), UploadPieceStreamingOptions{
		PieceCID: v1,
	})
	if err == nil || !strings.Contains(err.Error(), "PieceCIDv2") {
		t.Fatalf("want PieceCIDv2 validation error, got %v", err)
	}
}

func TestUploadPieceStreaming_DoesNotUseDefaultClientTimeout(t *testing.T) {
	var putDeadlineOK bool
	var createDone, putDone bool
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pdp/piece/uploads":
			createDone = true
			h := make(http.Header)
			h.Set("Location", "/pdp/piece/uploads/u1")
			return &http.Response{StatusCode: http.StatusCreated, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
		case r.Method == http.MethodPut && r.URL.Path == "/pdp/piece/uploads/u1":
			_, hasDeadline := r.Context().Deadline()
			putDeadlineOK = !hasDeadline
			putDone = true
			_, _ = io.Copy(io.Discard, r.Body)
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		case r.Method == http.MethodPost && r.URL.Path == "/pdp/piece/uploads/u1":
			_, _ = io.Copy(io.Discard, r.Body)
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return nil, fmt.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	})
	c, err := New("https://example.com", WithHTTPClient(&http.Client{Timeout: DefaultHTTPTimeout, Transport: rt}))
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0xab}, 512)
	if _, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader(payload), UploadPieceStreamingOptions{Size: int64(len(payload))}); err != nil {
		t.Fatalf("UploadPieceStreaming: %v", err)
	}
	if !createDone || !putDone {
		t.Fatalf("createDone=%v putDone=%v", createDone, putDone)
	}
	if !putDeadlineOK {
		t.Error("PUT request inherited client timeout deadline")
	}
}

func TestFindPiece_FoundAndMissing(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("pieceCid") == "" {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		if strings.Contains(r.URL.Query().Get("pieceCid"), "unknown") {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"pieceCid":%q}`, r.URL.Query().Get("pieceCid"))
	}))
	pc := testPieceInfoV2(t).CIDv2
	got, err := c.FindPiece(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	if got.PieceCID != pc.String() {
		t.Errorf("got %s", got.PieceCID)
	}
	// Not-found path: inject a bogus CID string path by hand via httptest.
	// Use a second server that always 404s.
	c2, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	}))
	if _, err := c2.FindPiece(context.Background(), pc); !errors.Is(err, ErrPieceNotFound) {
		t.Fatalf("want ErrPieceNotFound, got %v", err)
	}
}

func TestFindPiece_RejectsV1(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	pc := testPieceInfoV2(t).CIDv1
	if _, err := c.FindPiece(context.Background(), pc); err == nil || !strings.Contains(err.Error(), "PieceCIDv2") {
		t.Fatalf("want PieceCIDv2 validation error, got %v", err)
	}
}

func TestFindPiece_Processing(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusAccepted)
	}))
	pc := testPieceInfoV2(t).CIDv2
	if _, err := c.FindPiece(context.Background(), pc); !errors.Is(err, ErrPieceProcessing) {
		t.Fatalf("want ErrPieceProcessing, got %v", err)
	}
}

func TestWaitForPieceParked(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"pieceCid":%q}`, r.URL.Query().Get("pieceCid"))
	}))
	pc := testPieceInfoV2(t).CIDv2
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.WaitForPieceParked(ctx, pc, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestWaitForPieceParked_RetriesProcessing(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			http.Error(w, "", http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"pieceCid":%q}`, r.URL.Query().Get("pieceCid"))
	}))
	pc := testPieceInfoV2(t).CIDv2
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.WaitForPieceParked(ctx, pc, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestWaitForPieceParked_RejectsV1(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	pc := testPieceInfoV2(t).CIDv1
	if err := c.WaitForPieceParked(context.Background(), pc, time.Millisecond); err == nil || !strings.Contains(err.Error(), "PieceCIDv2") {
		t.Fatalf("want PieceCIDv2 validation error, got %v", err)
	}
}

func TestCreateDataSet_Success(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pdp/data-sets" {
			t.Fatalf("bad req: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Location", "/pdp/data-sets/created/0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
		w.WriteHeader(http.StatusCreated)
	}))
	rk := common.HexToAddress("0x1111111111111111111111111111111111111111")
	res, err := c.CreateDataSet(context.Background(), rk, []byte{0xde, 0xad, 0xbe, 0xef})
	if err != nil {
		t.Fatal(err)
	}
	want := common.HexToHash("0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
	if res.TxHash != want {
		t.Errorf("tx=%s", res.TxHash.Hex())
	}
	if !strings.Contains(res.StatusURL, "/pdp/data-sets/created/") {
		t.Errorf("status url=%s", res.StatusURL)
	}
}

func TestCreateDataSet_RootRelativeLocationPreservesBaseOrigin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/pdp/data-sets" {
			t.Fatalf("bad path: %s", r.URL.Path)
		}
		w.Header().Set("Location", "/pdp/data-sets/created/0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, err := New(srv.URL + "/prefix")
	if err != nil {
		t.Fatal(err)
	}
	rk := common.HexToAddress("0x1111111111111111111111111111111111111111")
	res, err := c.CreateDataSet(context.Background(), rk, []byte{0xde, 0xad})
	if err != nil {
		t.Fatal(err)
	}
	want := srv.URL + "/pdp/data-sets/created/0xabcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	if res.StatusURL != want {
		t.Fatalf("StatusURL = %s, want %s", res.StatusURL, want)
	}
}

func TestCreateDataSet_ZeroArgs(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	if _, err := c.CreateDataSet(context.Background(), common.Address{}, []byte{1}); err == nil {
		t.Error("zero recordKeeper should error")
	}
	if _, err := c.CreateDataSet(context.Background(), common.HexToAddress("0x01"), nil); err == nil {
		t.Error("empty extraData should error")
	}
}

func TestWaitForDataSetCreated(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = fmt.Fprint(w, `{"createMessageHash":"0x1","service":"svc","txStatus":"pending","dataSetCreated":false,"ok":null}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"createMessageHash":"0x1","service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":42}`)
	}))
	status, err := c.WaitForDataSetCreated(context.Background(), c.BaseURL().String()+"pdp/data-sets/created/0x1", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if status.DataSetID == nil || !status.DataSetID.Equal(types.NewBigInt(42)) {
		t.Fatalf("id=%v", status.DataSetID)
	}
}

func TestWaitForDataSetCreated_ConfirmedFalseStillPending(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = fmt.Fprint(w, `{"createMessageHash":"0x1","service":"svc","txStatus":"confirmed","dataSetCreated":false,"ok":null}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"createMessageHash":"0x1","service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":42}`)
	}))
	status, err := c.WaitForDataSetCreated(context.Background(), c.BaseURL().String()+"pdp/data-sets/created/0x1", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("expected multiple polls, got %d", calls)
	}
	if status.DataSetID == nil || !status.DataSetID.Equal(types.NewBigInt(42)) {
		t.Fatalf("id=%v", status.DataSetID)
	}
}

func TestWaitForDataSetCreated_Rejected(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"createMessageHash":"0x1","service":"svc","txStatus":"rejected","dataSetCreated":false,"ok":false}`)
	}))
	_, err := c.WaitForDataSetCreated(context.Background(), c.BaseURL().String()+"pdp/data-sets/created/0x1", 10*time.Millisecond)
	if !errors.Is(err, ErrTxRejected) {
		t.Fatalf("want ErrTxRejected, got %v", err)
	}
}

func TestWaitForDataSetCreated_404ReturnsHTTPError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	}))
	_, err := c.WaitForDataSetCreated(context.Background(), c.BaseURL().String()+"pdp/data-sets/created/0x1", 10*time.Millisecond)
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want HTTPError, got %T (%v)", err, err)
	}
	if he.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", he.StatusCode)
	}
}

func TestGetDataSet(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pdp/data-sets/7" {
			t.Errorf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"id":7,"nextChallengeEpoch":1000,"pieces":[{"pieceCid":"bafy","pieceId":1,"subPieceCid":"bafy","subPieceOffset":0}]}`)
	}))
	ds, err := c.GetDataSet(context.Background(), types.NewBigInt(7))
	if err != nil {
		t.Fatal(err)
	}
	if !ds.ID.Equal(types.NewBigInt(7)) || len(ds.Pieces) != 1 {
		t.Fatalf("%+v", ds)
	}
}

func TestDataSetJSONPreservesIDNumbers(t *testing.T) {
	raw := []byte(`{"id":7,"nextChallengeEpoch":1000,"pieces":[{"pieceCid":"bafy","pieceId":1,"subPieceCid":"bafy","subPieceOffset":0}]}`)
	var ds DataSet
	if err := json.Unmarshal(raw, &ds); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !ds.ID.Equal(types.NewBigInt(7)) || len(ds.Pieces) != 1 || !ds.Pieces[0].PieceID.Equal(types.NewBigInt(1)) {
		t.Fatalf("decoded dataset=%+v", ds)
	}
	out, err := json.Marshal(ds)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !bytes.Contains(out, []byte(`"id":7`)) || !bytes.Contains(out, []byte(`"pieceId":1`)) {
		t.Fatalf("json=%s", out)
	}
	if bytes.Contains(out, []byte(`"id":"7"`)) || bytes.Contains(out, []byte(`"pieceId":"1"`)) {
		t.Fatalf("ids should marshal as JSON numbers: %s", out)
	}
}

func TestAddPieces(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pdp/data-sets/5/pieces" {
			t.Fatalf("bad req: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Location", "/pdp/data-sets/5/pieces/added/0xdead000000000000000000000000000000000000000000000000000000000000")
		w.WriteHeader(http.StatusCreated)
	}))
	pcInfo, _ := piece.CalculateFromBytes([]byte("hi"))
	pc := pcInfo.CIDv1
	res, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: pc}}, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	want := common.HexToHash("0xdead000000000000000000000000000000000000000000000000000000000000")
	if res.TxHash != want {
		t.Errorf("tx=%s", res.TxHash.Hex())
	}
}

func TestAddPieces_RootRelativeLocationPreservesBaseOrigin(t *testing.T) {
	pcInfo, _ := piece.CalculateFromBytes([]byte("hi"))
	pc := pcInfo.CIDv1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/prefix/pdp/data-sets/5/pieces" {
			t.Fatalf("bad path: %s", r.URL.Path)
		}
		w.Header().Set("Location", "/pdp/data-sets/5/pieces/added/0xdead000000000000000000000000000000000000000000000000000000000000")
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	c, err := New(srv.URL + "/prefix")
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: pc}}, []byte{1, 2, 3})
	if err != nil {
		t.Fatal(err)
	}
	want := srv.URL + "/pdp/data-sets/5/pieces/added/0xdead000000000000000000000000000000000000000000000000000000000000"
	if res.StatusURL != want {
		t.Fatalf("StatusURL = %s, want %s", res.StatusURL, want)
	}
}

func TestWaitForPiecesAdded(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"pending","dataSetId":5,"pieceCount":1,"addMessageOk":null,"piecesAdded":false}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[10,11]}`)
	}))
	status, err := c.WaitForPiecesAdded(context.Background(), c.BaseURL().String()+"status", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.ConfirmedPieceIDs) != 2 {
		t.Fatalf("len=%d", len(status.ConfirmedPieceIDs))
	}
}

func TestWaitForPiecesAdded_ConfirmedFalseStillPending(t *testing.T) {
	var calls int
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":null,"piecesAdded":false}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[10,11]}`)
	}))
	status, err := c.WaitForPiecesAdded(context.Background(), c.BaseURL().String()+"status", 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("expected multiple polls, got %d", calls)
	}
	if len(status.ConfirmedPieceIDs) != 2 {
		t.Fatalf("len=%d", len(status.ConfirmedPieceIDs))
	}
}

func TestWaitForPiecesAdded_404ReturnsHTTPError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusNotFound)
	}))
	_, err := c.WaitForPiecesAdded(context.Background(), c.BaseURL().String()+"status", 10*time.Millisecond)
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want HTTPError, got %T (%v)", err, err)
	}
	if he.StatusCode != http.StatusNotFound {
		t.Fatalf("status=%d", he.StatusCode)
	}
}

func TestGetAddPiecesStatus_LargeUint64DataSetID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":9223372036854775808,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[10]}`)
	}))
	status, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status")
	if err != nil {
		t.Fatal(err)
	}
	want, err := types.ParseBigInt("9223372036854775808")
	if err != nil {
		t.Fatal(err)
	}
	if !status.DataSetID.Equal(want) {
		t.Fatalf("DataSetID = %s", status.DataSetID.String())
	}
}

func TestSchedulePieceDeletion(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/pdp/data-sets/5/pieces/9" {
			t.Fatalf("bad req: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0xabc0000000000000000000000000000000000000000000000000000000000000"}`)
	}))
	h, err := c.SchedulePieceDeletion(context.Background(), types.NewBigInt(5), types.NewBigInt(9), []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if h == (common.Hash{}) {
		t.Fatal("zero hash")
	}
}

func TestLastPathSegment(t *testing.T) {
	cases := map[string]string{
		"/pdp/piece/upload/abc":                       "abc",
		"http://x.example/pdp/data-sets/created/0xfe": "0xfe",
		"abc":         "abc",
		"/trail/":     "trail",
		"":            "",
		"/":           "",
		"just/a/path": "path",
	}
	for in, want := range cases {
		if got := lastPathSegment(in); got != want {
			t.Errorf("lastPathSegment(%q)=%q want %q", in, got, want)
		}
	}
}

// ---- retry tests ----

// timeoutError implements net.Error with Timeout()=true, simulating a transport
// timeout (e.g. client.Timeout exceeded).
type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context canceled", context.Canceled, false},
		{"context deadline", context.DeadlineExceeded, false},
		{"HTTP 500", &HTTPError{StatusCode: 500}, true},
		{"HTTP 503", &HTTPError{StatusCode: 503}, true},
		{"HTTP 429", &HTTPError{StatusCode: 429}, true},
		{"HTTP 404", &HTTPError{StatusCode: 404}, false},
		{"HTTP 400", &HTTPError{StatusCode: 400}, false},
		{"HTTP 501", &HTTPError{StatusCode: 501}, false},
		// Unknown error types are NOT retried anymore (was "optimistic retry").
		{"unknown error (no retry)", errors.New("connection reset"), false},
		// Curated transient errors via errors.Is.
		{"syscall ECONNREFUSED", syscall.ECONNREFUSED, true},
		{"syscall ECONNRESET", syscall.ECONNRESET, true},
		{"syscall EPIPE", syscall.EPIPE, true},
		{"io.ErrUnexpectedEOF", io.ErrUnexpectedEOF, true},
		{"HTTP2 GOAWAY value", http2.GoAwayError{LastStreamID: 1, ErrCode: http2.ErrCodeNo}, true},
		{"HTTP2 stream refused value", http2.StreamError{StreamID: 1, Code: http2.ErrCodeRefusedStream}, true},
		// url.Error wrapping transient syscall error — retry.
		{"url.Error+ECONNRESET", &url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "read", Err: syscall.ECONNRESET}}, true},
		// url.Error wrapping a plain (non-OpError) error — permanent, no retry.
		{"url.Error+plain error", &url.Error{Op: "Get", URL: "http://x", Err: errors.New("some permanent failure")}, false},
		// TLS handshake rejections are permanent — bad/expired cert.
		{"url.Error+net.OpError(TLS alert)", &url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "remote error", Err: tls.AlertError(42)}}, false},
		// DNS: temporary/timeout are retried; permanent lookup failures are not.
		{"DNS temporary", &net.DNSError{Err: "server misbehaving", Name: "x", IsTemporary: true}, true},
		{"DNS timeout", &net.DNSError{Err: "i/o timeout", Name: "x", IsTimeout: true}, true},
		{"DNS permanent (NXDOMAIN)", &net.DNSError{Err: "no such host", Name: "x", IsNotFound: true}, false},
		// Timeout on url.Error still retries (e.g. client.Timeout exceeded).
		{"url.Error timeout", &url.Error{Op: "Get", URL: "http://x", Err: timeoutError{}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isRetryable(tc.err); got != tc.want {
				t.Errorf("isRetryable(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestHTTPRetryDelay(t *testing.T) {
	// 429 with explicit Retry-After takes precedence.
	err429 := &HTTPError{StatusCode: 429, RetryAfter: 7 * time.Second}
	if d := httpRetryDelay(err429, 0); d != 7*time.Second {
		t.Errorf("expected 7s from Retry-After, got %v", d)
	}
	// 503 with Retry-After is also honoured.
	err503 := &HTTPError{StatusCode: 503, RetryAfter: 10 * time.Second}
	if d := httpRetryDelay(err503, 0); d != 10*time.Second {
		t.Errorf("expected 10s from 503 Retry-After, got %v", d)
	}
	// Retry-After is capped at 30s even if the server sends a larger value.
	err429Large := &HTTPError{StatusCode: 429, RetryAfter: 60 * time.Second}
	if d := httpRetryDelay(err429Large, 0); d != 30*time.Second {
		t.Errorf("expected Retry-After capped at 30s, got %v", d)
	}
	// 429 without Retry-After falls back to exponential backoff.
	err429NoHeader := &HTTPError{StatusCode: 429}
	if d := httpRetryDelay(err429NoHeader, 0); d != 1*time.Second {
		t.Errorf("expected 1s exponential at attempt 0, got %v", d)
	}
	// Exponential progression.
	netErr := errors.New("network error")
	if d := httpRetryDelay(netErr, 1); d != 2*time.Second {
		t.Errorf("expected 2s at attempt 1, got %v", d)
	}
	if d := httpRetryDelay(netErr, 2); d != 4*time.Second {
		t.Errorf("expected 4s at attempt 2, got %v", d)
	}
	// Cap at 30s.
	if d := httpRetryDelay(netErr, 10); d != 30*time.Second {
		t.Errorf("expected cap 30s at attempt 10, got %v", d)
	}
	// Very large attempt values must not overflow — must still return ≤30s.
	if d := httpRetryDelay(netErr, 1000); d > 30*time.Second || d <= 0 {
		t.Errorf("expected positive ≤30s for large attempt, got %v", d)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Now().UTC()
	tests := []struct {
		input   string
		wantMin time.Duration
		wantMax time.Duration
	}{
		{"", 0, 0},
		{"0", 0, 0},
		{"5", 5 * time.Second, 5 * time.Second},
		{"120", 120 * time.Second, 120 * time.Second},
		{"bad", 0, 0},
		// HTTP-date format (future) — should return approximately the remaining duration.
		{now.Add(10 * time.Second).Format(http.TimeFormat), 8 * time.Second, 11 * time.Second},
		// HTTP-date format (past) — should return 0 (fall back to exponential backoff).
		{now.Add(-5 * time.Second).Format(http.TimeFormat), 0, 0},
	}
	for _, tc := range tests {
		got := parseRetryAfter(tc.input)
		if got < tc.wantMin || got > tc.wantMax {
			t.Errorf("parseRetryAfter(%q) = %v, want [%v, %v]", tc.input, got, tc.wantMin, tc.wantMax)
		}
	}
}

func TestNewHTTPError_RetryAfter(t *testing.T) {
	makeResp := func(code int, retryAfter string) *http.Response {
		h := http.Header{}
		if retryAfter != "" {
			h.Set("Retry-After", retryAfter)
		}
		return &http.Response{StatusCode: code, Header: h}
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)

	// 429 with Retry-After.
	e := newHTTPError(req, makeResp(http.StatusTooManyRequests, "10"), nil)
	if e.RetryAfter != 10*time.Second {
		t.Errorf("429: expected RetryAfter=10s, got %v", e.RetryAfter)
	}
	// 503 with Retry-After.
	e = newHTTPError(req, makeResp(http.StatusServiceUnavailable, "20"), nil)
	if e.RetryAfter != 20*time.Second {
		t.Errorf("503: expected RetryAfter=20s, got %v", e.RetryAfter)
	}
	// 500 without Retry-After — RetryAfter should be zero.
	e = newHTTPError(req, makeResp(http.StatusInternalServerError, ""), nil)
	if e.RetryAfter != 0 {
		t.Errorf("500: expected RetryAfter=0, got %v", e.RetryAfter)
	}
}

func noRetryDelay(_ error, _ int) time.Duration { return 0 }

func TestDoRetryable_RetriesOn5xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.retryDelayFn = noRetryDelay

	if err = c.getJSON(context.Background(), "", nil); err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestDoRetryable_NoRetryOn4xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c.retryDelayFn = noRetryDelay
	_ = c.getJSON(context.Background(), "", nil)
	if attempts != 1 {
		t.Errorf("4xx should not be retried: expected 1 attempt, got %d", attempts)
	}
}

func TestDoRetryable_429WithRetryAfter(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "60")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	// Zero-delay but verify Retry-After is parsed into HTTPError.
	c.retryDelayFn = func(err error, attempt int) time.Duration {
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusTooManyRequests {
			if httpErr.RetryAfter != 60*time.Second {
				t.Errorf("expected RetryAfter=60s, got %v", httpErr.RetryAfter)
			}
		}
		return 0
	}
	if err = c.getJSON(context.Background(), "", nil); err != nil {
		t.Fatalf("expected success after 429+retry, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestDoRetryable_ExhaustsRetries(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithMaxRetries(2))
	if err != nil {
		t.Fatal(err)
	}
	c.retryDelayFn = noRetryDelay
	if err = c.getJSON(context.Background(), "", nil); err == nil {
		t.Fatal("expected error after max retries")
	}
	// initial attempt + 2 retries = 3 total
	if attempts != 3 {
		t.Errorf("expected 3 attempts (1 + 2 retries), got %d", attempts)
	}
}

func TestDoRetryable_DisabledWhenZero(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	_ = c.getJSON(context.Background(), "", nil)
	if attempts != 1 {
		t.Errorf("retry disabled: expected 1 attempt, got %d", attempts)
	}
}

func TestDoRetryable_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithMaxRetries(10))
	if err != nil {
		t.Fatal(err)
	}
	c.retryDelayFn = noRetryDelay
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	err = c.getJSON(ctx, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// TestDoRetryable_ContextCancellationDuringSleep ensures that cancelling the
// context while doRetryable is sleeping between retries returns context.Canceled.
// This exercises the `select { case <-ctx.Done() }` branch, which the pre-cancel
// test above never reaches.
func TestDoRetryable_ContextCancellationDuringSleep(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithMaxRetries(5))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Cancel the context inside retryDelayFn, before the sleep select runs.
	c.retryDelayFn = func(err error, attempt int) time.Duration {
		cancel()
		return 100 * time.Millisecond
	}
	err = c.getJSON(ctx, "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

func TestWithMaxRetries_NegativeClampsToZero(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	// Negative value must behave identically to WithMaxRetries(0): no retry,
	// exactly one attempt, and a proper error returned (not silent success).
	c, err := New(srv.URL, WithMaxRetries(-5))
	if err != nil {
		t.Fatal(err)
	}
	gotErr := c.getJSON(context.Background(), "", nil)
	if gotErr == nil {
		t.Fatal("expected error, got nil (silent success is a bug)")
	}
	if attempts != 1 {
		t.Errorf("expected exactly 1 attempt with negative maxRetries, got %d", attempts)
	}
}

// ---- B2 tests ----

// TestPostJSON_NoRetry verifies that POST requests are executed exactly once
// even when the server returns a retryable status code. POST is not
// idempotent; retrying on 5xx after a partial server-side processing could
// cause duplicate submissions.
func TestPostJSON_NoRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	c.retryDelayFn = noRetryDelay
	_, _, err = c.postJSON(context.Background(), "", map[string]string{"a": "b"})
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
	if attempts != 1 {
		t.Errorf("POST must not retry on 5xx: expected 1 attempt, got %d", attempts)
	}
}

// TestDeleteJSON_NoRetry verifies that DELETE requests are executed exactly
// once.
func TestDeleteJSON_NoRetry(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithMaxRetries(3))
	if err != nil {
		t.Fatal(err)
	}
	c.retryDelayFn = noRetryDelay
	err = c.deleteJSON(context.Background(), "", nil, nil)
	if err == nil {
		t.Fatal("expected error from 503 response")
	}
	if attempts != 1 {
		t.Errorf("DELETE must not retry on 5xx: expected 1 attempt, got %d", attempts)
	}
}

// TestDoWithClient_RejectsOversizeBody verifies that control-plane responses
// larger than MaxControlResponseBytes are rejected to protect against
// unbounded or hostile servers.
func TestDoWithClient_RejectsOversizeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Stream just over the limit without allocating full cap up front.
		buf := make([]byte, 1<<20)
		for i := 0; i < (MaxControlResponseBytes/len(buf))+2; i++ {
			_, _ = w.Write(buf)
		}
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	err = c.getJSON(context.Background(), "", nil)
	if err == nil {
		t.Fatal("expected error from oversized response")
	}
	if !strings.Contains(err.Error(), "MaxControlResponseBytes") {
		t.Errorf("expected error to mention MaxControlResponseBytes, got: %v", err)
	}
}

// TestDoWithClient_AllowsExactBoundary verifies a response sized exactly
// at the limit is accepted (off-by-one regression guard).
func TestDoWithClient_AllowsExactBoundary(t *testing.T) {
	// Use a small cap via a dedicated path trick: we can't change the constant
	// at test time, so construct a response just under the limit. Use 1 MiB of
	// JSON padding to keep the test fast.
	body := `{"pad":"` + strings.Repeat("x", 1<<20) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	var dst map[string]string
	if err := c.getJSON(context.Background(), "", &dst); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dst["pad"]) != 1<<20 {
		t.Errorf("body not decoded fully: got pad len %d", len(dst["pad"]))
	}
}
