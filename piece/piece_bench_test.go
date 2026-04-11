package piece

import (
	"bytes"
	"testing"
)

var benchSizes = []struct {
	name string
	size int
}{
	{"1KB", 1 << 10},
	{"1MB", 1 << 20},
	{"10MB", 10 << 20},
}

func BenchmarkCalculate(b *testing.B) {
	for _, tc := range benchSizes {
		data := make([]byte, tc.size)
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(tc.size))
			b.ResetTimer()
			for range b.N {
				_, err := Calculate(bytes.NewReader(data))
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkCalculateFromBytes(b *testing.B) {
	for _, tc := range benchSizes {
		data := make([]byte, tc.size)
		b.Run(tc.name, func(b *testing.B) {
			b.SetBytes(int64(tc.size))
			b.ResetTimer()
			for range b.N {
				_, err := CalculateFromBytes(data)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkValidate(b *testing.B) {
	data := make([]byte, 1024)
	info, err := CalculateFromBytes(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if err := Validate(info.CIDv1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExtractRoot(b *testing.B) {
	data := make([]byte, 1024)
	info, err := CalculateFromBytes(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := ExtractRoot(info.CIDv1); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseV2(b *testing.B) {
	data := make([]byte, 1024)
	info, err := CalculateFromBytes(data)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for range b.N {
		if _, err := ParseV2(info.CIDv2); err != nil {
			b.Fatal(err)
		}
	}
}
