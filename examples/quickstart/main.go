// Quickstart runs the shortest complete FOC flow on Calibration:
// prepare funds when needed, upload data, download the first copy, and verify
// the bytes match.
//
// Usage:
//
//	export SYNAPSE_PRIVATE_KEY=0x...
//	go run ./examples/quickstart
//	go run ./examples/quickstart --file ./payload.bin
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/examples/internal/exampleutil"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/storage"
)

const (
	quickstartSource             = "synapse-go-quickstart"
	quickstartDownloadAttempts   = 5
	quickstartDownloadRetryDelay = 30 * time.Second
)

var defaultPayload = bytes.Repeat([]byte("synapse-go quickstart payload\n"), 8)

type quickstartConfig struct {
	FilePath string
	Payload  []byte
}

func main() {
	ctx, cancel := exampleutil.WithTimeout(context.Background(), 30*time.Minute)
	err := realMain(ctx, os.Args[1:], os.Getenv, os.Stdout)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	payload, err := loadPayload(cfg.FilePath)
	if err != nil {
		return err
	}
	cfg.Payload = payload

	env, err := exampleutil.LoadEnv(getenv)
	if err != nil {
		return err
	}
	client, err := exampleutil.NewClient(
		ctx,
		env,
		synapse.WithSource(quickstartSource),
	)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	if err := exampleutil.WriteKV(stdout, "chain", client.Chain()); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "address", client.Address().Hex()); err != nil {
		return err
	}
	return runQuickstart(ctx, cfg, storageWorkflow{svc: client.Storage()}, stdout)
}

func parseConfig(args []string) (quickstartConfig, error) {
	cfg := quickstartConfig{}
	fs := flag.NewFlagSet("quickstart", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.FilePath, "file", "", "optional file to upload")
	if err := fs.Parse(args); err != nil {
		return quickstartConfig{}, err
	}
	if fs.NArg() != 0 {
		return quickstartConfig{}, errors.New("usage: go run ./examples/quickstart [--file path]")
	}
	return cfg, nil
}

func loadPayload(path string) ([]byte, error) {
	if path == "" {
		return append([]byte(nil), defaultPayload...), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}
	return data, nil
}

func runQuickstart(ctx context.Context, cfg quickstartConfig, svc quickstartStorage, stdout io.Writer) error {
	if err := exampleutil.ValidateUploadSize("payload", int64(len(cfg.Payload))); err != nil {
		return err
	}

	prepare, err := svc.Prepare(ctx, &storage.PrepareOptions{DataSize: uint64(len(cfg.Payload))})
	if err != nil {
		return fmt.Errorf("prepare upload: %w", err)
	}
	if err := exampleutil.PrintPrepare(stdout, prepare); err != nil {
		return err
	}
	if prepare.Transaction != nil {
		result, err := prepare.Transaction.Execute(ctx, payments.WithWait(10*time.Minute))
		if err != nil {
			return fmt.Errorf("execute prepare transaction: %w", err)
		}
		if err := exampleutil.WriteTx(stdout, "prepare", result); err != nil {
			return err
		}
	}

	upload, err := svc.Upload(ctx, bytes.NewReader(cfg.Payload), nil)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	if err := printUpload(stdout, upload); err != nil {
		return err
	}
	if len(upload.Copies) == 0 {
		return errors.New("upload returned no committed copies")
	}
	first := upload.Copies[0]
	if first.RetrievalURL == "" {
		return errors.New("first upload copy has no retrieval URL")
	}

	if err := downloadAndVerify(ctx, svc, upload.PieceCID, first.RetrievalURL, cfg.Payload, quickstartDownloadAttempts, quickstartDownloadRetryDelay); err != nil {
		return err
	}
	return exampleutil.WriteKV(stdout, "verified", true)
}

type quickstartStorage interface {
	Prepare(context.Context, *storage.PrepareOptions) (*storage.PrepareResult, error)
	Upload(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
	Download(context.Context, cid.Cid, *storage.DownloadOptions) (io.ReadCloser, error)
}

type storageWorkflow struct {
	svc *storage.Service
}

func (w storageWorkflow) Prepare(ctx context.Context, opts *storage.PrepareOptions) (*storage.PrepareResult, error) {
	return w.svc.Prepare(ctx, opts)
}

func (w storageWorkflow) Upload(ctx context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
	return w.svc.Upload(ctx, r, opts)
}

func (w storageWorkflow) Download(ctx context.Context, pieceCID cid.Cid, opts *storage.DownloadOptions) (io.ReadCloser, error) {
	return w.svc.Download(ctx, pieceCID, opts)
}

func downloadAndVerify(ctx context.Context, svc quickstartStorage, pieceCID cid.Cid, url string, want []byte, attempts int, delay time.Duration) error {
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("download first copy: %w", err)
		}
		if err := tryDownloadAndVerify(ctx, svc, pieceCID, url, want); err != nil {
			lastErr = err
		} else {
			return nil
		}
		if attempt == attempts {
			break
		}
		if delay <= 0 {
			continue
		}
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return fmt.Errorf("download first copy: %w", ctx.Err())
		}
	}
	return fmt.Errorf("download first copy after %d attempt(s): %w", attempts, lastErr)
}

func tryDownloadAndVerify(ctx context.Context, svc quickstartStorage, pieceCID cid.Cid, url string, want []byte) error {
	reader, err := svc.Download(ctx, pieceCID, &storage.DownloadOptions{URL: url})
	if err != nil {
		return err
	}
	downloaded, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return fmt.Errorf("read download: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close download: %w", closeErr)
	}
	if !bytes.Equal(downloaded, want) {
		return errors.New("downloaded bytes do not match uploaded payload")
	}
	return nil
}

func printUpload(stdout io.Writer, result *storage.UploadResult) error {
	if result == nil {
		return errors.New("upload returned nil result")
	}
	if err := exampleutil.WriteKV(stdout, "pieceCID", result.PieceCID); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "size", result.Size); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "complete", result.Complete); err != nil {
		return err
	}
	for i, copy := range result.Copies {
		prefix := fmt.Sprintf("copy.%d", i+1)
		if err := exampleutil.WriteKV(stdout, prefix+".providerID", copy.ProviderID); err != nil {
			return err
		}
		if err := exampleutil.WriteKV(stdout, prefix+".dataSetID", copy.DataSetID); err != nil {
			return err
		}
		if err := exampleutil.WriteKV(stdout, prefix+".pieceID", copy.PieceID); err != nil {
			return err
		}
		if err := exampleutil.WriteKV(stdout, prefix+".retrievalURL", copy.RetrievalURL); err != nil {
			return err
		}
	}
	return nil
}
