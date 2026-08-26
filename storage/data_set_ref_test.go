package storage

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/strahe/synapse-go/types"
)

func TestNewDataSetRefValidationAndExplicitZeroClientID(t *testing.T) {
	providerID := types.NewBigInt(7)
	dataSetID := types.NewBigInt(11)

	for name, call := range map[string]func() error{
		"zero provider": func() error {
			_, err := NewDataSetRef(types.BigInt{}, dataSetID, types.BigInt{})
			return err
		},
		"zero data set": func() error {
			_, err := NewDataSetRef(providerID, types.BigInt{}, types.BigInt{})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v want ErrInvalidArgument", err)
			}
		})
	}

	ref, err := NewDataSetRef(providerID, dataSetID, types.BigInt{})
	if err != nil {
		t.Fatalf("NewDataSetRef: %v", err)
	}
	if !ref.ClientDataSetID().IsZero() {
		t.Fatalf("ClientDataSetID=%s want explicit zero", ref.ClientDataSetID().String())
	}
}

func TestDataSetRefCopiesInputsAndAccessors(t *testing.T) {
	providerID := types.NewBigInt(7)
	dataSetID := types.NewBigInt(11)
	clientDataSetID := types.NewBigInt(13)
	ref, err := NewDataSetRef(providerID, dataSetID, clientDataSetID)
	if err != nil {
		t.Fatalf("NewDataSetRef: %v", err)
	}

	for target, value := range map[*types.BigInt]string{
		&providerID:      "70",
		&dataSetID:       "110",
		&clientDataSetID: "130",
	} {
		if err := target.UnmarshalText([]byte(value)); err != nil {
			t.Fatalf("mutate constructor input: %v", err)
		}
	}
	if !ref.ProviderID().Equal(types.NewBigInt(7)) ||
		!ref.DataSetID().Equal(types.NewBigInt(11)) ||
		!ref.ClientDataSetID().Equal(types.NewBigInt(13)) {
		t.Fatalf("ref changed with constructor inputs: %+v", ref)
	}

	got := ref.DataSetID()
	if !got.Equal(types.NewBigInt(11)) {
		t.Fatalf("DataSetID=%s want 11", got.String())
	}
	if err := got.UnmarshalText([]byte("99")); err != nil {
		t.Fatalf("mutate accessor result: %v", err)
	}
	if !got.Equal(types.NewBigInt(99)) {
		t.Fatalf("mutated accessor result=%s want 99", got.String())
	}
	if !ref.DataSetID().Equal(types.NewBigInt(11)) {
		t.Fatal("accessor returned an alias")
	}
}

func TestDataSetRefJSONRoundTripAndStrictValidation(t *testing.T) {
	ref, err := NewDataSetRef(types.NewBigInt(7), types.NewBigInt(11), types.BigInt{})
	if err != nil {
		t.Fatalf("NewDataSetRef: %v", err)
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded DataSetRef
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !decoded.Equal(ref) {
		t.Fatalf("decoded=%+v want %+v", decoded, ref)
	}
	if string(encoded) != `{"providerId":"7","dataSetId":"11","clientDataSetId":"0"}` {
		t.Fatalf("JSON=%s", encoded)
	}

	for name, tc := range map[string]struct {
		input       string
		wantInvalid bool
	}{
		"missing client ID": {input: `{"providerId":"7","dataSetId":"11"}`, wantInvalid: true},
		"null client ID":    {input: `{"providerId":"7","dataSetId":"11","clientDataSetId":null}`, wantInvalid: true},
		"unknown field":     {input: `{"providerId":"7","dataSetId":"11","clientDataSetId":"0","other":1}`, wantInvalid: true},
		"PascalCase":        {input: `{"ProviderID":"7","DataSetID":"11","ClientDataSetID":"0"}`, wantInvalid: true},
		"trailing value":    {input: `{"providerId":"7","dataSetId":"11","clientDataSetId":"0"} {}`},
	} {
		t.Run(name, func(t *testing.T) {
			before := decoded
			err := json.Unmarshal([]byte(tc.input), &decoded)
			if err == nil || tc.wantInvalid && !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("Unmarshal error=%v want rejection", err)
			}
			if !decoded.Equal(before) {
				t.Fatal("failed decode mutated receiver")
			}
		})
	}

	if _, err := json.Marshal(DataSetRef{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Marshal zero ref error=%v want ErrInvalidArgument", err)
	}
}
