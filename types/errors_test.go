package types_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/types"
)

func TestBroadcastError(t *testing.T) {
	tx := ethtypes.NewTx(&ethtypes.LegacyTx{Nonce: 7, Data: []byte("private transaction payload")})
	want := &types.BroadcastError{Transaction: tx, Cause: io.ErrUnexpectedEOF}
	err := fmt.Errorf("withdraw: %w", want)
	got, ok := errors.AsType[*types.BroadcastError](err)
	if !ok || got != want || !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("error lost transaction identity or cause: %v", err)
	}
	if !strings.Contains(err.Error(), tx.Hash().Hex()) || !strings.Contains(err.Error(), io.ErrUnexpectedEOF.Error()) ||
		strings.Contains(err.Error(), "private transaction payload") {
		t.Fatalf("unexpected error text: %v", err)
	}
	for _, empty := range []*types.BroadcastError{nil, {}} {
		if empty.Error() == "" || empty.Unwrap() != nil {
			t.Fatalf("invalid zero-value behavior: %v", empty)
		}
	}
}
