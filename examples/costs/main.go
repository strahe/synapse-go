// Costs example demonstrates querying storage costs and account status.
//
// Usage:
//
//	export PRIVATE_KEY=0x...
//	export RPC_URL=https://api.calibration.node.glif.io
//	export CHAIN=calibration # optional, auto-detected from RPC
//	go run ./examples/costs/ [data-size-bytes]
//
// data-size-bytes defaults to 1 GiB (1073741824).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/costs"
)

const defaultDataSize = 1 << 30 // 1 GiB

type costsConfig struct {
	RPCURL        string
	PrivateKeyHex string
	Chain         *chain.Chain
	DataSizeBytes *big.Int
}

type costQuerier interface {
	GetUploadCosts(ctx context.Context, payer common.Address, dataSizeBytes *big.Int, opts *costs.UploadCostOptions) (*costs.UploadCosts, error)
}

type accountQuerier interface {
	GetAccountSummary(ctx context.Context, owner common.Address) (*costs.AccountSummary, error)
}

func main() {
	if err := realMain(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	cfg, err := parseConfig(args, getenv)
	if err != nil {
		return err
	}

	opts := []synapse.ClientOption{
		synapse.WithPrivateKeyHex(cfg.PrivateKeyHex),
		synapse.WithRPCURL(cfg.RPCURL),
	}
	if cfg.Chain != nil {
		opts = append(opts, synapse.WithChain(*cfg.Chain))
	}

	client, err := synapse.New(ctx, opts...)
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}
	defer func() { _ = client.Close() }()

	costsService := client.Costs()
	return runCosts(ctx, cfg, client.Address(), costsService, costsService, stdout)
}

func parseConfig(args []string, getenv func(string) string) (costsConfig, error) {
	if len(args) > 1 {
		return costsConfig{}, errors.New("usage: go run ./examples/costs/ [data-size-bytes]")
	}
	rpcURL := strings.TrimSpace(getenv("RPC_URL"))
	if rpcURL == "" {
		return costsConfig{}, errors.New("RPC_URL is required")
	}
	privateKeyHex := strings.TrimSpace(getenv("PRIVATE_KEY"))
	if privateKeyHex == "" {
		return costsConfig{}, errors.New("PRIVATE_KEY is required")
	}

	var selectedChain *chain.Chain
	if rawChain := strings.TrimSpace(getenv("CHAIN")); rawChain != "" {
		switch strings.ToLower(rawChain) {
		case "calibration":
			c := chain.Calibration
			selectedChain = &c
		case "mainnet":
			c := chain.Mainnet
			selectedChain = &c
		default:
			return costsConfig{}, fmt.Errorf("unsupported CHAIN %q", rawChain)
		}
	}

	dataSize := big.NewInt(defaultDataSize)
	if len(args) == 1 {
		dataSize = new(big.Int)
		if _, ok := dataSize.SetString(args[0], 10); !ok || dataSize.Sign() <= 0 {
			return costsConfig{}, fmt.Errorf("invalid data size %q", args[0])
		}
	}

	return costsConfig{
		RPCURL:        rpcURL,
		PrivateKeyHex: privateKeyHex,
		Chain:         selectedChain,
		DataSizeBytes: dataSize,
	}, nil
}

func runCosts(ctx context.Context, cfg costsConfig, addr common.Address, cq costQuerier, aq accountQuerier, stdout io.Writer) error {
	uc, err := cq.GetUploadCosts(ctx, addr, cfg.DataSizeBytes, nil)
	if err != nil {
		return fmt.Errorf("costs.GetUploadCosts: %w", err)
	}

	as, err := aq.GetAccountSummary(ctx, addr)
	if err != nil {
		return fmt.Errorf("costs.GetAccountSummary: %w", err)
	}

	var writeErr error
	w := func(format string, a ...any) {
		if writeErr == nil {
			_, writeErr = fmt.Fprintf(stdout, format, a...)
		}
	}

	w("=== Upload Cost Estimate ===\n")
	w("dataSize=%s bytes\n", cfg.DataSizeBytes)
	w("ratePerEpoch=%s\n", uc.Rate.RatePerEpoch)
	w("ratePerMonth=%s\n", uc.Rate.RatePerMonth)
	w("\n--- Lockup ---\n")
	w("rateLockup=%s\n", uc.Lockup.RateLockup)
	w("cdnLockup=%s\n", uc.Lockup.CDNFixedLockup)
	w("sybilFee=%s\n", uc.Lockup.SybilFee)
	w("totalLockup=%s\n", uc.Lockup.TotalLockup)
	w("\ndepositNeeded=%s\n", uc.DepositNeeded)
	w("ready=%t\n", uc.Ready)
	w("\n=== Account Summary ===\n")
	w("funds=%s\n", as.Funds)
	w("available=%s\n", as.AvailableFunds)
	w("debt=%s\n", as.Debt)
	w("fundedUntilEpoch=%s\n", as.FundedUntilEpoch)
	return writeErr
}
