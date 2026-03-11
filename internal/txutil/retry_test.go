package txutil

import (
	"context"
	"errors"
	"testing"
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
	if !IsGasError(errors.New("gas required exceeds allowance")) {
		t.Fatal("expected true")
	}
	if !IsGasError(errors.New("insufficient funds for gas * price + value")) {
		t.Fatal("expected true")
	}
	if !IsGasError(errors.New("transaction underpriced")) {
		t.Fatal("expected true")
	}
	if IsGasError(errors.New("nonce too low")) {
		t.Fatal("expected false")
	}
}
