package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/warmstorage"
)

type PDPProviderSource interface {
	GetPDPProvider(context.Context, *big.Int) (*spregistry.PDPProvider, error)
	SelectActivePDPProviders(context.Context, spregistry.ProviderFilter) ([]spregistry.PDPProvider, error)
}

type DataSetCatalog interface {
	GetApprovedProviderIDs(context.Context, *big.Int, *big.Int) ([]*big.Int, error)
	GetClientDataSets(context.Context, common.Address, *big.Int, *big.Int) ([]*warmstorage.DataSetInfo, error)
	GetDataSet(context.Context, *big.Int) (*warmstorage.DataSetInfo, error)
	GetAllDataSetMetadata(context.Context, *big.Int) (map[string]string, error)
}

type ResolvedUploadContext struct {
	Provider        Provider
	DataSetID       *big.Int
	ClientDataSetID *big.Int
	DataSetMetadata map[string]string
}

type ContextFactory func(ResolvedUploadContext, *UploadOptions) (UploadContext, error)

type ServiceResolverOptions struct {
	Payer       common.Address
	SPRegistry  PDPProviderSource
	WarmStorage DataSetCatalog
	NewContext  ContextFactory
}

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

func NewServiceResolver(opts ServiceResolverOptions) (*ServiceResolver, error) {
	if opts.Payer == (common.Address{}) {
		return nil, errors.New("storage.NewServiceResolver: zero payer")
	}
	if opts.SPRegistry == nil {
		return nil, errors.New("storage.NewServiceResolver: nil SPRegistry")
	}
	if opts.WarmStorage == nil {
		return nil, errors.New("storage.NewServiceResolver: nil WarmStorage")
	}
	if opts.NewContext == nil {
		return nil, errors.New("storage.NewServiceResolver: nil NewContext")
	}
	return &ServiceResolver{
		payer:       opts.Payer,
		spRegistry:  opts.SPRegistry,
		warmStorage: opts.WarmStorage,
		newContext:  opts.NewContext,
	}, nil
}

func (r *ServiceResolver) ResolveUploadContexts(ctx context.Context, opts *UploadOptions) ([]UploadContext, bool, error) {
	selections, explicit, err := r.resolveSelections(ctx, opts, nil)
	if err != nil {
		return nil, false, err
	}
	contexts := make([]UploadContext, 0, len(selections))
	for _, selection := range selections {
		uploadCtx, err := r.newContext(selection, opts)
		if err != nil {
			return nil, false, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: build context for provider %s: %w", selection.Provider.ID, err)
		}
		contexts = append(contexts, uploadCtx)
	}
	return contexts, explicit, nil
}

func (r *ServiceResolver) SelectReplacement(ctx context.Context, usedProviders map[string]struct{}, opts *UploadOptions) (UploadContext, error) {
	selections, _, err := r.resolveSelections(ctx, withCopies(opts, 1), usedProviders)
	if err != nil {
		return nil, err
	}
	if len(selections) == 0 {
		return nil, errors.New("storage.ServiceResolver.SelectReplacement: no remaining providers")
	}
	uploadCtx, err := r.newContext(selections[0], opts)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.SelectReplacement: build context for provider %s: %w", selections[0].Provider.ID, err)
	}
	return uploadCtx, nil
}

func (r *ServiceResolver) resolveSelections(ctx context.Context, opts *UploadOptions, extraExcludes map[string]struct{}) ([]ResolvedUploadContext, bool, error) {
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
	ids := dedupeBigInts(opts.DataSetIDs)
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
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set %s: %w", dataSetID, err)
		}
		if dataSet == nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: data set %s does not exist", dataSetID)
		}
		if dataSet.Payer != r.payer {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: data set %s is not owned by %s", dataSetID, r.payer.Hex())
		}
		if dataSet.PDPEndEpoch == nil || dataSet.PDPEndEpoch.Sign() != 0 {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: data set %s is not active", dataSetID)
		}
		provider, err := r.spRegistry.GetPDPProvider(ctx, dataSet.ProviderID)
		if err != nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get provider %s: %w", dataSet.ProviderID, err)
		}
		if provider == nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: provider %s for data set %s not found", dataSet.ProviderID, dataSetID)
		}
		if _, ok := seenProviders[provider.Info.ID.String()]; ok {
			return nil, errors.New("storage.ServiceResolver.ResolveUploadContexts: dataSetIDs resolve to duplicate providers")
		}
		seenProviders[provider.Info.ID.String()] = struct{}{}
		metadata, err := r.warmStorage.GetAllDataSetMetadata(ctx, dataSetID)
		if err != nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set metadata %s: %w", dataSetID, err)
		}
		out = append(out, buildResolvedUploadContext(*provider, dataSetID, dataSet.ClientDataSetID, metadata))
	}
	return out, nil
}

func (r *ServiceResolver) resolveByProviderIDs(ctx context.Context, opts *UploadOptions) ([]ResolvedUploadContext, error) {
	ids := dedupeBigInts(opts.ProviderIDs)
	count := opts.Copies
	if count == 0 {
		count = len(ids)
	}
	if len(ids) != count {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: requested %d context(s) but providerIDs resolved to %d after deduplication", count, len(ids))
	}

	dataSets, err := r.warmStorage.GetClientDataSets(ctx, r.payer, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get client data sets: %w", err)
	}
	requestedMetadata := dataSetMetadataFromOptions(opts)
	out := make([]ResolvedUploadContext, 0, len(ids))
	for _, providerID := range ids {
		provider, err := r.spRegistry.GetPDPProvider(ctx, providerID)
		if err != nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get provider %s: %w", providerID, err)
		}
		if provider == nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: provider %s not found", providerID)
		}
		dataSetID, clientDataSetID, metadata, err := r.selectMatchingDataSet(ctx, providerID, dataSets, requestedMetadata)
		if err != nil {
			return nil, err
		}
		out = append(out, buildResolvedUploadContext(*provider, dataSetID, clientDataSetID, metadata))
	}
	return out, nil
}

func (r *ServiceResolver) autoSelect(ctx context.Context, opts *UploadOptions, extraExcludes map[string]struct{}) ([]ResolvedUploadContext, error) {
	count := opts.Copies
	if count == 0 {
		count = 2
	}
	approvedIDs, err := r.warmStorage.GetApprovedProviderIDs(ctx, nil, nil)
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
		if id != nil {
			approvedSet[id.String()] = struct{}{}
		}
	}

	dataSets, err := r.warmStorage.GetClientDataSets(ctx, r.payer, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get client data sets: %w", err)
	}
	requestedMetadata := dataSetMetadataFromOptions(opts)
	withDataSet := make([]ResolvedUploadContext, 0, len(providers))
	withoutDataSet := make([]ResolvedUploadContext, 0, len(providers))
	for _, provider := range providers {
		if _, ok := approvedSet[provider.Info.ID.String()]; !ok {
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

func (r *ServiceResolver) selectMatchingDataSet(ctx context.Context, providerID *big.Int, dataSets []*warmstorage.DataSetInfo, requestedMetadata map[string]string) (*big.Int, *big.Int, map[string]string, error) {
	matching := make([]*warmstorage.DataSetInfo, 0)
	for _, dataSet := range dataSets {
		if dataSet == nil || dataSet.DataSetID == nil || dataSet.ProviderID == nil || dataSet.PDPEndEpoch == nil {
			continue
		}
		if dataSet.ProviderID.Cmp(providerID) != 0 || dataSet.PDPEndEpoch.Sign() != 0 {
			continue
		}
		matching = append(matching, dataSet)
	}
	sort.Slice(matching, func(i, j int) bool {
		return matching[i].DataSetID.Cmp(matching[j].DataSetID) < 0
	})
	// TODO: N+1 RPC calls — one GetAllDataSetMetadata per candidate dataset.
	// Batch metadata fetch would require a new server API endpoint.
	for _, dataSet := range matching {
		metadata, err := r.warmStorage.GetAllDataSetMetadata(ctx, dataSet.DataSetID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set metadata %s: %w", dataSet.DataSetID, err)
		}
		if metadataMatches(metadata, requestedMetadata) {
			return new(big.Int).Set(dataSet.DataSetID), copyBigInt(dataSet.ClientDataSetID), metadata, nil
		}
	}
	return nil, nil, cloneStringMap(requestedMetadata), nil
}

func buildResolvedUploadContext(provider spregistry.PDPProvider, dataSetID, clientDataSetID *big.Int, metadata map[string]string) ResolvedUploadContext {
	return ResolvedUploadContext{
		Provider: Provider{
			ID:              new(big.Int).Set(provider.Info.ID),
			ServiceURL:      provider.Offering.ServiceURL,
			ServiceProvider: provider.Info.ServiceProvider,
			Payee:           provider.Info.Payee,
		},
		DataSetID:       copyBigInt(dataSetID),
		ClientDataSetID: copyBigInt(clientDataSetID),
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
	if opts.WithCDN {
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
		cloned.ProviderIDs = cloneBigInts(opts.ProviderIDs)
	}
	if len(opts.DataSetIDs) != 0 {
		cloned.DataSetIDs = cloneBigInts(opts.DataSetIDs)
	}
	if len(opts.ExcludeProviderIDs) != 0 {
		cloned.ExcludeProviderIDs = cloneBigInts(opts.ExcludeProviderIDs)
	}
	return &cloned
}

func appendExcludedIDs(excluded []*big.Int, extra map[string]struct{}) []*big.Int {
	out := cloneBigInts(excluded)
	for id := range extra {
		if _, ok := containsBigIntString(out, id); ok {
			continue
		}
		if parsed, ok := new(big.Int).SetString(id, 10); ok {
			out = append(out, parsed)
		}
	}
	return out
}

func dedupeBigInts(values []*big.Int) []*big.Int {
	if len(values) == 0 {
		return nil
	}
	out := make([]*big.Int, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == nil {
			continue
		}
		key := value.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, new(big.Int).Set(value))
	}
	return out
}

func containsBigIntString(values []*big.Int, target string) (*big.Int, bool) {
	for _, value := range values {
		if value != nil && value.String() == target {
			return value, true
		}
	}
	return nil, false
}
