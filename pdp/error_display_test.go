package pdp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestTerminateErrorsBoundProviderDetails(t *testing.T) {
	detail := "provider\nforged\x1b[31m" + strings.Repeat("x", 8<<10)
	already := &ServiceAlreadyTerminatedError{ServiceTerminationEpoch: 42, Message: detail}
	pending := &TerminateServicePendingError{Message: detail}
	unsupported := &TerminateServiceNotSupportedError{Body: detail}
	for _, tt := range []struct {
		name   string
		err    error
		detail *string
	}{
		{name: "already terminated", err: already, detail: &already.Message},
		{name: "pending", err: pending, detail: &pending.Message},
		{name: "unsupported", err: unsupported, detail: &unsupported.Body},
	} {
		t.Run(tt.name, func(t *testing.T) {
			message := tt.err.Error()
			if len(message) > 4*(4<<10)+256 {
				t.Fatalf("error contains %d bytes", len(message))
			}
			if strings.ContainsAny(message, "\n\r\t\x1b") || !strings.Contains(message, "bytes omitted") {
				t.Fatal("provider detail is not bounded and escaped")
			}
			if *tt.detail != detail {
				t.Fatal("structured provider detail changed")
			}
		})
	}
}

func TestGetAddPiecesStatusBoundsJSONNumberError(t *testing.T) {
	number := strings.Repeat("9", 1<<20)
	for _, tt := range []struct {
		name      string
		body      string
		maxBytes  int
		typeError bool
	}{
		{
			name:      "integer overflow",
			body:      fmt.Sprintf(`{"txHash":%q,"txStatus":"pending","dataSetId":5,"pieceCount":%s}`, testTxOne, number),
			maxBytes:  512,
			typeError: true,
		},
		{
			name:     "invalid quoted number",
			body:     fmt.Sprintf(`{"txHash":%q,"txStatus":"pending","dataSetId":%q}`, testTxOne, number+"x"),
			maxBytes: 4*(4<<10) + 256,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, tt.body)
			}))
			_, err := c.GetAddPiecesStatus(context.Background(), c.BaseURL().String()+"status/"+testTxOne)
			if err == nil {
				t.Fatal("expected invalid number error")
			}
			if len(err.Error()) > tt.maxBytes {
				t.Fatalf("error contains %d bytes", len(err.Error()))
			}
			if tt.typeError {
				typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err)
				if !ok || typeErr.Value != "number "+number || typeErr.Field != "pieceCount" {
					t.Fatal("original JSON type error was not preserved")
				}
				if !errors.Is(err, typeErr) {
					t.Fatal("original JSON type error is not in the error chain")
				}
			} else {
				preserved := false
				for original := errors.Unwrap(err); original != nil; original = errors.Unwrap(original) {
					if strings.Contains(original.Error(), number+"x") {
						preserved = true
						break
					}
				}
				if !preserved {
					t.Fatal("original JSON number diagnostic was not preserved")
				}
			}
		})
	}
}

func TestTransactionStatusBoundsUnknownValue(t *testing.T) {
	unknown := "bad\n\x1b[31m" + strings.Repeat("x", 1<<20)
	for _, tt := range []struct {
		name   string
		create bool
	}{
		{name: "add pieces"},
		{name: "create data set", create: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response := map[string]any{"txStatus": unknown}
			if tt.create {
				response["createMessageHash"] = testTxOne
			} else {
				response["txHash"] = testTxOne
				response["dataSetId"] = 5
			}
			body, err := json.Marshal(response)
			if err != nil {
				t.Fatal(err)
			}
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(body)
			}))
			statusURL := c.BaseURL().String() + "status/" + testTxOne
			var original string
			if tt.create {
				var snapshot *CreateDataSetStatus
				snapshot, err = c.GetDataSetCreationStatus(context.Background(), statusURL)
				if snapshot != nil {
					original = snapshot.TxStatus
				}
			} else {
				var snapshot *AddPiecesStatus
				snapshot, err = c.GetAddPiecesStatus(context.Background(), statusURL)
				if snapshot != nil {
					original = snapshot.TxStatus
				}
			}
			if !errors.Is(err, ErrInvalidStatus) {
				t.Fatalf("error = %v, want ErrInvalidStatus", err)
			}
			if original != unknown {
				t.Fatal("response status was not preserved")
			}
			message := err.Error()
			if len(message) > 4*(4<<10)+256 || !strings.Contains(message, "bytes omitted") {
				t.Fatalf("status error is not bounded: %d bytes", len(message))
			}
			if strings.ContainsAny(message, "\n\r\x1b") || !strings.Contains(message, `bad\n\x1b[31m`) {
				t.Fatal("status error did not escape controls exactly once")
			}
		})
	}
}

func TestPullPiecesBoundsUnknownStatus(t *testing.T) {
	unknown := PullStatus("bad\n\x1b[31m" + strings.Repeat("x", 1<<20))
	pc := testPieceInfoV2(t).CIDv2
	req := PullRequest{
		RecordKeeper: common.HexToAddress("0xabc"),
		ExtraData:    []byte{1},
		Pieces:       []PullPieceInput{{PieceCID: pc, SourceURL: "https://source.example/piece"}},
	}
	for _, tt := range []struct {
		name     string
		response PullResult
	}{
		{name: "overall", response: PullResult{Status: unknown}},
		{name: "piece", response: PullResult{Status: PullStatusPending, Pieces: []PullPieceStatus{{PieceCID: pc.String(), Status: unknown}}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.response)
			if err != nil {
				t.Fatal(err)
			}
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(body)
			}))
			_, err = c.PullPieces(context.Background(), req)
			if !errors.Is(err, ErrInvalidStatus) {
				t.Fatalf("error = %v, want ErrInvalidStatus", err)
			}
			message := err.Error()
			if len(message) > 4*(4<<10)+256 || !strings.Contains(message, "bytes omitted") {
				t.Fatalf("pull status error is not bounded: %d bytes", len(message))
			}
			if strings.ContainsAny(message, "\n\r\x1b") || !strings.Contains(message, `bad\n\x1b[31m`) {
				t.Fatal("pull status error did not escape controls exactly once")
			}
		})
	}
}
