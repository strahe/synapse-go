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

const maxSecondaryAttempts = 5

// defaultDownloadTimeout is applied to the Manager's HTTP client for
// URL-based downloads.  It is long enough for multi-GiB files transferred
// over a typical storage network while preventing indefinite hangs.
const defaultDownloadTimeout = 24 * time.Hour

type UploadContext interface {
	ProviderID() *big.Int
	ServiceURL() string
	PieceURL(cid.Cid) string
	StoreBytes(context.Context, []byte, *StoreOptions) (*StoreResult, error)
	PresignForCommit(context.Context, []PieceInput) ([]byte, error)
	Pull(context.Context, PullRequest) (*PullResult, error)
	Commit(context.Context, CommitRequest) (*CommitResult, error)
}

type UploadResolver interface {
	ResolveUploadContexts(context.Context, *UploadOptions) ([]UploadContext, bool, error)
	SelectReplacement(context.Context, map[string]struct{}, *UploadOptions) (UploadContext, error)
}

type Manager struct {
	resolver   UploadResolver
	httpClient *http.Client
}

type Option func(*Manager)

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

func NewManager(opts ...Option) *Manager {
	m := &Manager{
		httpClient: &http.Client{Timeout: defaultDownloadTimeout},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	return m
}

// Upload reads r entirely into memory before uploading.  For large payloads,
// use UploadBytes after pre-reading the data yourself so you control buffering.
func (m *Manager) Upload(ctx context.Context, r io.Reader, opts *UploadOptions) (*UploadResult, error) {
	if r == nil {
		return nil, errors.New("storage.Manager.Upload: nil reader")
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("storage.Manager.Upload: read input: %w", err)
	}
	return m.UploadBytes(ctx, data, opts)
}

func (m *Manager) UploadBytes(ctx context.Context, data []byte, opts *UploadOptions) (*UploadResult, error) {
	if len(data) == 0 {
		return nil, errors.New("storage.Manager.UploadBytes: empty data")
	}
	if m.resolver == nil {
		return nil, errors.New("storage.Manager.UploadBytes: no upload resolver configured")
	}

	// Capture the caller's intent before resolving (the resolver may return
	// fewer contexts than requested; Complete must reflect the shortfall).
	requestedCopies := requestedCopiesForUpload(opts)

	contexts, explicitProviders, err := m.resolver.ResolveUploadContexts(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("storage.Manager.UploadBytes: resolve contexts: %w", err)
	}
	if len(contexts) == 0 {
		return nil, errors.New("storage.Manager.UploadBytes: no upload contexts available")
	}

	primary := contexts[0]
	secondaries := contexts[1:]

	storeResult, err := primary.StoreBytes(ctx, data, &StoreOptions{})
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
		for attempt := 0; attempt < maxSecondaryAttempts; attempt++ {
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

			if explicitProviders || attempt == maxSecondaryAttempts-1 {
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
