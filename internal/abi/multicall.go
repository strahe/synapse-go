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

func mustParseABI(def string) gethabi.ABI {
	a, err := gethabi.JSON(strings.NewReader(def))
	if err != nil {
		panic(err) //nolint:forbidigo // mustParseABI is only used to parse compile-time-constant ABI strings during package init
	}
	return a
}
