package storage

import (
	"context"
	"fmt"

	"github.com/strahe/synapse-go/types"
)

// CreateContextsOptions configures Service.CreateContexts and mirrors the
// TS CreateContextsOptions shape (synapse-sdk .../types.ts:331-394).
//
// Copies controls how many provider copies to create. When zero, the
// resolver uses its default (two copies when no explicit providers or
// datasets are pinned; otherwise len(ProviderIDs) / len(DataSetIDs)).
type CreateContextsOptions struct {
	Copies             int
	ProviderIDs        []types.ProviderID
	DataSetIDs         []types.DataSetID
	ExcludeProviderIDs []types.ProviderID
	DataSetMetadata    map[string]string
	WithCDN            bool
}

// CreateContextOptions configures Service.CreateContext — the
// single-copy variant. Same knobs as CreateContextsOptions minus Copies.
type CreateContextOptions struct {
	ProviderIDs        []types.ProviderID
	DataSetIDs         []types.DataSetID
	ExcludeProviderIDs []types.ProviderID
	DataSetMetadata    map[string]string
	WithCDN            bool
}

// toUploadOptions maps CreateContextsOptions onto the resolver's
// internal UploadOptions. PieceMetadata / PieceCID / OnProgress are
// irrelevant at context-creation time and left unset.
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
// uploading. Mirrors TS StorageManager.createContexts
// (synapse-sdk/.../manager.ts:843-957): picks providers / reuses or
// creates data sets according to opts and returns the resulting
// contexts. When opts is nil or opts.Copies is zero the resolver
// default (two copies in auto-select) applies.
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
	return out, nil
}

// CreateContext is the single-copy convenience wrapper around
// CreateContexts. Mirrors TS StorageManager.createContext.
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
	return concrete, nil
}

// GetDefaultContext returns a single auto-selected context using
// resolver defaults. Mirrors TS StorageManager.getDefaultContext.
func (s *Service) GetDefaultContext(ctx context.Context) (*Context, error) {
	return s.CreateContext(ctx, nil)
}
