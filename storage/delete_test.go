package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/types"
)

// TestContext_DeletePiece_ExistingDataSetRequiresClientDataSetID locks
// the guard that rejects existing-dataset contexts without a supplied
// ClientDataSetID. See ensureClientDataSetID in storage/delete.go.
func TestContext_DeletePiece_ExistingDataSetRequiresClientDataSetID(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42)}}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithPDPVerifierReader(pdp),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = c.DeletePiece(context.Background(), info.CIDv2)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "clientDataSetID is required") {
		t.Fatalf("error must mention clientDataSetID requirement; got: %v", err)
	}
}

func TestContext_DeletePiece_Success(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42)}}
	var gotDataSetID, gotPieceID types.BigInt
	var gotExtraData []byte
	fake := &fakePDPProviderClient{
		scheduleDeletionFn: func(_ context.Context, dsID, pID types.BigInt, extraData []byte) (common.Hash, error) {
			gotDataSetID = dsID
			gotPieceID = pID
			gotExtraData = extraData
			return common.HexToHash("0xabc123"), nil
		},
	}
	c, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
		WithPDPVerifierReader(pdp),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	res, err := c.DeletePiece(context.Background(), info.CIDv2)
	if err != nil {
		t.Fatalf("DeletePiece: %v", err)
	}
	if res.Hash == (common.Hash{}) {
		t.Fatal("expected non-zero hash")
	}
	if !gotDataSetID.Equal(types.NewBigInt(77)) || !gotPieceID.Equal(types.NewBigInt(42)) {
		t.Fatalf("unexpected (dataSetID, pieceID) = (%s, %s)", gotDataSetID.String(), gotPieceID.String())
	}
	if len(gotExtraData) == 0 {
		t.Fatal("expected non-empty extraData")
	}
}

func TestContext_DeletePiece_PieceNotFound(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: nil}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
		WithPDPVerifierReader(pdp),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.DeletePiece(context.Background(), info.CIDv2); err == nil {
		t.Fatal("expected error when piece not found")
	}
}

func TestContext_DeletePiece_FindError(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findErr: errors.New("rpc boom")}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
		WithPDPVerifierReader(pdp),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.DeletePiece(context.Background(), info.CIDv2); err == nil {
		t.Fatal("expected error")
	}
}

func TestContext_DeletePiece_InvalidCID(t *testing.T) {
	pdp := &fakePDPReader{}
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
		WithPDPVerifierReader(pdp),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := c.DeletePiece(context.Background(), cid.Undef); err == nil {
		t.Fatal("expected error for undefined CID")
	}
}

func TestContext_DeletePiece_RejectsZeroRecordKeeper(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42)}}
	fake := &fakePDPProviderClient{
		scheduleDeletionFn: func(context.Context, types.BigInt, types.BigInt, []byte) (common.Hash, error) {
			t.Fatal("SchedulePieceDeletion should not be called with zero recordKeeper")
			return common.Hash{}, nil
		},
	}
	c, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
		WithPDPVerifierReader(pdp),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = c.DeletePiece(context.Background(), info.CIDv2)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "zero recordKeeper") {
		t.Fatalf("err=%v want zero recordKeeper", err)
	}
}

func TestContext_DeletePiece_TypedNilPDPReaderTreatedAsUnset(t *testing.T) {
	info := mustPieceInfo(t)
	var pdp *fakePDPReader

	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
		WithPDPVerifierReader(pdp),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DeletePiece panicked with typed-nil PDP reader: %v", r)
		}
	}()

	_, err = c.DeletePiece(context.Background(), info.CIDv2)
	if err == nil || !strings.Contains(err.Error(), "PDPVerifier reader not configured") {
		t.Fatalf("err=%v want PDPVerifier reader not configured", err)
	}
}
