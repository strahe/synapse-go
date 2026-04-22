package spregistry

import (
	"context"
	"errors"
	"testing"

	"github.com/strahe/synapse-go/types"
)

func TestZeroValueReturnsErrUninitialized(t *testing.T) {
	var s Service
	if _, err := s.GetPDPProviders(context.Background(), false, types.ListOptions{Limit: 10}); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("want ErrUninitialized, got %v", err)
	}
}
