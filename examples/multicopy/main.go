// Multicopy example demonstrates uploading a file to multiple storage
// providers with CDN enabled.
//
// Usage:
//
//	export PRIVATE_KEY=0x...
//	export RPC_URL=https://api.calibration.node.glif.io
//	export CHAIN=calibration # optional, auto-detected from RPC
//	go run ./examples/multicopy/ ./path/to/file
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/storage"
)

type multicopyConfig struct {
	FilePath      string
	RPCURL        string
	PrivateKeyHex string
	Chain         *chain.Chain
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

	return runMulticopy(ctx, cfg, client.Storage(), stdout)
}

func parseConfig(args []string, getenv func(string) string) (multicopyConfig, error) {
	if len(args) != 1 {
		return multicopyConfig{}, errors.New("usage: go run ./examples/multicopy/ <file-path>")
	}
	rpcURL := strings.TrimSpace(getenv("RPC_URL"))
	if rpcURL == "" {
		return multicopyConfig{}, errors.New("RPC_URL is required")
	}
	privateKeyHex := strings.TrimSpace(getenv("PRIVATE_KEY"))
	if privateKeyHex == "" {
		return multicopyConfig{}, errors.New("PRIVATE_KEY is required")
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
			return multicopyConfig{}, fmt.Errorf("unsupported CHAIN %q", rawChain)
		}
	}

	return multicopyConfig{
		FilePath:      args[0],
		RPCURL:        rpcURL,
		PrivateKeyHex: privateKeyHex,
		Chain:         selectedChain,
	}, nil
}

func runMulticopy(ctx context.Context, cfg multicopyConfig, mgr uploader, stdout io.Writer) error {
	file, err := os.Open(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", cfg.FilePath, err)
	}
	defer func() { _ = file.Close() }()

	withCDN := true
	result, err := mgr.Upload(ctx, file, &storage.UploadOptions{
		Copies:  3,
		WithCDN: &withCDN,
	})
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	var writeErr error
	w := func(format string, a ...any) {
		if writeErr == nil {
			_, writeErr = fmt.Fprintf(stdout, format, a...)
		}
	}

	w("pieceCID=%s\n", result.PieceCID)
	w("size=%d\n", result.Size)
	w("requestedCopies=%d\n", result.RequestedCopies)
	w("complete=%t\n", result.Complete)
	w("\nCopies (%d):\n", len(result.Copies))
	for i, c := range result.Copies {
		w("  [%d] provider=%d role=%s retrievalURL=%s\n",
			i+1, c.ProviderID, c.Role, c.RetrievalURL)
	}
	if len(result.FailedAttempts) > 0 {
		w("\nFailed attempts (%d):\n", len(result.FailedAttempts))
		for i, f := range result.FailedAttempts {
			w("  [%d] provider=%d role=%s stage=%s err=%v\n",
				i+1, f.ProviderID, f.Role, f.Stage, f.Err)
		}
	}
	return writeErr
}
