package storage

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestZeroValueReturnsErrUninitialized(t *testing.T) {
	var s Service
	if _, err := s.Upload(context.Background(), bytes.NewReader(nil), nil); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("want ErrUninitialized, got %v", err)
	}
}
