package pdp

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/types"
)

func TestEstimateAddPiecesMessageSizeMatchesGeneratedABI(t *testing.T) {
	info := testPieceInfoV2(t)
	extraData := []byte{0xde, 0xad, 0xbe, 0xef}
	pieces := []AddPieceInput{{PieceCID: info.CIDv2}}

	got, err := EstimateAddPiecesMessageSize(pieces, extraData)
	if err != nil {
		t.Fatalf("EstimateAddPiecesMessageSize: %v", err)
	}
	contractABI, err := pdpverifier.PDPVerifierMetaData.GetAbi()
	if err != nil {
		t.Fatalf("GetAbi: %v", err)
	}
	encoded, err := contractABI.Pack(
		"addPieces",
		new(big.Int),
		common.Address{},
		[]pdpverifier.CidsCid{{Data: info.CIDv2.Bytes()}},
		extraData,
	)
	if err != nil {
		t.Fatalf("Pack(addPieces): %v", err)
	}
	if got != len(encoded) {
		t.Fatalf("size=%d want generated ABI size %d", got, len(encoded))
	}
}

func TestEstimateAddPiecesMessageSizeBoundary(t *testing.T) {
	pieces := []AddPieceInput{{PieceCID: testPieceInfoV2(t).CIDv2}}
	base, err := EstimateAddPiecesMessageSize(pieces, nil)
	if err != nil {
		t.Fatalf("estimate base: %v", err)
	}
	largestExtraData := (MaxAddPiecesMessageSize - base) / 32 * 32
	accepted, err := EstimateAddPiecesMessageSize(pieces, make([]byte, largestExtraData))
	if err != nil {
		t.Fatalf("estimate accepted: %v", err)
	}
	rejected, err := EstimateAddPiecesMessageSize(pieces, make([]byte, largestExtraData+1))
	if err != nil {
		t.Fatalf("estimate rejected: %v", err)
	}
	if accepted > MaxAddPiecesMessageSize {
		t.Fatalf("accepted size=%d max=%d", accepted, MaxAddPiecesMessageSize)
	}
	if rejected <= MaxAddPiecesMessageSize {
		t.Fatalf("rejected size=%d max=%d", rejected, MaxAddPiecesMessageSize)
	}
	if rejected-accepted != 32 {
		t.Fatalf("ABI padding step=%d want 32", rejected-accepted)
	}
}

func TestAddPiecesEntryPointsRejectOversizedMessageBeforeRequest(t *testing.T) {
	info := testPieceInfoV2(t)
	pieces := []AddPieceInput{{PieceCID: info.CIDv2}}
	extraData := make([]byte, MaxAddPiecesMessageSize)
	tests := []struct {
		name string
		call func(context.Context, *Client) error
	}{
		{
			name: "add pieces",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.AddPieces(ctx, types.NewBigInt(1), pieces, extraData)
				return err
			},
		},
		{
			name: "create and add",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.CreateDataSetAndAddPieces(ctx, common.HexToAddress("0x1234"), pieces, extraData)
				return err
			},
		},
		{
			name: "pull pieces",
			call: func(ctx context.Context, client *Client) error {
				_, err := client.PullPieces(ctx, PullRequest{
					RecordKeeper: common.HexToAddress("0x1234"),
					ExtraData:    extraData,
					Pieces: []PullPieceInput{{
						PieceCID:  info.CIDv2,
						SourceURL: "https://source.example.com/piece/1",
					}},
				})
				return err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			requests := 0
			client, _ := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				requests++
			}))
			err := test.call(context.Background(), client)
			if !errors.Is(err, ErrAddPiecesMessageTooLarge) {
				t.Fatalf("error=%v want ErrAddPiecesMessageTooLarge", err)
			}
			var sizeError *AddPiecesMessageTooLargeError
			if !errors.As(err, &sizeError) {
				t.Fatalf("error=%v want AddPiecesMessageTooLargeError", err)
			}
			if sizeError.Size <= sizeError.Max || sizeError.Max != MaxAddPiecesMessageSize {
				t.Fatalf("size error=%+v", sizeError)
			}
			if requests != 0 {
				t.Fatalf("requests=%d want 0", requests)
			}
		})
	}
}

func TestEstimateAddPiecesMessageSizeRejectsUndefinedCID(t *testing.T) {
	_, err := EstimateAddPiecesMessageSize([]AddPieceInput{{}}, []byte{1})
	if err == nil {
		t.Fatal("EstimateAddPiecesMessageSize error=nil")
	}
}
