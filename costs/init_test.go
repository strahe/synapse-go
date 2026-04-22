package costs

import (
	"context"
	"errors"
	"testing"
)

func TestZeroValueReturnsErrUninitialized(t *testing.T) {
	var s Service
	if _, err := s.GetServicePrice(context.Background()); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("want ErrUninitialized, got %v", err)
	}
}
