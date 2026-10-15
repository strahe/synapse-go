package txutil

import (
	"context"
	"errors"
	"math/big"
	"slices"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
)

// PrepareTransactOpts validates the signing identity and returns an owned copy
// whose context and transferred value belong to the current operation.
func PrepareTransactOpts(opts *bind.TransactOpts, from common.Address, ctx context.Context, value *big.Int) (*bind.TransactOpts, error) {
	if opts == nil {
		return nil, errors.New("signer returned nil options")
	}
	if opts.Signer == nil {
		return nil, errors.New("signer returned options without a signing function")
	}
	if opts.From != from {
		return nil, errors.New("transaction sender differs from signer address")
	}
	owned := *opts
	owned.Nonce = cloneInt(opts.Nonce)
	owned.Value = cloneInt(value)
	owned.GasPrice = cloneInt(opts.GasPrice)
	owned.GasFeeCap = cloneInt(opts.GasFeeCap)
	owned.GasTipCap = cloneInt(opts.GasTipCap)
	owned.AccessList = slices.Clone(opts.AccessList)
	for i := range owned.AccessList {
		owned.AccessList[i].StorageKeys = slices.Clone(opts.AccessList[i].StorageKeys)
	}
	owned.Context = ctx
	return &owned, nil
}

func cloneInt(n *big.Int) *big.Int {
	if n == nil {
		return nil
	}
	return new(big.Int).Set(n)
}
