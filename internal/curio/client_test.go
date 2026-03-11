package curio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	return c, srv
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
	_, err := c.UploadPiece(context.Background(), pc)
	if !errors.Is(err, ErrLocationHeader) {
		t.Fatalf("want ErrLocationHeader, got %v", err)
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
	res, err := c.UploadPieceFromBytes(context.Background(), pc, []byte("hi"))
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
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

func TestFindPiece_Processing(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", http.StatusAccepted)
	}))
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.WaitForPieceParked(ctx, pc, 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if calls < 2 {
		t.Fatalf("calls=%d", calls)
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
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
	pc, _, _ := piece.CalculateFromBytes([]byte("hi"))
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
