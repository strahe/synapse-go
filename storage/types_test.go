package storage

import (
	"testing"

	"github.com/ipfs/go-cid"

	sdktypes "github.com/strahe/synapse-go/types"
)

func TestUploadResultSuccessCount(t *testing.T) {
	oneCopy := CopyResult{ProviderID: sdktypes.NewBigInt(1), DataSetID: sdktypes.NewBigInt(1), PieceID: sdktypes.NewBigInt(1)}

	tests := []struct {
		name string
		r    *UploadResult
		want int
	}{
		{"nil receiver", nil, 0},
		{"no copies", &UploadResult{}, 0},
		{"one copy", &UploadResult{Copies: []CopyResult{oneCopy}}, 1},
		{"two copies", &UploadResult{Copies: []CopyResult{oneCopy, oneCopy}}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.SuccessCount(); got != tt.want {
				t.Fatalf("SuccessCount() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestUploadResultPartialSuccess(t *testing.T) {
	dummyCID := cid.MustParse("baga6ea4seaqao7s73y24kcutaosvacpdjgfe74urr3enp3bccbm2fszfxwqvria")
	oneCopy := CopyResult{ProviderID: sdktypes.NewBigInt(1), DataSetID: sdktypes.NewBigInt(1), PieceID: sdktypes.NewBigInt(1)}

	tests := []struct {
		name string
		r    *UploadResult
		want bool
	}{
		{"nil receiver", nil, false},
		{"complete", &UploadResult{PieceCID: dummyCID, Complete: true, RequestedCopies: 1, Copies: []CopyResult{oneCopy}}, false},
		{"no copies succeeded", &UploadResult{PieceCID: dummyCID, Complete: false, RequestedCopies: 2}, false},
		{"partial: 1 of 2", &UploadResult{PieceCID: dummyCID, Complete: false, RequestedCopies: 2, Copies: []CopyResult{oneCopy}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.PartialSuccess(); got != tt.want {
				t.Fatalf("PartialSuccess() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUploadResultPrimaryDataSetID(t *testing.T) {
	primary := CopyResult{Role: CopyRolePrimary, ProviderID: sdktypes.NewBigInt(10), DataSetID: sdktypes.NewBigInt(42)}
	secondary := CopyResult{Role: CopyRoleSecondary, ProviderID: sdktypes.NewBigInt(11), DataSetID: sdktypes.NewBigInt(99)}

	t.Run("nil receiver", func(t *testing.T) {
		var r *UploadResult
		if id, ok := r.PrimaryDataSetID(); ok || !id.IsZero() {
			t.Fatalf("got (%s, %v), want (0, false)", id, ok)
		}
	})

	t.Run("no primary", func(t *testing.T) {
		r := &UploadResult{Copies: []CopyResult{secondary}}
		if id, ok := r.PrimaryDataSetID(); ok || !id.IsZero() {
			t.Fatalf("got (%s, %v), want (0, false)", id, ok)
		}
	})

	t.Run("primary present", func(t *testing.T) {
		r := &UploadResult{Copies: []CopyResult{secondary, primary}}
		id, ok := r.PrimaryDataSetID()
		if !ok || !id.Equal(sdktypes.NewBigInt(42)) {
			t.Fatalf("got (%s, %v), want (42, true)", id, ok)
		}
	})
}

func TestUploadResultSuccessfulProviderIDs(t *testing.T) {
	c1 := CopyResult{Role: CopyRolePrimary, ProviderID: sdktypes.NewBigInt(10), DataSetID: sdktypes.NewBigInt(1)}
	c2 := CopyResult{Role: CopyRoleSecondary, ProviderID: sdktypes.NewBigInt(11), DataSetID: sdktypes.NewBigInt(2)}

	t.Run("nil receiver", func(t *testing.T) {
		var r *UploadResult
		if got := r.SuccessfulProviderIDs(); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("no copies", func(t *testing.T) {
		if got := (&UploadResult{}).SuccessfulProviderIDs(); got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})

	t.Run("preserves Copies order", func(t *testing.T) {
		r := &UploadResult{Copies: []CopyResult{c1, c2}}
		got := r.SuccessfulProviderIDs()
		want := []sdktypes.BigInt{sdktypes.NewBigInt(10), sdktypes.NewBigInt(11)}
		if len(got) != len(want) || !got[0].Equal(want[0]) || !got[1].Equal(want[1]) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
}
