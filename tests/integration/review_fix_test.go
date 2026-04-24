//go:build integration

package integration_test

import (
	"errors"
	"testing"
)

func TestIsExecutionRevert(t *testing.T) {
	t.Run("matches execution reverted", func(t *testing.T) {
		if !isExecutionRevert(errors.New("execution reverted: nothing to settle")) {
			t.Fatal("execution reverted error should match")
		}
	})

	t.Run("matches fevm message execution failed", func(t *testing.T) {
		err := errors.New("payments.Settle: message execution failed (exit=[33], revert reason=[0xdeadbeef], vm error=[message failed with backtrace])")
		if !isExecutionRevert(err) {
			t.Fatal("FEVM revert error should match")
		}
	})

	t.Run("does not match unrelated error", func(t *testing.T) {
		if isExecutionRevert(errors.New("context deadline exceeded")) {
			t.Fatal("unrelated error should not match")
		}
	})
}
