package abi

import (
	"context"
	"fmt"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
)

type Call3 struct {
	Target       common.Address
	AllowFailure bool
	CallData     []byte
}

type Result3 struct {
	Success    bool
	ReturnData []byte
}

// DefaultMaxMulticallCalls is the default maximum number of sub-calls sent in
// one aggregate3 invocation by SDK methods whose batch size depends on input.
const DefaultMaxMulticallCalls = 64

var (
	multicall3Address = chain.Mainnet.Addresses().Multicall3
	multicall3ABI     = mustParseABI(`[{"inputs":[{"components":[{"internalType":"address","name":"target","type":"address"},{"internalType":"bool","name":"allowFailure","type":"bool"},{"internalType":"bytes","name":"callData","type":"bytes"}],"internalType":"struct Multicall3.Call3[]","name":"calls","type":"tuple[]"}],"name":"aggregate3","outputs":[{"components":[{"internalType":"bool","name":"success","type":"bool"},{"internalType":"bytes","name":"returnData","type":"bytes"}],"internalType":"struct Multicall3.Result[]","name":"returnData","type":"tuple[]"}],"stateMutability":"payable","type":"function"}]`)
)

func BatchCall(ctx context.Context, caller ContractCaller, calls []Call3) ([]Result3, error) {
	if caller == nil {
		return nil, fmt.Errorf("abi.BatchCall: nil caller")
	}
	if len(calls) == 0 {
		return nil, nil
	}
	data, err := multicall3ABI.Pack("aggregate3", calls)
	if err != nil {
		return nil, fmt.Errorf("abi.BatchCall: pack aggregate3: %w", err)
	}
	raw, err := caller.CallContract(ctx, ethereum.CallMsg{
		To:   &multicall3Address,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("abi.BatchCall: aggregate3: %w", err)
	}
	var out []Result3
	if err := multicall3ABI.UnpackIntoInterface(&out, "aggregate3", raw); err != nil {
		return nil, fmt.Errorf("abi.BatchCall: decode aggregate3: %w", err)
	}
	return out, nil
}

// BatchCallChunked executes calls in serial aggregate3 batches of at most
// maxCalls entries and returns results in input order. It stops at the first
// context or batch error and does not retry a failed batch.
func BatchCallChunked(ctx context.Context, caller ContractCaller, calls []Call3, maxCalls int) ([]Result3, error) {
	if maxCalls <= 0 {
		return nil, fmt.Errorf("abi.BatchCallChunked: maxCalls must be > 0")
	}
	if caller == nil {
		return nil, fmt.Errorf("abi.BatchCallChunked: nil caller")
	}
	if len(calls) == 0 {
		return nil, nil
	}

	results := make([]Result3, 0, len(calls))
	for start := 0; start < len(calls); {
		end := start + min(maxCalls, len(calls)-start)
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("abi.BatchCallChunked: calls [%d:%d): %w", start, end, err)
		}
		batch, err := BatchCall(ctx, caller, calls[start:end])
		if err != nil {
			return nil, fmt.Errorf("abi.BatchCallChunked: calls [%d:%d): %w", start, end, err)
		}
		if len(batch) != end-start {
			return nil, fmt.Errorf(
				"abi.BatchCallChunked: calls [%d:%d): expected %d results, got %d",
				start,
				end,
				end-start,
				len(batch),
			)
		}
		results = append(results, batch...)
		start = end
	}
	return results, nil
}

func mustParseABI(def string) gethabi.ABI {
	a, err := gethabi.JSON(strings.NewReader(def))
	if err != nil {
		panic(err) //nolint:forbidigo // mustParseABI is only used to parse compile-time-constant ABI strings during package init
	}
	return a
}
