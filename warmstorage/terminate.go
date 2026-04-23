package warmstorage

import (
	"context"
	"fmt"

	"github.com/strahe/synapse-go/internal/idconv"
	sdktypes "github.com/strahe/synapse-go/types"
)

// TerminateDataSet terminates the FWSS-managed payment rails for the
// given data set. It maps to FWSS.terminateService(uint256).
//
// See FilecoinWarmStorageService.sol and the TS
// warmStorage.terminateService for semantics.
func (s *Service) TerminateDataSet(ctx context.Context, dataSetID sdktypes.DataSetID, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.fwssWrite == nil || s.signer == nil || s.backend == nil || s.nonces == nil {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w", ErrWriteNotConfigured)
	}
	if !s.chainID.IsValid() {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w: invalid ChainID", ErrInvalidArgument)
	}
	if dataSetID == 0 {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w: zero dataSetID", ErrInvalidArgument)
	}
	txOpts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w", err)
	}
	defer release()
	tx, err := s.fwssWrite.TerminateService(txOpts, idconv.Big(dataSetID))
	release()
	if err != nil {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}
