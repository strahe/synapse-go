package storage

import (
	"fmt"
	"math/big"
)

type StoreError struct {
	ProviderID *big.Int
	Endpoint   string
	Cause      error
}

func (e *StoreError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("storage.StoreError: provider %s (%s)", bigIntString(e.ProviderID), e.Endpoint)
	}
	return fmt.Sprintf("storage.StoreError: provider %s (%s): %v", bigIntString(e.ProviderID), e.Endpoint, e.Cause)
}

func (e *StoreError) Unwrap() error { return e.Cause }

type CommitError struct {
	ProviderID *big.Int
	Endpoint   string
	Cause      error
}

func (e *CommitError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("storage.CommitError: provider %s (%s)", bigIntString(e.ProviderID), e.Endpoint)
	}
	return fmt.Sprintf("storage.CommitError: provider %s (%s): %v", bigIntString(e.ProviderID), e.Endpoint, e.Cause)
}

func (e *CommitError) Unwrap() error { return e.Cause }

func bigIntString(v *big.Int) string {
	if v == nil {
		return "<nil>"
	}
	return v.String()
}
