// List-providers shows active PDP storage providers and their capabilities.
//
// Usage:
//
//	export SYNAPSE_PRIVATE_KEY=0x...
//	go run ./examples/list-providers --piece-size 1048576
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"time"

	"github.com/strahe/synapse-go/examples/internal/exampleutil"
	"github.com/strahe/synapse-go/spregistry"
)

type providerConfig struct {
	PieceSize int64
}

type providerSelector interface {
	SelectActivePDPProviders(context.Context, spregistry.ProviderFilter) ([]spregistry.PDPProvider, error)
}

func main() {
	ctx, cancel := exampleutil.WithTimeout(context.Background(), 30*time.Minute)
	err := realMain(ctx, os.Args[1:], os.Getenv, os.Stdout)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	env, err := exampleutil.LoadEnv(getenv)
	if err != nil {
		return err
	}
	client, err := exampleutil.NewClient(ctx, env)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := exampleutil.WriteKV(stdout, "chain", client.Chain()); err != nil {
		return err
	}
	return runListProviders(ctx, cfg, client.SPRegistry(), stdout)
}

func parseConfig(args []string) (providerConfig, error) {
	cfg := providerConfig{}
	fs := flag.NewFlagSet("list-providers", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Int64Var(&cfg.PieceSize, "piece-size", 0, "optional raw payload size filter")
	if err := fs.Parse(args); err != nil {
		return providerConfig{}, err
	}
	if fs.NArg() != 0 {
		return providerConfig{}, fmt.Errorf("usage: go run ./examples/list-providers [--piece-size bytes]")
	}
	if cfg.PieceSize < 0 {
		return providerConfig{}, fmt.Errorf("piece-size must be non-negative, got %d", cfg.PieceSize)
	}
	return cfg, nil
}

func runListProviders(ctx context.Context, cfg providerConfig, selector providerSelector, stdout io.Writer) error {
	filter := spregistry.ProviderFilter{}
	if cfg.PieceSize > 0 {
		filter.PieceSizeBytes = big.NewInt(cfg.PieceSize)
	}
	providers, err := selector.SelectActivePDPProviders(ctx, filter)
	if err != nil {
		return fmt.Errorf("select active PDP providers: %w", err)
	}
	if err := exampleutil.WriteKV(stdout, "providerCount", len(providers)); err != nil {
		return err
	}
	for i, provider := range providers {
		if err := printProvider(stdout, i+1, provider); err != nil {
			return err
		}
	}
	return nil
}

func printProvider(stdout io.Writer, index int, provider spregistry.PDPProvider) error {
	prefix := fmt.Sprintf("provider.%d", index)
	if err := exampleutil.WriteKV(stdout, prefix+".id", provider.Info.ID); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".name", provider.Info.Name); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".active", provider.Info.IsActive); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".serviceURL", provider.Offering.ServiceURL); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".minPieceSize", provider.Offering.MinPieceSizeInBytes); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".maxPieceSize", provider.Offering.MaxPieceSizeInBytes); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".pricePerTiBPerDay", provider.Offering.StoragePricePerTiBPerDay); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".location", provider.Offering.Location); err != nil {
		return err
	}
	return exampleutil.WriteKV(stdout, prefix+".productActive", provider.Product.IsActive)
}
