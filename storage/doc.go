// Package storage provides provider selection, immutable storage targets, and
// single- or multi-copy upload orchestration.
//
// # Contexts
//
// [ProviderContext] identifies one provider but no data set. Its Commit and
// Pull operations create a new data set. [DataSetContext] identifies one
// provider and one existing data set; its Commit and Pull operations always
// use that data set. Neither type changes target after construction.
//
// Both types expose provider-scoped operations such as Store and Download.
// Download therefore behaves the same on both: it retrieves the requested
// piece from the configured provider or CDN. Data-set inspection, deletion,
// and termination methods are available only on DataSetContext, while
// standalone data-set creation and recovery are available only on
// ProviderContext.
//
// Use [Service.NewProviderContext] or [Service.NewDataSetContext] when the
// target ID is already known. Use [Service.SelectProviderContext] to choose one
// healthy provider without looking up data sets. Use
// [Service.SelectUploadContexts] when preparing a new upload; it may reuse
// writable data sets whose metadata matches.
//
// [DataSetRef] is the persistent target reference shared by creation results,
// DataSetContext, and [ProviderContext.ForDataSet]. Construct it with
// [NewDataSetRef]; its zero value is invalid.
//
// Persistent data-set references, context identities, and create/commit
// lifecycle values use strict lowerCamel JSON field names. Alternate
// capitalization, unknown or duplicate fields, and incomplete objects are
// rejected.
//
// # Upload flow
//
// [Service.Upload] automatically selects targets and performs store, pull, and
// commit. Copies must be explicitly positive. If fewer targets are available,
// the upload continues with those targets and reports the requested and actual
// copy counts through [UploadResult].
//
// For an exact preflight and upload, use the same context instances throughout:
//
//  1. Call [Service.SelectUploadContexts]. A non-empty partial selection is
//     returned with [InsufficientUploadContextsError].
//  2. Pass the selected contexts to [Service.Prepare].
//  3. Execute the returned [PrepareTransaction], if any.
//  4. Pass the same contexts, in the desired primary-to-secondary order, to
//     [Service.UploadToContexts].
//
// UploadToContexts does not select replacements. The first context stores the
// reader; later contexts pull from it. Service.Upload retains automatic
// replacement for failed secondary copies.
//
// Contexts carry an immutable [ContextIdentity] containing payer, chain, and
// record-keeper identities. Service validates this identity before cost
// calculation or upload work, preventing contexts from another account or
// chain from being used accidentally.
//
// # Submission recovery
//
// [ProviderContext.CreateDataSet] leaves its receiver unbound. Persist the
// [CreateDataSetSubmission] received through [CreateDataSetOptions.OnSubmitted]
// when confirmation must survive a restart. A fresh ProviderContext for the
// same provider can resume with [ProviderContext.WaitForDataSetCreated]. Pass
// the returned DataSetRef to [ProviderContext.ForDataSet] to obtain a
// DataSetContext.
//
// [ProviderContext.Commit] and [DataSetContext.Commit] are convenience methods
// that submit once and wait for confirmation. Applications that must survive
// process restarts can split that lifecycle with SubmitCommit,
// GetCommitStatus, and WaitForCommit on the same concrete context type.
// Persist the complete [CommitSubmission] returned by SubmitCommit before
// waiting. A fresh context for the same immutable target can resume that
// submission without signing or submitting another transaction.
//
// GetCommitStatus performs one logical status check and returns
// [CommitStatePending], [CommitStateConfirmed], or [CommitStateRejected]. A
// rejected status is returned without an error; WaitForCommit reports the same
// terminal state as [CommitRejectedError].
//
// # Downloads
//
// Context downloads use the PDP and optional CDN clients attached to that
// context. URL-based [Service.Download] uses the Service HTTP client. Its
// default client rejects private and reserved network destinations; configure
// [Options.AllowPrivateNetworks] only for trusted private infrastructure.
// [Options.DownloadMaxBytes] can cap URL-based downloads.
//
// # Stability
//
// During the 0.x phase, public APIs may change between minor releases.
package storage
