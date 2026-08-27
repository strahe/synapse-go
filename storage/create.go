package storage

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/types"
)

// NewProviderContextOptions configures an explicitly identified provider
// context. WithCDN nil inherits the Service default.
type NewProviderContextOptions struct {
	DataSetMetadata map[string]string
	WithCDN         *bool
}

// NewDataSetContextOptions configures an explicitly identified data-set
// context. ProviderID, when set, asserts the data set's provider.
type NewDataSetContextOptions struct {
	ProviderID *types.BigInt
	WithCDN    *bool
}

// SelectProviderContextOptions configures selection of one healthy provider.
type SelectProviderContextOptions struct {
	ExcludeProviderIDs []types.BigInt
	DataSetMetadata    map[string]string
	WithCDN            *bool
}

// SelectUploadContextsOptions configures selection of upload targets.
type SelectUploadContextsOptions struct {
	Copies             int
	ExcludeProviderIDs []types.BigInt
	DataSetMetadata    map[string]string
	WithCDN            *bool
}

// UploadContextSelection contains the targets selected for one upload.
type UploadContextSelection struct {
	Contexts        []StorageContext
	RequestedCopies int
	Complete        bool
}

// NewProviderContext opens a registered provider without selecting it or
// checking its current approval, activity, or endpoint health.
func (s *Service) NewProviderContext(ctx context.Context, providerID types.BigInt, opts NewProviderContextOptions) (*ProviderContext, error) {
	const op = "storage.Service.NewProviderContext"
	providerID = copyBigInt(providerID)
	opts = NewProviderContextOptions{
		DataSetMetadata: cloneStringMap(opts.DataSetMetadata),
		WithCDN:         copyBoolPtr(opts.WithCDN),
	}
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if providerID.IsZero() {
		return nil, fmt.Errorf("%s: %w: zero providerID", op, ErrInvalidArgument)
	}
	if s.contextResolver == nil {
		return nil, fmt.Errorf("%s: %w: context resolver not configured", op, ErrUninitialized)
	}
	resolved := s.resolveNewProviderContextOptions(opts)
	storageCtx, err := s.contextResolver.ResolveProviderContext(ctx, copyBigInt(providerID), resolved)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := s.validateProviderContext(op, storageCtx, providerID); err != nil {
		return nil, err
	}
	return storageCtx, nil
}

// NewDataSetContext opens an existing data set owned by the Service payer.
// The data set may be terminated or otherwise not currently writable.
func (s *Service) NewDataSetContext(ctx context.Context, dataSetID types.BigInt, opts NewDataSetContextOptions) (*DataSetContext, error) {
	const op = "storage.Service.NewDataSetContext"
	dataSetID = copyBigInt(dataSetID)
	opts = cloneNewDataSetContextOptions(opts)
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("%s: %w: zero dataSetID", op, ErrInvalidArgument)
	}
	if opts.ProviderID != nil && opts.ProviderID.IsZero() {
		return nil, fmt.Errorf("%s: %w: zero providerID", op, ErrInvalidArgument)
	}
	if s.contextResolver == nil {
		return nil, fmt.Errorf("%s: %w: context resolver not configured", op, ErrUninitialized)
	}
	resolved := opts
	resolved.WithCDN = boolPtr(resolveBoolDefault(resolved.WithCDN, s.defaultWithCDN))
	storageCtx, err := s.contextResolver.ResolveDataSetContext(ctx, copyBigInt(dataSetID), resolved)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := s.validateDataSetContext(op, storageCtx, dataSetID, resolved.ProviderID); err != nil {
		return nil, err
	}
	return storageCtx, nil
}

// SelectProviderContext selects one approved, active, healthy provider without
// querying or reusing an existing data set.
func (s *Service) SelectProviderContext(ctx context.Context, opts SelectProviderContextOptions) (*ProviderContext, error) {
	const op = "storage.Service.SelectProviderContext"
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.contextSelector == nil {
		return nil, fmt.Errorf("%s: %w: context selector not configured", op, ErrUninitialized)
	}
	resolved, err := s.resolveSelectProviderContextOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	storageCtx, err := s.contextSelector.SelectProviderContext(ctx, resolved)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	if err := s.validateProviderContext(op, storageCtx, storageCtxID(storageCtx)); err != nil {
		return nil, err
	}
	if err := validateExcludedStorageContexts(op, []StorageContext{storageCtx}, resolved.ExcludeProviderIDs); err != nil {
		return nil, err
	}
	return storageCtx, nil
}

// SelectUploadContexts selects targets for Prepare and UploadToContexts. A
// partial result is returned with InsufficientUploadContextsError.
func (s *Service) SelectUploadContexts(ctx context.Context, opts SelectUploadContextsOptions) (*UploadContextSelection, error) {
	const op = "storage.Service.SelectUploadContexts"
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.contextSelector == nil {
		return nil, fmt.Errorf("%s: %w: context selector not configured", op, ErrUninitialized)
	}
	resolved, err := s.resolveSelectUploadContextsOptions(opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	selection, selectErr := s.contextSelector.SelectUploadContexts(ctx, resolved)
	if selectErr != nil && !errors.Is(selectErr, ErrInsufficientUploadContexts) {
		return nil, fmt.Errorf("%s: %w", op, selectErr)
	}
	if err := s.validateUploadContextSelection(op, selection, resolved.Copies, selectErr); err != nil {
		return nil, err
	}
	if err := validateExcludedStorageContexts(op, selection.Contexts, resolved.ExcludeProviderIDs); err != nil {
		return nil, err
	}
	if selectErr != nil {
		return selection, fmt.Errorf("%s: %w", op, selectErr)
	}
	return selection, nil
}

func (s *Service) resolveNewProviderContextOptions(opts NewProviderContextOptions) NewProviderContextOptions {
	metadata := cloneStringMap(opts.DataSetMetadata)
	if s.source != "" {
		if metadata == nil {
			metadata = make(map[string]string, 1)
		}
		if _, exists := metadata["source"]; !exists {
			metadata["source"] = s.source
		}
	}
	return NewProviderContextOptions{
		DataSetMetadata: metadata,
		WithCDN:         boolPtr(resolveBoolDefault(opts.WithCDN, s.defaultWithCDN)),
	}
}

func (s *Service) resolveSelectProviderContextOptions(opts SelectProviderContextOptions) (SelectProviderContextOptions, error) {
	if err := validateNonZeroIDs("ExcludeProviderID", opts.ExcludeProviderIDs...); err != nil {
		return SelectProviderContextOptions{}, err
	}
	providerOpts := s.resolveNewProviderContextOptions(NewProviderContextOptions{
		DataSetMetadata: opts.DataSetMetadata,
		WithCDN:         opts.WithCDN,
	})
	return SelectProviderContextOptions{
		ExcludeProviderIDs: cloneBigIntSlice(opts.ExcludeProviderIDs),
		DataSetMetadata:    providerOpts.DataSetMetadata,
		WithCDN:            providerOpts.WithCDN,
	}, nil
}

func (s *Service) resolveSelectUploadContextsOptions(opts SelectUploadContextsOptions) (SelectUploadContextsOptions, error) {
	if opts.Copies <= 0 {
		return SelectUploadContextsOptions{}, fmt.Errorf("%w: Copies must be greater than zero", ErrInvalidArgument)
	}
	providerOpts, err := s.resolveSelectProviderContextOptions(SelectProviderContextOptions{
		ExcludeProviderIDs: opts.ExcludeProviderIDs,
		DataSetMetadata:    opts.DataSetMetadata,
		WithCDN:            opts.WithCDN,
	})
	if err != nil {
		return SelectUploadContextsOptions{}, err
	}
	return SelectUploadContextsOptions{
		Copies:             opts.Copies,
		ExcludeProviderIDs: providerOpts.ExcludeProviderIDs,
		DataSetMetadata:    providerOpts.DataSetMetadata,
		WithCDN:            providerOpts.WithCDN,
	}, nil
}

func cloneNewDataSetContextOptions(opts NewDataSetContextOptions) NewDataSetContextOptions {
	return NewDataSetContextOptions{
		ProviderID: copyBigIntPtr(opts.ProviderID),
		WithCDN:    copyBoolPtr(opts.WithCDN),
	}
}

func resolveBoolDefault(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func boolPtr(value bool) *bool {
	out := value
	return &out
}

func copyBoolPtr(value *bool) *bool {
	if value == nil {
		return nil
	}
	return boolPtr(*value)
}

func (s *Service) validateProviderContext(op string, storageCtx *ProviderContext, providerID types.BigInt) error {
	if storageCtx == nil || storageCtx.core == nil {
		return fmt.Errorf("%s: %w: resolver returned nil provider context", op, ErrInvalidArgument)
	}
	if providerID.IsZero() || !storageCtx.ProviderID().Equal(providerID) {
		return fmt.Errorf("%s: %w: resolver returned providerID %s, want %s", op, ErrInvalidArgument, storageCtx.ProviderID().String(), providerID.String())
	}
	if _, bound := storageCtx.DataSetRef(); bound {
		return fmt.Errorf("%s: %w: resolver returned a bound provider context", op, ErrInvalidArgument)
	}
	return s.validateContextIdentity(op, storageCtx)
}

func (s *Service) validateDataSetContext(op string, storageCtx *DataSetContext, dataSetID types.BigInt, providerID *types.BigInt) error {
	if storageCtx == nil || storageCtx.core == nil {
		return fmt.Errorf("%s: %w: resolver returned nil data-set context", op, ErrInvalidArgument)
	}
	ref, bound := storageCtx.DataSetRef()
	if !bound || !ref.valid() || !ref.dataSetID.Equal(dataSetID) {
		return fmt.Errorf("%s: %w: resolver returned the wrong data-set target", op, ErrInvalidArgument)
	}
	if providerID != nil && !ref.providerID.Equal(*providerID) {
		return fmt.Errorf("%s: %w: data set providerID %s does not match requested providerID %s", op, ErrInvalidArgument, ref.providerID.String(), providerID.String())
	}
	if !storageCtx.ProviderID().Equal(ref.providerID) {
		return fmt.Errorf("%s: %w: context provider does not match data-set ref", op, ErrInvalidArgument)
	}
	return s.validateContextIdentity(op, storageCtx)
}

func (s *Service) validateContextIdentity(op string, storageCtx StorageContext) error {
	if isNilStorageContext(storageCtx) {
		return fmt.Errorf("%s: %w: nil storage context", op, ErrInvalidArgument)
	}
	if s.payerAddr == (common.Address{}) || !s.chainID.IsValid() || s.recordKeeper == (common.Address{}) {
		return fmt.Errorf("%s: %w: service identity is incomplete (payer, chain, and record keeper)", op, ErrInvalidArgument)
	}
	identity := storageCtx.ContextIdentity()
	if identity.Payer == s.payerAddr && identity.ChainID == s.chainID && identity.RecordKeeper == s.recordKeeper {
		return nil
	}
	return fmt.Errorf("%s: %w: context identity does not match service payer, chain, and record keeper", op, ErrInvalidArgument)
}

func (s *Service) validateUploadContextSelection(op string, selection *UploadContextSelection, requested int, selectionErr error) error {
	if selection == nil {
		return fmt.Errorf("%s: %w: selector returned nil selection", op, ErrInvalidArgument)
	}
	if selection.RequestedCopies != requested {
		return fmt.Errorf("%s: %w: selection requestedCopies %d, want %d", op, ErrInvalidArgument, selection.RequestedCopies, requested)
	}
	if err := s.validateStorageContexts(op, selection.Contexts); err != nil {
		return err
	}
	partial := errors.Is(selectionErr, ErrInsufficientUploadContexts)
	if partial {
		var insufficient *InsufficientUploadContextsError
		if !errors.As(selectionErr, &insufficient) || insufficient.Requested != requested || insufficient.Available != len(selection.Contexts) || len(selection.Contexts) == 0 || len(selection.Contexts) >= requested || selection.Complete {
			return fmt.Errorf("%s: %w: inconsistent partial selection", op, ErrInvalidArgument)
		}
		return nil
	}
	if !selection.Complete || len(selection.Contexts) != requested {
		return fmt.Errorf("%s: %w: inconsistent complete selection", op, ErrInvalidArgument)
	}
	return nil
}

func (s *Service) validateStorageContexts(op string, contexts []StorageContext) error {
	if len(contexts) == 0 {
		return fmt.Errorf("%s: %w: no storage contexts", op, ErrInvalidArgument)
	}
	seen := make(map[string]struct{}, len(contexts))
	for i, storageCtx := range contexts {
		if isNilStorageContext(storageCtx) {
			return fmt.Errorf("%s: %w: nil storage context at index %d", op, ErrInvalidArgument, i)
		}
		providerID := storageCtx.ProviderID()
		if providerID.IsZero() {
			return fmt.Errorf("%s: %w: zero providerID at index %d", op, ErrInvalidArgument, i)
		}
		key := idconv.Key(providerID)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%s: %w: duplicate providerID %s", op, ErrInvalidArgument, providerID.String())
		}
		seen[key] = struct{}{}
		if ref, bound := storageCtx.DataSetRef(); bound {
			if !ref.valid() || !ref.providerID.Equal(providerID) {
				return fmt.Errorf("%s: %w: invalid data-set ref at index %d", op, ErrInvalidArgument, i)
			}
		}
		if err := s.validateContextIdentity(op, storageCtx); err != nil {
			return err
		}
	}
	return nil
}

func isNilStorageContext(storageCtx StorageContext) bool {
	if storageCtx == nil {
		return true
	}
	switch c := storageCtx.(type) {
	case *ProviderContext:
		return c == nil || c.core == nil
	case *DataSetContext:
		return c == nil || c.core == nil
	}
	value := reflect.ValueOf(storageCtx)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func storageCtxID(storageCtx *ProviderContext) types.BigInt {
	if storageCtx == nil || storageCtx.core == nil {
		return types.BigInt{}
	}
	return storageCtx.ProviderID()
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

func validateExcludedStorageContexts(op string, contexts []StorageContext, excluded []types.BigInt) error {
	if len(excluded) == 0 {
		return nil
	}
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, providerID := range excluded {
		excludedSet[idconv.Key(providerID)] = struct{}{}
	}
	for _, storageCtx := range contexts {
		if _, found := excludedSet[idconv.Key(storageCtx.ProviderID())]; found {
			return fmt.Errorf("%s: %w: selector returned excluded providerID %s", op, ErrInvalidArgument, storageCtx.ProviderID().String())
		}
	}
	return nil
}

func (s *Service) validateUploadReplacement(op string, replacement StorageContext, usedProviders map[string]types.BigInt, excluded []types.BigInt) error {
	if err := s.validateStorageContexts(op, []StorageContext{replacement}); err != nil {
		return err
	}
	if err := validateExcludedStorageContexts(op, []StorageContext{replacement}, excluded); err != nil {
		return err
	}
	id := replacement.ProviderID()
	if _, exists := usedProviders[idconv.Key(id)]; exists {
		return fmt.Errorf("%s: %w: duplicate providerID %s", op, ErrInvalidArgument, id.String())
	}
	return nil
}
