package payments

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestZeroValueReturnsErrUninitialized verifies that calling methods on a
// zero-value Service returns ErrUninitialized instead of panicking.
func TestZeroValueReturnsErrUninitialized(t *testing.T) {
	var s Service
	ctx := context.Background()
	if _, err := s.Balance(ctx, common.Address{}, common.Address{}); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("Balance: want ErrUninitialized, got %v", err)
	}
}
