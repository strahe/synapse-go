package curio

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/piece"
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

func TestUploadPiece_AlreadyExists(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pdp/piece" && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	pc := testPieceInfoV2(t).CIDv2
	res, err := c.UploadPiece(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	if !res.AlreadyExists {
		t.Fatal("expected AlreadyExists")
	}
}

func TestUploadPiece_Created(t *testing.T) {
	c, srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pdp/piece":
			w.Header().Set("Location", "/pdp/piece/upload/abc-123")
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
		}
	}))
	_ = srv
	pc := testPieceInfoV2(t).CIDv2
	res, err := c.UploadPiece(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	if res.UploadUUID != "abc-123" {
		t.Fatalf("uuid=%q", res.UploadUUID)
	}
}

func TestUploadPiece_MissingLocation(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // no Location
	}))
	pc := testPieceInfoV2(t).CIDv2
	_, err := c.UploadPiece(context.Background(), pc)
	if !errors.Is(err, ErrLocationHeader) {
		t.Fatalf("want ErrLocationHeader, got %v", err)
	}
}

func TestUploadPiece_RejectsV1(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	pc := testPieceInfoV2(t).CIDv1
	if _, err := c.UploadPiece(context.Background(), pc); err == nil || !strings.Contains(err.Error(), "PieceCIDv2") {
		t.Fatalf("want PieceCIDv2 validation error, got %v", err)
	}
}

func TestUploadPieceBytes_OK(t *testing.T) {
	payload := []byte("the-bytes")
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || !strings.HasPrefix(r.URL.Path, "/pdp/piece/upload/") {
			t.Fatalf("bad req: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("content-type: %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != string(payload) {
			t.Errorf("body=%q", body)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	if err := c.UploadPieceBytes(context.Background(), "abc-123", strings.NewReader(string(payload)), int64(len(payload))); err != nil {
		t.Fatal(err)
	}
}

func TestUploadPieceBytes_DoesNotUseDefaultClientTimeout(t *testing.T) {
	c, err := New("https://example.com", WithHTTPClient(&http.Client{
		Timeout: DefaultHTTPTimeout,
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if _, ok := r.Context().Deadline(); ok {
				t.Fatal("upload request inherited client timeout deadline")
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Body:       io.NopCloser(strings.NewReader("")),
				Header:     make(http.Header),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.UploadPieceBytes(context.Background(), "upload-1", strings.NewReader("payload"), int64(len("payload"))); err != nil {
		t.Fatal(err)
	}
}

func TestUploadPieceFromBytes_FullFlow(t *testing.T) {
	var putCalled bool
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pdp/piece":
			w.Header().Set("Location", "/pdp/piece/upload/uuid-xyz")
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodPut && r.URL.Path == "/pdp/piece/upload/uuid-xyz":
			putCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	payload := bytes.Repeat([]byte{0xab}, 512)
	pc := testPieceInfoV2(t).CIDv2
	res, err := c.UploadPieceFromBytes(context.Background(), pc, payload)
	if err != nil {
		t.Fatal(err)
	}
	if res.AlreadyExists {
		t.Fatal("expected new upload")
	}
	if !putCalled {
		t.Fatal("PUT not called")
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
	if status.DataSetID == nil || status.DataSetID.Int64() != 42 {
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
	if status.DataSetID == nil || status.DataSetID.Int64() != 42 {
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
	ds, err := c.GetDataSet(context.Background(), 7)
	if err != nil {
		t.Fatal(err)
	}
	if ds.ID != 7 || len(ds.Pieces) != 1 {
		t.Fatalf("%+v", ds)
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
	res, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: pc}}, []byte{1, 2, 3})
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
	res, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: pc}}, []byte{1, 2, 3})
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
	if status.DataSetID != 9223372036854775808 {
		t.Fatalf("DataSetID = %d", status.DataSetID)
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
	h, err := c.SchedulePieceDeletion(context.Background(), 5, 9, []byte{1})
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

// tempNetError is a test helper implementing the Temporary() interface,
// representing a transient network error (e.g. ECONNRESET, EAGAIN).
type tempNetError struct{}

func (tempNetError) Error() string   { return "temporary network error" }
func (tempNetError) Temporary() bool { return true }

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
		{"network error (optimistic retry)", errors.New("connection reset"), true},
		// url.Error wrapping net.OpError with a transient inner error — retry.
		// Uses tempNetError which implements Temporary()=true (like syscall.EAGAIN).
		{"url.Error+net.OpError(transient)", &url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "read", Err: tempNetError{}}}, true},
		// url.Error wrapping a plain (non-OpError) error — permanent, no retry.
		{"url.Error+plain error", &url.Error{Op: "Get", URL: "http://x", Err: errors.New("some permanent failure")}, false},
		// url.Error wrapping *net.OpError with TLS alert — real Go TLS handshake
		// rejections arrive as *url.Error{Err: *net.OpError{Op:"remote error",
		// Err: tls.AlertError}}. tls.AlertError does not implement Temporary(),
		// so opErr.Temporary() returns false — correctly not retried.
		{"url.Error+net.OpError(TLS alert)", &url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "remote error", Err: tls.AlertError(42)}}, false},
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
