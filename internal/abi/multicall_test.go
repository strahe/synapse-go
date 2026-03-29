package abi

import (
	"context"
	"fmt"
	"math/big"
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
