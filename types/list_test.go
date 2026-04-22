package types

import (
	"errors"
	"testing"
)

func TestListOptions_Validate(t *testing.T) {
	t.Run("zero Limit", func(t *testing.T) {
		err := ListOptions{}.Validate()
		if !errors.Is(err, ErrInvalidListOptions) {
			t.Fatalf("errors.Is=false: %v", err)
		}
	})
	t.Run("positive Limit", func(t *testing.T) {
		if err := (ListOptions{Limit: 1}).Validate(); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
}
