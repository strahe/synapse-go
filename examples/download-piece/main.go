// Download-piece downloads a retrieval URL and verifies it against a PieceCID.
//
// Usage:
//
//	go run ./examples/download-piece --piece-cid <piece-cid> --url <retrieval-url> --out payload.bin
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/examples/internal/exampleutil"
	"github.com/strahe/synapse-go/storage"
)

type downloadConfig struct {
	PieceCID cid.Cid
	URL      string
	Output   string
	MaxBytes int64
}

type pieceDownloader interface {
	Download(context.Context, cid.Cid, *storage.DownloadOptions) (io.ReadCloser, error)
}

func main() {
	ctx, cancel := exampleutil.WithTimeout(context.Background(), 30*time.Minute)
	err := realMain(ctx, os.Args[1:], os.Stdout)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context, args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	svc, err := storage.New(storage.Options{
		AllowPrivateNetworks: true,
		DownloadMaxBytes:     cfg.MaxBytes,
	})
	if err != nil {
		return fmt.Errorf("create storage service: %w", err)
	}
	return runDownload(ctx, cfg, svc, stdout)
}

func parseConfig(args []string) (downloadConfig, error) {
	var rawCID string
	cfg := downloadConfig{}
	fs := flag.NewFlagSet("download-piece", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&rawCID, "piece-cid", "", "PieceCIDv1 or PieceCIDv2")
	fs.StringVar(&cfg.URL, "url", "", "retrieval URL")
	fs.StringVar(&cfg.Output, "out", "", "output file path")
	fs.Int64Var(&cfg.MaxBytes, "max-bytes", 0, "optional download byte cap")
	if err := fs.Parse(args); err != nil {
		return downloadConfig{}, err
	}
	if rawCID == "" || cfg.URL == "" || fs.NArg() != 0 {
		return downloadConfig{}, errors.New("usage: go run ./examples/download-piece --piece-cid <piece-cid> --url <retrieval-url> [--out path]")
	}
	pieceCID, err := cid.Parse(rawCID)
	if err != nil {
		return downloadConfig{}, fmt.Errorf("invalid piece CID: %w", err)
	}
	if cfg.Output == "" {
		cfg.Output = pieceCID.String() + ".bin"
	}
	if cfg.MaxBytes < 0 {
		return downloadConfig{}, fmt.Errorf("max-bytes must be non-negative, got %d", cfg.MaxBytes)
	}
	cfg.PieceCID = pieceCID
	return cfg, nil
}

func runDownload(ctx context.Context, cfg downloadConfig, downloader pieceDownloader, stdout io.Writer) error {
	reader, err := downloader.Download(ctx, cfg.PieceCID, &storage.DownloadOptions{URL: cfg.URL})
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	defer func() { _ = reader.Close() }()

	if dir := filepath.Dir(cfg.Output); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(cfg.Output), "."+filepath.Base(cfg.Output)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp output file: %w", err)
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	written, copyErr := io.Copy(tmp, reader)
	closeErr := tmp.Close()
	if copyErr != nil {
		return fmt.Errorf("write output file: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close output file: %w", closeErr)
	}
	if err := os.Rename(tmpPath, cfg.Output); err != nil {
		return fmt.Errorf("rename output file: %w", err)
	}
	renamed = true
	if err := exampleutil.WriteKV(stdout, "pieceCID", cfg.PieceCID); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "output", cfg.Output); err != nil {
		return err
	}
	return exampleutil.WriteKV(stdout, "bytes", written)
}
