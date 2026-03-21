// Upload example demonstrates a package-first MVP upload using storage.Manager
// plus the supporting warmstorage/spregistry/internal-curio services.
//
// Usage:
//
//	export PRIVATE_KEY=0x...
//	export RPC_URL=https://api.calibration.node.glif.io
//	export CHAIN=calibration # optional, default: calibration
//	export COPIES=2          # optional, default: 2
//	go run ./examples/upload/ ./path/to/file
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/strahe/synapse-go/chain"
	iabi "github.com/strahe/synapse-go/internal/abi"
	icurio "github.com/strahe/synapse-go/internal/curio"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

type uploadConfig struct {
	FilePath      string
	RPCURL        string
	PrivateKeyHex string
	Chain         chain.Chain
	Copies        int
}

type uploader interface {
	Upload(context.Context, io.Reader, *storage.UploadOptions) (*storage.UploadResult, error)
}

func main() {
	if err := realMain(context.Background(), os.Args[1:], os.Getenv, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	cfg, err := parseUploadConfig(args, getenv)
	if err != nil {
		return err
	}
	manager, closeFn, err := buildUploadManager(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = closeFn() }()
	return runUpload(ctx, cfg, manager, stdout)
}

func parseUploadConfig(args []string, getenv func(string) string) (uploadConfig, error) {
	if len(args) != 1 {
		return uploadConfig{}, errors.New("usage: go run ./examples/upload/ <file-path>")
	}
	rpcURL := strings.TrimSpace(getenv("RPC_URL"))
	if rpcURL == "" {
		return uploadConfig{}, errors.New("RPC_URL is required")
	}
	privateKeyHex := strings.TrimSpace(getenv("PRIVATE_KEY"))
	if privateKeyHex == "" {
		return uploadConfig{}, errors.New("PRIVATE_KEY is required")
	}
	selectedChain := chain.Calibration
	if rawChain := strings.TrimSpace(getenv("CHAIN")); rawChain != "" {
		switch strings.ToLower(rawChain) {
		case "calibration":
			selectedChain = chain.Calibration
		case "mainnet":
			selectedChain = chain.Mainnet
		default:
			return uploadConfig{}, fmt.Errorf("unsupported CHAIN %q", rawChain)
		}
	}
	copies := 2
	if rawCopies := strings.TrimSpace(getenv("COPIES")); rawCopies != "" {
		parsed, err := strconv.Atoi(rawCopies)
		if err != nil || parsed <= 0 {
			return uploadConfig{}, fmt.Errorf("invalid COPIES %q", rawCopies)
		}
		copies = parsed
	}
	return uploadConfig{
		FilePath:      args[0],
		RPCURL:        rpcURL,
		PrivateKeyHex: privateKeyHex,
		Chain:         selectedChain,
		Copies:        copies,
	}, nil
}

func runUpload(ctx context.Context, cfg uploadConfig, uploader uploader, stdout io.Writer) error {
	file, err := os.Open(cfg.FilePath)
	if err != nil {
		return fmt.Errorf("open %s: %w", cfg.FilePath, err)
	}
	defer func() { _ = file.Close() }()

	result, err := uploader.Upload(ctx, file, &storage.UploadOptions{Copies: cfg.Copies})
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(stdout, "pieceCID=%s\n", result.PieceCID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "size=%d\n", result.Size); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "requestedCopies=%d\n", result.RequestedCopies); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "complete=%t\n", result.Complete); err != nil {
		return err
	}
	for _, copy := range result.Copies {
		if _, err := fmt.Fprintf(stdout, "providerID=%s retrievalURL=%s\n", copy.ProviderID, copy.RetrievalURL); err != nil {
			return err
		}
	}
	return nil
}

func buildUploadManager(ctx context.Context, cfg uploadConfig) (*storage.Manager, func() error, error) {
	rpcClient, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		return nil, nil, fmt.Errorf("dial RPC_URL: %w", err)
	}
	closeFn := func() error {
		rpcClient.Close()
		return nil
	}

	evmSigner, err := signerFromHex(cfg.PrivateKeyHex)
	if err != nil {
		_ = closeFn()
		return nil, nil, err
	}
	addresses := iabi.ResolvedAddressesFromChain(cfg.Chain)

	warmStorageSvc, err := warmstorage.New(warmstorage.Options{
		Client:       rpcClient,
		FWSS:         addresses.FWSS,
		ViewContract: addresses.ViewContract,
	})
	if err != nil {
		_ = closeFn()
		return nil, nil, err
	}
	spRegistrySvc, err := spregistry.New(spregistry.Options{
		Client:  rpcClient,
		Address: addresses.SPRegistry,
	})
	if err != nil {
		_ = closeFn()
		return nil, nil, err
	}

	resolver, err := storage.NewServiceResolver(storage.ServiceResolverOptions{
		Payer:       evmSigner.EVMAddress(),
		SPRegistry:  spRegistrySvc,
		WarmStorage: warmStorageSvc,
		NewContext: func(selection storage.ResolvedUploadContext, opts *storage.UploadOptions) (storage.UploadContext, error) {
			curioClient, err := icurio.New(selection.Provider.ServiceURL)
			if err != nil {
				return nil, err
			}
			return storage.NewContext(
				selection.Provider,
				curioClient,
				evmSigner,
				storage.WithPayer(evmSigner.EVMAddress()),
				storage.WithChainID(cfg.Chain.BigChainID()),
				storage.WithRecordKeeper(addresses.FWSS),
				storage.WithDataSetID(selection.DataSetID),
				storage.WithDataSetMetadata(selection.DataSetMetadata),
				storage.WithCDN(opts != nil && opts.WithCDN),
			)
		},
	})
	if err != nil {
		_ = closeFn()
		return nil, nil, err
	}

	return storage.NewManager(storage.WithUploadResolver(resolver)), closeFn, nil
}

func signerFromHex(raw string) (signer.EVMSigner, error) {
	decoded, err := hex.DecodeString(strings.TrimPrefix(strings.TrimSpace(raw), "0x"))
	if err != nil {
		return nil, fmt.Errorf("decode PRIVATE_KEY: %w", err)
	}
	s, err := signer.NewSecp256k1SignerFromBytes(decoded)
	if err != nil {
		return nil, fmt.Errorf("build signer: %w", err)
	}
	return s, nil
}
