package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
)

// PrepareOptions configures Service.Prepare. Costs and context-based cost
// calculation are mutually exclusive modes.
type PrepareOptions struct {
	// PieceSizes contains the positive raw payload size of each piece, copied
	// to every context. Required when Costs is nil; must be empty otherwise.
	PieceSizes []uint64
	// Contexts is the exact set of upload targets used for cost calculation.
	// It is required when Costs is nil.
	Contexts []StorageContext
	// Costs short-circuits cost calculation. When set, no other
	// PrepareOptions fields are accepted.
	Costs *costs.MultiContextCosts
	// ExtraRunwayEpochs is additional runway (epochs) above the
	// minimum lockup period passed through to the cost calculator. It is
	// valid only when Costs is nil and must be non-negative.
	ExtraRunwayEpochs int64
	// BufferEpochs is the deposit cushion above current lockup usage
	// used to absorb transaction-latency epochs. Nil uses the cost service
	// default; a pointer to zero disables the buffer. It is valid only when
	// Costs is nil. Negative values return ErrInvalidArgument.
	BufferEpochs *int64
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
	// Execute performs the funding operation. When approval is required,
	// Prepare fixes the approval decision and max lockup period from Costs;
	// caller-provided payments.WriteOption values should be limited to write
	// controls such as wait, confirmations, or precheck behavior.
	Execute func(ctx context.Context, opts ...payments.WriteOption) (*types.WriteResult, error)
}

// PrepareResult is the value returned by Service.Prepare.
type PrepareResult struct {
	// Costs is the aggregated cost calculation that drove the decision.
	Costs *costs.MultiContextCosts
	// Transaction is non-nil only when funding is required (Ready=false).
	Transaction *PrepareTransaction
}

// Prepare returns the funding transaction needed, if any, to cover one upload
// of PieceSizes across the supplied contexts. Existing data sets require a
// DataSetLeafCountReader; missing configuration returns [ErrUninitialized],
// and unavailable data sets return [ErrDataSetUnavailable]. Precomputed Costs
// bypasses leaf-count reads.
func (s *Service) Prepare(ctx context.Context, opts *PrepareOptions) (*PrepareResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	opts = clonePrepareOptions(opts)
	if err := validatePrepareOptions(opts); err != nil {
		return nil, fmt.Errorf("storage.Service.Prepare: %w", err)
	}

	summary := opts.Costs
	if summary == nil {
		if err := s.validateStorageContexts("storage.Service.Prepare", opts.Contexts); err != nil {
			return nil, err
		}
		refs, err := s.prepareRefs(ctx, opts)
		if err != nil {
			return nil, err
		}
		if s.costCalc == nil {
			return nil, fmt.Errorf("storage.Service.Prepare: %w: no CostCalculator configured", ErrUninitialized)
		}
		payer := s.payerAddr
		if payer == (common.Address{}) {
			return nil, fmt.Errorf("storage.Service.Prepare: %w: zero payer and no default payer", ErrInvalidArgument)
		}
		summary, err = s.calculateMultiContextCosts(ctx, payer, opts.PieceSizes, refs, MultiCostOptions{
			ExtraRunwayEpochs: opts.ExtraRunwayEpochs,
			BufferEpochs:      opts.BufferEpochs,
		})
		if err != nil {
			return nil, fmt.Errorf("storage.Service.Prepare: %w", err)
		}
		if summary == nil {
			return nil, errors.New("storage.Service.Prepare: cost calculator returned nil costs")
		}
	}

	if err := validatePrepareCosts(summary); err != nil {
		return nil, fmt.Errorf("storage.Service.Prepare: %w", err)
	}

	if summary.Ready {
		return &PrepareResult{Costs: summary}, nil
	}

	if s.funder == nil {
		return nil, fmt.Errorf("storage.Service.Prepare: %w: no PaymentsFunder configured", ErrUninitialized)
	}

	deposit := summary.DepositNeeded
	needsApproval := summary.NeedsFWSSMaxApproval
	funder := s.funder

	return &PrepareResult{
		Costs: summary,
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
					if summary.RequiredLockupPeriod != nil {
						optsOut = append(optsOut, payments.WithFundApprovalLockupPeriod(summary.RequiredLockupPeriod))
					}
				}
				return funder.FundSync(ctx, deposit, optsOut...)
			},
		},
	}, nil
}

func clonePrepareOptions(opts *PrepareOptions) *PrepareOptions {
	if opts == nil {
		return nil
	}
	out := *opts
	out.Contexts = slices.Clone(opts.Contexts)
	out.PieceSizes = slices.Clone(opts.PieceSizes)
	if opts.BufferEpochs != nil {
		bufferEpochs := *opts.BufferEpochs
		out.BufferEpochs = &bufferEpochs
	}
	return &out
}

func validatePrepareOptions(opts *PrepareOptions) error {
	if opts == nil {
		return fmt.Errorf("%w: options must not be nil", ErrInvalidArgument)
	}
	if opts.ExtraRunwayEpochs < 0 {
		return fmt.Errorf("%w: ExtraRunwayEpochs must be non-negative", ErrInvalidArgument)
	}
	if opts.BufferEpochs != nil && *opts.BufferEpochs < 0 {
		return fmt.Errorf("%w: BufferEpochs must be non-negative", ErrInvalidArgument)
	}
	if opts.Costs != nil {
		if len(opts.Contexts) != 0 {
			return fmt.Errorf("%w: Contexts cannot be set when Costs is set", ErrInvalidArgument)
		}
		if len(opts.PieceSizes) != 0 {
			return fmt.Errorf("%w: PieceSizes cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.ExtraRunwayEpochs != 0 {
			return fmt.Errorf("%w: ExtraRunwayEpochs cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.BufferEpochs != nil {
			return fmt.Errorf("%w: BufferEpochs cannot be set when Costs is set", ErrInvalidArgument)
		}
		return nil
	}
	if err := validateCostPieceSizes(opts.PieceSizes); err != nil {
		return err
	}
	if len(opts.Contexts) == 0 {
		return fmt.Errorf("%w: Contexts must not be empty when Costs is nil", ErrInvalidArgument)
	}
	return nil
}

func validatePrepareCosts(summary *costs.MultiContextCosts) error {
	if summary.Ready {
		return nil
	}
	if summary.DepositNeeded == nil {
		return fmt.Errorf("%w: DepositNeeded is required when costs are not ready", ErrInvalidArgument)
	}
	if summary.DepositNeeded.Sign() < 0 {
		return fmt.Errorf("%w: DepositNeeded must be non-negative", ErrInvalidArgument)
	}
	return nil
}

// prepareRefs builds storage cost refs from the user-supplied contexts.
// For existing-dataset contexts, the current on-chain leaf count is fetched in
// parallel via [DataSetLeafCountReader] so the cost calculator can price
// lockup against real storage usage rather than the floor rate.
func (s *Service) prepareRefs(ctx context.Context, opts *PrepareOptions) ([]ContextCostRef, error) {
	contexts := opts.Contexts

	refs := make([]ContextCostRef, len(contexts))
	type leafCountJob struct {
		idx int
		id  types.BigInt
	}
	var jobs []leafCountJob
	for i, uploadCtx := range contexts {
		refs[i] = ContextCostRef{
			Provider: uploadCtx.GetProviderInfo(),
			WithCDN:  uploadCtx.CDNEnabled(),
		}
		if dataSet, ok := uploadCtx.DataSetRef(); ok {
			id := dataSet.DataSetID()
			refs[i].DataSetID = &id
			if s.leafCountReader == nil {
				return nil, fmt.Errorf("storage.Service.Prepare: %w: no DataSetLeafCountReader configured", ErrUninitialized)
			}
			jobs = append(jobs, leafCountJob{idx: i, id: id})
		}
	}

	if len(jobs) > 0 {
		type leafCountResult struct {
			idx    int
			leaves *big.Int
			err    error
		}
		results := make(chan leafCountResult, len(jobs))
		for _, j := range jobs {
			go func(j leafCountJob) {
				leaves, err := s.leafCountReader.GetDataSetLeafCount(ctx, j.id)
				results <- leafCountResult{idx: j.idx, leaves: leaves, err: err}
			}(j)
		}
		for range jobs {
			r := <-results
			if r.err != nil {
				return nil, fmt.Errorf("storage.Service.Prepare: GetDataSetLeafCount for data set %s: %w", refs[r.idx].DataSetID, r.err)
			}
			if r.leaves == nil {
				return nil, fmt.Errorf("storage.Service.Prepare: reader returned nil leaf count for data set %s", refs[r.idx].DataSetID)
			}
			if r.leaves.Sign() < 0 {
				return nil, fmt.Errorf("storage.Service.Prepare: reader returned negative leaf count for data set %s", refs[r.idx].DataSetID)
			}
			refs[r.idx].CurrentDataSetLeafCount = new(big.Int).Set(r.leaves)
		}
	}

	return refs, nil
}
