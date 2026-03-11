package txutil

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
)

// GasEstimator abstracts the gas-estimation methods of an Ethereum client.
type GasEstimator interface {
	EstimateGas(ctx context.Context, msg ethereum.CallMsg) (uint64, error)
	SuggestGasPrice(ctx context.Context) (*big.Int, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
}

// EstimateGasWithBuffer estimates gas and adds a percentage safety buffer.
// Filecoin's FEVM commonly requires a buffer because estimation is not a
// tight upper bound.
//
// bufferPercent must be in [0, 1000]; 0 disables the buffer.
func EstimateGasWithBuffer(ctx context.Context, client GasEstimator, msg ethereum.CallMsg, bufferPercent int) (uint64, error) {
	if bufferPercent < 0 || bufferPercent > 1000 {
		return 0, fmt.Errorf("txutil.EstimateGasWithBuffer: bufferPercent %d out of range [0,1000]", bufferPercent)
	}
	limit, err := client.EstimateGas(ctx, msg)
	if err != nil {
		return 0, fmt.Errorf("txutil.EstimateGasWithBuffer: %w", err)
	}
	if bufferPercent == 0 {
		return limit, nil
	}
	buf := new(big.Int).Mul(new(big.Int).SetUint64(limit), big.NewInt(int64(bufferPercent)))
	buf.Quo(buf, big.NewInt(100))
	total := new(big.Int).Add(new(big.Int).SetUint64(limit), buf)
	if !total.IsUint64() {
		return 0, fmt.Errorf("txutil.EstimateGasWithBuffer: buffered limit overflows uint64")
	}
	return total.Uint64(), nil
}

// SuggestGasPriceWithMultiplier returns the suggested legacy gas price
// multiplied by the given float. multiplier<=1 returns the unmodified
// price. Multiplication rounds down to the nearest wei.
func SuggestGasPriceWithMultiplier(ctx context.Context, client GasEstimator, multiplier float64) (*big.Int, error) {
	price, err := client.SuggestGasPrice(ctx)
	if err != nil {
		return nil, fmt.Errorf("txutil.SuggestGasPriceWithMultiplier: %w", err)
	}
	return scaleBigInt(price, multiplier), nil
}

// SuggestGasTipCapWithMultiplier returns the suggested EIP-1559 tip cap
// multiplied by the given float.
func SuggestGasTipCapWithMultiplier(ctx context.Context, client GasEstimator, multiplier float64) (*big.Int, error) {
	tip, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		return nil, fmt.Errorf("txutil.SuggestGasTipCapWithMultiplier: %w", err)
	}
	return scaleBigInt(tip, multiplier), nil
}

func scaleBigInt(v *big.Int, multiplier float64) *big.Int {
	if v == nil || multiplier <= 1.0 {
		if v == nil {
			return nil
		}
		return new(big.Int).Set(v)
	}
	f := new(big.Float).SetPrec(256).SetInt(v)
	f.Mul(f, big.NewFloat(multiplier))
	out, _ := f.Int(nil)
	return out
}
