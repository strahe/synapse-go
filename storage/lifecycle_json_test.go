package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

func TestLifecycleJSONUsesStrictLowerCamelFields(t *testing.T) {
	providerID, err := types.ParseBigInt("340282366920938463463374607431768211457")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := NewDataSetRef(providerID, types.NewBigInt(42), types.BigInt{})
	if err != nil {
		t.Fatal(err)
	}
	identity := ContextIdentity{
		Payer:        common.HexToAddress("0x1234"),
		ChainID:      types.ChainID(314159),
		RecordKeeper: common.HexToAddress("0x5678"),
	}
	commitStatus := CommitStatus{
		Kind:          CommitKindAddPieces,
		State:         CommitStatePending,
		TransactionID: common.HexToHash("0x11").Hex(),
		DataSet:       &ref,
	}
	commitResult := CommitResult{
		TransactionID: common.HexToHash("0x11").Hex(),
		DataSet:       ref,
		PieceIDs:      []types.BigInt{types.NewBigInt(7)},
	}
	createResult := CreateDataSetResult{
		TransactionID: common.HexToHash("0x22").Hex(),
		DataSet:       ref,
	}

	tests := []struct {
		name        string
		value       any
		newTarget   func() any
		pascalField string
	}{
		{name: "data-set ref", value: ref, newTarget: func() any { return new(DataSetRef) }, pascalField: "providerId"},
		{name: "context identity", value: identity, newTarget: func() any { return new(ContextIdentity) }, pascalField: "payer"},
		{name: "commit status", value: commitStatus, newTarget: func() any { return new(CommitStatus) }, pascalField: "kind"},
		{name: "commit result", value: commitResult, newTarget: func() any { return new(CommitResult) }, pascalField: "transactionId"},
		{name: "create result", value: createResult, newTarget: func() any { return new(CreateDataSetResult) }, pascalField: "transactionId"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, test.newTarget()); err != nil {
				t.Fatalf("round trip: %v, JSON=%s", err, encoded)
			}
			pascal := strings.ToUpper(test.pascalField[:1]) + test.pascalField[1:]
			legacy := bytes.Replace(encoded, []byte(`"`+test.pascalField+`"`), []byte(`"`+pascal+`"`), 1)
			if err := json.Unmarshal(legacy, test.newTarget()); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("PascalCase error=%v want ErrInvalidArgument, JSON=%s", err, legacy)
			}
		})
	}
}

func TestLifecycleJSONRejectsNilReceivers(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "data-set ref", call: func() error { return (*DataSetRef)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "context identity", call: func() error { return (*ContextIdentity)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "commit status", call: func() error { return (*CommitStatus)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "commit result", call: func() error { return (*CommitResult)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "create result", call: func() error { return (*CreateDataSetResult)(nil).UnmarshalJSON([]byte(`{}`)) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v want ErrInvalidArgument", err)
			}
		})
	}
}
