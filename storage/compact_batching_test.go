package storage

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
)

func compactTestPieces(t *testing.T, count int) []PieceInput {
	t.Helper()
	pieces := make([]PieceInput, count)
	for i := range pieces {
		pieces[i] = batchTestPiece(t, fmt.Sprintf("compact-piece-%d", i))
	}
	return pieces
}

func TestContextLayoutConfiguration(t *testing.T) {
	for _, test := range []struct {
		name string
		opts []ContextOption
		want uint64
	}{
		{"unknown", nil, 0},
		{"mainnet", []ContextOption{WithChainID(314)}, 1559},
		{"calibration", []ContextOption{WithChainID(314159)}, 32331},
		{"explicit zero", []ContextOption{WithLegacyPieceStorageIDLimit(0), WithChainID(314159)}, 0},
		{"last override", []ContextOption{WithLegacyPieceStorageIDLimit(10), WithChainID(314), WithLegacyPieceStorageIDLimit(20)}, 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := mustProviderContext(t, &fakePDPProviderClient{}, test.opts...)
			bound, err := provider.ForDataSet(testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)))
			if err != nil {
				t.Fatal(err)
			}
			if provider.legacyPieceStorageLimit() != test.want || bound.legacyPieceStorageLimit() != test.want {
				t.Fatalf("provider=%d bound=%d want=%d", provider.legacyPieceStorageLimit(), bound.legacyPieceStorageLimit(), test.want)
			}
		})
	}
}

func TestContextBatchLimitsBeforeSideEffects(t *testing.T) {
	largeID, err := types.BigIntFromBig(new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		id     types.BigInt
		limit  uint64
		count  int
		reject bool
	}{
		{"legacy 80", types.NewBigInt(99), 100, 80, false},
		{"legacy 81", types.NewBigInt(99), 100, 81, true},
		{"compact 41", types.NewBigInt(100), 100, 41, false},
		{"compact 81", types.NewBigInt(100), 100, 81, false},
		{"large ID", largeID, 100, 81, false},
		{"unknown", types.NewBigInt(99), 0, 81, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pieces := compactTestPieces(t, test.count)
			ref := testDataSetRef(test.id, types.NewBigInt(7))
			calls, sourceCalls := 0, 0
			client := &fakePDPProviderClient{
				addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
					calls++
					return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x11"), StatusURL: "https://sp.example.com/status/add"}, nil
				},
				waitForPullFn: func(_ context.Context, req pdp.PullRequest) (*pdp.PullResult, error) {
					calls++
					return completePullResult(req), nil
				},
			}
			tracking := &trackingStorageSigner{inner: mustTestSigner(t)}
			bound, err := NewDataSetContext(testProvider(), client, tracking, ref,
				WithChainID(314159), WithPayer(testPayer()), WithRecordKeeper(testRecordKeeper()), WithLegacyPieceStorageIDLimit(test.limit))
			if err != nil {
				t.Fatal(err)
			}
			_, signErr := bound.PresignForCommit(context.Background(), pieces)
			_, internalErr := bound.SubmitCommit(context.Background(), CommitRequest{Pieces: pieces})
			_, externalErr := bound.SubmitCommit(context.Background(), CommitRequest{Pieces: pieces, ExtraData: []byte{1}})
			_, pullErr := bound.Pull(context.Background(), PullRequest{Pieces: pieceCIDs(pieces), ExtraData: []byte{1}, From: func(cid.Cid) string { sourceCalls++; return "https://source.example.com/piece" }})
			for name, err := range map[string]error{"presign": signErr, "internal": internalErr, "external": externalErr, "pull": pullErr} {
				if test.reject {
					if !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, pdp.ErrTooManyPieces) {
						t.Errorf("%s error=%v", name, err)
					}
				} else if err != nil {
					t.Errorf("%s: %v", name, err)
				}
			}
			if test.reject && (calls != 0 || tracking.calls != 0 || sourceCalls != 0) {
				t.Fatalf("rejected batch produced side effects: requests=%d signatures=%d sources=%d", calls, tracking.calls, sourceCalls)
			}
		})
	}
}

func TestCompactCommitRecoversLargeBatch(t *testing.T) {
	pieces := compactTestPieces(t, 81)
	ref := testDataSetRef(types.NewBigInt(32331), types.NewBigInt(7))
	ids := make([]types.BigInt, len(pieces))
	for i := range ids {
		ids[i] = types.NewBigInt(uint64(i + 1))
	}
	posts := 0
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			posts++
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x11"), StatusURL: "https://sp.example.com/status/add"}, nil
		},
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{TxHash: common.HexToHash("0x11"), DataSetID: ref.DataSetID(), PiecesAdded: true, PieceCount: 81, ConfirmedPieceIDs: ids}, nil
		},
	}
	bound := mustWritableDataSetContext(t, client, ref)
	submission, err := bound.SubmitCommit(context.Background(), CommitRequest{Pieces: pieces})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.waitForCommit(context.Background(), *submission); err != nil {
		t.Fatal(err)
	}
	resumed := mustWritableDataSetContext(t, client, ref)
	status, err := resumed.GetCommitStatus(context.Background(), submission.StatusURL)
	if err != nil || len(status.PieceIDs) != 81 {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	result, err := resumed.WaitForCommit(context.Background(), submission.StatusURL)
	if err != nil || len(result.PieceIDs) != 81 || posts != 1 {
		t.Fatalf("result=%+v err=%v posts=%d", result, err, posts)
	}
	legacy := mustWritableDataSetContext(t, client, ref, WithLegacyPieceStorageIDLimit(32332))
	if _, err := legacy.GetCommitStatus(context.Background(), submission.StatusURL); !errors.Is(err, pdp.ErrTooManyPieces) {
		t.Fatalf("legacy recovery error=%v", err)
	}
}

func TestUploadBatcherCompactFlushesBeyondForty(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := sharedBatchTestTarget(identity, 1)
	commits := recordSharedBatchCommits(target)
	for _, input := range compactTestPieces(t, 81) {
		enqueueBatchTestPiece(t, batcher, target, input)
	}
	if len(commits.snapshot()) != 0 {
		t.Fatal("compact batch sealed before Flush")
	}
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := commits.snapshot()
	if len(got) != 1 || !got[0].create || got[0].pieces != 81 {
		t.Fatalf("commits=%+v", got)
	}
}

func TestUploadBatcherLayoutChangesWindowButSharesDataSet(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	first := sharedBatchTestTarget(identity, 1)
	first.legacyLimit = 1
	commits := recordSharedBatchCommits(first)
	second := *first
	second.legacyLimit = 2
	firstTask := enqueueBatchTestPiece(t, batcher, first, batchTestPiece(t, "first-layout"))
	secondTask := enqueueBatchTestPiece(t, batcher, &second, batchTestPiece(t, "second-layout"))
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := commits.snapshot()
	if len(got) != 2 || !got[0].create || got[1].create || got[0].pieces != 1 || got[1].pieces != 1 {
		t.Fatalf("commits=%+v, want separate windows and one create", got)
	}
	for _, task := range []*uploadBatchTask{firstTask, secondTask} {
		result, _, err := task.wait(context.Background(), nil)
		if err != nil || !result.DataSet.DataSetID().Equal(types.NewBigInt(101)) {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}

func TestUploadBatcherReplacementKeepsCreationBudget(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := sharedBatchTestTarget(identity, 1)
	commits := recordSharedBatchCommits(target)
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "initial-budget"))
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	pieces := compactTestPieces(t, 500)
	id := types.NewBigInt(1)
	limit := 1
	for limit < len(pieces) && batcher.validateCandidate(target, nil, &id, pieces[:limit+1]) == nil {
		limit++
	}
	if limit >= len(pieces) {
		t.Fatal("fixture did not reach the real ABI boundary")
	}
	ref := testCommitDataSetRef(1, 101)
	if err := batcher.validateCandidate(target, &ref, nil, pieces[:limit+1]); err != nil {
		t.Fatalf("next item must fit add-pieces but exceed creation budget: %v", err)
	}
	terminated := false
	commits.submitHook = func(commit sharedBatchCommit) error {
		if !commit.create && !terminated {
			terminated = true
			return &DataSetPDPPaymentTerminatedError{DataSetID: types.NewBigInt(101), PDPEndEpoch: 10}
		}
		return nil
	}
	var first *uploadBatchTask
	for i, input := range pieces[:limit+1] {
		task := enqueueBatchTestPiece(t, batcher, target, input)
		if i == 0 {
			first = task
		}
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, _, err := first.wait(waitCtx, nil); err != nil {
		t.Fatal(err)
	}
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := commits.snapshot()
	if len(got) != 4 || got[1].create || !got[2].create || got[2].pieces != limit || got[3].create || got[3].pieces != 1 {
		t.Fatalf("commits=%+v, want size split and replacement create", got)
	}
}

func TestUploadBatcherAccountsForLegalMetadata(t *testing.T) {
	metadata := func(count int, prefix string) map[string]string {
		result := make(map[string]string, count)
		for i := range count {
			result[strings.Repeat(fmt.Sprintf("%s%d", prefix, i), 16)] = strings.Repeat("v", maxMetadataValueLength)
		}
		return result
	}
	pieces := compactTestPieces(t, 81)
	for i := range pieces {
		pieces[i].PieceMetadata = metadata(maxPieceMetadataKeys, "p")
	}
	dataSetMetadata := metadata(maxDataSetMetadataKeys, "d")
	tracking := &trackingStorageSigner{inner: mustTestSigner(t)}
	provider, err := NewProviderContext(testProvider(), &fakePDPProviderClient{}, tracking, WithChainID(314159), WithPayer(testPayer()), WithRecordKeeper(testRecordKeeper()), WithDataSetMetadata(dataSetMetadata))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.PresignForCommit(context.Background(), pieces); !errors.Is(err, pdp.ErrAddPiecesMessageTooLarge) || tracking.calls != 0 {
		t.Fatalf("oversized metadata error=%v signatures=%d", err, tracking.calls)
	}
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := sharedBatchTestTarget(identity, 1)
	target.dataSetMetadata = dataSetMetadata
	commits := recordSharedBatchCommits(target)
	submit := target.commitRequestFn
	target.commitRequestFn = func(ctx context.Context, req commitRequest) (*CommitSubmission, error) {
		inputs := make([]pdp.AddPieceInput, len(req.Pieces))
		for i, input := range req.Pieces {
			inputs[i] = pdp.AddPieceInput{PieceCID: input.PieceCID}
		}
		size, err := pdp.EstimateAddPiecesMessageSize(inputs, req.ExtraData)
		if err != nil {
			return nil, err
		}
		if size > pdp.MaxAddPiecesMessageSize {
			return nil, fmt.Errorf("submitted metadata batch size=%d exceeds %d", size, pdp.MaxAddPiecesMessageSize)
		}
		return submit(ctx, req)
	}
	for _, input := range pieces {
		enqueueBatchTestPiece(t, batcher, target, input)
	}
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := commits.snapshot()
	if len(got) < 2 {
		t.Fatalf("commits=%+v, want byte-sized windows", got)
	}
	total, creates := 0, 0
	for _, commit := range got {
		total += commit.pieces
		if commit.create {
			creates++
		}
	}
	if total != len(pieces) || creates != 1 {
		t.Fatalf("pieces=%d creates=%d, want 81 pieces in one shared data set", total, creates)
	}
}
