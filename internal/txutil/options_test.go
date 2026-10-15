package txutil

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

func TestPrepareTransactOptsOwnsMutableFields(t *testing.T) {
	from := common.HexToAddress("0x1234")
	template := &bind.TransactOpts{
		From: from, Nonce: big.NewInt(7), Value: big.NewInt(19),
		GasPrice: big.NewInt(23), GasFeeCap: big.NewInt(29), GasTipCap: big.NewInt(31),
		GasLimit: 100_000, NoSend: true, Context: context.Background(),
		AccessList: types.AccessList{{Address: from, StorageKeys: []common.Hash{common.HexToHash("0xab")}}},
		Signer:     func(_ common.Address, tx *types.Transaction) (*types.Transaction, error) { return tx, nil },
	}
	ctx := t.Context()
	value := big.NewInt(37)
	owned, err := PrepareTransactOpts(template, from, ctx, value)
	if err != nil {
		t.Fatal(err)
	}
	if owned == template || owned.Context != ctx || owned.GasLimit != template.GasLimit || !owned.NoSend {
		t.Fatalf("operation options = %+v", owned)
	}
	for _, pair := range []struct{ got, source *big.Int }{
		{owned.Nonce, template.Nonce},
		{owned.Value, value},
		{owned.GasPrice, template.GasPrice},
		{owned.GasFeeCap, template.GasFeeCap},
		{owned.GasTipCap, template.GasTipCap},
	} {
		if pair.got == pair.source || pair.got.Cmp(pair.source) != 0 {
			t.Fatalf("integer copy = %v (%p), source = %v (%p)", pair.got, pair.got, pair.source, pair.source)
		}
		pair.got.SetInt64(0)
		if pair.source.Sign() == 0 {
			t.Fatal("mutating owned integer changed its source")
		}
	}
	owned.AccessList[0].Address = common.Address{}
	owned.AccessList[0].StorageKeys[0] = common.Hash{}
	if template.AccessList[0].Address != from || template.AccessList[0].StorageKeys[0] != common.HexToHash("0xab") {
		t.Fatal("mutating owned access list changed the template")
	}
	if template.Value.Int64() != 19 || template.Context != context.Background() {
		t.Fatal("operation preparation changed template value or context")
	}
	nonpayable, err := PrepareTransactOpts(template, from, ctx, nil)
	if err != nil || nonpayable.Value != nil {
		t.Fatalf("nonpayable options = %+v, error = %v", nonpayable, err)
	}
	tx := types.NewTx(&types.LegacyTx{})
	if signed, err := owned.Signer(from, tx); err != nil || signed != tx {
		t.Fatalf("copied signing function returned %v, %v", signed, err)
	}
}
