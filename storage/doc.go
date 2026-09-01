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
// piece from the configured provider or CDN. Data-set inspection, single or
// batch deletion, and termination methods are available only on DataSetContext,
// while standalone data-set creation and recovery are available only on
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
// # Signing and payer identity
//
// Storage authorization uses the [signer.StorageSigner] capability: an EVM
// address plus 32-byte hash signing. Context constructors accept a nil or
// typed-nil signer so read-only contexts remain usable; operations that require
// a signature then return [ErrInvalidArgument]. [WithPayer] configures the
// paying account independently from the signer.
//
// Standalone [Service] configuration has two additional payer inputs.
// [Options.PayerAddress] applies to manager-level helpers and defaults to the
// configured signer address only when left zero. Set it explicitly when a
// delegated signer acts for another payer. [ServiceResolverOptions.Payer]
// independently determines the payer assigned to contexts created by that
// resolver. The root synapse Client keeps both values on the root account when
// [synapse.WithStorageSigner] configures a delegated signer.
//
// # Provider selection
//
// Automatic upload selection requires an endorsed primary by default. The
// primary is selected from providers that are endorsed, FWSS-approved,
// registry-active, and healthy. Secondary copies use the complete approved,
// active, healthy pool after excluding the primary. Set
// [UploadOptions.AllowUnendorsedPrimary] or
// [SelectUploadContextsOptions.AllowUnendorsedPrimary] to true to use the
// complete approved pool for the primary and skip the endorsement query.
// [Service.SelectProviderContext], explicit provider or data-set contexts, and
// replacement selection do not query endorsements.
//
// Standalone [ServiceResolver] users configure the single-method
// [EndorsedProviderSource] through [ServiceResolverOptions.Endorsements]. With
// that source configured, the zero-value selection policy is strict. Without
// it, strict selection returns [ErrEndorsementsNotConfigured] when upload
// selection runs; setting AllowUnendorsedPrimary continues normally with the
// approved pool. A nil slice with a nil source error is an
// empty set, while source errors remain query errors. Implementations outside
// this module may satisfy the interface directly; its one-method shape and
// nil/error contract are part of the public dependency-injection contract.
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
// context. They enforce the exact raw payload size encoded in PieceCIDv2 and
// return [ErrMaxBytesExceeded] if a response is larger. URL-based
// [Service.Download] uses the Service HTTP client. Its default client rejects
// private and reserved network destinations; configure
// [Options.AllowPrivateNetworks] only for trusted private infrastructure.
// [Options.DownloadMaxBytes] can cap URL-based downloads.
//
// # Stability
//
// During the 0.x phase, public APIs may change between minor releases.
// [PDPProviderClient] and [PDPVerifierReader] are SDK assembly interfaces. Their
// supported implementations are [pdp.Client] and the PDPVerifier adapter
// assembled by the root SDK client; user-defined implementations are not
// compatibility targets.
//
// [pdp.Client]: https://pkg.go.dev/github.com/strahe/synapse-go/pdp#Client
// [signer.StorageSigner]: https://pkg.go.dev/github.com/strahe/synapse-go/signer#StorageSigner
// [synapse.WithStorageSigner]: https://pkg.go.dev/github.com/strahe/synapse-go#WithStorageSigner
package storage
