package pdp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

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
				var target *TerminateServicePendingError
				if !errors.As(err, &target) {
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
				var target *TerminateServiceNotSupportedError
				if !errors.As(err, &target) {
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
		var calls int
		var callbackHash common.Hash
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			if calls == 1 {
				_, _ = w.Write([]byte(`{"terminationTxHash":"` + txHash.Hex() + `","fwssTerminated":null,"serviceTerminationEpoch":null}`))
				return
			}
			_, _ = w.Write([]byte(`{"terminationTxHash":"` + txHash.Hex() + `","fwssTerminated":true,"serviceTerminationEpoch":456}`))
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
	})

	t.Run("not found before hash", func(t *testing.T) {
		c, _ := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		}))
		_, err := c.WaitForTerminateService(context.Background(), types.NewBigInt(7), time.Nanosecond, nil)
		var target *WaitForTerminateServiceNotFoundError
		if !errors.As(err, &target) {
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
