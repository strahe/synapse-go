package storage

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// FindDataSetsOptions configures Service.FindDataSets. A nil pointer or
// zero value selects the configured default signer as the payer and
// disables the "only managed" filter.
type FindDataSetsOptions struct {
	// Payer overrides the default signer address. Zero means "use the
	// signer configured via Options.SignerAddress".
	Payer common.Address
	// OnlyManaged restricts the returned set to data sets whose
	// record-keeper is the configured FWSS contract.
	OnlyManaged bool
}

// FindDataSets returns the enriched list of data sets owned by the caller
// or by the payer in opts.
func (s *Service) FindDataSets(ctx context.Context, opts *FindDataSetsOptions) ([]*DataSetInfo, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.finder == nil {
		return nil, fmt.Errorf("storage.Service.FindDataSets: %w: no DataSetFinder configured", ErrUninitialized)
	}
	payer := s.signerAddr
	onlyManaged := false
	if opts != nil {
		if opts.Payer != (common.Address{}) {
			payer = opts.Payer
		}
		onlyManaged = opts.OnlyManaged
	}
	if payer == (common.Address{}) {
		return nil, fmt.Errorf("storage.Service.FindDataSets: %w: zero payer and no default signer", ErrInvalidArgument)
	}
	return s.finder.FindDataSets(ctx, payer, onlyManaged)
}

// GetStorageInfoOptions configures Service.GetStorageInfo. A nil pointer
// selects the configured default signer as the client address.
type GetStorageInfoOptions struct {
	// Client overrides the default signer. Zero means "use the signer
	// configured via Options.SignerAddress"; if that too is zero the
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
	client := s.signerAddr
	if opts != nil && opts.Client != (common.Address{}) {
		client = opts.Client
	}
	return s.info.GetStorageInfo(ctx, client)
}

// TerminateDataSetOptions configures Service.TerminateDataSet.
type TerminateDataSetOptions struct {
	// WriteOptions are forwarded to warmstorage.TerminateDataSet.
	WriteOptions []warmstorage.WriteOption
}

// TerminateDataSet terminates an FWSS-managed data set by ID.
func (s *Service) TerminateDataSet(ctx context.Context, dataSetID types.BigInt, opts *TerminateDataSetOptions) (*types.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.terminator == nil {
		return nil, fmt.Errorf("storage.Service.TerminateDataSet: %w: no DataSetTerminator configured", ErrUninitialized)
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("storage.Service.TerminateDataSet: %w: zero dataSetID", ErrInvalidArgument)
	}
	var writeOpts []warmstorage.WriteOption
	if opts != nil {
		writeOpts = opts.WriteOptions
	}
	return s.terminator.TerminateDataSet(ctx, dataSetID, writeOpts...)
}

// CalculateMultiContextCosts fans out the cost calculation across the given
// refs and returns an aggregate result.
func (s *Service) CalculateMultiContextCosts(ctx context.Context, dataSizeBytes uint64, refs []ContextCostRef, opts MultiCostOptions, payer common.Address) (*MultiContextCosts, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.costCalc == nil {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: no CostCalculator configured", ErrUninitialized)
	}
	if payer == (common.Address{}) {
		payer = s.signerAddr
	}
	if payer == (common.Address{}) {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: zero payer and no default signer", ErrInvalidArgument)
	}
	if len(refs) == 0 {
		return nil, fmt.Errorf("storage.Service.CalculateMultiContextCosts: %w: empty refs", ErrInvalidArgument)
	}
	size := new(big.Int).SetUint64(dataSizeBytes)
	return s.costCalc.CalculateMultiContextCosts(ctx, payer, size, refs, opts)
}
