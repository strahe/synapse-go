// Package storage provides the multi-copy upload orchestration service.
//
// The central types are [*Service] (high-level upload/download operations),
// [*Context] (per-provider store/pull/commit/download operations), and
// [*ServiceResolver] (selection + dataset-reuse wiring against warmstorage and
// spregistry services). [UploadResolver] is the upload-facing selection
// contract; [ContextResolver] is the managed-context creation contract used by
// CreateContext(s) and Prepare's auto-create path.
//
// # Manager-level operations
//
// [Service] also exposes manager-level operations:
//
//   - [Service.FindDataSets] — enumerate the caller's data sets.
//   - [Service.GetStorageInfo] — aggregated pricing / providers / allowances view.
//   - [Service.TerminateService] — terminate service through the provider relay by default.
//   - [Service.TerminateDataSet] — legacy direct FWSS termination write.
//   - [Service.CalculateMultiContextCosts] — aggregate cost calculation across refs.
//   - [Service.CreateContext] / [Service.CreateContexts] / [Service.GetDefaultContext] —
//     build upload contexts without invoking the full upload pipeline.
//   - [Service.Prepare] — compute required funding and return a deferred
//     [PrepareTransaction] to move funds into place when the account is not Ready.
//
// For exact preparation, create contexts first with [Service.CreateContexts],
// pass the same contexts to [Service.Prepare], execute any returned funding
// transaction, then upload through the matching staged or per-context path.
// Prepare's auto-created contexts are estimate-only; they are not cached,
// reserved, or bound to a later [Service.Upload] call.
//
// Per-context manager operations live on [*Context]: [Context.Upload]
// (single-copy), [Context.DeletePieceByID], [Context.DeletePiece],
// [Context.PieceStatus], [Context.GetScheduledRemovals] and
// [Context.Terminate] and [Context.TerminateService]. TerminateService asks
// the provider to relay by default; pass SkipProvider for the direct FWSS
// transaction path while still receiving EndEpoch from the receipt.
//
// # Upload Flow
//
// The multi-copy upload follows a store → pull → commit pipeline:
//
//  1. Store: Upload data to the primary storage provider.
//  2. Pull: Secondary providers fetch data from the primary (SP-to-SP).
//  3. Commit: All providers call AddPieces on-chain with EIP-712 signatures.
//
// The Service handles orchestration of the full multi-copy flow, while
// ServiceResolver implements both resolver contracts. It reuses provider-local
// datasets only when metadata matches exactly and the warmstorage-approved
// provider set intersects active PDP providers from spregistry. For explicit
// provider IDs, the default warmstorage service checks matching data sets with
// bounded concurrency, preferring the oldest one with active pieces and falling
// back to the oldest empty match. Custom catalogs without active-piece reads
// retain the original oldest-match behavior. Automatically selected providers
// must also pass a bounded PDP health check before a context is created;
// explicitly selected provider and data set IDs are not probed. If only some
// automatically selected providers are healthy, the resolver returns the
// healthy subset so upload callers can surface partial-copy results.
// Existing data sets that cannot accept writes surface typed errors such as
// DataSetPDPPaymentTerminatedError. Use errors.AsType to access fields like
// PDPEndEpoch.
//
// Downloads are validated as they stream so callers can keep io.Reader-style
// boundaries without skipping PieceCID verification. Context.Download can use
// a CDN-backed retriever first when the context has CDN enabled, then fall back
// to provider PDP retrieval on ordinary CDN failures. For URL-based
// [Service.Download] calls, the default HTTP client refuses to dial local,
// private, multicast, unspecified, or otherwise reserved address ranges to
// guard against SSRF, and it ignores environment-variable proxies for the same
// reason; set [Options.AllowPrivateNetworks] when connecting to trusted private
// infrastructure, or provide [Options.HTTPClient] if you need explicit proxy
// control. [Context.Download] uses the PDP/CDN clients attached to the context
// and is not covered by this default SSRF guard. Bound the number of bytes
// accepted per URL-based Service.Download call via [Options.DownloadMaxBytes];
// Context.Download is not subject to this cap.
//
// [UploadOptions] exposes per-upload lifecycle hooks covering the full
// store → pull → commit pipeline:
//
//   - [UploadOptions.OnProgress] — bytes streamed to the primary provider.
//   - [UploadOptions.OnStored] — primary provider confirmed storage.
//   - [UploadOptions.OnPiecesAdded] — on-chain AddPieces transaction submitted
//     (batch-shaped: carries a []SubmittedPiece per provider per commit).
//   - [UploadOptions.OnPiecesConfirmed] — on-chain AddPieces transaction confirmed
//     (batch-shaped: carries a []ConfirmedPiece with assigned on-chain IDs).
//   - [UploadOptions.OnCopyComplete] — secondary SP-to-SP pull succeeded.
//   - [UploadOptions.OnCopyFailed] — a secondary SP-to-SP copy attempt failed
//     (presign failures remain FailedAttempts-only).
//   - [UploadOptions.OnPullProgress] — per-piece status update during a secondary pull.
//
// [Service.Upload] and [Context.Upload] recover and ignore [UploadOptions]
// callback panics. If a logger is configured, the first panic per callback name
// in an upload is logged as a warning. Direct staged hooks on [StoreOptions],
// [PullRequest], and [CommitRequest] are invoked as-is and are not covered by
// this recovery guarantee.
//
// [Context.Pull] checks that each requested piece resolves to a non-empty
// source URL. The PDP provider performs stricter source-URL validation before executing
// the provider-to-provider pull.
//
// Callers that need restartable staged uploads can split secondary creation
// from piece registration: build a context with [Service.CreateContext], call
// [Context.CreateDataSet], persist the [CreateDataSetSubmission] from
// [CreateDataSetOptions.OnSubmitted], resume with [Context.WaitForDataSetCreated]
// if needed, then run [Context.Pull] and [Context.Commit].
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package storage
