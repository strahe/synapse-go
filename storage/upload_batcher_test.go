package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

func TestNewUploadBatcherOptions(t *testing.T) {
	identity := serviceTestIdentity()
	storageSigner := mustTestSigner(t)

	tests := []struct {
		name    string
		opts    []UploadBatcherOption
		wantErr bool
		check   func(*testing.T, uploadBatcherConfig)
	}{
		{
			name: "defaults",
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if !cfg.idleWaitEnabled || cfg.idleWait != 3*time.Second {
					t.Fatalf("idle=(%t,%s), want enabled 3s", cfg.idleWaitEnabled, cfg.idleWait)
				}
				if cfg.maxWaitEnabled {
					t.Fatalf("max=(%t,%s), want disabled", cfg.maxWaitEnabled, cfg.maxWait)
				}
				if cfg.maxConcurrentSubmissions != 4 {
					t.Fatalf("maxConcurrentSubmissions=%d, want 4", cfg.maxConcurrentSubmissions)
				}
			},
		},
		{
			name: "immediate",
			opts: []UploadBatcherOption{WithUploadIdleWait(0)},
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if !cfg.idleWaitEnabled || cfg.idleWait != 0 {
					t.Fatalf("idle=(%t,%s), want enabled zero", cfg.idleWaitEnabled, cfg.idleWait)
				}
			},
		},
		{
			name: "flush only",
			opts: []UploadBatcherOption{WithoutUploadIdleWait(), WithoutUploadMaxWait()},
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if cfg.idleWaitEnabled || cfg.maxWaitEnabled {
					t.Fatalf("timers enabled: idle=%t max=%t", cfg.idleWaitEnabled, cfg.maxWaitEnabled)
				}
			},
		},
		{
			name: "idle disabled",
			opts: []UploadBatcherOption{WithoutUploadIdleWait()},
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if cfg.idleWaitEnabled || cfg.maxWaitEnabled {
					t.Fatalf("idle enabled=%t max enabled=%t, want flush-only defaults", cfg.idleWaitEnabled, cfg.maxWaitEnabled)
				}
			},
		},
		{
			name: "max enabled",
			opts: []UploadBatcherOption{WithUploadMaxWait(30 * time.Second)},
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if !cfg.idleWaitEnabled || cfg.idleWait != 3*time.Second || !cfg.maxWaitEnabled || cfg.maxWait != 30*time.Second {
					t.Fatalf("idle=(%t,%s) max=(%t,%s), want default idle and 30s max", cfg.idleWaitEnabled, cfg.idleWait, cfg.maxWaitEnabled, cfg.maxWait)
				}
			},
		},
		{
			name: "max disabled after enabled",
			opts: []UploadBatcherOption{WithUploadMaxWait(time.Minute), WithoutUploadMaxWait()},
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if !cfg.idleWaitEnabled || cfg.idleWait != 3*time.Second || cfg.maxWaitEnabled {
					t.Fatalf("idle=(%t,%s) max enabled=%t, want default idle and max disabled", cfg.idleWaitEnabled, cfg.idleWait, cfg.maxWaitEnabled)
				}
			},
		},
		{
			name: "per piece",
			opts: []UploadBatcherOption{WithoutUploadIdleWait(), WithUploadMaxWait(0)},
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if cfg.idleWaitEnabled || !cfg.maxWaitEnabled || cfg.maxWait != 0 {
					t.Fatalf("idle enabled=%t max=(%t,%s), want idle disabled and zero max", cfg.idleWaitEnabled, cfg.maxWaitEnabled, cfg.maxWait)
				}
			},
		},
		{name: "negative idle", opts: []UploadBatcherOption{WithUploadIdleWait(-time.Second)}, wantErr: true},
		{name: "negative max", opts: []UploadBatcherOption{WithUploadMaxWait(-time.Second)}, wantErr: true},
		{name: "idle exceeds max", opts: []UploadBatcherOption{WithUploadIdleWait(time.Minute), WithUploadMaxWait(time.Second)}, wantErr: true},
		{name: "zero max with default idle", opts: []UploadBatcherOption{WithUploadMaxWait(0)}, wantErr: true},
		{name: "zero concurrency", opts: []UploadBatcherOption{WithUploadMaxConcurrentSubmissions(0)}, wantErr: true},
		{
			name: "last option wins",
			opts: []UploadBatcherOption{WithoutUploadIdleWait(), WithUploadIdleWait(3 * time.Second)},
			check: func(t *testing.T, cfg uploadBatcherConfig) {
				t.Helper()
				if !cfg.idleWaitEnabled || cfg.idleWait != 3*time.Second {
					t.Fatalf("idle=(%t,%s), want enabled 3s", cfg.idleWaitEnabled, cfg.idleWait)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			batcher, err := NewUploadBatcher(UploadBatcherOptions{Identity: identity, Signer: storageSigner}, tt.opts...)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("error=%v, want ErrInvalidArgument", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewUploadBatcher: %v", err)
			}
			t.Cleanup(func() { _ = batcher.Close() })
			if tt.check != nil {
				tt.check(t, batcher.config)
			}
		})
	}
}

func TestUploadBatcherFlushCombinesCompatiblePieces(t *testing.T) {
	identity := serviceTestIdentity()
	storageSigner := mustTestSigner(t)
	ref := testCommitDataSetRef(1, 11)
	var (
		mu          sync.Mutex
		submitted   []PieceInput
		submitCalls int
		waitCalls   int
	)
	target := batchTestTarget(identity, ref)
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		mu.Lock()
		defer mu.Unlock()
		submitCalls++
		submitted = append([]PieceInput(nil), req.Pieces...)
		return &CommitSubmission{TransactionID: "0xbatch", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, _ CommitSubmission) (*CommitResult, error) {
		mu.Lock()
		waitCalls++
		mu.Unlock()
		return &CommitResult{
			TransactionID: "0xbatch",
			DataSet:       ref,
			PieceIDs:      []types.BigInt{types.NewBigInt(101), types.NewBigInt(102)},
		}, nil
	}
	batcher := mustUploadBatcher(t, identity, storageSigner, WithoutUploadIdleWait(), WithoutUploadMaxWait())
	pieceA := batchTestPiece(t, "piece-a")
	pieceB := batchTestPiece(t, "piece-b")
	taskA := enqueueBatchTestPiece(t, batcher, target, pieceA)
	taskB := enqueueBatchTestPiece(t, batcher, target, pieceB)
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	var txA, txB string
	resultA, _, err := taskA.wait(context.Background(), func(tx string) { txA = tx })
	if err != nil {
		t.Fatalf("wait A: %v", err)
	}
	resultB, _, err := taskB.wait(context.Background(), func(tx string) { txB = tx })
	if err != nil {
		t.Fatalf("wait B: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if submitCalls != 1 || waitCalls != 1 || len(submitted) != 2 {
		t.Fatalf("submitCalls=%d waitCalls=%d pieces=%d, want 1,1,2", submitCalls, waitCalls, len(submitted))
	}
	if txA != "0xbatch" || txB != "0xbatch" {
		t.Fatalf("transaction IDs=(%q,%q), want shared 0xbatch", txA, txB)
	}
	if got := resultA.PieceIDs[0].String(); got != "101" {
		t.Fatalf("piece A ID=%s, want 101", got)
	}
	if got := resultB.PieceIDs[0].String(); got != "102" {
		t.Fatalf("piece B ID=%s, want 102", got)
	}
}

func TestUploadBatcherConcurrentFlushesWaitForSameFlight(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submitEntered := make(chan struct{})
	releaseSubmit := make(chan struct{})
	var submitOnce sync.Once
	var submissions atomic.Int32
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		submissions.Add(1)
		submitOnce.Do(func() { close(submitEntered) })
		<-releaseSubmit
		return &CommitSubmission{TransactionID: "0xflush", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		ref, _ := target.DataSetRef()
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
	}
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "concurrent-flush"))
	results := make(chan error, 2)
	go func() { results <- batcher.Flush(context.Background()) }()
	select {
	case <-submitEntered:
	case <-time.After(time.Second):
		t.Fatal("batch submission did not start")
	}
	go func() { results <- batcher.Flush(context.Background()) }()
	waitForUploadBatchFlushes(t, batcher, 2)
	close(releaseSubmit)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("Flush: %v", err)
		}
	}
	if got := submissions.Load(); got != 1 {
		t.Fatalf("submissions=%d, want 1", got)
	}
}

func TestUploadBatcherFlushReportsCompletedFailuresUntilAcknowledged(t *testing.T) {
	newFailedFlight := func(t *testing.T) (*UploadBatcher, *uploadReservation, error) {
		t.Helper()
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
		target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
		failure := errors.New("completed batch failed")
		target.submitCommitFn = func(context.Context, CommitRequest) (*CommitSubmission, error) {
			return nil, failure
		}
		reservation, err := batcher.reserve()
		if err != nil {
			t.Fatalf("reserve: %v", err)
		}
		if _, err := batcher.enqueue(context.Background(), reservation.seq, target, batchTestPiece(t, "historical-failure"), nil); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		waitForNoUploadBatchFlights(t, batcher)
		return batcher, reservation, failure
	}

	t.Run("completed flush reports and consumes failure", func(t *testing.T) {
		batcher, reservation, failure := newFailedFlight(t)
		flushDone := make(chan error, 1)
		go func() { flushDone <- batcher.Flush(context.Background()) }()
		waitForUploadBatchFlush(t, batcher)
		reservation.release()
		if err := <-flushDone; !errors.Is(err, failure) {
			t.Fatalf("Flush error=%v, want completed failure", err)
		}
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatalf("second Flush error=%v, want consumed failure", err)
		}
	})

	t.Run("canceled flush does not consume failure", func(t *testing.T) {
		batcher, reservation, failure := newFailedFlight(t)
		ctx, cancel := context.WithCancel(context.Background())
		flushDone := make(chan error, 1)
		go func() { flushDone <- batcher.Flush(ctx) }()
		waitForUploadBatchFlush(t, batcher)
		cancel()
		if err := <-flushDone; !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled Flush error=%v, want context.Canceled", err)
		}
		reservation.release()
		if err := batcher.Flush(context.Background()); !errors.Is(err, failure) {
			t.Fatalf("next Flush error=%v, want retained failure", err)
		}
	})

	t.Run("concurrent flushes retain their snapshots", func(t *testing.T) {
		batcher, reservation, failure := newFailedFlight(t)
		results := make(chan error, 2)
		go func() { results <- batcher.Flush(context.Background()) }()
		go func() { results <- batcher.Flush(context.Background()) }()
		waitForUploadBatchFlushes(t, batcher, 2)
		reservation.release()
		for range 2 {
			if err := <-results; !errors.Is(err, failure) {
				t.Fatalf("concurrent Flush error=%v, want snapshotted failure", err)
			}
		}
	})
}

func TestUploadBatcherWaitConsumesFailedFlightForLaterFlush(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	failure := errors.New("observed batch failed")
	target.submitCommitFn = func(context.Context, CommitRequest) (*CommitSubmission, error) {
		return nil, failure
	}
	task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "observed-failure"))
	if _, _, err := task.wait(context.Background(), nil); !errors.Is(err, failure) {
		t.Fatalf("wait error=%v, want observed failure", err)
	}
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush after wait error=%v, want nil", err)
	}
}

func TestUploadBatcherFlushReportsFailureWhenWaitMissesTerminalEvent(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	failure := errors.New("unobserved batch failed")
	started := make(chan struct{})
	release := make(chan struct{})
	target.submitCommitFn = func(context.Context, CommitRequest) (*CommitSubmission, error) {
		close(started)
		<-release
		return nil, failure
	}
	task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "unobserved-failure"))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("submit did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	waitDone := make(chan error, 1)
	go func() {
		_, _, err := task.wait(ctx, nil)
		waitDone <- err
	}()
	cancel()
	if err := <-waitDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error=%v, want context.Canceled", err)
	}
	close(release)
	waitForNoUploadBatchFlights(t, batcher)
	batcher.mu.Lock()
	if len(batcher.flights) != 0 {
		batcher.mu.Unlock()
		t.Fatalf("flights=%d, want 0 after unobserved failure", len(batcher.flights))
	}
	if len(batcher.failedFlights) != 1 {
		batcher.mu.Unlock()
		t.Fatalf("failedFlights=%d, want 1 slim error record", len(batcher.failedFlights))
	}
	for _, rec := range batcher.failedFlights {
		if rec == nil || rec.seq == 0 || rec.minReservation == 0 || !errors.Is(rec.err, failure) {
			batcher.mu.Unlock()
			t.Fatalf("slim failure record=%+v, want seq, reservation, and %v", rec, failure)
		}
	}
	batcher.mu.Unlock()
	if err := batcher.Flush(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("Flush error=%v, want unobserved failure", err)
	}
}

func TestUploadBatcherFlushMayIncludeCompatiblePostBarrierSlot(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submittedPieces := make(chan int, 1)
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		submittedPieces <- len(req.Pieces)
		return &CommitSubmission{TransactionID: "0xbarrier", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		ref, _ := target.DataSetRef()
		pieceIDs := make([]types.BigInt, len(submission.PieceCIDs))
		for i := range pieceIDs {
			pieceIDs[i] = types.NewBigInt(uint64(i + 1))
		}
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: pieceIDs}, nil
	}
	firstReservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve first: %v", err)
	}
	first, err := batcher.enqueue(context.Background(), firstReservation.seq, target, batchTestPiece(t, "barrier-first"), nil)
	if err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	waitForUploadBatchFlush(t, batcher)
	secondReservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve second: %v", err)
	}
	second, err := batcher.enqueue(context.Background(), secondReservation.seq, target, batchTestPiece(t, "barrier-second"), nil)
	secondReservation.release()
	if err != nil {
		t.Fatalf("enqueue second: %v", err)
	}
	firstReservation.release()
	if err := <-flushDone; err != nil {
		t.Fatalf("Flush: %v", err)
	}
	select {
	case pieces := <-submittedPieces:
		if pieces != 2 {
			t.Fatalf("submitted pieces=%d, want shared window with post-barrier slot", pieces)
		}
	default:
		t.Fatal("shared window was not submitted")
	}
	for name, task := range map[string]*uploadBatchTask{"first": first, "second": second} {
		result, _, err := task.wait(context.Background(), nil)
		if err != nil {
			t.Fatalf("wait %s: %v", name, err)
		}
		if result.TransactionID != "0xbarrier" {
			t.Fatalf("wait %s transaction=%q, want shared transaction", name, result.TransactionID)
		}
	}
}

func TestUploadBatcherRejectsReorderedSubmissionForWholeBatch(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	pieceA := batchTestPiece(t, "ordered-a")
	pieceB := batchTestPiece(t, "ordered-b")
	target.submitCommitFn = func(context.Context, CommitRequest) (*CommitSubmission, error) {
		return &CommitSubmission{TransactionID: "0xreordered", PieceCIDs: []cid.Cid{pieceB.PieceCID, pieceA.PieceCID}}, nil
	}
	target.waitCommitFn = func(context.Context, CommitSubmission) (*CommitResult, error) {
		return nil, errors.New("WaitForCommit must not run for a reordered submission")
	}
	tasks := []*uploadBatchTask{
		enqueueBatchTestPiece(t, batcher, target, pieceA),
		enqueueBatchTestPiece(t, batcher, target, pieceB),
	}
	err := batcher.Flush(context.Background())
	if err == nil || !strings.Contains(err.Error(), "submission piece order") {
		t.Fatalf("Flush error=%v, want submission piece order error", err)
	}
	for i, task := range tasks {
		if _, _, err := task.wait(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "submission piece order") {
			t.Fatalf("task %d error=%v, want whole-batch order error", i, err)
		}
	}
}

func TestUploadBatcherZeroWaitSealsEachReadyPieceImmediately(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0), withUploadBatchClock(clock))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	var submissions atomic.Int32
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		submissions.Add(1)
		return &CommitSubmission{TransactionID: "0ximmediate", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		ref, _ := target.DataSetRef()
		return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
	}
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "immediate-a"))
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "immediate-b"))
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := submissions.Load(); got != 2 {
		t.Fatalf("submissions=%d, want one immediate submission per piece", got)
	}
}

func TestUploadBatcherSealsAtPieceLimit(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	var (
		mu     sync.Mutex
		counts []int
	)
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		mu.Lock()
		counts = append(counts, len(req.Pieces))
		mu.Unlock()
		return &CommitSubmission{TransactionID: "0xlimit", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		pieceIDs := make([]types.BigInt, len(submission.PieceCIDs))
		for i := range pieceIDs {
			pieceIDs[i] = types.NewBigInt(uint64(i + 1))
		}
		ref, _ := target.DataSetRef()
		return &CommitResult{DataSet: ref, PieceIDs: pieceIDs}, nil
	}
	for i := range pdp.MaxAddPiecesBatchSize + 1 {
		enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "piece-limit-"+strconv.Itoa(i)))
	}
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(counts) != 2 {
		t.Fatalf("submitted piece counts=%v, want two batches", counts)
	}
	if min(counts[0], counts[1]) != 1 || max(counts[0], counts[1]) != pdp.MaxAddPiecesBatchSize {
		t.Fatalf("submitted piece counts=%v, want %d and 1", counts, pdp.MaxAddPiecesBatchSize)
	}
}

func TestUploadBatcherSealsAtMessageSizeBoundary(t *testing.T) {
	originalEstimator := estimateAddPiecesMessageSize
	estimateAddPiecesMessageSize = func(pieces []pdp.AddPieceInput, _ []byte) (int, error) {
		if len(pieces) > 1 {
			return pdp.MaxAddPiecesMessageSize + 1, nil
		}
		return pdp.MaxAddPiecesMessageSize, nil
	}
	t.Cleanup(func() { estimateAddPiecesMessageSize = originalEstimator })

	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	var (
		mu     sync.Mutex
		counts []int
	)
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		mu.Lock()
		counts = append(counts, len(req.Pieces))
		mu.Unlock()
		return &CommitSubmission{TransactionID: "0xsize", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		ref, _ := target.DataSetRef()
		return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
	}
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "size-a"))
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "size-b"))
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(counts) != 2 || counts[0] != 1 || counts[1] != 1 {
		t.Fatalf("submitted piece counts=%v, want two single-piece batches", counts)
	}
}

func TestUploadBatcherRejectsOversizedSinglePiece(t *testing.T) {
	originalEstimator := estimateAddPiecesMessageSize
	estimateAddPiecesMessageSize = func([]pdp.AddPieceInput, []byte) (int, error) {
		return pdp.MaxAddPiecesMessageSize + 1, nil
	}
	t.Cleanup(func() { estimateAddPiecesMessageSize = originalEstimator })

	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	defer reservation.release()
	_, err = batcher.enqueue(context.Background(), reservation.seq, batchTestTarget(identity, testCommitDataSetRef(1, 11)), batchTestPiece(t, "oversized"), nil)
	if !errors.Is(err, pdp.ErrAddPiecesMessageTooLarge) {
		t.Fatalf("enqueue error=%v, want pdp.ErrAddPiecesMessageTooLarge", err)
	}
}

func TestUploadBatchTargetKeyMergesOnlyExactTargets(t *testing.T) {
	identity := serviceTestIdentity()
	newTarget := func(endpoint string, metadata map[string]string) *fakeUploadContext {
		target := batchTestTarget(identity, DataSetRef{})
		target.id = types.NewBigInt(1)
		target.endpoint = endpoint
		target.dataSetID = nil
		target.clientDataSetID = nil
		target.dataSetMetadata = metadata
		return target
	}
	left := newTarget("https://provider.example.com/custom", map[string]string{"a": "1", "b": "2"})
	right := newTarget("https://provider.example.com/custom", map[string]string{"b": "2", "a": "1"})
	leftKey, _, err := uploadBatchTargetKey(left)
	if err != nil {
		t.Fatalf("left target key: %v", err)
	}
	rightKey, _, err := uploadBatchTargetKey(right)
	if err != nil {
		t.Fatalf("right target key: %v", err)
	}
	if leftKey != rightKey {
		t.Fatal("equivalent sorted metadata did not share a new-data-set target key")
	}
	differentURLKey, _, err := uploadBatchTargetKey(newTarget("https://provider.example.com/custom/", map[string]string{"a": "1", "b": "2"}))
	if err != nil {
		t.Fatalf("different URL target key: %v", err)
	}
	if leftKey == differentURLKey {
		t.Fatal("textually different service URLs shared a target key")
	}

	refA, err := NewDataSetRef(types.NewBigInt(1), types.NewBigInt(11), types.NewBigInt(21))
	if err != nil {
		t.Fatalf("NewDataSetRef A: %v", err)
	}
	refB, err := NewDataSetRef(types.NewBigInt(1), types.NewBigInt(11), types.NewBigInt(22))
	if err != nil {
		t.Fatalf("NewDataSetRef B: %v", err)
	}
	existingA, _, err := uploadBatchTargetKey(batchTestTarget(identity, refA))
	if err != nil {
		t.Fatalf("existing target A key: %v", err)
	}
	existingB, _, err := uploadBatchTargetKey(batchTestTarget(identity, refB))
	if err != nil {
		t.Fatalf("existing target B key: %v", err)
	}
	if existingA == existingB {
		t.Fatal("different complete DataSetRef values shared a target key")
	}
}

func TestUploadBatcherSerializesSameDataSetSubmissionsButNotConfirmations(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submitEntered := make(chan struct{}, 2)
	submitRelease := make(chan struct{}, 2)
	confirmationEntered := make(chan struct{}, 2)
	confirmationRelease := make(chan struct{})
	var activeSubmissions atomic.Int32
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		if activeSubmissions.Add(1) != 1 {
			return nil, errors.New("same data set submitted concurrently")
		}
		submitEntered <- struct{}{}
		<-submitRelease
		activeSubmissions.Add(-1)
		return &CommitSubmission{TransactionID: "0xserial", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		confirmationEntered <- struct{}{}
		<-confirmationRelease
		ref, _ := target.DataSetRef()
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
	}
	pieceInput := batchTestPiece(t, "serial-boundary")
	enqueueBatchTestPiece(t, batcher, target, pieceInput)
	enqueueBatchTestPiece(t, batcher, target, pieceInput)
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	select {
	case <-submitEntered:
	case <-time.After(time.Second):
		t.Fatal("first submission did not start")
	}
	select {
	case <-submitEntered:
		t.Fatal("same data set submissions overlapped")
	default:
	}
	submitRelease <- struct{}{}
	select {
	case <-confirmationEntered:
	case <-time.After(time.Second):
		t.Fatal("first confirmation did not start")
	}
	select {
	case <-submitEntered:
	case <-time.After(time.Second):
		t.Fatal("second submission waited for first confirmation")
	}
	submitRelease <- struct{}{}
	select {
	case <-confirmationEntered:
	case <-time.After(time.Second):
		t.Fatal("second confirmation did not start")
	}
	close(confirmationRelease)
	if err := <-flushDone; err != nil {
		t.Fatalf("Flush: %v", err)
	}
	batcher.mu.Lock()
	gateCount := len(batcher.datasetGates)
	batcher.mu.Unlock()
	if gateCount != 0 {
		t.Fatalf("data-set gates=%d, want 0 after submissions finish", gateCount)
	}
}

func TestUploadBatcherFlushJoinsErrorsInReservationOrder(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	targets := []*fakeUploadContext{
		batchTestTarget(identity, testCommitDataSetRef(2, 22)),
		batchTestTarget(identity, testCommitDataSetRef(1, 11)),
	}
	for i, target := range targets {
		message := []string{"first batch failed", "second batch failed"}[i]
		target.submitCommitFn = func(context.Context, CommitRequest) (*CommitSubmission, error) {
			return nil, errors.New(message)
		}
		enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, message))
	}
	err := batcher.Flush(context.Background())
	if err == nil || err.Error() != "first batch failed\nsecond batch failed" {
		t.Fatalf("Flush error=%q, want reservation-ordered joined errors", err)
	}
}

func TestUploadBatcherIdleWaitResetsAndMaxWaitDoesNot(t *testing.T) {
	tests := []struct {
		name         string
		idleWait     time.Duration
		maxWait      time.Duration
		secondAt     time.Duration
		triggerAfter time.Duration
	}{
		{name: "idle reset", idleWait: 10 * time.Second, maxWait: time.Minute, secondAt: 6 * time.Second, triggerAfter: 10 * time.Second},
		{name: "max retained", idleWait: 10 * time.Second, maxWait: 12 * time.Second, secondAt: 6 * time.Second, triggerAfter: 6 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := serviceTestIdentity()
			clock := newManualUploadBatchClock()
			batcher := mustUploadBatcher(t, identity, mustTestSigner(t),
				WithUploadIdleWait(tt.idleWait),
				WithUploadMaxWait(tt.maxWait),
				withUploadBatchClock(clock),
			)
			submitted := make(chan struct{}, 1)
			target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
			target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
				submitted <- struct{}{}
				return &CommitSubmission{TransactionID: "0xtimer", PieceCIDs: pieceCIDs(req.Pieces)}, nil
			}
			target.waitCommitFn = func(context.Context, CommitSubmission) (*CommitResult, error) {
				ref, _ := target.DataSetRef()
				return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1), types.NewBigInt(2)}}, nil
			}
			enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "first"))
			clock.Advance(tt.secondAt)
			enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "second"))
			clock.Advance(tt.triggerAfter - time.Nanosecond)
			select {
			case <-submitted:
				t.Fatal("window submitted before its deadline")
			default:
			}
			clock.Advance(time.Nanosecond)
			select {
			case <-submitted:
			case <-time.After(time.Second):
				t.Fatal("window did not submit at its deadline")
			}
		})
	}
}

func TestUploadBatcherIdleWaitWaitsForInProgressTransfers(t *testing.T) {
	tests := []struct {
		name       string
		finish     func(*testing.T, *UploadBatcher, *fakeUploadContext, *uploadReservation, *uploadBatchTransfer)
		wantPieces int
	}{
		{
			name: "transfer joins window",
			finish: func(t *testing.T, batcher *UploadBatcher, target *fakeUploadContext, reservation *uploadReservation, transfer *uploadBatchTransfer) {
				t.Helper()
				if _, err := batcher.enqueue(context.Background(), reservation.seq, target, batchTestPiece(t, "late"), transfer); err != nil {
					t.Fatalf("enqueue late piece: %v", err)
				}
				reservation.release()
			},
			wantPieces: 2,
		},
		{
			name: "transfer fails",
			finish: func(_ *testing.T, _ *UploadBatcher, _ *fakeUploadContext, _ *uploadReservation, transfer *uploadBatchTransfer) {
				transfer.end()
			},
			wantPieces: 1,
		},
		{
			name: "reservation released",
			finish: func(_ *testing.T, _ *UploadBatcher, _ *fakeUploadContext, reservation *uploadReservation, _ *uploadBatchTransfer) {
				reservation.release()
			},
			wantPieces: 1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := serviceTestIdentity()
			clock := newManualUploadBatchClock()
			batcher := mustUploadBatcher(t, identity, mustTestSigner(t), withUploadBatchClock(clock))
			target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
			submitted := captureBatchTestSubmissions(target)
			reservation, transfer := beginBatchTestTransfer(t, batcher, target)

			enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "ready"))
			clock.Advance(5 * time.Second)
			assertNoBatchTestSubmission(t, submitted)

			tt.finish(t, batcher, target, reservation, transfer)
			clock.Advance(3*time.Second - time.Nanosecond)
			assertNoBatchTestSubmission(t, submitted)
			clock.Advance(time.Nanosecond)
			if got := waitBatchTestSubmission(t, submitted); got != tt.wantPieces {
				t.Fatalf("submitted pieces=%d, want %d", got, tt.wantPieces)
			}
			assertNoUploadBatchTransfers(t, batcher)
		})
	}
}

func TestUploadBatcherTransferInvalidatesFiredIdleTimer(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), withUploadBatchClock(clock))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submitted := captureBatchTestSubmissions(target)
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "fired"))
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	defer reservation.release()
	key, _, err := uploadBatchTargetKey(target)
	if err != nil {
		t.Fatalf("uploadBatchTargetKey: %v", err)
	}

	// Fire the idle timer while the batcher lock is held, so its callback runs
	// only after a transfer has started for the same target.
	batcher.mu.Lock()
	advanced := make(chan struct{})
	go func() {
		clock.Advance(3 * time.Second)
		close(advanced)
	}()
	waitForManualUploadBatchTimerFired(t, clock)
	batcher.beginTransferLocked(reservation, key)
	batcher.mu.Unlock()
	<-advanced
	assertNoBatchTestSubmission(t, submitted)
}

func TestUploadBatcherMaxWaitDoesNotWaitForTransfers(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadMaxWait(10*time.Second), withUploadBatchClock(clock))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submitted := captureBatchTestSubmissions(target)
	reservation, _ := beginBatchTestTransfer(t, batcher, target)
	defer reservation.release()

	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "capped"))
	clock.Advance(10*time.Second - time.Nanosecond)
	assertNoBatchTestSubmission(t, submitted)
	clock.Advance(time.Nanosecond)
	if got := waitBatchTestSubmission(t, submitted); got != 1 {
		t.Fatalf("submitted pieces=%d, want 1", got)
	}
}

func TestUploadBatcherDefaultHasNoMaxWait(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), withUploadBatchClock(clock))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submitted := captureBatchTestSubmissions(target)
	reservation, _ := beginBatchTestTransfer(t, batcher, target)

	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "uncapped"))
	clock.Advance(time.Hour)
	assertNoBatchTestSubmission(t, submitted)

	reservation.release()
	clock.Advance(3 * time.Second)
	if got := waitBatchTestSubmission(t, submitted); got != 1 {
		t.Fatalf("submitted pieces=%d, want 1", got)
	}
}

func TestUploadBatcherZeroIdleWaitWaitsForTransfers(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0), withUploadBatchClock(clock))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submitted := captureBatchTestSubmissions(target)
	reservation, transfer := beginBatchTestTransfer(t, batcher, target)

	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "zero-first"))
	assertNoBatchTestSubmission(t, submitted)
	if _, err := batcher.enqueue(context.Background(), reservation.seq, target, batchTestPiece(t, "zero-second"), transfer); err != nil {
		t.Fatalf("enqueue second piece: %v", err)
	}
	reservation.release()
	if got := waitBatchTestSubmission(t, submitted); got != 2 {
		t.Fatalf("submitted pieces=%d, want 2", got)
	}
}

func TestUploadBatcherTransferIsTargetScoped(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), withUploadBatchClock(clock))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	other := batchTestTarget(identity, testCommitDataSetRef(2, 22))
	submitted := captureBatchTestSubmissions(target)
	reservation, _ := beginBatchTestTransfer(t, batcher, other)
	defer reservation.release()

	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "unrelated"))
	clock.Advance(3 * time.Second)
	if got := waitBatchTestSubmission(t, submitted); got != 1 {
		t.Fatalf("submitted pieces=%d, want 1", got)
	}
}

func TestUploadBatcherEnqueueEndsRejectedTransfer(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	reservation, transfer := beginBatchTestTransfer(t, batcher, target)
	defer reservation.release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := batcher.enqueue(ctx, reservation.seq, target, batchTestPiece(t, "rejected"), transfer); !errors.Is(err, context.Canceled) {
		t.Fatalf("enqueue error=%v, want context.Canceled", err)
	}
	assertNoUploadBatchTransfers(t, batcher)
}

func TestUploadBatcherCloseEndsTransfers(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	reservation, transfer := beginBatchTestTransfer(t, batcher, target)
	if err := batcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	transfer.end()
	reservation.release()
	assertNoUploadBatchTransfers(t, batcher)
	next, err := batcher.reserve()
	if !errors.Is(err, ErrClosed) || next != nil {
		t.Fatalf("reserve after Close=(%v, %v), want ErrClosed", next, err)
	}
	if _, err := reservation.beginTransfer(target); !errors.Is(err, ErrClosed) {
		t.Fatalf("beginTransfer after Close error=%v, want ErrClosed", err)
	}
}

func TestUploadBatcherCancellationRetainsAcceptedSlotAndOriginalMaxWait(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t),
		WithoutUploadIdleWait(),
		WithUploadMaxWait(12*time.Second),
		withUploadBatchClock(clock),
	)
	submitted := make(chan int, 1)
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		submitted <- len(req.Pieces)
		return &CommitSubmission{TransactionID: "0xmax", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(context.Context, CommitSubmission) (*CommitResult, error) {
		ref, _ := target.DataSetRef()
		return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1), types.NewBigInt(2)}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	first, err := batcher.enqueue(ctx, reservation.seq, target, batchTestPiece(t, "first-canceled"), nil)
	reservation.release()
	if err != nil {
		t.Fatalf("enqueue first: %v", err)
	}
	cancel()
	if _, _, err := first.wait(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("first wait error=%v, want context.Canceled", err)
	}
	clock.Advance(6 * time.Second)
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "second-remains"))
	clock.Advance(6*time.Second - time.Nanosecond)
	select {
	case <-submitted:
		t.Fatal("window submitted before the original max deadline")
	default:
	}
	clock.Advance(time.Nanosecond)
	select {
	case pieces := <-submitted:
		if pieces != 2 {
			t.Fatalf("submitted pieces=%d, want canceled waiter's accepted piece retained", pieces)
		}
	case <-time.After(time.Second):
		t.Fatal("window did not use the first accepted piece's max deadline")
	}
}

func TestUploadBatcherRejectsCancellationBeforeAcceptance(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	defer reservation.release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = batcher.enqueue(ctx, reservation.seq, batchTestTarget(identity, testCommitDataSetRef(1, 11)), batchTestPiece(t, "pre-canceled"), nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("enqueue error=%v, want context.Canceled", err)
	}
	batcher.mu.Lock()
	windows := len(batcher.windows)
	flights := len(batcher.flights)
	batcher.mu.Unlock()
	if windows != 0 || flights != 0 {
		t.Fatalf("windows=%d flights=%d, want no accepted work", windows, flights)
	}
}

func TestUploadBatcherSubmissionLimitDoesNotIncludeConfirmation(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait(), WithUploadMaxConcurrentSubmissions(1))
	waitRelease := make(chan struct{})
	submitted := make(chan string, 2)

	targetA := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	targetB := batchTestTarget(identity, testCommitDataSetRef(2, 22))
	for name, target := range map[string]*fakeUploadContext{"a": targetA, "b": targetB} {
		target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
			submitted <- name
			return &CommitSubmission{TransactionID: name, PieceCIDs: pieceCIDs(req.Pieces)}, nil
		}
		target.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
			select {
			case <-waitRelease:
				ref, _ := target.DataSetRef()
				return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	enqueueBatchTestPiece(t, batcher, targetA, batchTestPiece(t, "a"))
	enqueueBatchTestPiece(t, batcher, targetB, batchTestPiece(t, "b"))
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	for range 2 {
		select {
		case <-submitted:
		case <-time.After(time.Second):
			t.Fatal("second submission was blocked by the first confirmation")
		}
	}
	close(waitRelease)
	if err := <-flushDone; err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestUploadBatcherCallerCancellationWhileWaitingForSubmissionSlotDoesNotAbortFlight(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0), WithUploadMaxConcurrentSubmissions(1))
	first := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	second := batchTestTarget(identity, testCommitDataSetRef(2, 22))
	firstSubmitStarted := make(chan struct{})
	releaseFirstSubmit := make(chan struct{})
	secondSubmitted := make(chan struct{})
	first.submitCommitFn = func(ctx context.Context, req CommitRequest) (*CommitSubmission, error) {
		close(firstSubmitStarted)
		select {
		case <-releaseFirstSubmit:
			return &CommitSubmission{TransactionID: "0xfirst", PieceCIDs: pieceCIDs(req.Pieces)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	second.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		close(secondSubmitted)
		return &CommitSubmission{TransactionID: "0xsecond", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	for _, target := range []*fakeUploadContext{first, second} {
		target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
			ref, _ := target.DataSetRef()
			return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
		}
	}
	enqueueBatchTestPiece(t, batcher, first, batchTestPiece(t, "slot-first"))
	select {
	case <-firstSubmitStarted:
	case <-time.After(time.Second):
		t.Fatal("first submission did not occupy the global slot")
	}
	ctx, cancel := context.WithCancel(context.Background())
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	task, err := batcher.enqueue(ctx, reservation.seq, second, batchTestPiece(t, "slot-second"), nil)
	reservation.release()
	if err != nil {
		t.Fatalf("enqueue second: %v", err)
	}
	cancel()
	if _, _, err := task.wait(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("second wait error=%v, want context.Canceled", err)
	}
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	close(releaseFirstSubmit)
	select {
	case <-secondSubmitted:
	case <-time.After(time.Second):
		t.Fatal("canceled waiter's accepted flight was not submitted")
	}
	if err := <-flushDone; err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestUploadBatcherCallerCancellationDuringSigningDoesNotAbortFlight(t *testing.T) {
	identity := serviceTestIdentity()
	blocking := &blockingStorageSigner{
		StorageSigner: mustTestSigner(t),
		started:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	batcher := mustUploadBatcher(t, identity, blocking, WithUploadIdleWait(0))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	submitted := make(chan struct{})
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		close(submitted)
		return &CommitSubmission{TransactionID: "0xsign", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		ref, _ := target.DataSetRef()
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	task, err := batcher.enqueue(ctx, reservation.seq, target, batchTestPiece(t, "signing"), nil)
	reservation.release()
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("signer did not start")
	}
	cancel()
	if _, _, err := task.wait(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error=%v, want context.Canceled", err)
	}
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	close(blocking.release)
	select {
	case <-submitted:
	case <-time.After(time.Second):
		t.Fatal("canceled waiter's accepted flight was not submitted after signing")
	}
	if err := <-flushDone; err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestUploadBatchTaskPublishedTerminalResultWinsCancellationAndClose(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	configureBatchTargetCommit(target, testCommitDataSetRef(1, 11), nil)
	task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "terminal-wins"))
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := batcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	callbackCalled := false
	result, _, err := task.wait(ctx, func(string) { callbackCalled = true })
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if result == nil || len(result.PieceIDs) != 1 {
		t.Fatalf("result=%+v, want published terminal success", result)
	}
	if callbackCalled {
		t.Fatal("submission callback ran after caller cancellation")
	}
}

// Pins the contract rather than the interleaving: it does not reproduce a Close
// that races the publish itself.
func TestUploadBatcherCloseKeepsAcceptedSubmission(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	accepted := make(chan struct{})
	release := make(chan struct{})
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		return &CommitSubmission{TransactionID: "0xaccepted", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
		close(accepted)
		<-release
		return nil, ctx.Err()
	}

	task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "accepted-then-close"))
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	select {
	case <-accepted:
	case <-time.After(time.Second):
		t.Fatal("provider did not accept the batch")
	}
	if err := batcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	close(release)

	_, submission, err := task.wait(context.Background(), nil)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("wait error=%v, want ErrClosed", err)
	}
	if submission == nil || submission.TransactionID != "0xaccepted" {
		t.Fatalf("submission=%+v, want the accepted transaction 0xaccepted", submission)
	}
	if err := <-flushDone; !errors.Is(err, ErrClosed) {
		t.Fatalf("Flush error=%v, want ErrClosed", err)
	}
}

func TestUploadBatcherCloseUnblocksWaitersDuringSigning(t *testing.T) {
	identity := serviceTestIdentity()
	baseSigner := mustTestSigner(t)
	blocking := &blockingStorageSigner{
		StorageSigner: baseSigner,
		started:       make(chan struct{}),
		release:       make(chan struct{}),
	}
	batcher := mustUploadBatcher(t, identity, blocking, WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	var submitted atomic.Int32
	target.submitCommitFn = func(context.Context, CommitRequest) (*CommitSubmission, error) {
		submitted.Add(1)
		return nil, errors.New("unexpected submission")
	}
	task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "close"))
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	select {
	case <-blocking.started:
	case <-time.After(time.Second):
		t.Fatal("signer did not start")
	}
	if err := batcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, _, err := task.wait(context.Background(), nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("task error=%v, want ErrClosed", err)
	}
	if err := <-flushDone; !errors.Is(err, ErrClosed) {
		t.Fatalf("Flush error=%v, want ErrClosed", err)
	}
	close(blocking.release)
	waitForNoUploadBatchFlights(t, batcher)
	if submitted.Load() != 0 {
		t.Fatalf("submissions=%d, want 0", submitted.Load())
	}
}

func TestUploadBatcherCloseCancelsBoundContextsWithErrClosed(t *testing.T) {
	batcher := mustUploadBatcher(t, serviceTestIdentity(), mustTestSigner(t))
	ctx, release := batcher.bindContext(context.Background())
	defer release()
	if err := batcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !errors.Is(context.Cause(ctx), ErrClosed) {
		t.Fatalf("bound context cause=%v, want ErrClosed", context.Cause(ctx))
	}
}

func TestUploadBatchTaskCancellationSuppressesQueuedSubmissionCallback(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		return &CommitSubmission{TransactionID: "0xqueued", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(context.Context, CommitSubmission) (*CommitResult, error) {
		close(waitStarted)
		<-releaseWait
		ref, _ := target.DataSetRef()
		return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	task, err := batcher.enqueue(ctx, reservation.seq, target, batchTestPiece(t, "queued-callback"), nil)
	reservation.release()
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	select {
	case <-waitStarted:
	case <-time.After(time.Second):
		t.Fatal("confirmation wait did not start")
	}
	cancel()
	callbackCalled := false
	if _, _, err := task.wait(ctx, func(string) { callbackCalled = true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error=%v, want context.Canceled", err)
	}
	if callbackCalled {
		t.Fatal("submission callback ran after caller cancellation")
	}
	close(releaseWait)
	if err := <-flushDone; err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestUploadBatcherCloseCancelsHighLevelStoreAndFlushReservationWait(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	storeStarted := make(chan struct{})
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(ctx context.Context, _ io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			close(storeStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	target := mustWritableProviderContext(t, client, WithUploadBatcher(batcher))
	uploadDone := make(chan error, 1)
	go func() {
		_, err := target.Upload(context.Background(), bytes.NewReader(bytes.Repeat([]byte("close-store"), 128)), nil)
		uploadDone <- err
	}()
	select {
	case <-storeStarted:
	case <-time.After(time.Second):
		t.Fatal("Store did not start")
	}
	flushDone := make(chan error, 1)
	go func() { flushDone <- batcher.Flush(context.Background()) }()
	waitForUploadBatchFlush(t, batcher)
	if err := batcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-uploadDone:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Upload error=%v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel Store")
	}
	select {
	case err := <-flushDone:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("Flush error=%v, want ErrClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not unblock Flush reservation wait")
	}
}

func TestProviderContextUploadCancellationRetainsAcceptedSlotForFlush(t *testing.T) {
	data := bytes.Repeat([]byte("cancel-open-slot"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	var submitted atomic.Int32
	var callbacks atomic.Int32
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			submitted.Add(1)
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xcafe"),
				StatusURL: "https://sp.example.com/status",
			}, nil
		},
		waitForCreateAndAddFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xcafe"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(77)},
			}, nil
		},
	}
	target := mustWritableProviderContext(t, client, WithUploadBatcher(batcher))
	ctx, cancel := context.WithCancel(context.Background())
	uploadDone := make(chan error, 1)
	go func() {
		_, err := target.Upload(ctx, bytes.NewReader(data), &ContextUploadOptions{
			OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) { callbacks.Add(1) },
			OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) {
				callbacks.Add(1)
			},
		})
		uploadDone <- err
	}()
	waitForUploadBatchWindow(t, batcher)
	cancel()
	if err := <-uploadDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("Upload error=%v, want context.Canceled", err)
	}
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := submitted.Load(); got != 1 {
		t.Fatalf("submissions=%d, want accepted work submitted once", got)
	}
	if got := callbacks.Load(); got != 0 {
		t.Fatalf("callbacks=%d, want canceled waiter callbacks suppressed", got)
	}
}

func TestServiceUploadPreservesConfirmedCopyWhenContextCancelsDuringAnotherCommit(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	pieceInput := batchTestPiece(t, "partial-cancel")
	primaryRef := testCommitDataSetRef(1, 11)
	primary := batchTestTarget(identity, primaryRef)
	primary.storeFn = func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
		return &StoreResult{PieceCID: pieceInput.PieceCID, Size: 1}, nil
	}
	configureBatchTargetCommit(primary, primaryRef, nil)
	requireBatchedSubmit(primary)

	secondary := sharedBatchTestTarget(identity, 2)
	secondary.pullFn = func(context.Context, PullRequest) (*PullResult, error) {
		return &PullResult{Status: PullStatusComplete}, nil
	}
	secondary.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		return &CommitSubmission{TransactionID: "0xsecondary", PieceCIDs: pieceCIDs(req.Pieces), ClientDataSetID: copyBigIntPtr(req.ClientDataSetID)}, nil
	}
	secondaryCommitStarted := make(chan struct{})
	secondary.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
		close(secondaryCommitStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	service := mustNewService(t, Options{UploadBatcher: batcher})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var confirmedCallbacks atomic.Int32
	type outcome struct {
		result *UploadResult
		err    error
	}
	uploadDone := make(chan outcome, 1)
	go func() {
		result, err := service.UploadToContexts(ctx, bytes.NewReader([]byte("ignored")), []StorageContext{primary, secondary}, &UploadToContextsOptions{
			OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) {
				confirmedCallbacks.Add(1)
			},
		})
		uploadDone <- outcome{result: result, err: err}
	}()
	select {
	case <-secondaryCommitStarted:
	case <-time.After(time.Second):
		t.Fatal("secondary commit did not start")
	}
	waitForUploadBatchFlightCount(t, batcher, 1)
	cancel()
	select {
	case got := <-uploadDone:
		if got.err != nil {
			t.Fatalf("UploadToContexts error=%v, want confirmed primary result", got.err)
		}
		if got.result == nil || got.result.Complete || len(got.result.Copies) != 1 || got.result.Copies[0].Role != CopyRolePrimary {
			t.Fatalf("result=%+v, want one confirmed primary copy and incomplete upload", got.result)
		}
	case <-time.After(time.Second):
		t.Fatal("UploadToContexts did not return after cancellation")
	}
	if got := confirmedCallbacks.Load(); got != 0 {
		t.Fatalf("OnPiecesConfirmed callbacks=%d, want suppressed after cancellation", got)
	}
}

func TestServiceUploadPreservesConfirmedCopyWhenContextCancelsDuringSecondaryPull(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	pieceInput := batchTestPiece(t, "pull-cancel")
	primaryRef := testCommitDataSetRef(1, 11)
	primary := batchTestTarget(identity, primaryRef)
	primary.storeFn = func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
		return &StoreResult{PieceCID: pieceInput.PieceCID, Size: 1}, nil
	}
	configureBatchTargetCommit(primary, primaryRef, nil)
	requireBatchedSubmit(primary)

	secondary := batchTestTarget(identity, DataSetRef{})
	secondary.id = types.NewBigInt(2)
	secondary.dataSetID = nil
	secondary.clientDataSetID = nil
	secondary.presignFn = func(context.Context, []PieceInput) ([]byte, error) {
		return []byte{0x01}, nil
	}
	secondaryPullStarted := make(chan struct{})
	secondary.pullFn = func(ctx context.Context, _ PullRequest) (*PullResult, error) {
		close(secondaryPullStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	secondary.commitFn = func(context.Context, CommitRequest) (*CommitResult, error) {
		return nil, errors.New("canceled secondary must not commit")
	}
	service := mustNewService(t, Options{UploadBatcher: batcher})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var confirmedCallbacks atomic.Int32
	type outcome struct {
		result *UploadResult
		err    error
	}
	uploadDone := make(chan outcome, 1)
	go func() {
		result, err := service.UploadToContexts(ctx, bytes.NewReader([]byte("ignored")), []StorageContext{primary, secondary}, &UploadToContextsOptions{
			OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) {
				confirmedCallbacks.Add(1)
			},
		})
		uploadDone <- outcome{result: result, err: err}
	}()
	select {
	case <-secondaryPullStarted:
	case <-time.After(time.Second):
		t.Fatal("secondary pull did not start")
	}
	waitForNoUploadBatchFlights(t, batcher)
	cancel()
	select {
	case got := <-uploadDone:
		if got.err != nil {
			t.Fatalf("UploadToContexts error=%v, want confirmed primary result", got.err)
		}
		if got.result == nil || got.result.Complete || len(got.result.Copies) != 1 || got.result.Copies[0].Role != CopyRolePrimary {
			t.Fatalf("result=%+v, want one confirmed primary copy and incomplete upload", got.result)
		}
	case <-time.After(time.Second):
		t.Fatal("UploadToContexts did not return after cancellation")
	}
	if got := confirmedCallbacks.Load(); got != 0 {
		t.Fatalf("OnPiecesConfirmed callbacks=%d, want suppressed after cancellation", got)
	}
	assertNoUploadBatchTransfers(t, batcher)
}

func TestUploadBatchContextErrorPreservesPublishedFailure(t *testing.T) {
	rejected := &CommitRejectedError{
		Submission: CommitSubmission{TransactionID: "0xrej", ProviderID: types.NewBigInt(1)},
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	err := uploadBatchContextError(canceled, rejected)
	var got *CommitRejectedError
	if !errors.As(err, &got) || !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("error=%v, want published CommitRejectedError", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("published rejection was replaced by context.Canceled")
	}

	closed, closeCancel := context.WithCancelCause(context.Background())
	closeCancel(ErrClosed)
	if err := uploadBatchContextError(closed, nil); !errors.Is(err, ErrClosed) {
		t.Fatalf("nil fallback with closed ctx: %v, want ErrClosed", err)
	}
	if err := uploadBatchContextError(closed, context.Canceled); !errors.Is(err, ErrClosed) {
		t.Fatalf("canceled fallback with closed ctx: %v, want ErrClosed", err)
	}
	if err := uploadBatchContextError(canceled, ErrClosed); !errors.Is(err, ErrClosed) {
		t.Fatalf("ErrClosed fallback with canceled ctx: %v, want ErrClosed", err)
	}
	if err := uploadBatchContextError(canceled, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("nil fallback with canceled ctx: %v, want context.Canceled", err)
	}
}

func TestDataSetContextUploadKeepsCommitRejectionWhenCallerCancelsAfterSubmit(t *testing.T) {
	data := bytes.Repeat([]byte("keep-rejection"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x33")
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{
				TxHash:    originalTx,
				StatusURL: "https://sp.example.com/status/add",
			}, nil
		},
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:       originalTx,
				TxStatus:     "failed",
				DataSetID:    ref.DataSetID(),
				PieceCount:   0,
				AddMessageOK: new(false),
			}, pdp.ErrTxRejected
		},
	}
	target := mustWritableDataSetContext(t, client, ref)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = target.Upload(ctx, bytes.NewReader(data), &ContextUploadOptions{
		OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) {
			cancel()
		},
	})
	var rejected *CommitRejectedError
	if !errors.As(err, &rejected) || !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("Upload error=%v, want CommitRejectedError", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("published rejection was replaced by context.Canceled")
	}
}

func TestDataSetContextUploadKeepsBatchedCommitRejectionWhenCallerCancelsAfterSubmit(t *testing.T) {
	data := bytes.Repeat([]byte("keep-batched-rejection"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	originalTx := common.HexToHash("0x44")
	waitStarted := make(chan struct{})
	releaseWait := make(chan struct{})
	var startOnce sync.Once
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{
				TxHash:    originalTx,
				StatusURL: "https://sp.example.com/status/add",
			}, nil
		},
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			startOnce.Do(func() { close(waitStarted) })
			<-releaseWait
			return &pdp.AddPiecesStatus{
				TxHash:       originalTx,
				TxStatus:     "failed",
				DataSetID:    ref.DataSetID(),
				PieceCount:   0,
				AddMessageOK: new(false),
			}, pdp.ErrTxRejected
		},
	}
	target := mustWritableDataSetContext(t, client, ref, WithUploadBatcher(batcher))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err = target.Upload(ctx, bytes.NewReader(data), &ContextUploadOptions{
		OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) {
			select {
			case <-waitStarted:
			case <-time.After(time.Second):
				t.Error("WaitForCommit did not start")
				close(releaseWait)
				cancel()
				return
			}
			close(releaseWait)
			waitForNoUploadBatchFlights(t, batcher)
			cancel()
		},
	})
	var rejected *CommitRejectedError
	if !errors.As(err, &rejected) || !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("Upload error=%v, want CommitRejectedError", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Fatalf("published rejection was replaced by context.Canceled")
	}
	commitErr, ok := errors.AsType[*CommitError](err)
	if !ok || len(commitErr.FailedAttempts) != 1 {
		t.Fatalf("Upload error=%#v, want CommitError with one failed attempt", err)
	}
	if got := commitErr.FailedAttempts[0].Submission; got == nil || got.TransactionID != originalTx.Hex() {
		t.Fatalf("Submission=%+v, want the batched submission %s", got, originalTx.Hex())
	}
}

func TestDataSetContextUploadKeepsConfirmedResultWhenCallerCancelsAfterSubmit(t *testing.T) {
	data := bytes.Repeat([]byte("keep-confirmed"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x11")
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{
				TxHash:    originalTx,
				StatusURL: "https://sp.example.com/status/add",
			}, nil
		},
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            originalTx,
				ConfirmedTxHash:   common.HexToHash("0x22"),
				TxStatus:          "confirmed",
				DataSetID:         ref.DataSetID(),
				PieceCount:        1,
				AddMessageOK:      new(true),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(9)},
			}, nil
		},
	}
	target := mustWritableDataSetContext(t, client, ref)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var confirmedCallbacks atomic.Int32
	result, err := target.Upload(ctx, bytes.NewReader(data), &ContextUploadOptions{
		OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) {
			cancel()
		},
		OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) {
			confirmedCallbacks.Add(1)
		},
	})
	if err != nil {
		t.Fatalf("Upload error=%v, want confirmed result", err)
	}
	if result == nil || len(result.Copies) != 1 || !result.Copies[0].PieceID.Equal(types.NewBigInt(9)) {
		t.Fatalf("result=%+v, want confirmed piece ID 9", result)
	}
	if got := confirmedCallbacks.Load(); got != 0 {
		t.Fatalf("OnPiecesConfirmed callbacks=%d, want suppressed after cancellation", got)
	}
}

func TestProviderContextUploadCancelInOnStoredReturnsCommitError(t *testing.T) {
	data := bytes.Repeat([]byte("onstored-context"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stored bool
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		createAndAddFn: func(ctx context.Context, _ common.Address, _ []pdp.AddPieceInput, _ []byte) (*pdp.CreateDataSetResult, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, errors.New("create-and-add must not run after cancel")
		},
	}
	target := mustWritableProviderContext(t, client)
	_, err = target.Upload(ctx, bytes.NewReader(data), &ContextUploadOptions{
		OnStored: func(types.BigInt, cid.Cid) {
			stored = true
			cancel()
		},
	})
	if !stored {
		t.Fatal("OnStored was not called")
	}
	if _, ok := errors.AsType[*CommitError](err); !ok {
		t.Fatalf("error=%v (%T), want CommitError", err, err)
	}
}

func TestProviderContextUploadRejectsBatcherIdentityBeforeStore(t *testing.T) {
	identity := serviceTestIdentity()
	identity.Payer = common.HexToAddress("0x9999")
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t))
	var stores atomic.Int32
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(context.Context, io.Reader, pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			stores.Add(1)
			return nil, errors.New("store must not run")
		},
	}
	target := mustWritableProviderContext(t, client, WithUploadBatcher(batcher))
	_, err := target.Upload(context.Background(), bytes.NewReader(bytes.Repeat([]byte("identity"), 128)), nil)
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("Upload error=%v, want identity ErrInvalidArgument", err)
	}
	if got := stores.Load(); got != 0 {
		t.Fatalf("Store calls=%d, want 0", got)
	}
}

func TestServiceUploadToContextsBatchesConcurrentCommits(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	ref := testCommitDataSetRef(1, 11)
	target := batchTestTarget(identity, ref)
	storesReady := make(chan struct{})
	releaseStores := make(chan struct{})
	var storeCount atomic.Int32
	target.storeFn = func(ctx context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		info, err := piece.CalculateFromBytes(data)
		if err != nil {
			return nil, err
		}
		if storeCount.Add(1) == 2 {
			close(storesReady)
		}
		select {
		case <-releaseStores:
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	var submitCount atomic.Int32
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		submitCount.Add(1)
		return &CommitSubmission{TransactionID: "0xshared", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		pieceIDs := make([]types.BigInt, len(submission.PieceCIDs))
		for i := range pieceIDs {
			pieceIDs[i] = types.NewBigInt(uint64(i + 1))
		}
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: pieceIDs}, nil
	}
	requireBatchedSubmit(target)
	service := mustNewService(t, Options{UploadBatcher: batcher})

	type outcome struct {
		result *UploadResult
		tx     string
		err    error
	}
	outcomes := make(chan outcome, 2)
	for _, data := range [][]byte{bytes.Repeat([]byte("first"), 128), bytes.Repeat([]byte("second"), 128)} {
		data := append([]byte(nil), data...)
		go func() {
			var tx string
			result, err := service.UploadToContexts(context.Background(), bytes.NewReader(data), []StorageContext{target}, &UploadToContextsOptions{
				OnPiecesAdded: func(got string, _ types.BigInt, pieces []SubmittedPiece) {
					tx = got
					if len(pieces) != 1 {
						t.Errorf("OnPiecesAdded pieces=%d, want 1", len(pieces))
					}
				},
			})
			outcomes <- outcome{result: result, tx: tx, err: err}
		}()
	}
	select {
	case <-storesReady:
	case <-time.After(time.Second):
		t.Fatal("concurrent stores did not start")
	}
	flushDone := make(chan error, 1)
	go func() { flushDone <- service.Flush(context.Background()) }()
	close(releaseStores)
	if err := <-flushDone; err != nil {
		t.Fatalf("Flush: %v", err)
	}
	for range 2 {
		got := <-outcomes
		if got.err != nil {
			t.Fatalf("UploadToContexts: %v", got.err)
		}
		if got.tx != "0xshared" {
			t.Errorf("OnPiecesAdded tx=%q, want 0xshared", got.tx)
		}
		if got.result == nil || len(got.result.Copies) != 1 {
			t.Fatalf("result=%+v, want one copy", got.result)
		}
	}
	if got := submitCount.Load(); got != 1 {
		t.Fatalf("SubmitCommit calls=%d, want 1", got)
	}
}

func TestServiceUploadToContextsWaitsForInProgressStore(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), withUploadBatchClock(clock))
	ref := testCommitDataSetRef(1, 11)
	target := batchTestTarget(identity, ref)
	slowStoreStarted := make(chan struct{})
	releaseSlowStore := make(chan struct{})
	target.storeFn = func(ctx context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		info, err := piece.CalculateFromBytes(data)
		if err != nil {
			return nil, err
		}
		if bytes.HasPrefix(data, []byte("slow")) {
			close(slowStoreStarted)
			select {
			case <-releaseSlowStore:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
	}
	submitted := captureBatchTestSubmissions(target)
	requireBatchedSubmit(target)
	service := mustNewService(t, Options{UploadBatcher: batcher})

	outcomes := make(chan error, 2)
	upload := func(data []byte) {
		_, err := service.UploadToContexts(context.Background(), bytes.NewReader(data), []StorageContext{target}, nil)
		outcomes <- err
	}
	go upload(bytes.Repeat([]byte("slow"), 128))
	select {
	case <-slowStoreStarted:
	case <-time.After(time.Second):
		t.Fatal("slow store did not start")
	}
	go upload(bytes.Repeat([]byte("fast"), 128))
	waitForUploadBatchWindowSlots(t, batcher, 1)

	clock.Advance(5 * time.Second)
	assertNoBatchTestSubmission(t, submitted)
	close(releaseSlowStore)
	waitForUploadBatchWindowSlots(t, batcher, 2)
	clock.Advance(3 * time.Second)
	if got := waitBatchTestSubmission(t, submitted); got != 2 {
		t.Fatalf("submitted pieces=%d, want 2", got)
	}
	for range 2 {
		if err := <-outcomes; err != nil {
			t.Fatalf("UploadToContexts: %v", err)
		}
	}
	assertNoUploadBatchTransfers(t, batcher)
}

func TestServiceFlushWithoutBatcherIsNoop(t *testing.T) {
	service := mustNewService(t, Options{})
	if err := service.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestServiceUploadToContextsResignsExistingSecondaryForBatch(t *testing.T) {
	identity := serviceTestIdentity()
	storageSigner := &atomicStorageSigner{StorageSigner: mustTestSigner(t)}
	batcher := mustUploadBatcher(t, identity, storageSigner, WithoutUploadIdleWait(), WithoutUploadMaxWait())
	primaryRef := testCommitDataSetRef(1, 11)
	secondaryRef := testCommitDataSetRef(2, 22)
	primary := batchTestTarget(identity, primaryRef)
	secondary := batchTestTarget(identity, secondaryRef)
	pieceInput := batchTestPiece(t, "secondary-resign")
	primary.storeFn = func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
		return &StoreResult{PieceCID: pieceInput.PieceCID, Size: 1}, nil
	}
	pullAuthorization := make(chan []byte, 1)
	secondary.presignFn = func(context.Context, []PieceInput) ([]byte, error) {
		return nil, errors.New("context signer must not authorize a batched pull")
	}
	secondary.pullFn = func(_ context.Context, req PullRequest) (*PullResult, error) {
		pullAuthorization <- append([]byte(nil), req.ExtraData...)
		return &PullResult{Status: PullStatusComplete}, nil
	}
	commitAuthorization := make(chan []byte, 1)
	configureBatchTargetCommit(primary, primaryRef, nil)
	configureBatchTargetCommit(secondary, secondaryRef, commitAuthorization)
	service := mustNewService(t, Options{UploadBatcher: batcher})
	uploadDone := make(chan error, 1)
	go func() {
		_, err := service.UploadToContexts(context.Background(), bytes.NewReader([]byte("ignored")), []StorageContext{primary, secondary}, nil)
		uploadDone <- err
	}()
	var pullExtra []byte
	select {
	case pullExtra = <-pullAuthorization:
	case <-time.After(time.Second):
		t.Fatal("secondary pull was not authorized")
	}
	if err := service.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if err := <-uploadDone; err != nil {
		t.Fatalf("UploadToContexts: %v", err)
	}
	var commitExtra []byte
	select {
	case commitExtra = <-commitAuthorization:
	default:
		t.Fatal("secondary batch commit was not submitted")
	}
	if bytes.Equal(pullExtra, commitExtra) {
		t.Fatal("secondary pull authorization was reused for the batch commit")
	}
	if got := storageSigner.calls.Load(); got != 3 {
		t.Fatalf("batcher signer calls=%d, want 3", got)
	}
}

func TestProviderContextUploadUsesInjectedBatcher(t *testing.T) {
	data := bytes.Repeat([]byte("context-batch"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	var submittedPieces int
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		createAndAddFn: func(_ context.Context, recordKeeper common.Address, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.CreateDataSetResult, error) {
			if recordKeeper != testRecordKeeper() || len(extraData) == 0 {
				t.Fatalf("CreateDataSetAndAddPieces recordKeeper=%s extraData=%x", recordKeeper, extraData)
			}
			submittedPieces = len(pieces)
			return &pdp.CreateDataSetResult{TxHash: common.HexToHash("0xabc"), StatusURL: "https://sp.example.com/status"}, nil
		},
		waitForCreateAndAddFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xabc"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(77)},
			}, nil
		},
	}
	target := mustWritableProviderContext(t, client, WithUploadBatcher(batcher))
	result, err := target.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if submittedPieces != 1 || result == nil || len(result.Copies) != 1 || !result.Copies[0].PieceID.Equal(types.NewBigInt(77)) {
		t.Fatalf("submittedPieces=%d result=%+v", submittedPieces, result)
	}
	if _, bound := target.DataSetRef(); bound {
		t.Fatal("batched upload mutated ProviderContext")
	}
}

func TestUploadBatcherSharesNewDataSetAcrossPieceLimit(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := sharedBatchTestTarget(identity, 1)
	commits := recordSharedBatchCommits(target)
	tasks := make([]*uploadBatchTask, 0, pdp.MaxAddPiecesBatchSize+1)
	for i := range pdp.MaxAddPiecesBatchSize + 1 {
		tasks = append(tasks, enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, fmt.Sprintf("shared-limit-%d", i))))
	}
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	got := commits.snapshot()
	if len(got) != 2 || !got[0].create || got[0].pieces != pdp.MaxAddPiecesBatchSize || got[1].create || got[1].pieces != 1 {
		t.Fatalf("commits=%+v, want one full create-and-add followed by one add-pieces", got)
	}
	for i, task := range tasks {
		result, _, err := task.wait(context.Background(), nil)
		if err != nil {
			t.Fatalf("wait %d: %v", i, err)
		}
		if !result.DataSet.DataSetID().Equal(types.NewBigInt(101)) || result.IsNewDataSet != (i < pdp.MaxAddPiecesBatchSize) {
			t.Fatalf("result %d=%+v, want shared data set 101", i, result)
		}
	}
}

func TestUploadBatcherLaterWindowWaitsForPendingCreate(t *testing.T) {
	identity := serviceTestIdentity()
	clock := newManualUploadBatchClock()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(time.Second), WithoutUploadMaxWait(), withUploadBatchClock(clock))
	target := sharedBatchTestTarget(identity, 1)
	commits := recordSharedBatchCommits(target)
	createWaiting := make(chan struct{})
	releaseCreate := make(chan struct{})
	commits.waitHook = func(_ context.Context, commit sharedBatchCommit) error {
		if commit.create {
			close(createWaiting)
			<-releaseCreate
		}
		return nil
	}
	first := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "pending-first"))
	clock.Advance(time.Second)
	select {
	case <-createWaiting:
	case <-time.After(time.Second):
		t.Fatal("create-and-add was not submitted")
	}
	second := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "pending-second"))
	clock.Advance(time.Second)
	waitForUploadBatchFlightCount(t, batcher, 2)
	if got := commits.snapshot(); len(got) != 1 {
		t.Fatalf("commits=%+v, want later window to wait for the pending create", got)
	}
	close(releaseCreate)
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	firstResult, _, err := first.wait(context.Background(), nil)
	if err != nil {
		t.Fatalf("wait first: %v", err)
	}
	secondResult, _, err := second.wait(context.Background(), nil)
	if err != nil {
		t.Fatalf("wait second: %v", err)
	}
	if !secondResult.DataSet.Equal(firstResult.DataSet) || secondResult.IsNewDataSet {
		t.Fatalf("second result=%+v, want add-pieces to %+v", secondResult, firstResult.DataSet)
	}
	if got := commits.snapshot(); len(got) != 2 || !got[0].create || got[1].create {
		t.Fatalf("commits=%+v, want create-and-add then add-pieces", got)
	}
}

func TestUploadBatcherReusesSharedDataSetForLaterUploads(t *testing.T) {
	tests := map[string][2]string{
		"sequential upload": {"sequential-first", "sequential-second"},
		"repeated piece":    {"repeated-piece", "repeated-piece"},
	}
	for name, values := range tests {
		t.Run(name, func(t *testing.T) {
			identity := serviceTestIdentity()
			batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
			target := sharedBatchTestTarget(identity, 1)
			commits := recordSharedBatchCommits(target)
			var results []*CommitResult
			for _, value := range values {
				task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, value))
				if err := batcher.Flush(context.Background()); err != nil {
					t.Fatalf("Flush: %v", err)
				}
				result, _, err := task.wait(context.Background(), nil)
				if err != nil {
					t.Fatalf("wait: %v", err)
				}
				results = append(results, result)
			}
			if !results[1].DataSet.Equal(results[0].DataSet) || !results[0].IsNewDataSet || results[1].IsNewDataSet {
				t.Fatalf("results=%+v, want the second upload added to the first data set", results)
			}
			if got := commits.snapshot(); len(got) != 2 || !got[0].create || got[1].create {
				t.Fatalf("commits=%+v, want create-and-add then add-pieces", got)
			}
			if _, bound := target.DataSetRef(); bound {
				t.Fatal("batching mutated the unbound target")
			}
		})
	}
}

func TestUploadBatcherRecoversSharedDataSetAfterFailedCreate(t *testing.T) {
	recovered := testCommitDataSetRef(1, 555)
	tests := []struct {
		name       string
		find       func(DataSetRef) func(context.Context, types.BigInt) (DataSetRef, bool, error)
		wantCreate bool
		wantSameID bool
	}{
		{
			name: "found on chain",
			find: func(ref DataSetRef) func(context.Context, types.BigInt) (DataSetRef, bool, error) {
				return func(context.Context, types.BigInt) (DataSetRef, bool, error) { return ref, true, nil }
			},
		},
		{
			name: "not found",
			find: func(DataSetRef) func(context.Context, types.BigInt) (DataSetRef, bool, error) {
				return func(context.Context, types.BigInt) (DataSetRef, bool, error) { return DataSetRef{}, false, nil }
			},
			wantCreate: true,
			wantSameID: true,
		},
		{
			name:       "no chain reader",
			wantCreate: true,
			wantSameID: true,
		},
		{
			name: "correlation conflict",
			find: func(DataSetRef) func(context.Context, types.BigInt) (DataSetRef, bool, error) {
				return func(context.Context, types.BigInt) (DataSetRef, bool, error) {
					return DataSetRef{}, false, ErrDataSetCorrelationConflict
				}
			},
			wantCreate: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := serviceTestIdentity()
			batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
			target := sharedBatchTestTarget(identity, 1)
			commits := recordSharedBatchCommits(target)
			var failed atomic.Bool
			commits.waitHook = func(_ context.Context, commit sharedBatchCommit) error {
				if commit.create && failed.CompareAndSwap(false, true) {
					return errors.New("provider status unavailable")
				}
				return nil
			}
			enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "recover-first"))
			if err := batcher.Flush(context.Background()); err == nil {
				t.Fatal("Flush error=nil, want failed create")
			}
			first := commits.snapshot()[0]
			ref := recovered
			if tt.find != nil {
				ref = testCommitDataSetRefWithClientID(t, 1, 555, first.clientDataSetID)
				target.findDataSetFn = tt.find(ref)
				commits.useDataSet(ref)
			}
			task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "recover-second"))
			if err := batcher.Flush(context.Background()); err != nil {
				t.Fatalf("Flush after recovery: %v", err)
			}
			result, _, err := task.wait(context.Background(), nil)
			if err != nil {
				t.Fatalf("wait: %v", err)
			}
			got := commits.snapshot()
			if len(got) != 2 || got[1].create != tt.wantCreate {
				t.Fatalf("commits=%+v, want create=%t after recovery", got, tt.wantCreate)
			}
			if tt.wantCreate {
				if got[1].clientDataSetID.Equal(first.clientDataSetID) != tt.wantSameID {
					t.Fatalf("retry client data-set ID=%s first=%s, want same=%t", got[1].clientDataSetID, first.clientDataSetID, tt.wantSameID)
				}
				return
			}
			if !result.DataSet.Equal(ref) {
				t.Fatalf("result data set=%+v, want recovered %+v", result.DataSet, ref)
			}
		})
	}
}

func TestUploadBatcherWaitsForCreatedDataSetVisibility(t *testing.T) {
	t.Run("visible", func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait(), withSharedDataSetPolling(time.Millisecond, time.Second))
		target := sharedBatchTestTarget(identity, 1)
		commits := recordSharedBatchCommits(target)
		var lookups atomic.Int32
		target.findDataSetFn = func(context.Context, types.BigInt) (DataSetRef, bool, error) {
			if lookups.Add(1) < 3 {
				return DataSetRef{}, false, nil
			}
			return commits.lastDataSet(), true, nil
		}
		enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "visible-first"))
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatalf("Flush first: %v", err)
		}
		task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "visible-second"))
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatalf("Flush second: %v", err)
		}
		if _, _, err := task.wait(context.Background(), nil); err != nil {
			t.Fatalf("wait: %v", err)
		}
		if got := lookups.Load(); got != 3 {
			t.Fatalf("lookups=%d, want add-pieces after the data set became visible", got)
		}
		if got := commits.snapshot(); len(got) != 2 || got[1].create {
			t.Fatalf("commits=%+v, want add-pieces after visibility", got)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait(), withSharedDataSetPolling(time.Millisecond, 5*time.Millisecond))
		target := sharedBatchTestTarget(identity, 1)
		commits := recordSharedBatchCommits(target)
		target.findDataSetFn = func(context.Context, types.BigInt) (DataSetRef, bool, error) {
			return DataSetRef{}, false, nil
		}
		enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "hidden-first"))
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatalf("Flush first: %v", err)
		}
		task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "hidden-second"))
		if err := batcher.Flush(context.Background()); !errors.Is(err, ErrDataSetUnavailable) {
			t.Fatalf("Flush error=%v, want ErrDataSetUnavailable", err)
		}
		if _, _, err := task.wait(context.Background(), nil); !errors.Is(err, ErrDataSetUnavailable) {
			t.Fatalf("wait error=%v, want ErrDataSetUnavailable", err)
		}
		enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "hidden-third"))
		if err := batcher.Flush(context.Background()); !errors.Is(err, ErrDataSetUnavailable) {
			t.Fatalf("Flush third error=%v, want ErrDataSetUnavailable", err)
		}
		if got := commits.snapshot(); len(got) != 1 {
			t.Fatalf("commits=%+v, want no create or add-pieces while the confirmed data set is invisible", got)
		}
		target.findDataSetFn = func(context.Context, types.BigInt) (DataSetRef, bool, error) {
			return commits.lastDataSet(), true, nil
		}
		visible := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "hidden-visible"))
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatalf("Flush after visibility: %v", err)
		}
		result, _, err := visible.wait(context.Background(), nil)
		if err != nil {
			t.Fatalf("wait after visibility: %v", err)
		}
		if !result.DataSet.DataSetID().Equal(types.NewBigInt(101)) || result.IsNewDataSet {
			t.Fatalf("result=%+v, want add-pieces to confirmed data set 101", result)
		}
	})
}

func TestUploadBatcherReplacesTerminatedSharedDataSet(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	target := sharedBatchTestTarget(identity, 1)
	commits := recordSharedBatchCommits(target)
	var terminated atomic.Bool
	commits.submitHook = func(commit sharedBatchCommit) error {
		if !commit.create && terminated.CompareAndSwap(false, true) {
			return fmt.Errorf("validate data set: %w", &DataSetPDPPaymentTerminatedError{DataSetID: types.NewBigInt(101), PDPEndEpoch: 10})
		}
		return nil
	}
	enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "terminated-first"))
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush first: %v", err)
	}
	task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "terminated-second"))
	if err := batcher.Flush(context.Background()); err != nil {
		t.Fatalf("Flush second: %v", err)
	}
	result, _, err := task.wait(context.Background(), nil)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	got := commits.snapshot()
	if len(got) != 3 || !got[0].create || got[1].create || !got[2].create || got[2].clientDataSetID.Equal(got[0].clientDataSetID) {
		t.Fatalf("commits=%+v, want replacement create with a new client data-set ID", got)
	}
	if !result.DataSet.DataSetID().Equal(types.NewBigInt(102)) || !result.IsNewDataSet {
		t.Fatalf("result=%+v, want replacement data set 102", result)
	}
}

func TestUploadBatcherCloseStopsWindowsWaitingForCreate(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	target := sharedBatchTestTarget(identity, 1)
	commits := recordSharedBatchCommits(target)
	createWaiting := make(chan struct{})
	commits.waitHook = func(ctx context.Context, commit sharedBatchCommit) error {
		if commit.create {
			close(createWaiting)
			<-ctx.Done()
			return ctx.Err()
		}
		return nil
	}
	first := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "close-first"))
	select {
	case <-createWaiting:
	case <-time.After(time.Second):
		t.Fatal("create-and-add was not submitted")
	}
	second := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "close-second"))
	waitForUploadBatchFlightCount(t, batcher, 2)
	if err := batcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for name, task := range map[string]*uploadBatchTask{"first": first, "second": second} {
		if _, _, err := task.wait(context.Background(), nil); !errors.Is(err, ErrClosed) {
			t.Fatalf("wait %s error=%v, want ErrClosed", name, err)
		}
	}
	waitForNoUploadBatchFlights(t, batcher)
	if got := commits.snapshot(); len(got) != 1 {
		t.Fatalf("commits=%+v, want waiting window never submitted", got)
	}
}

func TestUploadBatcherRejectsUnboundTargetThatCannotShare(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	defer reservation.release()
	target := opaqueStorageContext{StorageContext: sharedBatchTestTarget(identity, 1)}
	if _, err := batcher.enqueue(context.Background(), reservation.seq, target, batchTestPiece(t, "opaque"), nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("enqueue error=%v, want ErrInvalidArgument", err)
	}
}

func TestServiceUploadToContextsSharesUnboundSecondaryCreate(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait())
	const uploads = 3
	primary, secondary := sharedServiceTestTargets(t, identity, uploads)
	pullIDs := make(chan types.BigInt, uploads)
	secondary.pullFn = func(_ context.Context, req PullRequest) (*PullResult, error) {
		clientDataSetID, err := decodeCreateAndAddIdentity("test", req.ExtraData, identity.Payer)
		if err != nil {
			t.Errorf("pull extraData is not create-and-add: %v", err)
			return nil, err
		}
		pullIDs <- *clientDataSetID
		return &PullResult{Status: PullStatusComplete}, nil
	}
	commits := recordSharedBatchCommits(secondary)
	service := mustNewService(t, Options{UploadBatcher: batcher})
	outcomes := startSharedServiceUploads(service, primary, secondary, uploads)
	var ids []types.BigInt
	for range uploads {
		select {
		case id := <-pullIDs:
			ids = append(ids, id)
		case <-time.After(time.Second):
			t.Fatal("secondary pull was not authorized")
		}
	}
	if err := service.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	assertSharedSecondaryUploads(t, outcomes, uploads)
	got := commits.snapshot()
	if len(got) != 1 || !got[0].create || got[0].pieces != uploads {
		t.Fatalf("secondary commits=%+v, want one create-and-add for every upload", got)
	}
	for _, id := range ids {
		if !id.Equal(got[0].clientDataSetID) {
			t.Fatalf("pull client data-set IDs=%v, want every pull to authorize %s", ids, got[0].clientDataSetID)
		}
	}
}

func TestServiceUploadToContextsPullWaitsForSubmittedSecondaryCreate(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	primary, secondary := sharedServiceTestTargets(t, identity, 2)
	pullCreates := make(chan bool, 2)
	secondary.pullFn = func(_ context.Context, req PullRequest) (*PullResult, error) {
		_, err := decodeCreateAndAddIdentity("test", req.ExtraData, identity.Payer)
		pullCreates <- err == nil
		return &PullResult{Status: PullStatusComplete}, nil
	}
	commits := recordSharedBatchCommits(secondary)
	createWaiting := make(chan struct{})
	releaseCreate := make(chan struct{})
	commits.waitHook = func(_ context.Context, commit sharedBatchCommit) error {
		if commit.create {
			close(createWaiting)
			<-releaseCreate
		}
		return nil
	}
	service := mustNewService(t, Options{UploadBatcher: batcher})
	first := startSharedServiceUploads(service, primary, secondary, 1)
	if create := <-pullCreates; !create {
		t.Fatal("first secondary pull did not authorize create-and-add")
	}
	select {
	case <-createWaiting:
	case <-time.After(time.Second):
		t.Fatal("secondary create-and-add was not submitted")
	}
	second := startSharedServiceUploads(service, primary, secondary, 1)
	select {
	case <-pullCreates:
		t.Fatal("secondary pull started before the submitted create was confirmed")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseCreate)
	select {
	case create := <-pullCreates:
		if create {
			t.Fatal("later secondary pull authorized another create-and-add")
		}
	case <-time.After(time.Second):
		t.Fatal("later secondary pull did not start after confirmation")
	}
	assertSharedSecondaryUploads(t, first, 1)
	assertSharedSecondaryUploads(t, second, 1)
	if got := commits.snapshot(); len(got) != 2 || !got[0].create || got[1].create {
		t.Fatalf("secondary commits=%+v, want create-and-add then add-pieces", got)
	}
}

func TestProviderContextUploadReusesBatcherDataSet(t *testing.T) {
	payloads := [][]byte{bytes.Repeat([]byte("context-reuse-a"), 128), bytes.Repeat([]byte("context-reuse-b"), 128)}
	pieceCIDs := make([]cid.Cid, len(payloads))
	for i, payload := range payloads {
		info, err := piece.CalculateFromBytes(payload)
		if err != nil {
			t.Fatalf("CalculateFromBytes: %v", err)
		}
		pieceCIDs[i] = info.CIDv2
	}
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	var uploadsSeen atomic.Int32
	var addedDataSet types.BigInt
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			if _, err := io.Copy(io.Discard, r); err != nil {
				return nil, err
			}
			i := uploadsSeen.Add(1) - 1
			return &pdp.UploadStreamingResult{PieceCID: pieceCIDs[i], Size: int64(len(payloads[i]))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{TxHash: common.HexToHash("0xabc"), StatusURL: "https://sp.example.com/create"}, nil
		},
		waitForCreateAndAddFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xabc"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(77)},
			}, nil
		},
		addPiecesFn: func(_ context.Context, dataSetID types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			addedDataSet = dataSetID
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0xdef"), StatusURL: "https://sp.example.com/add"}, nil
		},
		waitForAddedFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xdef"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(78)},
			}, nil
		},
	}
	target := mustWritableProviderContext(t, client, WithUploadBatcher(batcher))
	var results []*UploadResult
	for _, payload := range payloads {
		result, err := target.Upload(context.Background(), bytes.NewReader(payload), nil)
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		results = append(results, result)
	}
	if !addedDataSet.Equal(types.NewBigInt(55)) {
		t.Fatalf("add-pieces data set=%s, want 55", addedDataSet)
	}
	second := results[1].Copies[0]
	if !second.DataSetID.Equal(results[0].Copies[0].DataSetID) || second.IsNewDataSet || !second.PieceID.Equal(types.NewBigInt(78)) {
		t.Fatalf("second copy=%+v, want piece 78 added to data set 55", second)
	}
	if _, bound := target.DataSetRef(); bound {
		t.Fatal("batched upload mutated ProviderContext")
	}
}

type opaqueStorageContext struct {
	StorageContext
}

type sharedBatchCommit struct {
	create          bool
	clientDataSetID types.BigInt
	pieces          int
}

type sharedBatchCommits struct {
	mu         sync.Mutex
	providerID types.BigInt
	commits    []sharedBatchCommit
	dataSets   []DataSetRef
	submitHook func(sharedBatchCommit) error
	waitHook   func(context.Context, sharedBatchCommit) error
}

// recordSharedBatchCommits creates data sets 101, 102, ... and adds to the latest.
func recordSharedBatchCommits(target *fakeUploadContext) *sharedBatchCommits {
	rec := &sharedBatchCommits{providerID: target.id}
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		commit := sharedBatchCommit{create: req.ClientDataSetID != nil, pieces: len(req.Pieces)}
		if commit.create {
			commit.clientDataSetID = copyBigInt(*req.ClientDataSetID)
		}
		rec.mu.Lock()
		rec.commits = append(rec.commits, commit)
		hook := rec.submitHook
		rec.mu.Unlock()
		if hook != nil {
			if err := hook(commit); err != nil {
				return nil, err
			}
		}
		return &CommitSubmission{
			TransactionID:   fmt.Sprintf("0x%x", len(rec.snapshot())),
			PieceCIDs:       pieceCIDs(req.Pieces),
			ClientDataSetID: copyBigIntPtr(req.ClientDataSetID),
		}, nil
	}
	target.waitCommitFn = func(ctx context.Context, submission CommitSubmission) (*CommitResult, error) {
		commit := sharedBatchCommit{create: submission.ClientDataSetID != nil, pieces: len(submission.PieceCIDs)}
		rec.mu.Lock()
		hook := rec.waitHook
		rec.mu.Unlock()
		if hook != nil {
			if err := hook(ctx, commit); err != nil {
				return nil, err
			}
		}
		result := &CommitResult{TransactionID: submission.TransactionID, PieceIDs: make([]types.BigInt, len(submission.PieceCIDs))}
		for i := range result.PieceIDs {
			result.PieceIDs[i] = types.NewBigInt(uint64(i + 1))
		}
		rec.mu.Lock()
		defer rec.mu.Unlock()
		if commit.create {
			ref, err := NewDataSetRef(rec.providerID, types.NewBigInt(uint64(101+len(rec.dataSets))), *submission.ClientDataSetID)
			if err != nil {
				return nil, err
			}
			rec.dataSets = append(rec.dataSets, ref)
			result.DataSet = ref
			result.IsNewDataSet = true
			return result, nil
		}
		if len(rec.dataSets) == 0 {
			return nil, errors.New("add-pieces before a data set exists")
		}
		result.DataSet = rec.dataSets[len(rec.dataSets)-1]
		return result, nil
	}
	return rec
}

func (r *sharedBatchCommits) snapshot() []sharedBatchCommit {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sharedBatchCommit(nil), r.commits...)
}

func (r *sharedBatchCommits) lastDataSet() DataSetRef {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.dataSets) == 0 {
		return DataSetRef{}
	}
	return r.dataSets[len(r.dataSets)-1]
}

func (r *sharedBatchCommits) useDataSet(ref DataSetRef) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dataSets = append(r.dataSets, ref)
}

func sharedBatchTestTarget(identity ContextIdentity, providerID uint64) *fakeUploadContext {
	return &fakeUploadContext{id: types.NewBigInt(providerID), endpoint: "https://provider.example.com", identity: &identity}
}

func testCommitDataSetRefWithClientID(t *testing.T, providerID, dataSetID uint64, clientDataSetID types.BigInt) DataSetRef {
	t.Helper()
	ref, err := NewDataSetRef(types.NewBigInt(providerID), types.NewBigInt(dataSetID), clientDataSetID)
	if err != nil {
		t.Fatalf("NewDataSetRef: %v", err)
	}
	return ref
}

// sharedServiceTestTargets returns a bound primary and an unbound secondary.
func sharedServiceTestTargets(t *testing.T, identity ContextIdentity, uploads int) (primary, secondary *fakeUploadContext) {
	t.Helper()
	pieces := make([]PieceInput, uploads)
	for i := range pieces {
		pieces[i] = batchTestPiece(t, fmt.Sprintf("shared-service-%d", i))
	}
	primaryRef := testCommitDataSetRef(1, 11)
	primary = batchTestTarget(identity, primaryRef)
	var stored atomic.Int32
	primary.storeFn = func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
		i := stored.Add(1) - 1
		return &StoreResult{PieceCID: pieces[i].PieceCID, Size: 1}, nil
	}
	primary.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		return &CommitSubmission{TransactionID: "0xprimary", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	primary.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		pieceIDs := make([]types.BigInt, len(submission.PieceCIDs))
		for i := range pieceIDs {
			pieceIDs[i] = types.NewBigInt(uint64(i + 1))
		}
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: primaryRef, PieceIDs: pieceIDs}, nil
	}
	return primary, sharedBatchTestTarget(identity, 2)
}

type sharedServiceUploadOutcome struct {
	result *UploadResult
	err    error
}

func startSharedServiceUploads(service *Service, primary, secondary StorageContext, uploads int) <-chan sharedServiceUploadOutcome {
	outcomes := make(chan sharedServiceUploadOutcome, uploads)
	for range uploads {
		go func() {
			result, err := service.UploadToContexts(context.Background(), bytes.NewReader([]byte("ignored")), []StorageContext{primary, secondary}, nil)
			outcomes <- sharedServiceUploadOutcome{result: result, err: err}
		}()
	}
	return outcomes
}

func assertSharedSecondaryUploads(t *testing.T, outcomes <-chan sharedServiceUploadOutcome, uploads int) {
	t.Helper()
	for range uploads {
		select {
		case outcome := <-outcomes:
			if outcome.err != nil {
				t.Fatalf("UploadToContexts: %v", outcome.err)
			}
			if !outcome.result.Complete || len(outcome.result.Copies) != 2 {
				t.Fatalf("result=%+v, want both copies", outcome.result)
			}
			if cp := outcome.result.Copies[1]; cp.Role != CopyRoleSecondary || !cp.DataSetID.Equal(types.NewBigInt(101)) {
				t.Fatalf("secondary copy=%+v, want shared data set 101", cp)
			}
		case <-time.After(time.Second):
			t.Fatal("UploadToContexts did not finish")
		}
	}
}

func waitForUploadBatchFlightCount(t *testing.T, batcher *UploadBatcher, count int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		batcher.mu.Lock()
		ready := len(batcher.flights) == count
		batcher.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("upload batch flights did not reach %d", count)
		default:
			runtime.Gosched()
		}
	}
}

type blockingStorageSigner struct {
	signer.StorageSigner
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

type atomicStorageSigner struct {
	signer.StorageSigner
	calls atomic.Int32
}

func (s *atomicStorageSigner) SignHash(hash []byte) ([]byte, error) {
	s.calls.Add(1)
	return s.StorageSigner.SignHash(hash)
}

type manualUploadBatchClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualUploadBatchTimer
}

type manualUploadBatchTimer struct {
	clock   *manualUploadBatchClock
	at      time.Time
	fn      func()
	stopped bool
	fired   bool
}

func newManualUploadBatchClock() *manualUploadBatchClock {
	return &manualUploadBatchClock{now: time.Unix(1, 0)}
}

func (c *manualUploadBatchClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *manualUploadBatchClock) AfterFunc(wait time.Duration, fn func()) uploadBatchTimer {
	c.mu.Lock()
	defer c.mu.Unlock()
	timer := &manualUploadBatchTimer{clock: c, at: c.now.Add(wait), fn: fn}
	c.timers = append(c.timers, timer)
	return timer
}

func (c *manualUploadBatchClock) Advance(elapsed time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(elapsed)
	var due []*manualUploadBatchTimer
	for _, timer := range c.timers {
		if !timer.stopped && !timer.fired && !timer.at.After(c.now) {
			timer.fired = true
			due = append(due, timer)
		}
	}
	c.mu.Unlock()
	for _, timer := range due {
		timer.fn()
	}
}

func (t *manualUploadBatchTimer) Stop() bool {
	t.clock.mu.Lock()
	defer t.clock.mu.Unlock()
	if t.stopped || t.fired {
		return false
	}
	t.stopped = true
	return true
}

func (s *blockingStorageSigner) SignHash(hash []byte) ([]byte, error) {
	s.once.Do(func() { close(s.started) })
	<-s.release
	return s.StorageSigner.SignHash(hash)
}

func mustUploadBatcher(t *testing.T, identity ContextIdentity, storageSigner signer.StorageSigner, opts ...UploadBatcherOption) *UploadBatcher {
	t.Helper()
	batcher, err := NewUploadBatcher(UploadBatcherOptions{Identity: identity, Signer: storageSigner}, opts...)
	if err != nil {
		t.Fatalf("NewUploadBatcher: %v", err)
	}
	t.Cleanup(func() { _ = batcher.Close() })
	return batcher
}

func batchTestTarget(identity ContextIdentity, ref DataSetRef) *fakeUploadContext {
	dataSetID := ref.DataSetID()
	clientDataSetID := ref.ClientDataSetID()
	return &fakeUploadContext{
		id:              ref.ProviderID(),
		endpoint:        "https://provider.example.com",
		dataSetID:       &dataSetID,
		clientDataSetID: &clientDataSetID,
		identity:        &identity,
	}
}

func configureBatchTargetCommit(target *fakeUploadContext, ref DataSetRef, extraData chan<- []byte) {
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		if extraData != nil {
			extraData <- append([]byte(nil), req.ExtraData...)
		}
		return &CommitSubmission{TransactionID: "0x" + ref.DataSetID().String(), PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
	}
}

// requireBatchedSubmit rejects commits that bypass the batcher. The batcher
// always submits its own signed extraData; a batched Service passes none.
func requireBatchedSubmit(target *fakeUploadContext) {
	submit := target.submitCommitFn
	target.submitCommitFn = func(ctx context.Context, req CommitRequest) (*CommitSubmission, error) {
		if len(req.ExtraData) == 0 {
			return nil, errors.New("commit must use the batcher")
		}
		return submit(ctx, req)
	}
}

func batchTestPiece(t *testing.T, value string) PieceInput {
	t.Helper()
	info, err := piece.CalculateFromBytes(bytes.Repeat([]byte(value), 128))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	return PieceInput{PieceCID: info.CIDv2}
}

func enqueueBatchTestPiece(t *testing.T, batcher *UploadBatcher, target StorageContext, piece PieceInput) *uploadBatchTask {
	t.Helper()
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	task, err := batcher.enqueue(context.Background(), reservation.seq, target, piece, nil)
	reservation.release()
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return task
}

func beginBatchTestTransfer(t *testing.T, batcher *UploadBatcher, target StorageContext) (*uploadReservation, *uploadBatchTransfer) {
	t.Helper()
	reservation, err := batcher.reserve()
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	transfer, err := reservation.beginTransfer(target)
	if err != nil {
		reservation.release()
		t.Fatalf("beginTransfer: %v", err)
	}
	return reservation, transfer
}

// captureBatchTestSubmissions reports the piece count of each submission.
func captureBatchTestSubmissions(target *fakeUploadContext) <-chan int {
	submitted := make(chan int, 8)
	target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
		submitted <- len(req.Pieces)
		return &CommitSubmission{TransactionID: "0xtransfer", PieceCIDs: pieceCIDs(req.Pieces)}, nil
	}
	target.waitCommitFn = func(_ context.Context, submission CommitSubmission) (*CommitResult, error) {
		ref, _ := target.DataSetRef()
		pieceIDs := make([]types.BigInt, len(submission.PieceCIDs))
		for i := range pieceIDs {
			pieceIDs[i] = types.NewBigInt(uint64(i + 1))
		}
		return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: pieceIDs}, nil
	}
	return submitted
}

func waitBatchTestSubmission(t *testing.T, submitted <-chan int) int {
	t.Helper()
	select {
	case pieces := <-submitted:
		return pieces
	case <-time.After(time.Second):
		t.Fatal("window was not submitted")
		return 0
	}
}

// assertNoBatchTestSubmission checks that no submission starts. Timer
// callbacks launch flights on goroutines, so allow them a moment to run.
func assertNoBatchTestSubmission(t *testing.T, submitted <-chan int) {
	t.Helper()
	select {
	case pieces := <-submitted:
		t.Fatalf("window submitted early with %d pieces", pieces)
	case <-time.After(50 * time.Millisecond):
	}
}

func assertNoUploadBatchTransfers(t *testing.T, batcher *UploadBatcher) {
	t.Helper()
	batcher.mu.Lock()
	defer batcher.mu.Unlock()
	if len(batcher.transfers) != 0 {
		t.Fatalf("open transfers=%v, want none", batcher.transfers)
	}
}

func waitForUploadBatchWindowSlots(t *testing.T, batcher *UploadBatcher, want int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		batcher.mu.Lock()
		got := 0
		for _, window := range batcher.windows {
			got += len(window.slots)
		}
		batcher.mu.Unlock()
		if got == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("window slots=%d, want %d", got, want)
		default:
			runtime.Gosched()
		}
	}
}

func waitForManualUploadBatchTimerFired(t *testing.T, clock *manualUploadBatchClock) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		clock.mu.Lock()
		fired := false
		for _, timer := range clock.timers {
			fired = fired || timer.fired
		}
		clock.mu.Unlock()
		if fired {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timer did not fire")
		default:
			runtime.Gosched()
		}
	}
}

func waitForUploadBatchFlush(t *testing.T, batcher *UploadBatcher) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		batcher.mu.Lock()
		started := len(batcher.flushes) > 0
		batcher.mu.Unlock()
		if started {
			return
		}
		select {
		case <-deadline:
			t.Fatal("Flush did not start")
		default:
			runtime.Gosched()
		}
	}
}

func waitForUploadBatchFlushes(t *testing.T, batcher *UploadBatcher, count int) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		batcher.mu.Lock()
		started := len(batcher.flushes) >= count
		batcher.mu.Unlock()
		if started {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("%d concurrent Flush calls did not start", count)
		default:
			runtime.Gosched()
		}
	}
}

func waitForUploadBatchWindow(t *testing.T, batcher *UploadBatcher) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		batcher.mu.Lock()
		ready := len(batcher.windows) > 0
		batcher.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline:
			t.Fatal("upload batch window did not open")
		default:
			runtime.Gosched()
		}
	}
}

func waitForNoUploadBatchFlights(t *testing.T, batcher *UploadBatcher) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		batcher.mu.Lock()
		finished := len(batcher.flights) == 0
		batcher.mu.Unlock()
		if finished {
			return
		}
		select {
		case <-deadline:
			t.Fatal("upload batch flights did not finish")
		default:
			runtime.Gosched()
		}
	}
}

func pieceCIDs(pieces []PieceInput) []cid.Cid {
	out := make([]cid.Cid, len(pieces))
	for i := range pieces {
		out[i] = pieces[i].PieceCID
	}
	return out
}
