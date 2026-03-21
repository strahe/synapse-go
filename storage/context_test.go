package storage

import (
	"bytes"
	"context"
	"io"
	"math"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"

	icurio "github.com/strahe/synapse-go/internal/curio"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
)

func TestContextStoreBytes_UploadsAndWaits(t *testing.T) {
	data := bytes.Repeat([]byte("st"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	fake := &fakeCurioClient{
		uploadFromBytesFn: func(_ context.Context, pieceCID cid.Cid, got []byte) (*icurio.UploadPieceResult, error) {
			if pieceCID != info.CIDv2 {
				t.Fatalf("pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("uploaded bytes mismatch")
			}
			return &icurio.UploadPieceResult{AlreadyExists: false, UploadUUID: "upload-1"}, nil
		},
		waitForPieceFn: func(_ context.Context, pieceCID cid.Cid, _ time.Duration) error {
			if pieceCID != info.CIDv2 {
				t.Fatalf("wait pieceCID=%s want %s", pieceCID, info.CIDv2)
			}
			return nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.StoreBytes(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("StoreBytes: %v", err)
	}
	if got.PieceCID != info.CIDv2 {
		t.Fatalf("pieceCID=%s want %s", got.PieceCID, info.CIDv2)
	}
	if got.Size != int64(len(data)) {
		t.Fatalf("size=%d want %d", got.Size, len(data))
	}
}

func TestContextPull_NewDataSetUsesRecordKeeper(t *testing.T) {
	info := mustPieceInfo(t)
	recordKeeper := testRecordKeeper()
	primaryURL := "https://primary.example.com/pdp/piece/" + info.CIDv2.String()

	fake := &fakeCurioClient{
		pullPiecesFn: func(_ context.Context, req icurio.PullRequest) (*icurio.PullResult, error) {
			if req.DataSetID != 0 {
				t.Fatalf("dataSetID=%d want 0", req.DataSetID)
			}
			if req.RecordKeeper != recordKeeper {
				t.Fatalf("recordKeeper=%s want %s", req.RecordKeeper, recordKeeper)
			}
			if len(req.Pieces) != 1 || req.Pieces[0].SourceURL != primaryURL {
				t.Fatalf("unexpected pull pieces: %+v", req.Pieces)
			}
			return &icurio.PullResult{
				Status: icurio.PullStatusComplete,
				Pieces: []icurio.PullPieceStatus{{PieceCID: info.CIDv2.String(), Status: icurio.PullStatusComplete}},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(recordKeeper),
		WithChainID(big.NewInt(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Pull(context.Background(), PullRequest{
		Pieces:    []cid.Cid{info.CIDv2},
		From:      func(cid.Cid) string { return primaryURL },
		ExtraData: []byte{0xaa},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if got.Status != PullStatusComplete {
		t.Fatalf("status=%q want %q", got.Status, PullStatusComplete)
	}
}

func TestContextCommit_ExistingDataSetUsesAddPieces(t *testing.T) {
	info := mustPieceInfo(t)
	dataSetID := big.NewInt(42)

	fake := &fakeCurioClient{
		addPiecesFn: func(_ context.Context, gotDataSetID uint64, pieces []icurio.AddPieceInput, extraData []byte) (*icurio.AddPiecesResult, error) {
			if gotDataSetID != dataSetID.Uint64() {
				t.Fatalf("dataSetID=%d want %d", gotDataSetID, dataSetID.Uint64())
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("unexpected pieces: %+v", pieces)
			}
			if !bytes.Equal(extraData, []byte{0x01}) {
				t.Fatalf("extraData=%x want 01", extraData)
			}
			return &icurio.AddPiecesResult{TxHash: common.HexToHash("0x01"), StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, statusURL string, _ time.Duration) (*icurio.AddPiecesStatus, error) {
			if statusURL == "" {
				t.Fatal("empty statusURL")
			}
			return &icurio.AddPiecesStatus{
				TxHash:            common.HexToHash("0x01"),
				DataSetID:         dataSetID.Uint64(),
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(7)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
		WithDataSetID(dataSetID),
		WithClientDataSetID(big.NewInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got.DataSetID.Cmp(dataSetID) != 0 {
		t.Fatalf("dataSetID=%s want %s", got.DataSetID, dataSetID)
	}
	if len(got.PieceIDs) != 1 || got.PieceIDs[0].Cmp(big.NewInt(7)) != 0 {
		t.Fatalf("pieceIDs=%v want [7]", got.PieceIDs)
	}
}

func TestContextCommit_NewDataSetUsesCreateAndAdd(t *testing.T) {
	info := mustPieceInfo(t)

	fake := &fakeCurioClient{
		createAndAddFn: func(_ context.Context, recordKeeper common.Address, pieces []icurio.AddPieceInput, extraData []byte) (*icurio.CreateDataSetResult, error) {
			if recordKeeper != testRecordKeeper() {
				t.Fatalf("recordKeeper=%s want %s", recordKeeper, testRecordKeeper())
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("unexpected pieces: %+v", pieces)
			}
			if !bytes.Equal(extraData, []byte{0x02}) {
				t.Fatalf("extraData=%x want 02", extraData)
			}
			return &icurio.CreateDataSetResult{TxHash: common.HexToHash("0x02"), StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, statusURL string, _ time.Duration) (*icurio.AddPiecesStatus, error) {
			if statusURL == "" {
				t.Fatal("empty statusURL")
			}
			return &icurio.AddPiecesStatus{
				TxHash:            common.HexToHash("0x02"),
				DataSetID:         55,
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(8)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x02},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got.DataSetID.Cmp(big.NewInt(55)) != 0 {
		t.Fatalf("dataSetID=%s want 55", got.DataSetID)
	}
	if ctx.dataSetID == nil || ctx.dataSetID.Cmp(big.NewInt(55)) != 0 {
		t.Fatalf("context dataSetID=%v want 55", ctx.dataSetID)
	}
}

func TestContextPresignForCommit_NewDataSetCombinedEncoding(t *testing.T) {
	info := mustPieceInfo(t)
	signer := mustTestSigner(t)

	ctx, err := NewContext(testProvider(), &fakeCurioClient{}, signer,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
		WithCDN(true),
		WithDataSetMetadata(map[string]string{"z": "last", "a": "first"}),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	extraData, err := ctx.PresignForCommit(context.Background(), []PieceInput{{
		PieceCID:      info.CIDv2,
		PieceMetadata: map[string]string{"z": "last", "a": "first"},
	}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}

	bytesType, _ := abi.NewType("bytes", "", nil)
	outerArgs := abi.Arguments{{Type: bytesType}, {Type: bytesType}}
	outer, err := outerArgs.Unpack(extraData)
	if err != nil {
		t.Fatalf("unpack outer: %v", err)
	}
	createPayload := outer[0].([]byte)
	addPayload := outer[1].([]byte)
	if len(createPayload) == 0 || len(addPayload) == 0 {
		t.Fatal("combined payload parts must not be empty")
	}

	addressType, _ := abi.NewType("address", "", nil)
	uint256Type, _ := abi.NewType("uint256", "", nil)
	stringArrayType, _ := abi.NewType("string[]", "", nil)
	stringArray2DType, _ := abi.NewType("string[][]", "", nil)

	createArgs := abi.Arguments{
		{Type: addressType},
		{Type: uint256Type},
		{Type: stringArrayType},
		{Type: stringArrayType},
		{Type: bytesType},
	}
	createVals, err := createArgs.Unpack(createPayload)
	if err != nil {
		t.Fatalf("unpack create: %v", err)
	}
	if createVals[0].(common.Address) != testPayer() {
		t.Fatalf("payer=%s want %s", createVals[0], testPayer())
	}
	keys := createVals[2].([]string)
	values := createVals[3].([]string)
	if got := strings.Join(keys, ","); got != "a,withCDN,z" {
		t.Fatalf("dataset keys=%q want a,withCDN,z", got)
	}
	if got := strings.Join(values, ","); got != "first,,last" {
		t.Fatalf("dataset values=%q want first,,last", got)
	}

	addArgs := abi.Arguments{
		{Type: uint256Type},
		{Type: stringArray2DType},
		{Type: stringArray2DType},
		{Type: bytesType},
	}
	addVals, err := addArgs.Unpack(addPayload)
	if err != nil {
		t.Fatalf("unpack add: %v", err)
	}
	pieceKeys := addVals[1].([][]string)
	pieceValues := addVals[2].([][]string)
	if len(pieceKeys) != 1 || len(pieceValues) != 1 {
		t.Fatalf("piece metadata lengths=%d/%d want 1/1", len(pieceKeys), len(pieceValues))
	}
	if got := strings.Join(pieceKeys[0], ","); got != "a,z" {
		t.Fatalf("piece keys=%q want a,z", got)
	}
	if got := strings.Join(pieceValues[0], ","); got != "first,last" {
		t.Fatalf("piece values=%q want first,last", got)
	}
}

func mustPieceInfo(t *testing.T) piece.PieceInfo {
	t.Helper()
	info, err := piece.CalculateFromBytes(bytes.Repeat([]byte("pi"), 128))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	return info
}

func mustTestSigner(t *testing.T) signer.EVMSigner {
	t.Helper()
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	s, err := signer.NewSecp256k1Signer(key)
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	return s
}

func testProvider() Provider {
	return Provider{
		ID:              big.NewInt(1),
		ServiceURL:      "https://sp.example.com",
		ServiceProvider: common.HexToAddress("0x1001"),
		Payee:           common.HexToAddress("0x2002"),
	}
}

func testPayer() common.Address {
	return common.HexToAddress("0x3003")
}

func testRecordKeeper() common.Address {
	return common.HexToAddress("0x4004")
}

type fakeCurioClient struct {
	uploadFromBytesFn     func(context.Context, cid.Cid, []byte) (*icurio.UploadPieceResult, error)
	downloadPieceFn       func(context.Context, cid.Cid) (io.ReadCloser, int64, error)
	waitForPieceFn        func(context.Context, cid.Cid, time.Duration) error
	pullPiecesFn          func(context.Context, icurio.PullRequest) (*icurio.PullResult, error)
	addPiecesFn           func(context.Context, uint64, []icurio.AddPieceInput, []byte) (*icurio.AddPiecesResult, error)
	waitForAddedFn        func(context.Context, string, time.Duration) (*icurio.AddPiecesStatus, error)
	createAndAddFn        func(context.Context, common.Address, []icurio.AddPieceInput, []byte) (*icurio.CreateDataSetResult, error)
	waitForCreateAndAddFn func(context.Context, string, time.Duration) (*icurio.AddPiecesStatus, error)
}

func (f *fakeCurioClient) UploadPieceFromBytes(ctx context.Context, pieceCID cid.Cid, data []byte) (*icurio.UploadPieceResult, error) {
	return f.uploadFromBytesFn(ctx, pieceCID, data)
}

func (f *fakeCurioClient) DownloadPiece(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, int64, error) {
	return f.downloadPieceFn(ctx, pieceCID)
}

func (f *fakeCurioClient) WaitForPieceParked(ctx context.Context, pieceCID cid.Cid, pollInterval time.Duration) error {
	return f.waitForPieceFn(ctx, pieceCID, pollInterval)
}

func (f *fakeCurioClient) WaitForPullComplete(ctx context.Context, req icurio.PullRequest, pollInterval time.Duration, _ func(*icurio.PullResult)) (*icurio.PullResult, error) {
	return f.pullPiecesFn(ctx, req)
}

func (f *fakeCurioClient) AddPieces(ctx context.Context, dataSetID uint64, pieces []icurio.AddPieceInput, extraData []byte) (*icurio.AddPiecesResult, error) {
	return f.addPiecesFn(ctx, dataSetID, pieces, extraData)
}

func (f *fakeCurioClient) WaitForPiecesAdded(ctx context.Context, statusURL string, pollInterval time.Duration) (*icurio.AddPiecesStatus, error) {
	return f.waitForAddedFn(ctx, statusURL, pollInterval)
}

func (f *fakeCurioClient) CreateDataSetAndAddPieces(ctx context.Context, recordKeeper common.Address, pieces []icurio.AddPieceInput, extraData []byte) (*icurio.CreateDataSetResult, error) {
	return f.createAndAddFn(ctx, recordKeeper, pieces, extraData)
}

func (f *fakeCurioClient) WaitForCreateDataSetAndAddPieces(ctx context.Context, statusURL string, pollInterval time.Duration) (*icurio.AddPiecesStatus, error) {
	return f.waitForCreateAndAddFn(ctx, statusURL, pollInterval)
}

// TestContextCommit_ExistingDataSet_LargeIDPreserved proves that a DataSetID
// with the high bit set (value > math.MaxInt64) is not truncated when the
// uint64 is converted to *big.Int.
func TestContextCommit_ExistingDataSet_LargeIDPreserved(t *testing.T) {
	info := mustPieceInfo(t)
	// 1<<63 has the high bit set; int64(1<<63) == math.MinInt64 (wrong sign).
	largeID := uint64(1) << 63
	expectedBig := new(big.Int).SetUint64(largeID)

	fake := &fakeCurioClient{
		addPiecesFn: func(_ context.Context, _ uint64, _ []icurio.AddPieceInput, _ []byte) (*icurio.AddPiecesResult, error) {
			return &icurio.AddPiecesResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*icurio.AddPiecesStatus, error) {
			return &icurio.AddPiecesStatus{
				DataSetID:         largeID,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(1)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
		WithDataSetID(expectedBig),
		WithClientDataSetID(big.NewInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got.DataSetID.Cmp(expectedBig) != 0 {
		t.Fatalf("DataSetID=%s want %s (uint64 high-bit truncation bug)", got.DataSetID, expectedBig)
	}
}

// TestContextCommit_NewDataSet_LargeIDPreserved proves the same for the
// create-and-add path that returns a new DataSetID.
func TestContextCommit_NewDataSet_LargeIDPreserved(t *testing.T) {
	info := mustPieceInfo(t)
	largeID := uint64(math.MaxUint64) // all bits set; int64 cast gives -1
	expectedBig := new(big.Int).SetUint64(largeID)

	fake := &fakeCurioClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []icurio.AddPieceInput, _ []byte) (*icurio.CreateDataSetResult, error) {
			return &icurio.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*icurio.AddPiecesStatus, error) {
			return &icurio.AddPiecesStatus{
				DataSetID:         largeID,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(2)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if got.DataSetID.Cmp(expectedBig) != 0 {
		t.Fatalf("DataSetID=%s want %s (uint64 high-bit truncation bug)", got.DataSetID, expectedBig)
	}
	// The context should also cache the correct value.
	if ctx.dataSetID == nil || ctx.dataSetID.Cmp(expectedBig) != 0 {
		t.Fatalf("cached dataSetID=%v want %s", ctx.dataSetID, expectedBig)
	}
}

// TestContextPull_DataSetIDExceedsUint64ReturnsError proves that Pull returns
// an explicit error when the stored dataSetID exceeds the uint64 range,
// matching the explicit check already present in Commit.
func TestContextPull_DataSetIDExceedsUint64ReturnsError(t *testing.T) {
	info := mustPieceInfo(t)
	// A value that cannot fit in uint64 (requires more than 64 bits).
	overflowID := new(big.Int).Lsh(big.NewInt(1), 64) // 2^64

	ctx, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
		WithDataSetID(overflowID),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{info.CIDv2},
		From:   func(c cid.Cid) string { return "https://primary.example.com/piece/" + c.String() },
	})
	if err == nil {
		t.Fatal("expected error when dataSetID exceeds uint64, got nil")
	}
}

// TestContextPull_ExistingDataSetCarriesRecordKeeper proves that Pull always
// includes RecordKeeper even when a dataSetID is already known, because
// internal/curio.PullPieces requires RecordKeeper in all cases.
func TestContextPull_ExistingDataSetCarriesRecordKeeper(t *testing.T) {
	info := mustPieceInfo(t)
	dataSetID := big.NewInt(42)
	rk := testRecordKeeper()

	var capturedReq icurio.PullRequest
	fake := &fakeCurioClient{
		pullPiecesFn: func(_ context.Context, req icurio.PullRequest) (*icurio.PullResult, error) {
			capturedReq = req
			return &icurio.PullResult{Status: "complete"}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithRecordKeeper(rk),
		WithDataSetID(dataSetID),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{info.CIDv2},
		From:   func(c cid.Cid) string { return "https://primary.example.com/piece/" + c.String() },
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if capturedReq.RecordKeeper != rk {
		t.Fatalf("RecordKeeper=%s want %s (existing-dataset pull must carry RecordKeeper)", capturedReq.RecordKeeper, rk)
	}
	if capturedReq.DataSetID != dataSetID.Uint64() {
		t.Fatalf("DataSetID=%d want %d", capturedReq.DataSetID, dataSetID.Uint64())
	}
}

// TestContextDownload_RequiresPieceCIDv2 proves that the provider-backed
// download path rejects PieceCIDv1 at the storage boundary (curio only
// accepts v2; raw-size is unavailable here so v1->v2 cannot be normalised).
func TestContextDownload_RequiresPieceCIDv2(t *testing.T) {
	info := mustPieceInfo(t)
	ctx, err := NewContext(testProvider(), &fakeCurioClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Download(context.Background(), info.CIDv1)
	if err == nil {
		t.Fatal("expected error: provider-backed Download should reject PieceCIDv1")
	}
}

// TestContextCommit_ConcurrentCommitsNoDuplicateDataSet proves that concurrent
// Commit calls on a single Context create exactly one new dataset (the first)
// and add pieces to it for subsequent calls.
func TestContextCommit_ConcurrentCommitsNoDuplicateDataSet(t *testing.T) {
	info := mustPieceInfo(t)

	var mu sync.Mutex
	createCalls := 0
	addCalls := 0

	fake := &fakeCurioClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []icurio.AddPieceInput, _ []byte) (*icurio.CreateDataSetResult, error) {
			mu.Lock()
			createCalls++
			mu.Unlock()
			return &icurio.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*icurio.AddPiecesStatus, error) {
			return &icurio.AddPiecesStatus{
				DataSetID:         99,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(1)},
			}, nil
		},
		addPiecesFn: func(_ context.Context, _ uint64, _ []icurio.AddPieceInput, _ []byte) (*icurio.AddPiecesResult, error) {
			mu.Lock()
			addCalls++
			mu.Unlock()
			return &icurio.AddPiecesResult{StatusURL: "https://sp.example.com/status2"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*icurio.AddPiecesStatus, error) {
			return &icurio.AddPiecesStatus{
				DataSetID:         99,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(1)},
			}, nil
		},
	}

	storageCtx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(big.NewInt(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	n := 4
	errCh := make(chan error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := storageCtx.Commit(context.Background(), CommitRequest{
				Pieces: []PieceInput{{PieceCID: info.CIDv2}},
			})
			errCh <- err
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
	if createCalls != 1 {
		t.Fatalf("createCalls=%d want 1 (TOCTOU: concurrent Commits must create exactly one dataset)", createCalls)
	}
	if addCalls != n-1 {
		t.Fatalf("addCalls=%d want %d", addCalls, n-1)
	}
}
