package payments

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestErrInvalidArgument_Detection verifies every New() validation path
// returns ErrInvalidArgument detectable via errors.Is.
func TestErrInvalidArgument_Detection(t *testing.T) {
	// Minimal fake backend for the "invalid ChainID" / "zero FilPayAddress"
	// branches that only need opts.Backend != nil to pass the first guard.
	nonNilBackend := newMockBackend(t)

	tests := []struct {
		name string
		opts Options
	}{
		{
			name: "nil Backend",
			opts: Options{ChainID: big.NewInt(314), FilPayAddress: common.HexToAddress("0x01")},
		},
		{
			name: "nil ChainID",
			opts: Options{Backend: nonNilBackend, FilPayAddress: common.HexToAddress("0x01")},
		},
		{
			name: "zero ChainID",
			opts: Options{Backend: nonNilBackend, ChainID: big.NewInt(0), FilPayAddress: common.HexToAddress("0x01")},
		},
		{
			name: "zero FilPayAddress",
			opts: Options{Backend: nonNilBackend, ChainID: big.NewInt(314)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("errors.Is(err, ErrInvalidArgument)=false; err=%v", err)
			}
		})
	}
}

// TestErrInvalidArgument_NegativeMatch verifies ErrInvalidArgument does NOT
// match unrelated sentinels.
func TestErrInvalidArgument_NegativeMatch(t *testing.T) {
	for _, e := range []error{ErrTxFailed, ErrInsufficientBalance, ErrInsufficientAllowance, ErrZeroAddress} {
		if errors.Is(e, ErrInvalidArgument) {
			t.Fatalf("%v must not match ErrInvalidArgument", e)
		}
	}
	if errors.Is(errors.New("unrelated"), ErrInvalidArgument) {
		t.Fatal("unrelated error must not match ErrInvalidArgument")
	}
}
