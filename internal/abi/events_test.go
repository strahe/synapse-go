package abi

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/strahe/synapse-go/internal/contracts/fwss"
)

func TestExtractEvent_NoReceipt(t *testing.T) {
	if err := ExtractEvent(nil, nil, "X", common.Address{}, nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractEvent_NilABI(t *testing.T) {
	receipt := &types.Receipt{Logs: []*types.Log{}}
	err := ExtractEvent(receipt, nil, "Foo", common.Address{}, nil)
	if err == nil {
		t.Fatal("expected error for nil abi")
	}
}

func TestExtractEvent_ReceiptWithNilLog(t *testing.T) {
	parsed, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	// Receipt has a nil log entry and an empty-topics log — neither should match.
	receipt := &types.Receipt{
		Logs: []*types.Log{nil, {Topics: []common.Hash{}}},
	}
	err = ExtractEvent(receipt, parsed, "DataSetCreated", common.Address{}, nil)
	if !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("want ErrEventNotFound, got %v", err)
	}
}

func TestExtractEvent_UnpackError(t *testing.T) {
	parsed, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := parsed.Events["DataSetCreated"]
	if !ok {
		t.Skip("DataSetCreated event not in ABI")
	}
	emitter := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")
	// Craft a log with matching topic but garbage data so unpack fails.
	log := &types.Log{
		Address: emitter,
		Topics:  []common.Hash{ev.ID},
		Data:    []byte{0x01, 0x02, 0x03}, // invalid ABI-encoded data
	}
	receipt := &types.Receipt{Logs: []*types.Log{log}}

	type dummy struct{ X *big.Int }
	var d dummy
	err = ExtractEvent(receipt, parsed, "DataSetCreated", emitter, &d)
	if err == nil {
		t.Fatal("expected unpack error")
	}
}

func TestExtractEvent_Found(t *testing.T) {
	parsed, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}

	// Build a synthetic DataSetCreated log. Signature:
	//   DataSetCreated(uint256 indexed dataSetId, address indexed payer,
	//                  address indexed payee, address serviceProvider, ...)
	// For a minimal round-trip we only need the event topic[0] to match and
	// the Data section to be decodable. We pack the non-indexed args using
	// the ABI.
	ev, ok := parsed.Events["DataSetCreated"]
	if !ok {
		t.Skip("DataSetCreated event not present in current ABI")
	}

	// indexed-topic placeholders (dataSetId, payer, payee)
	topic1 := common.BigToHash(big.NewInt(7))
	topic2 := common.HexToHash("0x000000000000000000000000aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	topic3 := common.HexToHash("0x000000000000000000000000bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	// Build non-indexed args using the ABI event's inputs.
	var nonIndexedArgs []any
	for _, in := range ev.Inputs {
		if in.Indexed {
			continue
		}
		switch in.Type.String() {
		case "address":
			nonIndexedArgs = append(nonIndexedArgs, common.HexToAddress("0x1111111111111111111111111111111111111111"))
		case "uint256":
			nonIndexedArgs = append(nonIndexedArgs, big.NewInt(1))
		case "bool":
			nonIndexedArgs = append(nonIndexedArgs, true)
		case "string":
			nonIndexedArgs = append(nonIndexedArgs, "")
		case "bytes":
			nonIndexedArgs = append(nonIndexedArgs, []byte{})
		default:
			t.Skipf("unhandled input type %s in DataSetCreated - this test must be adapted", in.Type.String())
		}
	}
	data, err := ev.Inputs.NonIndexed().Pack(nonIndexedArgs...)
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	emitter := common.HexToAddress("0xcccccccccccccccccccccccccccccccccccccccc")
	log := &types.Log{
		Address: emitter,
		Topics:  []common.Hash{ev.ID, topic1, topic2, topic3},
		Data:    data,
	}
	receipt := &types.Receipt{
		Logs: []*types.Log{{Topics: []common.Hash{crypto.Keccak256Hash([]byte("other"))}}, log},
	}

	// Use a nil dst (caller only wanted presence check).
	if err := ExtractEvent(receipt, parsed, "DataSetCreated", emitter, nil); err != nil {
		t.Fatalf("extract: %v", err)
	}

	// Wrong emitter filter returns not found.
	err = ExtractEvent(receipt, parsed, "DataSetCreated",
		common.HexToAddress("0xdddddddddddddddddddddddddddddddddddddddd"), nil)
	if !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("want ErrEventNotFound, got %v", err)
	}
}

func TestExtractEvent_NotInABI(t *testing.T) {
	parsed, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	err = ExtractEvent(&types.Receipt{}, parsed, "NoSuchEvent", common.Address{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractEvent_NoMatch(t *testing.T) {
	parsed, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	err = ExtractEvent(&types.Receipt{Logs: []*types.Log{{Topics: []common.Hash{crypto.Keccak256Hash([]byte("unrelated"))}}}},
		parsed, "DataSetCreated", common.Address{}, nil)
	if !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("want ErrEventNotFound, got %v", err)
	}
}
