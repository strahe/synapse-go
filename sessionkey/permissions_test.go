package sessionkey

import (
	"maps"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// encodeTypeStrings are the full EIP-712 types used to derive standard permissions.
var encodeTypeStrings = map[string]string{
	"CreateDataSet":         "CreateDataSet(uint256 clientDataSetId,address payee,MetadataEntry[] metadata)MetadataEntry(string key,string value)",
	"AddPieces":             "AddPieces(uint256 clientDataSetId,uint256 nonce,Cid[] pieceData,PieceMetadata[] pieceMetadata)Cid(bytes data)MetadataEntry(string key,string value)PieceMetadata(uint256 pieceIndex,MetadataEntry[] metadata)",
	"SchedulePieceRemovals": "SchedulePieceRemovals(uint256 clientDataSetId,uint256[] pieceIds)",
	"TerminateService":      "TerminateService(uint256 dataSetId)",
}

func expectedFWSSPermissions() []Permission {
	return []Permission{
		Permission(common.HexToHash("0x25ebf20299107c91b4624d5bac3a16d32cabf0db23b450ee09ab7732983b1dc9")),
		Permission(common.HexToHash("0x954bdc254591a7eab1b73f03842464d9283a08352772737094d710a4428fd183")),
		Permission(common.HexToHash("0x5415701e313bb627e755b16924727217bb356574fe20e7061442c200b0822b22")),
		Permission(common.HexToHash("0x522bd88a11de1cdc6574394dde7a21ae488ff13e16e7408d0ea721dd8479dffc")),
	}
}

func TestPermissionHashes(t *testing.T) {
	tests := []struct {
		name          string
		encType       string
		getPermission func() Permission
		wantHex       string
	}{
		{
			name:          "CreateDataSet",
			encType:       encodeTypeStrings["CreateDataSet"],
			getPermission: CreateDataSetPermission,
			wantHex:       "0x25ebf20299107c91b4624d5bac3a16d32cabf0db23b450ee09ab7732983b1dc9",
		},
		{
			name:          "AddPieces",
			encType:       encodeTypeStrings["AddPieces"],
			getPermission: AddPiecesPermission,
			wantHex:       "0x954bdc254591a7eab1b73f03842464d9283a08352772737094d710a4428fd183",
		},
		{
			name:          "SchedulePieceRemovals",
			encType:       encodeTypeStrings["SchedulePieceRemovals"],
			getPermission: SchedulePieceRemovalsPermission,
			wantHex:       "0x5415701e313bb627e755b16924727217bb356574fe20e7061442c200b0822b22",
		},
		{
			name:          "TerminateService",
			encType:       encodeTypeStrings["TerminateService"],
			getPermission: TerminateServicePermission,
			wantHex:       "0x522bd88a11de1cdc6574394dde7a21ae488ff13e16e7408d0ea721dd8479dffc",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PermissionFromEncodeType(tt.encType)
			if want := tt.getPermission(); got != want {
				t.Errorf("PermissionFromEncodeType(%q)\n  got  %s\n  want %s", tt.name, got.Hex(), want.Hex())
			}
			if got.Hex() != tt.wantHex {
				t.Errorf("hex mismatch for %s:\n  got  %s\n  want %s", tt.name, got.Hex(), tt.wantHex)
			}
			permission := tt.getPermission()
			permission[0] ^= 0xff
			if fresh := tt.getPermission(); fresh.Hex() != tt.wantHex {
				t.Errorf("permission changed after modifying a returned value: got %s, want %s", fresh.Hex(), tt.wantHex)
			}
			if name, ok := PermissionName(got); !ok || name != tt.name {
				t.Errorf("PermissionName(%s) = (%q, %v), want (%q, true)", got.Hex(), name, ok, tt.name)
			}
		})
	}
}

func TestDefaultFWSSPermissions(t *testing.T) {
	expected := expectedFWSSPermissions()
	permissions := DefaultFWSSPermissions()
	if !slices.Equal(permissions, expected) {
		t.Fatalf("default permissions = %v, want %v", permissions, expected)
	}
	clear(permissions)
	if fresh := DefaultFWSSPermissions(); !slices.Equal(fresh, expected) {
		t.Fatalf("default permissions changed after clearing a returned slice: got %v, want %v", fresh, expected)
	}
}

func TestDefaultEmptyExpirations(t *testing.T) {
	permissions := expectedFWSSPermissions()
	expected := make(Expirations, len(permissions))
	for _, p := range permissions {
		expected[p] = 0
	}
	e := DefaultEmptyExpirations()
	if !maps.Equal(e, expected) {
		t.Fatalf("empty expirations = %v, want %v", e, expected)
	}
	e[permissions[0]] = 42
	delete(e, permissions[1])
	if fresh := DefaultEmptyExpirations(); !maps.Equal(fresh, expected) {
		t.Fatalf("empty expirations changed after modifying a returned map: got %v, want %v", fresh, expected)
	}
}

func TestPermissionString(t *testing.T) {
	p := CreateDataSetPermission()
	s := p.String()
	if s != p.Hex() {
		t.Errorf("String() = %q, Hex() = %q, want equal", s, p.Hex())
	}
}

func TestPermissionName(t *testing.T) {
	tests := []struct {
		permission Permission
		want       string
	}{
		{CreateDataSetPermission(), "CreateDataSet"},
		{AddPiecesPermission(), "AddPieces"},
		{SchedulePieceRemovalsPermission(), "SchedulePieceRemovals"},
		{TerminateServicePermission(), "TerminateService"},
	}
	for _, tt := range tests {
		got, ok := PermissionName(tt.permission)
		if !ok {
			t.Fatalf("PermissionName(%s) ok=false", tt.permission.Hex())
		}
		if got != tt.want {
			t.Fatalf("PermissionName(%s)=%q want %q", tt.permission.Hex(), got, tt.want)
		}
	}

	var unknown Permission
	unknown[0] = 1
	if got, ok := PermissionName(unknown); ok || got != "" {
		t.Fatalf("PermissionName(unknown)=(%q, %v), want empty false", got, ok)
	}
}
