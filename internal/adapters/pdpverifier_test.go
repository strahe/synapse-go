package adapters

import (
	"context"
	"errors"
	"math/big"
	"slices"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"

	pdpverifierbind "github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/storage"
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

func (e revertDataError) ErrorData() any {
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

func TestPDPVerifierReader_FindPieceIDsByCIDs_UsesChunkedMulticall(t *testing.T) {
	target := common.HexToAddress("0x1111111111111111111111111111111111111111")
	backend := newPDPBatchTestBackend(t, target, [][]*big.Int{
		{big.NewInt(10)},
		{big.NewInt(20), big.NewInt(21)},
		{},
	})
	caller, err := pdpverifierbind.NewPDPVerifierCaller(target, backend)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}
	reader := NewPDPVerifierReader(caller, backend, target, 2).(*pdpVerifierReader)
	pieceCIDs := []cid.Cid{testPieceCID, testPieceCID, testPieceCID}

	got, err := reader.FindPieceIDsByCIDs(context.Background(), types.NewBigInt(42), pieceCIDs)
	if err != nil {
		t.Fatalf("FindPieceIDsByCIDs: %v", err)
	}
	if len(got) != 3 || !equalAdapterBigInts(got[0], []types.BigInt{types.NewBigInt(10)}) ||
		!equalAdapterBigInts(got[1], []types.BigInt{types.NewBigInt(20), types.NewBigInt(21)}) || len(got[2]) != 0 {
		t.Fatalf("piece IDs=%v want [[10] [20 21] []]", got)
	}
	if !slices.Equal(backend.batchSizes, []int{2, 1}) {
		t.Fatalf("multicall batch sizes=%v want [2 1]", backend.batchSizes)
	}
}

func TestPDPVerifierReader_FindPieceIDsByCIDs_ClassifiesUnavailableSubcall(t *testing.T) {
	target := common.HexToAddress("0x1111111111111111111111111111111111111111")
	backend := newPDPBatchTestBackend(t, target, [][]*big.Int{nil})
	hash := crypto.Keccak256([]byte("DataSetNotFound()"))
	backend.failures = map[int][]byte{0: hash[:4]}
	caller, err := pdpverifierbind.NewPDPVerifierCaller(target, backend)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}
	reader := NewPDPVerifierReader(caller, backend, target, 2).(*pdpVerifierReader)

	got, err := reader.FindPieceIDsByCIDs(context.Background(), types.NewBigInt(42), []cid.Cid{testPieceCID})
	if got != nil || !errors.Is(err, storage.ErrDataSetUnavailable) {
		t.Fatalf("result=%v error=%v, want unavailable sentinel", got, err)
	}
}

func TestPDPVerifierReader_FindPieceIDsByCIDs_PreservesUnknownSubcallData(t *testing.T) {
	target := common.HexToAddress("0x1111111111111111111111111111111111111111")
	backend := newPDPBatchTestBackend(t, target, [][]*big.Int{nil})
	backend.failures = map[int][]byte{0: {0xde, 0xad}}
	caller, err := pdpverifierbind.NewPDPVerifierCaller(target, backend)
	if err != nil {
		t.Fatalf("NewPDPVerifierCaller: %v", err)
	}
	reader := NewPDPVerifierReader(caller, backend, target, 2).(*pdpVerifierReader)

	_, err = reader.FindPieceIDsByCIDs(context.Background(), types.NewBigInt(42), []cid.Cid{testPieceCID})
	if err == nil || errors.Is(err, storage.ErrDataSetUnavailable) || !strings.Contains(err.Error(), "0xdead") {
		t.Fatalf("error = %v, want raw unknown failure without unavailable sentinel", err)
	}
}

func TestPDPVerifierReader_ReturnsSentinelForUnavailableDataSet(t *testing.T) {
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
			if got != nil || !errors.Is(err, storage.ErrDataSetUnavailable) || !errors.Is(err, tt.err) {
				t.Fatalf("piece ids=%v error=%v, want nil and unavailable sentinel", got, err)
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
			if got != nil || !errors.Is(err, storage.ErrDataSetUnavailable) || !errors.Is(err, tt.err) {
				t.Fatalf("scheduled removals=%v error=%v, want nil and unavailable sentinel", got, err)
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
			if got != nil || !errors.Is(err, storage.ErrDataSetUnavailable) || !errors.Is(err, tt.err) {
				t.Fatalf("next challenge epoch=%v error=%v, want nil and unavailable sentinel", got, err)
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
			if got != nil || !errors.Is(err, storage.ErrDataSetUnavailable) || !errors.Is(err, tt.err) {
				t.Fatalf("dataset size=%v error=%v, want nil and unavailable sentinel", got, err)
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

func equalAdapterBigInts(got, want []types.BigInt) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !got[i].Equal(want[i]) {
			return false
		}
	}
	return true
}

const pdpBatchTestMulticallABI = `[{"inputs":[{"components":[{"name":"target","type":"address"},{"name":"allowFailure","type":"bool"},{"name":"callData","type":"bytes"}],"name":"calls","type":"tuple[]"}],"name":"aggregate3","outputs":[{"components":[{"name":"success","type":"bool"},{"name":"returnData","type":"bytes"}],"name":"returnData","type":"tuple[]"}],"stateMutability":"payable","type":"function"}]`

type pdpBatchTestBackend struct {
	t          *testing.T
	target     common.Address
	multicall  abi.ABI
	pdp        abi.ABI
	replies    [][]*big.Int
	failures   map[int][]byte
	nextReply  int
	batchSizes []int
}

func newPDPBatchTestBackend(t *testing.T, target common.Address, replies [][]*big.Int) *pdpBatchTestBackend {
	t.Helper()
	multicall, err := abi.JSON(strings.NewReader(pdpBatchTestMulticallABI))
	if err != nil {
		t.Fatalf("parse multicall ABI: %v", err)
	}
	pdp, err := pdpverifierbind.PDPVerifierMetaData.GetAbi()
	if err != nil {
		t.Fatalf("parse PDPVerifier ABI: %v", err)
	}
	return &pdpBatchTestBackend{t: t, target: target, multicall: multicall, pdp: *pdp, replies: replies}
}

func (b *pdpBatchTestBackend) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return []byte{0x01}, nil
}

func (b *pdpBatchTestBackend) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	method := b.multicall.Methods["aggregate3"]
	if len(msg.Data) < 4 || !slices.Equal(msg.Data[:4], method.ID) {
		return nil, errors.New("unexpected contract call")
	}
	values, err := method.Inputs.Unpack(msg.Data[4:])
	if err != nil {
		return nil, err
	}
	calls := values[0].([]struct {
		Target       common.Address `json:"target"`
		AllowFailure bool           `json:"allowFailure"`
		CallData     []byte         `json:"callData"`
	})
	b.batchSizes = append(b.batchSizes, len(calls))
	results := make([]struct {
		Success    bool   `json:"success"`
		ReturnData []byte `json:"returnData"`
	}, len(calls))
	pdpMethod := b.pdp.Methods["findPieceIdsByCid"]
	for i, call := range calls {
		if call.Target != b.target || !call.AllowFailure {
			b.t.Fatalf("call[%d]=(target %s, allowFailure %t), want (%s, true)", i, call.Target, call.AllowFailure, b.target)
		}
		if len(call.CallData) < 4 || !slices.Equal(call.CallData[:4], pdpMethod.ID) {
			b.t.Fatalf("call[%d] does not invoke findPieceIdsByCid", i)
		}
		args, err := pdpMethod.Inputs.Unpack(call.CallData[4:])
		if err != nil {
			return nil, err
		}
		if args[0].(*big.Int).Cmp(big.NewInt(42)) != 0 || args[2].(*big.Int).Sign() != 0 || args[3].(*big.Int).Cmp(big.NewInt(1)) != 0 {
			b.t.Fatalf("call[%d] has unexpected findPieceIdsByCid arguments", i)
		}
		resultIndex := b.nextReply
		b.nextReply++
		if failure, ok := b.failures[resultIndex]; ok {
			results[i].Success = false
			results[i].ReturnData = failure
			continue
		}
		if resultIndex >= len(b.replies) {
			b.t.Fatalf("call[%d] has no configured reply", i)
		}
		returnData, err := pdpMethod.Outputs.Pack(b.replies[resultIndex])
		if err != nil {
			return nil, err
		}
		results[i].Success = true
		results[i].ReturnData = returnData
	}
	return method.Outputs.Pack(results)
}

func (b *pdpBatchTestBackend) BlockNumber(context.Context) (uint64, error) {
	return 0, nil
}
