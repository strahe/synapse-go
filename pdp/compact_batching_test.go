package pdp

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

func TestPullPiecesLayoutLimits(t *testing.T) {
	largeID, err := types.BigIntFromBig(new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		id     *types.BigInt
		limit  uint64
		count  int
		reject bool
	}{
		{"legacy 80", new(types.NewBigInt(99)), 100, 80, false},
		{"legacy 81", new(types.NewBigInt(99)), 100, 81, true},
		{"compact cutoff", new(types.NewBigInt(100)), 100, 81, false},
		{"large ID", &largeID, 100, 81, false},
		{"new", nil, 100, 81, false},
		{"unknown", new(types.NewBigInt(99)), 0, 81, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				_, _ = w.Write([]byte(`{"status":"complete","pieces":[]}`))
			}))
			defer server.Close()
			client, err := New(server.URL, WithLegacyPieceStorageIDLimit(test.limit), WithMaxRetries(0))
			if err != nil {
				t.Fatal(err)
			}
			pieces := make([]PullPieceInput, test.count)
			for i := range pieces {
				info, err := piece.CalculateFromBytes(bytes.Repeat([]byte{byte(i), 0xa5}, 128))
				if err != nil {
					t.Fatal(err)
				}
				pieces[i] = PullPieceInput{PieceCID: info.CIDv2, SourceURL: "https://source.example.com/piece/" + info.CIDv2.String()}
			}
			_, err = client.PullPieces(context.Background(), PullRequest{RecordKeeper: common.HexToAddress("0xabc"), DataSetID: test.id, Pieces: pieces, ExtraData: []byte{1}})
			if test.reject {
				if !errors.Is(err, ErrTooManyPieces) || requests != 0 {
					t.Fatalf("err=%v requests=%d", err, requests)
				}
			} else if err != nil || requests != 1 {
				t.Fatalf("err=%v requests=%d", err, requests)
			}
		})
	}
}
