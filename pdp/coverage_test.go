package pdp

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
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
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
	if !strings.Contains(buf.String(), "pdp request") {
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

func TestHTTPError_RedactedURL(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://user:secret@example.com/piece?token=private&part=1", nil)
	if err != nil {
		t.Fatal(err)
	}
	he := newHTTPError(req, &http.Response{StatusCode: http.StatusUnauthorized, Header: make(http.Header)}, nil)
	if got := he.RedactedURL(); strings.Contains(got, "secret") || strings.Contains(got, "private") {
		t.Fatalf("RedactedURL leaked credentials: %q", got)
	}
	if !strings.Contains(he.RedactedURL(), "part=1") {
		t.Fatalf("RedactedURL removed non-sensitive query value: %q", he.RedactedURL())
	}
}

func TestDataSetJSON_RoundTripsFullWidthIDs(t *testing.T) {
	largeDataSet, err := types.BigIntFromBig(new(big.Int).Lsh(big.NewInt(1), 200))
	if err != nil {
		t.Fatal(err)
	}
	largePiece, err := types.BigIntFromBig(new(big.Int).Add(largeDataSet.Big(), big.NewInt(123)))
	if err != nil {
		t.Fatal(err)
	}
	want := DataSet{
		ID:                 largeDataSet,
		NextChallengeEpoch: 456,
		Pieces: []DataSetPiece{{
			PieceCID:       "baga6ea4seaqtest",
			PieceID:        largePiece,
			SubPieceCID:    "baga6ea4seaqsubpiece",
			SubPieceOffset: 123,
		}},
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got DataSet
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !got.ID.Equal(want.ID) || got.NextChallengeEpoch != want.NextChallengeEpoch || len(got.Pieces) != 1 ||
		got.Pieces[0].PieceCID != want.Pieces[0].PieceCID || !got.Pieces[0].PieceID.Equal(want.Pieces[0].PieceID) ||
		got.Pieces[0].SubPieceCID != want.Pieces[0].SubPieceCID || got.Pieces[0].SubPieceOffset != want.Pieces[0].SubPieceOffset {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	pieceEncoded, err := json.Marshal(want.Pieces[0])
	if err != nil {
		t.Fatalf("Marshal piece: %v", err)
	}
	var pieceGot DataSetPiece
	if err := json.Unmarshal(pieceEncoded, &pieceGot); err != nil {
		t.Fatalf("Unmarshal piece: %v", err)
	}
	if !pieceGot.PieceID.Equal(largePiece) || pieceGot.PieceCID != want.Pieces[0].PieceCID {
		t.Fatalf("piece round trip = %+v, want %+v", pieceGot, want.Pieces[0])
	}
}

func TestTerminateErrors_IncludeStableContext(t *testing.T) {
	txHash := common.HexToHash("0x1234")
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"already terminated", &ServiceAlreadyTerminatedError{ServiceTerminationEpoch: 42, Message: "done"}, "epoch 42: done"},
		{"pending", &TerminateServicePendingError{Message: "queued"}, "pending: queued"},
		{"unsupported", &TerminateServiceNotSupportedError{Body: "upgrade required"}, "upgrade required"},
		{"not found", &WaitForTerminateServiceNotFoundError{}, "status not found"},
		{"rejected", &WaitForTerminateServiceRejectedError{TxHash: txHash}, txHash.Hex()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("Error() = %q, want substring %q", got, tt.want)
			}
		})
	}

	withoutDetails := []error{
		&ServiceAlreadyTerminatedError{ServiceTerminationEpoch: 7},
		&TerminateServicePendingError{},
		&TerminateServiceNotSupportedError{},
	}
	for _, err := range withoutDetails {
		if got := err.Error(); got == "" {
			t.Fatalf("%T returned an empty error", err)
		}
	}
}

func TestTerminateServiceStatusURL_ValidatesDataSetID(t *testing.T) {
	client, err := New("https://provider.example/base/")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.TerminateServiceStatusURL(types.NewBigInt(0)); err == nil {
		t.Fatal("TerminateServiceStatusURL(0) returned nil error")
	}
	got, err := client.TerminateServiceStatusURL(types.NewBigInt(12))
	if err != nil {
		t.Fatalf("TerminateServiceStatusURL: %v", err)
	}
	if got != "https://provider.example/base/pdp/data-sets/12/terminate" {
		t.Fatalf("TerminateServiceStatusURL = %q", got)
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
	if !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
	}
}

func TestWaitForCreateDataSetAndAddPieces_ZeroCreateMessageHash(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"createMessageHash":"0x0000000000000000000000000000000000000000000000000000000000000000","service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":42}`)
	}))
	_, err := c.WaitForCreateDataSetAndAddPieces(context.Background(), c.BaseURL().String()+"status", 5*time.Millisecond)
	if !errors.Is(err, ErrInvalidStatus) {
		t.Errorf("want ErrInvalidStatus, got %v", err)
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
	_, err := c.AddPieces(context.Background(), types.NewBigInt(5), nil, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "no pieces") {
		t.Errorf("want no pieces error, got %v", err)
	}
}

func TestAddPieces_EmptyExtraData(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	info, _ := piece.CalculateFromBytes(make([]byte, 256))
	_, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: info.CIDv1}}, nil)
	if err == nil || !strings.Contains(err.Error(), "empty extraData") {
		t.Errorf("want empty extraData error, got %v", err)
	}
}

func TestAddPieces_UndefinedPieceCID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not be called")
	}))
	_, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: emptyCID()}}, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "undefined pieceCID") {
		t.Errorf("want undefined pieceCID error, got %v", err)
	}
}

func TestAddPieces_BadLocation(t *testing.T) {
	info, _ := piece.CalculateFromBytes([]byte("hi"))
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated) // no Location
	}))
	_, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
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
	_, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
	if !errors.Is(err, ErrLocationHeader) {
		t.Errorf("want ErrLocationHeader for zero hash, got %v", err)
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
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000001","txStatus":"rejected","dataSetId":5,"pieceCount":1,"addMessageOk":false,"piecesAdded":false}`)
	}))
	_, err := c.WaitForPiecesAdded(context.Background(), c.BaseURL().String()+"status", 10*time.Millisecond)
	if !errors.Is(err, ErrTxRejected) {
		t.Fatalf("want ErrTxRejected, got %v", err)
	}
}

func TestWaitForPiecesAdded_ContextCancelled(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000001","txStatus":"pending","dataSetId":5,"pieceCount":0,"piecesAdded":false}`)
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
	_, err := c.SchedulePieceDeletion(context.Background(), types.NewBigInt(5), types.NewBigInt(9), nil)
	if err == nil || !strings.Contains(err.Error(), "empty extraData") {
		t.Errorf("want empty extraData error, got %v", err)
	}
}

func TestSchedulePieceDeletion_ZeroTxHash(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000000"}`)
	}))
	_, err := c.SchedulePieceDeletion(context.Background(), types.NewBigInt(5), types.NewBigInt(9), []byte{1})
	if err == nil || !strings.Contains(err.Error(), "zero txHash") {
		t.Errorf("want zero txHash error, got %v", err)
	}
}

func TestSchedulePieceDeletions_Validation(t *testing.T) {
	tooLarge, err := types.ParseBigInt("9223372036854775808")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		pieceIDs  []types.BigInt
		extraData []byte
		wantIs    error
		wantText  string
	}{
		{name: "empty", extraData: []byte{1}, wantText: "no pieces"},
		{name: "too many", pieceIDs: makeBigInts(MaxDeletePiecesBatchSize + 1), extraData: []byte{1}, wantIs: ErrTooManyPieces},
		{name: "duplicate", pieceIDs: []types.BigInt{types.NewBigInt(1), types.NewBigInt(1)}, extraData: []byte{1}, wantText: "duplicate pieceID"},
		{name: "above Curio range", pieceIDs: []types.BigInt{tooLarge}, extraData: []byte{1}, wantText: "outside Curio's supported range"},
		{name: "empty extraData", pieceIDs: []types.BigInt{types.NewBigInt(1)}, wantText: "empty extraData"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Fatal("should not be called")
			}))
			_, err := c.SchedulePieceDeletions(context.Background(), types.NewBigInt(5), tt.pieceIDs, tt.extraData)
			if err == nil || !strings.Contains(err.Error(), tt.wantText) {
				t.Fatalf("err=%v want text %q", err, tt.wantText)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("err=%v want errors.Is(%v)", err, tt.wantIs)
			}
		})
	}
}

func TestSchedulePieceDeletions_InvalidTxHash(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x1234"}`)
	}))
	_, err := c.SchedulePieceDeletions(context.Background(), types.NewBigInt(5), []types.BigInt{types.NewBigInt(1)}, []byte{1})
	if err == nil || !strings.Contains(err.Error(), "invalid txHash") {
		t.Fatalf("err=%v want invalid txHash", err)
	}
}

func TestSchedulePieceDeletions_HTTP429Classification(t *testing.T) {
	tests := []struct {
		name           string
		body           string
		wantQueueFull  bool
		wantRetryAfter time.Duration
	}{
		{
			name:           "deletion queue full",
			body:           "data set 5 already has 200 scheduled removals queued (limit 200); retry once they have been processed",
			wantQueueFull:  true,
			wantRetryAfter: 30 * time.Second,
		},
		{
			name:           "IP throttled",
			body:           "too many requests",
			wantRetryAfter: 30 * time.Second,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "30")
				http.Error(w, tt.body, http.StatusTooManyRequests)
			}))
			_, err := c.SchedulePieceDeletions(context.Background(), types.NewBigInt(5), []types.BigInt{types.NewBigInt(1)}, []byte{1})
			if got := errors.Is(err, ErrTooManyPiecesQueued); got != tt.wantQueueFull {
				t.Fatalf("errors.Is(ErrTooManyPiecesQueued)=%t want %t: %v", got, tt.wantQueueFull, err)
			}
			httpErr, ok := errors.AsType[*HTTPError](err)
			if !ok {
				t.Fatalf("err=%T %v want *HTTPError", err, err)
			}
			if httpErr.StatusCode != http.StatusTooManyRequests || httpErr.RetryAfter != tt.wantRetryAfter {
				t.Fatalf("HTTPError=%+v", httpErr)
			}
		})
	}
}

func makeBigInts(count int) []types.BigInt {
	ids := make([]types.BigInt, count)
	for i := range ids {
		ids[i] = types.NewBigInt(uint64(i))
	}
	return ids
}

func TestSchedulePieceDeletion_ServerError(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	_, err := c.SchedulePieceDeletion(context.Background(), types.NewBigInt(5), types.NewBigInt(9), []byte{1})
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
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000001","txStatus":"confirmed","dataSetId":1.5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true}`)
	}))
	_, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status")
	if err == nil || !strings.Contains(err.Error(), "bad dataSetId") {
		t.Errorf("want bad dataSetId error, got %v", err)
	}
}

func TestGetAddPiecesStatus_BadConfirmedPieceID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000001","txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[1.5]}`)
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
		_, _ = fmt.Fprint(w, `{"createMessageHash":"0x0000000000000000000000000000000000000000000000000000000000000001","service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":1.5}`)
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
		_, _ = fmt.Fprint(w, `{"createMessageHash":"0x0000000000000000000000000000000000000000000000000000000000000001","service":"svc","txStatus":"pending","dataSetCreated":false}`)
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
	he, ok := errors.AsType[*HTTPError](err)
	if !ok {
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
	if _, ok := errors.AsType[*HTTPError](err); !ok {
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
	_, err := c.GetDataSet(context.Background(), types.NewBigInt(7))
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
	_, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
	if err == nil {
		t.Error("expected server error")
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
	if _, ok := errors.AsType[*HTTPError](err); !ok {
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
	if !strings.Contains(buf.String(), "pdp request") {
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
	res, err := c.AddPieces(context.Background(), types.NewBigInt(5), []AddPieceInput{{PieceCID: info.CIDv1}}, []byte{1})
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
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000001","txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[10]}`)
	}))
	status, err := c.WaitForPiecesAdded(context.Background(), c.BaseURL().String()+"status", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !status.PiecesAdded {
		t.Error("expected piecesAdded=true")
	}
}

// ---------- GetAddPiecesStatus full-width DataSetID ----------

func TestGetAddPiecesStatus_FullWidthDataSetID(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"txHash":"0x0000000000000000000000000000000000000000000000000000000000000001","txStatus":"confirmed","dataSetId":99999999999999999999999999999999,"pieceCount":1,"addMessageOk":true,"piecesAdded":true}`)
	}))
	status, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status")
	if err != nil {
		t.Fatal(err)
	}
	if status.DataSetID.String() != "99999999999999999999999999999999" {
		t.Fatalf("dataSetID=%s", status.DataSetID.String())
	}
}

// ---------- UploadPieceStreaming with non-default client timeout ----------

func TestUploadPieceStreaming_NonDefaultClientTimeout(t *testing.T) {
	var gotUA string
	rt := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		gotUA = r.Header.Get("User-Agent")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/pdp/piece/uploads":
			h := make(http.Header)
			h.Set("Location", "/pdp/piece/uploads/u1")
			return &http.Response{StatusCode: http.StatusCreated, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
		case r.Method == http.MethodPut:
			_, _ = io.Copy(io.Discard, r.Body)
			return &http.Response{StatusCode: http.StatusNoContent, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		case r.Method == http.MethodPost:
			_, _ = io.Copy(io.Discard, r.Body)
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		return nil, fmt.Errorf("unexpected %s %s", r.Method, r.URL.Path)
	})
	c, err := New("https://example.com", WithHTTPClient(&http.Client{Timeout: 5 * time.Minute, Transport: rt}))
	if err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte{0xab}, 512)
	if _, err := c.UploadPieceStreaming(context.Background(), bytes.NewReader(payload), UploadPieceStreamingOptions{Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if gotUA != DefaultUserAgent {
		t.Errorf("ua=%q", gotUA)
	}
}
