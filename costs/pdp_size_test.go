package costs

import (
	"encoding/binary"
	"math/big"
	"testing"

	commcid "github.com/filecoin-project/go-fil-commcid"
	mh "github.com/multiformats/go-multihash"
	"github.com/strahe/synapse-go/piece"
)

// The contract oracle is FilecoinServicesRef's
// PriceListUSDFC.calculateStorageRate and its pinned Cids dependency. Recheck
// these vectors when the contract baseline changes.
func TestPDPSize_RealPieceCIDs(t *testing.T) {
	for _, tc := range []struct{ raw, leaves, billed uint64 }{
		{127, 4, 127}, {128, 5, 158}, {159, 6, 190}, {190, 6, 190},
	} {
		info, err := piece.CalculateFromBytes(make([]byte, tc.raw))
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := mh.Decode(info.CIDv2.Hash())
		if err != nil {
			t.Fatal(err)
		}
		padding, n := binary.Uvarint(decoded.Digest)
		if n <= 0 || n >= len(decoded.Digest) {
			t.Fatal("invalid CID padding")
		}
		cidLeaves := leavesFromHeightAndPadding(decoded.Digest[n], padding)
		leaves := pieceSizesToLeafCount([]uint64{tc.raw})
		if leaves.Cmp(cidLeaves) != 0 || leaves.Uint64() != tc.leaves {
			t.Fatalf("raw=%d: leaves=%s CID leaves=%s want %d", tc.raw, leaves, cidLeaves, tc.leaves)
		}
		if got := leafCountToBillableBytes(leaves); got.Uint64() != tc.billed {
			t.Fatalf("raw=%d: billable bytes=%s want %d", tc.raw, got, tc.billed)
		}
	}
}

func leavesFromHeightAndPadding(height uint8, padding uint64) *big.Int {
	leaves := new(big.Int).Lsh(big.NewInt(1), uint(height))
	fullyPadded := new(big.Int).SetUint64(padding)
	fullyPadded.Mul(fullyPadded, big.NewInt(128))
	fullyPadded.Div(fullyPadded, big.NewInt(127))
	fullyPadded.Div(fullyPadded, big.NewInt(32))
	return leaves.Sub(leaves, fullyPadded)
}

func TestPDPSize_MatchesCommCIDHeightAndPadding(t *testing.T) {
	check := func(raw uint64) {
		t.Helper()
		height, padding, err := commcid.PayloadSizeToV1TreeHeightAndPadding(raw)
		if err != nil {
			t.Fatalf("raw=%d: %v", raw, err)
		}
		got := pieceSizesToLeafCount([]uint64{raw})
		want := leavesFromHeightAndPadding(height, padding)
		if got.Cmp(want) != 0 {
			t.Fatalf("raw=%d height=%d padding=%d: leaves=%s want %s", raw, height, padding, got, want)
		}
	}
	for raw := uint64(127); raw <= 65_536; raw++ {
		check(raw)
	}
	for k := uint(0); k <= 40; k++ {
		boundary := uint64(127) << k
		for delta := int64(-2); delta <= 2; delta++ {
			raw := uint64(int64(boundary) + delta)
			if raw >= 127 {
				check(raw)
			}
		}
	}
}

func TestPDPSize_ArbitraryPrecisionAndZeroValues(t *testing.T) {
	for _, tc := range []struct {
		sizes          []uint64
		leaves, billed string
	}{
		{nil, "0", "0"},
		{[]uint64{}, "0", "0"},
		{[]uint64{0, 128, 0}, "5", "158"},
		{[]uint64{128, 190}, "11", "349"},
		{[]uint64{159, 159}, "12", "381"},
		{[]uint64{^uint64(0), ^uint64(0)}, "1161999626690365458", "36893488147419103291"},
	} {
		leaves := pieceSizesToLeafCount(tc.sizes)
		before := leaves.String()
		billed := leafCountToBillableBytes(leaves)
		if leaves.String() != tc.leaves || billed.String() != tc.billed || leaves.String() != before {
			t.Fatalf("sizes=%v: leaves=%s billed=%s want %s and %s", tc.sizes, leaves, billed, tc.leaves, tc.billed)
		}
	}
}
