package adapters

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"

	pdpverifierbind "github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/types"
)

type revertDataError struct {
	data string
}

func (e revertDataError) Error() string {
	return "execution reverted"
}

func (e revertDataError) ErrorCode() int {
	return 3
}

func (e revertDataError) ErrorData() interface{} {
	return e.data
}

func pdpCustomError(name string) error {
	hash := crypto.Keccak256([]byte(name + "()"))
	return revertDataError{data: hexutil.Encode(hash[:4])}
}

func TestPDPVerifierReader_GetScheduledRemovals_Dedupes(t *testing.T) {
	mc := newStorageInfoTestCaller(t)
	mc.setPDPReply(t, "getScheduledRemovals", []*big.Int{big.NewInt(2), big.NewInt(2), big.NewInt(5)})

	caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}

	got, err := (&pdpVerifierReader{caller: caller}).GetScheduledRemovals(context.Background(), types.NewBigInt(42))
	if err != nil {
		t.Fatalf("GetScheduledRemovals: %v", err)
	}
	if len(got) != 2 || !got[0].Equal(types.NewBigInt(2)) || !got[1].Equal(types.NewBigInt(5)) {
		t.Fatalf("scheduled removals=%v want [2 5]", got)
	}
}

func TestPDPVerifierReader_ReturnsEmptyValuesForUnavailableDataSet(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "legacy not live string", err: errors.New("execution reverted: Data set not live")},
		{name: "custom not live string", err: errors.New("execution reverted: DataSetNotLive()")},
		{name: "custom not found string", err: errors.New("execution reverted: DataSetNotFound()")},
		{name: "custom not live selector", err: pdpCustomError("DataSetNotLive")},
		{name: "custom not found selector", err: pdpCustomError("DataSetNotFound")},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/piece ids", func(t *testing.T) {
			mc := newStorageInfoTestCaller(t)
			mc.setPDPError("findPieceIdsByCid", tt.err)

			caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
			if err != nil {
				t.Fatalf("NewPDPVerifierCaller: %v", err)
			}

			got, err := (&pdpVerifierReader{caller: caller}).FindPieceIdsByCid(context.Background(), types.NewBigInt(42), testPieceCID, 0, 1)
			if err != nil {
				t.Fatalf("FindPieceIdsByCid: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("piece ids=%v want empty", got)
			}
		})

		t.Run(tt.name+"/scheduled removals", func(t *testing.T) {
			mc := newStorageInfoTestCaller(t)
			mc.setPDPError("getScheduledRemovals", tt.err)

			caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
			if err != nil {
				t.Fatalf("NewPDPVerifierCaller: %v", err)
			}

			got, err := (&pdpVerifierReader{caller: caller}).GetScheduledRemovals(context.Background(), types.NewBigInt(42))
			if err != nil {
				t.Fatalf("GetScheduledRemovals: %v", err)
			}
			if len(got) != 0 {
				t.Fatalf("scheduled removals=%v want empty", got)
			}
		})

		t.Run(tt.name+"/next challenge epoch", func(t *testing.T) {
			mc := newStorageInfoTestCaller(t)
			mc.setPDPError("getNextChallengeEpoch", tt.err)

			caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
			if err != nil {
				t.Fatalf("NewPDPVerifierCaller: %v", err)
			}

			got, err := (&pdpVerifierReader{caller: caller}).GetNextChallengeEpoch(context.Background(), types.NewBigInt(42))
			if err != nil {
				t.Fatalf("GetNextChallengeEpoch: %v", err)
			}
			if got != nil {
				t.Fatalf("next challenge epoch=%v want nil", got)
			}
		})

		t.Run(tt.name+"/dataset size", func(t *testing.T) {
			mc := newStorageInfoTestCaller(t)
			mc.setPDPError("getDataSetLeafCount", tt.err)

			caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
			if err != nil {
				t.Fatalf("NewPDPVerifierCaller: %v", err)
			}

			got, err := (&pdpVerifierReader{caller: caller}).GetDataSetSizeBytes(context.Background(), types.NewBigInt(42))
			if err != nil {
				t.Fatalf("GetDataSetSizeBytes: %v", err)
			}
			if got.Sign() != 0 {
				t.Fatalf("dataset size=%v want 0", got)
			}
		})
	}
}

func TestPDPVerifierReader_PropagatesOrdinaryErrors(t *testing.T) {
	tests := []struct {
		name   string
		method string
		call   func(context.Context, *pdpVerifierReader) error
	}{
		{
			name:   "piece ids",
			method: "findPieceIdsByCid",
			call: func(ctx context.Context, reader *pdpVerifierReader) error {
				_, err := reader.FindPieceIdsByCid(ctx, types.NewBigInt(42), testPieceCID, 0, 1)
				return err
			},
		},
		{
			name:   "scheduled removals",
			method: "getScheduledRemovals",
			call: func(ctx context.Context, reader *pdpVerifierReader) error {
				_, err := reader.GetScheduledRemovals(ctx, types.NewBigInt(42))
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mc := newStorageInfoTestCaller(t)
			mc.setPDPError(tt.method, errors.New("rpc unavailable"))

			caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
			if err != nil {
				t.Fatalf("NewPDPVerifierCaller: %v", err)
			}

			err = tt.call(context.Background(), &pdpVerifierReader{caller: caller})
			if err == nil {
				t.Fatal("expected ordinary error")
			}
		})
	}
}

var testPieceCID = cid.MustParse("baga6ea4seaqao7s73y24kcutaosvacpdjgfe74urr3enp3bccbm2fszfxwqvria")

func TestPDPVerifierReader_GetNextChallengeEpoch_ReturnsNilForNonPositiveEpoch(t *testing.T) {
	t.Run("zero", func(t *testing.T) {
		mc := newStorageInfoTestCaller(t)
		mc.setPDPReply(t, "getNextChallengeEpoch", big.NewInt(0))

		caller, err := pdpverifierbind.NewPDPVerifierCaller(common.Address{}, mc)
		if err != nil {
			t.Fatalf("NewPDPVerifierCaller: %v", err)
		}

		got, err := (&pdpVerifierReader{caller: caller}).GetNextChallengeEpoch(context.Background(), types.NewBigInt(42))
		if err != nil {
			t.Fatalf("GetNextChallengeEpoch: %v", err)
		}
		if got != nil {
			t.Fatalf("next challenge epoch=%v want nil", got)
		}
	})
}
