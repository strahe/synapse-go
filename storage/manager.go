package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
)

const maxSecondaryAttemptsDefault = 5

// defaultDownloadTimeout is applied to the Manager's HTTP client for
// URL-based downloads.  It is long enough for multi-GiB files transferred
// over a typical storage network while preventing indefinite hangs.
const defaultDownloadTimeout = 24 * time.Hour

// UploadContext abstracts a single provider's upload operations.
// Implementations are returned by UploadResolver and are safe for concurrent use.
type UploadContext interface {
	ProviderID() *big.Int
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
	SelectReplacement(context.Context, map[string]struct{}, *UploadOptions) (UploadContext, error)
}

// Manager orchestrates multi-copy uploads and downloads.
// Create with NewManager; configure via Option functions.
type Manager struct {
	resolver             UploadResolver
	httpClient           *http.Client
	source               string
	maxSecondaryAttempts int
}

// Option configures a Manager.
type Option func(*Manager)

// WithUploadResolver sets the resolver used to select providers for each upload.
func WithUploadResolver(resolver UploadResolver) Option {
	return func(m *Manager) {
		m.resolver = resolver
	}
}

// WithHTTPClient sets the HTTP client used for URL-based downloads.
// Use this to inject custom timeouts, proxies, or transports.
// If not provided, a client with defaultDownloadTimeout is used.
func WithHTTPClient(c *http.Client) Option {
	return func(m *Manager) {
		m.httpClient = c
	}
}

// WithSource sets the application source identifier for dataset namespace
// isolation. Datasets with different source values are treated as separate
// namespaces; reuse only occurs within the same source.
func WithSource(s string) Option {
	return func(m *Manager) {
		m.source = s
	}
}

// WithMaxSecondaryAttempts sets the maximum number of provider candidates tried
// for each secondary copy slot before giving up. Values <= 0 are ignored and
// the default of 5 is used.
func WithMaxSecondaryAttempts(n int) Option {
	return func(m *Manager) {
		if n > 0 {
			m.maxSecondaryAttempts = n
		}
	}
}

// NewManager creates a Manager with the given options.
// A default HTTP client with a 24-hour timeout is used unless overridden by WithHTTPClient.
func NewManager(opts ...Option) *Manager {
	m := &Manager{
		httpClient:           &http.Client{Timeout: defaultDownloadTimeout},
		maxSecondaryAttempts: maxSecondaryAttemptsDefault,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

// Upload runs the multi-copy upload pipeline streaming from r in a single
// pass. opts may be nil (defaults apply). Returns UploadResult whose
// Complete field indicates whether all requested copies were committed
// on-chain.
//
// The reader is consumed once by the primary provider; secondary copies
// are populated via server-to-server Pulls. On success the reader is
// fully drained; on error it may be only partially consumed.
func (m *Manager) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	if r == nil {
		return nil, errors.New("storage.Manager.Upload: nil reader")
	}
	if m.resolver == nil {
		return nil, errors.New("storage.Manager.Upload: no upload resolver configured")
	}

	// Inject manager-level source into dataset metadata if set and not
	// already overridden by the caller.
	if m.source != "" {
		opts = m.withSourceMetadata(opts)
	}

	// Capture the caller's intent before resolving (the resolver may return
	// fewer contexts than requested; Complete must reflect the shortfall).
	requestedCopies := requestedCopiesForUpload(opts)

	contexts, explicitProviders, err := m.resolver.ResolveUploadContexts(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("storage.Manager.Upload: resolve contexts: %w", err)
	}
	if len(contexts) == 0 {
		return nil, errors.New("storage.Manager.Upload: no upload contexts available")
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

	usedProviders := make(map[string]struct{}, len(contexts))
	for _, c := range contexts {
		usedProviders[c.ProviderID().String()] = struct{}{}
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
		maxAttempts := m.maxSecondaryAttempts
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
			replacement, replErr := m.resolver.SelectReplacement(ctx, usedProviders, opts)
			if replErr != nil {
				break
			}
			current = replacement
			usedProviders[current.ProviderID().String()] = struct{}{}
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
		if outcome.result == nil || outcome.result.DataSetID == nil || len(outcome.result.PieceIDs) == 0 || outcome.result.PieceIDs[0] == nil {
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
			DataSetID:    new(big.Int).Set(outcome.result.DataSetID),
			PieceID:      new(big.Int).Set(outcome.result.PieceIDs[0]),
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
		return len(dedupeBigInts(opts.DataSetIDs))
	}
	if len(opts.ProviderIDs) > 0 {
		return len(dedupeBigInts(opts.ProviderIDs))
	}
	return 2
}

// withSourceMetadata returns a shallow clone of opts with the manager-level
// "source" key injected into DataSetMetadata, unless the caller already set it.
func (m *Manager) withSourceMetadata(opts *UploadOptions) *UploadOptions {
	if opts == nil {
		return &UploadOptions{
			DataSetMetadata: map[string]string{"source": m.source},
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
	cloned.DataSetMetadata["source"] = m.source
	return &cloned
}
