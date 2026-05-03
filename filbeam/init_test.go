package filbeam

import (
	"context"
	"errors"
	"testing"

	"github.com/strahe/synapse-go/types"
)

func TestZeroValueReturnsErrUninitialized(t *testing.T) {
	var s Service
	if _, err := s.GetDataSetStats(context.Background(), types.NewBigInt(1)); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("want ErrUninitialized, got %v", err)
	}
}
