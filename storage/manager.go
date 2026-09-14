package storage

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/costs"
)

// FindDataSetsOptions configures Service.FindDataSets. A nil pointer or
// zero value selects the configured default payer and
// disables the "only managed" filter.
type FindDataSetsOptions struct {
	// Payer overrides the address configured via Options.PayerAddress.
	// Zero means use the configured default payer.
	Payer common.Address
	// OnlyManaged restricts the returned set to data sets whose
	// record-keeper is the configured FWSS contract.
	OnlyManaged bool
}

// FindDataSets returns the enriched list of data sets owned by the caller
// or by the payer in opts.
func (s *Service) FindDataSets(ctx context.Context, opts *FindDataSetsOptions) ([]*DataSetDetails, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.finder == nil {
		return nil, fmt.Errorf("storage.Service.FindDataSets: %w: no DataSetFinder configured", ErrUninitialized)
	}
	payer := s.payerAddr
	onlyManaged := false
	if opts != nil {
		if opts.Payer != (common.Address{}) {
			payer = opts.Payer
		}
		onlyManaged = opts.OnlyManaged
	}
	if payer == (common.Address{}) {
		return nil, fmt.Errorf("storage.Service.FindDataSets: %w: zero payer and no default payer", ErrInvalidArgument)
	}
	return s.finder.FindDataSets(ctx, payer, onlyManaged)
}

// GetStorageInfoOptions configures Service.GetStorageInfo. A nil pointer
// selects the configured default payer as the client address.
type GetStorageInfoOptions struct {
	// Client overrides the default payer. Zero means use the address
	// configured via Options.PayerAddress; if that too is zero the
	// allowances section of the returned StorageInfo will be nil.
	Client common.Address
}

// GetStorageInfo returns the comprehensive chain-wide storage view needed
// to author an upload.
func (s *Service) GetStorageInfo(ctx context.Context, opts *GetStorageInfoOptions) (*StorageInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.info == nil {
		return nil, fmt.Errorf("storage.Service.GetStorageInfo: %w: no StorageInfoReader configured", ErrUninitialized)
	}
	client := s.payerAddr
	if opts != nil && opts.Client != (common.Address{}) {
		client = opts.Client
	}
	return s.info.GetStorageInfo(ctx, client)
}

// CalculateMultiContextCosts estimates aggregate costs for the given storage
// targets. pieceSizes contains the positive raw payload size of each piece,
// replicated to every target. Existing refs require a non-negative leaf count.
// A zero payer uses the configured default payer.
func (s *Service) CalculateMultiContextCosts(ctx context.Context, pieceSizes []uint64, refs []ContextCostRef, opts MultiCostOptions, payer common.Address) (*costs.MultiContextCosts, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if opts.BufferEpochs != nil && *opts.BufferEpochs < 0 {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: BufferEpochs must be non-negative", ErrInvalidArgument)
	}
	if opts.ExtraRunwayEpochs < 0 {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: ExtraRunwayEpochs must be non-negative", ErrInvalidArgument)
	}
	if err := validateCostPieceSizes(pieceSizes); err != nil {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w", err)
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: empty refs", ErrInvalidArgument)
	}
	for i, ref := range refs {
		if ref.DataSetID != nil && (ref.CurrentDataSetLeafCount == nil || ref.CurrentDataSetLeafCount.Sign() < 0) {
			return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: refs[%d] requires a non-negative CurrentDataSetLeafCount", ErrInvalidArgument, i)
		}
	}
	if s.costCalc == nil {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: no CostCalculator configured", ErrUninitialized)
	}
	if payer == (common.Address{}) {
		payer = s.payerAddr
	}
	if payer == (common.Address{}) {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: zero payer and no default payer", ErrInvalidArgument)
	}
	return s.calculateMultiContextCosts(ctx, payer, pieceSizes, refs, opts)
}

func (s *Service) calculateMultiContextCosts(ctx context.Context, payer common.Address, pieceSizes []uint64, refs []ContextCostRef, opts MultiCostOptions) (*costs.MultiContextCosts, error) {
	costRefs := make([]costs.MultiContextRef, len(refs))
	for i, ref := range refs {
		isNewDataSet := ref.DataSetID == nil
		currentLeaves := ref.CurrentDataSetLeafCount
		if isNewDataSet {
			currentLeaves = nil
		} else if currentLeaves != nil {
			currentLeaves = new(big.Int).Set(currentLeaves)
		}
		costRefs[i] = costs.MultiContextRef{
			IsNewDataSet:            isNewDataSet,
			CurrentDataSetLeafCount: currentLeaves,
			WithCDN:                 ref.WithCDN || opts.EnableCDN,
		}
	}
	return s.costCalc.CalculateMultiContextCosts(ctx, payer, pieceSizes, costRefs, &costs.UploadCostOptions{
		ExtraRunwayEpochs: opts.ExtraRunwayEpochs,
		BufferEpochs:      opts.BufferEpochs,
	})
}

func validateCostPieceSizes(pieceSizes []uint64) error {
	if len(pieceSizes) == 0 {
		return fmt.Errorf("%w: PieceSizes must not be empty", ErrInvalidArgument)
	}
	for i, size := range pieceSizes {
		if size == 0 {
			return fmt.Errorf("%w: PieceSizes[%d] must be greater than zero", ErrInvalidArgument, i)
		}
	}
	return nil
}
