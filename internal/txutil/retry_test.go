package txutil

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
)

func TestIsRetryableRPCError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{context.Canceled, false},
		{context.DeadlineExceeded, false},
		{errors.New("dial tcp: i/o timeout"), true},
		{errors.New("connection refused"), true},
		{errors.New("EOF"), true},
		{errors.New("invalid argument"), false},
		{rpc.HTTPError{StatusCode: 408, Status: "408 Request Timeout"}, true},
		{&rpc.HTTPError{StatusCode: 408}, true},
		{fmt.Errorf("eth_call: %w", rpc.HTTPError{StatusCode: 408}), true},
		{fmt.Errorf("eth_call: %w", &rpc.HTTPError{StatusCode: 408}), true},
		{rpc.HTTPError{StatusCode: 429}, true},
		{&rpc.HTTPError{StatusCode: 502}, true},
		{fmt.Errorf("eth_call: %w", rpc.HTTPError{StatusCode: 503}), true},
		{fmt.Errorf("eth_call: %w", &rpc.HTTPError{StatusCode: 504}), true},
		{rpc.HTTPError{StatusCode: 400, Body: []byte("timeout")}, false},
		{&rpc.HTTPError{StatusCode: 401, Body: []byte("try again")}, false},
		{rpc.HTTPError{StatusCode: 403, Body: []byte("temporary failure")}, false},
		{rpc.HTTPError{StatusCode: 501}, false},
		{errors.Join(context.Canceled, rpc.HTTPError{StatusCode: 429}), false},
		{errors.Join(context.DeadlineExceeded, &rpc.HTTPError{StatusCode: 503}), false},
		{errors.Join(context.Canceled, rpc.HTTPError{StatusCode: 408}), false},
		{errors.Join(context.DeadlineExceeded, &rpc.HTTPError{StatusCode: 408}), false},
	}
	for _, tc := range cases {
		if got := IsRetryableRPCError(tc.err); got != tc.want {
			t.Fatalf("%v: got %v want %v", tc.err, got, tc.want)
		}
	}
}

func TestIsNonceError(t *testing.T) {
	if !IsNonceError(errors.New("nonce too low")) {
		t.Fatal("expected true")
	}
	if !IsNonceError(errors.New("known transaction: already known")) {
		t.Fatal("expected true")
	}
	if IsNonceError(errors.New("revert")) {
		t.Fatal("expected false")
	}
	if IsNonceError(nil) {
		t.Fatal("nil should be false")
	}
}

func TestIsGasError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("gas required exceeds allowance"), true},
		{errors.New("insufficient funds for gas * price + value"), true},
		{errors.New("transaction underpriced"), true},
		{errors.New("max fee cap below base fee"), true},
		{errors.New("nonce too low"), false},
		{errors.New("revert"), false},
	}
	for _, tc := range cases {
		if got := IsGasError(tc.err); got != tc.want {
			t.Errorf("IsGasError(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
