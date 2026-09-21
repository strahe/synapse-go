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
  `signer.StorageSigner`. This can be a `*signer.Secp256k1Signer`, a KMS/HSM
  integration, a remote signer, or a decorator. Call
  `client.SessionKey().Login(...)` or `LoginWithOptions(...)` for its address
  before the first storage write; client construction does not check
  authorization.
- `WithRPCURL` / `WithEthClient`: configure chain access.
- `WithMaxMulticallCalls`: limit dynamic Multicall3 requests.
- `WithSource`: namespace datasets for this application.
- `WithCDN`: set the client default for CDN-backed storage.
- `WithUploadBatching`: configure the default high-level upload commit
  coordinator.
- `WithoutUploadBatching`: restore immediate, independent commits for each
  high-level upload.
- `WithAllowPrivateNetworks`: opt into private-network access for provider
  PDP, FilBeam, and URL downloads.
- `Close`: abort unflushed upload batches and release SDK-owned network
  clients. Call `Storage().Flush(ctx)` first for a graceful drain.

By default, provider PDP, FilBeam, and URL requests reject private or reserved
destinations; match rejections with
`errors.Is(err, synapse.ErrPrivateNetwork)`. Use
`WithAllowPrivateNetworks(true)` only for trusted private infrastructure. These
safeguards do not apply to Ethereum JSON-RPC configured with `WithRPCURL` or
`WithEthClient`; a custom `WithHTTPClient` replaces them.

`WithStorageSigner` does not change `Client.Address()` or the payer. Payments,
operator approvals, nonce management, and direct storage termination continue
to use the root private key. Direct termination uses `Storage().TerminateService`
with `SkipProvider` enabled or `WarmStorage().TerminateDataSet`.

A custom Storage signer receives a pre-computed 32-byte digest and cannot
inspect the original EIP-712 message. Use a dedicated authorization key and
restrict the signing endpoint to trusted callers; do not expose it as a general
signing service.

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
`storage.ErrMaxBytesExceeded`. The top-level client leaves URL downloads
uncapped. Standalone `storage.Service` users can set
`storage.Options.DownloadMaxBytes`; exceeding it returns the same error.

## Upload Controls

`Service.Upload` performs automatic target selection. Its options include:

- `Copies`: required number of provider copies. It must be greater than zero.
- `ExcludeProviderIDs`: skip providers only during automatic selection.
- `AllowUnendorsedPrimary`: the zero value requires an endorsed primary;
  `true` selects the primary from the full approved pool and skips the
  endorsement query.
- `DataSetMetadata`: metadata used when creating or reusing datasets.
- `PieceMetadata`: metadata validated and emitted in the FWSS `PieceAdded`
  event. Read it from contract events or an indexer; FWSS does not persist it
  in contract state.
- `WithCDN`: per-upload CDN override. `nil` inherits the client default.
- `PieceCID`: precomputed PieceCIDv2 when you already calculated it.
- `OnProgress`, `OnStored`, `OnCopyComplete`, `OnCopyFailed`,
  `OnPullProgress`, `OnPiecesAdded`, `OnPiecesConfirmed`: lifecycle callbacks.

Callbacks may run on internal goroutines. If a callback panics, the upload
stops as if its context were canceled, later callbacks are suppressed (ones
already running are not interrupted), and `Upload` re-panics with the same
value on your goroutine, where your own `recover` can handle it. A configured
logger records the original stack first. A piece already handed to the commit
batcher may still be committed.

`Upload` succeeds when at least one copy is confirmed on-chain. If you do not
cancel the call, it waits for every commit it started. Check
`UploadResult.Complete` to see whether every requested copy succeeded. Canceling
after a piece has entered a batch is covered in
[Commit Batching](#commit-batching).

### Commit Batching

The root client batches compatible high-level upload commits by default.
Ready pieces are submitted together once no other upload to the same target
is in progress and 3 seconds pass without a new piece. There is no time limit
by default, so a slow upload delays the others for that target. At most four
batches are signed and submitted concurrently; provider confirmation waits
continue outside that limit.

The wait policy is explicit:

- `WithUploadIdleWait(d)` changes the 3-second delay.
- `WithUploadMaxWait(d)` submits a batch at most `d` after its first piece is
  ready, even while other uploads are still running.
- `WithoutUploadIdleWait()` with `WithUploadMaxWait(0)` submits every piece
  immediately.
- `WithoutUploadIdleWait()` alone creates Flush-only batches. They still
  submit at the 40-piece limit, the provider message-size limit, or a
  repeated piece CID.
- `WithoutUploadBatching()` bypasses the coordinator and preserves one commit
  per high-level upload.

Options of the same kind use the last value. Negative waits, an effective idle
wait greater than the effective maximum, or a submission limit below one make
`synapse.New` return an error matching `storage.ErrInvalidArgument`.

For example, use a custom timed window:

```go
client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex("0x..."),
    synapse.WithRPCURL("https://api.calibration.node.glif.io/rpc/v1"),
    synapse.WithUploadBatching(
        storage.WithUploadIdleWait(time.Second),
        storage.WithUploadMaxWait(15*time.Second), // optional hard limit
        storage.WithUploadMaxConcurrentSubmissions(2),
    ),
)
```

The caller context controls target resolution, Store, Pull, validation, and
admission to the coordinator. Admission transfers ownership of that piece to
the batcher. After that point, canceling the caller context stops that caller's
wait and suppresses later callbacks, but it does not remove the piece or stop
signing, submission, or confirmation. `Upload` may therefore return
`context.Canceled` or `context.DeadlineExceeded` while the accepted work later
creates a data set or commits on-chain. Reconcile provider or chain state
before retrying; a timeout alone does not prove that a retry is safe. If a
terminal result was already published when cancellation races, `Upload`
returns that result.

`client.Storage().Flush(ctx)` establishes a barrier at the call: it waits for
uploads already in progress to finish Store and Pull, submits their windows,
and waits for final provider confirmation. A compatible upload that begins
after the barrier may join the same still-open window, so later uploads are
neither guaranteed to be included nor guaranteed to be excluded. Canceling the
Flush context stops only the caller's wait; shared batches continue and a
later Flush can still report their failures. A completed Flush publishes
terminal results before returning, so an immediate `client.Close()` does not
replace those results with `storage.ErrClosed`. Call Flush before Close for a
graceful drain because Close aborts pending work. Close cannot retract a
transaction that a provider has already accepted.

Flush-only mode is intended for applications that coordinate several uploads
concurrently and call Flush after those uploads have started. Calling a
blocking `Upload` and then Flush sequentially cannot release that Upload's
window. `Service.Flush` is a no-op when a standalone service has no batcher,
and returns an error matching `storage.ErrClosed` after its root client closes.

Existing data sets share a window only when the provider, complete data-set
reference, and exact service URL match. New data sets also require the same
payee, CDN setting, and data-set metadata. Piece metadata remains per piece.
Uploads to the same new target, including sequential `ProviderContext.Upload`
calls, share one data set for the life of the client, so split windows add
transactions rather than data sets. Use different data-set metadata to keep
uploads in separate data sets. A `ProviderContext` remains unbound after the
upload.

In a multi-copy `Service.Upload` or `UploadToContexts` call, the primary and
every secondary participate in batching. Replicas from one call are not
guaranteed to share a transaction. Each caller still receives callbacks and
the PieceID only for its own piece; pieces in one batch report the same
transaction ID.

Low-level context operations remain immediate and are never implicitly batched.
Standalone users opt in by constructing `storage.NewUploadBatcher`, injecting
it through `storage.Options.UploadBatcher` or `storage.WithUploadBatcher`, and
owning its Flush and Close lifecycle.

Dataset metadata must match exactly for automatic dataset reuse. Use stable
metadata values when you want uploads to share payment rails.

Only data sets created after the PDPVerifier 3.5.0 upgrade use compact piece
storage; older data sets are never converted. Because reuse matches metadata,
uploads may keep going to an older data set. To switch, add a new stable
metadata value, or create a data set with `ProviderContext.CreateDataSet` and
upload through `ForDataSet`. Reuse the new data set afterward: each one costs
a creation fee and a lifecycle reserve.

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

Pass each piece's raw payload size in `PieceSizes`; use one element for the
single-file upload below. Piece count is derived from the list. For multiple
pieces, a total size and count cannot reproduce per-piece billing rounding.
Cost estimates conservatively treat every piece as a separate add-pieces
operation because the eventual transaction batches are not known yet. Actual
fees can be lower when pieces are submitted together.

For existing datasets, the root client reads both the PDP leaf count and FWSS
lifecycle reserve state automatically when the resolved contract topology
includes PDPVerifier. If PDPVerifier is not configured, `Prepare` returns
`ErrUninitialized` because the leaf count cannot be read. A standalone storage
service must configure both `DataSetLeafCountReader` and `FWSSDataSetReader`;
without either, `Prepare` returns `ErrUninitialized`. Precomputed `Costs` and
new-dataset contexts do not use these readers.

The result reports operation fees in `Fees`, but does not add them directly to
`DepositNeeded`. FWSS pays those fees from the lifecycle reserve. The required
deposit includes the initial reserve for new datasets in
`Lockup.LifecycleLockup` and any conditional top-up in
`Lockup.ReserveReplenishment`.

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
    PieceSizes: []uint64{uint64(payloadSize)},
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
    &storage.UploadToContextsOptions{
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
`CalculateMultiContextCosts`. An existing `ContextCostRef` must include its
current leaf count, lifecycle reserve balance, pending one-time payments when
non-zero, and a pointer to its PDP end epoch. A non-zero end epoch is rejected
before account or pricing reads.

## Contexts And Datasets

There are two immutable context types:

- `ProviderContext` identifies one provider and no dataset. `CreateAndAdd` and
  `Pull` create a new dataset.
- `DataSetContext` identifies one provider and one existing dataset. `Commit`
  and `Pull` always target that dataset.

`StorageContext` is the sealed mixed-target interface used by selection,
`Prepare`, and `UploadToContexts`. It exposes only capabilities that have the
same meaning for both target kinds. Applications can receive, store, and pass
SDK-created values in `[]StorageContext`, but commit lifecycles and direct
single-context `Upload` remain on the concrete context types.

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

result, err := dataSetCtx.Upload(ctx, file, &storage.ContextUploadOptions{
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

To create an empty dataset first, save the status URL and original client
dataset ID if the process may restart before confirmation. Creation is
available only on `ProviderContext`.

```go
var statusURL string
var clientDataSetID types.BigInt

created, err := providerCtx.CreateDataSet(ctx, &storage.CreateDataSetOptions{
    OnSubmitted: func(s storage.CreateDataSetSubmission) {
        statusURL = s.StatusURL
        clientDataSetID = s.ClientDataSetID
        // Save both values before this callback returns.
    },
})
if err != nil {
    return err
}
fmt.Println("dataset:", created.DataSet.DataSetID())
```

Resume a submitted create transaction with any fresh `ProviderContext` for the
same provider, then convert the returned reference without mutating that
context. Pass the exact client dataset ID used for the original submission;
zero is valid only if that original ID was zero. The status URL alone cannot
recover a lost client dataset ID:

```go
created, err := providerCtx.WaitForDataSetCreated(ctx, statusURL, clientDataSetID)
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
`PresignForCommit`, and either `CreateAndAdd` or `Commit`.

One direct `CreateAndAdd`, `Commit`, or `Pull` request can contain at most 40
pieces and must fit the encoded add-pieces message limit. Oversized requests
return `pdp.ErrAddPiecesMessageTooLarge` before provider submission. Split
direct requests into smaller requests; low-level context methods do not split
them automatically.

### Recovering create-and-add and add-pieces submissions

`CreateAndAddRequest.OnSubmitted` and `CommitRequest.OnSubmitted` receive an
independent `CommitSubmission` after the provider accepts the request and
before confirmation begins. This value contains runtime and diagnostic data;
it is not a persistence schema. Save only the recovery fields needed by the
operation.

Create-and-add requires the status URL and original client dataset ID. Pass
zero only if the original submission used zero; the status URL alone cannot
recover a lost client dataset ID:

```go
var statusURL string
var clientDataSetID types.BigInt
var providerID types.BigInt

result, err := providerCtx.CreateAndAdd(ctx, storage.CreateAndAddRequest{
    Pieces: pieces,
    OnSubmitted: func(s storage.CommitSubmission) {
        statusURL = s.StatusURL
        clientDataSetID = *s.ClientDataSetID
        providerID = s.ProviderID
        // Save these values before this callback returns.
    },
})
if err != nil {
    if statusURL == "" {
        return err
    }
    recoveryCtx, cancel := context.WithTimeout(context.Background(), 3 * time.Minute)
    defer cancel()

    fresh, openErr := client.Storage().NewProviderContext(
        recoveryCtx,
        providerID,
        storage.NewProviderContextOptions{},
    )
    if openErr != nil {
        return openErr
    }
    result, err = fresh.WaitForCreateAndAdd(
        recoveryCtx,
        statusURL,
        clientDataSetID,
    )
}
if err != nil {
    return err
}
fmt.Println("dataset:", result.DataSet.DataSetID())
```

For restart-safe workflows, prefer splitting submission from confirmation so
the application can persist the handle before it starts waiting:

```go
submitted, err := providerCtx.SubmitCreateAndAdd(ctx, storage.CreateAndAddRequest{
    Pieces: pieces,
})
if err != nil {
    return err
}
// Persist submitted.StatusURL and the original
// *submitted.ClientDataSetID before waiting.
result, err := providerCtx.WaitForCreateAndAdd(
    ctx,
    submitted.StatusURL,
    *submitted.ClientDataSetID,
)
```

For an existing dataset, use `DataSetContext.SubmitCommit`,
`GetCommitStatus`, and `WaitForCommit` in the same pattern, persisting the
`DataSetRef` and `submitted.StatusURL`. Use
`ProviderContext.GetCreateAndAddStatus` and `WaitForCreateAndAdd` for a new
dataset. High-level upload recovery continues to use
`FailedAttempt.Submission`: extract the same minimal fields rather than storing
the complete value. `OnPiecesAdded` remains a transaction progress event and
still receives a transaction hash rather than a recovery handle.

Applications that need to map returned piece IDs to CIDs must persist that
business mapping with their original request.

## Discovery And Lifecycle

Common management calls:

- `FindDataSets`: list datasets owned by the signer or another payer.
- `GetStorageInfo`: inspect providers, pricing, limits, and allowances.
- `ProviderContext.Download` / `DataSetContext.Download`: download from a known provider.
- `DataSetContext.DeletePieceByID`: schedule exact removal by on-chain piece ID.
- `DataSetContext.DeletePiece`: schedule removal by piece CID convenience lookup. Prefer
  `DeletePieceByID` when available, because repeated uploads can share a CID.
- `DataSetContext.TerminateService` / `Service.TerminateService`: terminate service
  and wait for confirmation. The provider relays by default; set `SkipProvider`
  to submit directly through FWSS.
- `WarmStorage().TerminateDataSet`: submit directly and obtain a raw `WriteResult`.

Termination and removal are storage lifecycle actions. Treat them as
application-level destructive operations and gate them accordingly.

### Terminating A Service

Termination uses provider relay by default and requires full payment-account
settlement. To submit from the root wallet without provider cooperation, set
`SkipProvider: true`:

```go
termination, err := client.Storage().TerminateService(ctx, dataSetID, &storage.TerminateServiceOptions{
	SkipProvider:      true,
	DirectWaitTimeout: 3 * time.Minute,
})
if err != nil {
	log.Fatal(err)
}
fmt.Println("service ends at epoch:", termination.EndEpoch)
```

Both paths wait for confirmation. Relay ends service immediately; direct
submission keeps service and payments active until `EndEpoch`. Confirmation
does not mean that data has been deleted. For direct submission, use `OnSubmitted`
to save the hash before waiting: a timeout can occur after broadcast, so do not
blindly resubmit. Use `WarmStorage().TerminateDataSet` for broadcast-only calls
or raw receipts; see the package API documentation for write options.

## Services

`synapse.Client` exposes these service entry points:

| Service | Use it for |
|---------|------------|
| `Storage()` | Upload, download, prepare, contexts, datasets |
| `Payments()` | USDFC balances, account summary, deposits, withdrawals, approvals, rails |
| `Costs()` | Upload cost, lockup, and deposit estimates |
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
