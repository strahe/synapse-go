# Getting Started

This guide covers common tasks with the root `synapse.Client`. Full behavior
contracts, standalone service wiring, and edge cases are in the
[API reference](https://pkg.go.dev/github.com/strahe/synapse-go).

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

Load the key from your config or secret manager; never hardcode production
keys. Mainnet and Calibration are detected from the RPC endpoint unless you
pass `WithChain`. `WithSource` namespaces the data sets your application
creates.

Other common options:

- `WithCDN`: default CDN setting for new uploads.
- `WithUploadBatching` / `WithoutUploadBatching`: see
  [Commit Batching](#commit-batching).
- `WithStorageSigner`: sign storage requests with a delegated
  `signer.StorageSigner`, such as a KMS or remote signer.
- `WithAllowPrivateNetworks(true)`: allow provider, FilBeam, and URL requests
  to private or reserved addresses. They are rejected by default with an error
  matching `synapse.ErrPrivateNetwork`; enable this only for trusted
  infrastructure.

With `WithStorageSigner`, authorize the signer's address with
`client.SessionKey().Login(...)` before the first storage write. The root
private key still pays and signs payment, approval, and direct termination
transactions. The signer receives only a 32-byte digest, so use a dedicated key
and do not expose its signing endpoint as a general signing service.

## Upload And Download

`Storage().Upload` selects providers, stores the primary copy, has secondary
providers pull it, and commits each copy on-chain. Payloads must be between 127
bytes and `chain.MaxUploadSize` (about 1 GiB).

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
    fmt.Println("provider:", copy.ProviderID, "dataset:", copy.DataSetID, "piece id:", copy.PieceID)
    fmt.Println("retrieve:", copy.RetrievalURL)
}
```

`Upload` succeeds when at least one copy is confirmed; check `Complete` to see
whether every requested copy succeeded.

Upload uses an existing data set when its `DataSetMetadata` matches exactly;
otherwise it creates one, which costs a creation fee and a lifecycle reserve.
Keep metadata values stable so uploads share data sets. `PieceMetadata` is
emitted in the FWSS `PieceAdded` event and is not stored in contract state.
See [`UploadOptions`](https://pkg.go.dev/github.com/strahe/synapse-go/storage#UploadOptions)
for exclusions and progress callbacks.

Callbacks from different providers can overlap. Protect shared state when
updating it; for example, record completed secondary copies with a mutex.
This example also uses `sync`, `github.com/ipfs/go-cid`, and
`github.com/strahe/synapse-go/types`:

```go
var mu sync.Mutex
copiedProviders := make(map[string]bool)

result, err := client.Storage().Upload(ctx, bytes.NewReader(data), &storage.UploadOptions{
    Copies: 3,
    OnCopyComplete: func(providerID types.BigInt, _ cid.Cid) {
        mu.Lock()
        copiedProviders[providerID.String()] = true
        mu.Unlock()
    },
})
if err != nil {
    return err
}
fmt.Println("all copies confirmed:", result.Complete)
```

Use the same mutex if another goroutine reads the map. `OnCopyComplete` reports
pull completion; use the upload result to check on-chain confirmation.

By default the primary copy goes to an endorsed provider. If none is available,
`Upload` returns an error matching `storage.ErrNoEndorsedProvider`. Set
`AllowUnendorsedPrimary: true` to choose the primary from all approved
providers.

Download with a retrieval URL from the upload result:

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

The reader verifies the PieceCID when it reaches EOF, so always check the final
`Read` or `io.ReadAll` error.

To download from one of your data sets, set `DataSetID` instead of `URL`. If
the data set has CDN enabled, the SDK tries CDN first, which works even when
its provider is no longer active. Otherwise it downloads from the provider if
the provider is still active:

```go
dataSetID := result.Copies[0].DataSetID
reader, err := client.Storage().Download(ctx, result.PieceCID, &storage.DownloadOptions{
    DataSetID: &dataSetID,
})
```

### Commit Batching

The root client batches commits from concurrent uploads to the same target.
A batch is submitted when no other upload to that target is in progress and 3
seconds pass without a new piece. Change this with `WithUploadBatching`:

```go
client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex("0x..."),
    synapse.WithRPCURL("https://api.calibration.node.glif.io/rpc/v1"),
    synapse.WithUploadBatching(
        storage.WithUploadIdleWait(time.Second),
        storage.WithUploadMaxWait(15*time.Second),
    ),
)
```

Pass `storage.WithoutUploadIdleWait()` and `storage.WithUploadMaxWait(0)` to
submit every piece immediately, or use `synapse.WithoutUploadBatching()` to
commit each upload on its own.

Once a piece joins a batch, canceling its `Upload` context stops only that
call's wait; the piece may still be committed. Check provider or chain state
before retrying a canceled or timed-out upload.

`Close` aborts pending batches. Call `client.Storage().Flush(ctx)` first to
submit and confirm them.

## Funding Preflight

`Prepare` checks whether the account has enough USDFC deposit and FWSS
approval, and returns a transaction that adds what is missing. To make the
estimate match the upload, select targets first and pass the same contexts to
`Prepare` and `UploadToContexts`:

```go
selection, err := client.Storage().SelectUploadContexts(ctx,
    storage.SelectUploadContextsOptions{
        Copies:          2,
        DataSetMetadata: map[string]string{"app": "my-app"},
    },
)
if err != nil && !errors.Is(err, storage.ErrInsufficientUploadContexts) {
    return err
}

prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{
    PieceSizes: []uint64{uint64(payloadSize)},
    Contexts:   selection.Contexts,
})
if err != nil {
    return err
}
if prep.Transaction != nil {
    if _, err := prep.Transaction.Execute(ctx, payments.WithWait(10*time.Minute)); err != nil {
        return err
    }
}

result, err := client.Storage().UploadToContexts(ctx, file, selection.Contexts, nil)
```

When fewer providers are available than requested, selection returns the
available contexts together with `storage.ErrInsufficientUploadContexts`; you
can continue with them or stop. `UploadToContexts` never replaces a failed
provider.

List each piece's raw size in `PieceSizes`. `DepositNeeded` already includes
the lifecycle reserve for new data sets. Fee estimates assume each piece is
committed separately, so actual fees can be lower. For read-only estimates,
use `CalculateMultiContextCosts` or `GetStorageInfo`.

## Contexts And Data Sets

Use a context when you need a specific target:

- `ProviderContext`: one provider, no data set. `CreateDataSet` and
  `CreateAndAdd` create a new data set.
- `DataSetContext`: one existing data set. `Upload` and `Commit` add pieces to
  it.

```go
dataSetCtx, err := client.Storage().NewDataSetContext(ctx, dataSetID, storage.NewDataSetContextOptions{})
if err != nil {
    return err
}

result, err := dataSetCtx.Upload(ctx, file, &storage.ContextUploadOptions{
    PieceMetadata: map[string]string{"name": "payload.bin"},
})
```

`NewDataSetContext` opens only data sets owned by your account. Use
`NewProviderContext` to open a provider by ID, or `SelectProviderContext` to
pick a healthy approved one. Contexts never change target; use
`providerCtx.ForDataSet(ref)` to get a `DataSetContext` for a created data set.

Low-level `CreateAndAdd`, `Commit`, and `Pull` requests are sent as one
request and are not split. A request must fit 65,248 bytes of encoded
calldata, including metadata and signatures; otherwise it fails with
`pdp.ErrAddPiecesMessageTooLarge`. Data sets created before compact piece
storage also accept at most 80 pieces per request (`pdp.ErrTooManyPieces`).
High-level `Upload` sizes batches for you.

### Recovering Submissions

To resume confirmation after a restart, submit and wait in separate steps and
persist the status URL in between:

```go
submitted, err := providerCtx.SubmitCreateAndAdd(ctx, storage.CreateAndAddRequest{
    Pieces: pieces,
})
if err != nil {
    return err
}
// Persist submitted.ProviderID, submitted.StatusURL, and
// *submitted.ClientDataSetID before waiting.
result, err := providerCtx.WaitForCreateAndAdd(ctx, submitted.StatusURL, *submitted.ClientDataSetID)
```

After a restart, open the provider with `NewProviderContext` and call
`WaitForCreateAndAdd` with the saved values. For an existing data set, use
`DataSetContext.SubmitCommit` and `WaitForCommit` and persist the data set ID
with the status URL. A failed high-level upload reports the same values in
`FailedAttempt.Submission`. See the
[storage package docs](https://pkg.go.dev/github.com/strahe/synapse-go/storage#hdr-Submission_recovery)
for recovery when the provider response itself is lost.

## Discovery And Lifecycle

- `FindDataSets`: list data sets owned by your account or another payer.
- `GetStorageInfo`: providers, pricing, limits, and allowances.
- `DataSetContext.DeletePiecesByID`: schedule removal by on-chain piece ID.
  `DeletePieces` looks pieces up by CID; prefer IDs, because repeated uploads
  can share a CID.
- `TerminateService`: end storage service for a data set.

A deletion request is sent as one transaction and is not split or retried.
Removal is scheduled; pieces are removed later in the proof lifecycle. Treat
deletion and termination as destructive operations in your application.

### Terminating A Service

Termination is relayed by the provider by default and requires your payment
account to be fully settled. To submit from your own wallet without the
provider, set `SkipProvider`:

```go
termination, err := client.Storage().TerminateService(ctx, dataSetID, &storage.TerminateServiceOptions{
    SkipProvider:      true,
    DirectWaitTimeout: 3 * time.Minute,
})
if err != nil {
    return err
}
fmt.Println("service ends at epoch:", termination.EndEpoch)
```

Relayed termination ends service immediately. Direct termination keeps service
and payments active until `EndEpoch`. Neither deletes the data. A direct wait
can time out after the transaction was broadcast, so record the hash from
`OnSubmitted` and do not resubmit blindly.

## Services

`synapse.Client` exposes these service entry points:

| Service | Use it for |
|---------|------------|
| `Storage()` | Upload, download, prepare, contexts, data sets |
| `Payments()` | USDFC balances, account summary, deposits, withdrawals, approvals, rails |
| `Costs()` | Upload cost, lockup, and deposit estimates |
| `WarmStorage()` | FWSS data set metadata, pricing, approved-provider discovery, termination |
| `SPRegistry()` | Provider discovery and PDP capability lookup |
| `FilBeam()` | CDN quota and data set usage |
| `SessionKey()` | Delegated session key authorization |

To calculate a PieceCID locally, use `piece.Calculate(file)`.

## Runnable Examples

The programs under [examples](../examples/) cover upload, download, provider
discovery, data set listing, and local PieceCID inspection. CLI example
variables are listed in [examples/README.md](../examples/README.md).
