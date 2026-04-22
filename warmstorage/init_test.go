package warmstorage

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/types"
)

func TestZeroValueReturnsErrUninitialized(t *testing.T) {
	var s Service
	if _, err := s.GetClientDataSets(context.Background(), common.Address{}, types.ListOptions{Limit: 10}); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("want ErrUninitialized, got %v", err)
	}
}
