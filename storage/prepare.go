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

// PrepareOptions configures Service.Prepare. Costs and context-based cost
// calculation are mutually exclusive modes.
type PrepareOptions struct {
	// DataSize is required when Costs is nil, and must be zero when Costs
	// is set. It is the payload size in bytes used for cost calculation.
	DataSize uint64
	// Contexts is the exact set of upload targets used for cost calculation.
	// It is required when Costs is nil.
	Contexts []StorageContext
	// PieceCount is the number of pieces added to each context. Nil or
	// non-positive values default to one.
	PieceCount *big.Int
	// Costs short-circuits cost calculation. When set, no other
	// PrepareOptions fields are accepted.
	Costs *MultiContextCosts
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
	Costs *MultiContextCosts
	// Transaction is non-nil only when funding is required (Ready=false).
	Transaction *PrepareTransaction
}

// Prepare returns the funding transaction needed, if any, to cover one
// upload of DataSize bytes across the supplied contexts.
func (s *Service) Prepare(ctx context.Context, opts *PrepareOptions) (*PrepareResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	opts = clonePrepareOptions(opts)
	if err := validatePrepareOptions(opts); err != nil {
		return nil, fmt.Errorf("storage.Service.Prepare: %w", err)
	}

	costs := opts.Costs
	if costs == nil {
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
		size := new(big.Int).SetUint64(opts.DataSize)
		costs, err = s.costCalc.CalculateMultiContextCosts(ctx, payer, size, refs, MultiCostOptions{
			PieceCount:        opts.PieceCount,
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
					if costs.RequiredLockupPeriod != nil {
						optsOut = append(optsOut, payments.WithFundApprovalLockupPeriod(costs.RequiredLockupPeriod))
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
	out.Contexts = append([]StorageContext(nil), opts.Contexts...)
	if opts.PieceCount != nil {
		out.PieceCount = new(big.Int).Set(opts.PieceCount)
	}
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
		if opts.DataSize != 0 {
			return fmt.Errorf("%w: DataSize cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.PieceCount != nil {
			return fmt.Errorf("%w: PieceCount cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.ExtraRunwayEpochs != 0 {
			return fmt.Errorf("%w: ExtraRunwayEpochs cannot be set when Costs is set", ErrInvalidArgument)
		}
		if opts.BufferEpochs != nil {
			return fmt.Errorf("%w: BufferEpochs cannot be set when Costs is set", ErrInvalidArgument)
		}
		return nil
	}
	if opts.DataSize == 0 {
		return fmt.Errorf("%w: DataSize must be greater than zero when Costs is nil", ErrInvalidArgument)
	}
	if len(opts.Contexts) == 0 {
		return fmt.Errorf("%w: Contexts must not be empty when Costs is nil", ErrInvalidArgument)
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

// prepareRefs builds the []ContextCostRef the cost calculator expects
// from the user-supplied Contexts. For
// existing-dataset contexts, the current on-chain size is fetched in
// parallel via [DataSetSizeReader] so the cost calculator can price
// lockup against real storage usage rather than the floor rate.
func (s *Service) prepareRefs(ctx context.Context, opts *PrepareOptions) ([]ContextCostRef, error) {
	contexts := opts.Contexts

	refs := make([]ContextCostRef, len(contexts))
	type sizeJob struct {
		idx int
		id  types.BigInt
	}
	var jobs []sizeJob
	for i, uploadCtx := range contexts {
		refs[i] = ContextCostRef{
			Provider: uploadCtx.GetProviderInfo(),
			WithCDN:  uploadCtx.CDNEnabled(),
		}
		if dataSet, ok := uploadCtx.DataSetRef(); ok {
			id := dataSet.DataSetID()
			refs[i].DataSetID = &id
			if s.sizeReader != nil {
				jobs = append(jobs, sizeJob{idx: i, id: id})
			}
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
