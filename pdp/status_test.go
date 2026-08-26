package pdp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

const (
	testOriginalTx  = "0x0000000000000000000000000000000000000000000000000000000000000011"
	testConfirmedTx = "0x0000000000000000000000000000000000000000000000000000000000000022"
)

func TestGetAddPiecesStatusNormalizesWireStates(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError error
	}{
		{
			name: "pending",
			body: fmt.Sprintf(`{"txHash":%q,"txStatus":"pending","dataSetId":5,"pieceCount":0,"piecesAdded":false}`, testOriginalTx),
		},
		{
			name:      "failed",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"failed","dataSetId":5,"pieceCount":1,"addMessageOk":false,"piecesAdded":false}`, testOriginalTx),
			wantError: ErrTxRejected,
		},
		{
			name:      "legacy rejected",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"rejected","dataSetId":5,"pieceCount":1,"addMessageOk":false,"piecesAdded":false}`, testOriginalTx),
			wantError: ErrTxRejected,
		},
		{
			name:      "confirmed false",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":false,"piecesAdded":false}`, testOriginalTx),
			wantError: ErrTxRejected,
		},
		{
			name:      "reorged after success",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"reorged","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[7]}`, testOriginalTx),
			wantError: ErrTxRejected,
		},
		{
			name: "confirmed processing",
			body: fmt.Sprintf(`{"txHash":%q,"txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":false}`, testOriginalTx),
		},
		{
			name: "confirmed without result",
			body: fmt.Sprintf(`{"txHash":%q,"txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":null,"piecesAdded":false}`, testOriginalTx),
		},
		{
			name:      "confirmed processing without result",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":null,"piecesAdded":true,"confirmedPieceIds":[7]}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name: "confirmed",
			body: fmt.Sprintf(`{"txHash":%q,"confirmedTxHash":%q,"txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[7]}`, testOriginalTx, testConfirmedTx),
		},
		{
			name:      "unknown",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"queued","dataSetId":5,"pieceCount":0,"piecesAdded":false}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "contradictory",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"pending","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[7]}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "zero data set id",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"pending","dataSetId":0,"pieceCount":0,"piecesAdded":false}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "negative piece count",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"pending","dataSetId":5,"pieceCount":-1,"piecesAdded":false}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "confirmed ids before pieces added",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"confirmed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":false,"confirmedPieceIds":[7]}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "failed with successful add",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"failed","dataSetId":5,"pieceCount":1,"addMessageOk":true,"piecesAdded":false}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, test.body)
			}))
			status, err := client.GetAddPiecesStatus(context.Background(), client.BaseURL().String()+"status")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error=%v want %v", err, test.wantError)
			}
			if requests != 1 {
				t.Fatalf("requests=%d want 1", requests)
			}
			if status == nil {
				t.Fatal("expected decoded snapshot")
			}
			if test.name == "confirmed" && status.ConfirmedTxHash != common.HexToHash(testConfirmedTx) {
				t.Fatalf("confirmedTxHash=%s", status.ConfirmedTxHash)
			}
		})
	}
}

func TestGetDataSetCreationStatusNormalizesWireStates(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantError error
	}{
		{
			name: "pending",
			body: fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"pending","dataSetCreated":false}`, testOriginalTx),
		},
		{
			name:      "failed",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"failed","dataSetCreated":false,"ok":false}`, testOriginalTx),
			wantError: ErrTxRejected,
		},
		{
			name:      "confirmed false",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"confirmed","dataSetCreated":false,"ok":false}`, testOriginalTx),
			wantError: ErrTxRejected,
		},
		{
			name:      "reorged after success",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"reorged","dataSetCreated":true,"ok":true,"dataSetId":5}`, testOriginalTx),
			wantError: ErrTxRejected,
		},
		{
			name: "confirmed processing",
			body: fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"confirmed","dataSetCreated":false,"ok":true}`, testOriginalTx),
		},
		{
			name: "confirmed",
			body: fmt.Sprintf(`{"createMessageHash":%q,"confirmedTxHash":%q,"txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":5}`, testOriginalTx, testConfirmedTx),
		},
		{
			name: "confirmed without result",
			body: fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"confirmed","dataSetCreated":false}`, testOriginalTx),
		},
		{
			name:      "created without result",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"confirmed","dataSetCreated":true,"dataSetId":5}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "created without data set id",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"confirmed","dataSetCreated":true,"ok":true}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "pending with result",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"pending","dataSetCreated":false,"ok":true}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "failed with successful result",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"failed","dataSetCreated":false,"ok":true}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
		{
			name:      "rejected with created data set",
			body:      fmt.Sprintf(`{"createMessageHash":%q,"txStatus":"confirmed","dataSetCreated":true,"dataSetId":5,"ok":false}`, testOriginalTx),
			wantError: ErrInvalidStatus,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, test.body)
			}))
			status, err := client.GetDataSetCreationStatus(context.Background(), client.BaseURL().String()+"status")
			if !errors.Is(err, test.wantError) {
				t.Fatalf("error=%v want %v", err, test.wantError)
			}
			if status == nil {
				t.Fatal("expected decoded snapshot")
			}
		})
	}
}

func TestGetAddPiecesStatusRetriesTransportStatusButNotPending(t *testing.T) {
	requests := 0
	client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "try again", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"txHash":%q,"txStatus":"pending","dataSetId":5,"pieceCount":0,"piecesAdded":false}`, testOriginalTx)
	}))
	status, err := client.GetAddPiecesStatus(context.Background(), client.BaseURL().String()+"status")
	if err != nil {
		t.Fatal(err)
	}
	if status.TxStatus != "pending" || requests != 2 {
		t.Fatalf("status=%q requests=%d", status.TxStatus, requests)
	}
}

func TestGetAddPiecesStatusRetriesTransportTimeout(t *testing.T) {
	attempts := 0
	rt := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, timeoutError{}
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body: io.NopCloser(strings.NewReader(fmt.Sprintf(
				`{"txHash":%q,"txStatus":"pending","dataSetId":5,"pieceCount":0,"piecesAdded":false}`,
				testOriginalTx,
			))),
		}, nil
	})
	client, err := New(
		"https://provider.example",
		WithHTTPClient(&http.Client{Transport: rt}),
		WithMaxRetries(1),
	)
	if err != nil {
		t.Fatal(err)
	}
	client.retryDelayFn = noRetryDelay
	status, err := client.GetAddPiecesStatus(context.Background(), "https://provider.example/status")
	if err != nil {
		t.Fatal(err)
	}
	if status.TxStatus != "pending" || attempts != 2 {
		t.Fatalf("status=%q attempts=%d", status.TxStatus, attempts)
	}
}

func TestStatusTimeoutPreservesClassificationAndRedactsURL(t *testing.T) {
	rt := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return nil, timeoutError{}
	})
	client, err := New(
		"https://provider.example",
		WithHTTPClient(&http.Client{Transport: rt}),
		WithMaxRetries(0),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetAddPiecesStatus(context.Background(), "https://provider.example/status?token=secret")
	if err == nil {
		t.Fatal("expected timeout")
	}
	urlErr, ok := errors.AsType[*url.Error](err)
	if !ok || !urlErr.Timeout() {
		t.Fatalf("error=%T %v want timeout url.Error", err, err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked sensitive query: %v", err)
	}
}

func TestGetAddPiecesStatusDoesNotRetryCallerCancellation(t *testing.T) {
	attempts := 0
	ctx, cancel := context.WithCancel(context.Background())
	rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		attempts++
		cancel()
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	client, err := New(
		"https://provider.example",
		WithHTTPClient(&http.Client{Transport: rt}),
		WithMaxRetries(2),
	)
	if err != nil {
		t.Fatal(err)
	}
	client.retryDelayFn = noRetryDelay
	_, err = client.GetAddPiecesStatus(ctx, "https://provider.example/status")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context.Canceled", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts=%d want 1", attempts)
	}
}

func TestStatusURLOriginValidation(t *testing.T) {
	client, err := New("https://Example.COM/provider")
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		"https://example.com/status",
		"https://example.com:443/status?tx=1",
	} {
		u, parseErr := url.Parse(raw)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if err := client.validateStatusURL(u); err != nil {
			t.Fatalf("validate %q: %v", raw, err)
		}
	}
	for _, raw := range []string{
		"/relative",
		"ftp://example.com/status",
		"http://example.com/status",
		"https://example.com:444/status",
		"https://other.example/status",
	} {
		u, parseErr := url.Parse(raw)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if err := client.validateStatusURL(u); !errors.Is(err, ErrStatusURLOrigin) {
			t.Fatalf("validate %q: %v", raw, err)
		}
	}
}

func TestGetStatusRejectsCrossOriginRedirectBeforeFollowing(t *testing.T) {
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetRequests++
	}))
	t.Cleanup(target.Close)

	customRedirectCalls := 0
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/status?token=secret", http.StatusFound)
	}))
	t.Cleanup(source.Close)
	client, err := New(source.URL, WithHTTPClient(&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			customRedirectCalls++
			return nil
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetAddPiecesStatus(context.Background(), source.URL+"/start")
	if !errors.Is(err, ErrStatusURLOrigin) {
		t.Fatalf("error=%v want ErrStatusURLOrigin", err)
	}
	if targetRequests != 0 || customRedirectCalls != 0 {
		t.Fatalf("targetRequests=%d customRedirectCalls=%d", targetRequests, customRedirectCalls)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaked sensitive query value: %v", err)
	}
}

func TestGetStatusPreservesCustomRedirectPolicy(t *testing.T) {
	want := errors.New("redirect denied")
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/status", http.StatusFound)
			return
		}
		t.Fatal("custom redirect policy should stop the redirect")
	}))
	t.Cleanup(source.Close)
	client, err := New(source.URL, WithHTTPClient(&http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return want },
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.GetAddPiecesStatus(context.Background(), source.URL+"/start")
	if !errors.Is(err, want) {
		t.Fatalf("error=%v want custom redirect error", err)
	}
}

func TestSubmissionsRejectCrossOriginStatusLocations(t *testing.T) {
	info, err := piece.CalculateFromBytes(make([]byte, 256))
	if err != nil {
		t.Fatal(err)
	}
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("status target should not be queried during submission")
	}))
	t.Cleanup(target.Close)
	location := target.URL + "/status/" + testOriginalTx + "?token=secret"

	for _, test := range []struct {
		name string
		call func(context.Context, *Client) error
	}{
		{
			name: "create data set",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.CreateDataSet(ctx, common.HexToAddress("0x1"), []byte{1})
				return err
			},
		},
		{
			name: "add pieces",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.AddPieces(ctx, types.NewBigInt(5), []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{1})
				return err
			},
		},
		{
			name: "create and add",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.CreateDataSetAndAddPieces(ctx, common.HexToAddress("0x1"), []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{1})
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", location)
				w.WriteHeader(http.StatusCreated)
			}))
			if err := test.call(context.Background(), client); !errors.Is(err, ErrStatusURLOrigin) {
				t.Fatalf("error=%v want ErrStatusURLOrigin", err)
			} else if strings.Contains(err.Error(), "secret") {
				t.Fatalf("error leaked Location query value: %v", err)
			}
		})
	}
}

func TestGetCreateDataSetAndAddPiecesStatusRejectsCrossStageMismatch(t *testing.T) {
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/create":
			_, _ = fmt.Fprintf(w, `{"createMessageHash":%q,"txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":42}`, testOriginalTx)
		case "/pdp/data-sets/42/pieces/added/" + testOriginalTx:
			_, _ = fmt.Fprintf(w, `{"txHash":%q,"txStatus":"confirmed","dataSetId":99,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[7]}`, testOriginalTx)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	status, err := client.GetCreateDataSetAndAddPiecesStatus(context.Background(), server.URL+"/create")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("error=%v want ErrInvalidStatus", err)
	}
	if status == nil || status.Create == nil || status.Add == nil {
		t.Fatalf("expected both decoded snapshots: %+v", status)
	}
}

func TestGetCreateDataSetAndAddPiecesStatusRejectsConfirmedHashConflict(t *testing.T) {
	createConfirmed := "0x00000000000000000000000000000000000000000000000000000000000000a1"
	addConfirmed := "0x00000000000000000000000000000000000000000000000000000000000000a2"
	client, server := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/create":
			_, _ = fmt.Fprintf(w, `{"createMessageHash":%q,"confirmedTxHash":%q,"txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":42}`, testOriginalTx, createConfirmed)
		case "/pdp/data-sets/42/pieces/added/" + testOriginalTx:
			_, _ = fmt.Fprintf(w, `{"txHash":%q,"confirmedTxHash":%q,"txStatus":"confirmed","dataSetId":42,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[7]}`, testOriginalTx, addConfirmed)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	_, err := client.GetCreateDataSetAndAddPiecesStatus(context.Background(), server.URL+"/create")
	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("error=%v want ErrInvalidStatus", err)
	}
}
