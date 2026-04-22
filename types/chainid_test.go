package types

import (
	"math/big"
	"testing"
)

func TestChainID_BigInt(t *testing.T) {
	got := ChainID(314159).BigInt()
	if got.Cmp(big.NewInt(314159)) != 0 {
		t.Errorf("BigInt() = %v, want 314159", got)
	}
	// mutating the returned pointer must not affect subsequent calls
	got.SetInt64(0)
	again := ChainID(314159).BigInt()
	if again.Int64() != 314159 {
		t.Errorf("second BigInt() = %v, want 314159 (caller should own the value)", again)
	}
}

func TestChainID_Int64(t *testing.T) {
	if got := ChainID(0).Int64(); got != 0 {
		t.Errorf("zero Int64() = %d, want 0", got)
	}
	if got := ChainID(314).Int64(); got != 314 {
		t.Errorf("Int64() = %d, want 314", got)
	}
}

func TestChainID_IsValid(t *testing.T) {
	tests := []struct {
		c    ChainID
		want bool
	}{
		{0, false},
		{-1, false},
		{1, true},
		{314, true},
		{314159, true},
	}
	for _, tt := range tests {
		if got := tt.c.IsValid(); got != tt.want {
			t.Errorf("ChainID(%d).IsValid() = %v, want %v", tt.c, got, tt.want)
		}
	}
}
