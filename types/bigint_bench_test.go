package types

import (
	"strings"
	"testing"
)

func BenchmarkParseBigInt(b *testing.B) {
	for _, bb := range []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "uint256", raw: "115792089237316195423570985008687907853269984665640564039457584007913129639935"},
		{name: "overlong decimal", raw: "1" + strings.Repeat("0", 2_000_000), wantErr: true},
		{name: "leading zeros", raw: strings.Repeat("0", 2_000_000) + "5"},
	} {
		b.Run(bb.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(bb.raw)))
			for b.Loop() {
				_, err := ParseBigInt(bb.raw)
				if (err != nil) != bb.wantErr {
					b.Fatalf("ParseBigInt error = %v, wantErr = %v", err, bb.wantErr)
				}
			}
		})
	}
}
