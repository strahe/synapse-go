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
// release exactly once after either broadcasting the transaction (so that
// subsequent Acquire calls observe the bumped pending count) or abandoning
// the attempt. Until release is called no other goroutine can acquire a
// nonce.
//
// This mirrors the TS SDK / viem behavior: every transaction round-trips the
// node for its nonce, with no client-side cache. The added serialization is
// the cost of guaranteeing monotonically-increasing nonces across concurrent
// goroutines.
type NonceManager struct {
	mu      sync.Mutex
	client  NonceProvider
	address common.Address
}

// NewNonceManager constructs a NonceManager for the given address.
func NewNonceManager(client NonceProvider, address common.Address) *NonceManager {
	return &NonceManager{client: client, address: address}
}

// Acquire locks the manager and returns the next pending nonce together with
// a release function. The caller MUST call release exactly once after either
// broadcasting the transaction or abandoning it. release is idempotent. On
// error, the returned release is a no-op.
//
// Acquire blocks until it can take the manager mutex; ctx only governs the
// PendingNonceAt RPC call. If ctx is cancelled while another goroutine holds
// the lock, this call will continue to wait for the lock — this matches the
// semantics of stdlib sync.Mutex.
func (m *NonceManager) Acquire(ctx context.Context) (nonce uint64, release func(), err error) {
	m.mu.Lock()
	locked := true
	defer func() {
		if locked {
			m.mu.Unlock()
		}
	}()

	n, err := m.client.PendingNonceAt(ctx, m.address)
	if err != nil {
		return 0, func() {}, fmt.Errorf("txutil.NonceManager.Acquire: %w", err)
	}

	var once sync.Once
	locked = false
	return n, func() { once.Do(m.mu.Unlock) }, nil
}
