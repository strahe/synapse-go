package storage

import (
	"context"
	"fmt"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/types"
)

// CreateContextsOptions configures Service.CreateContexts.
//
// Copies controls how many provider copies to create. When zero, the
// resolver uses its default (two copies when no explicit providers or
// datasets are pinned; otherwise len(ProviderIDs) / len(DataSetIDs)).
//
// WithCDN is tri-state: nil means inherit the Client-level default
// configured via [synapse.WithCDN]; non-nil explicitly overrides for this
// call. Declare a local variable to take its address:
//
//	b := true
//	opts := &storage.CreateContextsOptions{WithCDN: &b}
//
// [synapse.WithCDN]: https://pkg.go.dev/github.com/strahe/synapse-go#WithCDN
type CreateContextsOptions struct {
	Copies             int
	ProviderIDs        []types.BigInt
	DataSetIDs         []types.BigInt
	ExcludeProviderIDs []types.BigInt
	DataSetMetadata    map[string]string
	WithCDN            *bool
}

// CreateContextOptions configures Service.CreateContext — the
// single-copy variant. Same knobs as CreateContextsOptions minus Copies.
//
// WithCDN follows the same tri-state convention as
// [CreateContextsOptions.WithCDN].
//
// [CreateContextsOptions.WithCDN]: https://pkg.go.dev/github.com/strahe/synapse-go/storage#CreateContextsOptions.WithCDN
type CreateContextOptions struct {
	ProviderIDs        []types.BigInt
	DataSetIDs         []types.BigInt
	ExcludeProviderIDs []types.BigInt
	DataSetMetadata    map[string]string
	WithCDN            *bool
}

// toUploadOptions maps CreateContextsOptions onto the resolver's
// internal UploadOptions. PieceMetadata, PieceCID, and upload lifecycle
// callbacks are irrelevant at context-creation time and left unset.
func (o *CreateContextsOptions) toUploadOptions() *UploadOptions {
	if o == nil {
		return &UploadOptions{}
	}
	return &UploadOptions{
		Copies:             o.Copies,
		ProviderIDs:        o.ProviderIDs,
		DataSetIDs:         o.DataSetIDs,
		ExcludeProviderIDs: o.ExcludeProviderIDs,
		DataSetMetadata:    o.DataSetMetadata,
		WithCDN:            o.WithCDN,
	}
}

func (o *CreateContextOptions) toUploadOptions() *UploadOptions {
	if o == nil {
		return &UploadOptions{Copies: 1}
	}
	return &UploadOptions{
		Copies:             1,
		ProviderIDs:        o.ProviderIDs,
		DataSetIDs:         o.DataSetIDs,
		ExcludeProviderIDs: o.ExcludeProviderIDs,
		DataSetMetadata:    o.DataSetMetadata,
		WithCDN:            o.WithCDN,
	}
}

// CreateContexts provisions one or more concrete storage contexts without
// uploading. It picks providers, reuses or creates data sets according to
// opts, and returns the resulting contexts. When opts is nil or opts.Copies
// is zero the resolver default (two copies in auto-select) applies.
func (s *Service) CreateContexts(ctx context.Context, opts *CreateContextsOptions) ([]*Context, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.resolver == nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w: resolver not configured", ErrUninitialized)
	}
	uploadOpts := opts.toUploadOptions()
	if s.source != "" {
		uploadOpts = s.withSourceMetadata(uploadOpts)
	}
	uploadOpts = s.resolveWithCDN(uploadOpts)
	contexts, _, err := s.resolver.ResolveUploadContexts(ctx, uploadOpts)
	if err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w", err)
	}
	out := make([]*Context, len(contexts))
	for i, ctx := range contexts {
		concrete, ok := ctx.(*Context)
		if !ok {
			return nil, fmt.Errorf("storage.Service.CreateContexts: %w: resolver returned non-*Context value", ErrInvalidArgument)
		}
		out[i] = concrete
	}
	if err := s.populateClientDataSetIDs(ctx, out); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w", err)
	}
	return out, nil
}

// CreateContext is the single-copy convenience wrapper around CreateContexts.
func (s *Service) CreateContext(ctx context.Context, opts *CreateContextOptions) (*Context, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.resolver == nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w: resolver not configured", ErrUninitialized)
	}
	uploadOpts := opts.toUploadOptions()
	if s.source != "" {
		uploadOpts = s.withSourceMetadata(uploadOpts)
	}
	uploadOpts = s.resolveWithCDN(uploadOpts)
	contexts, _, err := s.resolver.ResolveUploadContexts(ctx, uploadOpts)
	if err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w", err)
	}
	if len(contexts) == 0 {
		return nil, fmt.Errorf("storage.Service.CreateContext: resolver returned no contexts")
	}
	concrete, ok := contexts[0].(*Context)
	if !ok {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w: resolver returned non-*Context value", ErrInvalidArgument)
	}
	if err := s.populateClientDataSetIDs(ctx, []*Context{concrete}); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w", err)
	}
	return concrete, nil
}

// populateClientDataSetIDs is the F-48b safety net: for each resolved
// Context bound to an existing on-chain dataSetID but missing
// clientDataSetID (the resolver path normally populates it from the
// FWSS dataset record, but a nil result on certain code paths is
// possible), fetch the canonical value via FWSSDataSetReader and
// inject it. Skipped silently when no FWSSDataSetReader is configured.
//
// Errors are surfaced unwrapped so callers can route transient
// failures (context.Canceled, RPC timeouts, contract reverts) without
// misclassifying them as ErrInvalidArgument. Only the genuinely-empty
// FWSS result (info == nil) is wrapped as ErrInvalidArgument because
// that indicates the dataSetID does not resolve to a valid record.
func (s *Service) populateClientDataSetIDs(ctx context.Context, contexts []*Context) error {
	if s.dsReader == nil {
		return nil
	}
	cache := make(map[string]types.BigInt)
	for _, c := range contexts {
		if c == nil {
			continue
		}
		c.mu.Lock()
		needsFetch := c.dataSetID != nil && c.clientDataSetID == nil
		var dsID types.BigInt
		if needsFetch {
			dsID = *c.dataSetID
		}
		c.mu.Unlock()
		if !needsFetch {
			continue
		}
		key := idconv.Key(dsID)
		if cachedID, ok := cache[key]; ok {
			c.mu.Lock()
			if c.clientDataSetID == nil {
				c.clientDataSetID = copyClientDataSetIDPtr(cachedID)
			}
			c.mu.Unlock()
			continue
		}
		info, err := s.dsReader.GetDataSet(ctx, dsID)
		if err != nil {
			return fmt.Errorf("fetch ClientDataSetID for dataSetID %s: %w", dsID.String(), err)
		}
		if info == nil {
			return fmt.Errorf("%w: FWSS returned no ClientDataSetID for dataSetID %s", ErrInvalidArgument, dsID.String())
		}
		cache[key] = copyClientDataSetID(info.ClientDataSetID)
		c.mu.Lock()
		// Re-check under lock in case a concurrent setter populated it
		// between the unlocked read above and this point.
		if c.clientDataSetID == nil {
			c.clientDataSetID = copyClientDataSetIDPtr(cache[key])
		}
		c.mu.Unlock()
	}
	return nil
}

// populateClientDataSetIDsFromInterfaces is the Service.Upload-side
// counterpart of populateClientDataSetIDs: the resolver returns a
// []UploadContext interface slice, so non-*Context implementations
// are skipped silently (the safety net only applies to the SDK's
// concrete Context type).
func (s *Service) populateClientDataSetIDsFromInterfaces(ctx context.Context, contexts []UploadContext) error {
	if s.dsReader == nil {
		return nil
	}
	concretes := make([]*Context, 0, len(contexts))
	for _, uc := range contexts {
		if c, ok := uc.(*Context); ok {
			concretes = append(concretes, c)
		}
	}
	if len(concretes) == 0 {
		return nil
	}
	return s.populateClientDataSetIDs(ctx, concretes)
}

// GetDefaultContext returns a single auto-selected context using resolver
// defaults.
func (s *Service) GetDefaultContext(ctx context.Context) (*Context, error) {
	return s.CreateContext(ctx, nil)
}
