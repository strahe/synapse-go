package txutil_test

import (
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/internal/txutil"
)

func TestValidateUint256(t *testing.T) {
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	tests := []struct {
		name    string
		value   *big.Int
		wantErr bool
	}{
		{name: "nil", wantErr: true},
		{name: "negative", value: big.NewInt(-1), wantErr: true},
		{name: "zero", value: new(big.Int)},
		{name: "one", value: big.NewInt(1)},
		{name: "maximum", value: new(big.Int).Sub(limit, big.NewInt(1))},
		{name: "overflow", value: new(big.Int).Set(limit), wantErr: true},
		{name: "overflow with low bits", value: new(big.Int).Add(limit, big.NewInt(7)), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var before *big.Int
			if tt.value != nil {
				before = new(big.Int).Set(tt.value)
			}
			if err := txutil.ValidateUint256(tt.value); (err != nil) != tt.wantErr {
				t.Fatalf("ValidateUint256(%v) error = %v, want error=%t", tt.value, err, tt.wantErr)
			}
			if before != nil && tt.value.Cmp(before) != 0 {
				t.Fatalf("input changed from %s to %s", before, tt.value)
			}
		})
	}
}
