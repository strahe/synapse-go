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

	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/storage"
)

func TestParseConfig_Valid(t *testing.T) {
	cfg, err := parseConfig([]string{"./payload.bin"}, func(key string) string {
		switch key {
		case "RPC_URL":
			return "https://api.calibration.node.glif.io"
		case "PRIVATE_KEY":
			return "0xabc"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.FilePath != "./payload.bin" {
		t.Fatalf("FilePath=%q", cfg.FilePath)
	}
	if cfg.RPCURL != "https://api.calibration.node.glif.io" {
		t.Fatalf("RPCURL=%q", cfg.RPCURL)
	}
	if cfg.PrivateKeyHex != "0xabc" {
		t.Fatalf("PrivateKeyHex=%q", cfg.PrivateKeyHex)
	}
	if cfg.Chain != nil {
		t.Fatalf("Chain=%v want nil", cfg.Chain)
	}
}

func TestParseConfig_MissingPrivateKey(t *testing.T) {
	_, err := parseConfig([]string{"./f.bin"}, func(key string) string {
		if key == "RPC_URL" {
			return "https://rpc.example.com"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "PRIVATE_KEY") {
		t.Fatalf("err=%v want PRIVATE_KEY error", err)
	}
}

func TestParseConfig_MissingRPCURL(t *testing.T) {
	_, err := parseConfig([]string{"./f.bin"}, func(key string) string {
		if key == "PRIVATE_KEY" {
			return "0x1"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "RPC_URL") {
		t.Fatalf("err=%v want RPC_URL error", err)
	}
}

func TestParseConfig_MissingFilePath(t *testing.T) {
	_, err := parseConfig(nil, func(key string) string {
		switch key {
		case "RPC_URL":
			return "https://rpc.example.com"
		case "PRIVATE_KEY":
			return "0x1"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("err=%v want usage error", err)
	}
}

func TestRunMulticopy_ThreeCopies(t *testing.T) {
	data := bytes.Repeat([]byte("mc"), 128)
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
			if _, err := io.ReadAll(r); err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			if opts.Copies != 3 {
				t.Fatalf("Copies=%d want 3", opts.Copies)
			}
			if !opts.WithCDN {
				t.Fatal("WithCDN=false want true")
			}
			return &storage.UploadResult{
				PieceCID:        info.CIDv2,
				Size:            int64(len(data)),
				RequestedCopies: 3,
				Complete:        true,
				Copies: []storage.CopyResult{
					{ProviderID: big.NewInt(1), Role: storage.CopyRolePrimary, RetrievalURL: "https://sp1.example.com/piece/abc"},
					{ProviderID: big.NewInt(2), Role: storage.CopyRoleSecondary, RetrievalURL: "https://sp2.example.com/piece/abc"},
					{ProviderID: big.NewInt(3), Role: storage.CopyRoleSecondary, RetrievalURL: "https://sp3.example.com/piece/abc"},
				},
			}, nil
		},
	}

	err = runMulticopy(context.Background(), multicopyConfig{FilePath: filePath}, fake, &stdout)
	if err != nil {
		t.Fatalf("runMulticopy: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"pieceCID=" + info.CIDv2.String(),
		"requestedCopies=3",
		"complete=true",
		"role=primary",
		"role=secondary",
		"Copies (3)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

func TestRunMulticopy_PartialFailure(t *testing.T) {
	data := bytes.Repeat([]byte("pf"), 64)
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
		uploadFn: func(_ context.Context, r io.Reader, _ *storage.UploadOptions) (*storage.UploadResult, error) {
			if _, err := io.ReadAll(r); err != nil {
				t.Fatalf("ReadAll: %v", err)
			}
			return &storage.UploadResult{
				PieceCID:        info.CIDv2,
				Size:            int64(len(data)),
				RequestedCopies: 3,
				Complete:        false,
				Copies: []storage.CopyResult{
					{ProviderID: big.NewInt(1), Role: storage.CopyRolePrimary, RetrievalURL: "https://sp1.example.com/piece/abc"},
					{ProviderID: big.NewInt(2), Role: storage.CopyRoleSecondary, RetrievalURL: "https://sp2.example.com/piece/abc"},
				},
				FailedAttempts: []storage.FailedAttempt{
					{ProviderID: big.NewInt(3), Role: storage.CopyRoleSecondary, Stage: storage.CopyStagePull, Err: errors.New("timeout")},
				},
			}, nil
		},
	}

	err = runMulticopy(context.Background(), multicopyConfig{FilePath: filePath}, fake, &stdout)
	if err != nil {
		t.Fatalf("runMulticopy: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"complete=false",
		"Copies (2)",
		"Failed attempts (1)",
		"stage=pull",
		"timeout",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

type fakeUploader struct {
	uploadFn func(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
}

func (f *fakeUploader) Upload(ctx context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
	return f.uploadFn(ctx, r, opts)
}
