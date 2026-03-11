package txutil

import (
	"context"
	"errors"
	"math"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum"
)

type fakeGasClient struct {
	limit    uint64
	price    *big.Int
	tip      *big.Int
	estErr   error
	priceErr error
	tipErr   error
}

func (f *fakeGasClient) EstimateGas(_ context.Context, _ ethereum.CallMsg) (uint64, error) {
	return f.limit, f.estErr
}

func (f *fakeGasClient) SuggestGasPrice(_ context.Context) (*big.Int, error) {
	return f.price, f.priceErr
}

func (f *fakeGasClient) SuggestGasTipCap(_ context.Context) (*big.Int, error) {
	return f.tip, f.tipErr
}

func TestEstimateGasWithBuffer(t *testing.T) {
	c := &fakeGasClient{limit: 100_000}
	tests := []struct {
		buffer int
		want   uint64
		err    bool
	}{
		{0, 100_000, false},
		{20, 120_000, false},
		{100, 200_000, false},
		{-1, 0, true},
		{1001, 0, true},
	}
	for _, tc := range tests {
		got, err := EstimateGasWithBuffer(context.Background(), c, ethereum.CallMsg{}, tc.buffer)
		if (err != nil) != tc.err {
			t.Fatalf("buffer=%d err=%v wantErr=%v", tc.buffer, err, tc.err)
		}
		if err == nil && got != tc.want {
			t.Fatalf("buffer=%d got=%d want=%d", tc.buffer, got, tc.want)
		}
	}
}

func TestEstimateGasWithBuffer_EstimateErr(t *testing.T) {
	boom := errors.New("revert")
	c := &fakeGasClient{estErr: boom}
	_, err := EstimateGasWithBuffer(context.Background(), c, ethereum.CallMsg{}, 10)
	if !errors.Is(err, boom) {
		t.Fatalf("want wrapped boom, got %v", err)
	}
}

func TestEstimateGasWithBuffer_LargeUint64(t *testing.T) {
	c := &fakeGasClient{limit: math.MaxInt64 + 10}
	got, err := EstimateGasWithBuffer(context.Background(), c, ethereum.CallMsg{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	const want uint64 = 10145709240540253398
	if got != want {
		t.Fatalf("got=%d want=%d", got, want)
	}
}

func TestSuggestGasPriceWithMultiplier(t *testing.T) {
	c := &fakeGasClient{price: big.NewInt(1_000_000_000)}
	got, err := SuggestGasPriceWithMultiplier(context.Background(), c, 1.5)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmp(big.NewInt(1_500_000_000)) != 0 {
		t.Fatalf("got %s", got)
	}
	// multiplier <=1 returns copy unchanged
	got, _ = SuggestGasPriceWithMultiplier(context.Background(), c, 1.0)
	if got.Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatalf("got %s", got)
	}
	// Ensure returned value is independent of source.
	got.Add(got, big.NewInt(1))
	if c.price.Cmp(big.NewInt(1_000_000_000)) != 0 {
		t.Fatal("source mutated")
	}
}

func TestSuggestGasTipCapWithMultiplier(t *testing.T) {
	c := &fakeGasClient{tip: big.NewInt(2_000_000_000)}
	got, err := SuggestGasTipCapWithMultiplier(context.Background(), c, 1.25)
	if err != nil {
		t.Fatal(err)
	}
	if got.Cmp(big.NewInt(2_500_000_000)) != 0 {
		t.Fatalf("got %s", got)
	}
}

func TestSuggestGasPriceWithMultiplier_LargeWeiPrecision(t *testing.T) {
	v := new(big.Int).Lsh(big.NewInt(1), 200)
	c := &fakeGasClient{price: v}
	got, err := SuggestGasPriceWithMultiplier(context.Background(), c, 1.25)
	if err != nil {
		t.Fatal(err)
	}
	want := new(big.Int).Add(v, new(big.Int).Rsh(new(big.Int).Set(v), 2))
	if got.Cmp(want) != 0 {
		t.Fatalf("got %s want %s", got, want)
	}
}
