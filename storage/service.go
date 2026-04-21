package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
)

const maxSecondaryAttemptsDefault = 5

// defaultDownloadTimeout is applied to the Service's HTTP client for
// URL-based downloads.  It is long enough for multi-GiB files transferred
// over a typical storage network while preventing indefinite hangs.
const defaultDownloadTimeout = 24 * time.Hour

// UploadContext abstracts a single provider's upload operations.
// Implementations are returned by UploadResolver and are safe for concurrent use.
type UploadContext interface {
	ProviderID() types.ProviderID
	ServiceURL() string
	PieceURL(cid.Cid) string
	Store(context.Context, io.Reader, *StoreOptions) (*StoreResult, error)
	PresignForCommit(context.Context, []PieceInput) ([]byte, error)
	Pull(context.Context, PullRequest) (*PullResult, error)
	Commit(context.Context, CommitRequest) (*CommitResult, error)
}

// UploadResolver selects the set of providers for an upload and provides
// replacement candidates when a secondary provider fails.
type UploadResolver interface {
	ResolveUploadContexts(context.Context, *UploadOptions) ([]UploadContext, bool, error)
	SelectReplacement(context.Context, map[types.ProviderID]struct{}, *UploadOptions) (UploadContext, error)
}

// Service orchestrates multi-copy uploads and downloads.
// Create with New; configure via the [Options] struct.
type Service struct {
	resolver             UploadResolver
	httpClient           *http.Client
	source               string
	maxSecondaryAttempts int
}

// Options configures a Service. Unset fields fall back to sensible defaults.
type Options struct {
	// Resolver selects providers for each upload and supplies replacement
	// candidates when a secondary provider fails. A nil resolver is allowed
	// so the Service can still serve DownloadFromContext / download-by-URL
	// calls; Upload then returns a clean validation error.
	Resolver UploadResolver

	// HTTPClient is used for URL-based downloads. nil installs a client with
	// a 24-hour timeout — long enough for multi-GiB transfers over typical
	// storage networks while preventing indefinite hangs.
	HTTPClient *http.Client

	// Source is the application identifier for dataset namespace isolation.
	// Datasets with different Source values are treated as separate
	// namespaces; reuse only occurs within the same Source.
	Source string

	// MaxSecondaryAttempts caps the number of provider candidates tried for
	// each secondary copy slot before giving up. Values <= 0 select the
	// default of 5.
	MaxSecondaryAttempts int
}

// New creates a Service from the given Options.
// The returned error is always nil today; callers should still check it for
// forward compatibility.
func New(opts Options) (*Service, error) {
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: defaultDownloadTimeout}
	}
	if opts.MaxSecondaryAttempts <= 0 {
		opts.MaxSecondaryAttempts = maxSecondaryAttemptsDefault
	}
	return &Service{
		resolver:             opts.Resolver,
		httpClient:           opts.HTTPClient,
		source:               opts.Source,
		maxSecondaryAttempts: opts.MaxSecondaryAttempts,
	}, nil
}

// Upload runs the multi-copy upload pipeline streaming from r in a single
// pass. opts may be nil (defaults apply). Returns UploadResult whose
// Complete field indicates whether all requested copies were committed
// on-chain.
//
// The reader is consumed once by the primary provider; secondary copies
// are populated via server-to-server Pulls. On success the reader is
// fully drained; on error it may be only partially consumed.
func (s *Service) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	if r == nil {
		return nil, errors.New("storage.Service.Upload: nil reader")
	}
	if s.resolver == nil {
		return nil, errors.New("storage.Service.Upload: no upload resolver configured")
	}

	// Inject manager-level source into dataset metadata if set and not
	// already overridden by the caller.
	if s.source != "" {
		opts = s.withSourceMetadata(opts)
	}

	// Capture the caller's intent before resolving (the resolver may return
	// fewer contexts than requested; Complete must reflect the shortfall).
	requestedCopies := requestedCopiesForUpload(opts)

	contexts, explicitProviders, err := s.resolver.ResolveUploadContexts(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("storage.Service.Upload: resolve contexts: %w", err)
	}
	if len(contexts) == 0 {
		return nil, errors.New("storage.Service.Upload: no upload contexts available")
	}

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

	pieceInputs := []PieceInput{{
		PieceCID:      storeResult.PieceCID,
		PieceMetadata: cloneMetadata(opts),
	}}

	usedProviders := make(map[types.ProviderID]struct{}, len(contexts))
	for _, c := range contexts {
		usedProviders[c.ProviderID()] = struct{}{}
	}

	type successfulSecondary struct {
		ctx       UploadContext
		extraData []byte
	}

	var (
		successfulSecondaries []successfulSecondary
		failedAttempts        []FailedAttempt
	)

	for _, secondary := range secondaries {
		current := secondary
		maxAttempts := s.maxSecondaryAttempts
		for attempt := 0; attempt < maxAttempts; attempt++ {
			extraData, presignErr := current.PresignForCommit(ctx, pieceInputs)
			if presignErr == nil {
				pullResult, pullErr := current.Pull(ctx, PullRequest{
					Pieces:    []cid.Cid{storeResult.PieceCID},
					From:      primary.PieceURL,
					ExtraData: extraData,
				})
				if pullErr == nil && pullResult != nil && pullResult.Status == PullStatusComplete {
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

			if explicitProviders || attempt == maxAttempts-1 {
				break
			}
			replacement, replErr := s.resolver.SelectReplacement(ctx, usedProviders, opts)
			if replErr != nil {
				break
			}
			current = replacement
			usedProviders[current.ProviderID()] = struct{}{}
		}
	}

	type commitTarget struct {
		ctx       UploadContext
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
	var wg sync.WaitGroup
	for i := range targets {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			outcomes[idx].result, outcomes[idx].err = targets[idx].ctx.Commit(ctx, CommitRequest{
				Pieces:    pieceInputs,
				ExtraData: targets[idx].extraData,
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
		if outcome.result == nil || outcome.result.DataSetID == 0 || len(outcome.result.PieceIDs) == 0 || outcome.result.PieceIDs[0] == 0 {
			err := errors.New("commit result missing confirmed identifiers")
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

func cloneMetadata(opts *UploadOptions) map[string]string {
	if opts == nil || len(opts.PieceMetadata) == 0 {
		return nil
	}
	out := make(map[string]string, len(opts.PieceMetadata))
	for k, v := range opts.PieceMetadata {
		out[k] = v
	}
	return out
}

func requestedCopiesForUpload(opts *UploadOptions) int {
	if opts == nil {
		return 2
	}
	if opts.Copies > 0 {
		return opts.Copies
	}
	if len(opts.DataSetIDs) > 0 {
		return len(dedupeIDs(opts.DataSetIDs))
	}
	if len(opts.ProviderIDs) > 0 {
		return len(dedupeIDs(opts.ProviderIDs))
	}
	return 2
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
	for k, v := range opts.DataSetMetadata {
		cloned.DataSetMetadata[k] = v
	}
	cloned.DataSetMetadata["source"] = s.source
	return &cloned
}
