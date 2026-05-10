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
			err:        &StoreError{ProviderID: types.NewBigInt(0), Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.StoreError: provider 0",
		},
		{
			name:       "with ProviderID, nil Cause",
			err:        &StoreError{ProviderID: types.NewBigInt(42), Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.StoreError: provider 42",
		},
		{
			name:       "with Cause",
			err:        &StoreError{ProviderID: types.NewBigInt(7), Endpoint: "https://sp.example.com", Cause: errors.New("timeout")},
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
			err:        &CommitError{ProviderID: types.NewBigInt(0), Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.CommitError: provider 0",
		},
		{
			name:       "with ProviderID, nil Cause",
			err:        &CommitError{ProviderID: types.NewBigInt(99), Endpoint: "https://sp.example.com"},
			wantPrefix: "storage.CommitError: provider 99",
		},
		{
			name:       "with Cause",
			err:        &CommitError{ProviderID: types.NewBigInt(5), Endpoint: "https://sp.example.com", Cause: errors.New("conflict")},
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

// TestErrInvalidArgument_Detection verifies validation wraps return
// errors matching ErrInvalidArgument via errors.Is.
func TestErrInvalidArgument_Detection(t *testing.T) {
	t.Run("NewServiceResolver: zero payer", func(t *testing.T) {
		_, err := NewServiceResolver(ServiceResolverOptions{})
		requireInvalidArgument(t, err)
	})
	// The remaining NewServiceResolver guards (nil SPRegistry/WarmStorage/
	// NewContext) also wrap ErrInvalidArgument; a representative case
	// above is enough — all paths share the same wrap convention.
}

// TestErrInvalidArgument_NegativeMatch verifies ErrInvalidArgument does
// not accidentally match unrelated errors.
func TestErrInvalidArgument_NegativeMatch(t *testing.T) {
	if errors.Is(ErrInvalidDownloadOptions, ErrInvalidArgument) {
		t.Fatal("ErrInvalidDownloadOptions must not match ErrInvalidArgument")
	}
	if errors.Is(errors.New("unrelated"), ErrInvalidArgument) {
		t.Fatal("unrelated error must not match ErrInvalidArgument")
	}
}

func TestDataSetPDPPaymentTerminatedError_DoesNotMatchInvalidArgument(t *testing.T) {
	dataSetID := types.NewBigInt(13269)
	err := &DataSetPDPPaymentTerminatedError{
		DataSetID:   dataSetID,
		PDPEndEpoch: 3778900,
	}

	requireDataSetPDPPaymentTerminated(t, err, dataSetID, 3778900)
	if !strings.Contains(err.Error(), "has PDP payment rail end epoch 3778900") {
		t.Fatalf("Error()=%q want future-safe end epoch wording", err.Error())
	}
	if strings.Contains(err.Error(), "ended at epoch") {
		t.Fatalf("Error()=%q must not imply the payment rail already ended", err.Error())
	}
}

func requireInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("errors.Is(err, ErrInvalidArgument)=false; err=%v", err)
	}
}

func requireDataSetPDPPaymentTerminated(t *testing.T, err error, dataSetID types.BigInt, pdpEndEpoch types.Epoch) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	got, ok := errors.AsType[*DataSetPDPPaymentTerminatedError](err)
	if !ok {
		t.Fatalf("errors.AsType[*DataSetPDPPaymentTerminatedError]=false; err=%T %v", err, err)
	}
	if !got.DataSetID.Equal(dataSetID) {
		t.Fatalf("DataSetID=%s want %s", got.DataSetID.String(), dataSetID.String())
	}
	if got.PDPEndEpoch != pdpEndEpoch {
		t.Fatalf("PDPEndEpoch=%d want %d", got.PDPEndEpoch, pdpEndEpoch)
	}
	if errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("DataSetPDPPaymentTerminatedError must not match ErrInvalidArgument: %v", err)
	}
}
