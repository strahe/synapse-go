package txutil

import (
	"context"
	"errors"
	"fmt"
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

// WaitForConfirmation waits until the transaction is mined and its inclusion
// block has accumulated the requested number of confirmations.
func WaitForConfirmation(ctx context.Context, client ReceiptClient, txHash common.Hash, confirmations uint64) (*types.Receipt, error) {
	return waitForReceipt(ctx, client, txHash, DefaultReceiptWaitConfig(), confirmations)
}

// WaitForReceiptWithConfig gives full control over polling parameters and
// required confirmations.
func WaitForReceiptWithConfig(ctx context.Context, client ReceiptClient, txHash common.Hash, cfg ReceiptWaitConfig, confirmations uint64) (*types.Receipt, error) {
	return waitForReceipt(ctx, client, txHash, cfg, confirmations)
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

	tryPoll := func() (*types.Receipt, bool, error) {
		polls++
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err != nil {
			if errors.Is(err, ethereum.NotFound) {
				consecutive = 0
				return nil, false, nil
			}
			if !IsRetryableRPCError(err) {
				return nil, false, fmt.Errorf("%w: %w", ErrReceiptRPCFailure, err)
			}
			consecutive++
			lastErr = err
			if consecutive >= maxErr {
				return nil, false, fmt.Errorf("%w: %d consecutive errors after %d polls, last err: %w", ErrReceiptRPCFailure, consecutive, polls, lastErr)
			}
			return nil, false, nil
		}
		consecutive = 0
		if receipt.Status != types.ReceiptStatusSuccessful {
			return receipt, false, fmt.Errorf("%w: status=%d tx=%s", ErrTxFailed, receipt.Status, txHash.Hex())
		}
		if confirmations == 0 {
			return receipt, true, nil
		}
		if receipt.BlockNumber == nil {
			return nil, false, nil
		}
		head, err := client.BlockNumber(ctx)
		if err != nil {
			if !IsRetryableRPCError(err) {
				return nil, false, fmt.Errorf("%w: %w", ErrReceiptRPCFailure, err)
			}
			consecutive++
			lastErr = err
			if consecutive >= maxErr {
				return nil, false, fmt.Errorf("%w: %d consecutive errors after %d polls, last err: %w", ErrReceiptRPCFailure, consecutive, polls, lastErr)
			}
			return nil, false, nil
		}
		requiredHead := receipt.BlockNumber.Uint64() + confirmations - 1
		if head >= requiredHead {
			return receipt, true, nil
		}
		return nil, false, nil
	}

	for {
		if receipt, done, err := tryPoll(); done || err != nil {
			return receipt, err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("%w after %d polls: %w (last err: %w)", ErrReceiptTimeout, polls, ctx.Err(), lastErr)
			}
			return nil, fmt.Errorf("%w after %d polls: %w", ErrReceiptTimeout, polls, ctx.Err())
		case <-ticker.C:
		}
	}
}
