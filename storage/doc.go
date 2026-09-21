// Package storage provides provider selection, immutable storage targets, and
// single- or multi-copy upload orchestration.
//
// # Contexts
//
// [ProviderContext] identifies one provider but no data set. Its CreateAndAdd
// and Pull operations create a new data set. [DataSetContext] identifies one
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
// [DataSetDetails] exposes its base data-set fields directly. Reading a
// promoted field from a zero DataSetDetails does not panic; each field retains
// its ordinary zero value, including nil pointer and map fields. Check
// DataSetID and ProviderID when a complete identity is required.
//
// Persistent data-set references, context identities, and commit status and
// result values use strict lowerCamel JSON field names. Alternate
// capitalization, unknown or duplicate fields, and incomplete objects are
// rejected. Submission values are runtime results, not persistence schemas.
//
// # Upload batching
//
// The root synapse Client enables commit batching for high-level Upload calls
// by default. Ready pieces are submitted together once no other upload to the
// same target is in progress and three seconds pass without a new piece. There
// is no time limit by default, so a slow upload delays the others for that
// target. At most four batches are signed and submitted at once; provider
// confirmation waits do not consume that limit. Configure the root coordinator
// with [synapse.WithUploadBatching], or disable it with
// [synapse.WithoutUploadBatching].
//
// [WithUploadIdleWait] changes the three-second delay. [WithUploadMaxWait]
// submits a batch at most that long after its first piece is ready.
// [WithoutUploadIdleWait] with a zero [WithUploadMaxWait] submits every piece
// immediately. With both timers disabled, a batch is submitted only by
// [Service.Flush], the 40-piece limit, the provider message-size limit, or a
// repeated piece CID.
//
// Compatible uploads share a window only when their immutable target matches.
// Existing data sets match by provider, complete DataSetRef, and exact service
// URL. New data sets also require the same payee, CDN setting, and data-set
// metadata. Uploads to the same new target share one data set for the
// coordinator's lifetime, across windows and sequential uploads, without
// binding or mutating the source ProviderContext.
//
// Store, Pull, target resolution, and coordinator admission use the caller
// context. Successful admission transfers ownership of the piece to the
// coordinator. Later caller cancellation stops that caller's wait and
// suppresses later callbacks, but the coordinator continues signing,
// submission, and confirmation. An Upload context error therefore does not
// prove that the accepted piece will not commit; reconcile external state
// before retrying. A terminal result already published when cancellation races
// takes precedence over the context error.
//
// [Service.Flush] waits for uploads that began before the call to reach the
// coordinator, submits their corresponding windows, and waits for final
// confirmations. A later compatible upload may join one of those windows, so
// later uploads are neither guaranteed to be included nor excluded. A waiter
// that receives a terminal result consumes that piece's failure, so a later
// Flush does not report it again. Flush still reports failures that no waiter
// observed. Canceling the Flush context stops only that wait and does not
// discard those unobserved failures. A completed Flush publishes terminal
// results before returning. The root [synapse.Client.Close] closes its
// coordinator without flushing, so call Service.Flush first when a graceful
// drain is required. Close cannot retract a provider submission that already
// succeeded.
//
// Standalone users can construct [UploadBatcher] with [NewUploadBatcher] and
// inject it through [Options.UploadBatcher] or [WithUploadBatcher]. The caller
// owns an injected coordinator and must flush or close it. With no injected
// coordinator, standalone services and contexts retain immediate per-upload
// commits.
//
// During a multi-copy Service upload, the primary and every secondary
// participate in batching. Replicas from one Upload call are not guaranteed to
// share a transaction. Low-level Store, Pull, and commit lifecycle methods keep
// their direct behavior; only high-level Upload methods use the coordinator.
//
// The Go API intentionally uses an explicitly owned coordinator, functional
// options, context-aware Flush, and immutable context injection rather than
// copying the upstream TypeScript SDK's configuration and lifecycle shape.
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
// copy counts through [UploadResult]. Configure this path with [UploadOptions].
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
// Cost estimates use [costs.MultiContextCosts].
// [Service.CalculateMultiContextCosts] returns the estimate directly;
// [Service.Prepare] includes it in [PrepareResult.Costs]. To use a precomputed
// estimate, set [PrepareOptions.Costs] without any other preparation options.
// Otherwise, supply each piece's raw payload size in [PrepareOptions.PieceSizes].
// Every size must be between chain.MinUploadSize and chain.MaxUploadSize. The
// same plan applies to each context; piece count is derived from the list.
// Existing contexts require [Options.DataSetLeafCountReader]. The root client
// assembles it when the resolved contract topology includes PDPVerifier.
// Services without that reader return [ErrUninitialized] instead of estimating
// existing usage as zero. New contexts and precomputed costs do not need it.
// Reader results must be non-nil and non-negative; zero means known empty.
// Read errors retain their original cause and unavailable datasets retain
// [ErrDataSetUnavailable]. Invalid successful reader results return ordinary
// errors, rather than caller argument or unavailable-dataset errors.
//
// UploadToContexts does not select replacements. The first context stores the
// reader; later contexts pull from it. Configure this path with
// [UploadToContextsOptions]. Service.Upload retains automatic replacement for
// failed secondary copies. Direct [ProviderContext.Upload] and
// [DataSetContext.Upload] calls store one copy and accept [ContextUploadOptions].
// [StorageContext] is the sealed, ordered mixed-target view used by selection,
// preparation, and UploadToContexts; commit lifecycle methods remain on the
// concrete context whose target determines their meaning.
//
// Contexts carry an immutable [ContextIdentity] containing payer, chain, and
// record-keeper identities. Service validates this identity before cost
// calculation or upload work, preventing contexts from another account or
// chain from being used accidentally.
//
// # Submission recovery
//
// When creation must remain recoverable even if the provider receives the
// request but its HTTP response is lost, choose and persist a client data-set
// ID before submitting. Persist the provider ID and context identity beside
// it, then rebuild the same ProviderContext before looking up the result:
//
//	clientDataSetID := types.NewBigInt(123)
//	providerID := providerContext.ProviderID()
//	identity := providerContext.ContextIdentity()
//	// Persist clientDataSetID, providerID, and identity before this call.
//	_, err := providerContext.CreateDataSet(ctx, &CreateDataSetOptions{
//		ClientDataSetID: &clientDataSetID,
//	})
//	// After an ambiguous error, rebuild the same provider context and poll:
//	ref, found, err := freshProviderContext.FindDataSetByClientDataSetID(ctx, clientDataSetID)
//
// A create-and-add request uses [CreateAndAddRequest.ClientDataSetID] in the same
// way. Persist the operation kind and piece CIDs as application state too,
// because finding the data set does not prove that its pieces were added.
// A false found result means the matching data set is not visible in the
// current chain state; it is not evidence that the provider rejected the
// request. Applications own polling and deadline policy.
//
// A nil ClientDataSetID preserves the default random-ID behavior, but an ID
// generated inside a request cannot be recovered if the provider handle is
// lost. Client data-set IDs and add-pieces nonces share a payer-scoped on-chain
// namespace. Never reuse a consumed value, including after data-set deletion.
// The ID is a correlation key, not an HTTP idempotency key; create POSTs are
// sent once and are not automatically retried.
//
// [ProviderContext.CreateDataSet] leaves its receiver unbound. When
// confirmation must survive a restart, persist StatusURL and ClientDataSetID
// from the [CreateDataSetSubmission] received through
// [CreateDataSetOptions.OnSubmitted]. A fresh ProviderContext for the same
// provider can pass those values to [ProviderContext.WaitForDataSetCreated].
// Pass the returned DataSetRef to [ProviderContext.ForDataSet] to obtain a
// DataSetContext. Waiting does not require an FWSS reader.
//
// [ProviderContext.CreateAndAdd] and [DataSetContext.Commit] are convenience
// methods that submit once and wait for confirmation. Their OnSubmitted
// callback receives an independent runtime [CommitSubmission] after the
// provider handle is validated and before confirmation starts. For
// create-and-add, persist StatusURL and the original ClientDataSetID. For
// add-pieces, persist StatusURL with the target DataSetRef.
// For explicit recovery control, prefer SubmitCreateAndAdd followed by
// WaitForCreateAndAdd on ProviderContext, or SubmitCommit followed by
// WaitForCommit on DataSetContext.
//
// GetCreateAndAddStatus and GetCommitStatus perform one logical status check
// and return [CommitStatePending], [CommitStateConfirmed], or
// [CommitStateRejected]. A rejected status is returned without an error;
// WaitForCreateAndAdd and WaitForCommit report the same terminal state as
// [CommitRejectedError].
//
// Uploads keep the same handle. When an upload commit fails after the provider
// accepted its submission, including a failed or timed-out wait, the
// [FailedAttempt] in [UploadResult.FailedAttempts] or
// [CommitError.FailedAttempts] carries it as Submission. Resume a
// create-and-add submission on [Service.NewProviderContext] for its ProviderID
// and call WaitForCreateAndAdd with its StatusURL and original ClientDataSetID.
// For add-pieces, open its DataSet with [Service.NewDataSetContext] and call
// WaitForCommit with StatusURL. A batched submission can include other uploads'
// pieces. Applications that need a durable CID-to-piece-ID mapping must retain
// their original request order; generic recovery validates only that the
// provider's confirmed count matches its returned piece IDs.
//
// # Service termination
//
// [Service.TerminateService] and [DataSetContext.TerminateService] wait for
// confirmed termination and return [TerminateServiceResult]. By default the
// provider relays an immediate termination requiring full payment-account
// settlement. There is no automatic fallback to direct submission. Set
// [TerminateServiceOptions.SkipProvider] to submit through FWSS without provider
// cooperation; the service and payments continue until the returned EndEpoch.
// Neither path waits until EndEpoch or cleans up the remaining data-set state.
//
// Direct termination always waits for a receipt. DirectWaitTimeout controls
// that wait and overrides WithWait in WriteOptions. OnSubmitted reports the
// original hash synchronously after successful direct broadcast, before receipt
// polling. Save that hash to track the transaction if waiting later fails.
// The callback does not indicate confirmation; waiting errors still return no
// partial high-level result and do not prove that nothing was broadcast.
// Direct submission dependencies receive the callback and wait timeout
// explicitly through [FWSSTerminationOptions].
//
// For broadcast-only submission, raw receipts, or partial transaction results
// returned alongside errors, use [warmstorage.Service.TerminateDataSet], available
// through the root client's WarmStorage method. Its default and non-positive
// WithWait values return after broadcast. A positive WithWait waits for a receipt
// while retaining the submission hash on waiting errors and the receipt on tx failure.
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
// [StorageContext] is sealed and implemented only by [ProviderContext] and
// [DataSetContext]. Custom resolvers may return those built-in contexts but
// cannot provide their own implementation. [PDPProviderClient],
// [PDPVerifierReader], [FWSSDataSetReader], [FWSSTerminator], and
// [MultiCostCalculator] are SDK assembly interfaces implemented by [pdp.Client],
// [costs.Service], and adapters assembled by the root SDK client.
//
// [pdp.Client]: https://pkg.go.dev/github.com/strahe/synapse-go/pdp#Client
// [costs.MultiContextCosts]: https://pkg.go.dev/github.com/strahe/synapse-go/costs#MultiContextCosts
// [costs.Service]: https://pkg.go.dev/github.com/strahe/synapse-go/costs#Service
// [warmstorage.Service.TerminateDataSet]: https://pkg.go.dev/github.com/strahe/synapse-go/warmstorage#Service.TerminateDataSet
// [signer.StorageSigner]: https://pkg.go.dev/github.com/strahe/synapse-go/signer#StorageSigner
// [synapse.WithStorageSigner]: https://pkg.go.dev/github.com/strahe/synapse-go#WithStorageSigner
// [synapse.WithUploadBatching]: https://pkg.go.dev/github.com/strahe/synapse-go#WithUploadBatching
// [synapse.WithoutUploadBatching]: https://pkg.go.dev/github.com/strahe/synapse-go#WithoutUploadBatching
package storage
