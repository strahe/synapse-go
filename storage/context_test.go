package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

func testBigIntPtr(id types.BigInt) *types.BigInt {
	cp := copyBigInt(id)
	return &cp
}

func TestContextStore_UploadsAndWaits(t *testing.T) {
	data := bytes.Repeat([]byte("st"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	fake := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, opts pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("uploaded bytes mismatch")
			}
			if opts.Size != int64(len(data)) {
				t.Fatalf("size=%d want %d", opts.Size, len(data))
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(got))}, nil
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
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Store(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if got.PieceCID != info.CIDv2 {
		t.Fatalf("pieceCID=%s want %s", got.PieceCID, info.CIDv2)
	}
	if got.Size != int64(len(data)) {
		t.Fatalf("size=%d want %d", got.Size, len(data))
	}
}

func TestContextStore_RejectsNonV2PieceCID(t *testing.T) {
	data := bytes.Repeat([]byte("st"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	fake := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, opts pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("uploaded bytes mismatch")
			}
			if opts.Size != int64(len(data)) {
				t.Fatalf("size=%d want %d", opts.Size, len(data))
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv1, Size: int64(len(got))}, nil
		},
		waitForPieceFn: func(_ context.Context, _ cid.Cid, _ time.Duration) error {
			t.Fatal("WaitForPieceParked should not be called for non-v2 PieceCID")
			return nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Store(context.Background(), bytes.NewReader(data), nil)
	if err == nil {
		t.Fatal("expected error for non-v2 PieceCID")
	}
	if !strings.Contains(err.Error(), "invalid PieceCIDv2") {
		t.Fatalf("error = %v, want invalid PieceCIDv2", err)
	}
}

func TestNewContext_RejectsZeroDataSetID(t *testing.T) {
	_, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), WithDataSetID(types.NewBigInt(0)))
	if err == nil {
		t.Fatal("expected error for zero dataSetID")
	}
}

func TestNewContext_AllowsZeroClientDataSetID(t *testing.T) {
	zero := types.NewBigInt(0)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t), WithClientDataSetID(zero))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if ctx.clientDataSetID == nil {
		t.Fatal("clientDataSetID should be stored")
	}
	if !ctx.clientDataSetID.IsZero() {
		t.Fatalf("clientDataSetID = %s, want 0", ctx.clientDataSetID.String())
	}
}

func TestContextPresignForCommit_GeneratesFullWidthClientDataSetID(t *testing.T) {
	info := mustPieceInfo(t)
	fullWidth := append([]byte{0x80}, make([]byte, 31)...)
	nonce := append([]byte{0x01}, make([]byte, 31)...)
	prev := randReader
	randReader = bytes.NewReader(append(fullWidth, nonce...))
	defer func() { randReader = prev }()

	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}}); err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	want, err := types.BigIntFromBig(new(big.Int).SetBytes(fullWidth))
	if err != nil {
		t.Fatal(err)
	}
	if ctx.clientDataSetID == nil {
		t.Fatal("clientDataSetID should be generated")
	}
	if !ctx.clientDataSetID.Equal(want) {
		t.Fatalf("clientDataSetID = %s, want %s", ctx.clientDataSetID.String(), want.String())
	}
}

func TestContextPresignForCommit_RandomFailureReturnsError(t *testing.T) {
	info := mustPieceInfo(t)
	prev := randReader
	randReader = failingReader{err: errors.New("rng down")}
	defer func() { randReader = prev }()

	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}}); err == nil {
		t.Fatal("expected random-source error")
	}
}

func TestContextPull_NewDataSetUsesRecordKeeper(t *testing.T) {
	info := mustPieceInfo(t)
	recordKeeper := testRecordKeeper()
	primaryURL := "https://primary.example.com/piece/" + info.CIDv2.String()

	fake := &fakePDPProviderClient{
		pullPiecesFn: func(_ context.Context, req pdp.PullRequest) (*pdp.PullResult, error) {
			if req.DataSetID != nil {
				t.Fatalf("dataSetID=%s want nil", req.DataSetID.String())
			}
			if req.RecordKeeper != recordKeeper {
				t.Fatalf("recordKeeper=%s want %s", req.RecordKeeper, recordKeeper)
			}
			if len(req.Pieces) != 1 || req.Pieces[0].SourceURL != primaryURL {
				t.Fatalf("unexpected pull pieces: %+v", req.Pieces)
			}
			return &pdp.PullResult{
				Status: pdp.PullStatusComplete,
				Pieces: []pdp.PullPieceStatus{{PieceCID: info.CIDv2.String(), Status: pdp.PullStatusComplete}},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(recordKeeper),
		WithChainID(types.ChainID(314159)),
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
	dataSetID := types.NewBigInt(42)

	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, gotDataSetID types.BigInt, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.AddPiecesResult, error) {
			if !gotDataSetID.Equal(dataSetID) {
				t.Fatalf("dataSetID=%s want %s", gotDataSetID.String(), dataSetID.String())
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("unexpected pieces: %+v", pieces)
			}
			if !bytes.Equal(extraData, []byte{0x01}) {
				t.Fatalf("extraData=%x want 01", extraData)
			}
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x01"), StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, statusURL string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			if statusURL == "" {
				t.Fatal("empty statusURL")
			}
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x01"),
				DataSetID:         dataSetID,
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(7)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(dataSetID),
		WithClientDataSetID(types.NewBigInt(99)),
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
	if !got.DataSetID.Equal(dataSetID) {
		t.Fatalf("dataSetID=%s want %s", got.DataSetID.String(), dataSetID.String())
	}
	if len(got.PieceIDs) != 1 || !got.PieceIDs[0].Equal(types.NewBigInt(7)) {
		t.Fatalf("pieceIDs=%v want [7]", got.PieceIDs)
	}
}

func TestContextCommit_ExistingDataSet_RejectsZeroStatusDataSetID(t *testing.T) {
	info := mustPieceInfo(t)

	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(0),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(7)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
	})
	if err == nil {
		t.Fatal("expected error for zero dataSetID in add-pieces status")
	}
}

func TestContextCommit_ExistingDataSet_RejectsMismatchedStatusDataSetID(t *testing.T) {
	info := mustPieceInfo(t)
	expected := types.NewBigInt(42)

	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(43),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(7)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(expected),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
	})
	if err == nil {
		t.Fatal("expected error for mismatched dataSetID in add-pieces status")
	}
}

func TestContextCommit_ExistingDataSet_RejectsMismatchedConfirmedPieceIDs(t *testing.T) {
	info := mustPieceInfo(t)

	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(42),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(7)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{
			{PieceCID: info.CIDv2},
			{PieceCID: info.CIDv2},
		},
	})
	if err == nil {
		t.Fatal("expected error for mismatched confirmed pieceIDs")
	}
}

func TestContextCommit_NewDataSetUsesCreateAndAdd(t *testing.T) {
	info := mustPieceInfo(t)

	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, recordKeeper common.Address, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.CreateDataSetResult, error) {
			if recordKeeper != testRecordKeeper() {
				t.Fatalf("recordKeeper=%s want %s", recordKeeper, testRecordKeeper())
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("unexpected pieces: %+v", pieces)
			}
			if !bytes.Equal(extraData, []byte{0x02}) {
				t.Fatalf("extraData=%x want 02", extraData)
			}
			return &pdp.CreateDataSetResult{TxHash: common.HexToHash("0x02"), StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, statusURL string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			if statusURL == "" {
				t.Fatal("empty statusURL")
			}
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x02"),
				DataSetID:         types.NewBigInt(55),
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	if !got.DataSetID.Equal(types.NewBigInt(55)) {
		t.Fatalf("dataSetID=%s want 55", got.DataSetID.String())
	}
	if ctx.dataSetID == nil || !ctx.dataSetID.Equal(types.NewBigInt(55)) {
		t.Fatalf("context dataSetID=%v want 55", ctx.dataSetID)
	}
}

func TestContextCreateDataSet_SubmitsWaitsAndBinds(t *testing.T) {
	wantClientID := types.NewBigInt(99)
	dataSetID := types.NewBigInt(77)
	statusURL := "https://sp.example.com/pdp/data-sets/created/0xbeef"
	wantTx := common.HexToHash("0xbeef").Hex()
	var submitted *CreateDataSetSubmission
	var submittedBeforeWait bool
	createCalls := 0

	fake := &fakePDPProviderClient{
		createDataSetFn: func(_ context.Context, recordKeeper common.Address, extraData []byte) (*pdp.CreateDataSetResult, error) {
			createCalls++
			if recordKeeper != testRecordKeeper() {
				t.Fatalf("recordKeeper=%s want %s", recordKeeper, testRecordKeeper())
			}
			if len(extraData) == 0 {
				t.Fatal("extraData should be signed")
			}
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xbeef"),
				StatusURL: statusURL,
			}, nil
		},
		waitForCreatedFn: func(_ context.Context, gotStatusURL string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			if gotStatusURL != statusURL {
				t.Fatalf("statusURL=%q want %q", gotStatusURL, statusURL)
			}
			submittedBeforeWait = submitted != nil
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0xbeef"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(wantClientID),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	result, err := ctx.CreateDataSet(context.Background(), &CreateDataSetOptions{
		OnSubmitted: func(got CreateDataSetSubmission) {
			submitted = &got
		},
	})
	if err != nil {
		t.Fatalf("CreateDataSet: %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("CreateDataSet calls=%d want 1", createCalls)
	}
	if !submittedBeforeWait {
		t.Fatal("OnSubmitted must fire before WaitForDataSetCreated")
	}
	if submitted == nil {
		t.Fatal("OnSubmitted was not called")
	}
	if submitted.TransactionID != wantTx || submitted.StatusURL != statusURL {
		t.Fatalf("submitted=%+v want tx=%s statusURL=%s", submitted, wantTx, statusURL)
	}
	if submitted.ClientDataSetID == nil || !submitted.ClientDataSetID.Equal(wantClientID) {
		t.Fatalf("submitted ClientDataSetID=%v want %s", submitted.ClientDataSetID, wantClientID.String())
	}
	if result.TransactionID != wantTx || !result.DataSetID.Equal(dataSetID) {
		t.Fatalf("result=%+v want tx=%s dataSetID=77", result, wantTx)
	}
	if !result.ClientDataSetID.Equal(wantClientID) {
		t.Fatalf("result ClientDataSetID=%s want %s", result.ClientDataSetID.String(), wantClientID.String())
	}
	if id := ctx.DataSetID(); id == nil || !id.Equal(dataSetID) {
		t.Fatalf("context DataSetID=%v want 77", id)
	}
}

func TestContextCreateDataSetThenCommitUsesAddPieces(t *testing.T) {
	info := mustPieceInfo(t)
	createCalls := 0
	addCalls := 0
	createAndAddCalls := 0

	fake := &fakePDPProviderClient{
		createDataSetFn: func(_ context.Context, _ common.Address, _ []byte) (*pdp.CreateDataSetResult, error) {
			createCalls++
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0x01"),
				StatusURL: "https://sp.example.com/pdp/data-sets/created/0x01",
			}, nil
		},
		waitForCreatedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			dataSetID := types.NewBigInt(77)
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0x01"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
		addPiecesFn: func(_ context.Context, dataSetID types.BigInt, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.AddPiecesResult, error) {
			addCalls++
			if !dataSetID.Equal(types.NewBigInt(77)) {
				t.Fatalf("dataSetID=%s want 77", dataSetID.String())
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("pieces=%+v want %s", pieces, info.CIDv2)
			}
			if len(extraData) == 0 {
				t.Fatal("extraData should be signed")
			}
			return &pdp.AddPiecesResult{
				TxHash:    common.HexToHash("0x02"),
				StatusURL: "https://sp.example.com/pdp/data-sets/77/pieces/added/0x02",
			}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x02"),
				DataSetID:         types.NewBigInt(77),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
			}, nil
		},
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			createAndAddCalls++
			return nil, errors.New("create and add should not be called after CreateDataSet")
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := ctx.CreateDataSet(context.Background(), nil); err != nil {
		t.Fatalf("CreateDataSet: %v", err)
	}
	commit, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if createCalls != 1 || addCalls != 1 || createAndAddCalls != 0 {
		t.Fatalf("calls create=%d add=%d createAndAdd=%d", createCalls, addCalls, createAndAddCalls)
	}
	if !commit.DataSetID.Equal(types.NewBigInt(77)) || len(commit.PieceIDs) != 1 || !commit.PieceIDs[0].Equal(types.NewBigInt(8)) || commit.IsNewDataSet {
		t.Fatalf("commit=%+v want existing dataset 77 piece 8", commit)
	}
}

func TestContextCreateDataSetRejectsIncompleteProviderSubmission(t *testing.T) {
	tests := []struct {
		name   string
		result *pdp.CreateDataSetResult
	}{
		{
			name: "zero transaction",
			result: &pdp.CreateDataSetResult{
				StatusURL: "https://sp.example.com/pdp/data-sets/created/0xbeef",
			},
		},
		{
			name: "empty status url",
			result: &pdp.CreateDataSetResult{
				TxHash: common.HexToHash("0xbeef"),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			waitCalls := 0
			fake := &fakePDPProviderClient{
				createDataSetFn: func(_ context.Context, _ common.Address, _ []byte) (*pdp.CreateDataSetResult, error) {
					return tt.result, nil
				},
				waitForCreatedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
					waitCalls++
					return nil, errors.New("wait should not be called")
				},
			}
			ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
				WithPayer(testPayer()),
				WithRecordKeeper(testRecordKeeper()),
				WithChainID(types.ChainID(314159)),
				WithClientDataSetID(types.NewBigInt(99)),
			)
			if err != nil {
				t.Fatalf("NewContext: %v", err)
			}

			if _, err := ctx.CreateDataSet(context.Background(), nil); err == nil {
				t.Fatal("CreateDataSet returned nil error")
			}
			if waitCalls != 0 {
				t.Fatalf("WaitForDataSetCreated calls=%d want 0", waitCalls)
			}
			if ctx.pendingCreate != nil || ctx.createInFlight {
				t.Fatalf("pendingCreate=%v createInFlight=%v want cleared", ctx.pendingCreate, ctx.createInFlight)
			}
		})
	}
}

func TestContextCreateDataSetRejectsExistingDataSet(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.CreateDataSet(context.Background(), nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CreateDataSet error=%v want ErrInvalidArgument", err)
	}
}

func TestContextCreateDataSetRetryWaitsForPendingSubmission(t *testing.T) {
	waitErr := errors.New("wait failed")
	statusURL := "https://sp.example.com/pdp/data-sets/created/0xbeef"
	createCalls := 0
	waitCalls := 0

	fake := &fakePDPProviderClient{
		createDataSetFn: func(_ context.Context, _ common.Address, _ []byte) (*pdp.CreateDataSetResult, error) {
			createCalls++
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xbeef"),
				StatusURL: statusURL,
			}, nil
		},
		waitForCreatedFn: func(_ context.Context, gotStatusURL string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			if gotStatusURL != statusURL {
				t.Fatalf("statusURL=%q want %q", gotStatusURL, statusURL)
			}
			waitCalls++
			if waitCalls == 1 {
				return nil, waitErr
			}
			dataSetID := types.NewBigInt(77)
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0xbeef"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	if _, err := ctx.CreateDataSet(context.Background(), nil); !errors.Is(err, waitErr) {
		t.Fatalf("first CreateDataSet error=%v want waitErr", err)
	}
	result, err := ctx.CreateDataSet(context.Background(), nil)
	if err != nil {
		t.Fatalf("second CreateDataSet: %v", err)
	}
	if createCalls != 1 || waitCalls != 2 {
		t.Fatalf("calls create=%d wait=%d want create=1 wait=2", createCalls, waitCalls)
	}
	if !result.DataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("DataSetID=%s want 77", result.DataSetID.String())
	}
}

func TestContextCreateDataSetRejectedSubmissionCanRetry(t *testing.T) {
	statusURL1 := "https://sp.example.com/pdp/data-sets/created/0xbeef"
	statusURL2 := "https://sp.example.com/pdp/data-sets/created/0xcafe"
	createCalls := 0

	fake := &fakePDPProviderClient{
		createDataSetFn: func(_ context.Context, _ common.Address, _ []byte) (*pdp.CreateDataSetResult, error) {
			createCalls++
			switch createCalls {
			case 1:
				return &pdp.CreateDataSetResult{
					TxHash:    common.HexToHash("0xbeef"),
					StatusURL: statusURL1,
				}, nil
			case 2:
				return &pdp.CreateDataSetResult{
					TxHash:    common.HexToHash("0xcafe"),
					StatusURL: statusURL2,
				}, nil
			default:
				t.Fatalf("unexpected CreateDataSet call %d", createCalls)
				return nil, nil
			}
		},
		waitForCreatedFn: func(_ context.Context, gotStatusURL string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			switch gotStatusURL {
			case statusURL1:
				return nil, pdp.ErrTxRejected
			case statusURL2:
				dataSetID := types.NewBigInt(77)
				return &pdp.CreateDataSetStatus{
					CreateMessageHash: common.HexToHash("0xcafe"),
					TxStatus:          "confirmed",
					DataSetCreated:    true,
					DataSetID:         &dataSetID,
				}, nil
			default:
				t.Fatalf("unexpected statusURL %q", gotStatusURL)
				return nil, nil
			}
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	if _, err := ctx.CreateDataSet(context.Background(), nil); !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("first CreateDataSet error=%v want ErrTxRejected", err)
	}
	result, err := ctx.CreateDataSet(context.Background(), nil)
	if err != nil {
		t.Fatalf("second CreateDataSet: %v", err)
	}
	if createCalls != 2 {
		t.Fatalf("CreateDataSet calls=%d want 2", createCalls)
	}
	if result.TransactionID != common.HexToHash("0xcafe").Hex() || !result.DataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("result=%+v want tx=0xcafe dataSetID=77", result)
	}
}

func TestContextCreateDataSetInFlightBlocksPresignAndPull(t *testing.T) {
	info := mustPieceInfo(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseCreate := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseCreate()

	fake := &fakePDPProviderClient{
		createDataSetFn: func(ctx context.Context, _ common.Address, _ []byte) (*pdp.CreateDataSetResult, error) {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xbeef"),
				StatusURL: "https://sp.example.com/pdp/data-sets/created/0xbeef",
			}, nil
		},
		waitForCreatedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			dataSetID := types.NewBigInt(77)
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0xbeef"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := ctx.CreateDataSet(context.Background(), nil)
		done <- err
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("CreateDataSet did not enter provider call")
	}
	if _, err := ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("PresignForCommit error=%v want ErrInvalidArgument", err)
	}
	if _, err := ctx.Pull(context.Background(), PullRequest{
		Pieces:    []cid.Cid{info.CIDv2},
		From:      func(cid.Cid) string { return "https://primary.example.com/piece" },
		ExtraData: []byte{0x01},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Pull error=%v want ErrInvalidArgument", err)
	}
	if _, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Commit error=%v want ErrInvalidArgument", err)
	}

	releaseCreate()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CreateDataSet: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("CreateDataSet did not finish")
	}
}

func TestContextCreateDataSetWaitDoesNotBlockCommit(t *testing.T) {
	info := mustPieceInfo(t)
	waiting := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseWait := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseWait()

	fake := &fakePDPProviderClient{
		createDataSetFn: func(_ context.Context, _ common.Address, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xbeef"),
				StatusURL: "https://sp.example.com/pdp/data-sets/created/0xbeef",
			}, nil
		},
		waitForCreatedFn: func(ctx context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			close(waiting)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			dataSetID := types.NewBigInt(77)
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0xbeef"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := ctx.CreateDataSet(context.Background(), nil)
		done <- err
	}()

	select {
	case <-waiting:
	case <-time.After(time.Second):
		t.Fatal("CreateDataSet did not enter wait")
	}

	commitDone := make(chan error, 1)
	go func() {
		_, err := ctx.Commit(context.Background(), CommitRequest{
			Pieces: []PieceInput{{PieceCID: info.CIDv2}},
		})
		commitDone <- err
	}()
	select {
	case err := <-commitDone:
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("Commit error=%v want ErrInvalidArgument", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Commit blocked while CreateDataSet was waiting")
	}

	releaseWait()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("CreateDataSet: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("CreateDataSet did not finish")
	}
}

func TestContextPendingCreateBlocksOtherWritePaths(t *testing.T) {
	info := mustPieceInfo(t)
	fake := &fakePDPProviderClient{
		createDataSetFn: func(_ context.Context, _ common.Address, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xbeef"),
				StatusURL: "https://sp.example.com/pdp/data-sets/created/0xbeef",
			}, nil
		},
		waitForCreatedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			return nil, errors.New("wait failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	if _, err := ctx.CreateDataSet(context.Background(), nil); err == nil {
		t.Fatal("expected initial CreateDataSet wait error")
	}

	if _, err := ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("PresignForCommit error=%v want ErrInvalidArgument", err)
	}
	if _, err := ctx.Pull(context.Background(), PullRequest{
		Pieces:    []cid.Cid{info.CIDv2},
		From:      func(cid.Cid) string { return "https://primary.example.com/piece" },
		ExtraData: []byte{0x01},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Pull error=%v want ErrInvalidArgument", err)
	}
	if _, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("Commit error=%v want ErrInvalidArgument", err)
	}
}

func TestContextWaitForDataSetCreatedBindsSubmission(t *testing.T) {
	wantClientID := types.NewBigInt(99)
	dataSetID := types.NewBigInt(77)
	submission := CreateDataSetSubmission{
		TransactionID:   common.HexToHash("0xbeef").Hex(),
		StatusURL:       "https://sp.example.com/pdp/data-sets/created/0xbeef",
		ClientDataSetID: testBigIntPtr(wantClientID),
	}
	fake := &fakePDPProviderClient{
		waitForCreatedFn: func(_ context.Context, gotStatusURL string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			if gotStatusURL != submission.StatusURL {
				t.Fatalf("statusURL=%q want %q", gotStatusURL, submission.StatusURL)
			}
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0xbeef"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	result, err := ctx.WaitForDataSetCreated(context.Background(), submission)
	if err != nil {
		t.Fatalf("WaitForDataSetCreated: %v", err)
	}
	if result.TransactionID != submission.TransactionID || !result.DataSetID.Equal(dataSetID) {
		t.Fatalf("result=%+v want tx=%s dataSetID=77", result, submission.TransactionID)
	}
	if !result.ClientDataSetID.Equal(wantClientID) {
		t.Fatalf("ClientDataSetID=%s want %s", result.ClientDataSetID.String(), wantClientID.String())
	}
	if id := ctx.DataSetID(); id == nil || !id.Equal(dataSetID) {
		t.Fatalf("context DataSetID=%v want 77", id)
	}
}

func TestContextWaitForDataSetCreatedRejectsInvalidTransactionID(t *testing.T) {
	tests := []struct {
		name          string
		transactionID string
	}{
		{name: "empty", transactionID: ""},
		{name: "short", transactionID: "0xbeef"},
		{name: "zero", transactionID: common.Hash{}.Hex()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			waitCalls := 0
			fake := &fakePDPProviderClient{
				waitForCreatedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
					waitCalls++
					return nil, errors.New("wait should not be called")
				},
			}
			ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
				WithPayer(testPayer()),
				WithRecordKeeper(testRecordKeeper()),
				WithChainID(types.ChainID(314159)),
				WithClientDataSetID(types.NewBigInt(99)),
			)
			if err != nil {
				t.Fatalf("NewContext: %v", err)
			}

			_, err = ctx.WaitForDataSetCreated(context.Background(), CreateDataSetSubmission{
				TransactionID:   tt.transactionID,
				StatusURL:       "https://sp.example.com/pdp/data-sets/created/0xbeef",
				ClientDataSetID: testBigIntPtr(types.NewBigInt(99)),
			})
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("WaitForDataSetCreated error=%v want ErrInvalidArgument", err)
			}
			if waitCalls != 0 {
				t.Fatalf("WaitForDataSetCreated wait calls=%d want 0", waitCalls)
			}
		})
	}
}

func TestContextWaitForDataSetCreatedRejectsMissingClientDataSetID(t *testing.T) {
	waitCalls := 0
	fake := &fakePDPProviderClient{
		waitForCreatedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			waitCalls++
			return nil, errors.New("wait should not be called")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.WaitForDataSetCreated(context.Background(), CreateDataSetSubmission{
		TransactionID: common.HexToHash("0xbeef").Hex(),
		StatusURL:     "https://sp.example.com/pdp/data-sets/created/0xbeef",
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("WaitForDataSetCreated error=%v want ErrInvalidArgument", err)
	}
	if waitCalls != 0 {
		t.Fatalf("WaitForDataSetCreated wait calls=%d want 0", waitCalls)
	}
}

func TestContextWaitForDataSetCreatedRejectsMismatchedExistingDataSet(t *testing.T) {
	fake := &fakePDPProviderClient{
		waitForCreatedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			dataSetID := types.NewBigInt(77)
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0xbeef"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.WaitForDataSetCreated(context.Background(), CreateDataSetSubmission{
		TransactionID:   common.HexToHash("0xbeef").Hex(),
		StatusURL:       "https://sp.example.com/pdp/data-sets/created/0xbeef",
		ClientDataSetID: testBigIntPtr(types.NewBigInt(99)),
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("WaitForDataSetCreated error=%v want ErrInvalidArgument", err)
	}
}

func TestContextWaitForDataSetCreatedRejectsMismatchedTransactionIDWithoutPoisoningContext(t *testing.T) {
	waitCalls := 0
	fake := &fakePDPProviderClient{
		waitForCreatedFn: func(_ context.Context, statusURL string, _ time.Duration) (*pdp.CreateDataSetStatus, error) {
			waitCalls++
			switch statusURL {
			case "https://sp.example.com/pdp/data-sets/created/0xbeef":
			case "https://sp.example.com/pdp/data-sets/created/0xcafe":
			default:
				t.Fatalf("statusURL=%q", statusURL)
			}
			dataSetID := types.NewBigInt(77)
			return &pdp.CreateDataSetStatus{
				CreateMessageHash: common.HexToHash("0xcafe"),
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				DataSetID:         &dataSetID,
			}, nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.WaitForDataSetCreated(context.Background(), CreateDataSetSubmission{
		TransactionID:   common.HexToHash("0xbeef").Hex(),
		StatusURL:       "https://sp.example.com/pdp/data-sets/created/0xbeef",
		ClientDataSetID: testBigIntPtr(types.NewBigInt(99)),
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("WaitForDataSetCreated error=%v want ErrInvalidArgument", err)
	}
	if id := ctx.DataSetID(); id != nil {
		t.Fatalf("context DataSetID=%v want nil", id)
	}

	result, err := ctx.WaitForDataSetCreated(context.Background(), CreateDataSetSubmission{
		TransactionID:   common.HexToHash("0xcafe").Hex(),
		StatusURL:       "https://sp.example.com/pdp/data-sets/created/0xcafe",
		ClientDataSetID: testBigIntPtr(types.NewBigInt(100)),
	})
	if err != nil {
		t.Fatalf("WaitForDataSetCreated retry: %v", err)
	}
	if !result.DataSetID.Equal(types.NewBigInt(77)) {
		t.Fatalf("retry DataSetID=%s want 77", result.DataSetID.String())
	}
	if waitCalls != 2 {
		t.Fatalf("wait calls=%d want 2", waitCalls)
	}
}

func TestContextCommit_NewDataSet_RejectsMismatchedConfirmedPieceIDs(t *testing.T) {
	info := mustPieceInfo(t)

	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(55),
				PieceCount:        2,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{
			{PieceCID: info.CIDv2},
			{PieceCID: info.CIDv2},
		},
	})
	if err == nil {
		t.Fatal("expected error for mismatched confirmed pieceIDs")
	}
}

func TestContextPresignForCommit_NewDataSetCombinedEncoding(t *testing.T) {
	info := mustPieceInfo(t)
	signer := mustTestSigner(t)

	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, signer,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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

func TestContextPresignForCommit_ExistingDataSetAddPiecesEncoding(t *testing.T) {
	info := mustPieceInfo(t)
	signer := mustTestSigner(t)

	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, signer,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	extraData, err := ctx.PresignForCommit(context.Background(), []PieceInput{{
		PieceCID:      info.CIDv2,
		PieceMetadata: map[string]string{"k": "v"},
	}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}

	bytesType, _ := abi.NewType("bytes", "", nil)
	uint256Type, _ := abi.NewType("uint256", "", nil)
	stringArray2DType, _ := abi.NewType("string[][]", "", nil)
	addArgs := abi.Arguments{
		{Type: uint256Type},
		{Type: stringArray2DType},
		{Type: stringArray2DType},
		{Type: bytesType},
	}
	vals, err := addArgs.Unpack(extraData)
	if err != nil {
		t.Fatalf("unpack add pieces: %v", err)
	}
	nonce := vals[0].(*big.Int)
	if nonce == nil || nonce.Sign() == 0 {
		t.Fatal("nonce must be non-zero")
	}
	sig := vals[3].([]byte)
	if len(sig) != 65 {
		t.Fatalf("sig length=%d want 65", len(sig))
	}
	pieceKeys := vals[1].([][]string)
	pieceValues := vals[2].([][]string)
	if len(pieceKeys) != 1 || len(pieceValues) != 1 {
		t.Fatalf("metadata array lengths=%d/%d want 1/1", len(pieceKeys), len(pieceValues))
	}
	if pieceKeys[0][0] != "k" || pieceValues[0][0] != "v" {
		t.Fatalf("metadata=%v/%v want [k]/[v]", pieceKeys[0], pieceValues[0])
	}
}

func TestContextPresignForCommit_CtxCancelled(t *testing.T) {
	info := mustPieceInfo(t)
	sctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sctx.PresignForCommit(cancelled, []PieceInput{{PieceCID: info.CIDv2}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}
}

func TestContextPresignForCommit_InvalidArgumentPrecedesCtxCancelled(t *testing.T) {
	sctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = sctx.PresignForCommit(cancelled, nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
}

// TestContextPresignForCommit_ExistingDataSetRequiresClientDataSetID
// locks the PresignForCommit guard that rejects existing-dataset
// contexts constructed without WithClientDataSetID. The contract
// reconstructs the EIP-712 hash from the clientDataSetId it stored at
// creation time, so a randomly-minted value would produce a signature
// the verifier rejects.
func TestContextPresignForCommit_ExistingDataSetRequiresClientDataSetID(t *testing.T) {
	info := mustPieceInfo(t)
	sctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = sctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("err=%v want ErrInvalidArgument", err)
	}
	if !strings.Contains(err.Error(), "clientDataSetID is required") {
		t.Fatalf("error should mention clientDataSetID requirement; got: %v", err)
	}
}

func TestContextPresignForCommit_WrappedSignerUnsupported(t *testing.T) {
	info := mustPieceInfo(t)
	// Wrap the signer in a no-op type that only implements EVMSigner via embedding, not hashSigner
	type wrappedSigner struct{ signer.EVMSigner }
	ws := &wrappedSigner{mustTestSigner(t)}

	// Existing-dataset path
	sctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, ws,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(1)),
		WithClientDataSetID(types.NewBigInt(7)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = sctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if !errors.Is(err, signer.ErrUnsupportedSigner) {
		t.Fatalf("err=%v want ErrUnsupportedSigner (existing-dataset)", err)
	}
	if err == nil || !strings.Contains(err.Error(), "wrapped/decorated EVMSigner values are unsupported") {
		t.Fatalf("error message must mention unsupported wrapped/decorated EVMSigner policy (existing-dataset), got: %v", err)
	}

	// New-dataset path
	sctx2, err := NewContext(testProvider(), &fakePDPProviderClient{}, ws,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext (new-dataset): %v", err)
	}
	_, err = sctx2.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if !errors.Is(err, signer.ErrUnsupportedSigner) {
		t.Fatalf("err=%v want ErrUnsupportedSigner (new-dataset)", err)
	}
	if err == nil || !strings.Contains(err.Error(), "wrapped/decorated EVMSigner values are unsupported") {
		t.Fatalf("error message must mention unsupported wrapped/decorated EVMSigner policy (new-dataset), got: %v", err)
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
		ID:              types.NewBigInt(1),
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

type fakePDPProviderClient struct {
	uploadStreamingFn     func(context.Context, io.Reader, pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error)
	downloadPieceFn       func(context.Context, cid.Cid) (io.ReadCloser, int64, error)
	waitForPieceFn        func(context.Context, cid.Cid, time.Duration) error
	pullPiecesFn          func(context.Context, pdp.PullRequest) (*pdp.PullResult, error)
	pullPiecesFnWithCb    func(context.Context, pdp.PullRequest, func(*pdp.PullResult)) (*pdp.PullResult, error)
	addPiecesFn           func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error)
	waitForAddedFn        func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error)
	createDataSetFn       func(context.Context, common.Address, []byte) (*pdp.CreateDataSetResult, error)
	waitForCreatedFn      func(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error)
	createAndAddFn        func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error)
	waitForCreateAndAddFn func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error)
	scheduleDeletionFn    func(context.Context, types.BigInt, types.BigInt, []byte) (common.Hash, error)
}

type failingReader struct {
	err error
}

func (r failingReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (f *fakePDPProviderClient) UploadPieceStreaming(ctx context.Context, r io.Reader, opts pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
	return f.uploadStreamingFn(ctx, r, opts)
}

func (f *fakePDPProviderClient) DownloadPiece(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, int64, error) {
	return f.downloadPieceFn(ctx, pieceCID)
}

func (f *fakePDPProviderClient) WaitForPieceParked(ctx context.Context, pieceCID cid.Cid, pollInterval time.Duration) error {
	return f.waitForPieceFn(ctx, pieceCID, pollInterval)
}

func (f *fakePDPProviderClient) WaitForPullComplete(ctx context.Context, req pdp.PullRequest, pollInterval time.Duration, cb func(*pdp.PullResult)) (*pdp.PullResult, error) {
	if f.pullPiecesFnWithCb != nil {
		return f.pullPiecesFnWithCb(ctx, req, cb)
	}
	return f.pullPiecesFn(ctx, req)
}

func (f *fakePDPProviderClient) AddPieces(ctx context.Context, dataSetID types.BigInt, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.AddPiecesResult, error) {
	return f.addPiecesFn(ctx, dataSetID, pieces, extraData)
}

func (f *fakePDPProviderClient) WaitForPiecesAdded(ctx context.Context, statusURL string, pollInterval time.Duration) (*pdp.AddPiecesStatus, error) {
	return f.waitForAddedFn(ctx, statusURL, pollInterval)
}

func (f *fakePDPProviderClient) CreateDataSet(ctx context.Context, recordKeeper common.Address, extraData []byte) (*pdp.CreateDataSetResult, error) {
	return f.createDataSetFn(ctx, recordKeeper, extraData)
}

func (f *fakePDPProviderClient) WaitForDataSetCreated(ctx context.Context, statusURL string, pollInterval time.Duration) (*pdp.CreateDataSetStatus, error) {
	return f.waitForCreatedFn(ctx, statusURL, pollInterval)
}

func (f *fakePDPProviderClient) CreateDataSetAndAddPieces(ctx context.Context, recordKeeper common.Address, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.CreateDataSetResult, error) {
	return f.createAndAddFn(ctx, recordKeeper, pieces, extraData)
}

func (f *fakePDPProviderClient) WaitForCreateDataSetAndAddPieces(ctx context.Context, statusURL string, pollInterval time.Duration) (*pdp.AddPiecesStatus, error) {
	return f.waitForCreateAndAddFn(ctx, statusURL, pollInterval)
}

func (f *fakePDPProviderClient) SchedulePieceDeletion(ctx context.Context, dataSetID, pieceID types.BigInt, extraData []byte) (common.Hash, error) {
	if f.scheduleDeletionFn == nil {
		return common.Hash{}, fmt.Errorf("fakePDPProviderClient.SchedulePieceDeletion: not configured")
	}
	return f.scheduleDeletionFn(ctx, dataSetID, pieceID, extraData)
}

// TestContextCommit_ExistingDataSet_LargeIDPreserved proves that a DataSetID
// with the high bit set (value > math.MaxInt64) is not truncated when moved
// through the PDP/context boundary.
func TestContextCommit_ExistingDataSet_LargeIDPreserved(t *testing.T) {
	info := mustPieceInfo(t)
	largeID := uint64(1) << 63
	expected := types.NewBigInt(largeID)

	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         expected,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(expected),
		WithClientDataSetID(types.NewBigInt(99)),
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
	if !got.DataSetID.Equal(expected) {
		t.Fatalf("DataSetID=%s want %s (uint64 high-bit truncation bug)", got.DataSetID.String(), expected.String())
	}
}

// TestContextCommit_NewDataSet_LargeIDPreserved proves the same for the
// create-and-add path that returns a new DataSetID.
func TestContextCommit_NewDataSet_LargeIDPreserved(t *testing.T) {
	info := mustPieceInfo(t)
	largeID := uint64(math.MaxUint64) // all bits set; int64 cast gives -1
	expected := types.NewBigInt(largeID)

	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         expected,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(2)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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
	if !got.DataSetID.Equal(expected) {
		t.Fatalf("DataSetID=%s want %s (uint64 high-bit truncation bug)", got.DataSetID.String(), expected.String())
	}
	// The context should also cache the correct value.
	if ctx.dataSetID == nil || !ctx.dataSetID.Equal(expected) {
		t.Fatalf("cached dataSetID=%v want %s", ctx.dataSetID, expected.String())
	}
}

// TestContextPull_ExistingDataSetCarriesRecordKeeper proves that Pull always
// includes RecordKeeper even when a dataSetID is already known, because
// pdp.PullPieces requires RecordKeeper in all cases.
func TestContextPull_ExistingDataSetCarriesRecordKeeper(t *testing.T) {
	info := mustPieceInfo(t)
	dataSetID := types.NewBigInt(42)
	rk := testRecordKeeper()

	var capturedReq pdp.PullRequest
	fake := &fakePDPProviderClient{
		pullPiecesFn: func(_ context.Context, req pdp.PullRequest) (*pdp.PullResult, error) {
			capturedReq = req
			return &pdp.PullResult{Status: "complete"}, nil
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
	if capturedReq.DataSetID == nil || !capturedReq.DataSetID.Equal(dataSetID) {
		t.Fatalf("DataSetID=%v want %s", capturedReq.DataSetID, dataSetID.String())
	}
}

// TestContextDownload_RequiresPieceCIDv2 proves that the provider-backed
// download path rejects PieceCIDv1 at the storage boundary because the PDP
// provider only accepts v2 and raw size is unavailable here.
func TestContextDownload_RequiresPieceCIDv2(t *testing.T) {
	info := mustPieceInfo(t)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
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

	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			mu.Lock()
			createCalls++
			mu.Unlock()
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(99),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)},
			}, nil
		},
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			mu.Lock()
			addCalls++
			mu.Unlock()
			return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/status2"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(99),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)},
			}, nil
		},
	}

	storageCtx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
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

func TestContextPieceURL(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) { return nil, 0, nil },
	}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	info := mustPieceInfo(t)
	got := ctx.PieceURL(info.CIDv2)
	want := "https://sp.example.com/piece/" + info.CIDv2.String()
	if got != want {
		t.Fatalf("PieceURL()=%q want %q", got, want)
	}
}

func TestContextProviderID(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) { return nil, 0, nil },
	}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got := ctx.ProviderID()
	if !got.Equal(types.NewBigInt(1)) {
		t.Fatalf("ProviderID()=%s want 1", got.String())
	}
}

func TestContextServiceURL(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{
		downloadPieceFn: func(context.Context, cid.Cid) (io.ReadCloser, int64, error) { return nil, 0, nil },
	}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	if got := ctx.ServiceURL(); got != "https://sp.example.com" {
		t.Fatalf("ServiceURL()=%q want https://sp.example.com", got)
	}
}

func TestPieceURLFor_InvalidBaseURL(t *testing.T) {
	ctx := &Context{
		provider: Provider{
			ID:         types.NewBigInt(1),
			ServiceURL: "://invalid-url",
		},
	}
	info := mustPieceInfo(t)
	// Invalid URL should fallback to returning the raw ServiceURL
	got := ctx.pieceURLFor(info.CIDv2)
	if got != "://invalid-url" {
		t.Fatalf("pieceURLFor with invalid URL=%q want raw ServiceURL", got)
	}
}

func TestContextNewContext_ValidationErrors(t *testing.T) {
	s := mustTestSigner(t)
	tests := []struct {
		name     string
		provider Provider
		client   PDPProviderClient
		wantErr  string
	}{
		{
			name:     "nil provider ID",
			provider: Provider{ServiceURL: "https://sp.example.com"},
			client:   &fakePDPProviderClient{},
			wantErr:  "zero provider ID",
		},
		{
			name:     "empty service URL",
			provider: Provider{ID: types.NewBigInt(1)},
			client:   &fakePDPProviderClient{},
			wantErr:  "empty provider service URL",
		},
		{
			name:     "nil client",
			provider: Provider{ID: types.NewBigInt(1), ServiceURL: "https://sp.example.com"},
			client:   nil,
			wantErr:  "nil PDP client",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewContext(tt.provider, tt.client, s)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err=%q, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestContextStore_NilReader(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Store(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil reader")
	}
}

func TestContextPresignForCommit_NilSigner(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, nil,
		WithChainID(types.ChainID(1)),
		WithRecordKeeper(testRecordKeeper()),
		WithPayer(testPayer()),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	info := mustPieceInfo(t)
	_, err = ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestContextPresignForCommit_NilChainID(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithRecordKeeper(testRecordKeeper()),
		WithPayer(testPayer()),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	info := mustPieceInfo(t)
	_, err = ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err == nil {
		t.Fatal("expected error for nil chainID")
	}
}

func TestContextPresignForCommit_NoPieces(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.PresignForCommit(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for no pieces")
	}
}

func TestContextCommit_NoPieces(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Commit(context.Background(), CommitRequest{})
	if err == nil {
		t.Fatal("expected error for no pieces")
	}
}

func TestContextPull_NoPieces(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Pull(context.Background(), PullRequest{})
	if err == nil {
		t.Fatal("expected error for no pieces")
	}
}

func TestContextPull_NilFrom(t *testing.T) {
	info := mustPieceInfo(t)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Pull(context.Background(), PullRequest{Pieces: []cid.Cid{info.CIDv2}})
	if err == nil {
		t.Fatal("expected error for nil From")
	}
}

func TestContextCommit_ZeroDataSetIDFromServer(t *testing.T) {
	info := mustPieceInfo(t)
	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{DataSetID: types.NewBigInt(0), ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if err == nil {
		t.Fatal("expected error for zero dataSetID from server")
	}
}

func TestContextPresignForCommit_ZeroRecordKeeper(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithChainID(types.ChainID(1)),
		WithPayer(testPayer()),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	info := mustPieceInfo(t)
	_, err = ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err == nil {
		t.Fatal("expected error for zero recordKeeper")
	}
}

func TestContextPresignForCommit_ZeroPayer(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithChainID(types.ChainID(1)),
		WithRecordKeeper(testRecordKeeper()),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	info := mustPieceInfo(t)
	_, err = ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err == nil {
		t.Fatal("expected error for zero payer")
	}
}

func TestMetadataEntries_KeyTooLong(t *testing.T) {
	longKey := strings.Repeat("k", maxMetadataKeyLength+1)
	_, err := metadataEntries(map[string]string{longKey: "v"}, 10)
	if err == nil {
		t.Fatal("expected error for key too long")
	}
}

func TestMetadataEntries_ValueTooLong(t *testing.T) {
	longValue := strings.Repeat("v", maxMetadataValueLength+1)
	_, err := metadataEntries(map[string]string{"k": longValue}, 10)
	if err == nil {
		t.Fatal("expected error for value too long")
	}
}

func TestMetadataEntries_TooManyKeys(t *testing.T) {
	m := make(map[string]string)
	for i := 0; i < 11; i++ {
		m[fmt.Sprintf("k%d", i)] = "v"
	}
	_, err := metadataEntries(m, 10)
	if err == nil {
		t.Fatal("expected error for too many keys")
	}
}

func TestContextStore_UploadError(t *testing.T) {
	data := bytes.Repeat([]byte("ue"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	fake := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, _ io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			return nil, errors.New("upload failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Store(context.Background(), bytes.NewReader(data), &StoreOptions{PieceCID: info.CIDv2})
	if err == nil {
		t.Fatal("expected error for upload failure")
	}
}

func TestContextStore_WaitError(t *testing.T) {
	data := bytes.Repeat([]byte("we"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	fake := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			_, _ = io.Copy(io.Discard, r)
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(_ context.Context, _ cid.Cid, _ time.Duration) error {
			return errors.New("wait failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Store(context.Background(), bytes.NewReader(data), nil)
	if err == nil {
		t.Fatal("expected error for wait failure")
	}
}

func TestContextUpload_AcceptsZeroPieceID(t *testing.T) {
	data := bytes.Repeat([]byte("up"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	fake := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			_, _ = io.Copy(io.Discard, r)
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(_ context.Context, _ cid.Cid, _ time.Duration) error { return nil },
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(42),
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(0)},
				TxHash:            common.HexToHash("0x1234"),
			}, nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got.SuccessCount() != 1 {
		t.Fatalf("success count=%d want 1", got.SuccessCount())
	}
	if len(got.Copies) != 1 || !got.Copies[0].PieceID.IsZero() {
		t.Fatalf("copies=%+v want pieceID 0", got.Copies)
	}
}

func TestContextPull_EmptySourceURL(t *testing.T) {
	info := mustPieceInfo(t)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{info.CIDv2},
		From:   func(cid.Cid) string { return "" },
	})
	if err == nil {
		t.Fatal("expected error for empty source URL")
	}
}

func TestContextPull_UndefinedPieceCID(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{cid.Undef},
		From:   func(cid.Cid) string { return "https://example.com" },
	})
	if err == nil {
		t.Fatal("expected error for undefined pieceCID")
	}
}

func TestContextPresignForCommit_ExistingDataSet(t *testing.T) {
	info := mustPieceInfo(t)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	extra, err := ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	if len(extra) == 0 {
		t.Fatal("expected non-empty extraData for existing dataset path")
	}
	// ABI-encoded data for add-pieces should be well-formed (at least one 32-byte word).
	if len(extra) < 32 {
		t.Fatalf("extraData too short (%d bytes), want ≥32", len(extra))
	}
}

func TestContextPresignForCommit_ExistingDataSet_TracksAddOnlyPayload(t *testing.T) {
	info := mustPieceInfo(t)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	payload, err := ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	if got := len(ctx.presignedKinds); got != 1 {
		t.Fatalf("tracked presigns=%d want 1 for existing dataset path", got)
	}
	if got := ctx.presignedKinds[presignedExtraDataKey(payload)]; got != commitExtraDataAddOnly {
		t.Fatalf("kind=%d want %d (AddOnly)", got, commitExtraDataAddOnly)
	}
}

func TestContextPresignForCommit_UndefinedPieceCID(t *testing.T) {
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: cid.Undef}})
	if err == nil {
		t.Fatal("expected error for undefined pieceCID")
	}
}

func TestContextCommit_AddPiecesError(t *testing.T) {
	info := mustPieceInfo(t)
	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			return nil, errors.New("add pieces failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if err == nil {
		t.Fatal("expected error for add pieces failure")
	}
}

func TestContextCommit_RefreshesStalePresignedExtraData(t *testing.T) {
	info1 := mustPieceInfo(t)
	info2, err := piece.CalculateFromBytes(bytes.Repeat([]byte("p2"), 128))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var staleExtra []byte
	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.CreateDataSetResult, error) {
			if len(pieces) != 1 || pieces[0].PieceCID != info1.CIDv2 {
				t.Fatalf("unexpected create pieces: %+v", pieces)
			}
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/create"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x01"),
				DataSetID:         types.NewBigInt(55),
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
			}, nil
		},
		addPiecesFn: func(_ context.Context, gotDataSetID types.BigInt, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.AddPiecesResult, error) {
			if !gotDataSetID.Equal(types.NewBigInt(55)) {
				t.Fatalf("dataSetID=%s want 55", gotDataSetID.String())
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info2.CIDv2 {
				t.Fatalf("unexpected add pieces: %+v", pieces)
			}
			if bytes.Equal(extraData, staleExtra) {
				return nil, errors.New("stale extraData used")
			}
			return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/add"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x02"),
				DataSetID:         types.NewBigInt(55),
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(9)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	firstExtra, err := ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info1.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit first: %v", err)
	}
	staleExtra, err = ctx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info2.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit second: %v", err)
	}

	if _, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info1.CIDv2}},
		ExtraData: firstExtra,
	}); err != nil {
		t.Fatalf("Commit first: %v", err)
	}
	if got := len(ctx.presignedKinds); got != 1 {
		t.Fatalf("tracked presigns after create=%d want 1 outstanding stale payload", got)
	}

	if _, err := ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info2.CIDv2}},
		ExtraData: staleExtra,
	}); err != nil {
		t.Fatalf("Commit second with stale extraData: %v", err)
	}
	if got := len(ctx.presignedKinds); got != 0 {
		t.Fatalf("tracked presigns after stale refresh=%d want 0", got)
	}
}

// TestContextPresignForCommit_ConcurrentWithReaders exercises the lock-split
// path in PresignForCommit: signing runs outside c.mu so concurrent callers
// that briefly acquire c.mu (presignedExtraDataIsStale, forgetPresignedExtraData)
// must not race with signing. Run under -race to validate.
func TestContextPresignForCommit_ConcurrentWithReaders(t *testing.T) {
	info := mustPieceInfo(t)
	ctx, err := NewContext(testProvider(), &fakePDPProviderClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	pieces := make([]PieceInput, 8)
	for i := range pieces {
		pieces[i] = PieceInput{PieceCID: info.CIDv2}
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})
	probe := []byte{0xde, 0xad}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = ctx.presignedExtraDataIsStale(probe)
				ctx.forgetPresignedExtraData(probe)
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				if _, err := ctx.PresignForCommit(context.Background(), pieces); err != nil {
					t.Errorf("PresignForCommit: %v", err)
					return
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	close(stop)
	<-done
}

func TestContextDataSetID_ConcurrentWithCommit(t *testing.T) {
	info := mustPieceInfo(t)
	releaseCreate := make(chan struct{})
	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			<-releaseCreate
			return &pdp.AddPiecesStatus{
				DataSetID:         types.NewBigInt(42),
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(11)},
				TxHash:            common.HexToHash("0x1234"),
			}, nil
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	stopReaders := make(chan struct{})
	var readers sync.WaitGroup
	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stopReaders:
					return
				default:
				}
				_ = ctx.DataSetID()
			}
		}()
	}

	commitDone := make(chan error, 1)
	go func() {
		_, err := ctx.Commit(context.Background(), CommitRequest{
			Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
			ExtraData: []byte{0x01},
		})
		commitDone <- err
	}()

	time.Sleep(20 * time.Millisecond)
	close(releaseCreate)
	if err := <-commitDone; err != nil {
		t.Fatalf("Commit: %v", err)
	}
	close(stopReaders)
	readers.Wait()
}

func TestContextCommit_WaitAddPiecesError(t *testing.T) {
	info := mustPieceInfo(t)
	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return nil, errors.New("wait failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(99)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if err == nil {
		t.Fatal("expected error for wait add pieces failure")
	}
}

func TestContextCommit_CreateAndAddError(t *testing.T) {
	info := mustPieceInfo(t)
	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return nil, errors.New("create failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if err == nil {
		t.Fatal("expected error for create and add failure")
	}
}

func TestContextCommit_WaitCreateAndAddError(t *testing.T) {
	info := mustPieceInfo(t)
	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return nil, errors.New("wait create failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if err == nil {
		t.Fatal("expected error for wait create and add failure")
	}
}

func TestContextDownload_ClientError(t *testing.T) {
	info := mustPieceInfo(t)
	fake := &fakePDPProviderClient{
		downloadPieceFn: func(_ context.Context, _ cid.Cid) (io.ReadCloser, int64, error) {
			return nil, 0, errors.New("download failed")
		},
	}
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	_, err = ctx.Download(context.Background(), info.CIDv2)
	if err == nil {
		t.Fatal("expected error for download failure")
	}
}

// TestContextPull_OnProgress proves that PullRequest.OnProgress fires for each
// piece status update delivered by WaitForPullComplete, with the caller-visible
// cid.Cid value and the corresponding PullStatus. Unknown server-side piece IDs
// must be silently ignored.
func TestContextPull_OnProgress(t *testing.T) {
	info := mustPieceInfo(t)

	// The fake delivers two progress snapshots then the final result.
	// The second snapshot includes an unknown piece ID that must be ignored.
	fake := &fakePDPProviderClient{
		pullPiecesFnWithCb: func(_ context.Context, req pdp.PullRequest, cb func(*pdp.PullResult)) (*pdp.PullResult, error) {
			if cb != nil {
				cb(&pdp.PullResult{
					Status: pdp.PullStatusInProgress,
					Pieces: []pdp.PullPieceStatus{
						{PieceCID: info.CIDv2.String(), Status: pdp.PullStatusInProgress},
						{PieceCID: "baga6ea4seaqao7s73y24kcutaosvacpdjgfe74urpi6rvmslce6hre5ioksjwq", Status: pdp.PullStatusInProgress},
					},
				})
				cb(&pdp.PullResult{
					Status: pdp.PullStatusComplete,
					Pieces: []pdp.PullPieceStatus{
						{PieceCID: info.CIDv2.String(), Status: pdp.PullStatusComplete},
					},
				})
			}
			return &pdp.PullResult{
				Status: pdp.PullStatusComplete,
				Pieces: []pdp.PullPieceStatus{
					{PieceCID: info.CIDv2.String(), Status: pdp.PullStatusComplete},
				},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithRecordKeeper(testRecordKeeper()),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	type progressEvent struct {
		pieceCID cid.Cid
		status   PullStatus
	}
	var events []progressEvent

	_, err = ctx.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{info.CIDv2},
		From:   func(c cid.Cid) string { return "https://primary.example.com/piece/" + c.String() },
		OnProgress: func(pieceCID cid.Cid, status PullStatus) {
			events = append(events, progressEvent{pieceCID: pieceCID, status: status})
		},
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("got %d progress events, want 2", len(events))
	}
	if events[0].pieceCID != info.CIDv2 || events[0].status != PullStatusInProgress {
		t.Errorf("event[0]={%s %s}, want {%s %s}", events[0].pieceCID, events[0].status, info.CIDv2, PullStatusInProgress)
	}
	if events[1].pieceCID != info.CIDv2 || events[1].status != PullStatusComplete {
		t.Errorf("event[1]={%s %s}, want {%s %s}", events[1].pieceCID, events[1].status, info.CIDv2, PullStatusComplete)
	}
}

// TestContextPull_ProgressStatusesStayAlignedWithPDP proves the public
// PullStatus constants still cover every PDP provider status we forward through
// PullRequest.OnProgress.
func TestContextPull_ProgressStatusesStayAlignedWithPDP(t *testing.T) {
	got := map[PullStatus]struct{}{
		PullStatusPending:    {},
		PullStatusInProgress: {},
		PullStatusRetrying:   {},
		PullStatusComplete:   {},
		PullStatusFailed:     {},
	}
	want := []PullStatus{
		PullStatus(pdp.PullStatusPending),
		PullStatus(pdp.PullStatusInProgress),
		PullStatus(pdp.PullStatusRetrying),
		PullStatus(pdp.PullStatusComplete),
		PullStatus(pdp.PullStatusFailed),
	}

	for _, status := range want {
		if _, ok := got[status]; !ok {
			t.Fatalf("missing exported PullStatus constant for %q", status)
		}
	}
}

// TestContextPull_IgnoresUnknownPieceIDsInResult proves that unknown server-side
// piece IDs are filtered not only from progress callbacks but also from the final
// caller-visible PullResult.
func TestContextPull_IgnoresUnknownPieceIDsInResult(t *testing.T) {
	info := mustPieceInfo(t)

	fake := &fakePDPProviderClient{
		pullPiecesFnWithCb: func(_ context.Context, req pdp.PullRequest, cb func(*pdp.PullResult)) (*pdp.PullResult, error) {
			return &pdp.PullResult{
				Status: pdp.PullStatusComplete,
				Pieces: []pdp.PullPieceStatus{
					{PieceCID: info.CIDv2.String(), Status: pdp.PullStatusComplete},
					{PieceCID: "baga6ea4seaqao7s73y24kcutaosvacpdjgfe74urpi6rvmslce6hre5ioksjwq", Status: pdp.PullStatusFailed},
				},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithRecordKeeper(testRecordKeeper()),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	got, err := ctx.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{info.CIDv2},
		From:   func(c cid.Cid) string { return "https://primary.example.com/piece/" + c.String() },
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if len(got.Pieces) != 1 {
		t.Fatalf("got %d pieces, want 1", len(got.Pieces))
	}
	if got.Pieces[0].PieceCID != info.CIDv2 || got.Pieces[0].Status != PullStatusComplete {
		t.Errorf("piece[0]={%s %s}, want {%s %s}", got.Pieces[0].PieceCID, got.Pieces[0].Status, info.CIDv2, PullStatusComplete)
	}
}

// TestContextCommit_OnSubmittedExistingDataSet proves that CommitRequest.OnSubmitted
// fires with the AddPieces tx hash immediately after submission and before
// waiting for on-chain confirmation.
func TestContextCommit_OnSubmittedExistingDataSet(t *testing.T) {
	info := mustPieceInfo(t)
	wantTxHash := "0x000000000000000000000000000000000000000000000000000000000000abcd"

	var submittedHash string
	var submittedBeforeWait bool
	waitCalled := false

	fake := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{
				TxHash:    common.HexToHash("0xabcd"),
				StatusURL: "https://sp.example.com/status",
			}, nil
		},
		waitForAddedFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			waitCalled = true
			submittedBeforeWait = submittedHash != ""
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xabcd"),
				DataSetID:         types.NewBigInt(42),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(7)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.NewBigInt(42)),
		WithClientDataSetID(types.NewBigInt(1)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
		OnSubmitted: func(txHash string) {
			submittedHash = txHash
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if !waitCalled {
		t.Fatal("WaitForPiecesAdded was not called")
	}
	if submittedHash != wantTxHash {
		t.Errorf("OnSubmitted txHash=%q, want %q", submittedHash, wantTxHash)
	}
	if !submittedBeforeWait {
		t.Error("OnSubmitted must fire before WaitForPiecesAdded")
	}
}

// TestContextCommit_OnSubmittedNewDataSet proves that CommitRequest.OnSubmitted
// fires with the CreateDataSetAndAddPieces tx hash immediately after submission
// and before waiting for on-chain confirmation.
func TestContextCommit_OnSubmittedNewDataSet(t *testing.T) {
	info := mustPieceInfo(t)
	wantTxHash := "0x000000000000000000000000000000000000000000000000000000000000beef"

	var submittedHash string
	var submittedBeforeWait bool
	waitCalled := false

	fake := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xbeef"),
				StatusURL: "https://sp.example.com/status",
			}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			waitCalled = true
			submittedBeforeWait = submittedHash != ""
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xbeef"),
				DataSetID:         types.NewBigInt(99),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(3)},
			}, nil
		},
	}

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	_, err = ctx.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: info.CIDv2}},
		OnSubmitted: func(txHash string) {
			submittedHash = txHash
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if !waitCalled {
		t.Fatal("WaitForCreateDataSetAndAddPieces was not called")
	}
	if submittedHash != wantTxHash {
		t.Errorf("OnSubmitted txHash=%q, want %q", submittedHash, wantTxHash)
	}
	if !submittedBeforeWait {
		t.Error("OnSubmitted must fire before WaitForCreateDataSetAndAddPieces")
	}
}
