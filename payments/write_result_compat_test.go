package payments

import (
	"errors"
	"testing"

	sdktypes "github.com/strahe/synapse-go/types"
)

var _ *WriteResult = (*sdktypes.WriteResult)(nil)

func TestErrTxFailedAlias(t *testing.T) {
	if !errors.Is(ErrTxFailed, sdktypes.ErrTxFailed) {
		t.Fatal("payments.ErrTxFailed must be the same sentinel as types.ErrTxFailed")
	}
}
