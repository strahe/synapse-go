package payments

import (
	"context"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	sdktypes "github.com/strahe/synapse-go/types"
)

type permitKey struct {
	chainID sdktypes.ChainID
	token   common.Address
	owner   common.Address
}

// permitCoordinator serializes permit-consuming writes for one
// (chainID, token, owner) tuple. Entries stay for the Service lifetime to
// avoid deleting a semaphore while another goroutine is waiting on it.
type permitCoordinator struct {
	mu   sync.Mutex
	sems map[permitKey]chan struct{}
}

func newPermitCoordinator() *permitCoordinator {
	return &permitCoordinator{sems: map[permitKey]chan struct{}{}}
}

// acquire blocks until no other permit write for key owns the semaphore.
// The returned release function is idempotent.
func (c *permitCoordinator) acquire(ctx context.Context, key permitKey) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sem := c.semaphore(key)
	select {
	case sem <- struct{}{}:
		var once sync.Once
		return func() {
			once.Do(func() { <-sem })
		}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *permitCoordinator) semaphore(key permitKey) chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sems == nil {
		c.sems = map[permitKey]chan struct{}{}
	}
	sem, ok := c.sems[key]
	if !ok {
		sem = make(chan struct{}, 1)
		c.sems[key] = sem
	}
	return sem
}
