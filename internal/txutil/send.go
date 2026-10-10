package txutil

import (
	"context"
	"errors"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/core/types"
)

// TransactionSender broadcasts an already signed transaction.
type TransactionSender interface {
	SendTransaction(context.Context, *types.Transaction) error
}

// SignAndSend constructs a signed transaction before attempting to broadcast it.
// A non-nil transaction with an error means SendTransaction returned that error;
// the sender may already have accepted the transaction. Earlier failures return
// a nil transaction. The caller must hold its nonce reservation until this returns.
func SignAndSend(ctx context.Context, sender TransactionSender, opts *bind.TransactOpts, build func(*bind.TransactOpts) (*types.Transaction, error)) (*types.Transaction, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	localOpts := *opts
	localOpts.Context = ctx
	localOpts.NoSend = true
	tx, err := build(&localOpts)
	if err != nil {
		return nil, err
	}
	if tx == nil {
		return nil, errors.New("transaction builder returned nil transaction")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := sender.SendTransaction(ctx, tx); err != nil {
		return tx, err
	}
	return tx, nil
}
