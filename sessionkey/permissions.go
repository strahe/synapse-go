package sessionkey

import (
	"encoding/hex"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
)

// Permission is a 32-byte keccak256 hash that identifies a specific
// operation that a session key may perform on behalf of the root account.
//
// Each permission corresponds to the keccak256 of the EIP-712 encodeType
// for that operation's primary type (including referenced struct types,
// sorted alphabetically).
type Permission [32]byte

// Hex returns the permission hash as a 0x-prefixed hex string.
func (p Permission) Hex() string {
	return "0x" + hex.EncodeToString(p[:])
}

// String implements fmt.Stringer and returns the same as Hex.
func (p Permission) String() string { return p.Hex() }

// CreateDataSetPermission authorises session keys to create new datasets.
//
// encodeType: "CreateDataSet(uint256 clientDataSetId,address payee,MetadataEntry[] metadata)MetadataEntry(string key,string value)"
var CreateDataSetPermission = mustPermission("25ebf20299107c91b4624d5bac3a16d32cabf0db23b450ee09ab7732983b1dc9")

// AddPiecesPermission authorises session keys to add pieces to a dataset.
//
// encodeType: "AddPieces(uint256 clientDataSetId,uint256 nonce,Cid[] pieceData,PieceMetadata[] pieceMetadata)Cid(bytes data)MetadataEntry(string key,string value)PieceMetadata(uint256 pieceIndex,MetadataEntry[] metadata)"
var AddPiecesPermission = mustPermission("954bdc254591a7eab1b73f03842464d9283a08352772737094d710a4428fd183")

// SchedulePieceRemovalsPermission authorises session keys to schedule
// piece removals from a dataset.
//
// encodeType: "SchedulePieceRemovals(uint256 clientDataSetId,uint256[] pieceIds)"
var SchedulePieceRemovalsPermission = mustPermission("5415701e313bb627e755b16924727217bb356574fe20e7061442c200b0822b22")

// DeleteDataSetPermission authorises session keys to delete a dataset.
//
// encodeType: "DeleteDataSet(uint256 clientDataSetId)"
var DeleteDataSetPermission = mustPermission("b5d6b3fc97881f05e96958136ac09d7e0bc7cbf17ea92fce7c431d88132d2b58")

// DefaultFWSSPermissions contains all four standard FWSS permissions.
// This is the default set used by Login when no explicit permissions
// are provided.
var DefaultFWSSPermissions = []Permission{
	CreateDataSetPermission,
	AddPiecesPermission,
	SchedulePieceRemovalsPermission,
	DeleteDataSetPermission,
}

// Expirations maps each Permission to its expiry timestamp (Unix epoch
// seconds). A zero value means the permission is not authorised.
type Expirations map[Permission]uint64

// DefaultEmptyExpirations returns an Expirations map containing the four
// default FWSS permissions, each with a zero (expired) expiry.
func DefaultEmptyExpirations() Expirations {
	return Expirations{
		CreateDataSetPermission:         0,
		AddPiecesPermission:             0,
		SchedulePieceRemovalsPermission: 0,
		DeleteDataSetPermission:         0,
	}
}

// PermissionFromEncodeType computes a Permission from the full EIP-712
// encodeType string by taking its keccak256 hash.
func PermissionFromEncodeType(encodeType string) Permission {
	var p Permission
	copy(p[:], crypto.Keccak256([]byte(encodeType)))
	return p
}

func mustPermission(hexStr string) Permission {
	b, err := hex.DecodeString(hexStr)
	if err != nil || len(b) != 32 {
		panic(fmt.Sprintf("invalid permission hex: %s", hexStr)) //nolint:forbidigo // package-level Permission constants are decoded at init from compile-time-constant strings
	}
	var p Permission
	copy(p[:], b)
	return p
}
