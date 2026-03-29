package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/piece"
)

func TestParseDownloadConfig_UsesExplicitOutputPath(t *testing.T) {
	info := mustDownloadPieceInfo(t)

	cfg, err := parseDownloadConfig([]string{
		info.CIDv2.String(),
		"https://sp.example.com/piece/" + info.CIDv2.String(),
		"./out.bin",
	})
	if err != nil {
		t.Fatalf("parseDownloadConfig: %v", err)
	}
	if cfg.PieceCID != info.CIDv2 {
		t.Fatalf("PieceCID=%s want %s", cfg.PieceCID, info.CIDv2)
	}
	if cfg.PieceURL != "https://sp.example.com/piece/"+info.CIDv2.String() {
		t.Fatalf("PieceURL=%q", cfg.PieceURL)
	}
	if cfg.OutputPath != "./out.bin" {
		t.Fatalf("OutputPath=%q", cfg.OutputPath)
	}
}

func TestRunDownload_WritesPieceToOutputFile(t *testing.T) {
	data := bytes.Repeat([]byte("dl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "piece.bin")

	fake := &fakeDownloader{
		downloadFn: func(_ context.Context, pieceCID cid.Cid, pieceURL string) (io.ReadCloser, error) {
			if pieceCID != info.CIDv2 {
				t.Fatalf("pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			if pieceURL == "" {
				t.Fatal("pieceURL should not be empty")
			}
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	}

	if err := runDownload(context.Background(), downloadConfig{
		PieceCID:   info.CIDv2,
		PieceURL:   "https://sp.example.com/piece/" + info.CIDv2.String(),
		OutputPath: outputPath,
	}, fake); err != nil {
		t.Fatalf("runDownload: %v", err)
	}
	got, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded file mismatch")
	}
}

func mustDownloadPieceInfo(t *testing.T) piece.PieceInfo {
	t.Helper()
	info, err := piece.CalculateFromBytes(bytes.Repeat([]byte("dw"), 128))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	return info
}

type fakeDownloader struct {
	downloadFn func(context.Context, cid.Cid, string) (io.ReadCloser, error)
}

func (f *fakeDownloader) DownloadPiece(ctx context.Context, pieceCID cid.Cid, pieceURL string) (io.ReadCloser, error) {
	return f.downloadFn(ctx, pieceCID, pieceURL)
}
