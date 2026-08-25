package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

const maxSecondaryAttemptsDefault = 5

const commitConcurrencyDefault = 4

// defaultDownloadTimeout is applied to the Service's HTTP client for
// URL-based downloads.  It is long enough for multi-GiB files transferred
// over a typical storage network while preventing indefinite hangs.
const defaultDownloadTimeout = 24 * time.Hour

// StorageContext is an immutable provider or data-set upload target.
type StorageContext interface {
	ContextIdentity() ContextIdentity
	ProviderID() types.BigInt
	GetProviderInfo() Provider
	DataSetRef() (DataSetRef, bool)
	ServiceURL() string
	CDNEnabled() bool
	PieceURL(cid.Cid) string
	Store(context.Context, io.Reader, *StoreOptions) (*StoreResult, error)
	Download(context.Context, cid.Cid) (io.ReadCloser, error)
	PresignForCommit(context.Context, []PieceInput) ([]byte, error)
	Pull(context.Context, PullRequest) (*PullResult, error)
	Commit(context.Context, CommitRequest) (*CommitResult, error)
	Upload(context.Context, io.Reader, *UploadOptions) (*UploadResult, error)
}

// UploadResolver selects provider contexts for upload operations and provides
// replacement candidates when a secondary provider fails.
type UploadResolver interface {
	ResolveUploadContexts(context.Context, *UploadOptions) ([]StorageContext, bool, error)
	SelectReplacement(context.Context, map[string]types.BigInt, *UploadOptions) (StorageContext, error)
}

// ContextResolver opens explicitly identified provider and data-set targets.
type ContextResolver interface {
	ResolveProviderContext(context.Context, types.BigInt, NewProviderContextOptions) (*ProviderContext, error)
	ResolveDataSetContext(context.Context, types.BigInt, NewDataSetContextOptions) (*DataSetContext, error)
}

// ContextSelector chooses healthy targets for new uploads.
type ContextSelector interface {
	SelectProviderContext(context.Context, SelectProviderContextOptions) (*ProviderContext, error)
	SelectUploadContexts(context.Context, SelectUploadContextsOptions) (*UploadContextSelection, error)
}

type writableUploadResolver interface {
	resolveWritableUploadContexts(context.Context, *UploadOptions) ([]StorageContext, bool, error)
	selectWritableReplacement(context.Context, map[string]types.BigInt, *UploadOptions) (StorageContext, error)
}

// Service orchestrates multi-copy uploads and downloads.
// Create with New; configure via the [Options] struct.
type Service struct {
	resolver             UploadResolver
	contextResolver      ContextResolver
	contextSelector      ContextSelector
	httpClient           *http.Client
	source               string
	defaultWithCDN       bool
	maxSecondaryAttempts int
	commitConcurrency    int
	downloadMaxBytes     int64
	logger               *slog.Logger
	lifecycle            *lifecycle.Lifecycle

	// Manager-level collaborators (all optional). When unset, the
	// corresponding public method returns a descriptive error.
	finder       DataSetFinder
	info         StorageInfoReader
	terminator   FWSSTerminator
	costCalc     MultiCostCalculator
	funder       PaymentsFunder
	sizeReader   DataSetSizeReader
	dsReader     FWSSDataSetReader
	providers    ProviderResolver
	payments     PaymentStateReader
	epochs       EpochReader
	signer       signer.EVMSigner
	chainID      types.ChainID
	recordKeeper common.Address
	paymentToken common.Address

	// signerAddr is the default client/payer used by manager-level
	// helpers when the caller does not explicitly supply one. Zero
	// address is allowed (callers must pass an address explicitly).
	signerAddr common.Address
}

// Options configures a Service. Unset fields fall back to sensible defaults.
type Options struct {
	// Resolver selects provider contexts for Upload and supplies replacement
	// candidates when a secondary provider fails. A nil resolver is allowed
	// so the Service can still serve DownloadFromContext / download-by-URL
	// calls; Upload then returns a clean validation error.
	Resolver UploadResolver

	// ContextResolver opens explicitly identified storage targets. When nil and
	// Resolver also implements ContextResolver, New reuses Resolver.
	ContextResolver ContextResolver

	// ContextSelector chooses healthy providers and upload targets. When nil and
	// Resolver also implements ContextSelector, New reuses Resolver.
	ContextSelector ContextSelector

	// HTTPClient is used for URL-based downloads. nil installs a client with
	// a 24-hour timeout — long enough for multi-GiB transfers over typical
	// storage networks while preventing indefinite hangs. The default client
	// also disables environment-variable proxies to avoid proxy-assisted SSRF
	// bypass. When set, the SDK's built-in SSRF protection is bypassed
	// entirely: the provided Transport is responsible for implementing
	// equivalent safeguards (private-network rejection, DNS-rebind close
	// window, redirect filtering). AllowPrivateNetworks has no effect in this
	// case.
	HTTPClient *http.Client

	// Source is the application identifier for dataset namespace isolation.
	// Datasets with different Source values are treated as separate
	// namespaces; reuse only occurs within the same Source.
	Source string

	// DefaultWithCDN is the Client-level CDN default applied when an
	// UploadOptions.WithCDN or context-selection WithCDN field is nil.
	// Ignored when the per-operation field is non-nil.
	DefaultWithCDN bool

	// MaxSecondaryAttempts caps the number of provider candidates tried for
	// each secondary copy slot before giving up. Values <= 0 select the
	// default of 5.
	MaxSecondaryAttempts int

	// CommitConcurrency caps the number of concurrent on-chain Commit RPCs
	// issued across primary + secondary copies. Values <= 0 select the
	// default of 4. The cap only matters when RequestedCopies exceeds it.
	CommitConcurrency int

	// AllowPrivateNetworks disables the default SSRF protection applied to
	// URL-based Service.Download calls. When false (the default), the
	// built-in HTTP client refuses to dial local, private, multicast,
	// unspecified, or otherwise reserved address ranges and returns
	// ErrPrivateNetwork.
	// Set to true only when you knowingly need to download from a private
	// network (e.g. in-cluster storage). Ignored when HTTPClient is set.
	AllowPrivateNetworks bool

	// DownloadMaxBytes caps the number of bytes a single URL-based
	// Service.Download will return. Zero (the default) disables the cap.
	// Exceeding it returns ErrMaxBytesExceeded either eagerly (via
	// Content-Length) or at the terminal Read of the returned reader.
	DownloadMaxBytes int64

	// Logger receives internal warnings. nil disables logging.
	Logger *slog.Logger

	// Lifecycle, when non-nil, ties this Service to the owning Client's
	// close state. After the Lifecycle is closed, every method returns
	// ErrClosed. Nil is allowed for standalone use.
	Lifecycle *lifecycle.Lifecycle

	// DataSetFinder backs Service.FindDataSets. Optional; when nil the
	// method returns an ErrUninitialized-wrapped error.
	DataSetFinder DataSetFinder

	// StorageInfoReader backs Service.GetStorageInfo. Optional.
	StorageInfoReader StorageInfoReader

	// DataSetTerminator backs Service.TerminateDataSet. Optional.
	DataSetTerminator FWSSTerminator

	// CostCalculator backs Service.CalculateMultiContextCosts and the
	// cost estimation inside Prepare. Optional.
	CostCalculator MultiCostCalculator

	// PaymentsFunder backs PrepareTransaction.Execute. Optional.
	PaymentsFunder PaymentsFunder

	// DataSetSizeReader backs the per-dataset size lookup performed by
	// Service.Prepare for existing-dataset contexts. Optional; when
	// nil, Prepare falls back to zero-size estimates. For
	// accurate add-pieces pricing, wire an implementation backed by
	// PDPVerifier.getDataSetLeafCount (leafCount * 32 bytes).
	DataSetSizeReader DataSetSizeReader

	// FWSSDataSetReader is used by explicit data-set resolution to read the
	// on-chain ClientDataSetID
	// and to equip returned contexts with upload-time ended-dataset
	// checks. When nil, those safety nets are skipped.
	FWSSDataSetReader FWSSDataSetReader

	// ProviderResolver resolves provider endpoints for manager-level
	// provider-relayed termination. Optional.
	ProviderResolver ProviderResolver

	// PaymentStateReader and EpochReader back the provider-relayed termination
	// debt pre-check. Optional.
	PaymentStateReader PaymentStateReader
	EpochReader        EpochReader
	PaymentToken       common.Address

	// Signer is required for manager-level provider-relayed termination.
	// ChainID and RecordKeeper also define the identity accepted by Prepare,
	// Upload, UploadToContexts, and the explicit context APIs.
	Signer       signer.EVMSigner
	ChainID      types.ChainID
	RecordKeeper common.Address

	// SignerAddress is the payer/client identity used by manager-level helpers
	// and validated against every StorageContext. When Signer is set, a non-zero
	// SignerAddress must match Signer.EVMAddress. When zero, New derives it from
	// Signer.
	SignerAddress common.Address
}

// New creates a Service from the given Options.
func New(opts Options) (*Service, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = newSafeHTTPClient(defaultDownloadTimeout, opts.AllowPrivateNetworks)
	}
	if opts.MaxSecondaryAttempts <= 0 {
		opts.MaxSecondaryAttempts = maxSecondaryAttemptsDefault
	}
	if opts.CommitConcurrency <= 0 {
		opts.CommitConcurrency = commitConcurrencyDefault
	}
	if opts.DownloadMaxBytes < 0 {
		opts.DownloadMaxBytes = 0
	}
	signerAddr := opts.SignerAddress
	if opts.Signer != nil {
		signerAddress := opts.Signer.EVMAddress()
		if signerAddr != (common.Address{}) && signerAddr != signerAddress {
			return nil, fmt.Errorf("storage.New: %w: SignerAddress does not match Signer", ErrInvalidArgument)
		}
		signerAddr = signerAddress
	}
	resolver := normalizeOptional(opts.Resolver)
	contextResolver := normalizeOptional(opts.ContextResolver)
	if contextResolver == nil {
		if inherited, ok := resolver.(ContextResolver); ok {
			contextResolver = inherited
		}
	}
	contextSelector := normalizeOptional(opts.ContextSelector)
	if contextSelector == nil {
		if inherited, ok := resolver.(ContextSelector); ok {
			contextSelector = inherited
		}
	}
	providers := normalizeOptional(opts.ProviderResolver)
	if providers == nil {
		if inherited, ok := contextResolver.(ProviderResolver); ok {
			providers = inherited
		}
	}
	return &Service{
		resolver:             resolver,
		contextResolver:      contextResolver,
		contextSelector:      contextSelector,
		httpClient:           opts.HTTPClient,
		source:               opts.Source,
		defaultWithCDN:       opts.DefaultWithCDN,
		maxSecondaryAttempts: opts.MaxSecondaryAttempts,
		commitConcurrency:    opts.CommitConcurrency,
		downloadMaxBytes:     opts.DownloadMaxBytes,
		logger:               opts.Logger,
		lifecycle:            opts.Lifecycle,
		finder:               normalizeOptional(opts.DataSetFinder),
		info:                 normalizeOptional(opts.StorageInfoReader),
		terminator:           normalizeOptional(opts.DataSetTerminator),
		costCalc:             normalizeOptional(opts.CostCalculator),
		funder:               normalizeOptional(opts.PaymentsFunder),
		sizeReader:           normalizeOptional(opts.DataSetSizeReader),
		dsReader:             normalizeOptional(opts.FWSSDataSetReader),
		providers:            providers,
		payments:             normalizeOptional(opts.PaymentStateReader),
		epochs:               normalizeOptional(opts.EpochReader),
		signer:               opts.Signer,
		chainID:              opts.ChainID,
		recordKeeper:         opts.RecordKeeper,
		paymentToken:         opts.PaymentToken,
		signerAddr:           signerAddr,
	}, nil
}

// Upload automatically selects targets and runs the multi-copy upload pipeline
// streaming from r in a single pass. opts must be non-nil. Returns UploadResult whose
// Complete field indicates whether all requested copies were committed
// on-chain.
//
// The reader is consumed once by the primary provider; secondary copies
// are populated via server-to-server Pulls. On success the reader is
// fully drained; on error it may be only partially consumed.
//
// Timeouts and cancellation: Upload honours ctx for every step —
// presign, store, pull and on-chain commit wait. To bound the total
// upload time (including the blockchain confirmation that populates the
// returned [UploadResult.Copies]), wrap ctx with [context.WithTimeout]; the
// Service itself does not impose an internal wait deadline. The built-in
// 24h HTTP timeout on Service only affects URL-based downloads; Upload,
// Pull, and Commit use the StorageContext implementation's own HTTP client
// configuration.
func (s *Service) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	const op = "storage.Service.Upload"
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if opts == nil {
		return nil, fmt.Errorf("%s: %w: options must not be nil", op, ErrInvalidArgument)
	}
	if opts.Copies <= 0 {
		return nil, fmt.Errorf("%s: %w: Copies must be greater than zero", op, ErrInvalidArgument)
	}
	if err := validateNonZeroIDs("ExcludeProviderID", opts.ExcludeProviderIDs...); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if s.resolver == nil {
		return nil, fmt.Errorf("%s: %w: upload resolver not configured", op, ErrUninitialized)
	}
	opts = cloneUploadOptions(opts)
	if s.source != "" {
		opts = s.withSourceMetadata(opts)
	}
	opts = s.resolveWithCDN(opts)
	contexts, explicitProviders, err := resolveUploadContextsForUpload(ctx, s.resolver, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: resolve contexts: %w", op, err)
	}
	if explicitProviders {
		return nil, fmt.Errorf("%s: %w: automatic resolver returned explicit targets", op, ErrInvalidArgument)
	}
	if err := s.validateStorageContexts(op, contexts); err != nil {
		return nil, err
	}
	if len(contexts) > opts.Copies {
		return nil, fmt.Errorf("%s: %w: resolver returned %d contexts for %d copies", op, ErrInvalidArgument, len(contexts), opts.Copies)
	}
	if err := validateExcludedStorageContexts(op, contexts, opts.ExcludeProviderIDs); err != nil {
		return nil, err
	}
	if err := s.validateUploadContextsWritable(ctx, contexts); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if r == nil {
		return nil, fmt.Errorf("%s: %w: nil reader", op, ErrInvalidArgument)
	}
	return s.uploadWithContexts(ctx, r, contexts, opts, opts.Copies, true)
}

// UploadToContexts uploads to the exact contexts supplied by the caller. The
// first context is primary; later contexts receive provider-to-provider pulls.
func (s *Service) UploadToContexts(ctx context.Context, r io.Reader, contexts []StorageContext, opts *UploadOptions) (*UploadResult, error) {
	const op = "storage.Service.UploadToContexts"
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := validateExplicitUploadOptions(opts); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	contexts = append([]StorageContext(nil), contexts...)
	if err := s.validateStorageContexts(op, contexts); err != nil {
		return nil, err
	}
	if err := s.validateUploadContextsWritable(ctx, contexts); err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if r == nil {
		return nil, fmt.Errorf("%s: %w: nil reader", op, ErrInvalidArgument)
	}
	return s.uploadWithContexts(ctx, r, contexts, cloneUploadOptions(opts), len(contexts), false)
}

func (s *Service) uploadWithContexts(ctx context.Context, r io.Reader, contexts []StorageContext, opts *UploadOptions, requestedCopies int, allowReplacement bool) (*UploadResult, error) {
	opts = newUploadCallbackGuard(s.logger).wrapUploadOptions(opts)
	explicitProviders := !allowReplacement

	primary := contexts[0]
	secondaries := contexts[1:]

	storeOpts := &StoreOptions{}
	if opts != nil {
		storeOpts.PieceCID = opts.PieceCID
		storeOpts.OnProgress = opts.OnProgress
	}
	storeResult, err := primary.Store(ctx, r, storeOpts)
	if err != nil {
		return nil, &StoreError{
			ProviderID: primary.ProviderID(),
			Endpoint:   primary.ServiceURL(),
			Cause:      err,
		}
	}

	if opts != nil && opts.OnStored != nil {
		opts.OnStored(primary.ProviderID(), storeResult.PieceCID)
	}

	pieceInputs := []PieceInput{{
		PieceCID:      storeResult.PieceCID,
		PieceMetadata: cloneMetadata(opts),
	}}

	usedProviders := make(map[string]types.BigInt, len(contexts))
	for _, c := range contexts {
		id := c.ProviderID()
		usedProviders[idconv.Key(id)] = id
	}

	type successfulSecondary struct {
		ctx       StorageContext
		extraData []byte
	}

	var (
		successfulSecondaries []successfulSecondary
		failedAttempts        []FailedAttempt
	)

	for _, secondary := range secondaries {
		current := secondary
		maxAttempts := s.maxSecondaryAttempts
		currentAttemptCounted := false
		for attemptsUsed := 0; attemptsUsed < maxAttempts || currentAttemptCounted; {
			if !currentAttemptCounted {
				attemptsUsed++
			}
			currentAttemptCounted = false
			extraData, presignErr := current.PresignForCommit(ctx, pieceInputs)
			if presignErr == nil {
				var onProgress func(cid.Cid, PullStatus)
				if opts != nil && opts.OnPullProgress != nil {
					pullProviderID := current.ProviderID()
					onProgress = func(pieceCID cid.Cid, status PullStatus) {
						opts.OnPullProgress(pullProviderID, pieceCID, status)
					}
				}
				pullResult, pullErr := current.Pull(ctx, PullRequest{
					Pieces:     []cid.Cid{storeResult.PieceCID},
					From:       primary.PieceURL,
					ExtraData:  extraData,
					OnProgress: onProgress,
				})
				if pullErr == nil && pullResult != nil && pullResult.Status == PullStatusComplete {
					if opts != nil && opts.OnCopyComplete != nil {
						opts.OnCopyComplete(current.ProviderID(), storeResult.PieceCID)
					}
					successfulSecondaries = append(successfulSecondaries, successfulSecondary{
						ctx:       current,
						extraData: append([]byte(nil), extraData...),
					})
					break
				}
				if pullErr == nil {
					if pullResult == nil {
						pullErr = errors.New("pull returned nil result")
					} else {
						pullErr = fmt.Errorf("pull status %s", pullResult.Status)
					}
				}
				if opts != nil && opts.OnCopyFailed != nil {
					opts.OnCopyFailed(current.ProviderID(), storeResult.PieceCID, pullErr)
				}
				failedAttempts = append(failedAttempts, FailedAttempt{
					ProviderID: current.ProviderID(),
					Role:       CopyRoleSecondary,
					Stage:      CopyStagePull,
					Err:        pullErr,
					Explicit:   explicitProviders,
				})
			} else {
				failedAttempts = append(failedAttempts, FailedAttempt{
					ProviderID: current.ProviderID(),
					Role:       CopyRoleSecondary,
					Stage:      CopyStagePresign,
					Err:        presignErr,
					Explicit:   explicitProviders,
				})
			}

			if explicitProviders || attemptsUsed >= maxAttempts {
				break
			}
			foundReplacement := false
			for attemptsUsed < maxAttempts {
				replacement, replErr := selectReplacementForUpload(ctx, s.resolver, usedProviders, opts)
				if replErr != nil {
					break
				}
				id := replacement.ProviderID()
				usedProviders[idconv.Key(id)] = id
				attemptsUsed++
				if err := s.validateUploadContextsWritable(ctx, []StorageContext{replacement}); err != nil {
					failedAttempts = append(failedAttempts, FailedAttempt{
						ProviderID: replacement.ProviderID(),
						Role:       CopyRoleSecondary,
						Stage:      CopyStagePresign,
						Err:        err,
						Explicit:   explicitProviders,
					})
					continue
				}
				current = replacement
				currentAttemptCounted = true
				foundReplacement = true
				break
			}
			if !foundReplacement {
				break
			}
		}
	}

	type commitTarget struct {
		ctx       StorageContext
		role      CopyRole
		extraData []byte
	}
	type commitOutcome struct {
		result *CommitResult
		err    error
	}

	targets := make([]commitTarget, 0, 1+len(successfulSecondaries))
	targets = append(targets, commitTarget{ctx: primary, role: CopyRolePrimary})
	for _, secondary := range successfulSecondaries {
		targets = append(targets, commitTarget{
			ctx:       secondary.ctx,
			role:      CopyRoleSecondary,
			extraData: secondary.extraData,
		})
	}

	outcomes := make([]commitOutcome, len(targets))
	concurrency := s.commitConcurrency
	if concurrency <= 0 {
		concurrency = commitConcurrencyDefault
	}
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				outcomes[idx].err = ctx.Err()
				return
			}
			defer func() { <-sem }()
			if err := ctx.Err(); err != nil {
				outcomes[idx].err = err
				return
			}
			var onSubmitted func(string)
			if opts != nil && opts.OnPiecesAdded != nil {
				commitProviderID := targets[idx].ctx.ProviderID()
				commitPieceCID := storeResult.PieceCID
				onSubmitted = func(txHash string) {
					opts.OnPiecesAdded(txHash, commitProviderID, []SubmittedPiece{{PieceCID: commitPieceCID}})
				}
			}
			outcomes[idx].result, outcomes[idx].err = targets[idx].ctx.Commit(ctx, CommitRequest{
				Pieces:      pieceInputs,
				ExtraData:   targets[idx].extraData,
				OnSubmitted: onSubmitted,
			})
		}(i)
	}
	wg.Wait()

	copies := make([]CopyResult, 0, len(targets))
	var primaryCommitErr error
	for i, target := range targets {
		outcome := outcomes[i]
		if outcome.err != nil {
			if target.role == CopyRolePrimary {
				primaryCommitErr = outcome.err
			}
			failedAttempts = append(failedAttempts, FailedAttempt{
				ProviderID: target.ctx.ProviderID(),
				Role:       target.role,
				Stage:      CopyStageCommit,
				Err:        outcome.err,
				Explicit:   explicitProviders,
			})
			continue
		}
		if outcome.result == nil || outcome.result.DataSetID.IsZero() {
			var err error
			switch {
			case outcome.result == nil:
				err = errors.New("commit result missing confirmed identifiers: nil result")
			case outcome.result.DataSetID.IsZero():
				err = errors.New("commit result missing confirmed identifiers: zero dataSetID")
			}
			if target.role == CopyRolePrimary {
				primaryCommitErr = err
			}
			failedAttempts = append(failedAttempts, FailedAttempt{
				ProviderID: target.ctx.ProviderID(),
				Role:       target.role,
				Stage:      CopyStageCommit,
				Err:        err,
				Explicit:   explicitProviders,
			})
			continue
		}
		if err := validateConfirmedPieceIDs(outcome.result.PieceIDs, len(pieceInputs)); err != nil {
			if target.role == CopyRolePrimary {
				primaryCommitErr = err
			}
			failedAttempts = append(failedAttempts, FailedAttempt{
				ProviderID: target.ctx.ProviderID(),
				Role:       target.role,
				Stage:      CopyStageCommit,
				Err:        err,
				Explicit:   explicitProviders,
			})
			continue
		}

		if opts != nil && opts.OnPiecesConfirmed != nil {
			confirmed := make([]ConfirmedPiece, len(outcome.result.PieceIDs))
			for j, id := range outcome.result.PieceIDs {
				confirmed[j] = ConfirmedPiece{PieceID: id, PieceCID: storeResult.PieceCID}
			}
			opts.OnPiecesConfirmed(outcome.result.DataSetID, target.ctx.ProviderID(), confirmed)
		}
		copies = append(copies, CopyResult{
			ProviderID:   target.ctx.ProviderID(),
			DataSetID:    outcome.result.DataSetID,
			PieceID:      outcome.result.PieceIDs[0],
			Role:         target.role,
			RetrievalURL: target.ctx.PieceURL(storeResult.PieceCID),
			IsNewDataSet: outcome.result.IsNewDataSet,
		})
	}

	if len(copies) == 0 {
		return nil, &CommitError{
			ProviderID: primary.ProviderID(),
			Endpoint:   primary.ServiceURL(),
			Cause:      primaryCommitErr,
		}
	}

	return &UploadResult{
		PieceCID:        storeResult.PieceCID,
		Size:            storeResult.Size,
		RequestedCopies: requestedCopies,
		Complete:        len(copies) >= requestedCopies,
		Copies:          copies,
		FailedAttempts:  failedAttempts,
	}, nil
}

func resolveUploadContextsForUpload(ctx context.Context, resolver UploadResolver, opts *UploadOptions) ([]StorageContext, bool, error) {
	if writable, ok := resolver.(writableUploadResolver); ok {
		return writable.resolveWritableUploadContexts(ctx, opts)
	}
	return resolver.ResolveUploadContexts(ctx, opts)
}

func selectReplacementForUpload(ctx context.Context, resolver UploadResolver, usedProviders map[string]types.BigInt, opts *UploadOptions) (StorageContext, error) {
	if writable, ok := resolver.(writableUploadResolver); ok {
		return writable.selectWritableReplacement(ctx, usedProviders, opts)
	}
	return resolver.SelectReplacement(ctx, usedProviders, opts)
}

func (s *Service) validateUploadContextsWritable(ctx context.Context, contexts []StorageContext) error {
	if s.dsReader == nil {
		return nil
	}
	for _, uploadCtx := range contexts {
		ref, ok := uploadCtx.DataSetRef()
		if !ok {
			continue
		}
		dataSetID := ref.DataSetID()
		info, err := s.dsReader.GetDataSet(ctx, dataSetID)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fmt.Errorf("validate data set %s: %w", dataSetID.String(), ctxErr)
			}
			return fmt.Errorf("validate data set %s: %w", dataSetID.String(), err)
		}
		if info == nil {
			return fmt.Errorf("%w: FWSS returned no data set for dataSetID %s", ErrInvalidArgument, dataSetID.String())
		}
		if err := validateDataSetAcceptsUploads(dataSetID, info.PDPEndEpoch); err != nil {
			return err
		}
	}
	return nil
}

func cloneMetadata(opts *UploadOptions) map[string]string {
	if opts == nil || len(opts.PieceMetadata) == 0 {
		return nil
	}
	out := make(map[string]string, len(opts.PieceMetadata))
	maps.Copy(out, opts.PieceMetadata)
	return out
}

// withSourceMetadata returns a shallow clone of opts with the manager-level
// "source" key injected into DataSetMetadata, unless the caller already set it.
func (s *Service) withSourceMetadata(opts *UploadOptions) *UploadOptions {
	if opts == nil {
		return &UploadOptions{
			DataSetMetadata: map[string]string{"source": s.source},
		}
	}
	if _, ok := opts.DataSetMetadata["source"]; ok {
		return opts // caller override wins
	}
	cloned := *opts
	cloned.DataSetMetadata = make(map[string]string, len(opts.DataSetMetadata)+1)
	maps.Copy(cloned.DataSetMetadata, opts.DataSetMetadata)
	cloned.DataSetMetadata["source"] = s.source
	return &cloned
}

// resolveWithCDN returns a shallow clone of opts with WithCDN resolved to a
// non-nil *bool: caller-provided value wins, otherwise the manager-level
// [Options.DefaultWithCDN] default is used. A nil opts is lifted to an empty
// UploadOptions so downstream code can assume opts != nil when CDN is
// relevant.
func (s *Service) resolveWithCDN(opts *UploadOptions) *UploadOptions {
	if opts == nil {
		b := s.defaultWithCDN
		return &UploadOptions{WithCDN: &b}
	}
	if opts.WithCDN != nil {
		return opts
	}
	cloned := *opts
	b := s.defaultWithCDN
	cloned.WithCDN = &b
	return &cloned
}

func validateExplicitUploadOptions(opts *UploadOptions) error {
	if opts == nil {
		return nil
	}
	if opts.Copies != 0 {
		return fmt.Errorf("%w: Copies is not supported for explicit-context uploads", ErrInvalidArgument)
	}
	if len(opts.ExcludeProviderIDs) != 0 {
		return fmt.Errorf("%w: ExcludeProviderIDs is not supported for explicit-context uploads; pass it to SelectUploadContexts instead", ErrInvalidArgument)
	}
	if len(opts.DataSetMetadata) != 0 {
		return fmt.Errorf("%w: DataSetMetadata is not supported for explicit-context uploads; pass it to SelectUploadContexts or the context constructor instead", ErrInvalidArgument)
	}
	if opts.WithCDN != nil {
		return fmt.Errorf("%w: WithCDN is not supported for explicit-context uploads; pass it to SelectUploadContexts or the context constructor instead", ErrInvalidArgument)
	}
	return nil
}

func validateContextUploadOptions(opts *UploadOptions) error {
	if err := validateExplicitUploadOptions(opts); err != nil {
		return err
	}
	if opts == nil {
		return nil
	}
	if opts.OnCopyComplete != nil || opts.OnCopyFailed != nil || opts.OnPullProgress != nil {
		return fmt.Errorf("%w: secondary-copy callbacks are not supported by context Upload", ErrInvalidArgument)
	}
	return nil
}

func cloneUploadOptions(opts *UploadOptions) *UploadOptions {
	if opts == nil {
		return nil
	}
	out := *opts
	out.PieceMetadata = cloneStringMap(opts.PieceMetadata)
	out.DataSetMetadata = cloneStringMap(opts.DataSetMetadata)
	out.ExcludeProviderIDs = cloneBigIntSlice(opts.ExcludeProviderIDs)
	out.WithCDN = copyBoolPtr(opts.WithCDN)
	return &out
}

func validateDataSetAcceptsUploads(dataSetID types.BigInt, pdpEndEpoch types.Epoch) error {
	if pdpEndEpoch == 0 {
		return nil
	}
	return &DataSetPDPPaymentTerminatedError{
		DataSetID:   dataSetID.Copy(),
		PDPEndEpoch: pdpEndEpoch,
	}
}

func validateConfirmedPieceIDs(ids []types.BigInt, want int) error {
	if len(ids) != want {
		return fmt.Errorf("commit result missing confirmed identifiers: got %d pieceIDs want %d", len(ids), want)
	}
	return nil
}
