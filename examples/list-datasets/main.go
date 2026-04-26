// List-datasets shows the storage state for the current wallet.
//
// Usage:
//
//	export SYNAPSE_PRIVATE_KEY=0x...
//	go run ./examples/list-datasets --managed
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/strahe/synapse-go/examples/internal/exampleutil"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
)

type listConfig struct {
	OnlyManaged bool
	DataSetID   uint64
}

type datasetReader interface {
	FindDataSets(context.Context, *storage.FindDataSetsOptions) ([]*storage.DataSetInfo, error)
	GetStorageInfo(context.Context, *storage.GetStorageInfoOptions) (*storage.StorageInfo, error)
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
	client, err := exampleutil.NewClient(ctx, env)
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
	return runList(ctx, cfg, client.Storage(), stdout)
}

func parseConfig(args []string) (listConfig, error) {
	cfg := listConfig{}
	fs := flag.NewFlagSet("list-datasets", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&cfg.OnlyManaged, "managed", false, "show only FWSS-managed datasets")
	fs.Uint64Var(&cfg.DataSetID, "dataset-id", 0, "optional dataset ID to highlight")
	if err := fs.Parse(args); err != nil {
		return listConfig{}, err
	}
	if fs.NArg() != 0 {
		return listConfig{}, fmt.Errorf("usage: go run ./examples/list-datasets [--managed] [--dataset-id id]")
	}
	return cfg, nil
}

func runList(ctx context.Context, cfg listConfig, reader datasetReader, stdout io.Writer) error {
	info, err := reader.GetStorageInfo(ctx, nil)
	if err != nil {
		return fmt.Errorf("get storage info: %w", err)
	}
	if err := printStorageInfo(stdout, info); err != nil {
		return err
	}

	dataSets, err := reader.FindDataSets(ctx, &storage.FindDataSetsOptions{OnlyManaged: cfg.OnlyManaged})
	if err != nil {
		return fmt.Errorf("find datasets: %w", err)
	}
	if cfg.DataSetID != 0 {
		dataSets = filterDataSets(dataSets, types.DataSetID(cfg.DataSetID))
	}
	if err := exampleutil.WriteKV(stdout, "datasetCount", len(dataSets)); err != nil {
		return err
	}
	for i, dataSet := range dataSets {
		if err := printDataSet(stdout, i+1, dataSet); err != nil {
			return err
		}
	}
	return nil
}

func printStorageInfo(stdout io.Writer, info *storage.StorageInfo) error {
	if info == nil {
		return fmt.Errorf("storage info is nil")
	}
	if err := exampleutil.WriteKV(stdout, "providerCount", len(info.Providers)); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "pricing.noCDN.perMonth", info.Pricing.NoCDN.PerMonth); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "pricing.withCDN.perMonth", info.Pricing.WithCDN.PerMonth); err != nil {
		return err
	}
	if info.Allowances == nil {
		return exampleutil.WriteKV(stdout, "allowances", "<none>")
	}
	if err := exampleutil.WriteKV(stdout, "allowances.approved", info.Allowances.IsApproved); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "allowances.rateAllowance", info.Allowances.RateAllowance); err != nil {
		return err
	}
	return exampleutil.WriteKV(stdout, "allowances.lockupAllowance", info.Allowances.LockupAllowance)
}

func printDataSet(stdout io.Writer, index int, dataSet *storage.DataSetInfo) error {
	prefix := fmt.Sprintf("dataset.%d", index)
	if dataSet == nil || dataSet.DataSetInfo == nil {
		return exampleutil.WriteKV(stdout, prefix, "<nil>")
	}
	if err := exampleutil.WriteKV(stdout, prefix+".dataSetID", dataSet.DataSetID); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".providerID", dataSet.ProviderID); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".payer", dataSet.Payer.Hex()); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".isLive", dataSet.IsLive); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".isManaged", dataSet.IsManaged); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".withCDN", dataSet.WithCDN); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, prefix+".activePieceCount", dataSet.ActivePieceCount); err != nil {
		return err
	}
	return exampleutil.WriteMap(stdout, prefix+".metadata", dataSet.Metadata)
}

func filterDataSets(dataSets []*storage.DataSetInfo, id types.DataSetID) []*storage.DataSetInfo {
	out := make([]*storage.DataSetInfo, 0, len(dataSets))
	for _, dataSet := range dataSets {
		if dataSet == nil || dataSet.DataSetInfo == nil {
			continue
		}
		if dataSet.DataSetID == id {
			out = append(out, dataSet)
		}
	}
	return out
}
