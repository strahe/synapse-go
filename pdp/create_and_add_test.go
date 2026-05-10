package pdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

type createAndAddBody struct {
	RecordKeeper string              `json:"recordKeeper"`
	ExtraData    string              `json:"extraData"`
	Pieces       []createAndAddPiece `json:"pieces"`
}

type createAndAddPiece struct {
	PieceCID  string                 `json:"pieceCid"`
	SubPieces []createAndAddSubPiece `json:"subPieces"`
}

type createAndAddSubPiece struct {
	SubPieceCID string `json:"subPieceCid"`
}

func TestCreateDataSetAndAddPieces_OK(t *testing.T) {
	info, err := piece.CalculateFromBytes(make([]byte, 256))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	recordKeeper := common.HexToAddress("0xabc")
	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pdp/data-sets/create-and-add" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body createAndAddBody
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.RecordKeeper != recordKeeper.Hex() {
			t.Errorf("recordKeeper=%q want %q", body.RecordKeeper, recordKeeper.Hex())
		}
		if body.ExtraData == "" {
			t.Error("extraData must not be empty")
		}
		if len(body.Pieces) != 1 {
			t.Fatalf("pieces len=%d want 1", len(body.Pieces))
		}
		if body.Pieces[0].PieceCID != info.CIDv2.String() {
			t.Errorf("pieceCid=%q want %q", body.Pieces[0].PieceCID, info.CIDv2.String())
		}
		if len(body.Pieces[0].SubPieces) != 1 || body.Pieces[0].SubPieces[0].SubPieceCID != info.CIDv2.String() {
			t.Errorf("subPieces=%+v want single %q", body.Pieces[0].SubPieces, info.CIDv2.String())
		}
		w.Header().Set("Location", "/pdp/data-sets/created/"+txHash)
		w.WriteHeader(http.StatusCreated)
	}))

	res, err := c.CreateDataSetAndAddPieces(context.Background(), recordKeeper, []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{0xde, 0xad})
	if err != nil {
		t.Fatalf("CreateDataSetAndAddPieces: %v", err)
	}
	if res.TxHash != common.HexToHash(txHash) {
		t.Fatalf("txHash=%s want %s", res.TxHash, txHash)
	}
	if want := c.BaseURL().ResolveReference(&url.URL{Path: "/pdp/data-sets/created/" + txHash}).String(); res.StatusURL != want {
		t.Fatalf("statusURL=%q want %q", res.StatusURL, want)
	}
}

func TestCreateDataSetAndAddPieces_RespectsHTTPClientTimeout(t *testing.T) {
	info, err := piece.CalculateFromBytes(make([]byte, 256))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	recordKeeper := common.HexToAddress("0xabc")
	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pdp/data-sets/create-and-add" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Location", "/pdp/data-sets/created/"+txHash)
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	c, err := New(srv.URL, WithHTTPClient(&http.Client{Timeout: 10 * time.Millisecond}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = c.CreateDataSetAndAddPieces(context.Background(), recordKeeper, []AddPieceInput{{PieceCID: info.CIDv2}}, []byte{0xde, 0xad})
	if err == nil {
		t.Fatal("expected CreateDataSetAndAddPieces to respect the configured HTTP timeout")
	}
	type timeoutError interface {
		error
		Timeout() bool
	}
	if timeoutErr, ok := errors.AsType[timeoutError](err); ok && timeoutErr.Timeout() {
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return
	}
	t.Fatalf("expected timeout error, got %v", err)
}

func TestWaitForCreateDataSetAndAddPieces_OK(t *testing.T) {
	txHash := "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	dataSetID := types.NewBigInt(42)
	pieceID := types.NewBigInt(7)

	c, srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pdp/data-sets/created/" + txHash:
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"createMessageHash":%q,"service":"svc","txStatus":"confirmed","dataSetCreated":true,"ok":true,"dataSetId":%s}`, txHash, dataSetID.String())
		case fmt.Sprintf("/pdp/data-sets/%s/pieces/added/%s", dataSetID.String(), txHash):
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"txHash":%q,"txStatus":"confirmed","dataSetId":%s,"pieceCount":1,"addMessageOk":true,"piecesAdded":true,"confirmedPieceIds":[%s]}`, txHash, dataSetID.String(), pieceID.String())
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))

	status, err := c.WaitForCreateDataSetAndAddPieces(context.Background(), srv.URL+"/pdp/data-sets/created/"+txHash, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitForCreateDataSetAndAddPieces: %v", err)
	}
	if status.TxHash != common.HexToHash(txHash) {
		t.Fatalf("txHash=%s want %s", status.TxHash, txHash)
	}
	if !status.DataSetID.Equal(dataSetID) {
		t.Fatalf("dataSetID=%s want %s", status.DataSetID.String(), dataSetID.String())
	}
	if len(status.ConfirmedPieceIDs) != 1 || !status.ConfirmedPieceIDs[0].Equal(pieceID) {
		t.Fatalf("confirmedPieceIDs=%v want [%s]", status.ConfirmedPieceIDs, pieceID.String())
	}
}
