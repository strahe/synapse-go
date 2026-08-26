package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

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
	pieceCID := mustPieceInfo(t).CIDv2
	clientDataSetID := types.BigInt{}
	commitSubmission := CommitSubmission{
		Kind:            CommitKindCreateAndAdd,
		TransactionID:   common.HexToHash("0x11").Hex(),
		StatusURL:       "https://provider.example/status",
		ProviderID:      providerID,
		Identity:        identity,
		ClientDataSetID: &clientDataSetID,
		PieceCIDs:       []cid.Cid{pieceCID},
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
	createSubmission := CreateDataSetSubmission{
		ProviderID:      providerID,
		TransactionID:   common.HexToHash("0x22").Hex(),
		StatusURL:       "https://provider.example/create",
		ClientDataSetID: &clientDataSetID,
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
		{name: "commit submission", value: commitSubmission, newTarget: func() any { return new(CommitSubmission) }, pascalField: "kind"},
		{name: "commit status", value: commitStatus, newTarget: func() any { return new(CommitStatus) }, pascalField: "kind"},
		{name: "commit result", value: commitResult, newTarget: func() any { return new(CommitResult) }, pascalField: "transactionId"},
		{name: "create submission", value: createSubmission, newTarget: func() any { return new(CreateDataSetSubmission) }, pascalField: "providerId"},
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

func TestCommitSubmissionJSONContract(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	v0Hash, err := multihash.Sum([]byte("cidv0"), multihash.SHA2_256, -1)
	if err != nil {
		t.Fatal(err)
	}
	pieceCIDv0 := cid.NewCidV0(v0Hash)
	clientDataSetID := types.BigInt{}
	submission := CommitSubmission{
		Kind:          CommitKindCreateAndAdd,
		TransactionID: common.HexToHash("0x11").Hex(),
		StatusURL:     "https://provider.example/status",
		ProviderID:    types.NewBigInt(7),
		Identity: ContextIdentity{
			Payer:        common.HexToAddress("0x1234"),
			ChainID:      types.ChainID(314159),
			RecordKeeper: common.HexToAddress("0x5678"),
		},
		ClientDataSetID: &clientDataSetID,
		PieceCIDs:       []cid.Cid{pieceCIDv0, pieceCID},
	}
	encoded, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		"kind", "transactionId", "statusUrl", "providerId", "identity",
		"dataSet", "clientDataSetId", "pieceCids",
	} {
		if _, ok := object[field]; !ok {
			t.Fatalf("missing field %q in %s", field, encoded)
		}
	}
	if len(object) != 8 {
		t.Fatalf("fields=%v", object)
	}
	if string(object["dataSet"]) != "null" || string(object["clientDataSetId"]) != `"0"` {
		t.Fatalf("nullable/zero fields dataSet=%s clientDataSetId=%s", object["dataSet"], object["clientDataSetId"])
	}

	var restored CommitSubmission
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ClientDataSetID == nil || !restored.ClientDataSetID.IsZero() ||
		len(restored.PieceCIDs) != 2 ||
		!restored.PieceCIDs[0].Equals(pieceCIDv0) || restored.PieceCIDs[0].Version() != 0 ||
		!restored.PieceCIDs[1].Equals(pieceCID) || restored.PieceCIDs[1].Version() != pieceCID.Version() {
		t.Fatalf("restored=%+v", restored)
	}

	for name, mutate := range map[string]func(map[string]json.RawMessage){
		"missing": func(raw map[string]json.RawMessage) { delete(raw, "statusUrl") },
		"unknown": func(raw map[string]json.RawMessage) { raw["other"] = json.RawMessage(`1`) },
		"non-nullable null": func(raw map[string]json.RawMessage) {
			raw["pieceCids"] = json.RawMessage(`null`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			var raw map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &raw); err != nil {
				t.Fatal(err)
			}
			mutate(raw)
			invalid, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			before := restored
			if err := json.Unmarshal(invalid, &restored); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v want ErrInvalidArgument", err)
			}
			if !restored.ProviderID.Equal(before.ProviderID) || len(restored.PieceCIDs) != len(before.PieceCIDs) {
				t.Fatal("failed decode mutated receiver")
			}
		})
	}
	if err := json.Unmarshal(append(encoded, []byte(` {}`)...), &restored); err == nil {
		t.Fatal("trailing JSON should be rejected")
	}
	duplicate := bytes.Replace(
		encoded,
		[]byte(`"transactionId":`),
		[]byte(`"transactionId":"duplicate","transactionId":`),
		1,
	)
	if err := json.Unmarshal(duplicate, &restored); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("duplicate field error=%v want ErrInvalidArgument", err)
	}
}

func TestLifecycleJSONRejectsNilReceivers(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{name: "data-set ref", call: func() error { return (*DataSetRef)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "context identity", call: func() error { return (*ContextIdentity)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "commit submission", call: func() error { return (*CommitSubmission)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "commit status", call: func() error { return (*CommitStatus)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "commit result", call: func() error { return (*CommitResult)(nil).UnmarshalJSON([]byte(`{}`)) }},
		{name: "create submission", call: func() error { return (*CreateDataSetSubmission)(nil).UnmarshalJSON([]byte(`{}`)) }},
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

func TestCommitSubmissionJSONRejectsMalformedTopLevelAndNestedValues(t *testing.T) {
	var submission CommitSubmission
	for _, data := range [][]byte{
		[]byte(`{`),
		[]byte(`null`),
		[]byte(`{"kind":"addPieces","transactionId":"tx","statusUrl":"https://provider.example/status","providerId":"7","identity":"invalid","dataSet":null,"clientDataSetId":null,"pieceCids":[]}`),
	} {
		if err := submission.UnmarshalJSON(data); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("JSON=%s error=%v want ErrInvalidArgument", data, err)
		}
	}
}
