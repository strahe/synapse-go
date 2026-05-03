package idconv_test

import (
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/types"
)

func TestFromBigRoundTrip(t *testing.T) {
	cases := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		new(big.Int).SetUint64(^uint64(0)),
		new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)),
	}
	for _, in := range cases {
		got, err := idconv.FromBig("id", in)
		if err != nil {
			t.Fatalf("FromBig(%s) err: %v", in, err)
		}
		if got.Big().Cmp(in) != 0 {
			t.Fatalf("round-trip: in=%s out=%s", in, got.String())
		}
	}
}

func TestFromBigErrors(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if _, err := idconv.FromBig("id", nil); err == nil {
			t.Fatal("expected error for nil")
		}
	})

	t.Run("negative", func(t *testing.T) {
		if _, err := idconv.FromBig("id", big.NewInt(-1)); err == nil {
			t.Fatal("expected error for negative")
		}
	})

	t.Run("overflow", func(t *testing.T) {
		over := new(big.Int).Lsh(big.NewInt(1), 256)
		if _, err := idconv.FromBig("id", over); err == nil {
			t.Fatal("expected error for overflow")
		}
	})
}

func TestBigSlice(t *testing.T) {
	ids := []types.BigInt{types.NewBigInt(1), types.NewBigInt(2), types.NewBigInt(3)}
	out := idconv.BigSlice(ids)
	if len(out) != 3 {
		t.Fatalf("len=%d", len(out))
	}
	for i, v := range out {
		if v.Cmp(ids[i].Big()) != 0 {
			t.Fatalf("mismatch at %d", i)
		}
	}
	if idconv.BigSlice(nil) != nil {
		t.Fatal("nil input should return nil")
	}
}

func TestFromBigSlice(t *testing.T) {
	in := []*big.Int{big.NewInt(10), big.NewInt(20)}
	out, err := idconv.FromBigSlice("id", in)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 2 || !out[0].Equal(types.NewBigInt(10)) || !out[1].Equal(types.NewBigInt(20)) {
		t.Fatalf("got %+v", out)
	}

	if _, err := idconv.FromBigSlice("id", []*big.Int{big.NewInt(1), nil}); err == nil {
		t.Fatal("expected error for nil element")
	}

	got, err := idconv.FromBigSlice("id", nil)
	if err != nil || got != nil {
		t.Fatalf("nil input: got=%v err=%v", got, err)
	}
}

func TestKey(t *testing.T) {
	id, err := types.BigIntFromBig(new(big.Int).Lsh(big.NewInt(1), 200))
	if err != nil {
		t.Fatalf("BigIntFromBig: %v", err)
	}
	same, err := types.BigIntFromBig(id.Big())
	if err != nil {
		t.Fatalf("BigIntFromBig(same): %v", err)
	}
	other := types.NewBigInt(1)

	if got := idconv.Key(id); len(got) != 32 {
		t.Fatalf("Key len=%d want 32", len(got))
	}
	if idconv.Key(id) != idconv.Key(same) {
		t.Fatal("same numeric ID should have same key")
	}
	if idconv.Key(id) == idconv.Key(other) {
		t.Fatal("different numeric IDs should have different keys")
	}
}
