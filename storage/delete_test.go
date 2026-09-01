package storage

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multihash"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
)

func mustDeleteContext(t *testing.T, client PDPProviderClient, opts ...ContextOption) *DataSetContext {
	t.Helper()
	return mustDataSetContext(t, client, testDataSetRef(types.NewBigInt(77), types.NewBigInt(3)), opts...)
}

func TestContext_DeletePiece_DeletesFirstCIDMatch(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42), types.NewBigInt(99)}}
	var gotDataSetID, gotPieceID types.BigInt
	var gotExtraData []byte
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, dsID types.BigInt, pieceIDs []types.BigInt, extraData []byte) (common.Hash, error) {
			if len(pieceIDs) != 1 {
				t.Fatalf("pieceIDs=%v want one ID", pieceIDs)
			}
			gotDataSetID = dsID
			gotPieceID = pieceIDs[0]
			gotExtraData = extraData
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
	)
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
		scheduleDeletionsFn: func(_ context.Context, dsID types.BigInt, pieceIDs []types.BigInt, _ []byte) (common.Hash, error) {
			if len(pieceIDs) != 1 {
				t.Fatalf("pieceIDs=%v want one ID", pieceIDs)
			}
			gotDataSetID = dsID
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
	)
	if _, err := c.DeletePiece(context.Background(), info.CIDv2); err != nil {
		t.Fatalf("DeletePiece: %v", err)
	}
	if !pdp.gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("FindPieceIdsByCid dataSetID=%s want 77", pdp.gotDataSetID.String())
	}
	if !gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("SchedulePieceDeletions dataSetID=%s want 77", gotDataSetID.String())
	}
}

func TestContext_DeletePieceByID_Success(t *testing.T) {
	var gotDataSetID, gotPieceID types.BigInt
	var gotExtraData []byte
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, dsID types.BigInt, pieceIDs []types.BigInt, extraData []byte) (common.Hash, error) {
			if len(pieceIDs) != 1 {
				t.Fatalf("pieceIDs=%v want one ID", pieceIDs)
			}
			gotDataSetID = dsID
			gotPieceID = pieceIDs[0]
			gotExtraData = extraData
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)

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
		scheduleDeletionsFn: func(_ context.Context, dsID types.BigInt, pieceIDs []types.BigInt, _ []byte) (common.Hash, error) {
			if len(pieceIDs) != 1 {
				t.Fatalf("pieceIDs=%v want one ID", pieceIDs)
			}
			gotDataSetID = dsID
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if _, err := c.DeletePieceByID(context.Background(), types.NewBigInt(42)); err != nil {
		t.Fatalf("DeletePieceByID: %v", err)
	}
	if !gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("SchedulePieceDeletions dataSetID=%s want 77", gotDataSetID.String())
	}
}

func TestContext_DeletePieceByID_AllowsZeroPieceID(t *testing.T) {
	var gotPieceID types.BigInt
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, _ types.BigInt, pieceIDs []types.BigInt, _ []byte) (common.Hash, error) {
			if len(pieceIDs) != 1 {
				t.Fatalf("pieceIDs=%v want one ID", pieceIDs)
			}
			gotPieceID = pieceIDs[0]
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)

	if _, err := c.DeletePieceByID(context.Background(), types.NewBigInt(0)); err != nil {
		t.Fatalf("DeletePieceByID: %v", err)
	}
	if !gotPieceID.Equal(types.NewBigInt(0)) {
		t.Fatalf("pieceID=%s want 0", gotPieceID.String())
	}
}

func TestContext_DeletePiecesByID_NormalizesBeforeBatchCall(t *testing.T) {
	var gotDataSetID types.BigInt
	var gotPieceIDs []types.BigInt
	var gotExtraData []byte
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, dataSetID types.BigInt, pieceIDs []types.BigInt, extraData []byte) (common.Hash, error) {
			gotDataSetID = dataSetID
			gotPieceIDs = append([]types.BigInt(nil), pieceIDs...)
			gotExtraData = append([]byte(nil), extraData...)
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)

	res, err := c.DeletePiecesByID(context.Background(), []types.BigInt{
		types.NewBigInt(3), types.NewBigInt(2), types.NewBigInt(3), types.NewBigInt(0),
	})
	if err != nil {
		t.Fatalf("DeletePiecesByID: %v", err)
	}
	want := []types.BigInt{types.NewBigInt(3), types.NewBigInt(2), types.NewBigInt(0)}
	if !equalBigInts(gotPieceIDs, want) {
		t.Fatalf("pieceIDs=%v want %v", gotPieceIDs, want)
	}
	if !gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("dataSetID=%s want 77", gotDataSetID.String())
	}
	if len(gotExtraData) == 0 || res.Hash == (common.Hash{}) {
		t.Fatal("expected signed extraData and transaction hash")
	}
	values, err := bytesArgs.Unpack(gotExtraData)
	if err != nil {
		t.Fatalf("unpack extraData: %v", err)
	}
	domain := ityped.NewDomain(big.NewInt(314159), testRecordKeeper())
	message := ityped.SchedulePieceRemovalsMessage(
		types.NewBigInt(3).Big(),
		[]*big.Int{big.NewInt(3), big.NewInt(2), new(big.Int)},
	)
	recovered := recoverRawTypedDataSigner(t, domain, "SchedulePieceRemovals", message, values[0].([]byte))
	if recovered != c.core.signer.EVMAddress() {
		t.Fatalf("signature signer=%s want %s", recovered, c.core.signer.EVMAddress())
	}
}

func TestContext_DeletePieces_DeduplicatesResolvedIDs(t *testing.T) {
	info := mustPieceInfo(t)
	reader := &dataSetMutatingPDPReader{fakePDPReader: fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42)}}}
	var gotPieceIDs []types.BigInt
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, _ types.BigInt, pieceIDs []types.BigInt, _ []byte) (common.Hash, error) {
			gotPieceIDs = append([]types.BigInt(nil), pieceIDs...)
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(reader),
	)

	if _, err := c.DeletePieces(context.Background(), []cid.Cid{info.CIDv2, info.CIDv2}); err != nil {
		t.Fatalf("DeletePieces: %v", err)
	}
	if len(gotPieceIDs) != 1 || !gotPieceIDs[0].Equal(types.NewBigInt(42)) {
		t.Fatalf("pieceIDs=%v want [42]", gotPieceIDs)
	}
	if reader.calls != 1 {
		t.Fatalf("FindPieceIdsByCid calls=%d want 1", reader.calls)
	}
}

func TestContext_DeletePieces_UsesBatchCIDResolver(t *testing.T) {
	pieceCIDs := sequenceDeleteCIDs(t, 3)
	reader := &batchCIDResultPDPReader{
		batchResults: [][]types.BigInt{
			{types.NewBigInt(42), types.NewBigInt(99)},
			{types.NewBigInt(7)},
			{types.NewBigInt(42)},
		},
	}
	var gotPieceIDs []types.BigInt
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, _ types.BigInt, pieceIDs []types.BigInt, _ []byte) (common.Hash, error) {
			gotPieceIDs = append([]types.BigInt(nil), pieceIDs...)
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(reader),
	)

	if _, err := c.DeletePieces(context.Background(), pieceCIDs); err != nil {
		t.Fatalf("DeletePieces: %v", err)
	}
	if reader.batchCalls != 1 || reader.singularCalls != 0 {
		t.Fatalf("resolver calls=(batch %d, singular %d), want (1, 0)", reader.batchCalls, reader.singularCalls)
	}
	if !reader.gotDataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("batch dataSetID=%s want 77", reader.gotDataSetID.String())
	}
	if len(reader.gotPieceCIDs) != len(pieceCIDs) {
		t.Fatalf("batch piece CIDs=%d want %d", len(reader.gotPieceCIDs), len(pieceCIDs))
	}
	for i := range pieceCIDs {
		if !reader.gotPieceCIDs[i].Equals(pieceCIDs[i]) {
			t.Fatalf("batch pieceCID[%d]=%s want %s", i, reader.gotPieceCIDs[i], pieceCIDs[i])
		}
	}
	wantPieceIDs := []types.BigInt{types.NewBigInt(42), types.NewBigInt(7)}
	if !equalBigInts(gotPieceIDs, wantPieceIDs) {
		t.Fatalf("pieceIDs=%v want %v", gotPieceIDs, wantPieceIDs)
	}
}

func TestContext_DeletePieces_FallsBackAfterBatchCIDResolutionFailure(t *testing.T) {
	pieceCIDs := sequenceDeleteCIDs(t, 2)
	want := errors.New("multicall unavailable")
	reader := &batchCIDResultPDPReader{
		batchErr: want,
		singularResults: map[string][]types.BigInt{
			pieceCIDs[0].KeyString(): {types.NewBigInt(11)},
			pieceCIDs[1].KeyString(): {types.NewBigInt(22)},
		},
	}
	var gotPieceIDs []types.BigInt
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, _ types.BigInt, pieceIDs []types.BigInt, _ []byte) (common.Hash, error) {
			gotPieceIDs = append([]types.BigInt(nil), pieceIDs...)
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(reader),
	)

	if _, err := c.DeletePieces(context.Background(), pieceCIDs); err != nil {
		t.Fatalf("DeletePieces: %v", err)
	}
	if reader.batchCalls != 1 || reader.singularCalls != 2 {
		t.Fatalf("resolver calls=(batch %d, singular %d), want (1, 2)", reader.batchCalls, reader.singularCalls)
	}
	if !equalBigInts(gotPieceIDs, []types.BigInt{types.NewBigInt(11), types.NewBigInt(22)}) {
		t.Fatalf("scheduled piece IDs = %v, want [11 22]", gotPieceIDs)
	}
}

func TestContext_DeletePieces_PreservesBatchAndFallbackFailures(t *testing.T) {
	pieceCIDs := sequenceDeleteCIDs(t, 2)
	batchErr := errors.New("multicall unavailable")
	singularErr := errors.New("direct call unavailable")
	reader := &batchCIDResultPDPReader{batchErr: batchErr, singularErr: singularErr}
	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(reader),
	)

	_, err := c.DeletePieces(context.Background(), pieceCIDs)
	if !errors.Is(err, batchErr) || !errors.Is(err, singularErr) {
		t.Fatalf("DeletePieces error = %v, want batch and fallback failures", err)
	}
	if reader.batchCalls != 1 || reader.singularCalls != 1 {
		t.Fatalf("resolver calls=(batch %d, singular %d), want (1, 1)", reader.batchCalls, reader.singularCalls)
	}
}

func TestContext_DeletePieces_PropagatesUnavailableDataSet(t *testing.T) {
	pieceCIDs := sequenceDeleteCIDs(t, 2)
	reader := &batchCIDResultPDPReader{batchErr: ErrDataSetUnavailable}
	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(reader),
	)
	if _, err := c.DeletePieces(context.Background(), pieceCIDs); !errors.Is(err, ErrDataSetUnavailable) {
		t.Fatalf("DeletePieces error = %v, want ErrDataSetUnavailable", err)
	}
	if reader.batchCalls != 1 || reader.singularCalls != 0 {
		t.Fatalf("resolver calls=(batch %d, singular %d), want (1, 0)", reader.batchCalls, reader.singularCalls)
	}
}

func TestContext_DeletePieces_RejectsOversizedBatchBeforeResolution(t *testing.T) {
	reader := &dataSetMutatingPDPReader{}
	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(reader),
	)

	_, err := c.DeletePieces(context.Background(), sequenceDeleteCIDs(t, pdp.MaxDeletePiecesBatchSize+1))
	if !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, pdp.ErrTooManyPieces) {
		t.Fatalf("err=%v want ErrInvalidArgument and pdp.ErrTooManyPieces", err)
	}
	if reader.calls != 0 {
		t.Fatalf("FindPieceIdsByCid calls=%d want 0", reader.calls)
	}
}

func TestContext_DeletePieces_ReportsOriginalCIDIndexAfterDeduplication(t *testing.T) {
	pieceCIDs := sequenceDeleteCIDs(t, 2)
	reader := &cidResultPDPReader{
		results: map[string][]types.BigInt{
			pieceCIDs[0].KeyString(): {types.NewBigInt(42)},
		},
	}
	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(reader),
	)

	_, err := c.DeletePieces(context.Background(), []cid.Cid{pieceCIDs[0], pieceCIDs[0], pieceCIDs[1]})
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "index 2 not found") {
		t.Fatalf("err=%v want original index 2 not-found error", err)
	}
}

func TestContext_DeletePiecesByID_Validation(t *testing.T) {
	tests := []struct {
		name        string
		pieceIDs    []types.BigInt
		wantTooMany bool
	}{
		{name: "empty"},
		{name: "above Curio range", pieceIDs: []types.BigInt{mustBigInt(t, "9223372036854775808")}},
		{name: "too many", pieceIDs: sequenceBigInts(pdp.MaxDeletePiecesBatchSize + 1), wantTooMany: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := mustDeleteContext(t, &fakePDPProviderClient{})
			_, err := c.DeletePiecesByID(context.Background(), tt.pieceIDs)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("err=%v want ErrInvalidArgument", err)
			}
			if got := errors.Is(err, pdp.ErrTooManyPieces); got != tt.wantTooMany {
				t.Fatalf("errors.Is(ErrTooManyPieces)=%t want %t", got, tt.wantTooMany)
			}
		})
	}
}

func mustBigInt(t *testing.T, value string) types.BigInt {
	t.Helper()
	id, err := types.ParseBigInt(value)
	if err != nil {
		t.Fatalf("ParseBigInt(%q): %v", value, err)
	}
	return id
}

func sequenceBigInts(count int) []types.BigInt {
	ids := make([]types.BigInt, count)
	for i := range ids {
		ids[i] = types.NewBigInt(uint64(i))
	}
	return ids
}

func sequenceDeleteCIDs(t *testing.T, count int) []cid.Cid {
	t.Helper()
	pieceCIDs := make([]cid.Cid, count)
	for i := range pieceCIDs {
		hash, err := multihash.Sum([]byte{byte(i)}, multihash.SHA2_256, -1)
		if err != nil {
			t.Fatalf("create multihash %d: %v", i, err)
		}
		pieceCIDs[i] = cid.NewCidV1(cid.Raw, hash)
	}
	return pieceCIDs
}

func equalBigInts(got, want []types.BigInt) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !got[i].Equal(want[i]) {
			return false
		}
	}
	return true
}

func TestContext_DeletePieceByID_DoesNotCallDataSetValidator(t *testing.T) {
	want := errors.New("validator should not block delete")
	var gotPieceID types.BigInt
	validator := &fakeDataSetValidator{err: want}
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(_ context.Context, _ types.BigInt, pieceIDs []types.BigInt, _ []byte) (common.Hash, error) {
			if len(pieceIDs) != 1 {
				t.Fatalf("pieceIDs=%v want one ID", pieceIDs)
			}
			gotPieceID = pieceIDs[0]
			return common.HexToHash("0xabc123"), nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetValidator(validator),
	)

	if _, err := c.DeletePieceByID(context.Background(), types.NewBigInt(42)); err != nil {
		t.Fatalf("DeletePieceByID: %v", err)
	}
	if !gotPieceID.Equal(types.NewBigInt(42)) {
		t.Fatalf("pieceID=%s want 42", gotPieceID.String())
	}
	if len(validator.calls) != 0 {
		t.Fatalf("validator calls=%d want 0", len(validator.calls))
	}
}

func TestContext_DeletePiece_PieceNotFound(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: nil}
	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
	)
	if _, err := c.DeletePiece(context.Background(), info.CIDv2); err == nil {
		t.Fatal("expected error when piece not found")
	}
}

func TestContext_DeletePiece_FindError(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findErr: errors.New("rpc boom")}
	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
	)
	if _, err := c.DeletePiece(context.Background(), info.CIDv2); err == nil {
		t.Fatal("expected error")
	}
}

func TestContext_DeletePiece_InvalidCID(t *testing.T) {
	pdp := &fakePDPReader{}
	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
	)
	if _, err := c.DeletePiece(context.Background(), cid.Undef); err == nil {
		t.Fatal("expected error for undefined CID")
	}
}

func TestContext_DeletePiece_RejectsZeroRecordKeeper(t *testing.T) {
	info := mustPieceInfo(t)
	pdp := &fakePDPReader{findIDs: []types.BigInt{types.NewBigInt(42)}}
	fake := &fakePDPProviderClient{
		scheduleDeletionsFn: func(context.Context, types.BigInt, []types.BigInt, []byte) (common.Hash, error) {
			t.Fatal("SchedulePieceDeletions should not be called with zero recordKeeper")
			return common.Hash{}, nil
		},
	}
	c := mustDeleteContext(t, fake,
		WithPayer(testPayer()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
	)
	_, err := c.DeletePiece(context.Background(), info.CIDv2)
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

	c := mustDeleteContext(t, &fakePDPProviderClient{},
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithPDPVerifierReader(pdp),
	)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("DeletePiece panicked with typed-nil PDP reader: %v", r)
		}
	}()

	_, err := c.DeletePiece(context.Background(), info.CIDv2)
	if err == nil || !strings.Contains(err.Error(), "PDPVerifier reader not configured") {
		t.Fatalf("err=%v want PDPVerifier reader not configured", err)
	}
}

type dataSetMutatingPDPReader struct {
	fakePDPReader
	mutate       func()
	gotDataSetID types.BigInt
	calls        int
}

type cidResultPDPReader struct {
	fakePDPReader
	results map[string][]types.BigInt
}

type batchCIDResultPDPReader struct {
	fakePDPReader
	batchResults    [][]types.BigInt
	batchErr        error
	singularErr     error
	singularResults map[string][]types.BigInt
	gotDataSetID    types.BigInt
	gotPieceCIDs    []cid.Cid
	batchCalls      int
	singularCalls   int
}

func (r *cidResultPDPReader) FindPieceIdsByCid(_ context.Context, _ types.BigInt, pieceCID cid.Cid, _, _ uint64) ([]types.BigInt, error) {
	return r.results[pieceCID.KeyString()], nil
}

func (r *batchCIDResultPDPReader) FindPieceIDsByCIDs(_ context.Context, dataSetID types.BigInt, pieceCIDs []cid.Cid) ([][]types.BigInt, error) {
	r.batchCalls++
	r.gotDataSetID = dataSetID
	r.gotPieceCIDs = append([]cid.Cid(nil), pieceCIDs...)
	return r.batchResults, r.batchErr
}

func (r *batchCIDResultPDPReader) FindPieceIdsByCid(_ context.Context, _ types.BigInt, pieceCID cid.Cid, _, _ uint64) ([]types.BigInt, error) {
	r.singularCalls++
	return r.singularResults[pieceCID.KeyString()], r.singularErr
}

func (r *dataSetMutatingPDPReader) FindPieceIdsByCid(_ context.Context, dataSetID types.BigInt, _ cid.Cid, _, _ uint64) ([]types.BigInt, error) {
	r.calls++
	r.gotDataSetID = dataSetID
	if r.mutate != nil {
		r.mutate()
	}
	return r.findIDs, r.findErr
}
