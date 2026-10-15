package txutil

import (
	"context"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"
)

// NonceProvider abstracts the subset of an Ethereum client used by NonceManager.
// It is satisfied by *ethclient.Client and can be faked in tests.
type NonceProvider interface {
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
}

// NonceManager serializes nonce acquisition for an EOA. Each Acquire call
// locks the manager, fetches PendingNonceAt(pending) from the network, and
// returns both the nonce and a release function. The caller MUST invoke
// release after either broadcasting the transaction (so that subsequent
// Acquire calls observe the bumped pending count) or abandoning the attempt.
// release is idempotent, so callers may invoke it more than once. Until
// release is called no other goroutine can acquire a nonce.
//
// Each transaction round-trips to the node for its nonce, with no client-side
// cache. Acquisition and submission are serialized; the node determines the
// pending nonce returned to each caller.
type NonceManager struct {
	gate    chan struct{}
	client  NonceProvider
	address common.Address
}

// NewNonceManager constructs a NonceManager for the given address.
func NewNonceManager(client NonceProvider, address common.Address) *NonceManager {
	return &NonceManager{gate: make(chan struct{}, 1), client: client, address: address}
}

// Acquire locks the manager and returns the next pending nonce together with
// a non-nil release function. The caller MUST call release after either
// broadcasting the transaction or abandoning it. release is idempotent, so
// callers may invoke it more than once. On error, the returned release is a
// no-op.
//
// ctx governs both waiting for the manager and the PendingNonceAt RPC call.
func (m *NonceManager) Acquire(ctx context.Context) (nonce uint64, release func(), err error) {
	if err := ctx.Err(); err != nil {
		return 0, func() {}, fmt.Errorf("txutil.NonceManager.Acquire: %w", err)
	}
	select {
	case m.gate <- struct{}{}:
	case <-ctx.Done():
		return 0, func() {}, fmt.Errorf("txutil.NonceManager.Acquire: %w", ctx.Err())
	}
	locked := true
	defer func() {
		if locked {
			<-m.gate
		}
	}()
	if err := ctx.Err(); err != nil {
		return 0, func() {}, fmt.Errorf("txutil.NonceManager.Acquire: %w", err)
	}

	n, err := m.client.PendingNonceAt(ctx, m.address)
	if err := ctx.Err(); err != nil {
		return 0, func() {}, fmt.Errorf("txutil.NonceManager.Acquire: %w", err)
	}
	if err != nil {
		return 0, func() {}, fmt.Errorf("txutil.NonceManager.Acquire: %w", err)
	}

	var once sync.Once
	locked = false
	return n, func() { once.Do(func() { <-m.gate }) }, nil
}
