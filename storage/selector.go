package storage

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"golang.org/x/sync/errgroup"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/retry"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// selectorListPageSize mirrors warmstorage's IterateAll* page size so resolver
// scans remain explicit after ListOptions started rejecting Limit==0.
const selectorListPageSize = 100

const selectorRetryInitialDelay = 200 * time.Millisecond

const providerPingTimeout = 8 * time.Second

const providerPingConcurrency = 16

const resolveConcurrency = 10

type autoSelectCandidate struct {
	provider        *spregistry.PDPProvider
	dataSetID       *types.BigInt
	clientDataSetID *types.BigInt
	metadata        map[string]string
}

type providerProbeState uint8

const (
	providerProbePending providerProbeState = iota
	providerProbeHealthy
	providerProbeUnhealthy
)

type providerProbeResult struct {
	index int
	err   error
}

type cachedProviderProbe struct {
	err error
}

type providerProbeCache struct {
	ping    func(context.Context, string) error
	mu      sync.Mutex
	entries map[string]cachedProviderProbe
}

func newProviderProbeCache(ping func(context.Context, string) error) *providerProbeCache {
	return &providerProbeCache{
		ping:    ping,
		entries: make(map[string]cachedProviderProbe),
	}
}

func (c *providerProbeCache) probe(ctx context.Context, providerKey, serviceURL string) error {
	c.mu.Lock()
	result, ok := c.entries[providerKey]
	c.mu.Unlock()
	if ok {
		return result.err
	}

	err := c.ping(ctx, serviceURL)
	// A speculative probe canceled after the stable selection frontier is known
	// has no reusable result. Completed failures, including the probe timeout,
	// remain valid for the secondary selection phase.
	if !errors.Is(ctx.Err(), context.Canceled) {
		c.mu.Lock()
		c.entries[providerKey] = cachedProviderProbe{err: err}
		c.mu.Unlock()
	}
	return err
}

// PDPProviderSource is the subset of spregistry.Service used by ServiceResolver.
type PDPProviderSource interface {
	GetPDPProvider(context.Context, types.BigInt) (*spregistry.PDPProvider, error)
	SelectActivePDPProviders(context.Context, spregistry.ProviderFilter) ([]spregistry.PDPProvider, error)
}

// EndorsedProviderSource supplies the ordered provider IDs eligible to act as
// automatic upload primaries. Implementations must be safe for concurrent use.
// A nil slice with a nil error is an empty endorsement set; query and decoding
// errors must be returned without being converted to an empty set.
type EndorsedProviderSource interface {
	GetEndorsedProviderIDs(context.Context) ([]types.BigInt, error)
}

// DataSetCatalog is the subset of warmstorage.Service used by ServiceResolver.
type DataSetCatalog interface {
	GetApprovedProviderIDs(context.Context, types.ListOptions) ([]types.BigInt, error)
	GetClientDataSets(context.Context, common.Address, types.ListOptions) ([]*warmstorage.DataSetInfo, error)
	GetDataSet(context.Context, types.BigInt) (*warmstorage.DataSetInfo, error)
	GetAllDataSetMetadata(context.Context, types.BigInt) (map[string]string, error)
}

type dataSetActivityReader interface {
	HasActivePieces(context.Context, types.BigInt) (bool, error)
}

type pdpVerifierAddressReader interface {
	PDPVerifierAddress() common.Address
}

// resolvedUploadContext is the pre-selection result for one provider copy.
type resolvedUploadContext struct {
	Provider        Provider
	DataSet         *DataSetRef       // nil when a new data set will be created
	DataSetMetadata map[string]string // metadata carried into the new data set if created
}

// ContextFactoryOptions configures construction of one provider context.
type ContextFactoryOptions struct {
	DataSetMetadata map[string]string
	WithCDN         bool
}

// ContextFactory builds an unbound immutable provider context.
type ContextFactory func(Provider, ContextFactoryOptions) (*ProviderContext, error)

// ServiceResolverOptions configures a ServiceResolver.
type ServiceResolverOptions struct {
	Payer      common.Address // EVM address of the paying account
	SPRegistry PDPProviderSource
	// Endorsements is required only when automatic upload selection uses its
	// default endorsed-primary policy. It may be nil when callers always set
	// AllowUnendorsedPrimary or use explicit provider contexts.
	Endorsements     EndorsedProviderSource
	WarmStorage      DataSetCatalog
	DataSetValidator DataSetValidator
	DataSetDetails   DataSetDetailsCatalog
	// ProviderPing checks one automatically selected provider endpoint. The
	// resolver may call it concurrently for up to 16 candidates per selection;
	// concurrent selections have independent limits. Each call receives a
	// context whose deadline is no later than eight seconds; a shorter parent
	// deadline takes precedence. Implementations must be safe for concurrent use
	// and honor the context. nil uses [pdp.Client.Ping] with up to two retries for
	// transient failures.
	ProviderPing func(context.Context, string) error
	NewContext   ContextFactory // called per-provider to construct an upload context
}

// ServiceResolver opens explicit targets and selects providers and data sets
// for uploads. Automatic selection prefers writable matching data sets with
// active pieces, then writable empty matches, in stable provider order.
type ServiceResolver struct {
	payer            common.Address
	spRegistry       PDPProviderSource
	endorsements     EndorsedProviderSource
	warmStorage      DataSetCatalog
	dataSetActivity  dataSetActivityReader
	dataSetValidator DataSetValidator
	dataSetDetails   DataSetDetailsCatalog
	providerPing     func(context.Context, string) error
	newContext       ContextFactory
}

var (
	_ PDPProviderSource        = (*spregistry.Service)(nil)
	_ EndorsedProviderSource   = (*spregistry.Service)(nil)
	_ DataSetCatalog           = (*warmstorage.Service)(nil)
	_ dataSetActivityReader    = (*warmstorage.Service)(nil)
	_ pdpVerifierAddressReader = (*warmstorage.Service)(nil)
	_ DataSetValidator         = (*warmstorage.Service)(nil)
	_ DataSetDetailsCatalog    = (*warmstorage.Service)(nil)
	_ UploadResolver           = (*ServiceResolver)(nil)
	_ ContextResolver          = (*ServiceResolver)(nil)
	_ ContextSelector          = (*ServiceResolver)(nil)
	_ ProviderResolver         = (*ServiceResolver)(nil)
)

// NewServiceResolver constructs a ServiceResolver. Payer, SPRegistry,
// WarmStorage, and NewContext are required. DataSetValidator, DataSetDetails,
// and active-piece reads are optional and auto-detected from WarmStorage when
// available and configured. ProviderPing is optional and defaults to a PDP
// ping with up to two retries for transient failures.
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
	validator := opts.DataSetValidator
	if validator == nil {
		validator, _ = opts.WarmStorage.(DataSetValidator)
	}
	details := opts.DataSetDetails
	if details == nil {
		details, _ = opts.WarmStorage.(DataSetDetailsCatalog)
	}
	activity, _ := opts.WarmStorage.(dataSetActivityReader)
	if addresses, ok := opts.WarmStorage.(pdpVerifierAddressReader); ok {
		addresses = normalizeOptional(addresses)
		if addresses != nil && addresses.PDPVerifierAddress() == (common.Address{}) {
			activity = nil
		}
	}
	providerPing := opts.ProviderPing
	if providerPing == nil {
		providerPing = defaultProviderPing
	}
	return &ServiceResolver{
		payer:            opts.Payer,
		spRegistry:       opts.SPRegistry,
		endorsements:     normalizeOptional(opts.Endorsements),
		warmStorage:      opts.WarmStorage,
		dataSetActivity:  activity,
		dataSetValidator: validator,
		dataSetDetails:   details,
		providerPing:     providerPing,
		newContext:       opts.NewContext,
	}, nil
}

func defaultProviderPing(ctx context.Context, serviceURL string) error {
	client, err := pdp.New(serviceURL, pdp.WithMaxRetries(2))
	if err != nil {
		return err
	}
	return client.Ping(ctx)
}

// ResolveProvider resolves one PDP provider by ID.
func (r *ServiceResolver) ResolveProvider(ctx context.Context, providerID types.BigInt) (Provider, error) {
	if providerID.IsZero() {
		return Provider{}, fmt.Errorf("storage.ServiceResolver.ResolveProvider: %w: zero providerID", ErrInvalidArgument)
	}
	provider, err := r.spRegistry.GetPDPProvider(ctx, providerID)
	if err != nil {
		return Provider{}, fmt.Errorf("storage.ServiceResolver.ResolveProvider: %w", err)
	}
	if provider == nil {
		return Provider{}, fmt.Errorf("storage.ServiceResolver.ResolveProvider: nil provider")
	}
	return buildProvider(*provider), nil
}

// ResolveProviderContext opens a registered provider without selection checks.
func (r *ServiceResolver) ResolveProviderContext(ctx context.Context, providerID types.BigInt, opts NewProviderContextOptions) (*ProviderContext, error) {
	const op = "storage.ServiceResolver.ResolveProviderContext"
	providerID = copyBigInt(providerID)
	opts = NewProviderContextOptions{
		DataSetMetadata: cloneStringMap(opts.DataSetMetadata),
		WithCDN:         copyBoolPtr(opts.WithCDN),
	}
	provider, err := r.ResolveProvider(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return r.buildProviderContext(op, provider, ContextFactoryOptions{
		DataSetMetadata: cloneStringMap(opts.DataSetMetadata),
		WithCDN:         resolveBoolDefault(opts.WithCDN, false),
	})
}

// ResolveDataSetContext opens an existing payer-owned data set without
// requiring it to be writable.
func (r *ServiceResolver) ResolveDataSetContext(ctx context.Context, dataSetID types.BigInt, opts NewDataSetContextOptions) (*DataSetContext, error) {
	const op = "storage.ServiceResolver.ResolveDataSetContext"
	dataSetID = copyBigInt(dataSetID)
	opts = cloneNewDataSetContextOptions(opts)
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("%s: %w: zero dataSetID", op, ErrInvalidArgument)
	}
	dataSet, err := r.warmStorage.GetDataSet(ctx, dataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: get data set %s: %w", op, dataSetID.String(), err)
	}
	if dataSet == nil {
		return nil, fmt.Errorf("%s: %w: nil data set", op, ErrInvalidArgument)
	}
	if dataSet.Payer != r.payer {
		return nil, fmt.Errorf("%s: %w: data set %s is not owned by payer %s", op, ErrInvalidArgument, dataSetID.String(), r.payer.Hex())
	}
	if dataSet.ProviderID.IsZero() {
		return nil, fmt.Errorf("%s: %w: data set has zero providerID", op, ErrInvalidArgument)
	}
	if opts.ProviderID != nil && !dataSet.ProviderID.Equal(*opts.ProviderID) {
		return nil, fmt.Errorf("%s: %w: data set providerID %s does not match requested providerID %s", op, ErrInvalidArgument, dataSet.ProviderID.String(), opts.ProviderID.String())
	}
	provider, err := r.ResolveProvider(ctx, dataSet.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	providerCtx, err := r.buildProviderContext(op, provider, ContextFactoryOptions{WithCDN: resolveBoolDefault(opts.WithCDN, false)})
	if err != nil {
		return nil, err
	}
	ref, err := NewDataSetRef(dataSet.ProviderID, dataSetID, dataSet.ClientDataSetID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	return providerCtx.ForDataSet(ref)
}

// SelectProviderContext selects one approved, active, healthy provider and
// returns it without a data-set binding.
func (r *ServiceResolver) SelectProviderContext(ctx context.Context, opts SelectProviderContextOptions) (*ProviderContext, error) {
	const op = "storage.ServiceResolver.SelectProviderContext"
	opts = SelectProviderContextOptions{
		ExcludeProviderIDs: cloneBigIntSlice(opts.ExcludeProviderIDs),
		DataSetMetadata:    cloneStringMap(opts.DataSetMetadata),
		WithCDN:            copyBoolPtr(opts.WithCDN),
	}
	selections, err := r.selectWithRetry(ctx, &UploadOptions{
		Copies:                 1,
		ExcludeProviderIDs:     cloneBigIntSlice(opts.ExcludeProviderIDs),
		DataSetMetadata:        cloneStringMap(opts.DataSetMetadata),
		AllowUnendorsedPrimary: true,
		WithCDN:                copyBoolPtr(opts.WithCDN),
	}, nil, false)
	if err != nil {
		return nil, err
	}
	if len(selections) == 0 {
		return nil, errors.New(op + ": no provider selected")
	}
	return r.buildProviderContext(op, selections[0].Provider, ContextFactoryOptions{
		DataSetMetadata: cloneStringMap(selections[0].DataSetMetadata),
		WithCDN:         resolveBoolDefault(opts.WithCDN, false),
	})
}

// SelectUploadContexts selects writable targets in stable provider order.
func (r *ServiceResolver) SelectUploadContexts(ctx context.Context, opts SelectUploadContextsOptions) (*UploadContextSelection, error) {
	const op = "storage.ServiceResolver.SelectUploadContexts"
	opts = SelectUploadContextsOptions{
		Copies:                 opts.Copies,
		ExcludeProviderIDs:     cloneBigIntSlice(opts.ExcludeProviderIDs),
		DataSetMetadata:        cloneStringMap(opts.DataSetMetadata),
		AllowUnendorsedPrimary: opts.AllowUnendorsedPrimary,
		WithCDN:                copyBoolPtr(opts.WithCDN),
	}
	if opts.Copies <= 0 {
		return nil, fmt.Errorf("%s: %w: Copies must be greater than zero", op, ErrInvalidArgument)
	}
	selections, err := r.selectWithRetry(ctx, &UploadOptions{
		Copies:                 opts.Copies,
		ExcludeProviderIDs:     cloneBigIntSlice(opts.ExcludeProviderIDs),
		DataSetMetadata:        cloneStringMap(opts.DataSetMetadata),
		AllowUnendorsedPrimary: opts.AllowUnendorsedPrimary,
		WithCDN:                copyBoolPtr(opts.WithCDN),
	}, nil, true)
	if err != nil {
		return nil, err
	}
	contexts, err := r.buildStorageContexts(op, selections, resolveBoolDefault(opts.WithCDN, false))
	if err != nil {
		return nil, err
	}
	result := &UploadContextSelection{
		Contexts:        contexts,
		RequestedCopies: opts.Copies,
		Complete:        len(contexts) == opts.Copies,
	}
	if !result.Complete {
		return result, &InsufficientUploadContextsError{Requested: opts.Copies, Available: len(contexts)}
	}
	return result, nil
}

// ResolveUploadContexts adapts automatic upload options to context selection.
func (r *ServiceResolver) ResolveUploadContexts(ctx context.Context, opts *UploadOptions) ([]StorageContext, bool, error) {
	if opts == nil || opts.Copies <= 0 {
		return nil, false, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: %w: Copies must be greater than zero", ErrInvalidArgument)
	}
	selection, err := r.SelectUploadContexts(ctx, SelectUploadContextsOptions{
		Copies:                 opts.Copies,
		ExcludeProviderIDs:     cloneBigIntSlice(opts.ExcludeProviderIDs),
		DataSetMetadata:        cloneStringMap(opts.DataSetMetadata),
		AllowUnendorsedPrimary: opts.AllowUnendorsedPrimary,
		WithCDN:                copyBoolPtr(opts.WithCDN),
	})
	if err != nil && !errors.Is(err, ErrInsufficientUploadContexts) {
		return nil, false, err
	}
	return selection.Contexts, false, nil
}

func (r *ServiceResolver) resolveWritableUploadContexts(ctx context.Context, opts *UploadOptions) ([]StorageContext, bool, error) {
	return r.ResolveUploadContexts(ctx, opts)
}

// SelectReplacement selects one writable provider not already used.
func (r *ServiceResolver) SelectReplacement(ctx context.Context, usedProviders map[string]types.BigInt, opts *UploadOptions) (StorageContext, error) {
	return r.selectReplacement(ctx, usedProviders, opts)
}

func (r *ServiceResolver) selectWritableReplacement(ctx context.Context, usedProviders map[string]types.BigInt, opts *UploadOptions) (StorageContext, error) {
	return r.selectReplacement(ctx, usedProviders, opts)
}

func (r *ServiceResolver) selectReplacement(ctx context.Context, usedProviders map[string]types.BigInt, opts *UploadOptions) (StorageContext, error) {
	const op = "storage.ServiceResolver.SelectReplacement"
	if opts == nil {
		return nil, fmt.Errorf("%s: %w: nil upload options", op, ErrInvalidArgument)
	}
	selectionOpts := withCopies(opts, 1)
	selectionOpts.AllowUnendorsedPrimary = true
	selections, err := r.selectWithRetry(ctx, selectionOpts, usedProviders, true)
	if err != nil {
		return nil, err
	}
	if len(selections) == 0 {
		return nil, errors.New(op + ": no remaining providers")
	}
	contexts, err := r.buildStorageContexts(op, selections[:1], resolveBoolDefault(opts.WithCDN, false))
	if err != nil {
		return nil, err
	}
	return contexts[0], nil
}

func (r *ServiceResolver) selectWithRetry(ctx context.Context, opts *UploadOptions, extraExcludes map[string]types.BigInt, reuseDataSets bool) ([]resolvedUploadContext, error) {
	return retry.Do(ctx, func(ctx context.Context) ([]resolvedUploadContext, error) {
		return r.autoSelect(ctx, opts, extraExcludes, reuseDataSets)
	},
		retry.WithMaxRetries(3),
		retry.WithInitialDelay(selectorRetryInitialDelay),
		retry.WithMaxDelay(2*time.Second),
		retry.WithRetryIf(txutil.IsRetryableRPCError),
	)
}

func (r *ServiceResolver) buildProviderContext(op string, provider Provider, opts ContextFactoryOptions) (*ProviderContext, error) {
	providerCtx, err := r.newContext(provider, ContextFactoryOptions{
		DataSetMetadata: cloneStringMap(opts.DataSetMetadata),
		WithCDN:         opts.WithCDN,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: build context for provider %s: %w", op, provider.ID.String(), err)
	}
	if providerCtx == nil || providerCtx.core == nil {
		return nil, fmt.Errorf("%s: %w: factory returned nil provider context", op, ErrInvalidArgument)
	}
	if !providerCtx.ProviderID().Equal(provider.ID) {
		return nil, fmt.Errorf("%s: %w: factory returned providerID %s, want %s", op, ErrInvalidArgument, providerCtx.ProviderID().String(), provider.ID.String())
	}
	if _, bound := providerCtx.DataSetRef(); bound {
		return nil, fmt.Errorf("%s: %w: factory returned bound context", op, ErrInvalidArgument)
	}
	return providerCtx, nil
}

func (r *ServiceResolver) buildStorageContexts(op string, selections []resolvedUploadContext, withCDN bool) ([]StorageContext, error) {
	contexts := make([]StorageContext, 0, len(selections))
	for _, selection := range selections {
		providerCtx, err := r.buildProviderContext(op, selection.Provider, ContextFactoryOptions{
			DataSetMetadata: selection.DataSetMetadata,
			WithCDN:         withCDN,
		})
		if err != nil {
			return nil, err
		}
		if selection.DataSet == nil {
			contexts = append(contexts, providerCtx)
			continue
		}
		dataSetCtx, err := providerCtx.ForDataSet(*selection.DataSet)
		if err != nil {
			return nil, fmt.Errorf("%s: bind data set: %w", op, err)
		}
		contexts = append(contexts, dataSetCtx)
	}
	return contexts, nil
}

func (r *ServiceResolver) autoSelect(ctx context.Context, opts *UploadOptions, extraExcludes map[string]types.BigInt, reuseDataSets bool) ([]resolvedUploadContext, error) {
	count := opts.Copies
	if count <= 0 {
		return nil, fmt.Errorf("storage.ServiceResolver.SelectUploadContexts: %w: Copies must be greater than zero", ErrInvalidArgument)
	}
	requireEndorsedPrimary := !opts.AllowUnendorsedPrimary
	var endorsedSet map[string]struct{}
	if requireEndorsedPrimary {
		if r.endorsements == nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: %w", ErrEndorsementsNotConfigured)
		}
		endorsedIDs, err := r.endorsements.GetEndorsedProviderIDs(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get endorsed providers: %w", err)
		}
		if len(endorsedIDs) == 0 {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: %w: endorsement set is empty", ErrNoEndorsedProvider)
		}
		endorsedSet = make(map[string]struct{}, len(endorsedIDs))
		for _, id := range endorsedIDs {
			endorsedSet[idconv.Key(id)] = struct{}{}
		}
	}
	approvedIDs, err := r.getAllApprovedProviderIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get approved providers: %w", err)
	}
	if len(approvedIDs) == 0 {
		if requireEndorsedPrimary {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: %w: no approved providers", ErrNoEndorsedProvider)
		}
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

	var detailedDataSets []*warmstorage.EnhancedDataSetInfo
	if reuseDataSets && r.dataSetDetails != nil {
		detailedDataSets, err = r.dataSetDetails.GetClientDataSetsWithDetails(ctx, r.payer, true)
		if err != nil {
			if errors.Is(err, warmstorage.ErrPDPVerifierNotConfigured) || errors.Is(err, ErrDataSetUnavailable) {
				detailedDataSets = nil
			} else {
				if ctxErr := ctx.Err(); ctxErr != nil {
					err = ctxErr
				}
				return nil, fmt.Errorf("storage.ServiceResolver.SelectUploadContexts: get client data set details: %w", err)
			}
		}
	}
	var basicDataSets []*warmstorage.DataSetInfo
	if reuseDataSets && len(detailedDataSets) == 0 {
		basicDataSets, err = r.getAllClientDataSets(ctx)
		if err != nil {
			return nil, fmt.Errorf("storage.ServiceResolver.SelectUploadContexts: get client data sets: %w", err)
		}
	}
	var providersWithDetails map[string][]*warmstorage.EnhancedDataSetInfo
	if len(detailedDataSets) > 0 {
		selectableProviders := selectableProviderSet(providers, approvedSet)
		providersWithDetails = detailedCandidateProviders(detailedDataSets, selectableProviders)
	}
	requestedMetadata := dataSetMetadataFromOptions(opts)
	withDataSet := make([]autoSelectCandidate, 0, min(count, len(providers)))
	withoutDataSet := make([]autoSelectCandidate, 0, min(count, len(providers)))
	seenProviderIDs := make(map[string]struct{}, min(count, len(providers)))
	for i := range providers {
		provider := &providers[i]
		providerKey := idconv.Key(provider.Info.ID)
		if _, ok := approvedSet[providerKey]; !ok {
			continue
		}
		if _, ok := seenProviderIDs[providerKey]; ok {
			continue
		}
		seenProviderIDs[providerKey] = struct{}{}

		var dataSetID, clientDataSetID *types.BigInt
		var metadata map[string]string
		if providerDataSets, ok := providersWithDetails[providerKey]; ok {
			dataSetID, clientDataSetID, metadata = selectMatchingDetailedDataSet(provider.Info.ID, providerDataSets, requestedMetadata)
		} else if reuseDataSets && len(basicDataSets) > 0 {
			dataSetID, clientDataSetID, metadata, err = r.selectMatchingDataSetWithWritable(ctx, provider.Info.ID, basicDataSets, requestedMetadata, true)
			if err != nil {
				return nil, err
			}
		}
		candidate := autoSelectCandidate{
			provider:        provider,
			dataSetID:       dataSetID,
			clientDataSetID: clientDataSetID,
			metadata:        metadata,
		}
		if dataSetID != nil {
			withDataSet = append(withDataSet, candidate)
		} else {
			candidate.metadata = requestedMetadata
			withoutDataSet = append(withoutDataSet, candidate)
		}
	}
	candidates := slices.Concat(withDataSet, withoutDataSet)
	if !requireEndorsedPrimary {
		return r.selectHealthyCandidates(ctx, candidates, count, ErrNoHealthyProviders, nil)
	}
	probes := newProviderProbeCache(r.providerPing)

	primaryCandidates := make([]autoSelectCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := endorsedSet[idconv.Key(candidate.provider.Info.ID)]; ok {
			primaryCandidates = append(primaryCandidates, candidate)
		}
	}
	primary, err := r.selectHealthyCandidates(ctx, primaryCandidates, 1, ErrNoEndorsedProvider, probes)
	if err != nil {
		return nil, err
	}
	if count == 1 {
		return primary, nil
	}

	primaryKey := idconv.Key(primary[0].Provider.ID)
	secondaryCandidates := make([]autoSelectCandidate, 0, len(candidates)-1)
	for _, candidate := range candidates {
		if idconv.Key(candidate.provider.Info.ID) != primaryKey {
			secondaryCandidates = append(secondaryCandidates, candidate)
		}
	}
	if len(secondaryCandidates) == 0 {
		return primary, nil
	}
	secondaries, err := r.selectHealthyCandidates(ctx, secondaryCandidates, count-1, ErrNoHealthyProviders, probes)
	if err != nil {
		if errors.Is(err, ErrNoHealthyProviders) {
			return primary, nil
		}
		return nil, err
	}
	return append(primary, secondaries...), nil
}

func (r *ServiceResolver) selectHealthyCandidates(
	ctx context.Context,
	candidates []autoSelectCandidate,
	count int,
	noHealthyError error,
	probes *providerProbeCache,
) ([]resolvedUploadContext, error) {
	if len(candidates) == 0 {
		if errors.Is(noHealthyError, ErrNoEndorsedProvider) {
			return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: %w: no eligible providers", noHealthyError)
		}
		return nil, errors.New("storage.ServiceResolver.ResolveUploadContexts: no eligible providers")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: health-check providers: %w", err)
	}

	probeCtx, cancelProbes := context.WithCancel(ctx)
	defer cancelProbes()
	states := make([]providerProbeState, len(candidates))
	probeErrors := make([]error, len(candidates))
	results := make(chan providerProbeResult, min(providerPingConcurrency, len(candidates)))
	probeCancels := make([]context.CancelFunc, len(candidates))
	nextIndex := 0
	inFlight := 0
	probeLimit := len(candidates)
	stopping := false
	selectionDetermined := false

	startProbes := func() {
		for !stopping && inFlight < providerPingConcurrency && nextIndex < probeLimit {
			index := nextIndex
			nextIndex++
			inFlight++
			pingCtx, cancel := context.WithTimeout(probeCtx, providerPingTimeout)
			probeCancels[index] = cancel
			go func() {
				provider := candidates[index].provider
				var err error
				if probes == nil {
					err = r.providerPing(pingCtx, provider.Offering.ServiceURL)
				} else {
					err = probes.probe(pingCtx, idconv.Key(provider.Info.ID), provider.Offering.ServiceURL)
				}
				cancel()
				results <- providerProbeResult{index: index, err: err}
			}()
		}
	}
	cancelProbesFrom := func(index int) {
		for i := index; i < nextIndex; i++ {
			if probeCancels[i] != nil {
				probeCancels[i]()
			}
		}
	}

	startProbes()
	for inFlight > 0 {
		result := <-results
		inFlight--
		probeCancels[result.index] = nil
		if !stopping {
			if ctxErr := ctx.Err(); ctxErr != nil {
				stopping = true
				cancelProbes()
			} else if result.index < probeLimit {
				if result.err == nil {
					states[result.index] = providerProbeHealthy
				} else {
					states[result.index] = providerProbeUnhealthy
					probeErrors[result.index] = fmt.Errorf(
						"provider %s (%s): %w",
						candidates[result.index].provider.Info.ID.String(),
						candidates[result.index].provider.Offering.ServiceURL,
						result.err,
					)
				}
				frontier, ready := providerSelectionProgress(states, count)
				if frontier >= 0 && frontier+1 < probeLimit {
					// Later candidates cannot enter the stable ranking once this
					// prefix already contains the requested healthy copies.
					probeLimit = frontier + 1
					cancelProbesFrom(probeLimit)
				}
				if ready {
					selectionDetermined = true
					stopping = true
					cancelProbes()
				}
			}
		}
		startProbes()
	}

	if err := ctx.Err(); err != nil && !selectionDetermined {
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: health-check providers: %w", err)
	}
	selected := make([]resolvedUploadContext, 0, min(count, len(candidates)))
	failedProviderIDs := make([]types.BigInt, 0, min(count, len(candidates)))
	for i, state := range states {
		switch state {
		case providerProbeHealthy:
			candidate := candidates[i]
			selection, err := buildResolvedUploadContext(
				*candidate.provider,
				candidate.dataSetID,
				candidate.clientDataSetID,
				candidate.metadata,
			)
			if err != nil {
				return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: provider %s: %w", candidate.provider.Info.ID.String(), err)
			}
			selected = append(selected, selection)
			if len(selected) == count {
				return selected, nil
			}
		case providerProbeUnhealthy:
			failedProviderIDs = append(failedProviderIDs, candidates[i].provider.Info.ID)
		}
	}

	if len(selected) == 0 {
		if len(failedProviderIDs) == 0 {
			if noHealthyError != nil {
				return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: %w: no eligible providers", noHealthyError)
			}
			return nil, errors.New("storage.ServiceResolver.ResolveUploadContexts: no remaining providers")
		}
		failures := make([]error, 0, len(failedProviderIDs)+1)
		failures = append(failures, fmt.Errorf("%w (provider IDs: %s)", noHealthyError, formatProviderIDs(failedProviderIDs)))
		for _, err := range probeErrors {
			if err != nil {
				failures = append(failures, err)
			}
		}
		return nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: health-check providers: %w", errors.Join(failures...))
	}
	return selected, nil
}

func providerSelectionProgress(states []providerProbeState, count int) (frontier int, ready bool) {
	healthy := 0
	pending := false
	for i, state := range states {
		if state == providerProbePending {
			pending = true
		}
		if state == providerProbeHealthy {
			healthy++
			if healthy == count {
				return i, !pending
			}
		}
	}
	return -1, false
}

func formatProviderIDs(ids []types.BigInt) string {
	values := make([]string, len(ids))
	for i, id := range ids {
		values[i] = id.String()
	}
	return strings.Join(values, ", ")
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

func (r *ServiceResolver) selectMatchingDataSetWithWritable(ctx context.Context, providerID types.BigInt, dataSets []*warmstorage.DataSetInfo, requestedMetadata map[string]string, requireWritable bool) (*types.BigInt, *types.BigInt, map[string]string, error) {
	matching := make([]*warmstorage.DataSetInfo, 0, len(dataSets))
	for _, dataSet := range dataSets {
		if dataSet == nil {
			continue
		}
		if dataSet.DataSetID.IsZero() || !dataSet.ProviderID.Equal(providerID) || dataSet.PDPEndEpoch != 0 {
			continue
		}
		matching = append(matching, dataSet)
	}
	slices.SortFunc(matching, func(a, b *warmstorage.DataSetInfo) int {
		return a.DataSetID.Cmp(b.DataSetID)
	})
	if len(matching) == 0 {
		return nil, nil, cloneStringMap(requestedMetadata), nil
	}
	if r.dataSetActivity == nil {
		return r.selectFirstMatchingDataSet(ctx, matching, requestedMetadata, requireWritable)
	}
	return r.selectPreferredMatchingDataSet(ctx, matching, requestedMetadata, requireWritable)
}

func (r *ServiceResolver) selectFirstMatchingDataSet(ctx context.Context, matching []*warmstorage.DataSetInfo, requestedMetadata map[string]string, requireWritable bool) (*types.BigInt, *types.BigInt, map[string]string, error) {
	for _, dataSet := range matching {
		metadata, err := r.warmStorage.GetAllDataSetMetadata(ctx, dataSet.DataSetID)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set metadata %s: %w", dataSet.DataSetID.String(), err)
		}
		if metadataMatches(metadata, requestedMetadata) {
			if requireWritable {
				ok, err := r.dataSetAcceptsUpload(ctx, dataSet.DataSetID)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: validate data set %s: %w", dataSet.DataSetID.String(), err)
				}
				if !ok {
					continue
				}
			}
			dsID := dataSet.DataSetID
			return &dsID, copyClientDataSetIDPtr(dataSet.ClientDataSetID), metadata, nil
		}
	}

	return nil, nil, cloneStringMap(requestedMetadata), nil
}

type evaluatedDataSet struct {
	dataSet  *warmstorage.DataSetInfo
	metadata map[string]string
}

func (r *ServiceResolver) selectPreferredMatchingDataSet(ctx context.Context, matching []*warmstorage.DataSetInfo, requestedMetadata map[string]string, requireWritable bool) (*types.BigInt, *types.BigInt, map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}

	results := make([]*evaluatedDataSet, len(matching))
	firstMatch := len(matching)
	bestNonEmpty := len(matching)
	var mu sync.Mutex

	err := runResolveWindow(
		ctx,
		len(matching),
		func(index int) bool {
			mu.Lock()
			defer mu.Unlock()
			return index <= bestNonEmpty
		},
		func(groupCtx context.Context, index int) error {
			dataSet := matching[index]
			mu.Lock()
			if index > bestNonEmpty {
				mu.Unlock()
				return nil
			}
			mu.Unlock()

			metadata, err := r.warmStorage.GetAllDataSetMetadata(groupCtx, dataSet.DataSetID)
			if err != nil {
				return fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: get data set metadata %s: %w", dataSet.DataSetID.String(), err)
			}
			if !metadataMatches(metadata, requestedMetadata) {
				return nil
			}

			mu.Lock()
			if index > bestNonEmpty {
				mu.Unlock()
				return nil
			}
			mu.Unlock()

			if requireWritable {
				ok, err := r.dataSetAcceptsUpload(groupCtx, dataSet.DataSetID)
				if err != nil {
					return fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: validate data set %s: %w", dataSet.DataSetID.String(), err)
				}
				if !ok {
					return nil
				}
			}

			mu.Lock()
			if index < firstMatch {
				firstMatch = index
			}
			if index > bestNonEmpty {
				mu.Unlock()
				return nil
			}
			mu.Unlock()

			hasPieces, err := r.dataSetActivity.HasActivePieces(groupCtx, dataSet.DataSetID)
			if err != nil {
				return fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: check active pieces for data set %s: %w", dataSet.DataSetID.String(), err)
			}

			mu.Lock()
			results[index] = &evaluatedDataSet{dataSet: dataSet, metadata: metadata}
			if hasPieces && index < bestNonEmpty {
				bestNonEmpty = index
			}
			mu.Unlock()
			return nil
		},
	)
	if err != nil {
		return nil, nil, nil, err
	}

	selected := firstMatch
	if bestNonEmpty < len(matching) {
		selected = bestNonEmpty
	}
	if selected == len(matching) {
		return nil, nil, cloneStringMap(requestedMetadata), nil
	}
	result := results[selected]
	if result == nil {
		return nil, nil, nil, fmt.Errorf("storage.ServiceResolver.ResolveUploadContexts: selected data set index %d was not evaluated", selected)
	}
	dsID := result.dataSet.DataSetID
	return &dsID, copyClientDataSetIDPtr(result.dataSet.ClientDataSetID), result.metadata, nil
}

// runResolveWindow keeps a sliding set of evaluations in flight and consults
// shouldStart after every completion before filling the newly available slot.
func runResolveWindow(ctx context.Context, count int, shouldStart func(int) bool, evaluate func(context.Context, int) error) error {
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(resolveConcurrency)
	completed := make(chan struct{}, resolveConcurrency)
	nextIndex := 0
	inFlight := 0
	failed := false
	var mu sync.Mutex

	for nextIndex < count || inFlight > 0 {
		for inFlight < resolveConcurrency && nextIndex < count {
			mu.Lock()
			stop := failed
			mu.Unlock()
			if stop || groupCtx.Err() != nil || !shouldStart(nextIndex) {
				break
			}

			index := nextIndex
			nextIndex++
			inFlight++
			group.Go(func() error {
				err := evaluate(groupCtx, index)
				if err != nil {
					mu.Lock()
					failed = true
					mu.Unlock()
				}
				completed <- struct{}{}
				return err
			})
		}

		if inFlight == 0 {
			break
		}
		<-completed
		inFlight--

		mu.Lock()
		stop := failed
		mu.Unlock()
		if stop || groupCtx.Err() != nil {
			break
		}
	}

	if err := group.Wait(); err != nil {
		return err
	}
	return ctx.Err()
}

func selectableProviderSet(providers []spregistry.PDPProvider, approvedSet map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, min(len(providers), len(approvedSet)))
	for _, provider := range providers {
		key := idconv.Key(provider.Info.ID)
		if _, ok := approvedSet[key]; !ok {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func detailedCandidateProviders(dataSets []*warmstorage.EnhancedDataSetInfo, selectableProviders map[string]struct{}) map[string][]*warmstorage.EnhancedDataSetInfo {
	out := make(map[string][]*warmstorage.EnhancedDataSetInfo, min(len(dataSets), len(selectableProviders)))
	for _, dataSet := range dataSets {
		if dataSet == nil || dataSet.DataSetInfo == nil {
			continue
		}
		if dataSet.ProviderID.IsZero() {
			continue
		}
		key := idconv.Key(dataSet.ProviderID)
		if _, ok := selectableProviders[key]; !ok {
			continue
		}
		out[key] = append(out[key], dataSet)
	}
	return out
}

func selectMatchingDetailedDataSet(providerID types.BigInt, dataSets []*warmstorage.EnhancedDataSetInfo, requestedMetadata map[string]string) (*types.BigInt, *types.BigInt, map[string]string) {
	var best *warmstorage.EnhancedDataSetInfo
	var bestHasPieces bool
	for _, dataSet := range dataSets {
		if dataSet == nil || dataSet.DataSetInfo == nil {
			continue
		}
		if dataSet.DataSetID.IsZero() || !dataSet.ProviderID.Equal(providerID) {
			continue
		}
		if dataSet.PDPEndEpoch != 0 || !dataSet.IsLive || !dataSet.IsManaged {
			continue
		}
		if !metadataMatches(dataSet.Metadata, requestedMetadata) {
			continue
		}
		hasPieces := dataSet.HasActivePieces
		if best == nil ||
			(hasPieces && !bestHasPieces) ||
			(hasPieces == bestHasPieces && dataSet.DataSetID.Cmp(best.DataSetID) < 0) {
			best = dataSet
			bestHasPieces = hasPieces
		}
	}
	if best == nil {
		return nil, nil, nil
	}
	return resolvedDetailedDataSet(best)
}

func (r *ServiceResolver) dataSetAcceptsUpload(ctx context.Context, dataSetID types.BigInt) (bool, error) {
	if r.dataSetValidator == nil {
		return false, nil
	}
	if err := r.dataSetValidator.ValidateDataSet(ctx, dataSetID); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return false, ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return false, err
		}
		if errors.Is(err, ErrDataSetUnavailable) {
			return false, nil
		}
		if errors.Is(err, warmstorage.ErrPDPVerifierNotConfigured) {
			return false, nil
		}
		if _, ok := errors.AsType[*warmstorage.DataSetNotManagedError](err); ok {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func resolvedDetailedDataSet(dataSet *warmstorage.EnhancedDataSetInfo) (*types.BigInt, *types.BigInt, map[string]string) {
	dsID := dataSet.DataSetID
	return &dsID, copyClientDataSetIDPtr(dataSet.ClientDataSetID), dataSet.Metadata
}

func buildResolvedUploadContext(provider spregistry.PDPProvider, dataSetID, clientDataSetID *types.BigInt, metadata map[string]string) (resolvedUploadContext, error) {
	selection := resolvedUploadContext{
		Provider:        buildProvider(provider),
		DataSetMetadata: cloneStringMap(metadata),
	}
	if dataSetID == nil {
		if clientDataSetID != nil {
			return resolvedUploadContext{}, fmt.Errorf("%w: clientDataSetID without dataSetID", ErrInvalidArgument)
		}
		return selection, nil
	}
	if clientDataSetID == nil {
		return resolvedUploadContext{}, fmt.Errorf("%w: missing clientDataSetID for dataSetID %s", ErrInvalidArgument, dataSetID.String())
	}
	ref, err := NewDataSetRef(provider.Info.ID, *dataSetID, *clientDataSetID)
	if err != nil {
		return resolvedUploadContext{}, err
	}
	selection.DataSet = &ref
	return selection, nil
}

func buildProvider(provider spregistry.PDPProvider) Provider {
	return Provider{
		ID:              copyBigInt(provider.Info.ID),
		ServiceURL:      provider.Offering.ServiceURL,
		ServiceProvider: provider.Info.ServiceProvider,
		Payee:           provider.Info.Payee,
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
	if len(opts.ExcludeProviderIDs) != 0 {
		cloned.ExcludeProviderIDs = cloneBigIntSlice(opts.ExcludeProviderIDs)
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
