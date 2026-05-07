package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

// TestContext_DeletePiece_ExistingDataSetRequiresClientDataSetID locks
// the guard that rejects existing-dataset contexts without a supplied
// ClientDataSetID.
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

func TestContext_DeletePiece_DeletesFirstCIDMatch(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42), types.NewBigInt(99)}}
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

func TestContext_DeletePiece_UsesSnapshotDataSetID(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &dataSetMutatingPDPReader{
		fakePDPReader: fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42)}},
	}
	var gotDataSetID types.BigInt
	fake := &fakePDPProviderClient{
		scheduleDeletionFn: func(_ context.Context, dsID, _ types.BigInt, _ []byte) (common.Hash, error) {
			gotDataSetID = dsID
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
	pdp.mutate = func() { setContextDataSetID(c, types.NewBigInt(88)) }

	if _, err := c.DeletePiece(context.Background(), info.CIDv2); err != nil {
		t.Fatalf("DeletePiece: %v", err)
	}
	if !pdp.gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("FindPieceIdsByCid dataSetID=%s want 77", pdp.gotDataSetID.String())
	}
	if !gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("SchedulePieceDeletion dataSetID=%s want 77", gotDataSetID.String())
	}
}

func TestContext_DeletePieceByID_Success(t *testing.T) {
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
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	res, err := c.DeletePieceByID(context.Background(), types.NewBigInt(42))
	if err != nil {
		t.Fatalf("DeletePieceByID: %v", err)
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

func TestContext_DeletePieceByID_UsesSnapshotDataSetID(t *testing.T) {
	var gotDataSetID types.BigInt
	fake := &fakePDPProviderClient{
		scheduleDeletionFn: func(_ context.Context, dsID, _ types.BigInt, _ []byte) (common.Hash, error) {
			gotDataSetID = dsID
			return common.HexToHash("0xabc123"), nil
		},
	}
	mutatingSigner := &dataSetMutatingSigner{EVMSigner: mustTestSigner(t)}
	c, err := NewContext(testProvider(), fake, mutatingSigner,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	mutatingSigner.mutate = func() { setContextDataSetID(c, types.NewBigInt(88)) }

	if _, err := c.DeletePieceByID(context.Background(), types.NewBigInt(42)); err != nil {
		t.Fatalf("DeletePieceByID: %v", err)
	}
	if !gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("SchedulePieceDeletion dataSetID=%s want 77", gotDataSetID.String())
	}
}

func TestContext_DeletePieceByID_AllowsZeroPieceID(t *testing.T) {
	var gotPieceID types.BigInt
	fake := &fakePDPProviderClient{
		scheduleDeletionFn: func(_ context.Context, _ types.BigInt, pID types.BigInt, _ []byte) (common.Hash, error) {
			gotPieceID = pID
			return common.HexToHash("0xabc123"), nil
		},
	}
	c, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
		WithClientDataSetID(types.NewBigInt(3)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	if _, err := c.DeletePieceByID(context.Background(), types.NewBigInt(0)); err != nil {
		t.Fatalf("DeletePieceByID: %v", err)
	}
	if !gotPieceID.Equal(types.NewBigInt(0)) {
		t.Fatalf("pieceID=%s want 0", gotPieceID.String())
	}
}

func TestContext_DeletePieceByID_ExistingDataSetRequiresClientDataSetID(t *testing.T) {
	c, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(77)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = c.DeletePieceByID(context.Background(), types.NewBigInt(42))
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "clientDataSetID is required") {
		t.Fatalf("error must mention clientDataSetID requirement; got: %v", err)
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

type dataSetMutatingPDPReader struct {
	fakePDPReader
	mutate       func()
	gotDataSetID types.BigInt
}

func (r *dataSetMutatingPDPReader) FindPieceIdsByCid(_ context.Context, dataSetID types.BigInt, _ cid.Cid, _, _ uint64) ([]types.BigInt, error) {
	r.gotDataSetID = dataSetID
	if r.mutate != nil {
		r.mutate()
	}
	return r.findIDs, r.findErr
}

type dataSetMutatingSigner struct {
	signer.EVMSigner
	mutate func()
}

func (s *dataSetMutatingSigner) SignHash(_ []byte) ([]byte, error) {
	if s.mutate != nil {
		s.mutate()
	}
	return make([]byte, 65), nil
}

func setContextDataSetID(c *Context, id types.BigInt) {
	c.mu.Lock()
	defer c.mu.Unlock()
	v := id
	c.dataSetID = &v
}
