// Upload example demonstrates uploading a file using synapse.Client.
//
// Usage:
//
//	export PRIVATE_KEY=0x...
//	export RPC_URL=https://api.calibration.node.glif.io
//	export CHAIN=calibration # optional, auto-detected from RPC
//	export COPIES=2          # optional, default: 2
//	go run ./examples/upload/ ./path/to/file
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/storage"
)

type uploadConfig struct {
	FilePath      string
	RPCURL        string
	PrivateKeyHex string
	Chain         *chain.Chain // nil = auto-detect
	Copies        int
}

type uploader interface {
	Upload(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
}

func main() {
	if err := realMain(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	cfg, err := parseUploadConfig(args, getenv)
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

	return runUpload(ctx, cfg, client.Storage(), stdout)
}

func parseUploadConfig(args []string, getenv func(string) string) (uploadConfig, error) {
	if len(args) != 1 {
		return uploadConfig{}, errors.New("usage: go run ./examples/upload/ <file-path>")
	}
	rpcURL := strings.TrimSpace(getenv("RPC_URL"))
	if rpcURL == "" {
		return uploadConfig{}, errors.New("RPC_URL is required")
	}
	privateKeyHex := strings.TrimSpace(getenv("PRIVATE_KEY"))
	if privateKeyHex == "" {
		return uploadConfig{}, errors.New("PRIVATE_KEY is required")
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
			return uploadConfig{}, fmt.Errorf("unsupported CHAIN %q", rawChain)
		}
	}

	copies := 2
	if rawCopies := strings.TrimSpace(getenv("COPIES")); rawCopies != "" {
		parsed, err := strconv.Atoi(rawCopies)
		if err != nil || parsed <= 0 {
			return uploadConfig{}, fmt.Errorf("invalid COPIES %q", rawCopies)
		}
		copies = parsed
	}
	return uploadConfig{
		FilePath:      args[0],
		RPCURL:        rpcURL,
		PrivateKeyHex: privateKeyHex,
		Chain:         selectedChain,
		Copies:        copies,
	}, nil
}

func runUpload(ctx context.Context, cfg uploadConfig, mgr uploader, stdout io.Writer) error {
	file, err := os.Open(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", cfg.FilePath, err)
	}
	defer func() { _ = file.Close() }()

	result, err := mgr.Upload(ctx, file, &storage.UploadOptions{Copies: cfg.Copies})
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "pieceCID=%s\n", result.PieceCID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "size=%d\n", result.Size); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "requestedCopies=%d\n", result.RequestedCopies); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "complete=%t\n", result.Complete); err != nil {
		return err
	}
	for _, copy := range result.Copies {
		if _, err := fmt.Fprintf(stdout, "providerID=%s retrievalURL=%s\n", copy.ProviderID, copy.RetrievalURL); err != nil {
			return err
		}
	}
	return nil
}
