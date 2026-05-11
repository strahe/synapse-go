//go:build integration

package integration_test

import (
	"context"
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

func TestIsIntegrationReadTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: true,
		},
		{
			name: "rpc timeout text",
			err:  errors.New(`warmstorage.GetClientDataSetsWithDetails: Post "https://api.calibration.node.glif.io/rpc/v1": context deadline exceeded`),
			want: true,
		},
		{
			name: "eof",
			err:  errors.New("filbeam.GetDataSetStats: EOF"),
			want: true,
		},
		{
			name: "contract revert",
			err:  errors.New("payments.Settle: message execution failed (exit=[33], revert reason=[0xdeadbeef], vm error=[message failed with backtrace])"),
			want: false,
		},
		{
			name: "business error",
			err:  errors.New("warmstorage.GetClientDataSetsWithDetails: dataSetLive dataSetID 7: contract reverted"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isIntegrationReadTransient(tt.err); got != tt.want {
				t.Fatalf("isIntegrationReadTransient()=%v want %v", got, tt.want)
			}
		})
	}
}
