package adapters

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	pdpverifierbind "github.com/strahe/synapse-go/internal/contracts/pdpverifier"
)

func TestPDPVerifierReader_GetScheduledRemovals_Dedupes(t *testing.T) {
	mc := newStorageInfoTestCaller(t)
	mc.setPDPReply(t, "getScheduledRemovals", []*big.Int{big.NewInt(2), big.NewInt(2), big.NewInt(5)})

	caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}

	got, err := (&pdpVerifierReader{caller: caller}).GetScheduledRemovals(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetScheduledRemovals: %v", err)
	}
	if len(got) != 2 || got[0] != 2 || got[1] != 5 {
		t.Fatalf("scheduled removals=%v want [2 5]", got)
	}
}

func TestPDPVerifierReader_GetScheduledRemovals_DataSetNotLiveReturnsEmpty(t *testing.T) {
	mc := newStorageInfoTestCaller(t)
	mc.setPDPError("getScheduledRemovals", errors.New("execution reverted: Data set not live"))

	caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}

	got, err := (&pdpVerifierReader{caller: caller}).GetScheduledRemovals(context.Background(), 42)
	if err != nil {
		t.Fatalf("GetScheduledRemovals: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("scheduled removals=%v want empty", got)
	}
}

func TestPDPVerifierReader_GetNextChallengeEpoch_ReturnsNilForUnavailableEpoch(t *testing.T) {
	t.Run("not live", func(t *testing.T) {
		mc := newStorageInfoTestCaller(t)
		mc.setPDPError("getNextChallengeEpoch", errors.New("execution reverted: Data set not live"))

		caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
		if err != nil {
			t.Fatalf("NewPDPVerifierCaller: %v", err)
		}

		got, err := (&pdpVerifierReader{caller: caller}).GetNextChallengeEpoch(context.Background(), 42)
		if err != nil {
			t.Fatalf("GetNextChallengeEpoch: %v", err)
		}
		if got != nil {
			t.Fatalf("next challenge epoch=%v want nil", got)
		}
	})

	t.Run("non positive", func(t *testing.T) {
		mc := newStorageInfoTestCaller(t)
		mc.setPDPReply(t, "getNextChallengeEpoch", big.NewInt(0))

		caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
		if err != nil {
			t.Fatalf("NewPDPVerifierCaller: %v", err)
		}

		got, err := (&pdpVerifierReader{caller: caller}).GetNextChallengeEpoch(context.Background(), 42)
		if err != nil {
			t.Fatalf("GetNextChallengeEpoch: %v", err)
		}
		if got != nil {
			t.Fatalf("next challenge epoch=%v want nil", got)
		}
	})
}
