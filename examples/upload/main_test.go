package main

import (
	"bytes"
	"context"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/storage"
)

func TestParseUploadConfig_DefaultsToCalibrationAndTwoCopies(t *testing.T) {
	cfg, err := parseUploadConfig([]string{"./payload.bin"}, func(key string) string {
		switch key {
		case "RPC_URL":
			return "https://api.calibration.node.glif.io"
		case "PRIVATE_KEY":
			return "0x1234"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("parseUploadConfig: %v", err)
	}
	if cfg.FilePath != "./payload.bin" {
		t.Fatalf("FilePath=%q want ./payload.bin", cfg.FilePath)
	}
	if cfg.RPCURL != "https://api.calibration.node.glif.io" {
		t.Fatalf("RPCURL=%q", cfg.RPCURL)
	}
	if cfg.PrivateKeyHex != "0x1234" {
		t.Fatalf("PrivateKeyHex=%q", cfg.PrivateKeyHex)
	}
	if cfg.Chain != nil {
		t.Fatalf("Chain=%v want nil (auto-detect)", cfg.Chain)
	}
	if cfg.Copies != 2 {
		t.Fatalf("Copies=%d want 2", cfg.Copies)
	}
}

func TestRunUpload_ReadsFileAndPrintsSummary(t *testing.T) {
	data := bytes.Repeat([]byte("up"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	dir := t.TempDir()
	filePath := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var stdout bytes.Buffer
	fake := &fakeUploader{
		uploadFn: func(_ context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("uploaded data mismatch")
			}
			if opts == nil || opts.Copies != 2 {
				t.Fatalf("opts=%+v want copies=2", opts)
			}
			return &storage.UploadResult{
				PieceCID:        info.CIDv2,
				Size:            int64(len(data)),
				RequestedCopies: 2,
				Complete:        true,
				Copies: []storage.CopyResult{
					{ProviderID: big.NewInt(101), RetrievalURL: "https://sp.example.com/piece/" + info.CIDv2.String()},
					{ProviderID: big.NewInt(202), RetrievalURL: "https://sp2.example.com/piece/" + info.CIDv2.String()},
				},
			}, nil
		},
	}

	err = runUpload(context.Background(), uploadConfig{
		FilePath: filePath,
		Copies:   2,
	}, fake, &stdout)
	if err != nil {
		t.Fatalf("runUpload: %v", err)
	}
	if got := stdout.String(); !strings.Contains(got, info.CIDv2.String()) || !strings.Contains(got, "complete=true") {
		t.Fatalf("stdout=%q", got)
	}
}

type fakeUploader struct {
	uploadFn func(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
}

func (f *fakeUploader) Upload(ctx context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
	return f.uploadFn(ctx, r, opts)
}
