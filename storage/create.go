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
// is zero the resolver default (two copies in auto-select) applies.
func (s *Service) CreateContexts(ctx context.Context, opts *CreateContextsOptions) ([]*Context, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.resolver == nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w: resolver not configured", ErrUninitialized)
	}
	if err := validateCreateContextsOptions(opts); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContexts: %w", err)
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
			return nil, fmt.Errorf("storage.Service.CreateContexts: resolver returned non-*Context value")
		}
		out[i] = concrete
	}
	s.injectContextUploadGuards(out)
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
	if err := validateCreateContextOptions(opts); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w", err)
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
		return nil, fmt.Errorf("storage.Service.CreateContext: resolver returned non-*Context value")
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
	s.injectContextUploadGuards([]*Context{concrete})
	if err := s.populateClientDataSetIDs(ctx, []*Context{concrete}); err != nil {
		return nil, fmt.Errorf("storage.Service.CreateContext: %w", err)
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

func (s *Service) injectContextUploadGuards(contexts []*Context) {
	reader := s.dsReader
	var validator DataSetValidator
	if resolver, ok := s.resolver.(*ServiceResolver); ok {
		if reader == nil {
			reader = resolver.warmStorage
		}
		validator = resolver.dataSetValidator
	}
	if reader == nil && validator == nil {
		return
	}
	for _, c := range contexts {
		if c == nil {
			continue
		}
		c.mu.Lock()
		if reader != nil && c.dataSetReader == nil {
			c.dataSetReader = reader
		}
		if validator != nil && c.dataSetValidator == nil {
			c.dataSetValidator = validator
		}
		c.mu.Unlock()
	}
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

// GetDefaultContext returns a single auto-selected context using resolver
// defaults.
func (s *Service) GetDefaultContext(ctx context.Context) (*Context, error) {
	return s.CreateContext(ctx, nil)
}
