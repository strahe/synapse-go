package sessionkey

import (
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// LoginOptions configures a Login or LoginAndFund call. All fields are
// optional; zero values cause sensible defaults to be applied.
type LoginOptions struct {
	// Permissions to authorise. Defaults to DefaultFWSSPermissions.
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
	// Permissions to revoke. Defaults to DefaultFWSSPermissions.
	Permissions []Permission
	// Origin is an application identifier stored on-chain. Defaults to
	// "synapse".
	Origin string
}

// WriteResult is returned by every state-changing call (Login, Revoke, etc.).
//
// Hash is populated as soon as the transaction is broadcast. Receipt is
// populated only when the call was made with WithWait and the transaction
// was mined before the timeout elapsed.
type WriteResult struct {
	Hash    common.Hash
	Receipt *types.Receipt
}

// WriteOption tunes the behaviour of a single state-changing call.
type WriteOption func(*writeConfig)

type writeConfig struct {
	waitTimeout   time.Duration
	confirmations uint64
}

func newWriteConfig(opts []WriteOption) writeConfig {
	cfg := writeConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	return cfg
}

// WithWait makes the call block until the transaction is mined, or the
// given timeout elapses. Use zero or a negative duration to return as soon
// as the tx is broadcast (the default).
func WithWait(timeout time.Duration) WriteOption {
	return func(c *writeConfig) { c.waitTimeout = timeout }
}

// WithConfirmations requires N block confirmations in addition to WithWait.
// Has no effect unless WithWait is also passed with a positive timeout.
func WithConfirmations(n uint64) WriteOption {
	return func(c *writeConfig) { c.confirmations = n }
}

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
