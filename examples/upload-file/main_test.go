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
	"github.com/strahe/synapse-go/types"
)

func TestParseConfigAcceptsPositionalFileAndMetadata(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--copies", "3",
		"--cdn",
		"--dataset-metadata", "app=docs",
		"--piece-metadata", "kind=sample",
		"./payload.bin",
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.FilePath != "./payload.bin" {
		t.Fatalf("FilePath=%q", cfg.FilePath)
	}
	if cfg.Copies != 3 {
		t.Fatalf("Copies=%d want 3", cfg.Copies)
	}
	if !cfg.CDN {
		t.Fatal("CDN=false want true")
	}
	if cfg.DataSetMetadata.Map()["app"] != "docs" {
		t.Fatalf("dataset metadata=%v", cfg.DataSetMetadata.Map())
	}
	if cfg.PieceMetadata.Map()["kind"] != "sample" {
		t.Fatalf("piece metadata=%v", cfg.PieceMetadata.Map())
	}
}

func TestParseConfigRejectsFileFlagWithExtraPath(t *testing.T) {
	_, err := parseConfig([]string{"--file", "good.bin", "ignored.bin"})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("err=%v want usage error", err)
	}
}

func TestRunUploadPreparesAndPrintsCopySummary(t *testing.T) {
	data := bytes.Repeat([]byte("up"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fake := &fakeUploadStorage{
		createFn: func(_ context.Context, opts *storage.CreateContextsOptions) ([]*storage.Context, error) {
			if opts.Copies != 2 {
				t.Fatalf("CreateContexts Copies=%d want 2", opts.Copies)
			}
			if opts.DataSetMetadata["app"] != "docs" {
				t.Fatalf("DataSetMetadata=%v", opts.DataSetMetadata)
			}
			return make([]*storage.Context, opts.Copies), nil
		},
		prepareFn: func(_ context.Context, opts *storage.PrepareOptions) (*storage.PrepareResult, error) {
			if opts.DataSize != uint64(len(data)) {
				t.Fatalf("DataSize=%d want %d", opts.DataSize, len(data))
			}
			if len(opts.Contexts) != 2 {
				t.Fatalf("Prepare Contexts=%d want 2", len(opts.Contexts))
			}
			return &storage.PrepareResult{
				Costs: &storage.MultiContextCosts{
					DepositNeeded: big.NewInt(0),
					Ready:         true,
				},
			}, nil
		},
		uploadFn: func(_ context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("uploaded data mismatch")
			}
			if opts.Copies != 2 {
				t.Fatalf("Copies=%d want 2", opts.Copies)
			}
			if opts.OnProgress != nil {
				opts.OnProgress(int64(len(data)))
			}
			if opts.OnStored != nil {
				opts.OnStored(types.NewBigInt(7), info.CIDv2)
			}
			return &storage.UploadResult{
				PieceCID:        info.CIDv2,
				Size:            int64(len(data)),
				RequestedCopies: 2,
				Complete:        true,
				Copies: []storage.CopyResult{
					{
						ProviderID:   types.NewBigInt(7),
						DataSetID:    types.NewBigInt(8),
						PieceID:      types.NewBigInt(9),
						Role:         storage.CopyRolePrimary,
						RetrievalURL: "https://provider.example/piece/" + info.CIDv2.String(),
					},
				},
			}, nil
		},
	}

	var stdout bytes.Buffer
	err = runUpload(context.Background(), uploadConfig{
		FilePath: path,
		Copies:   2,
		Source:   "test",
		DataSetMetadata: map[string]string{
			"app": "docs",
		},
	}, fake, &stdout)
	if err != nil {
		t.Fatalf("runUpload: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{
		"ready=true",
		"uploadedBytes=256",
		"storedProviderID=7",
		"pieceCID=" + info.CIDv2.String(),
		"copy.1.dataSetID=8",
		"copy.1.retrievalURL=https://provider.example/piece/",
		"downloadCommand=go run ./examples/download-piece",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestRunUploadRejectsSmallFileBeforePreparing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.bin")
	if err := os.WriteFile(path, []byte("small"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	fake := &fakeUploadStorage{
		createFn: func(context.Context, *storage.CreateContextsOptions) ([]*storage.Context, error) {
			t.Fatal("CreateContexts should not be called")
			return nil, nil
		},
	}
	err := runUpload(context.Background(), uploadConfig{
		FilePath: path,
		Copies:   2,
	}, fake, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "too small") {
		t.Fatalf("err=%v want too small", err)
	}
}

func TestRunUploadRejectsDirectoryBeforePreparing(t *testing.T) {
	fake := &fakeUploadStorage{
		createFn: func(context.Context, *storage.CreateContextsOptions) ([]*storage.Context, error) {
			t.Fatal("CreateContexts should not be called")
			return nil, nil
		},
	}
	err := runUpload(context.Background(), uploadConfig{
		FilePath: t.TempDir(),
		Copies:   2,
	}, fake, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("err=%v want not regular file", err)
	}
}

type fakeUploadStorage struct {
	createFn  func(context.Context, *storage.CreateContextsOptions) ([]*storage.Context, error)
	prepareFn func(context.Context, *storage.PrepareOptions) (*storage.PrepareResult, error)
	uploadFn  func(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
}

func (f *fakeUploadStorage) CreateContexts(ctx context.Context, opts *storage.CreateContextsOptions) ([]*storage.Context, error) {
	return f.createFn(ctx, opts)
}

func (f *fakeUploadStorage) Prepare(ctx context.Context, opts *storage.PrepareOptions) (*storage.PrepareResult, error) {
	return f.prepareFn(ctx, opts)
}

func (f *fakeUploadStorage) Upload(ctx context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
	return f.uploadFn(ctx, r, opts)
}
