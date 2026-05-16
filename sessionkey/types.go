package sessionkey

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	sdktypes "github.com/strahe/synapse-go/types"
)

// LoginOptions configures a Login or LoginAndFund call. All fields are
// optional; nil or zero values cause sensible defaults to be applied.
type LoginOptions struct {
	// Permissions to authorise. Nil defaults to DefaultFWSSPermissions;
	// an explicit empty slice authorises no permissions.
	Permissions []Permission
	// ExpiresAt is the Unix timestamp (seconds) when the session key
	// authorisation expires. Zero defaults to time.Now().Unix() + 3600
	// (one hour from now).
	ExpiresAt uint64
	// Origin is an application identifier stored on-chain. Defaults to
	// "synapse".
	Origin string
}

// RevokeOptions configures a Revoke call.
type RevokeOptions struct {
	// Permissions to revoke. Nil defaults to DefaultFWSSPermissions;
	// an explicit empty slice revokes no permissions.
	Permissions []Permission
	// Origin is an application identifier stored on-chain. Defaults to
	// "synapse".
	Origin string
}

// WriteResult is kept as an alias for backwards compatibility.
type WriteResult = sdktypes.WriteResult

// SessionKey represents a session key with its current authorization state.
// It does not perform event watching — call GetExpirations to refresh.
type SessionKey struct {
	// Address is the session key's own address.
	Address common.Address
	// RootAddress is the root account that authorized this session key.
	RootAddress common.Address
	// KeyType identifies the key algorithm (e.g. "secp256k1").
	KeyType string
	// Expirations maps each permission to its on-chain expiry.
	Expirations Expirations
}

// HasPermission returns true if the given permission is currently active
// (i.e., its expiry is in the future).
func (sk *SessionKey) HasPermission(p Permission) bool {
	return sk.HasPermissionAt(time.Now(), p)
}

// HasPermissions returns true if all given permissions are currently active.
func (sk *SessionKey) HasPermissions(ps []Permission) bool {
	now := time.Now()
	for _, p := range ps {
		if !sk.HasPermissionAt(now, p) {
			return false
		}
	}
	return true
}

// HasPermissionAt returns true if the given permission is active at the
// specified point in time. This variant enables deterministic testing.
func (sk *SessionKey) HasPermissionAt(t time.Time, p Permission) bool {
	if sk == nil || sk.Expirations == nil {
		return false
	}
	exp, ok := sk.Expirations[p]
	if !ok {
		return false
	}
	return exp > uint64(t.Unix())
}
