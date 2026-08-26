package storage

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type lifecycleJSONField struct {
	name     string
	nullable bool
}

func decodeLifecycleJSON(data []byte, op string, out any, fields []lifecycleJSONField) error {
	expected := make(map[string]bool, len(fields))
	for _, field := range fields {
		expected[field.name] = field.nullable
	}

	object := make(map[string]json.RawMessage, len(fields))
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if delimiter, ok := start.(json.Delim); !ok || delimiter != '{' {
		return fmt.Errorf("%s: %w: expected JSON object", op, ErrInvalidArgument)
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
		}
		name, ok := key.(string)
		if !ok {
			return fmt.Errorf("%s: %w: expected object field name", op, ErrInvalidArgument)
		}
		if _, ok := expected[name]; !ok {
			return fmt.Errorf("%s: %w: unknown field %q", op, ErrInvalidArgument, name)
		}
		if _, exists := object[name]; exists {
			return fmt.Errorf("%s: %w: duplicate field %q", op, ErrInvalidArgument, name)
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("%s: %w: field %q: %w", op, ErrInvalidArgument, name, err)
		}
		object[name] = value
	}
	end, err := decoder.Token()
	if err != nil {
		return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	if delimiter, ok := end.(json.Delim); !ok || delimiter != '}' {
		return fmt.Errorf("%s: %w: expected end of JSON object", op, ErrInvalidArgument)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("%s: %w: multiple JSON values", op, ErrInvalidArgument)
		}
		return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}

	for _, field := range fields {
		value, ok := object[field.name]
		if !ok {
			return fmt.Errorf("%s: %w: missing field %q", op, ErrInvalidArgument, field.name)
		}
		if !field.nullable && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("%s: %w: field %q must not be null", op, ErrInvalidArgument, field.name)
		}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("%s: %w: %w", op, ErrInvalidArgument, err)
	}
	return nil
}

func (i *ContextIdentity) UnmarshalJSON(data []byte) error {
	if i == nil {
		return fmt.Errorf("storage.ContextIdentity.UnmarshalJSON: %w: nil receiver", ErrInvalidArgument)
	}
	type wireIdentity ContextIdentity
	var wire wireIdentity
	if err := decodeLifecycleJSON(data, "storage.ContextIdentity.UnmarshalJSON", &wire, []lifecycleJSONField{
		{name: "payer"},
		{name: "chainId"},
		{name: "recordKeeper"},
	}); err != nil {
		return err
	}
	*i = ContextIdentity(wire)
	return nil
}

func (s *CommitSubmission) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("storage.CommitSubmission.UnmarshalJSON: %w: nil receiver", ErrInvalidArgument)
	}
	type wireSubmission CommitSubmission
	var wire wireSubmission
	if err := decodeLifecycleJSON(data, "storage.CommitSubmission.UnmarshalJSON", &wire, []lifecycleJSONField{
		{name: "kind"},
		{name: "transactionId"},
		{name: "statusUrl"},
		{name: "providerId"},
		{name: "identity"},
		{name: "dataSet", nullable: true},
		{name: "clientDataSetId", nullable: true},
		{name: "pieceCids"},
	}); err != nil {
		return err
	}
	*s = copyCommitSubmission(CommitSubmission(wire))
	return nil
}

func (s *CommitStatus) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("storage.CommitStatus.UnmarshalJSON: %w: nil receiver", ErrInvalidArgument)
	}
	type wireStatus CommitStatus
	var wire wireStatus
	if err := decodeLifecycleJSON(data, "storage.CommitStatus.UnmarshalJSON", &wire, []lifecycleJSONField{
		{name: "kind"},
		{name: "state"},
		{name: "transactionId"},
		{name: "confirmedTransactionId"},
		{name: "dataSet", nullable: true},
		{name: "pieceIds", nullable: true},
	}); err != nil {
		return err
	}
	*s = copyCommitStatus(CommitStatus(wire))
	return nil
}

func (r *CommitResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("storage.CommitResult.UnmarshalJSON: %w: nil receiver", ErrInvalidArgument)
	}
	type wireResult CommitResult
	var wire wireResult
	if err := decodeLifecycleJSON(data, "storage.CommitResult.UnmarshalJSON", &wire, []lifecycleJSONField{
		{name: "transactionId"},
		{name: "confirmedTransactionId"},
		{name: "dataSet"},
		{name: "pieceIds"},
		{name: "isNewDataSet"},
	}); err != nil {
		return err
	}
	*r = CommitResult(wire)
	r.DataSet = copyDataSetRef(r.DataSet)
	r.PieceIDs = copyBigInts(r.PieceIDs)
	return nil
}

func (s *CreateDataSetSubmission) UnmarshalJSON(data []byte) error {
	if s == nil {
		return fmt.Errorf("storage.CreateDataSetSubmission.UnmarshalJSON: %w: nil receiver", ErrInvalidArgument)
	}
	type wireSubmission CreateDataSetSubmission
	var wire wireSubmission
	if err := decodeLifecycleJSON(data, "storage.CreateDataSetSubmission.UnmarshalJSON", &wire, []lifecycleJSONField{
		{name: "providerId"},
		{name: "transactionId"},
		{name: "statusUrl"},
		{name: "clientDataSetId"},
	}); err != nil {
		return err
	}
	*s = CreateDataSetSubmission(wire)
	s.ProviderID = copyBigInt(s.ProviderID)
	s.ClientDataSetID = copyBigIntPtr(s.ClientDataSetID)
	return nil
}

func (r *CreateDataSetResult) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("storage.CreateDataSetResult.UnmarshalJSON: %w: nil receiver", ErrInvalidArgument)
	}
	type wireResult CreateDataSetResult
	var wire wireResult
	if err := decodeLifecycleJSON(data, "storage.CreateDataSetResult.UnmarshalJSON", &wire, []lifecycleJSONField{
		{name: "transactionId"},
		{name: "confirmedTransactionId"},
		{name: "dataSet"},
	}); err != nil {
		return err
	}
	*r = CreateDataSetResult(wire)
	r.DataSet = copyDataSetRef(r.DataSet)
	return nil
}
