package txutil

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// ReceiptClient abstracts the subset of ethclient used by receipt helpers.
type ReceiptClient interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// ReceiptWaitConfig configures polling behavior.
type ReceiptWaitConfig struct {
	Timeout              time.Duration // total timeout for the whole wait; 0 = 5 minutes
	PollInterval         time.Duration // polling cadence; 0 = 2 seconds
	MaxConsecutiveErrors int           // RPC errors in a row before giving up; 0 = 5
}

// DefaultReceiptWaitConfig returns a conservative default suitable for FEVM.
func DefaultReceiptWaitConfig() ReceiptWaitConfig {
	return ReceiptWaitConfig{
		Timeout:              5 * time.Minute,
		PollInterval:         2 * time.Second,
		MaxConsecutiveErrors: 5,
	}
}

// WaitForReceipt polls TransactionReceipt until the transaction is mined,
// returning an error if the receipt indicates failure. A zero timeout uses
// DefaultReceiptWaitConfig.
func WaitForReceipt(ctx context.Context, client ReceiptClient, txHash common.Hash, timeout time.Duration) (*types.Receipt, error) {
	cfg := DefaultReceiptWaitConfig()
	if timeout > 0 {
		cfg.Timeout = timeout
	}
	return waitForReceipt(ctx, client, txHash, cfg, 0)
}

// WaitForConfirmation waits until the transaction's inclusion block has the
// requested confirmations, then returns its receipt and any transaction failure.
func WaitForConfirmation(ctx context.Context, client ReceiptClient, txHash common.Hash, confirmations uint64) (*types.Receipt, error) {
	return waitForReceipt(ctx, client, txHash, DefaultReceiptWaitConfig(), confirmations)
}

// WaitForReceiptWithConfig gives full control over polling parameters and
// required confirmations.
func WaitForReceiptWithConfig(ctx context.Context, client ReceiptClient, txHash common.Hash, cfg ReceiptWaitConfig, confirmations uint64) (*types.Receipt, error) {
	return waitForReceipt(ctx, client, txHash, cfg, confirmations)
}

// HasConfirmations reports whether the inclusion block has the requested depth.
// The inclusion block counts as the first confirmation. A zero requirement does
// not require a block number or head.
func HasConfirmations(blockNumber *big.Int, head, confirmations uint64) bool {
	if confirmations == 0 {
		return true
	}
	if blockNumber == nil || !blockNumber.IsUint64() {
		return false
	}
	inclusion := blockNumber.Uint64()
	return head >= inclusion && head-inclusion >= confirmations-1
}

func waitForReceipt(ctx context.Context, client ReceiptClient, txHash common.Hash, cfg ReceiptWaitConfig, confirmations uint64) (*types.Receipt, error) {
	if cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Minute
	}
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 2 * time.Second
	}
	maxErr := cfg.MaxConsecutiveErrors
	if maxErr <= 0 {
		maxErr = 5
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	var (
		consecutive int
		polls       int
		lastErr     error
	)

	waitError := func() error {
		if errors.Is(ctx.Err(), context.Canceled) {
			return fmt.Errorf("receipt wait canceled after %d polls: %w", polls, ctx.Err())
		}
		if lastErr != nil {
			return fmt.Errorf("%w after %d polls: %w (last err: %w)", ErrReceiptTimeout, polls, ctx.Err(), lastErr)
		}
		return fmt.Errorf("%w after %d polls: %w", ErrReceiptTimeout, polls, ctx.Err())
	}

	tryPoll := func() (*types.Receipt, bool, error) {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				return nil, false, nil
			}
			return nil, false, err
		}
		if receipt == nil {
			return nil, false, nil
		}
		var head uint64
		if confirmations > 0 {
			if receipt.BlockNumber == nil || !receipt.BlockNumber.IsUint64() {
				return nil, false, nil
			}
			head, err = client.BlockNumber(ctx)
			if err != nil {
				return nil, false, err
			}
		}
		if !HasConfirmations(receipt.BlockNumber, head, confirmations) {
			return nil, false, nil
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			return receipt, true, fmt.Errorf("%w: status=%d tx=%s", ErrTxFailed, receipt.Status, txHash.Hex())
		}
		return receipt, true, nil
	}

	for {
		if ctx.Err() != nil {
			return nil, waitError()
		}
		polls++
		receipt, done, err := tryPoll()
		if done {
			return receipt, err
		}
		if err != nil && ctx.Err() != nil {
			return nil, waitError()
		}
		if err != nil {
			lastErr = err
			consecutive++
			if !IsRetryableRPCError(err) || consecutive >= maxErr {
				return nil, fmt.Errorf("%w: %d consecutive errors after %d polls, last err: %w", ErrReceiptRPCFailure, consecutive, polls, err)
			}
		} else {
			consecutive = 0
		}
		select {
		case <-ctx.Done():
			return nil, waitError()
		case <-ticker.C:
		}
	}
}
