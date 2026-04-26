package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/storage"
)

func TestParseConfigRequiresCIDAndURL(t *testing.T) {
	_, err := parseConfig(nil)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("err=%v want usage error", err)
	}
}

func TestRunDownloadWritesOutput(t *testing.T) {
	data := bytes.Repeat([]byte("dl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	output := filepath.Join(t.TempDir(), "payload.bin")
	fake := &fakeDownloader{
		downloadFn: func(_ context.Context, pieceCID cid.Cid, opts *storage.DownloadOptions) (io.ReadCloser, error) {
			if pieceCID != info.CIDv2 {
				t.Fatalf("pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			if opts.URL != "https://provider.example/piece/"+info.CIDv2.String() {
				t.Fatalf("URL=%q", opts.URL)
			}
			return io.NopCloser(bytes.NewReader(data)), nil
		},
	}

	var stdout bytes.Buffer
	err = runDownload(context.Background(), downloadConfig{
		PieceCID: info.CIDv2,
		URL:      "https://provider.example/piece/" + info.CIDv2.String(),
		Output:   output,
	}, fake, &stdout)
	if err != nil {
		t.Fatalf("runDownload: %v", err)
	}
	got, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded data mismatch")
	}
	out := stdout.String()
	for _, want := range []string{
		"pieceCID=" + info.CIDv2.String(),
		"output=" + output,
		"bytes=256",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q\ngot:\n%s", want, out)
		}
	}
}

func TestRunDownloadDoesNotLeaveOutputOnReadError(t *testing.T) {
	data := bytes.Repeat([]byte("bad"), 64)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	output := filepath.Join(t.TempDir(), "payload.bin")
	fake := &fakeDownloader{
		downloadFn: func(context.Context, cid.Cid, *storage.DownloadOptions) (io.ReadCloser, error) {
			return io.NopCloser(errorReader{}), nil
		},
	}

	var stdout bytes.Buffer
	err = runDownload(context.Background(), downloadConfig{
		PieceCID: info.CIDv2,
		URL:      "https://provider.example/piece/" + info.CIDv2.String(),
		Output:   output,
	}, fake, &stdout)
	if err == nil {
		t.Fatal("runDownload returned nil error")
	}
	if _, statErr := os.Stat(output); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("output file exists after failed validation: statErr=%v", statErr)
	}
}

type fakeDownloader struct {
	downloadFn func(context.Context, cid.Cid, *storage.DownloadOptions) (io.ReadCloser, error)
}

func (f *fakeDownloader) Download(ctx context.Context, pieceCID cid.Cid, opts *storage.DownloadOptions) (io.ReadCloser, error) {
	return f.downloadFn(ctx, pieceCID, opts)
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("validation failed")
}
