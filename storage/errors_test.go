package storage

import (
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
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
	var nilErr *StoreError
	if nilErr.Unwrap() != nil {
		t.Fatal("nil StoreError must unwrap to nil")
	}
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
	var nilErr *CommitError
	if nilErr.Unwrap() != nil {
		t.Fatal("nil CommitError must unwrap to nil")
	}
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

func TestCrossPackageErrorAliases(t *testing.T) {
	if reflect.ValueOf(ErrEndorsementsNotConfigured) != reflect.ValueOf(spregistry.ErrEndorsementsNotConfigured) {
		t.Fatal("ErrEndorsementsNotConfigured is not the spregistry sentinel")
	}
	if reflect.ValueOf(ErrDataSetUnavailable) != reflect.ValueOf(warmstorage.ErrDataSetUnavailable) {
		t.Fatal("ErrDataSetUnavailable is not the warmstorage sentinel")
	}
}

func TestDataSetPDPPaymentTerminatedError_DoesNotMatchInvalidArgument(t *testing.T) {
	var nilErr *DataSetPDPPaymentTerminatedError
	if got := nilErr.Error(); got != "<nil>" {
		t.Fatalf("nil Error() = %q", got)
	}
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

func TestTerminateServiceDebtError_Error(t *testing.T) {
	var nilErr *TerminateServiceDebtError
	if got := nilErr.Error(); got != "<nil>" {
		t.Fatalf("nil Error() = %q", got)
	}
	withoutAmount := (&TerminateServiceDebtError{}).Error()
	if !strings.Contains(withoutAmount, "settling existing payment debt") {
		t.Fatalf("Error() = %q", withoutAmount)
	}
	withAmount := (&TerminateServiceDebtError{Shortfall: big.NewInt(17)}).Error()
	if !strings.Contains(withAmount, "shortfall 17") {
		t.Fatalf("Error() = %q", withAmount)
	}
}

func TestDownloadError_ErrorAndUnwrap(t *testing.T) {
	cause := errors.New("connection reset")
	tests := []struct {
		name string
		err  *DownloadError
		want string
	}{
		{"nil receiver", nil, "<nil>"},
		{"status and cause", &DownloadError{URL: "https://sp.example/piece", StatusCode: 503, Cause: cause}, "status 503: connection reset"},
		{"status", &DownloadError{URL: "https://sp.example/piece", StatusCode: 404}, "status 404"},
		{"cause", &DownloadError{URL: "https://sp.example/piece", Cause: cause}, "connection reset"},
		{"url", &DownloadError{URL: "https://sp.example/piece"}, "https://sp.example/piece"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("Error() = %q, want substring %q", got, tt.want)
			}
		})
	}
	wrapped := &DownloadError{Cause: cause}
	if !errors.Is(wrapped, cause) {
		t.Fatalf("errors.Is(%v, cause) = false", wrapped)
	}
	var nilDownload *DownloadError
	if nilDownload.Unwrap() != nil {
		t.Fatal("nil DownloadError must unwrap to nil")
	}
}

func TestCIDMismatchError_Error(t *testing.T) {
	var nilErr *CIDMismatchError
	if got := nilErr.Error(); got != "<nil>" {
		t.Fatalf("nil Error() = %q", got)
	}
	err := &CIDMismatchError{}
	got := err.Error()
	if !strings.Contains(got, "pieceCID mismatch") || !strings.Contains(got, "want") {
		t.Fatalf("Error() = %q", got)
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
