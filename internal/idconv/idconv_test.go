package idconv_test

import (
	"math"
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/types"
)

func TestBigRoundTrip(t *testing.T) {
	cases := []uint64{0, 1, math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64}
	for _, in := range cases {
		id := types.DataSetID(in)
		big := idconv.Big(id)
		got, err := idconv.Safe[types.DataSetID]("ds", big)
		if err != nil {
			t.Fatalf("Safe(%d) err: %v", in, err)
		}
		if uint64(got) != in {
			t.Fatalf("round-trip: in=%d out=%d", in, uint64(got))
		}
	}
}

func TestSafeErrors(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if _, err := idconv.Safe[types.ProviderID]("pid", nil); err == nil {
			t.Fatal("expected error for nil")
		}
	})

	t.Run("negative", func(t *testing.T) {
		if _, err := idconv.Safe[types.ProviderID]("pid", big.NewInt(-1)); err == nil {
			t.Fatal("expected error for negative")
		}
	})

	t.Run("overflow", func(t *testing.T) {
		over := new(big.Int).Lsh(big.NewInt(1), 65)
		if _, err := idconv.Safe[types.ProviderID]("pid", over); err == nil {
			t.Fatal("expected error for overflow")
		}
	})
}

func TestBigSlice(t *testing.T) {
	ids := []types.ProviderID{1, 2, 3}
	out := idconv.BigSlice(ids)
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	for i, v := range out {
		if v.Uint64() != uint64(ids[i]) {
			t.Fatalf("mismatch at %d", i)
		}
	}
	if idconv.BigSlice[types.ProviderID](nil) != nil {
		t.Fatal("nil input should return nil")
	}
}

func TestSafeSlice(t *testing.T) {
	in := []*big.Int{big.NewInt(10), big.NewInt(20)}
	out, err := idconv.SafeSlice[types.DataSetID]("ds", in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 2 || out[0] != 10 || out[1] != 20 {
		t.Fatalf("got %+v", out)
	}

	if _, err := idconv.SafeSlice[types.DataSetID]("ds", []*big.Int{big.NewInt(1), nil}); err == nil {
		t.Fatal("expected error for nil element")
	}

	got, err := idconv.SafeSlice[types.DataSetID]("ds", nil)
	if err != nil || got != nil {
		t.Fatalf("nil input: got=%v err=%v", got, err)
	}
}
