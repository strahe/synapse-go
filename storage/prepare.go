package storage

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
)

// PrepareOptions configures Service.Prepare. At most one of Costs or
// Contexts should be set; when neither is provided, Prepare will call
// CreateContexts to obtain a default set.
type PrepareOptions struct {
	// DataSize is the payload size (bytes) used to derive per-upload
	// costs when Costs is not pre-supplied.
	DataSize uint64
	// Contexts (optional) restricts cost calculation to the given set
	// of upload contexts. Each element should be a *Context.
	Contexts []UploadContext
	// Costs (optional) short-circuits calculation when supplied.
	Costs *MultiContextCosts
	// EnableCDN mirrors the TS `withCDN` flag. Tri-state:
	//   nil         → inherit the Client-level default (synapse.WithCDN)
	//   &true/&false → explicit per-Prepare override
	// Only consulted on the auto-create-context branch (i.e. when neither
	// Costs nor Contexts is supplied); once Contexts are provided, each
	// context's own WithCDN() takes precedence.
	EnableCDN *bool
	// ExtraRunwayEpochs is additional runway (epochs) above the
	// minimum lockup period passed through to the cost calculator. Defaults
	// to 0 when unset. Mirrors TS `prepare({extraRunwayEpochs})`.
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit cushion above current lockup usage
	// used to absorb transaction-latency epochs. Zero falls back to
	// the cost service default (5 epochs). Mirrors TS
	// `prepare({bufferEpochs})`.
	BufferEpochs int64
}

// PrepareTransaction is the deferred funding step returned by Prepare
// when the account is not yet Ready. Execute performs the top-up.
type PrepareTransaction struct {
	// DepositAmount is the USDFC amount that will be moved into the
	// payments account.
	DepositAmount *big.Int
	// IncludesApproval reports whether the call will also set the FWSS
	// operator to max allowance.
	IncludesApproval bool
	// Execute performs the funding operation. Additional
	// payments.WriteOption values are appended to the built-in
	// WithFundNeedsFwssApproval option when applicable.
	Execute func(ctx context.Context, opts ...payments.WriteOption) (*types.WriteResult, error)
}

// PrepareResult is the value returned by Service.Prepare.
type PrepareResult struct {
	// Costs is the aggregated cost calculation that drove the decision.
	Costs *MultiContextCosts
	// Transaction is non-nil only when funding is required (Ready=false).
	Transaction *PrepareTransaction
}

// Prepare returns the funding transaction needed (if any) to cover the
// upload costs across the given (or auto-selected) contexts. Mirrors TS
// StorageManager.prepare.
func (s *Service) Prepare(ctx context.Context, opts *PrepareOptions) (*PrepareResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if opts == nil {
		opts = &PrepareOptions{}
	}

	costs := opts.Costs
	if costs == nil {
		refs, err := s.prepareRefs(ctx, opts)
		if err != nil {
			return nil, err
		}
		if s.costCalc == nil {
			return nil, fmt.Errorf("storage.Service.Prepare: %w: no CostCalculator configured", ErrUninitialized)
		}
		payer := s.signerAddr
		if payer == (common.Address{}) {
			return nil, fmt.Errorf("storage.Service.Prepare: %w: zero payer and no default signer", ErrInvalidArgument)
		}
		size := new(big.Int).SetUint64(opts.DataSize)
		costs, err = s.costCalc.CalculateMultiContextCosts(ctx, payer, size, refs, MultiCostOptions{
			ExtraRunwayEpochs: opts.ExtraRunwayEpochs,
			BufferEpochs:      opts.BufferEpochs,
		})
		if err != nil {
			return nil, fmt.Errorf("storage.Service.Prepare: %w", err)
		}
	}

	if costs.Ready {
		return &PrepareResult{Costs: costs}, nil
	}

	if s.funder == nil {
		return nil, fmt.Errorf("storage.Service.Prepare: %w: no PaymentsFunder configured", ErrUninitialized)
	}

	deposit := costs.DepositNeeded
	needsApproval := costs.NeedsFWSSMaxApproval
	funder := s.funder

	return &PrepareResult{
		Costs: costs,
		Transaction: &PrepareTransaction{
			DepositAmount:    deposit,
			IncludesApproval: needsApproval,
			Execute: func(ctx context.Context, extraOpts ...payments.WriteOption) (*types.WriteResult, error) {
				if err := s.checkInit(); err != nil {
					return nil, err
				}
				optsOut := extraOpts
				if needsApproval {
					optsOut = append(optsOut, payments.WithFundNeedsFwssApproval(true))
				}
				return funder.FundSync(ctx, deposit, optsOut...)
			},
		},
	}, nil
}

// prepareContext is the narrow subset of *Context that prepareRefs
// consumes. Exported as a private interface so unit tests and alternate
// UploadContext implementations can participate without a hard type
// assertion to *Context. All public SDK context producers return values
// that satisfy this interface.
type prepareContext interface {
	DataSetID() *types.DataSetID
	GetProviderInfo() Provider
	WithCDN() bool
}

// prepareRefs builds the []ContextCostRef the cost calculator expects
// from the user-supplied Contexts (or by calling CreateContexts). For
// existing-dataset contexts, the current on-chain size is fetched in
// parallel via [DataSetSizeReader] so the cost calculator can price
// lockup against real storage usage rather than the floor rate.
func (s *Service) prepareRefs(ctx context.Context, opts *PrepareOptions) ([]ContextCostRef, error) {
	contexts := opts.Contexts

	if len(contexts) == 0 {
		// Forward EnableCDN unchanged: nil lets resolveWithCDN inherit the
		// Client-level DefaultWithCDN, matching TS StorageManager.prepare().
		created, err := s.CreateContexts(ctx, &CreateContextsOptions{WithCDN: opts.EnableCDN})
		if err != nil {
			return nil, fmt.Errorf("storage.Service.Prepare: CreateContexts: %w", err)
		}
		contexts = make([]UploadContext, len(created))
		for i, ctx := range created {
			contexts[i] = ctx
		}
	}

	refs := make([]ContextCostRef, len(contexts))
	type sizeJob struct {
		idx int
		id  types.DataSetID
	}
	var jobs []sizeJob
	for i, uc := range contexts {
		cc, ok := uc.(prepareContext)
		if !ok {
			return nil, fmt.Errorf("storage.Service.Prepare: %w: context must expose DataSetID/GetProviderInfo/WithCDN", ErrInvalidArgument)
		}
		refs[i] = ContextCostRef{
			DataSetID: cc.DataSetID(),
			Provider:  cc.GetProviderInfo(),
			WithCDN:   cc.WithCDN(),
		}
		if id := cc.DataSetID(); id != nil && s.sizeReader != nil {
			jobs = append(jobs, sizeJob{idx: i, id: *id})
		}
	}

	if len(jobs) > 0 {
		type sizeResult struct {
			idx  int
			size *big.Int
			err  error
		}
		results := make(chan sizeResult, len(jobs))
		for _, j := range jobs {
			go func(j sizeJob) {
				sz, err := s.sizeReader.GetDataSetSizeBytes(ctx, j.id)
				results <- sizeResult{idx: j.idx, size: sz, err: err}
			}(j)
		}
		for range jobs {
			r := <-results
			if r.err != nil {
				return nil, fmt.Errorf("storage.Service.Prepare: GetDataSetSizeBytes: %w", r.err)
			}
			refs[r.idx].CurrentDataSetSizeBytes = r.size
		}
	}

	return refs, nil
}
