# Getting Started

This guide is for applications using `synapse.Client`. Use the
[API reference](https://pkg.go.dev/github.com/strahe/synapse-go) for complete
symbols and [examples](../examples/) for runnable CLI flows.

## Install

```bash
go get github.com/strahe/synapse-go
```

Requires Go 1.25+.

## Client

Create a client with a private key, RPC endpoint, and source name.

```go
client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex("0x..."),
    synapse.WithRPCURL("https://api.calibration.node.glif.io/rpc/v1"),
    synapse.WithSource("my-app"),
)
if err != nil {
    return err
}
defer client.Close()
```

Use real values from your config or secret manager. Never hardcode production
private keys.

Mainnet and Calibration are supported. The client detects the chain from the
RPC endpoint unless you pass `WithChain`.

Common setup options:

- `WithPrivateKeyHex` / `WithPrivateKey`: configure the signer.
- `WithRPCURL` / `WithEthClient`: configure chain access.
- `WithSource`: namespace datasets for this application.
- `WithCDN`: set the client default for CDN-backed storage.
- `WithAllowPrivateNetworks`: opt into private-network URL downloads.
- `Close`: release SDK-owned network clients.

## Upload And Download

`Storage().Upload` is the default path. It selects providers, stores the primary
copy, asks secondary providers to pull from it, then commits successful copies
on-chain. Payloads must be at least 127 bytes and no larger than
`chain.MaxUploadSize`, the PDP cap of about 1 GiB.

```go
withCDN := true

result, err := client.Storage().Upload(ctx, file, &storage.UploadOptions{
    Copies:  2,
    WithCDN: &withCDN,
    DataSetMetadata: map[string]string{
        "app": "my-app",
    },
    PieceMetadata: map[string]string{
        "name": "payload.bin",
    },
})
if err != nil {
    return err
}

if !result.Complete {
    log.Printf("partial upload: %d/%d copies", result.SuccessCount(), result.RequestedCopies)
}

fmt.Println("piece:", result.PieceCID)
for _, copy := range result.Copies {
    fmt.Println("provider:", copy.ProviderID)
    fmt.Println("dataset:", copy.DataSetID)
    fmt.Println("piece id:", copy.PieceID)
    fmt.Println("retrieve:", copy.RetrievalURL)
}
```

Download from a retrieval URL returned by upload:

```go
reader, err := client.Storage().Download(ctx, result.PieceCID, &storage.DownloadOptions{
    URL: result.Copies[0].RetrievalURL,
})
if err != nil {
    return err
}
defer reader.Close()

data, err := io.ReadAll(reader)
if err != nil {
    return err
}
fmt.Println("downloaded bytes:", len(data))
```

The download reader validates the PieceCID at EOF. Always check the final
`Read` or `io.ReadAll` error.

Use `DownloadOptions{Context: storageCtx}` or `storageCtx.Download` when you
want to read from a specific provider context. URL downloads reject private
network addresses by default; enable `WithAllowPrivateNetworks(true)` only for
trusted infrastructure. The top-level client leaves URL downloads uncapped.
Standalone `storage.Service` users can set `storage.Options.DownloadMaxBytes`;
exceeding it returns `storage.ErrMaxBytesExceeded`.

## Upload Controls

Use `storage.UploadOptions` when the default upload is not enough:

- `Copies`: requested provider copies. Zero means the resolver default.
- `ProviderIDs`: pin copies to specific providers.
- `DataSetIDs`: write to specific existing datasets.
- `ExcludeProviderIDs`: skip providers during automatic selection.
- `DataSetMetadata`: metadata used when creating or reusing datasets.
- `PieceMetadata`: metadata stored with the committed piece.
- `WithCDN`: per-upload CDN override. `nil` inherits the client default.
- `PieceCID`: precomputed PieceCIDv2 when you already calculated it.
- `OnProgress`, `OnStored`, `OnCopyComplete`, `OnCopyFailed`,
  `OnPullProgress`, `OnPiecesAdded`, `OnPiecesConfirmed`: lifecycle callbacks.

High-level upload callbacks are isolated from the upload flow: a callback panic
does not interrupt the upload, and a configured logger records a warning.

`Upload` succeeds when at least one copy commits on-chain. Before returning,
it waits for started commit attempts to settle. Check `UploadResult.Complete`
to know whether every requested copy succeeded.

Dataset metadata must match exactly for automatic dataset reuse. Use stable
metadata values when you want uploads to share payment rails.

## Funding Preflight

`Prepare` is optional. Use it before a first upload, before a large batch, or
when your UI needs to show whether the account has enough USDFC deposit and
FWSS approval. If `Transaction` is nil, the account is already ready for the
requested size and contexts. Match options that affect context selection, such
as CDN.

```go
withCDN := true

prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{
    DataSize:  uint64(payloadSize),
    EnableCDN: &withCDN,
})
if err != nil {
    return err
}

if prep.Transaction != nil {
    tx, err := prep.Transaction.Execute(ctx, payments.WithWait(10*time.Minute))
    if err != nil {
        return err
    }
    fmt.Println("prepare tx:", tx.Hash)
}
```

If you already selected contexts, pass them through `PrepareOptions.Contexts`
so the estimate matches the exact providers and datasets.

For read-only cost and account state, use `GetStorageInfo` or
`CalculateMultiContextCosts`.

## Contexts And Datasets

Use contexts when you need provider or dataset control before uploading.

```go
contexts, err := client.Storage().CreateContexts(ctx, &storage.CreateContextsOptions{
    Copies: 2,
    DataSetMetadata: map[string]string{
        "project": "photos",
    },
})
if err != nil {
    return err
}

fmt.Println("contexts:", len(contexts))
```

For one provider or one dataset:

```go
ctx1, err := client.Storage().CreateContext(ctx, &storage.CreateContextOptions{
    ProviderIDs: []types.BigInt{providerID},
})
if err != nil {
    return err
}

result, err := ctx1.Upload(ctx, file, &storage.UploadOptions{
    PieceMetadata: map[string]string{"name": "payload.bin"},
})
if err != nil {
    return err
}
fmt.Println(result.PieceCID)
```

To create an empty dataset first, persist the submission if your process may
restart before confirmation:

```go
var submitted storage.CreateDataSetSubmission

created, err := ctx1.CreateDataSet(ctx, &storage.CreateDataSetOptions{
    OnSubmitted: func(s storage.CreateDataSetSubmission) {
        submitted = s
    },
})
if err != nil {
    return err
}
fmt.Println("dataset:", created.DataSetID)
```

Resume a submitted create transaction:

```go
created, err := ctx1.WaitForDataSetCreated(ctx, submitted)
if err != nil {
    return err
}
fmt.Println("dataset:", created.DataSetID)
```

Use `GetDefaultContext` when the resolver defaults are enough. Advanced callers
can split a context upload into `Store`, `Pull`, `PresignForCommit`, and
`Commit`.

## Discovery And Lifecycle

Common management calls:

- `FindDataSets`: list datasets owned by the signer or another payer.
- `GetStorageInfo`: inspect providers, pricing, limits, and allowances.
- `Context.Download`: download from a known provider and dataset context.
- `Context.DeletePieceByID`: schedule exact removal by on-chain piece ID.
- `Context.DeletePiece`: schedule removal by piece CID convenience lookup. Prefer
  `DeletePieceByID` when available, because repeated uploads can share a CID.
- `Context.Terminate` / `Service.TerminateDataSet`: terminate an FWSS dataset.

Termination and removal are storage lifecycle actions. Treat them as
application-level destructive operations and gate them accordingly.

## Services

`synapse.Client` exposes these service entry points:

| Service | Use it for |
|---------|------------|
| `Storage()` | Upload, download, prepare, contexts, datasets |
| `Payments()` | USDFC balances, deposits, withdrawals, approvals, rails |
| `Costs()` | Storage estimates and account runway |
| `WarmStorage()` | FWSS dataset metadata, pricing, approved-provider discovery, termination |
| `SPRegistry()` | Provider discovery and PDP capability lookup |
| `FilBeam()` | CDN quota and dataset usage |
| `SessionKey()` | Delegated session key authorization |

For local PieceCID work:

```go
info, err := piece.Calculate(file)
if err != nil {
    return err
}
fmt.Println(info.CIDv2)
```

Advanced note: the top-level `pdp` package is a provider HTTP API client. It
does not create EIP-712 signatures. Most applications should use
`synapse.Client` and `storage`.

## Runnable Examples

The programs under [examples](../examples/) cover upload, download, provider
discovery, dataset listing, and local PieceCID inspection. CLI example
variables are listed in [examples/README.md](../examples/README.md).
