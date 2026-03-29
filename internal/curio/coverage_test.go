package curio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/piece"
)

// ---------- client option tests ----------

func TestWithUserAgent(t *testing.T) {
	uaCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uaCh <- r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithUserAgent("my-agent/1.0"))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := <-uaCh; got != "my-agent/1.0" {
		t.Errorf("User-Agent = %q, want %q", got, "my-agent/1.0")
	}
}

func TestWithUserAgent_Empty(t *testing.T) {
	c, err := New("https://example.com", WithUserAgent(""))
	if err != nil {
		t.Fatal(err)
	}
	if c.userAgent != DefaultUserAgent {
		t.Errorf("empty ua should keep default, got %q", c.userAgent)
	}
}

func TestWithLogger(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c, err := New(srv.URL, WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "curio request") {
		t.Errorf("expected log output, got %q", buf.String())
	}
}

func TestWithLogger_Nil(t *testing.T) {
	c, err := New("https://example.com", WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.logger != nil {
		t.Error("nil logger should set nil")
	}
}

// ---------- HTTPError tests ----------

func TestHTTPError_WithBody(t *testing.T) {
	e := &HTTPError{Method: "GET", URL: "http://x/path", StatusCode: 500, Body: "server error"}
	got := e.Error()
	if !strings.Contains(got, "500") || !strings.Contains(got, "server error") {
		t.Errorf("Error() = %q", got)
	}
}

func TestHTTPError_WithoutBody(t *testing.T) {
	e := &HTTPError{Method: "POST", URL: "http://x/path", StatusCode: 404, Body: ""}
	got := e.Error()
	if !strings.Contains(got, "404") || strings.Contains(got, ":  ") {
		t.Errorf("Error() = %q", got)
	}
}

// ---------- CreateDataSetAndAddPieces validation ----------

func TestCreateDataSetAndAddPieces_ZeroRecordKeeper(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.CreateDataSetAndAddPieces(context.Background(), common.Address{}, nil, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "zero recordKeeper") {
		t.Errorf("want zero recordKeeper error, got %v", err)
	}
}

func TestCreateDataSetAndAddPieces_EmptyPieces(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSetAndAddPieces(context.Background(), rk, nil, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "no pieces") {
		t.Errorf("want no pieces error, got %v", err)
	}
}

func TestCreateDataSetAndAddPieces_EmptyExtraData(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	rk := common.HexToAddress("0x01")
	info, _ := piece.CalculateFromBytes(make([]byte, 256))
	_, err := c.CreateDataSetAndAddPieces(context.Background(), rk, []AddPieceInput{{PieceCID: info.CIDv2}}, nil)
	if err == nil || !strings.Contains(err.Error(), "empty extraData") {
		t.Errorf("want empty extraData error, got %v", err)
	}
}

func TestCreateDataSetAndAddPieces_UndefinedPieceCID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSetAndAddPieces(context.Background(), rk, []AddPieceInput{{PieceCID: emptyCID()}}, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "undefined pieceCID") {
		t.Errorf("want undefined pieceCID error, got %v", err)
	}
}

func TestCreateDataSetAndAddPieces_BadLocation(t *testing.T) {
	info, _ := piece.CalculateFromBytes(make([]byte, 256))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No Location header
		w.WriteHeader(http.StatusCreated)
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSetAndAddPieces(context.Background(), rk, []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{1})
	if !errors.Is(err, ErrLocationHeader) {
		t.Errorf("want ErrLocationHeader, got %v", err)
	}
}

func TestCreateDataSetAndAddPieces_ZeroTxHash(t *testing.T) {
	info, _ := piece.CalculateFromBytes(make([]byte, 256))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/pdp/data-sets/created/0x0000000000000000000000000000000000000000000000000000000000000000")
		w.WriteHeader(http.StatusCreated)
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSetAndAddPieces(context.Background(), rk, []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{1})
	if !errors.Is(err, ErrLocationHeader) {
		t.Errorf("want ErrLocationHeader for zero hash, got %v", err)
	}
}

// ---------- WaitForCreateDataSetAndAddPieces edge cases ----------

func TestWaitForCreateDataSetAndAddPieces_MissingDataSetID(t *testing.T) {
	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// dataSetCreated=true but no dataSetId
		_, _ = fmt.Fprintf(w, `{"createMessageHash":%q,"service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true}`, txHash)
	}))
	_, err := c.WaitForCreateDataSetAndAddPieces(context.Background(), c.BaseURL().String()+"status", 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "missing dataSetId") {
		t.Errorf("want missing dataSetId error, got %v", err)
	}
}

func TestWaitForCreateDataSetAndAddPieces_ZeroCreateMessageHash(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"createMessageHash":"0x0000000000000000000000000000000000000000000000000000000000000000","service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":42}`)
	}))
	_, err := c.WaitForCreateDataSetAndAddPieces(context.Background(), c.BaseURL().String()+"status", 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "zero CreateMessageHash") {
		t.Errorf("want zero CreateMessageHash error, got %v", err)
	}
}

func TestWaitForCreateDataSetAndAddPieces_WaitError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	_, err := c.WaitForCreateDataSetAndAddPieces(context.Background(), c.BaseURL().String()+"status", 5*time.Millisecond)
	if err == nil {
		t.Error("expected error from WaitForDataSetCreated propagation")
	}
}

// ---------- AddPieces validation ----------

func TestAddPieces_EmptyPieces(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.AddPieces(context.Background(), 5, nil, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "no pieces") {
		t.Errorf("want no pieces error, got %v", err)
	}
}

func TestAddPieces_EmptyExtraData(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	info, _ := piece.CalculateFromBytes(make([]byte, 256))
	_, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: info.CIDv1}}, nil)
	if err == nil || !strings.Contains(err.Error(), "empty extraData") {
		t.Errorf("want empty extraData error, got %v", err)
	}
}

func TestAddPieces_UndefinedPieceCID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: emptyCID()}}, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "undefined pieceCID") {
		t.Errorf("want undefined pieceCID error, got %v", err)
	}
}

func TestAddPieces_BadLocation(t *testing.T) {
	info, _ := piece.CalculateFromBytes([]byte("hi"))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // no Location
	}))
	_, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
	if !errors.Is(err, ErrLocationHeader) {
		t.Errorf("want ErrLocationHeader, got %v", err)
	}
}

func TestAddPieces_ZeroTxHash(t *testing.T) {
	info, _ := piece.CalculateFromBytes([]byte("hi"))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/pdp/data-sets/5/pieces/added/0x0000000000000000000000000000000000000000000000000000000000000000")
		w.WriteHeader(http.StatusCreated)
	}))
	_, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
	if !errors.Is(err, ErrLocationHeader) {
		t.Errorf("want ErrLocationHeader for zero hash, got %v", err)
	}
}

// ---------- UploadPieceFromBytes error paths ----------

func TestUploadPieceFromBytes_UploadError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upload failed", http.StatusInternalServerError)
	}))
	pc := testPieceInfoV2(t).CIDv2
	_, err := c.UploadPieceFromBytes(context.Background(), pc, []byte("data"))
	if err == nil {
		t.Error("expected error from UploadPiece failure")
	}
}

func TestUploadPieceFromBytes_PutError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.Header().Set("Location", "/pdp/piece/upload/uuid-xyz")
			w.WriteHeader(http.StatusCreated)
		case http.MethodPut:
			http.Error(w, "put failed", http.StatusInternalServerError)
		}
	}))
	pc := testPieceInfoV2(t).CIDv2
	_, err := c.UploadPieceFromBytes(context.Background(), pc, []byte("data"))
	if err == nil {
		t.Error("expected error from UploadPieceBytes failure")
	}
}

func TestUploadPieceFromBytes_AlreadyExists(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	pc := testPieceInfoV2(t).CIDv2
	res, err := c.UploadPieceFromBytes(context.Background(), pc, []byte("data"))
	if err != nil {
		t.Fatal(err)
	}
	if !res.AlreadyExists {
		t.Error("expected AlreadyExists")
	}
	if res.UploadUUID != "" {
		t.Errorf("UploadUUID should be empty when AlreadyExists, got %q", res.UploadUUID)
	}
}

// ---------- UploadPieceBytes validation ----------

func TestUploadPieceBytes_EmptyUUID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	err := c.UploadPieceBytes(context.Background(), "", strings.NewReader("data"), 4)
	if err == nil || !strings.Contains(err.Error(), "empty uploadUUID") {
		t.Errorf("want empty uploadUUID error, got %v", err)
	}
}

func TestUploadPieceBytes_NilReader(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	err := c.UploadPieceBytes(context.Background(), "uuid", nil, 4)
	if err == nil || !strings.Contains(err.Error(), "nil data") {
		t.Errorf("want nil data error, got %v", err)
	}
}

func TestUploadPieceBytes_ZeroContentLength(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	err := c.UploadPieceBytes(context.Background(), "uuid", strings.NewReader("data"), 0)
	if err == nil || !strings.Contains(err.Error(), "invalid contentLength") {
		t.Errorf("want invalid contentLength error, got %v", err)
	}
}

func TestUploadPieceBytes_ServerError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "err", http.StatusInternalServerError)
	}))
	err := c.UploadPieceBytes(context.Background(), "uuid-1", strings.NewReader("data"), 4)
	if err == nil {
		t.Error("expected error from server")
	}
}

// ---------- WaitForPieceParked edge cases ----------

func TestWaitForPieceParked_TransportError(t *testing.T) {
	c, err := New("https://example.com", WithHTTPClient(&http.Client{
		Timeout:   100 * time.Millisecond,
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("conn refused") }),
	}))
	if err != nil {
		t.Fatal(err)
	}
	pc := testPieceInfoV2(t).CIDv2
	if err := c.WaitForPieceParked(context.Background(), pc, time.Millisecond); err == nil {
		t.Error("expected transport error")
	}
}

// ---------- WaitForPiecesAdded edge cases ----------

func TestWaitForPiecesAdded_Rejected(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"rejected","dataSetId":5,"pieceCount":1,"addMessageOk":false,"piecesAdded":false}`)
	}))
	_, err := c.WaitForPiecesAdded(context.Background(), c.BaseURL().String()+"status", 10*time.Millisecond)
	if !errors.Is(err, ErrTxRejected) {
		t.Fatalf("want ErrTxRejected, got %v", err)
	}
}

func TestWaitForPiecesAdded_ContextCancelled(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"pending","dataSetId":5,"pieceCount":0,"piecesAdded":false}`)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := c.WaitForPiecesAdded(ctx, c.BaseURL().String()+"status", 50*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

// ---------- SchedulePieceDeletion edge cases ----------

func TestSchedulePieceDeletion_EmptyExtraData(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.SchedulePieceDeletion(context.Background(), 5, 9, nil)
	if err == nil || !strings.Contains(err.Error(), "empty extraData") {
		t.Errorf("want empty extraData error, got %v", err)
	}
}

func TestSchedulePieceDeletion_ZeroTxHash(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000000"}`)
	}))
	_, err := c.SchedulePieceDeletion(context.Background(), 5, 9, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "empty txHash") {
		t.Errorf("want empty txHash error, got %v", err)
	}
}

func TestSchedulePieceDeletion_ServerError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	_, err := c.SchedulePieceDeletion(context.Background(), 5, 9, []byte{1})
	if err == nil {
		t.Error("expected server error")
	}
}

// ---------- GetAddPiecesStatus edge cases ----------

func TestGetAddPiecesStatus_EmptyURL(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.GetAddPiecesStatus(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "empty statusURL") {
		t.Errorf("want empty statusURL error, got %v", err)
	}
}

func TestGetAddPiecesStatus_BadJSON(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{invalid json`)
	}))
	_, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status")
	if err == nil {
		t.Error("expected JSON decode error")
	}
}

func TestGetAddPiecesStatus_BadDataSetID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":1.5,"pieceCount":1,"piecesAdded":true}`)
	}))
	_, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status")
	if err == nil || !strings.Contains(err.Error(), "bad dataSetId") {
		t.Errorf("want bad dataSetId error, got %v", err)
	}
}

func TestGetAddPiecesStatus_BadConfirmedPieceID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":5,"pieceCount":1,"piecesAdded":true,"confirmedPieceIds":[1.5]}`)
	}))
	_, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status")
	if err == nil || !strings.Contains(err.Error(), "bad confirmedPieceId") {
		t.Errorf("want bad confirmedPieceId error, got %v", err)
	}
}

// ---------- GetDataSetCreationStatus edge cases ----------

func TestGetDataSetCreationStatus_EmptyURL(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.GetDataSetCreationStatus(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "empty statusURL") {
		t.Errorf("want empty statusURL error, got %v", err)
	}
}

func TestGetDataSetCreationStatus_BadJSON(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `not json`)
	}))
	_, err := c.GetDataSetCreationStatus(context.Background(), c.BaseURL().String()+"status")
	if err == nil {
		t.Error("expected JSON decode error")
	}
}

func TestGetDataSetCreationStatus_BadDataSetID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"createMessageHash":"0x1","service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":1.5}`)
	}))
	_, err := c.GetDataSetCreationStatus(context.Background(), c.BaseURL().String()+"status")
	if err == nil || !strings.Contains(err.Error(), "bad dataSetId") {
		t.Errorf("want bad dataSetId error, got %v", err)
	}
}

// ---------- WaitForDataSetCreated context cancel ----------

func TestWaitForDataSetCreated_ContextCancelled(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"createMessageHash":"0x1","service":"svc","txStatus":"pending","dataSetCreated":false}`)
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := c.WaitForDataSetCreated(ctx, c.BaseURL().String()+"status", 50*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want DeadlineExceeded, got %v", err)
	}
}

// ---------- DownloadPiece edge cases ----------

func TestDownloadPiece_ServerError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	pc := testPieceInfoV2(t).CIDv2
	_, _, err := c.DownloadPiece(context.Background(), pc)
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want HTTPError, got %T (%v)", err, err)
	}
	if he.StatusCode != 500 {
		t.Errorf("status=%d", he.StatusCode)
	}
}

// ---------- doWithClient logger path ----------

func TestDoWithClient_LoggerAndNon2xx(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	c, err := New(srv.URL, WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	err = c.Ping(context.Background())
	if err == nil {
		t.Error("expected error")
	}
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want HTTPError, got %T", err)
	}
}

// ---------- CreateDataSet edge cases ----------

func TestCreateDataSet_ZeroTxHash(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/pdp/data-sets/created/0x0000000000000000000000000000000000000000000000000000000000000000")
		w.WriteHeader(http.StatusCreated)
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSet(context.Background(), rk, []byte{1})
	if !errors.Is(err, ErrLocationHeader) {
		t.Errorf("want ErrLocationHeader for zero hash, got %v", err)
	}
}

func TestCreateDataSet_NoLocation(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // no Location
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSet(context.Background(), rk, []byte{1})
	if !errors.Is(err, ErrLocationHeader) {
		t.Errorf("want ErrLocationHeader, got %v", err)
	}
}

// ---------- GetDataSet error path ----------

func TestGetDataSet_Error(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "err", http.StatusInternalServerError)
	}))
	_, err := c.GetDataSet(context.Background(), 7)
	if err == nil {
		t.Error("expected error")
	}
}

// ---------- getJSON decode error ----------

func TestGetJSON_DecodeError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `not valid json`)
	}))
	var out json.RawMessage
	err := c.getJSON(context.Background(), "some/path", &out)
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestGetJSON_NilDst(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	err := c.getJSON(context.Background(), "some/path", nil)
	if err != nil {
		t.Fatalf("nil dst should succeed, got %v", err)
	}
}

// ---------- postJSON marshal error ----------

func TestPostJSON_MarshalError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ch := make(chan int) // channels can't be marshaled
	_, _, err := c.postJSON(context.Background(), "some/path", ch)
	if err == nil {
		t.Error("expected marshal error")
	}
}

// ---------- deleteJSON edge cases ----------

func TestDeleteJSON_MarshalError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	ch := make(chan int)
	err := c.deleteJSON(context.Background(), "some/path", ch, nil)
	if err == nil {
		t.Error("expected marshal error")
	}
}

func TestDeleteJSON_NilPayload(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method=%s", r.Method)
		}
		if r.Header.Get("Content-Type") != "" {
			t.Error("nil payload should not set Content-Type")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"key":"value"}`)
	}))
	// nil dst: response is discarded, no decoding
	if err := c.deleteJSON(context.Background(), "some/path", nil, nil); err != nil {
		t.Fatal(err)
	}
	// non-nil dst: response JSON is decoded into the target
	c2, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"key":"value"}`)
	}))
	var out map[string]string
	if err := c2.deleteJSON(context.Background(), "some/path", nil, &out); err != nil {
		t.Fatalf("non-nil dst: %v", err)
	}
	if out["key"] != "value" {
		t.Errorf("decoded key=%q, want value", out["key"])
	}
}

func TestDeleteJSON_DecodeError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `not-json`)
	}))
	var out map[string]string
	err := c.deleteJSON(context.Background(), "some/path", nil, &out)
	if err == nil {
		t.Error("expected decode error")
	}
}

// ---------- Ping transport error ----------

func TestPing_TransportError(t *testing.T) {
	c, err := New("https://example.com", WithHTTPClient(&http.Client{
		Timeout:   100 * time.Millisecond,
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) { return nil, errors.New("conn refused") }),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err == nil {
		t.Error("expected transport error")
	}
}

// ---------- buildJSONRequest error ----------

func TestBuildJSONRequest_MarshalError(t *testing.T) {
	ch := make(chan int)
	_, err := buildJSONRequest(context.Background(), http.MethodPost, "http://example.com", ch)
	if err == nil {
		t.Error("expected marshal error")
	}
}

// ---------- doWithClient read body error ----------

func TestDoWithClient_ReadBodyError(t *testing.T) {
	c, err := New("https://example.com", WithHTTPClient(&http.Client{
		Timeout: 100 * time.Millisecond,
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(&errReader{}),
				Header:     make(http.Header),
			}, nil
		}),
	}))
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Ping(context.Background()); err == nil {
		t.Error("expected read body error")
	}
}

type errReader struct{}

func (e *errReader) Read(_ []byte) (int, error) { return 0, errors.New("read err") }

// ---------- CreateDataSetAndAddPieces location without 0x prefix ----------

func TestCreateDataSetAndAddPieces_LocationWithout0xPrefix(t *testing.T) {
	info, _ := piece.CalculateFromBytes(make([]byte, 256))
	// Location hash without 0x prefix
	hashHex := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/pdp/data-sets/created/"+hashHex)
		w.WriteHeader(http.StatusCreated)
	}))
	rk := common.HexToAddress("0x01")
	res, err := c.CreateDataSetAndAddPieces(context.Background(), rk, []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.TxHash != common.HexToHash("0x"+hashHex) {
		t.Errorf("txHash=%s", res.TxHash.Hex())
	}
}

// ---------- AddPieces server error ----------

func TestAddPieces_ServerError(t *testing.T) {
	info, _ := piece.CalculateFromBytes([]byte("hi"))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	_, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
	if err == nil {
		t.Error("expected server error")
	}
}

// ---------- UploadPiece undefined CID ----------

func TestUploadPiece_UndefinedCID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.UploadPiece(context.Background(), emptyCID())
	if err == nil || !strings.Contains(err.Error(), "undefined") {
		t.Errorf("want undefined pieceCID error, got %v", err)
	}
}

// ---------- FindPiece server error ----------

func TestFindPiece_ServerError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	pc := testPieceInfoV2(t).CIDv2
	_, err := c.FindPiece(context.Background(), pc)
	if err == nil {
		t.Error("expected server error")
	}
}

// ---------- CreateDataSet server error ----------

func TestCreateDataSet_ServerError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSet(context.Background(), rk, []byte{1})
	if err == nil {
		t.Error("expected server error")
	}
}

// ---------- WithHTTPClient nil ----------

func TestWithHTTPClient_Nil(t *testing.T) {
	c, err := New("https://example.com", WithHTTPClient(nil))
	if err != nil {
		t.Fatal(err)
	}
	if c.httpClient == nil {
		t.Error("nil client should keep default")
	}
}

// ---------- CreateDataSetAndAddPieces server error ----------

func TestCreateDataSetAndAddPieces_ServerError(t *testing.T) {
	info, _ := piece.CalculateFromBytes(make([]byte, 256))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	rk := common.HexToAddress("0x01")
	_, err := c.CreateDataSetAndAddPieces(context.Background(), rk, []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{1})
	if err == nil {
		t.Error("expected server error")
	}
}

// ---------- doWithClient expected statuses mismatch ----------

func TestDoWithClient_ExpectedStatusMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c, err := New(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/test", nil)
	_, _, err = c.do(req, http.StatusCreated) // expect 201 but get 200
	var he *HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want HTTPError, got %T (%v)", err, err)
	}
}

// ---------- DownloadPiece logger path ----------

func TestDownloadPiece_WithLogger(t *testing.T) {
	var buf strings.Builder
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("content"))
	}))
	defer srv.Close()
	c, err := New(srv.URL, WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	pc := testPieceInfoV2(t).CIDv2
	body, _, err := c.DownloadPiece(context.Background(), pc)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = body.Close() }()
	if !strings.Contains(buf.String(), "curio request") {
		t.Errorf("expected log, got %q", buf.String())
	}
	data, _ := io.ReadAll(body)
	if string(data) != "content" {
		t.Errorf("body=%q", data)
	}
}

// ---------- NewTestClient with 0x-prefixed hash no leading 0x for add pieces ----------

func TestAddPieces_LocationWithout0xPrefix(t *testing.T) {
	info, _ := piece.CalculateFromBytes([]byte("hi"))
	hashHex := "dead000000000000000000000000000000000000000000000000000000000000"
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/pdp/data-sets/5/pieces/added/"+hashHex)
		w.WriteHeader(http.StatusCreated)
	}))
	res, err := c.AddPieces(context.Background(), 5, []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
	if err != nil {
		t.Fatal(err)
	}
	if res.TxHash != common.HexToHash("0x"+hashHex) {
		t.Errorf("txHash=%s", res.TxHash.Hex())
	}
}

// ---------- New with existing NonceManager (covers New line 99-100) ----------

func TestWaitForPiecesAdded_ZeroPollIntervalUsesDefault(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[10]}`)
	}))
	status, err := c.WaitForPiecesAdded(context.Background(), c.BaseURL().String()+"status", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !status.PiecesAdded {
		t.Error("expected piecesAdded=true")
	}
}

// ---------- UploadPiece server error ----------

func TestUploadPiece_ServerError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	pc := testPieceInfoV2(t).CIDv2
	_, err := c.UploadPiece(context.Background(), pc)
	if err == nil {
		t.Error("expected server error")
	}
}

// ---------- GetAddPiecesStatus non-uint64 DataSetID ----------

func TestGetAddPiecesStatus_NonUint64DataSetID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Very large number that exceeds uint64 but IS a valid big.Int
		_, _ = fmt.Fprint(w, `{"txHash":"0x1","txStatus":"confirmed","dataSetId":99999999999999999999999999999999,"pieceCount":1,"addMessageOk":true,"piecesAdded":true}`)
	}))
	_, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status")
	if err == nil || !strings.Contains(err.Error(), "bad dataSetId") {
		t.Errorf("want bad dataSetId error for non-uint64, got %v", err)
	}
}

// ---------- UploadPieceBytes with non-default client timeout ----------

func TestUploadPieceBytes_NonDefaultClientTimeout(t *testing.T) {
	var gotUA string
	c, err := New("https://example.com", WithHTTPClient(&http.Client{
		Timeout: 5 * time.Minute, // not DefaultHTTPTimeout
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			gotUA = r.Header.Get("User-Agent")
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
	if err := c.UploadPieceBytes(context.Background(), "uuid-1", strings.NewReader("data"), 4); err != nil {
		t.Fatal(err)
	}
	if gotUA != DefaultUserAgent {
		t.Errorf("ua=%q", gotUA)
	}
}
