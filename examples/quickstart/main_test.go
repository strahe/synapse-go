package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/examples/internal/exampleutil"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
)

func TestParseConfigAcceptsOnlyFile(t *testing.T) {
	cfg, err := parseConfig([]string{"--file", "./payload.bin"})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.FilePath != "./payload.bin" {
		t.Fatalf("FilePath=%q", cfg.FilePath)
	}
}

func TestParseConfigRejectsAdvancedUploadFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--copies", "2"},
		{"--cdn"},
		{"--source", "test"},
	} {
		_, err := parseConfig(args)
		if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
			t.Fatalf("parseConfig(%v) err=%v want unknown flag", args, err)
		}
	}
}

func TestLoadPayloadUsesDefaultPayload(t *testing.T) {
	payload, err := loadPayload("")
	if err != nil {
		t.Fatalf("loadPayload: %v", err)
	}
	if err := exampleutil.ValidateUploadSize("payload", int64(len(payload))); err != nil {
		t.Fatalf("default payload should be uploadable: %v", err)
	}

	payload[0] = 'x'
	again, err := loadPayload("")
	if err != nil {
		t.Fatalf("loadPayload again: %v", err)
	}
	if again[0] == 'x' {
		t.Fatal("default payload was not copied")
	}
}

func TestLoadPayloadReadsFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payload.bin")
	want := bytes.Repeat([]byte("file"), 64)
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := loadPayload(path)
	if err != nil {
		t.Fatalf("loadPayload: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("payload mismatch")
	}
}

func TestRunQuickstartPreparesUploadsAndDownloads(t *testing.T) {
	payload := bytes.Repeat([]byte("qs"), 128)
	info, err := piece.CalculateFromBytes(payload)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var executedPrepare bool
	var uploaded bool
	fake := &fakeQuickstartStorage{
		prepareFn: func(_ context.Context, opts *storage.PrepareOptions) (*storage.PrepareResult, error) {
			if opts.DataSize != uint64(len(payload)) {
				t.Fatalf("DataSize=%d want %d", opts.DataSize, len(payload))
			}
			if len(opts.Contexts) != 0 {
				t.Fatalf("Prepare Contexts=%d want default contexts", len(opts.Contexts))
			}
			return &storage.PrepareResult{
				Costs: &storage.MultiContextCosts{
					DepositNeeded:        big.NewInt(1234),
					NeedsFWSSMaxApproval: true,
					Ready:                false,
				},
				Transaction: &storage.PrepareTransaction{
					DepositAmount:    big.NewInt(1234),
					IncludesApproval: true,
					Execute: func(context.Context, ...payments.WriteOption) (*types.WriteResult, error) {
						executedPrepare = true
						return &types.WriteResult{Hash: common.HexToHash("0x1234")}, nil
					},
				},
			}, nil
		},
		uploadFn: func(_ context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatal("uploaded payload mismatch")
			}
			if opts != nil {
				t.Fatalf("UploadOptions=%#v want nil defaults", opts)
			}
			uploaded = true
			return &storage.UploadResult{
				PieceCID:        info.CIDv2,
				Size:            int64(len(payload)),
				RequestedCopies: 2,
				Complete:        true,
				Copies: []storage.CopyResult{
					{
						ProviderID:   types.ProviderID(101),
						DataSetID:    types.DataSetID(202),
						PieceID:      types.PieceID(303),
						Role:         storage.CopyRolePrimary,
						RetrievalURL: "https://provider.example/piece/" + info.CIDv2.String(),
					},
				},
			}, nil
		},
		downloadFn: func(_ context.Context, pieceCID cid.Cid, opts *storage.DownloadOptions) (io.ReadCloser, error) {
			if pieceCID != info.CIDv2 {
				t.Fatalf("pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			if opts.URL == "" {
				t.Fatal("download URL is empty")
			}
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}

	var stdout bytes.Buffer
	err = runQuickstart(context.Background(), quickstartConfig{Payload: payload}, fake, &stdout)
	if err != nil {
		t.Fatalf("runQuickstart: %v", err)
	}
	if !executedPrepare {
		t.Fatal("prepare transaction was not executed")
	}
	if !uploaded {
		t.Fatal("upload was not called")
	}
	out := stdout.String()
	for _, want := range []string{
		"ready=false",
		"depositNeeded=1234",
		"prepare.txHash=0x0000000000000000000000000000000000000000000000000000000000001234",
		"pieceCID=" + info.CIDv2.String(),
		"copy.1.providerID=101",
		"verified=true",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestRunQuickstartRejectsSmallPayloadBeforePreparing(t *testing.T) {
	fake := &fakeQuickstartStorage{
		prepareFn: func(context.Context, *storage.PrepareOptions) (*storage.PrepareResult, error) {
			t.Fatal("Prepare should not be called")
			return nil, nil
		},
	}
	err := runQuickstart(context.Background(), quickstartConfig{Payload: []byte("small")}, fake, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "too small") {
		t.Fatalf("err=%v want too small", err)
	}
}

func TestDownloadAndVerifyRetriesTransientDownload(t *testing.T) {
	payload := bytes.Repeat([]byte("rv"), 128)
	info, err := piece.CalculateFromBytes(payload)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	attempts := 0
	fake := &fakeQuickstartStorage{
		downloadFn: func(_ context.Context, pieceCID cid.Cid, opts *storage.DownloadOptions) (io.ReadCloser, error) {
			if pieceCID != info.CIDv2 {
				t.Fatalf("pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			if opts.URL == "" {
				t.Fatal("download URL is empty")
			}
			attempts++
			if attempts == 1 {
				return nil, errors.New("piece not ready")
			}
			return io.NopCloser(bytes.NewReader(payload)), nil
		},
	}
	err = downloadAndVerify(context.Background(), fake, info.CIDv2, "https://provider.example/piece/"+info.CIDv2.String(), payload, 2, 0)
	if err != nil {
		t.Fatalf("downloadAndVerify: %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts=%d want 2", attempts)
	}
}

type fakeQuickstartStorage struct {
	prepareFn  func(context.Context, *storage.PrepareOptions) (*storage.PrepareResult, error)
	uploadFn   func(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
	downloadFn func(context.Context, cid.Cid, *storage.DownloadOptions) (io.ReadCloser, error)
}

func (f *fakeQuickstartStorage) Prepare(ctx context.Context, opts *storage.PrepareOptions) (*storage.PrepareResult, error) {
	if f.prepareFn == nil {
		return nil, errors.New("unexpected Prepare call")
	}
	return f.prepareFn(ctx, opts)
}

func (f *fakeQuickstartStorage) Upload(ctx context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
	if f.uploadFn == nil {
		return nil, errors.New("unexpected Upload call")
	}
	return f.uploadFn(ctx, r, opts)
}

func (f *fakeQuickstartStorage) Download(ctx context.Context, pieceCID cid.Cid, opts *storage.DownloadOptions) (io.ReadCloser, error) {
	if f.downloadFn == nil {
		return nil, errors.New("unexpected Download call")
	}
	return f.downloadFn(ctx, pieceCID, opts)
}
