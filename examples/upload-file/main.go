// Upload-file uploads a local file to FOC storage.
//
// Usage:
//
//	export SYNAPSE_PRIVATE_KEY=0x...
//	go run ./examples/upload-file --file ./payload.bin --copies 2 --cdn
package main

import (
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
	"github.com/strahe/synapse-go/types"
)

type uploadConfig struct {
	FilePath        string
	Copies          int
	CDN             bool
	Source          string
	DataSetMetadata exampleutil.MetadataFlag
	PieceMetadata   exampleutil.MetadataFlag
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
	env, err := exampleutil.LoadEnv(getenv)
	if err != nil {
		return err
	}
	client, err := exampleutil.NewClient(ctx, env, synapse.WithSource(cfg.Source), synapse.WithCDN(cfg.CDN))
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
	return runUpload(ctx, cfg, storageWorkflow{svc: client.Storage()}, stdout)
}

func parseConfig(args []string) (uploadConfig, error) {
	cfg := uploadConfig{
		Copies: 2,
		Source: "synapse-go-upload-file",
	}
	fs := flag.NewFlagSet("upload-file", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.FilePath, "file", "", "file to upload")
	fs.IntVar(&cfg.Copies, "copies", cfg.Copies, "number of provider copies")
	fs.BoolVar(&cfg.CDN, "cdn", false, "request CDN-enabled storage")
	fs.StringVar(&cfg.Source, "source", cfg.Source, "dataset namespace source")
	fs.Var(&cfg.DataSetMetadata, "dataset-metadata", "dataset metadata key=value; repeatable")
	fs.Var(&cfg.PieceMetadata, "piece-metadata", "piece metadata key=value; repeatable")
	if err := fs.Parse(args); err != nil {
		return uploadConfig{}, err
	}
	switch {
	case cfg.FilePath == "" && fs.NArg() == 1:
		cfg.FilePath = fs.Arg(0)
	case cfg.FilePath == "" || fs.NArg() != 0:
		return uploadConfig{}, errors.New("usage: go run ./examples/upload-file --file ./payload.bin [--copies n] [--cdn]")
	}
	if cfg.Copies <= 0 {
		return uploadConfig{}, fmt.Errorf("copies must be positive, got %d", cfg.Copies)
	}
	return cfg, nil
}

func runUpload(ctx context.Context, cfg uploadConfig, svc uploadStorage, stdout io.Writer) error {
	file, info, err := exampleutil.OpenRegularFile(cfg.FilePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	if err := exampleutil.ValidateUploadSize(cfg.FilePath, info.Size()); err != nil {
		return err
	}

	withCDN := cfg.CDN
	dataSetMetadata := cfg.DataSetMetadata.Map()
	contexts, err := svc.CreateUploadContexts(ctx, &storage.CreateContextsOptions{
		Copies:          cfg.Copies,
		DataSetMetadata: dataSetMetadata,
		WithCDN:         &withCDN,
	})
	if err != nil {
		return fmt.Errorf("create upload contexts: %w", err)
	}
	prepare, err := svc.Prepare(ctx, &storage.PrepareOptions{
		DataSize: uint64(info.Size()),
		Contexts: contexts,
	})
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

	var callbackErr error
	result, err := svc.Upload(ctx, file, &storage.UploadOptions{
		Copies:          cfg.Copies,
		WithCDN:         &withCDN,
		DataSetMetadata: dataSetMetadata,
		PieceMetadata:   cfg.PieceMetadata.Map(),
		OnProgress: func(uploaded int64) {
			if uploaded == info.Size() && callbackErr == nil {
				callbackErr = exampleutil.WriteKV(stdout, "uploadedBytes", uploaded)
			}
		},
		OnStored: func(providerID types.ProviderID, _ cid.Cid) {
			if callbackErr == nil {
				callbackErr = exampleutil.WriteKV(stdout, "storedProviderID", providerID)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	if callbackErr != nil {
		return callbackErr
	}
	return printUpload(stdout, result)
}

type uploadStorage interface {
	CreateUploadContexts(context.Context, *storage.CreateContextsOptions) ([]storage.UploadContext, error)
	Prepare(context.Context, *storage.PrepareOptions) (*storage.PrepareResult, error)
	Upload(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
}

type storageWorkflow struct {
	svc *storage.Service
}

func (w storageWorkflow) CreateUploadContexts(ctx context.Context, opts *storage.CreateContextsOptions) ([]storage.UploadContext, error) {
	contexts, err := w.svc.CreateContexts(ctx, opts)
	if err != nil {
		return nil, err
	}
	out := make([]storage.UploadContext, len(contexts))
	for i, c := range contexts {
		out[i] = c
	}
	return out, nil
}

func (w storageWorkflow) Prepare(ctx context.Context, opts *storage.PrepareOptions) (*storage.PrepareResult, error) {
	return w.svc.Prepare(ctx, opts)
}

func (w storageWorkflow) Upload(ctx context.Context, r io.Reader, opts *storage.UploadOptions) (*storage.UploadResult, error) {
	return w.svc.Upload(ctx, r, opts)
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
	if err := exampleutil.WriteKV(stdout, "requestedCopies", result.RequestedCopies); err != nil {
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
		if err := exampleutil.WriteKV(stdout, prefix+".role", copy.Role); err != nil {
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
	if len(result.Copies) > 0 {
		cmd := fmt.Sprintf("go run ./examples/download-piece --piece-cid %s --url %s", result.PieceCID, result.Copies[0].RetrievalURL)
		return exampleutil.WriteKV(stdout, "downloadCommand", cmd)
	}
	return nil
}
