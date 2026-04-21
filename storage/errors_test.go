package storage

import (
	"errors"
	"strings"
	"testing"

	"github.com/strahe/synapse-go/types"
)

func TestStoreError_Error(t *testing.T) {
	tests := []struct {
		name       string
		err        *StoreError
		wantPrefix string
		wantAlso   string // additional substring that must also be present
	}{
		{
			name:       "nil receiver",
			wantPrefix: "<nil>",
		},
		{
			name:       "zero ProviderID and nil Cause",
			err:        &StoreError{ProviderID: 0, Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.StoreError: provider 0",
		},
		{
			name:       "with ProviderID, nil Cause",
			err:        &StoreError{ProviderID: types.ProviderID(42), Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.StoreError: provider 42",
		},
		{
			name:       "with Cause",
			err:        &StoreError{ProviderID: types.ProviderID(7), Endpoint: "https://sp.example.com", Cause: errors.New("timeout")},
			wantPrefix: "storage.StoreError: provider 7",
			wantAlso:   "timeout",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("Error()=%q, want prefix %q", got, tt.wantPrefix)
			}
			if tt.wantAlso != "" && !strings.Contains(got, tt.wantAlso) {
				t.Fatalf("Error()=%q, want substring %q", got, tt.wantAlso)
			}
		})
	}
}

func TestStoreError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &StoreError{Cause: cause}
	if !errors.Is(err.Unwrap(), cause) {
		t.Fatalf("Unwrap()=%v want %v", err.Unwrap(), cause)
	}
}

func TestCommitError_Error(t *testing.T) {
	tests := []struct {
		name       string
		err        *CommitError
		wantPrefix string
		wantAlso   string
	}{
		{
			name:       "nil receiver",
			wantPrefix: "<nil>",
		},
		{
			name:       "zero ProviderID and nil Cause",
			err:        &CommitError{ProviderID: 0, Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.CommitError: provider 0",
		},
		{
			name:       "with ProviderID, nil Cause",
			err:        &CommitError{ProviderID: types.ProviderID(99), Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.CommitError: provider 99",
		},
		{
			name:       "with Cause",
			err:        &CommitError{ProviderID: types.ProviderID(5), Endpoint: "https://sp.example.com", Cause: errors.New("conflict")},
			wantPrefix: "storage.CommitError: provider 5",
			wantAlso:   "conflict",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Fatalf("Error()=%q, want prefix %q", got, tt.wantPrefix)
			}
			if tt.wantAlso != "" && !strings.Contains(got, tt.wantAlso) {
				t.Fatalf("Error()=%q, want substring %q", got, tt.wantAlso)
			}
		})
	}
}

func TestCommitError_Unwrap(t *testing.T) {
	cause := errors.New("root cause")
	err := &CommitError{Cause: cause}
	if !errors.Is(err.Unwrap(), cause) {
		t.Fatalf("Unwrap()=%v want %v", err.Unwrap(), cause)
	}
}
