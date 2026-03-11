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

// NonceManager tracks nonces for an EOA to allow many goroutines to build
// transactions concurrently without RPC round-trips on every call. It is
// safe for concurrent use.
//
// Usage contract:
//   - Get returns a fresh nonce and reserves it.
//   - Once the transaction is confirmed (mined, regardless of receipt status)
//     the caller should invoke MarkConfirmed.
//   - If the transaction cannot be broadcast (e.g., signing failure, gas
//     estimation failure) the caller should invoke MarkFailed, which releases
//     the reservation AND invalidates the cached counter so the next Get
//     refreshes from the network.
//   - Reset forces an immediate refresh and clears all reservations.
//
// The manager never talks to the network outside Get/Reset, so callers are
// responsible for receipt waiting.
type NonceManager struct {
	client  NonceProvider
	address common.Address

	mu       sync.Mutex
	current  uint64          // next nonce to hand out when valid == true
	valid    bool            // whether current is primed from the network
	reserved map[uint64]bool // in-flight nonces
	initCh   chan struct{}
	initErr  error
	version  uint64
}

// NewNonceManager constructs a NonceManager for the given address.
func NewNonceManager(client NonceProvider, address common.Address) *NonceManager {
	return &NonceManager{
		client:   client,
		address:  address,
		reserved: make(map[uint64]bool),
	}
}

// Get returns the next nonce and reserves it. If the internal counter is not
// primed it fetches PendingNonceAt once.
func (m *NonceManager) Get(ctx context.Context) (uint64, error) {
	for {
		m.mu.Lock()
		if m.valid {
			n := m.current
			m.reserved[n] = true
			m.current++
			m.mu.Unlock()
			return n, nil
		}
		if m.initErr != nil && m.initCh == nil {
			err := m.initErr
			m.initErr = nil
			m.mu.Unlock()
			return 0, fmt.Errorf("txutil.NonceManager.Get: %w", err)
		}
		if ch := m.initCh; ch != nil {
			m.mu.Unlock()
			select {
			case <-ch:
				continue
			case <-ctx.Done():
				return 0, fmt.Errorf("txutil.NonceManager.Get: %w", ctx.Err())
			}
		}
		ch := make(chan struct{})
		version := m.version
		m.initCh = ch
		m.mu.Unlock()

		n, err := m.client.PendingNonceAt(ctx, m.address)

		m.mu.Lock()
		if m.initCh == ch {
			if err != nil {
				m.initErr = err
			} else if m.version == version {
				m.current = m.nextNonceLocked(n)
				m.valid = true
				m.initErr = nil
			}
			m.initCh = nil
			close(ch)
		}
		m.mu.Unlock()
	}
}

// MarkConfirmed releases a reservation after the transaction has been mined.
// Safe to call multiple times with the same nonce.
func (m *NonceManager) MarkConfirmed(nonce uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.reserved, nonce)
}

// MarkFailed releases a reservation for a transaction that never reached the
// network (e.g., signing or gas estimation failure). It also invalidates the
// cached counter so the next Get re-reads PendingNonceAt, preventing nonce
// leaks from out-of-order reservations.
//
// Do NOT call this for transactions that were broadcast successfully but
// failed later — those still occupy a nonce on-chain.
func (m *NonceManager) MarkFailed(nonce uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.reserved, nonce)
	m.valid = false
	m.version++
	m.cancelInitLocked()
}

// Reset fetches a fresh nonce from the network and clears all reservations.
func (m *NonceManager) Reset(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.version++
	m.cancelInitLocked()
	n, err := m.client.PendingNonceAt(ctx, m.address)
	if err != nil {
		return fmt.Errorf("txutil.NonceManager.Reset: %w", err)
	}
	m.current = n
	m.valid = true
	m.reserved = make(map[uint64]bool)
	return nil
}

// PendingCount returns the number of currently reserved nonces.
func (m *NonceManager) PendingCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.reserved)
}

func (m *NonceManager) nextNonceLocked(network uint64) uint64 {
	next := network
	for reserved := range m.reserved {
		if reserved >= next {
			next = reserved + 1
		}
	}
	return next
}

func (m *NonceManager) cancelInitLocked() {
	if m.initCh == nil {
		return
	}
	close(m.initCh)
	m.initCh = nil
	m.initErr = nil
}
