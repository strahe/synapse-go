package sessionkey

import (
	"testing"
)

// encodeType strings from the EIP-712 type definitions in
// synapse-sdk/packages/synapse-core/src/typed-data/type-definitions.ts.
//
// These are the canonical inputs to keccak256 that produce each permission hash.
// The test verifies that our hardcoded constants match the computed values.
var encodeTypeStrings = map[string]string{
	"CreateDataSet":         "CreateDataSet(uint256 clientDataSetId,address payee,MetadataEntry[] metadata)MetadataEntry(string key,string value)",
	"AddPieces":             "AddPieces(uint256 clientDataSetId,uint256 nonce,Cid[] pieceData,PieceMetadata[] pieceMetadata)Cid(bytes data)MetadataEntry(string key,string value)PieceMetadata(uint256 pieceIndex,MetadataEntry[] metadata)",
	"SchedulePieceRemovals": "SchedulePieceRemovals(uint256 clientDataSetId,uint256[] pieceIds)",
	"DeleteDataSet":         "DeleteDataSet(uint256 clientDataSetId)",
}

func TestPermissionHashes(t *testing.T) {
	tests := []struct {
		name     string
		encType  string
		wantPerm Permission
		wantHex  string
	}{
		{
			name:     "CreateDataSet",
			encType:  encodeTypeStrings["CreateDataSet"],
			wantPerm: CreateDataSetPermission,
			wantHex:  "0x25ebf20299107c91b4624d5bac3a16d32cabf0db23b450ee09ab7732983b1dc9",
		},
		{
			name:     "AddPieces",
			encType:  encodeTypeStrings["AddPieces"],
			wantPerm: AddPiecesPermission,
			wantHex:  "0x954bdc254591a7eab1b73f03842464d9283a08352772737094d710a4428fd183",
		},
		{
			name:     "SchedulePieceRemovals",
			encType:  encodeTypeStrings["SchedulePieceRemovals"],
			wantPerm: SchedulePieceRemovalsPermission,
			wantHex:  "0x5415701e313bb627e755b16924727217bb356574fe20e7061442c200b0822b22",
		},
		{
			name:     "DeleteDataSet",
			encType:  encodeTypeStrings["DeleteDataSet"],
			wantPerm: DeleteDataSetPermission,
			wantHex:  "0xb5d6b3fc97881f05e96958136ac09d7e0bc7cbf17ea92fce7c431d88132d2b58",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PermissionFromEncodeType(tt.encType)
			if got != tt.wantPerm {
				t.Errorf("PermissionFromEncodeType(%q)\n  got  %s\n  want %s", tt.name, got.Hex(), tt.wantPerm.Hex())
			}
			if got.Hex() != tt.wantHex {
				t.Errorf("hex mismatch for %s:\n  got  %s\n  want %s", tt.name, got.Hex(), tt.wantHex)
			}
		})
	}
}

func TestDefaultFWSSPermissions(t *testing.T) {
	if len(DefaultFWSSPermissions) != 4 {
		t.Fatalf("expected 4 default permissions, got %d", len(DefaultFWSSPermissions))
	}
	expected := []Permission{
		CreateDataSetPermission,
		AddPiecesPermission,
		SchedulePieceRemovalsPermission,
		DeleteDataSetPermission,
	}
	for i, p := range DefaultFWSSPermissions {
		if p != expected[i] {
			t.Errorf("DefaultFWSSPermissions[%d] = %s, want %s", i, p.Hex(), expected[i].Hex())
		}
	}
}

func TestDefaultEmptyExpirations(t *testing.T) {
	e := DefaultEmptyExpirations()
	if len(e) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(e))
	}
	for _, p := range DefaultFWSSPermissions {
		if v, ok := e[p]; !ok {
			t.Errorf("missing permission %s", p.Hex())
		} else if v != 0 {
			t.Errorf("expected zero expiry for %s, got %d", p.Hex(), v)
		}
	}
}

func TestPermissionString(t *testing.T) {
	p := CreateDataSetPermission
	s := p.String()
	if s != p.Hex() {
		t.Errorf("String() = %q, Hex() = %q, want equal", s, p.Hex())
	}
}
