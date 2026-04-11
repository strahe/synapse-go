package abi

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// benchCaller returns a staticCaller pre-loaded with a valid aggregate3 response for n calls.
func benchCaller(b *testing.B, n int) *staticCaller {
	b.Helper()
	results := make([]Result3, n)
	for i := range results {
		results[i] = Result3{Success: true, ReturnData: []byte{}}
	}
	response, err := multicall3ABI.Methods["aggregate3"].Outputs.Pack(results)
	if err != nil {
		b.Fatalf("pack aggregate3 response: %v", err)
	}
	return &staticCaller{response: response}
}

func benchCalls(n int) []Call3 {
	calls := make([]Call3, n)
	for i := range calls {
		calls[i] = Call3{
			Target:       common.HexToAddress("0x1111111111111111111111111111111111111111"),
			AllowFailure: false,
			CallData:     []byte{0xab, 0xcd},
		}
	}
	return calls
}

func BenchmarkBatchCall_1(b *testing.B)   { benchmarkBatchCall(b, 1) }
func BenchmarkBatchCall_10(b *testing.B)  { benchmarkBatchCall(b, 10) }
func BenchmarkBatchCall_100(b *testing.B) { benchmarkBatchCall(b, 100) }

func benchmarkBatchCall(b *testing.B, n int) {
	b.Helper()
	caller := benchCaller(b, n)
	calls := benchCalls(n)
	ctx := context.Background()
	b.ResetTimer()
	for range b.N {
		results, err := BatchCall(ctx, caller, calls)
		if err != nil {
			b.Fatal(err)
		}
		if len(results) != n {
			b.Fatalf("expected %d results, got %d", n, len(results))
		}
	}
}
