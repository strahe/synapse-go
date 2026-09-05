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

var (
	createDataSetPermission         = mustPermission("25ebf20299107c91b4624d5bac3a16d32cabf0db23b450ee09ab7732983b1dc9")
	addPiecesPermission             = mustPermission("954bdc254591a7eab1b73f03842464d9283a08352772737094d710a4428fd183")
	schedulePieceRemovalsPermission = mustPermission("5415701e313bb627e755b16924727217bb356574fe20e7061442c200b0822b22")
	terminateServicePermission      = mustPermission("522bd88a11de1cdc6574394dde7a21ae488ff13e16e7408d0ea721dd8479dffc")
)

// CreateDataSetPermission returns the permission to create new datasets.
//
// encodeType: "CreateDataSet(uint256 clientDataSetId,address payee,MetadataEntry[] metadata)MetadataEntry(string key,string value)"
func CreateDataSetPermission() Permission { return createDataSetPermission }

// AddPiecesPermission returns the permission to add pieces to a dataset.
//
// encodeType: "AddPieces(uint256 clientDataSetId,uint256 nonce,Cid[] pieceData,PieceMetadata[] pieceMetadata)Cid(bytes data)MetadataEntry(string key,string value)PieceMetadata(uint256 pieceIndex,MetadataEntry[] metadata)"
func AddPiecesPermission() Permission { return addPiecesPermission }

// SchedulePieceRemovalsPermission returns the permission to schedule piece
// removals from a dataset.
//
// encodeType: "SchedulePieceRemovals(uint256 clientDataSetId,uint256[] pieceIds)"
func SchedulePieceRemovalsPermission() Permission { return schedulePieceRemovalsPermission }

// TerminateServicePermission returns the permission to terminate a service.
//
// encodeType: "TerminateService(uint256 dataSetId)"
func TerminateServicePermission() Permission { return terminateServicePermission }

// DefaultFWSSPermissions returns an independent slice of the four standard
// FWSS permissions, ordered as create dataset, add pieces, schedule removals,
// and terminate service. Modifying it does not change future defaults.
func DefaultFWSSPermissions() []Permission {
	return []Permission{
		CreateDataSetPermission(),
		AddPiecesPermission(),
		SchedulePieceRemovalsPermission(),
		TerminateServicePermission(),
	}
}

// Expirations maps each Permission to its expiry timestamp (Unix epoch
// seconds). A zero value means the permission is not authorised.
type Expirations map[Permission]uint64

// DefaultEmptyExpirations returns an Expirations map containing the four
// default FWSS permissions, each with a zero (expired) expiry.
func DefaultEmptyExpirations() Expirations {
	return Expirations{
		CreateDataSetPermission():         0,
		AddPiecesPermission():             0,
		SchedulePieceRemovalsPermission(): 0,
		TerminateServicePermission():      0,
	}
}

// PermissionName returns the EIP-712 primary type name for a standard FWSS
// permission. Unknown permissions return false and should be rendered by hash.
func PermissionName(p Permission) (string, bool) {
	switch p {
	case CreateDataSetPermission():
		return "CreateDataSet", true
	case AddPiecesPermission():
		return "AddPieces", true
	case SchedulePieceRemovalsPermission():
		return "SchedulePieceRemovals", true
	case TerminateServicePermission():
		return "TerminateService", true
	default:
		return "", false
	}
}

func permissionLabel(p Permission) string {
	if name, ok := PermissionName(p); ok {
		return fmt.Sprintf("%s (%s)", name, p.Hex())
	}
	return p.Hex()
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
		panic(fmt.Sprintf("invalid permission hex: %s", hexStr)) //nolint:forbidigo // permission hashes are decoded at init from fixed hex strings
	}
	var p Permission
	copy(p[:], b)
	return p
}
