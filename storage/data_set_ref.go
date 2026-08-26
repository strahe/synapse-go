package storage

import (
	"encoding/json"
	"fmt"

	"github.com/strahe/synapse-go/types"
)

// NewDataSetRef validates and constructs an immutable data-set target.
// ClientDataSetID may be explicitly zero.
func NewDataSetRef(providerID, dataSetID, clientDataSetID types.BigInt) (DataSetRef, error) {
	providerID = copyBigInt(providerID)
	dataSetID = copyBigInt(dataSetID)
	clientDataSetID = copyBigInt(clientDataSetID)
	if providerID.IsZero() {
		return DataSetRef{}, fmt.Errorf("storage.NewDataSetRef: %w: zero providerID", ErrInvalidArgument)
	}
	if dataSetID.IsZero() {
		return DataSetRef{}, fmt.Errorf("storage.NewDataSetRef: %w: zero dataSetID", ErrInvalidArgument)
	}
	return DataSetRef{
		providerID:      providerID,
		dataSetID:       dataSetID,
		clientDataSetID: clientDataSetID,
	}, nil
}

// ProviderID returns an independent copy of the owning provider ID.
func (r DataSetRef) ProviderID() types.BigInt { return copyBigInt(r.providerID) }

// DataSetID returns an independent copy of the on-chain data-set ID.
func (r DataSetRef) DataSetID() types.BigInt { return copyBigInt(r.dataSetID) }

// ClientDataSetID returns an independent copy of the client-chosen data-set ID.
func (r DataSetRef) ClientDataSetID() types.BigInt { return copyBigInt(r.clientDataSetID) }

// Equal reports whether both refs identify the same provider and data set.
func (r DataSetRef) Equal(other DataSetRef) bool {
	return r.providerID.Equal(other.providerID) &&
		r.dataSetID.Equal(other.dataSetID) &&
		r.clientDataSetID.Equal(other.clientDataSetID)
}

func (r DataSetRef) valid() bool {
	return !r.providerID.IsZero() && !r.dataSetID.IsZero()
}

// MarshalJSON encodes all three identity fields using lowerCamel names.
// Invalid zero-value refs are rejected instead of being persisted as usable
// targets.
func (r DataSetRef) MarshalJSON() ([]byte, error) {
	if !r.valid() {
		return nil, fmt.Errorf("storage.DataSetRef.MarshalJSON: %w: invalid data-set ref", ErrInvalidArgument)
	}
	type wireRef struct {
		ProviderID      types.BigInt `json:"providerId"`
		DataSetID       types.BigInt `json:"dataSetId"`
		ClientDataSetID types.BigInt `json:"clientDataSetId"`
	}
	return json.Marshal(wireRef{
		ProviderID:      r.ProviderID(),
		DataSetID:       r.DataSetID(),
		ClientDataSetID: r.ClientDataSetID(),
	})
}

// UnmarshalJSON requires the exact lowerCamel names and all three non-null
// identity fields.
func (r *DataSetRef) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("storage.DataSetRef.UnmarshalJSON: %w: nil receiver", ErrInvalidArgument)
	}
	type wireRef struct {
		ProviderID      *types.BigInt `json:"providerId"`
		DataSetID       *types.BigInt `json:"dataSetId"`
		ClientDataSetID *types.BigInt `json:"clientDataSetId"`
	}
	var wire wireRef
	if err := decodeLifecycleJSON(data, "storage.DataSetRef.UnmarshalJSON", &wire, []lifecycleJSONField{
		{name: "providerId"},
		{name: "dataSetId"},
		{name: "clientDataSetId"},
	}); err != nil {
		return err
	}
	if wire.ProviderID == nil || wire.DataSetID == nil || wire.ClientDataSetID == nil {
		return fmt.Errorf("storage.DataSetRef.UnmarshalJSON: %w: providerId, dataSetId, and clientDataSetId are required", ErrInvalidArgument)
	}
	parsed, err := NewDataSetRef(*wire.ProviderID, *wire.DataSetID, *wire.ClientDataSetID)
	if err != nil {
		return fmt.Errorf("storage.DataSetRef.UnmarshalJSON: %w", err)
	}
	*r = parsed
	return nil
}
