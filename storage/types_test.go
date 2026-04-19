package storage

import (
	"math/big"
	"testing"

	"github.com/ipfs/go-cid"
)

func TestUploadResultSuccessCount(t *testing.T) {
	oneCopy := CopyResult{ProviderID: big.NewInt(1), DataSetID: big.NewInt(1), PieceID: big.NewInt(1)}

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
	oneCopy := CopyResult{ProviderID: big.NewInt(1), DataSetID: big.NewInt(1), PieceID: big.NewInt(1)}

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
