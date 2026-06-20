package warmstorage

import (
	"context"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/internal/idconv"
	sdktypes "github.com/strahe/synapse-go/types"
)

// TerminateDataSet terminates the FWSS-managed payment rails for the
// given data set. It maps to FWSS.terminateService(uint256).
func (s *Service) TerminateDataSet(ctx context.Context, dataSetID sdktypes.BigInt, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if s.fwssWrite == nil || s.signer == nil || s.backend == nil || s.nonces == nil {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w", ErrWriteNotConfigured)
	}
	if !s.chainID.IsValid() {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w: invalid ChainID", ErrInvalidArgument)
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w: zero dataSetID", ErrInvalidArgument)
	}
	txOpts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w", err)
	}
	defer release()
	tx, err := s.fwssWrite.TerminateService(txOpts, dataSetID.Big())
	release()
	if err != nil {
		return nil, fmt.Errorf("warmstorage.TerminateDataSet: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// PDPPaymentTerminatedEvent is the FWSS termination event carrying the PDP
// rail end epoch.
type PDPPaymentTerminatedEvent struct {
	DataSetID sdktypes.BigInt
	EndEpoch  sdktypes.Epoch
	PDPRailID sdktypes.BigInt
}

// ExtractPDPPaymentTerminatedEvent extracts PDPPaymentTerminated from a
// transaction receipt.
func ExtractPDPPaymentTerminatedEvent(receipt *ethtypes.Receipt) (*PDPPaymentTerminatedEvent, error) {
	if receipt == nil {
		return nil, errors.New("warmstorage.ExtractPDPPaymentTerminatedEvent: nil receipt")
	}
	filterer, err := fwss.NewFWSSFilterer(common.Address{}, nil)
	if err != nil {
		return nil, fmt.Errorf("warmstorage.ExtractPDPPaymentTerminatedEvent: bind filterer: %w", err)
	}
	for _, log := range receipt.Logs {
		if log == nil {
			continue
		}
		ev, err := filterer.ParsePDPPaymentTerminated(*log)
		if err != nil {
			continue
		}
		dataSetID, err := idconv.FromBig("DataSetID", ev.DataSetId)
		if err != nil {
			return nil, fmt.Errorf("warmstorage.ExtractPDPPaymentTerminatedEvent: %w", err)
		}
		pdpRailID, err := idconv.FromBig("PDPRailID", ev.PdpRailId)
		if err != nil {
			return nil, fmt.Errorf("warmstorage.ExtractPDPPaymentTerminatedEvent: %w", err)
		}
		endEpoch, err := epochFromBig("EndEpoch", ev.EndEpoch)
		if err != nil {
			return nil, fmt.Errorf("warmstorage.ExtractPDPPaymentTerminatedEvent: %w", err)
		}
		return &PDPPaymentTerminatedEvent{
			DataSetID: dataSetID,
			EndEpoch:  endEpoch,
			PDPRailID: pdpRailID,
		}, nil
	}
	return nil, errors.New("warmstorage.ExtractPDPPaymentTerminatedEvent: PDPPaymentTerminated event not found")
}
