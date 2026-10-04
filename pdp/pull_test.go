package pdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/types"
)

// pullTestServer builds a test server that handles POST /pdp/piece/pull.
// It calls handleFn with the decoded request body and must return the response.
func pullTestServer(t *testing.T, handleFn func(body pullPiecesBody, callCount int) (int, pullResponse)) (*Client, *httptest.Server) {
	t.Helper()
	callCount := 0
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/pdp/piece/pull" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		callCount++
		var body pullPiecesBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		code, resp := handleFn(body, callCount)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// pullPiecesBody mirrors the wire shape sent by PullPieces.
type pullPiecesBody struct {
	ExtraData    string              `json:"extraData"`
	RecordKeeper string              `json:"recordKeeper,omitempty"`
	DataSetID    *json.Number        `json:"dataSetId,omitempty"`
	Pieces       []pullPieceBodyItem `json:"pieces"`
}

type pullPieceBodyItem struct {
	PieceCid  string `json:"pieceCid"`
	SourceURL string `json:"sourceUrl"`
}

// pullResponse mirrors PullResult wire shape.
type pullResponse struct {
	Status string              `json:"status"`
	Pieces []pullPieceRespItem `json:"pieces"`
}

type pullPieceRespItem struct {
	PieceCid string `json:"pieceCid"`
	Status   string `json:"status"`
}

func TestPullPieces_CreateNew(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	rk := common.HexToAddress("0xabc")
	extraData := []byte{0xde, 0xad}
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	c, _ := pullTestServer(t, func(body pullPiecesBody, _ int) (int, pullResponse) {
		// Validate request shape.
		if body.ExtraData == "" {
			t.Error("extraData must not be empty")
		}
		if body.RecordKeeper == "" {
			t.Error("recordKeeper required for new dataset")
		}
		if body.DataSetID != nil {
			t.Errorf("dataSetId should be absent for new dataset, got %s", body.DataSetID)
		}
		if len(body.Pieces) != 1 {
			t.Errorf("pieces len=%d want 1", len(body.Pieces))
		}
		if len(body.Pieces) > 0 {
			if body.Pieces[0].PieceCid != pc.String() {
				t.Errorf("pieceCid=%q want %q", body.Pieces[0].PieceCid, pc.String())
			}
			if body.Pieces[0].SourceURL != sourceURL {
				t.Errorf("sourceUrl=%q want %q", body.Pieces[0].SourceURL, sourceURL)
			}
		}

		return http.StatusOK, pullResponse{
			Status: "pending",
			Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "pending"}},
		}
	})

	res, err := c.PullPieces(context.Background(), PullRequest{
		RecordKeeper: rk,
		ExtraData:    extraData,
		Pieces: []PullPieceInput{
			{PieceCID: pc, SourceURL: sourceURL},
		},
	})
	if err != nil {
		t.Fatalf("PullPieces: %v", err)
	}
	if res.Status != PullStatusPending {
		t.Errorf("status=%q want pending", res.Status)
	}
	if len(res.Pieces) != 1 {
		t.Fatalf("pieces len=%d want 1", len(res.Pieces))
	}
	if res.Pieces[0].Status != PullStatusPending {
		t.Errorf("piece status=%q want pending", res.Pieces[0].Status)
	}
}

func TestPullPieces_ExistingDataSet(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	rk := common.HexToAddress("0xabc")
	extraData := []byte{0xca, 0xfe}
	dataSetID := types.NewBigInt(42)
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	c, _ := pullTestServer(t, func(body pullPiecesBody, _ int) (int, pullResponse) {
		if body.DataSetID == nil || body.DataSetID.String() != dataSetID.String() {
			t.Errorf("dataSetId: got %v want %s", body.DataSetID, dataSetID.String())
		}
		if body.RecordKeeper != rk.Hex() {
			t.Errorf("recordKeeper=%q want %q", body.RecordKeeper, rk.Hex())
		}
		return http.StatusOK, pullResponse{
			Status: "inProgress",
			Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "inProgress"}},
		}
	})

	res, err := c.PullPieces(context.Background(), PullRequest{
		RecordKeeper: rk,
		ExtraData:    extraData,
		DataSetID:    &dataSetID,
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: sourceURL}},
	})
	if err != nil {
		t.Fatalf("PullPieces: %v", err)
	}
	if res.Status != PullStatusInProgress {
		t.Errorf("status=%q want inProgress", res.Status)
	}
}

func TestPullPieces_ZeroDataSetIDRejected(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server when dataSetID is zero")
	}))
	zero := types.NewBigInt(0)

	_, err := c.PullPieces(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0xca, 0xfe},
		DataSetID:    &zero,
		Pieces: []PullPieceInput{{
			PieceCID:  pc,
			SourceURL: fmt.Sprintf("https://sp.example.com/piece/%s", pc.String()),
		}},
	})
	if err == nil {
		t.Fatal("expected error when dataSetID is zero")
	}
}

func TestPullPieces_ExistingDataSetRequiresRecordKeeper(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("should not reach server when recordKeeper is missing")
	}))

	_, err := c.PullPieces(context.Background(), PullRequest{
		ExtraData: []byte{0xca, 0xfe},
		DataSetID: func() *types.BigInt {
			id := types.NewBigInt(42)
			return &id
		}(),
		Pieces: []PullPieceInput{{
			PieceCID:  pc,
			SourceURL: fmt.Sprintf("https://sp.example.com/piece/%s", pc.String()),
		}},
	})
	if err == nil {
		t.Fatal("expected error when recordKeeper is missing")
	}
}

func TestPullPieces_EmptyInputs(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server")
	}))

	if _, err := c.PullPieces(context.Background(), PullRequest{
		ExtraData: []byte{0x01},
		Pieces:    []PullPieceInput{},
	}); err == nil {
		t.Error("expected error for empty pieces")
	}

	if _, err := c.PullPieces(context.Background(), PullRequest{
		ExtraData: nil,
		Pieces:    []PullPieceInput{{PieceCID: emptyCID(), SourceURL: "https://x.com/piece/x"}},
	}); err == nil {
		t.Error("expected error for empty extraData")
	}
}

func TestPullPieces_ServerError(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	c, _ := pullTestServer(t, func(_ pullPiecesBody, _ int) (int, pullResponse) {
		return http.StatusInternalServerError, pullResponse{}
	})

	_, err := c.PullPieces(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())}},
	})
	if err == nil {
		t.Fatal("expected error for server 500")
	}
	he, ok := errors.AsType[*HTTPError](err)
	if !ok {
		t.Fatalf("want HTTPError, got %T", err)
	}
	if he.StatusCode != http.StatusInternalServerError {
		t.Errorf("status=%d want 500", he.StatusCode)
	}
}

func TestPullPieces_RetriesTransientHTTP(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	var calls int
	c, _ := pullTestServer(t, func(_ pullPiecesBody, callCount int) (int, pullResponse) {
		calls = callCount
		if callCount == 1 {
			return http.StatusServiceUnavailable, pullResponse{}
		}
		return http.StatusOK, pullResponse{
			Status: "pending",
			Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "pending"}},
		}
	})

	res, err := c.PullPieces(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces: []PullPieceInput{{
			PieceCID:  pc,
			SourceURL: fmt.Sprintf("https://sp.example.com/piece/%s", pc.String()),
		}},
	})
	if err != nil {
		t.Fatalf("PullPieces: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
	if res.Status != PullStatusPending {
		t.Fatalf("status=%q want pending", res.Status)
	}
}

func TestPullPieces_QueueFullNotRetried(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	calls := 0
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "120")
		http.Error(w, "pull queue backpressure", http.StatusTooManyRequests)
	}))

	_, err := c.PullPieces(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces: []PullPieceInput{{
			PieceCID:  pc,
			SourceURL: fmt.Sprintf("https://sp.example.com/piece/%s", pc.String()),
		}},
	})
	if calls != 1 {
		t.Fatalf("calls=%d want 1", calls)
	}
	if !errors.Is(err, ErrPullQueueFull) {
		t.Fatalf("err=%v want ErrPullQueueFull", err)
	}
	httpErr, ok := errors.AsType[*HTTPError](err)
	if !ok || httpErr.RetryAfter != 2*time.Minute {
		t.Fatalf("HTTPError=%+v ok=%t, want RetryAfter=2m", httpErr, ok)
	}
}

func TestWaitForPullComplete_WaitsWhenQueueFull(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	req := PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces: []PullPieceInput{{
			PieceCID:  pc,
			SourceURL: fmt.Sprintf("https://sp.example.com/piece/%s", pc.String()),
		}},
	}
	// newServer reports a full queue for the first queueFullCalls requests,
	// then a complete pull.
	newServer := func(t *testing.T, queueFullCalls int) (*Client, *int) {
		calls := 0
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls <= queueFullCalls {
				w.Header().Set("Retry-After", "120")
				http.Error(w, "pull queue backpressure", http.StatusTooManyRequests)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(pullResponse{
				Status: "complete",
				Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "complete"}},
			})
		}))
		return c, &calls
	}

	t.Run("resends after Retry-After", func(t *testing.T) {
		c, calls := newServer(t, 1)
		var waits []time.Duration
		c.pullQueueFullDelayFn = func(err error) time.Duration {
			waits = append(waits, pullQueueFullDelay(err))
			return 0
		}
		res, err := c.WaitForPullComplete(context.Background(), req, time.Millisecond, nil)
		if err != nil {
			t.Fatalf("WaitForPullComplete: %v", err)
		}
		if res.Status != PullStatusComplete || *calls != 2 {
			t.Fatalf("status=%q calls=%d, want complete after 2 calls", res.Status, *calls)
		}
		// The provider's delay is used in full, not the 30s generic retry cap.
		if len(waits) != 1 || waits[0] != 2*time.Minute {
			t.Fatalf("waits=%v want [2m]", waits)
		}
	})

	t.Run("stops when context ends", func(t *testing.T) {
		c, calls := newServer(t, math.MaxInt)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.pullQueueFullDelayFn = func(error) time.Duration {
			cancel()
			return time.Hour
		}
		_, err := c.WaitForPullComplete(ctx, req, time.Millisecond, nil)
		if !errors.Is(err, context.Canceled) || *calls != 1 {
			t.Fatalf("err=%v calls=%d, want context.Canceled after 1 call", err, *calls)
		}
	})
}

func TestPullQueueFullDelay(t *testing.T) {
	tests := []struct {
		name       string
		retryAfter time.Duration
		want       time.Duration
	}{
		{name: "no Retry-After", want: time.Minute},
		{name: "oversized Retry-After", retryAfter: time.Hour, want: 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%w: %w", ErrPullQueueFull, &HTTPError{StatusCode: http.StatusTooManyRequests, RetryAfter: tt.retryAfter})
			if got := pullQueueFullDelay(err); got != tt.want {
				t.Fatalf("pullQueueFullDelay=%v want %v", got, tt.want)
			}
		})
	}
}

func TestWaitForPullComplete_CompletesImmediately(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	c, _ := pullTestServer(t, func(_ pullPiecesBody, _ int) (int, pullResponse) {
		return http.StatusOK, pullResponse{
			Status: "complete",
			Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "complete"}},
		}
	})

	res, err := c.WaitForPullComplete(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: sourceURL}},
	}, 10*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("WaitForPullComplete: %v", err)
	}
	if res.Status != PullStatusComplete {
		t.Errorf("status=%q want complete", res.Status)
	}
}

func TestWaitForPullComplete_EventuallyComplete(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	// First two calls return pending; third returns complete.
	c, _ := pullTestServer(t, func(_ pullPiecesBody, callCount int) (int, pullResponse) {
		if callCount < 3 {
			return http.StatusOK, pullResponse{
				Status: "pending",
				Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "pending"}},
			}
		}
		return http.StatusOK, pullResponse{
			Status: "complete",
			Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "complete"}},
		}
	})

	statusCount := 0
	res, err := c.WaitForPullComplete(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: sourceURL}},
	}, 5*time.Millisecond, func(r *PullResult) { statusCount++ })
	if err != nil {
		t.Fatalf("WaitForPullComplete: %v", err)
	}
	if res.Status != PullStatusComplete {
		t.Errorf("status=%q want complete", res.Status)
	}
	if statusCount < 2 {
		t.Errorf("onStatus called %d times, want >= 2", statusCount)
	}
}

func TestWaitForPullComplete_Failed(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	c, _ := pullTestServer(t, func(_ pullPiecesBody, _ int) (int, pullResponse) {
		return http.StatusOK, pullResponse{
			Status: "failed",
			Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "failed"}},
		}
	})

	res, err := c.WaitForPullComplete(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: sourceURL}},
	}, 5*time.Millisecond, nil)
	if !errors.Is(err, ErrPullFailed) {
		t.Fatalf("want ErrPullFailed, got %v", err)
	}
	if res == nil {
		t.Fatal("result should not be nil on ErrPullFailed")
	}
	if res.Status != PullStatusFailed {
		t.Errorf("status=%q want failed", res.Status)
	}
}

func TestWaitForPullComplete_ContextCancelled(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	c, _ := pullTestServer(t, func(_ pullPiecesBody, _ int) (int, pullResponse) {
		return http.StatusOK, pullResponse{
			Status: "pending",
			Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "pending"}},
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately after first poll.
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()

	_, err := c.WaitForPullComplete(ctx, PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: sourceURL}},
	}, 5*time.Millisecond, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}
}

// Issue 1: PullPieces must reject DataSetID==0 + zero RecordKeeper locally.
func TestPullPieces_ZeroDataSetIDRequiresRecordKeeper(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("should not reach server when local validation fails")
	}))

	_, err := c.PullPieces(context.Background(), PullRequest{
		// DataSetID is 0 (default) and RecordKeeper is zero address — invalid.
		ExtraData: []byte{0x01},
		Pieces:    []PullPieceInput{{PieceCID: pc, SourceURL: sourceURL}},
	})
	if err == nil {
		t.Fatal("expected error for zero DataSetID without RecordKeeper")
	}
	// Must be a local validation error, not an HTTP round-trip.
	if _, ok := errors.AsType[*HTTPError](err); ok {
		t.Fatalf("expected local validation error, got HTTPError: %v", err)
	}
}

// Issue 3: PullPieces must accept HTTP 201 and 202 in addition to 200.
func TestPullPieces_Accepts201And202(t *testing.T) {
	pc := testPieceInfoV2(t).CIDv2
	sourceURL := fmt.Sprintf("https://sp.example.com/piece/%s", pc.String())

	for _, code := range []int{http.StatusCreated, http.StatusAccepted} {
		t.Run(fmt.Sprintf("HTTP%d", code), func(t *testing.T) {
			c, _ := pullTestServer(t, func(_ pullPiecesBody, _ int) (int, pullResponse) {
				return code, pullResponse{
					Status: "pending",
					Pieces: []pullPieceRespItem{{PieceCid: pc.String(), Status: "pending"}},
				}
			})

			res, err := c.PullPieces(context.Background(), PullRequest{
				RecordKeeper: common.HexToAddress("0xabc"),
				ExtraData:    []byte{0x01},
				Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: sourceURL}},
			})
			if err != nil {
				t.Fatalf("PullPieces with %d: %v", code, err)
			}
			if res.Status != PullStatusPending {
				t.Errorf("status=%q want pending", res.Status)
			}
		})
	}
}

func TestPullPieces_RejectsV1(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	pc := testPieceInfoV2(t).CIDv1
	_, err := c.PullPieces(context.Background(), PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: "https://sp.example.com/piece/ignored"}},
	})
	if err == nil {
		t.Fatal("expected PieceCIDv2 validation error")
	}
}

func TestPullPieces_TooManyPieces(t *testing.T) {
	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected req %s %s", r.Method, r.URL.Path)
	}))
	c.legacyPieceStorageIDLimit = 10
	id := types.NewBigInt(5)
	reqPieces := make([]PullPieceInput, MaxLegacyAddPiecesBatchSize+1)
	_, err := c.PullPieces(context.Background(), PullRequest{
		DataSetID:    &id,
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{0x01},
		Pieces:       reqPieces,
	})
	if !errors.Is(err, ErrTooManyPieces) {
		t.Fatalf("err=%v want ErrTooManyPieces", err)
	}
}
