package sessionkey

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestZeroValueReturnsErrUninitialized(t *testing.T) {
	var s Service
	if _, err := s.GetExpirations(context.Background(), common.Address{}, common.Address{}, nil); !errors.Is(err, ErrUninitialized) {
		t.Fatalf("want ErrUninitialized, got %v", err)
	}
}
