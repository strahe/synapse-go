# Getting Started

This guide is for applications using `synapse.Client`. Use the
[API reference](https://pkg.go.dev/github.com/strahe/synapse-go) for complete
symbols and [examples](../examples/) for runnable CLI flows.

## Install

```bash
go get github.com/strahe/synapse-go
```

Requires Go 1.26.3+.

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

- `WithPrivateKeyHex` / `WithPrivateKey`: configure the root payer and
  transaction signer.
- `WithStorageSigner`: delegate Storage EIP-712 signing to an authorized
  `*signer.Secp256k1Signer`. Call `client.SessionKey().Login(...)` or
  `LoginWithOptions(...)` for its address before the first storage write;
  client construction does not check authorization.
- `WithRPCURL` / `WithEthClient`: configure chain access.
- `WithMaxMulticallCalls`: limit dynamic Multicall3 requests.
- `WithSource`: namespace datasets for this application.
- `WithCDN`: set the client default for CDN-backed storage.
- `WithAllowPrivateNetworks`: opt into private-network URL downloads.
- `Close`: release SDK-owned network clients.

`WithStorageSigner` does not change `Client.Address()` or the payer. Payments,
operator approvals, nonce management, and direct storage termination continue
to use the root private key. Direct termination includes `TerminateDataSet` and
`TerminateService` with `SkipProvider` enabled.

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
want to read from a specific provider context. Context downloads stop at the
raw payload size encoded in PieceCIDv2; a larger response returns
`storage.ErrMaxBytesExceeded`. URL downloads reject private network addresses
by default; enable `WithAllowPrivateNetworks(true)` only for trusted
infrastructure. The top-level client leaves URL downloads uncapped. Standalone
`storage.Service` users can set `storage.Options.DownloadMaxBytes`; exceeding
it returns the same error.

## Upload Controls

`Service.Upload` performs automatic target selection. Its options include:

- `Copies`: required number of provider copies. It must be greater than zero.
- `ExcludeProviderIDs`: skip providers only during automatic selection.
- `AllowUnendorsedPrimary`: the zero value requires an endorsed primary;
  `true` selects the primary from the full approved pool and skips the
  endorsement query.
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

The root client configures strict endorsed-primary selection by default. The
primary must be endorsed, approved, active, and healthy; secondary copies use
the full approved, active, healthy pool. This policy can concentrate primary
traffic among a smaller provider set. Applications that deliberately accept
any approved provider as primary can opt out for one upload:

```go
result, err := client.Storage().Upload(ctx, file, &storage.UploadOptions{
	Copies:                   2,
	AllowUnendorsedPrimary: true,
})
```

The default health check requires `/pdp/ping` to return HTTP 2xx with a body
that trims exactly to `curio-pdp`. Providers therefore need Curio v1.28.3 or
later; older empty-body ping responses are intentionally rejected.

An empty endorsement set or no eligible healthy endorsed provider returns
`storage.ErrNoEndorsedProvider`; endorsement query failures are returned as
query errors rather than treated as an empty set.

Use `NewProviderContext` or `NewDataSetContext` for a known target. Use
`UploadToContexts` when the caller, rather than the SDK, must determine the
exact providers and their primary-to-secondary order.

## Funding Preflight

`Prepare` is optional. Use it before a first upload, before a large batch, or
when your UI needs to show whether the account has enough USDFC deposit and
FWSS approval. Select the upload targets first, then pass the same contexts to
`Prepare` and `UploadToContexts`. This ensures the estimate and upload use the
same providers, datasets, payer, chain, and record keeper.

```go
withCDN := true

selection, selectErr := client.Storage().SelectUploadContexts(ctx,
    storage.SelectUploadContextsOptions{
        Copies:  2,
        WithCDN: &withCDN,
        DataSetMetadata: map[string]string{
            "project": "photos",
        },
    },
)
if selectErr != nil && !errors.Is(selectErr, storage.ErrInsufficientUploadContexts) {
    return selectErr
}
if selection == nil {
    return errors.New("no upload contexts available")
}

prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{
    DataSize: uint64(payloadSize),
    Contexts: selection.Contexts,
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

result, err := client.Storage().UploadToContexts(
    ctx,
    file,
    selection.Contexts,
    &storage.UploadOptions{
        PieceMetadata: map[string]string{"name": "payload.bin"},
    },
)
if err != nil {
    return err
}
```

When selection finds at least one but fewer than the requested targets, it
returns both a usable `UploadContextSelection` and an
`InsufficientUploadContextsError`. The application can continue with the
available contexts or stop before funding. With `UploadToContexts`, the
selection length becomes `UploadResult.RequestedCopies` and no replacement
provider is selected automatically.

For read-only cost and account state, use `GetStorageInfo` or
`CalculateMultiContextCosts`.

## Contexts And Datasets

There are two immutable context types:

- `ProviderContext` identifies one provider and no dataset. `Commit` and
  `Pull` create a new dataset.
- `DataSetContext` identifies one provider and one existing dataset. `Commit`
  and `Pull` always target that dataset.

Provider-scoped methods such as `Store` and `Download` are shared. For example,
both `ProviderContext.Download` and `DataSetContext.Download` retrieve a piece
from the same configured provider or CDN; the dataset binding does not change
piece retrieval. Dataset inspection, deletion, and termination methods exist
only on `DataSetContext`.

Select one approved, active, healthy provider without looking up datasets:

```go
providerCtx, err := client.Storage().SelectProviderContext(ctx,
    storage.SelectProviderContextOptions{
        DataSetMetadata: map[string]string{
            "project": "photos",
        },
    },
)
if err != nil {
    return err
}
```

Open a registered provider by ID without checking approval, activity, endpoint
health, or existing datasets:

```go
providerID := types.NewBigInt(123)
providerCtx, err := client.Storage().NewProviderContext(ctx, providerID,
    storage.NewProviderContextOptions{
        DataSetMetadata: map[string]string{"project": "photos"},
    },
)
if err != nil {
    return err
}
```

Open an existing dataset owned by the current payer. The optional provider ID
is an ownership assertion. Opening a terminated or currently unwritable
dataset is allowed for inspection and cleanup; a later `Commit` or `Upload`
still checks writability.

```go
providerID := types.NewBigInt(123)
dataSetID := types.NewBigInt(456)
dataSetCtx, err := client.Storage().NewDataSetContext(ctx, dataSetID,
    storage.NewDataSetContextOptions{ProviderID: &providerID},
)
if err != nil {
    return err
}

result, err := dataSetCtx.Upload(ctx, file, &storage.UploadOptions{
    PieceMetadata: map[string]string{"name": "payload.bin"},
})
if err != nil {
    return err
}
fmt.Println(result.PieceCID)
```

`DataSetRef` is the persistent reference for a complete provider and dataset
target. Its zero value is invalid; construct it explicitly and use accessors to
read IDs.

```go
ref, err := storage.NewDataSetRef(providerID, dataSetID, dataSetCtx.ClientDataSetID())
if err != nil {
    return err
}
fmt.Println("dataset:", ref.DataSetID())
```

To create an empty dataset first, persist the submission if the process may
restart before confirmation. Creation is available only on `ProviderContext`.

```go
var submitted storage.CreateDataSetSubmission

created, err := providerCtx.CreateDataSet(ctx, &storage.CreateDataSetOptions{
    OnSubmitted: func(s storage.CreateDataSetSubmission) {
        submitted = s
    },
})
if err != nil {
    return err
}
fmt.Println("dataset:", created.DataSet.DataSetID())
```

Resume a submitted create transaction with any fresh `ProviderContext` for the
same provider, then convert the returned reference without mutating that
context:

```go
created, err := providerCtx.WaitForDataSetCreated(ctx, submitted)
if err != nil {
    return err
}
dataSetCtx, err := providerCtx.ForDataSet(created.DataSet)
if err != nil {
    return err
}
fmt.Println("dataset:", dataSetCtx.DataSetID())
```

The receiver never binds or changes target after creation. Concurrent creates
on one `ProviderContext` are independent; adds on one `DataSetContext` may run
in parallel. Advanced callers can split a context upload into `Store`, `Pull`,
`PresignForCommit`, and `Commit`.

### Migrating From The Previous Context API

| Previous call | Replacement |
|---------------|-------------|
| `CreateContext(nil)` / `GetDefaultContext()` | `SelectProviderContext(...)` |
| `CreateContext` with `ProviderID` | `NewProviderContext(...)` |
| `CreateContext` with `DataSetID` | `NewDataSetContext(...)` |
| `CreateContexts` for a new upload | `SelectUploadContexts(...)` |
| `Upload` with provider or dataset IDs | construct/select contexts, then call `UploadToContexts(...)` |
| `Prepare` without contexts | select contexts first and pass the same slice to `Prepare` |

## Discovery And Lifecycle

Common management calls:

- `FindDataSets`: list datasets owned by the signer or another payer.
- `GetStorageInfo`: inspect providers, pricing, limits, and allowances.
- `ProviderContext.Download` / `DataSetContext.Download`: download from a known provider.
- `DataSetContext.DeletePieceByID`: schedule exact removal by on-chain piece ID.
- `DataSetContext.DeletePiece`: schedule removal by piece CID convenience lookup. Prefer
  `DeletePieceByID` when available, because repeated uploads can share a CID.
- `DataSetContext.TerminateService` / `Service.TerminateService`: terminate service
  through the provider by default; use `SkipProvider` for direct FWSS fallback.
- `DataSetContext.Terminate` / `Service.TerminateDataSet`: legacy direct FWSS
  termination write.

Termination and removal are storage lifecycle actions. Treat them as
application-level destructive operations and gate them accordingly.

## Services

`synapse.Client` exposes these service entry points:

| Service | Use it for |
|---------|------------|
| `Storage()` | Upload, download, prepare, contexts, datasets |
| `Payments()` | USDFC balances, account summary, deposits, withdrawals, approvals, rails |
| `Costs()` | Storage estimates; legacy account runway compatibility |
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
