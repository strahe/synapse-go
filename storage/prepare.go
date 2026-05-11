package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
)

// PrepareOptions configures Service.Prepare. Costs, explicit Contexts,
// and auto-created estimate contexts are mutually exclusive modes.
type PrepareOptions struct {
	// DataSize is required when Costs is nil, and must be zero when Costs
	// is set. It is the payload size in bytes used for cost calculation.
	DataSize uint64
	// Contexts restricts cost calculation to an explicit set of upload
	// contexts. It is valid only when Costs is nil. When set, EnableCDN
	// must be nil because each context already carries its own CDN state.
	Contexts []UploadContext
	// Costs short-circuits cost calculation. When set, no other
	// PrepareOptions fields are accepted.
	Costs *MultiContextCosts
	// EnableCDN is tri-state:
	//   nil         → inherit the Client-level default (synapse.WithCDN)
	//   &true/&false → explicit per-Prepare override
	// It is valid only on the auto-create-context branch, when Costs is
	// nil and Contexts is empty.
	EnableCDN *bool
	// ExtraRunwayEpochs is additional runway (epochs) above the
	// minimum lockup period passed through to the cost calculator. It is
	// valid only when Costs is nil and must be non-negative.
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit cushion above current lockup usage
	// used to absorb transaction-latency epochs. Zero falls back to
	// the cost service default. It is valid only when Costs is nil and
	// must be non-negative.
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

// Prepare returns the funding transaction needed, if any, to cover one
// upload of DataSize bytes across the given or auto-selected contexts.
// When Contexts is empty, Prepare creates contexts only for this estimate;
// it does not cache, reserve, or bind them to a later Upload call.
func (s *Service) Prepare(ctx context.Context, opts *PrepareOptions) (*PrepareResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := validatePrepareOptions(opts); err != nil {
		return nil, fmt.Errorf("storage.Service.Prepare: %w", err)
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
		if costs == nil {
			return nil, errors.New("storage.Service.Prepare: cost calculator returned nil costs")
		}
	}

	if err := validatePrepareCosts(costs); err != nil {
		return nil, fmt.Errorf("storage.Service.Prepare: %w", err)
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

func validatePrepareOptions(opts *PrepareOptions) error {
	if opts == nil {
		return fmt.Errorf("%w: options must not be nil", ErrInvalidArgument)
	}
	if opts.ExtraRunwayEpochs < 0 {
		return fmt.Errorf("%w: ExtraRunwayEpochs must be non-negative", ErrInvalidArgument)
	}
	if opts.BufferEpochs < 0 {
		return fmt.Errorf("%w: BufferEpochs must be non-negative", ErrInvalidArgument)
	}
	if opts.Costs != nil {
		if len(opts.Contexts) != 0 {
			return fmt.Errorf("%w: Contexts cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.DataSize != 0 {
			return fmt.Errorf("%w: DataSize cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.EnableCDN != nil {
			return fmt.Errorf("%w: EnableCDN cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.ExtraRunwayEpochs != 0 {
			return fmt.Errorf("%w: ExtraRunwayEpochs cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.BufferEpochs != 0 {
			return fmt.Errorf("%w: BufferEpochs cannot be set when Costs is set", ErrInvalidArgument)
		}
		return nil
	}
	if opts.DataSize == 0 {
		return fmt.Errorf("%w: DataSize must be greater than zero when Costs is nil", ErrInvalidArgument)
	}
	if len(opts.Contexts) != 0 && opts.EnableCDN != nil {
		return fmt.Errorf("%w: EnableCDN cannot be set when Contexts are supplied", ErrInvalidArgument)
	}
	return nil
}

func validatePrepareCosts(costs *MultiContextCosts) error {
	if costs.Ready {
		return nil
	}
	if costs.DepositNeeded == nil {
		return fmt.Errorf("%w: DepositNeeded is required when costs are not ready", ErrInvalidArgument)
	}
	if costs.DepositNeeded.Sign() < 0 {
		return fmt.Errorf("%w: DepositNeeded must be non-negative", ErrInvalidArgument)
	}
	return nil
}

// prepareContext is the narrow subset of *Context that prepareRefs
// consumes. Exported as a private interface so unit tests and alternate
// UploadContext implementations can participate without a hard type
// assertion to *Context. All public SDK context producers return values
// that satisfy this interface.
type prepareContext interface {
	DataSetID() *types.BigInt
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
		// Client-level DefaultWithCDN.
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
		id  types.BigInt
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
