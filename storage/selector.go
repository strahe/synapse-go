package storage

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/retry"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// selectorMetadataConcurrency caps fan-out of parallel
// GetAllDataSetMetadata calls made by ServiceResolver. Kept small because
// each call hits a public RPC endpoint.
const selectorMetadataConcurrency = 8

// selectorListPageSize mirrors warmstorage's IterateAll* page size so resolver
// scans remain explicit after ListOptions started rejecting Limit==0.
const selectorListPageSize = 100

const selectorRetryInitialDelay = 200 * time.Millisecond

type selectionResult struct {
	selections []ResolvedUploadContext
	explicit   bool
}

// PDPProviderSource is the subset of spregistry.Service used by ServiceResolver.
type PDPProviderSource interface {
	GetPDPProvider(context.Context, types.BigInt) (*spregistry.PDPProvider, error)
	SelectActivePDPProviders(context.Context, spregistry.ProviderFilter) ([]spregistry.PDPProvider, error)
}

// DataSetCatalog is the subset of warmstorage.Service used by ServiceResolver.
type DataSetCatalog interface {
	GetApprovedProviderIDs(context.Context, types.ListOptions) ([]types.BigInt, error)
	GetClientDataSets(context.Context, common.Address, types.ListOptions) ([]*warmstorage.DataSetInfo, error)
	GetDataSet(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error)
	GetAllDataSetMetadata(context.Context, types.BigInt) (map[string]string, error)
}

// ResolvedUploadContext is the pre-selection result for one provider copy.
type ResolvedUploadContext struct {
	Provider        Provider
	DataSetID       *types.BigInt     // nil when a new data set will be created
	ClientDataSetID *types.BigInt     // stable caller-chosen ID reused across commits; nil for new datasets
	DataSetMetadata map[string]string // metadata carried into the new data set if created
}

// ContextFactory builds an UploadContext from a resolved selection.
type ContextFactory func(ResolvedUploadContext, *UploadOptions) (UploadContext, error)

// ServiceResolverOptions configures a ServiceResolver.
type ServiceResolverOptions struct {
	Payer       common.Address // EVM address of the paying account
	SPRegistry  PDPProviderSource
	WarmStorage DataSetCatalog
	NewContext  ContextFactory // called per-provider to construct an UploadContext
}

// ServiceResolver selects providers and data sets for each upload, reusing
// existing data sets when metadata matches exactly.
type ServiceResolver struct {
	payer       common.Address
	spRegistry  PDPProviderSource
	warmStorage DataSetCatalog
	newContext  ContextFactory
}

var (
	_ PDPProviderSource = (*spregistry.Service)(nil)
	_ DataSetCatalog    = (*warmstorage.Service)(nil)
)

// NewServiceResolver constructs a ServiceResolver. All fields in opts are required.
func NewServiceResolver(opts ServiceResolverOptions) (*ServiceResolver, error) {
	if opts.Payer == (common.Address{}) {
		return nil, fmt.Errorf("storage.NewServiceResolver: %w: zero payer", ErrInvalidArgument)
	}
	if opts.SPRegistry == nil {
		return nil, fmt.Errorf("storage.NewServiceResolver: %w: nil SPRegistry", ErrInvalidArgument)
	}
	if opts.WarmStorage == nil {
		return nil, fmt.Errorf("storage.NewServiceResolver: %w: nil WarmStorage", ErrInvalidArgument)
	}
	if opts.NewContext == nil {
		return nil, fmt.Errorf("storage.NewServiceResolver: %w: nil NewContext", ErrInvalidArgument)
	}
	return &ServiceResolver{
		payer:       opts.Payer,
		spRegistry:  opts.SPRegistry,
		warmStorage: opts.WarmStorage,
		newContext:  opts.NewContext,
	}, nil
}

// ResolveUploadContexts returns one UploadContext per requested copy. When
// neither ProviderIDs nor DataSetIDs are set, providers are auto-selected from
// the warmstorage-approved and active-PDP intersection. The second return value
// is true when providers were explicitly specified by opts.
func (r *ServiceResolver) ResolveUploadContexts(ctx context.Context, opts *UploadOptions) ([]UploadContext, bool, error) {
	resolved, err := r.resolveSelectionsWithRetry(ctx, opts, nil)
	if err != nil {
		return nil, false, err
	}
	selections := resolved.selections
	contexts := make([]UploadContext, 0, len(selections))
	for _, selection := range selections {
		uploadCtx, err := r.newContext(selection, opts)
		if err != nil {
			return nil, false, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: build context for provider %s: %w", selection.Provider.ID.String(), err)
		}
		contexts = append(contexts, uploadCtx)
	}
	return contexts, resolved.explicit, nil
}

// SelectReplacement picks a single unused provider to replace a failed attempt.
func (r *ServiceResolver) SelectReplacement(ctx context.Context, usedProviders map[string]types.BigInt, opts *UploadOptions) (UploadContext, error) {
	resolved, err := r.resolveSelectionsWithRetry(ctx, withCopies(opts, 1), usedProviders)
	if err != nil {
		return nil, err
	}
	selections := resolved.selections
	if len(selections) == 0 {
		return nil, errors.New("storage.ServiceResolver.SelectReplacement: no remaining providers")
	}
	uploadCtx, err := r.newContext(selections[0], opts)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.SelectReplacement: build context for provider %s: %w", selections[0].Provider.ID.String(), err)
	}
	return uploadCtx, nil
}

func (r *ServiceResolver) resolveSelectionsWithRetry(ctx context.Context, opts *UploadOptions, extraExcludes map[string]types.BigInt) (selectionResult, error) {
	return retry.Do(ctx, func(ctx context.Context) (selectionResult, error) {
		selections, explicit, err := r.resolveSelections(ctx, opts, extraExcludes)
		if err != nil {
			return selectionResult{}, err
		}
		return selectionResult{selections: selections, explicit: explicit}, nil
	},
		retry.WithMaxRetries(3),
		retry.WithInitialDelay(selectorRetryInitialDelay),
		retry.WithMaxDelay(2*time.Second),
		retry.WithRetryIf(txutil.IsRetryableRPCError),
	)
}

func (r *ServiceResolver) resolveSelections(ctx context.Context, opts *UploadOptions, extraExcludes map[string]types.BigInt) ([]ResolvedUploadContext, bool, error) {
	if opts == nil {
		opts = &UploadOptions{}
	}
	switch {
	case len(opts.DataSetIDs) > 0:
		resolved, err := r.resolveByDataSetIDs(ctx, opts)
		return resolved, true, err
	case len(opts.ProviderIDs) > 0:
		resolved, err := r.resolveByProviderIDs(ctx, opts)
		return resolved, true, err
	default:
		resolved, err := r.autoSelect(ctx, opts, extraExcludes)
		return resolved, false, err
	}
}

func (r *ServiceResolver) resolveByDataSetIDs(ctx context.Context, opts *UploadOptions) ([]ResolvedUploadContext, error) {
	ids := dedupeIDs(opts.DataSetIDs)
	count := opts.Copies
	if count == 0 {
		count = len(ids)
	}
	if len(ids) != count {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: requested %d context(s) but dataSetIDs resolved to %d after deduplication", count, len(ids))
	}

	out := make([]ResolvedUploadContext, 0, len(ids))
	seenProviders := map[string]struct{}{}
	for _, dataSetID := range ids {
		dataSet, err := r.warmStorage.GetDataSet(ctx, dataSetID)
		if err != nil {
			if errors.Is(err, warmstorage.ErrNotFound) {
				return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: data set %s does not exist", dataSetID.String())
			}
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set %s: %w", dataSetID.String(), err)
		}
		if dataSet.Payer != r.payer {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: data set %s is not owned by %s", dataSetID.String(), r.payer.Hex())
		}
		if dataSet.PDPEndEpoch != 0 {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: data set %s is not active", dataSetID.String())
		}
		provider, err := r.spRegistry.GetPDPProvider(ctx, dataSet.ProviderID)
		if err != nil {
			if errors.Is(err, spregistry.ErrNotFound) {
				return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: provider %s for data set %s not found", dataSet.ProviderID.String(), dataSetID.String())
			}
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get provider %s: %w", dataSet.ProviderID.String(), err)
		}
		providerKey := idconv.Key(provider.Info.ID)
		if _, ok := seenProviders[providerKey]; ok {
			return nil, errors.New("storage.ServiceResolver.ResolveUploadContexts: dataSetIDs resolve to duplicate providers")
		}
		seenProviders[providerKey] = struct{}{}
		metadata, err := r.warmStorage.GetAllDataSetMetadata(ctx, dataSetID)
		if err != nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set metadata %s: %w", dataSetID.String(), err)
		}
		dsID := dataSetID
		out = append(out, buildResolvedUploadContext(*provider, &dsID, copyClientDataSetIDPtr(dataSet.ClientDataSetID), metadata))
	}
	return out, nil
}

func (r *ServiceResolver) resolveByProviderIDs(ctx context.Context, opts *UploadOptions) ([]ResolvedUploadContext, error) {
	ids := dedupeIDs(opts.ProviderIDs)
	count := opts.Copies
	if count == 0 {
		count = len(ids)
	}
	if len(ids) != count {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: requested %d context(s) but providerIDs resolved to %d after deduplication", count, len(ids))
	}

	dataSets, err := r.getAllClientDataSets(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get client data sets: %w", err)
	}
	requestedMetadata := dataSetMetadataFromOptions(opts)
	out := make([]ResolvedUploadContext, 0, len(ids))
	for _, providerID := range ids {
		provider, err := r.spRegistry.GetPDPProvider(ctx, providerID)
		if err != nil {
			if errors.Is(err, spregistry.ErrNotFound) {
				return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: provider %s not found", providerID.String())
			}
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get provider %s: %w", providerID.String(), err)
		}
		dataSetID, clientDataSetID, metadata, err := r.selectMatchingDataSet(ctx, providerID, dataSets, requestedMetadata)
		if err != nil {
			return nil, err
		}
		out = append(out, buildResolvedUploadContext(*provider, dataSetID, clientDataSetID, metadata))
	}
	return out, nil
}

func (r *ServiceResolver) autoSelect(ctx context.Context, opts *UploadOptions, extraExcludes map[string]types.BigInt) ([]ResolvedUploadContext, error) {
	count := opts.Copies
	if count == 0 {
		count = 2
	}
	approvedIDs, err := r.getAllApprovedProviderIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get approved providers: %w", err)
	}
	if len(approvedIDs) == 0 {
		return nil, errors.New("storage.ServiceResolver.ResolveUploadContexts: no approved providers")
	}
	excludeIDs := appendExcludedIDs(opts.ExcludeProviderIDs, extraExcludes)
	providers, err := r.spRegistry.SelectActivePDPProviders(ctx, spregistry.ProviderFilter{ExcludeIDs: excludeIDs})
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: select active PDP providers: %w", err)
	}
	approvedSet := make(map[string]struct{}, len(approvedIDs))
	for _, id := range approvedIDs {
		approvedSet[idconv.Key(id)] = struct{}{}
	}

	dataSets, err := r.getAllClientDataSets(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get client data sets: %w", err)
	}
	requestedMetadata := dataSetMetadataFromOptions(opts)
	withDataSet := make([]ResolvedUploadContext, 0, len(providers))
	withoutDataSet := make([]ResolvedUploadContext, 0, len(providers))
	for _, provider := range providers {
		if _, ok := approvedSet[idconv.Key(provider.Info.ID)]; !ok {
			continue
		}
		dataSetID, clientDataSetID, metadata, err := r.selectMatchingDataSet(ctx, provider.Info.ID, dataSets, requestedMetadata)
		if err != nil {
			return nil, err
		}
		resolved := buildResolvedUploadContext(provider, dataSetID, clientDataSetID, metadata)
		if dataSetID != nil {
			withDataSet = append(withDataSet, resolved)
		} else {
			withoutDataSet = append(withoutDataSet, resolved)
		}
	}

	selected := make([]ResolvedUploadContext, 0, len(withDataSet)+len(withoutDataSet))
	selected = append(selected, withDataSet...)
	selected = append(selected, withoutDataSet...)
	if len(selected) > count {
		selected = selected[:count]
	}
	if len(selected) == 0 {
		return nil, errors.New("storage.ServiceResolver.ResolveUploadContexts: no remaining providers")
	}
	return selected, nil
}

func (r *ServiceResolver) getAllClientDataSets(ctx context.Context) ([]*warmstorage.DataSetInfo, error) {
	var (
		offset uint64
		all    []*warmstorage.DataSetInfo
	)
	for {
		page, err := r.warmStorage.GetClientDataSets(ctx, r.payer, types.ListOptions{
			Offset: offset,
			Limit:  selectorListPageSize,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if uint64(len(page)) < selectorListPageSize {
			return all, nil
		}
		offset += uint64(len(page))
	}
}

func (r *ServiceResolver) getAllApprovedProviderIDs(ctx context.Context) ([]types.BigInt, error) {
	var (
		offset uint64
		all    []types.BigInt
	)
	for {
		page, err := r.warmStorage.GetApprovedProviderIDs(ctx, types.ListOptions{
			Offset: offset,
			Limit:  selectorListPageSize,
		})
		if err != nil {
			return nil, err
		}
		all = append(all, page...)
		if uint64(len(page)) < selectorListPageSize {
			return all, nil
		}
		offset += uint64(len(page))
	}
}

func (r *ServiceResolver) selectMatchingDataSet(ctx context.Context, providerID types.BigInt, dataSets []*warmstorage.DataSetInfo, requestedMetadata map[string]string) (*types.BigInt, *types.BigInt, map[string]string, error) {
	matching := make([]*warmstorage.DataSetInfo, 0)
	for _, dataSet := range dataSets {
		if dataSet == nil {
			continue
		}
		if dataSet.DataSetID.IsZero() || !dataSet.ProviderID.Equal(providerID) || dataSet.PDPEndEpoch != 0 {
			continue
		}
		matching = append(matching, dataSet)
	}
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].DataSetID.Cmp(matching[j].DataSetID) < 0
	})
	if len(matching) == 0 {
		return nil, nil, cloneStringMap(requestedMetadata), nil
	}

	// Fetch metadata for all candidates concurrently with a bounded worker
	// pool. The caller's ctx remains the only time budget; the iteration order
	// for the match check remains deterministic (sorted by DataSetID).
	//
	// Batched metadata fetch would require a new server API; this only
	// pipelines the existing N+1 calls.
	metadataByID := make(map[string]map[string]string, len(matching))
	var mu sync.Mutex
	workers := selectorMetadataConcurrency
	if workers > len(matching) {
		workers = len(matching)
	}
	sem := make(chan struct{}, workers)
	errCh := make(chan error, len(matching))
	var wg sync.WaitGroup
	for _, ds := range matching {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return nil, nil, nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: %w", ctx.Err())
		}
		wg.Add(1)
		go func(dsID types.BigInt) {
			defer wg.Done()
			defer func() { <-sem }()
			metadata, err := r.warmStorage.GetAllDataSetMetadata(ctx, dsID)
			if err != nil {
				errCh <- fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set metadata %s: %w", dsID.String(), err)
				return
			}
			mu.Lock()
			metadataByID[idconv.Key(dsID)] = metadata
			mu.Unlock()
		}(ds.DataSetID)
	}
	wg.Wait()
	close(errCh)
	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, nil, nil, errors.Join(errs...)
	}

	for _, dataSet := range matching {
		if metadata, ok := metadataByID[idconv.Key(dataSet.DataSetID)]; ok && metadataMatches(metadata, requestedMetadata) {
			dsID := dataSet.DataSetID
			return &dsID, copyClientDataSetIDPtr(dataSet.ClientDataSetID), metadata, nil
		}
	}

	return nil, nil, cloneStringMap(requestedMetadata), nil
}

func buildResolvedUploadContext(provider spregistry.PDPProvider, dataSetID, clientDataSetID *types.BigInt, metadata map[string]string) ResolvedUploadContext {
	return ResolvedUploadContext{
		Provider: Provider{
			ID:              provider.Info.ID,
			ServiceURL:      provider.Offering.ServiceURL,
			ServiceProvider: provider.Info.ServiceProvider,
			Payee:           provider.Info.Payee,
		},
		DataSetID:       copyIDPtr(dataSetID),
		ClientDataSetID: copyBigIntPtr(clientDataSetID),
		DataSetMetadata: cloneStringMap(metadata),
	}
}

func metadataMatches(dataSetMetadata, requestedMetadata map[string]string) bool {
	if len(dataSetMetadata) != len(requestedMetadata) {
		return false
	}
	for key, value := range requestedMetadata {
		if dataSetMetadata[key] != value {
			return false
		}
	}
	return true
}

func dataSetMetadataFromOptions(opts *UploadOptions) map[string]string {
	metadata := cloneStringMap(opts.DataSetMetadata)
	if opts.WithCDN != nil && *opts.WithCDN {
		if metadata == nil {
			metadata = make(map[string]string, 1)
		}
		metadata["withCDN"] = ""
	}
	return metadata
}

func withCopies(opts *UploadOptions, copies int) *UploadOptions {
	if opts == nil {
		return &UploadOptions{Copies: copies}
	}
	cloned := *opts
	cloned.Copies = copies
	if len(opts.PieceMetadata) != 0 {
		cloned.PieceMetadata = cloneStringMap(opts.PieceMetadata)
	}
	if len(opts.DataSetMetadata) != 0 {
		cloned.DataSetMetadata = cloneStringMap(opts.DataSetMetadata)
	}
	if len(opts.ProviderIDs) != 0 {
		cloned.ProviderIDs = append([]types.BigInt(nil), opts.ProviderIDs...)
	}
	if len(opts.DataSetIDs) != 0 {
		cloned.DataSetIDs = append([]types.BigInt(nil), opts.DataSetIDs...)
	}
	if len(opts.ExcludeProviderIDs) != 0 {
		cloned.ExcludeProviderIDs = append([]types.BigInt(nil), opts.ExcludeProviderIDs...)
	}
	return &cloned
}

func appendExcludedIDs(excluded []types.BigInt, extra map[string]types.BigInt) []types.BigInt {
	out := append([]types.BigInt(nil), excluded...)
	seen := make(map[string]struct{}, len(out))
	for _, id := range out {
		seen[idconv.Key(id)] = struct{}{}
	}
	for key, id := range extra {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, id)
	}
	return out
}

func dedupeIDs(values []types.BigInt) []types.BigInt {
	if len(values) == 0 {
		return nil
	}
	out := make([]types.BigInt, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		key := idconv.Key(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func copyIDPtr(v *types.BigInt) *types.BigInt {
	if v == nil {
		return nil
	}
	cp := copyBigInt(*v)
	return &cp
}

func copyBigInt(v types.BigInt) types.BigInt {
	return v.Copy()
}

func copyBigIntPtr(v *types.BigInt) *types.BigInt {
	if v == nil {
		return nil
	}
	cp := copyBigInt(*v)
	return &cp
}

func copyClientDataSetID(v types.BigInt) types.BigInt {
	return copyBigInt(v)
}

func copyClientDataSetIDPtr(v types.BigInt) *types.BigInt {
	cp := copyClientDataSetID(v)
	return &cp
}

func copyClientDataSetIDFromPtr(v *types.BigInt) types.BigInt {
	if v == nil {
		return types.BigInt{}
	}
	return copyClientDataSetID(*v)
}
