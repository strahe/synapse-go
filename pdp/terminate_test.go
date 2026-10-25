package pdp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

func TestGetTerminateServiceStatusEpochBounds(t *testing.T) {
	for _, tt := range []struct {
		name    string
		raw     string
		want    types.Epoch
		wantErr bool
	}{
		{name: "zero", raw: "0"},
		{name: "negative zero", raw: "-0"},
		{name: "quoted negative zero", raw: `"-0"`},
		{name: "maximum", raw: "18446744073709551615", want: types.Epoch(math.MaxUint64)},
		{name: "overflow", raw: "18446744073709551616", wantErr: true},
		{name: "negative", raw: "-1", wantErr: true},
		{name: "fraction", raw: "1.5", wantErr: true},
		{name: "exponent", raw: "1e0", wantErr: true},
		{name: "missing", raw: "null", wantErr: true},
		{name: "oversized", raw: strings.Repeat("9", 1<<20), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprintf(w, `{"fwssTerminated":true,"serviceTerminationEpoch":%s}`, tt.raw)
			}))
			got, err := c.GetTerminateServiceStatus(context.Background(), types.NewBigInt(7))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected invalid epoch error")
				}
				if len(err.Error()) > 256 {
					t.Fatalf("error contains %d bytes for an invalid epoch", len(err.Error()))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ServiceTerminationEpoch != tt.want {
				t.Fatalf("epoch = %d, want %d", got.ServiceTerminationEpoch, tt.want)
			}
		})
	}
}

func TestTerminateService_PostsExtraDataAndRetries429(t *testing.T) {
	var calls int
	c, srv := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodPost || r.URL.Path != "/pdp/data-sets/7/terminate" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if calls == 1 {
			http.Error(w, "slow down", http.StatusTooManyRequests)
			return
		}
		var body terminateServiceWire
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body.ExtraData != "0x0102" {
			t.Fatalf("extraData=%q want 0x0102", body.ExtraData)
		}
		w.WriteHeader(http.StatusAccepted)
	}))

	res, err := c.TerminateService(context.Background(), TerminateServiceRequest{
		DataSetID: types.NewBigInt(7),
		ExtraData: []byte{0x01, 0x02},
	})
	if err != nil {
		t.Fatalf("TerminateService: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d want 2", calls)
	}
	wantURL := srv.URL + "/pdp/data-sets/7/terminate"
	if res.StatusURL != wantURL {
		t.Fatalf("StatusURL=%q want %q", res.StatusURL, wantURL)
	}
}

func TestTerminateService_MapsProviderErrors(t *testing.T) {
	for _, tt := range []struct {
		name   string
		status int
		body   string
		assert func(*testing.T, error)
	}{
		{
			name:   "already terminated",
			status: http.StatusConflict,
			body:   `{"code":0,"message":"done","serviceTerminationEpoch":123}`,
			assert: func(t *testing.T, err error) {
				t.Helper()
				var target *ServiceAlreadyTerminatedError
				if !errors.As(err, &target) {
					t.Fatalf("err=%T %v want ServiceAlreadyTerminatedError", err, err)
				}
				if target.ServiceTerminationEpoch != 123 {
					t.Fatalf("epoch=%d want 123", target.ServiceTerminationEpoch)
				}
			},
		},
		{
			name:   "pending",
			status: http.StatusConflict,
			body:   `{"code":1,"message":"queued","serviceTerminationEpoch":null}`,
			assert: func(t *testing.T, err error) {
				t.Helper()
				if _, ok := errors.AsType[*TerminateServicePendingError](err); !ok {
					t.Fatalf("err=%T %v want TerminateServicePendingError", err, err)
				}
			},
		},
		{
			name:   "unsupported",
			status: http.StatusServiceUnavailable,
			body:   `old provider`,
			assert: func(t *testing.T, err error) {
				t.Helper()
				if _, ok := errors.AsType[*TerminateServiceNotSupportedError](err); !ok {
					t.Fatalf("err=%T %v want TerminateServiceNotSupportedError", err, err)
				}
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			_, err := c.TerminateService(context.Background(), TerminateServiceRequest{
				DataSetID: types.NewBigInt(7),
				ExtraData: []byte{0x01},
			})
			if err == nil {
				t.Fatal("expected error")
			}
			tt.assert(t, err)
		})
	}
}

func TestWaitForTerminateService_SuccessAndRejected404(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		txHash := common.HexToHash("0x1234")
		confirmedTxHash := common.HexToHash("0x5678")
		var calls int
		var callbackHash common.Hash
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				_, _ = w.Write([]byte(`{"terminationTxHash":"` + txHash.Hex() + `","fwssTerminated":null,"serviceTerminationEpoch":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"terminationTxHash":"` + txHash.Hex() + `","confirmedTxHash":"` + confirmedTxHash.Hex() + `","fwssTerminated":true,"serviceTerminationEpoch":456}`))
		}))

		status, err := c.WaitForTerminateService(context.Background(), types.NewBigInt(7), time.Nanosecond, func(h common.Hash) {
			callbackHash = h
		})
		if err != nil {
			t.Fatalf("WaitForTerminateService: %v", err)
		}
		if calls != 2 {
			t.Fatalf("calls=%d want 2", calls)
		}
		if callbackHash != txHash {
			t.Fatalf("callback hash=%s want %s", callbackHash, txHash)
		}
		if !status.FWSSTerminated || status.ServiceTerminationEpoch != 456 {
			t.Fatalf("status=%+v want terminated epoch 456", status)
		}
		if status.ConfirmedTxHash == nil || *status.ConfirmedTxHash != confirmedTxHash {
			t.Fatalf("confirmed hash=%v want %s", status.ConfirmedTxHash, confirmedTxHash)
		}
	})

	t.Run("not found before hash", func(t *testing.T) {
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		}))
		_, err := c.WaitForTerminateService(context.Background(), types.NewBigInt(7), time.Nanosecond, nil)
		if _, ok := errors.AsType[*WaitForTerminateServiceNotFoundError](err); !ok {
			t.Fatalf("err=%T %v want WaitForTerminateServiceNotFoundError", err, err)
		}
	})

	t.Run("not found after hash", func(t *testing.T) {
		txHash := common.HexToHash("0x1234")
		var calls int
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				_, _ = w.Write([]byte(`{"terminationTxHash":"` + txHash.Hex() + `","fwssTerminated":null,"serviceTerminationEpoch":null}`))
				return
			}
			http.NotFound(w, nil)
		}))
		_, err := c.WaitForTerminateService(context.Background(), types.NewBigInt(7), time.Nanosecond, nil)
		var target *WaitForTerminateServiceRejectedError
		if !errors.As(err, &target) {
			t.Fatalf("err=%T %v want WaitForTerminateServiceRejectedError", err, err)
		}
		if target.TxHash != txHash {
			t.Fatalf("TxHash=%s want %s", target.TxHash, txHash)
		}
	})
}

func TestGetTerminateServiceStatus_RejectsInvalidHashes(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "invalid original", body: `{"terminationTxHash":"0x1234","fwssTerminated":null,"serviceTerminationEpoch":null}`},
		{name: "zero original", body: `{"terminationTxHash":"0x0000000000000000000000000000000000000000000000000000000000000000","fwssTerminated":null,"serviceTerminationEpoch":null}`},
		{name: "invalid confirmed", body: `{"terminationTxHash":"","confirmedTxHash":"0x1234","fwssTerminated":null,"serviceTerminationEpoch":null}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			_, err := c.GetTerminateServiceStatus(context.Background(), types.NewBigInt(7))
			if !errors.Is(err, ErrInvalidStatus) {
				t.Fatalf("err=%v want ErrInvalidStatus", err)
			}
		})
	}
}

func TestWaitForTerminateService_RecoverySnapshots(t *testing.T) {
	a := common.HexToHash("0x1")
	b := common.HexToHash("0x2")
	c := common.HexToHash("0x3")
	d := common.HexToHash("0x4")
	zero := common.Hash{}
	type hashPair struct {
		original  common.Hash
		confirmed common.Hash
	}
	tests := []struct {
		name         string
		seed         common.Hash
		states       []hashPair
		complete     bool
		want         hashPair
		wantRejected common.Hash
		wantReported []common.Hash
	}{
		{name: "seed is not an observation", seed: a, wantRejected: a},
		{name: "zero seed has no known transaction"},
		{name: "seed is not reported again", seed: a, states: []hashPair{{a, zero}}, want: hashPair{a, zero}, wantRejected: a},
		{name: "new hash replaces seed", seed: a, states: []hashPair{{c, zero}}, want: hashPair{c, zero}, wantRejected: c, wantReported: []common.Hash{c}},
		{name: "tracking continues after notification", seed: a, states: []hashPair{{a, zero}, {c, zero}, {d, zero}}, want: hashPair{d, zero}, wantRejected: d, wantReported: []common.Hash{c}},
		{name: "status without hashes keeps the known hash", seed: a, states: []hashPair{{zero, zero}}, wantRejected: a},
		{name: "latest status is returned", states: []hashPair{{a, zero}, {a, b}}, want: hashPair{a, b}, wantRejected: a, wantReported: []common.Hash{a}},
		{name: "success returns the final status", states: []hashPair{{a, zero}, {a, b}}, complete: true, want: hashPair{a, b}, wantReported: []common.Hash{a}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("method=%s, want read-only polling", req.Method)
				}
				index := requests
				requests++
				if index >= len(tt.states) {
					return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody}, nil
				}
				pair := tt.states[index]
				return terminateStatusResponse(t, pair.original, pair.confirmed, tt.complete && index == len(tt.states)-1), nil
			})
			client, err := New("https://provider.example", WithHTTPClient(&http.Client{Transport: rt}), WithMaxRetries(0))
			if err != nil {
				t.Fatal(err)
			}
			var reported []common.Hash
			status, err := client.WaitForTerminateService(context.Background(), types.NewBigInt(7), time.Nanosecond,
				func(hash common.Hash) { reported = append(reported, hash) }, nil, WithKnownTerminationTxHash(tt.seed))
			switch {
			case tt.complete:
				if err != nil || status == nil || !status.FWSSTerminated || status.ServiceTerminationEpoch != 456 {
					t.Fatalf("status=%+v err=%v, want confirmed termination", status, err)
				}
			case tt.wantRejected != zero:
				rejected, ok := errors.AsType[*WaitForTerminateServiceRejectedError](err)
				if !ok || rejected.TxHash != tt.wantRejected {
					t.Fatalf("err=%v, want rejection of %s", err, tt.wantRejected)
				}
			default:
				if _, ok := errors.AsType[*WaitForTerminateServiceNotFoundError](err); !ok {
					t.Fatalf("err=%v, want status not found without a known transaction", err)
				}
			}
			if len(tt.states) == 0 {
				if status != nil {
					t.Fatalf("unobserved seed produced status=%+v", status)
				}
			} else {
				if status == nil {
					t.Fatal("wait lost its observed status")
				}
				var original, confirmed common.Hash
				if status.TerminationTxHash != nil {
					original = *status.TerminationTxHash
				}
				if status.ConfirmedTxHash != nil {
					confirmed = *status.ConfirmedTxHash
				}
				if original != tt.want.original || confirmed != tt.want.confirmed {
					t.Fatalf("hashes=%s/%s, want %s/%s", original, confirmed, tt.want.original, tt.want.confirmed)
				}
			}
			if len(reported) != len(tt.wantReported) {
				t.Fatalf("reported=%v, want %v", reported, tt.wantReported)
			}
			for i, hash := range reported {
				if hash != tt.wantReported[i] {
					t.Fatalf("reported=%v, want %v", reported, tt.wantReported)
				}
			}
			wantRequests := len(tt.states)
			if !tt.complete {
				wantRequests++
			}
			if requests != wantRequests {
				t.Fatalf("requests=%d, want %d", requests, wantRequests)
			}
		})
	}
}

func TestWaitForTerminateService_RetainsSnapshotOnFailure(t *testing.T) {
	original := common.HexToHash("0x1")
	confirmed := common.HexToHash("0x2")
	for _, tt := range []struct {
		name    string
		want    error
		cancel  bool
		invalid bool
	}{
		{name: "request deadline", want: context.DeadlineExceeded},
		{name: "cancellation after observation", want: context.Canceled, cancel: true},
		{name: "invalid later response", want: ErrInvalidStatus, invalid: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			requests := 0
			rt := roundTripperFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != http.MethodGet {
					t.Fatalf("method=%s, want GET", req.Method)
				}
				requests++
				if requests == 1 {
					return terminateStatusResponse(t, original, confirmed, false), nil
				}
				if tt.invalid {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     make(http.Header),
						Body:       io.NopCloser(bytes.NewBufferString(`{"terminationTxHash":"0x1234"}`)),
					}, nil
				}
				return nil, context.DeadlineExceeded
			})
			client, err := New("https://provider.example", WithHTTPClient(&http.Client{Transport: rt}), WithMaxRetries(0))
			if err != nil {
				t.Fatal(err)
			}
			status, err := client.WaitForTerminateService(ctx, types.NewBigInt(7), time.Nanosecond, func(common.Hash) {
				if tt.cancel {
					cancel()
				}
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("err=%v, want %v", err, tt.want)
			}
			if status == nil || status.TerminationTxHash == nil || status.ConfirmedTxHash == nil ||
				*status.TerminationTxHash != original || *status.ConfirmedTxHash != confirmed {
				t.Fatalf("status=%+v, want retained original and confirmed hashes", status)
			}
		})
	}
}

func terminateStatusResponse(t *testing.T, original, confirmed common.Hash, complete bool) *http.Response {
	t.Helper()
	wire := terminateStatusWire{FWSSTerminated: &complete}
	if original != (common.Hash{}) {
		wire.TerminationTxHash = original.Hex()
	}
	if confirmed != (common.Hash{}) {
		wire.ConfirmedTxHash = confirmed.Hex()
	}
	if complete {
		epoch := json.Number("456")
		wire.ServiceTerminationEpoch = &epoch
	}
	body, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body))}
}
