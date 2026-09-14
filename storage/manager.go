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
// targets. A zero payer uses the configured default payer. Use
// [costs.Service.CalculateMultiContextCosts] for normalized cost refs and
// arbitrary-precision payload sizes without storage-specific input conversion.
func (s *Service) CalculateMultiContextCosts(ctx context.Context, dataSizeBytes uint64, refs []ContextCostRef, opts MultiCostOptions, payer common.Address) (*costs.MultiContextCosts, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if opts.BufferEpochs != nil && *opts.BufferEpochs < 0 {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: BufferEpochs must be non-negative", ErrInvalidArgument)
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
	if len(refs) == 0 {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: empty refs", ErrInvalidArgument)
	}
	size := new(big.Int).SetUint64(dataSizeBytes)
	return s.calculateMultiContextCosts(ctx, payer, size, refs, opts)
}

func (s *Service) calculateMultiContextCosts(ctx context.Context, payer common.Address, dataSizeBytes *big.Int, refs []ContextCostRef, opts MultiCostOptions) (*costs.MultiContextCosts, error) {
	costRefs := make([]costs.MultiContextRef, len(refs))
	for i, ref := range refs {
		isNewDataSet := ref.DataSetID == nil
		currentSize := ref.CurrentDataSetSizeBytes
		if isNewDataSet {
			currentSize = nil
		}
		costRefs[i] = costs.MultiContextRef{
			IsNewDataSet:            isNewDataSet,
			CurrentDataSetSizeBytes: currentSize,
			WithCDN:                 ref.WithCDN || opts.EnableCDN,
		}
	}
	return s.costCalc.CalculateMultiContextCosts(ctx, payer, dataSizeBytes, costRefs, &costs.UploadCostOptions{
		PieceCount:        opts.PieceCount,
		ExtraRunwayEpochs: opts.ExtraRunwayEpochs,
		BufferEpochs:      opts.BufferEpochs,
	})
}
