package abi

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"sync"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

func TestBatchCall_NilCaller(t *testing.T) {
	_, err := BatchCall(context.Background(), nil, []Call3{{Target: common.Address{}}})
	if err == nil {
		t.Fatal("expected error for nil caller")
	}
}

func TestBatchCall_EmptyCalls(t *testing.T) {
	caller := &fakeMulticallCaller{t: t, multicallAddr: multicall3Address}
	results, err := BatchCall(context.Background(), caller, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if results != nil {
		t.Fatalf("expected nil results for empty calls, got %v", results)
	}
}

func TestBatchCall_CallContractError(t *testing.T) {
	caller := &errorCaller{err: fmt.Errorf("network down")}
	calls := []Call3{{Target: common.HexToAddress("0x1111111111111111111111111111111111111111")}}
	_, err := BatchCall(context.Background(), caller, calls)
	if err == nil {
		t.Fatal("expected error from CallContract")
	}
}

func TestBatchCall_UnpackError(t *testing.T) {
	// Return garbage data that won't unpack as aggregate3 output.
	caller := &staticCaller{response: []byte{0x01, 0x02, 0x03}}
	calls := []Call3{{Target: common.HexToAddress("0x1111111111111111111111111111111111111111")}}
	_, err := BatchCall(context.Background(), caller, calls)
	if err == nil {
		t.Fatal("expected unpack error")
	}
}

func TestBatchCallChunked_PreservesOrderAndLimit(t *testing.T) {
	caller := &recordingBatchCaller{}
	calls := make([]Call3, 65)
	for i := range calls {
		calls[i] = Call3{Target: common.Address{byte(i + 1)}, CallData: []byte{byte(i)}}
	}

	results, err := BatchCallChunked(context.Background(), caller, calls, 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != len(calls) {
		t.Fatalf("len(results) = %d, want %d", len(results), len(calls))
	}
	for i := range results {
		if len(results[i].ReturnData) != 1 || results[i].ReturnData[0] != byte(i) {
			t.Fatalf("results[%d].ReturnData = %x, want %02x", i, results[i].ReturnData, byte(i))
		}
	}
	if got, want := caller.batchSizes(), []int{64, 1}; !slices.Equal(got, want) {
		t.Fatalf("batch sizes = %v, want %v", got, want)
	}
	if got := caller.maxConcurrency(); got != 1 {
		t.Fatalf("max concurrency = %d, want 1", got)
	}
}

func TestBatchCallChunked_StopsAfterBatchError(t *testing.T) {
	wantErr := errors.New("RPC unavailable")
	caller := &recordingBatchCaller{failCall: 2, err: wantErr}
	calls := make([]Call3, 130)

	results, err := BatchCallChunked(context.Background(), caller, calls, 64)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if results != nil {
		t.Fatalf("results = %v, want nil", results)
	}
	if got := caller.callCount(); got != 2 {
		t.Fatalf("CallContract count = %d, want 2", got)
	}
	if !strings.Contains(err.Error(), "calls [64:128)") {
		t.Fatalf("error = %v, want failed range", err)
	}
}

func TestBatchCallChunked_CancellationStopsBeforeNextBatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	caller := &recordingBatchCaller{afterCall: cancel}

	_, err := BatchCallChunked(ctx, caller, make([]Call3, 2), 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if got := caller.callCount(); got != 1 {
		t.Fatalf("CallContract count = %d, want 1", got)
	}
}

func TestBatchCallChunked_RejectsNonPositiveLimit(t *testing.T) {
	for _, maxCalls := range []int{-1, 0} {
		t.Run(fmt.Sprint(maxCalls), func(t *testing.T) {
			_, err := BatchCallChunked(context.Background(), &recordingBatchCaller{}, []Call3{{}}, maxCalls)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestMustParseABI_Valid(t *testing.T) {
	// Verify that mustParseABI works for a valid definition (via the package-level var).
	if len(multicall3ABI.Methods) == 0 {
		t.Fatal("expected at least one method in multicall3ABI")
	}
}

// --- helpers ---

type errorCaller struct{ err error }

func (c *errorCaller) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return nil, nil
}

func (c *errorCaller) CallContract(_ context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	return nil, c.err
}

type staticCaller struct{ response []byte }

func (c *staticCaller) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return []byte{0x1}, nil
}

func (c *staticCaller) CallContract(_ context.Context, _ ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	return c.response, nil
}

type recordingBatchCaller struct {
	mu        sync.Mutex
	active    int
	maxActive int
	calls     int
	sizes     []int
	failCall  int
	err       error
	afterCall func()
}

func (c *recordingBatchCaller) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.mu.Lock()
	c.active++
	c.calls++
	callNumber := c.calls
	c.maxActive = max(c.maxActive, c.active)
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.active--
		c.mu.Unlock()
	}()

	if c.failCall == callNumber {
		return nil, c.err
	}
	values, err := multicall3ABI.Methods["aggregate3"].Inputs.Unpack(msg.Data[4:])
	if err != nil {
		return nil, err
	}
	calls := values[0].([]struct {
		Target       common.Address `json:"target"`
		AllowFailure bool           `json:"allowFailure"`
		CallData     []byte         `json:"callData"`
	})
	c.mu.Lock()
	c.sizes = append(c.sizes, len(calls))
	c.mu.Unlock()

	results := make([]Result3, len(calls))
	for i := range calls {
		results[i] = Result3{Success: true, ReturnData: calls[i].CallData}
	}
	response, err := multicall3ABI.Methods["aggregate3"].Outputs.Pack(results)
	if err == nil && c.afterCall != nil {
		c.afterCall()
	}
	return response, err
}

func (c *recordingBatchCaller) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *recordingBatchCaller) batchSizes() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.sizes)
}

func (c *recordingBatchCaller) maxConcurrency() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.maxActive
}
