// Download example demonstrates the current MVP download surface: callers
// provide the PieceCID together with an already-resolved piece URL, and the
// SDK streams the bytes while validating the PieceCID.
//
// Usage:
//
//	go run ./examples/download/ <piece-cid> <piece-url> [output-path]
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/storage"
)

type downloadConfig struct {
	PieceCID   cid.Cid
	PieceURL   string
	OutputPath string
}

type pieceDownloader interface {
	DownloadPiece(context.Context, cid.Cid, string) (io.ReadCloser, error)
}

func main() {
	if err := realMain(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context, args []string) error {
	cfg, err := parseDownloadConfig(args)
	if err != nil {
		return err
	}
	return runDownload(ctx, cfg, managerDownloader{manager: storage.NewManager()})
}

func parseDownloadConfig(args []string) (downloadConfig, error) {
	if len(args) < 2 || len(args) > 3 {
		return downloadConfig{}, errors.New("usage: go run ./examples/download/ <piece-cid> <piece-url> [output-path]")
	}
	pieceCID, err := cid.Parse(args[0])
	if err != nil {
		return downloadConfig{}, fmt.Errorf("parse piece CID: %w", err)
	}
	outputPath := pieceCID.String() + ".bin"
	if len(args) == 3 {
		outputPath = args[2]
	}
	return downloadConfig{
		PieceCID:   pieceCID,
		PieceURL:   args[1],
		OutputPath: outputPath,
	}, nil
}

func runDownload(ctx context.Context, cfg downloadConfig, downloader pieceDownloader) error {
	reader, err := downloader.DownloadPiece(ctx, cfg.PieceCID, cfg.PieceURL)
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()

	if dir := filepath.Dir(cfg.OutputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output dir: %w", err)
		}
	}
	file, err := os.Create(cfg.OutputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}

type managerDownloader struct {
	manager *storage.Manager
}

func (d managerDownloader) DownloadPiece(ctx context.Context, pieceCID cid.Cid, pieceURL string) (io.ReadCloser, error) {
	return d.manager.Download(ctx, pieceCID, &storage.DownloadOptions{URL: pieceURL})
}
