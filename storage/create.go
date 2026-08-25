package storage

import (
	"context"
	"fmt"

	"github.com/strahe/synapse-go/types"
)

// CreateContextsOptions configures Service.CreateContexts.
//
// Copies controls how many provider copies to create. When zero, the resolver
// uses the number of unique DataSetIDs or ProviderIDs when either list is set;
// otherwise it uses two copies.
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
	ProviderIDs        []types.BigInt // mutually exclusive with DataSetIDs
	DataSetIDs         []types.BigInt // mutually exclusive with ProviderIDs
	ExcludeProviderIDs []types.BigInt // only used when providers are auto-selected
	DataSetMetadata    map[string]string
	WithCDN            *bool
}

// CreateContextOptions configures Service.CreateContext.
//
// ProviderID pins the context to one provider. DataSetID pins it to one
// existing data set. When both are set, DataSetID selects the context and
// ProviderID asserts that the selected data set belongs to that provider.
//
// WithCDN follows the same tri-state convention as
// [CreateContextsOptions.WithCDN].
//
// [CreateContextsOptions.WithCDN]: https://pkg.go.dev/github.com/strahe/synapse-go/storage#CreateContextsOptions.WithCDN
type CreateContextOptions struct {
	ProviderID         *types.BigInt
	DataSetID          *types.BigInt
	ExcludeProviderIDs []types.BigInt // only used when no ProviderID or DataSetID is set
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
		ProviderIDs:        cloneBigIntSlice(o.ProviderIDs),
		DataSetIDs:         cloneBigIntSlice(o.DataSetIDs),
		ExcludeProviderIDs: cloneBigIntSlice(o.ExcludeProviderIDs),
		DataSetMetadata:    cloneStringMap(o.DataSetMetadata),
		WithCDN:            o.WithCDN,
	}
}

func (o *CreateContextOptions) toUploadOptions() *UploadOptions {
	if o == nil {
		return &UploadOptions{Copies: 1}
	}
	out := &UploadOptions{
		Copies:             1,
		ExcludeProviderIDs: cloneBigIntSlice(o.ExcludeProviderIDs),
		DataSetMetadata:    cloneStringMap(o.DataSetMetadata),
		WithCDN:            o.WithCDN,
	}
	if o.DataSetID != nil {
		out.DataSetIDs = []types.BigInt{copyBigInt(*o.DataSetID)}
	} else if o.ProviderID != nil {
		out.ProviderIDs = []types.BigInt{copyBigInt(*o.ProviderID)}
	}
	return out
}

// CreateContexts provisions one or more concrete storage contexts without
// uploading. It picks providers, reuses or creates data sets according to
// opts, and returns the resulting contexts. When opts is nil or opts.Copies
// is zero the context resolver default (two copies in auto-select) applies.
func (s *Service) CreateContexts(ctx context.Context, opts *CreateContextsOptions) ([]UploadContext, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.contextResolver == nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w: context resolver not configured", ErrUninitialized)
	}
	if err := validateCreateContextsOptions(opts); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w", err)
	}
	uploadOpts := opts.toUploadOptions()
	if s.source != "" {
		uploadOpts = s.withSourceMetadata(uploadOpts)
	}
	uploadOpts = s.resolveWithCDN(uploadOpts)
	contexts, err := s.contextResolver.ResolveContexts(ctx, uploadOpts)
	if err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w", err)
	}
	if err := validateResolvedContexts(contexts); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w", err)
	}
	return contexts, nil
}

// CreateContext is the single-copy convenience wrapper around CreateContexts.
func (s *Service) CreateContext(ctx context.Context, opts *CreateContextOptions) (UploadContext, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.contextResolver == nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w: context resolver not configured", ErrUninitialized)
	}
	if err := validateCreateContextOptions(opts); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w", err)
	}
	uploadOpts := opts.toUploadOptions()
	if s.source != "" {
		uploadOpts = s.withSourceMetadata(uploadOpts)
	}
	uploadOpts = s.resolveWithCDN(uploadOpts)
	contexts, err := s.contextResolver.ResolveContexts(ctx, uploadOpts)
	if err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w", err)
	}
	if len(contexts) == 0 {
		return nil, fmt.Errorf("storage.Service.CreateContext: resolver returned no contexts")
	}
	concrete := contexts[0]
	if concrete == nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: resolver returned nil context")
	}
	if opts != nil && opts.ProviderID != nil && opts.DataSetID != nil {
		gotProviderID := concrete.ProviderID()
		if !gotProviderID.Equal(*opts.ProviderID) {
			return nil, fmt.Errorf(
				"storage.Service.CreateContext: %w: DataSetID %s belongs to ProviderID %s, but ProviderID %s was requested",
				ErrInvalidArgument,
				opts.DataSetID.String(),
				gotProviderID.String(),
				opts.ProviderID.String(),
			)
		}
	}
	return concrete, nil
}

func validateCreateContextsOptions(opts *CreateContextsOptions) error {
	if opts == nil {
		return nil
	}
	if err := validateProviderAndDataSetIDs(opts.ProviderIDs, opts.DataSetIDs); err != nil {
		return err
	}
	if err := validateNonZeroIDs("ProviderID", opts.ProviderIDs...); err != nil {
		return err
	}
	if err := validateNonZeroIDs("DataSetID", opts.DataSetIDs...); err != nil {
		return err
	}
	if err := validateNonZeroIDs("ExcludeProviderID", opts.ExcludeProviderIDs...); err != nil {
		return err
	}
	return nil
}

func validateCreateContextOptions(opts *CreateContextOptions) error {
	if opts == nil {
		return nil
	}
	if opts.ProviderID != nil {
		if err := validateNonZeroIDs("ProviderID", *opts.ProviderID); err != nil {
			return err
		}
	}
	if opts.DataSetID != nil {
		if err := validateNonZeroIDs("DataSetID", *opts.DataSetID); err != nil {
			return err
		}
	}
	if err := validateNonZeroIDs("ExcludeProviderID", opts.ExcludeProviderIDs...); err != nil {
		return err
	}
	return nil
}

func validateResolvedContexts(contexts []UploadContext) error {
	for i, ctx := range contexts {
		if ctx == nil {
			return fmt.Errorf("resolver returned nil context at index %d", i)
		}
	}
	return nil
}

func validateNonZeroIDs(name string, ids ...types.BigInt) error {
	for _, id := range ids {
		if id.IsZero() {
			return fmt.Errorf("%w: zero %s", ErrInvalidArgument, name)
		}
	}
	return nil
}

func cloneBigIntSlice(ids []types.BigInt) []types.BigInt {
	if len(ids) == 0 {
		return nil
	}
	out := make([]types.BigInt, len(ids))
	for i, id := range ids {
		out[i] = copyBigInt(id)
	}
	return out
}

// GetDefaultContext returns a single auto-selected context using resolver
// defaults.
func (s *Service) GetDefaultContext(ctx context.Context) (UploadContext, error) {
	return s.CreateContext(ctx, nil)
}
