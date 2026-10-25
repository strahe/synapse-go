package warmstorage

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/internal/idconv"
	sdktypes "github.com/strahe/synapse-go/types"
)

// TerminateDataSet terminates the FWSS-managed payment rails for the
// given data set. It maps to FWSS.terminateService(uint256).
// By default it returns after broadcast with the submission hash and no receipt.
// WithOnSubmitted reports the submission hash before any receipt waiting.
// A positive WithWait waits for a receipt; waiting errors retain the submission
// hash in the returned WriteResult, and a failed transaction also retains its
// receipt. Confirmation does not wait for service expiry or data-set cleanup.
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
	tx, err := s.signAndSend(ctx, txOpts, func(opts *bind.TransactOpts) (*ethtypes.Transaction, error) {
		return s.fwssWrite.TerminateService(opts, dataSetID.Big())
	})
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

// ExtractPDPPaymentTerminatedEvent extracts the first PDPPaymentTerminated
// event without validating its contract address or data set. Use
// [Service.PDPPaymentTerminatedEvent] to verify both.
func ExtractPDPPaymentTerminatedEvent(receipt *ethtypes.Receipt) (*PDPPaymentTerminatedEvent, error) {
	return extractPDPPaymentTerminatedEvent("warmstorage.ExtractPDPPaymentTerminatedEvent", receipt, nil, nil)
}

// PDPPaymentTerminatedEvent extracts the configured FWSS contract's termination
// event for dataSetID from receipt.
func (s *Service) PDPPaymentTerminatedEvent(receipt *ethtypes.Receipt, dataSetID sdktypes.BigInt) (*PDPPaymentTerminatedEvent, error) {
	const op = "warmstorage.PDPPaymentTerminatedEvent"
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if dataSetID.IsZero() {
		return nil, fmt.Errorf("%s: %w: zero dataSetID", op, ErrInvalidArgument)
	}
	address := s.FWSSAddress()
	return extractPDPPaymentTerminatedEvent(op, receipt, &address, &dataSetID)
}

func extractPDPPaymentTerminatedEvent(op string, receipt *ethtypes.Receipt, address *common.Address, target *sdktypes.BigInt) (*PDPPaymentTerminatedEvent, error) {
	if receipt == nil {
		return nil, fmt.Errorf("%s: nil receipt", op)
	}
	filterer, err := fwss.NewFWSSFilterer(common.Address{}, nil)
	if err != nil {
		return nil, fmt.Errorf("%s: bind filterer: %w", op, err)
	}
	for _, log := range receipt.Logs {
		if log == nil || (address != nil && log.Address != *address) {
			continue
		}
		ev, err := filterer.ParsePDPPaymentTerminated(*log)
		if err != nil {
			continue
		}
		if target != nil && ev.DataSetId.Cmp(target.Big()) != 0 {
			continue
		}
		dataSetID, err := idconv.FromBig("DataSetID", ev.DataSetId)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		pdpRailID, err := idconv.FromBig("PDPRailID", ev.PdpRailId)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		endEpoch, err := epochFromBig("EndEpoch", ev.EndEpoch)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		return &PDPPaymentTerminatedEvent{
			DataSetID: dataSetID,
			EndEpoch:  endEpoch,
			PDPRailID: pdpRailID,
		}, nil
	}
	return nil, fmt.Errorf("%s: PDPPaymentTerminated event not found", op)
}
