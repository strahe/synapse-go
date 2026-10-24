package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"testing/synctest"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
)

func TestUploadBatcherCommitTimeoutRetainsRegisteredSubmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
		defer func() { _ = batcher.Close() }()
		ref := testCommitDataSetRef(1, 11)
		target := batchTestTarget(identity, ref)
		piece := batchTestPiece(t, "timeout-handle")
		target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
			first := CommitSubmission{TransactionID: "first", StatusURL: "https://provider.example.com/status/first", ProviderID: target.id, DataSet: &ref, PieceCIDs: pieceCIDs(req.Pieces)}
			req.OnSubmitted(first)
			first.PieceCIDs[0] = cid.Undef
			return &CommitSubmission{TransactionID: "second", PieceCIDs: pieceCIDs(req.Pieces)}, nil
		}
		target.waitCommitFn = func(ctx context.Context, submitted CommitSubmission) (*CommitResult, error) {
			if submitted.TransactionID != "first" || !submitted.PieceCIDs[0].Equals(piece.PieceCID) {
				return nil, fmt.Errorf("confirmation received mutated submission: %+v", submitted)
			}
			<-ctx.Done()
			return nil, ctx.Err()
		}
		task := enqueueBatchTestPiece(t, batcher, target, piece)
		start := time.Now()
		err := batcher.Flush(context.Background())
		timeout, ok := errors.AsType[*UploadCommitTimeoutError](err)
		if !ok || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Flush error=%v, want recoverable confirmation timeout", err)
		}
		if elapsed := time.Since(start); elapsed != 5*time.Minute {
			t.Fatalf("confirmation elapsed=%s, want 5m", elapsed)
		}
		if timeout.Submission.TransactionID != "first" || timeout.Submission.DataSet == nil || !timeout.Submission.DataSet.Equal(ref) || !timeout.Submission.PieceCIDs[0].Equals(piece.PieceCID) {
			t.Fatalf("timeout submission=%+v, want first registered handle", timeout.Submission)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := batcher.Close(); err != nil {
			t.Fatal(err)
		}
		_, submission, err := task.wait(ctx, nil)
		if _, ok := errors.AsType[*UploadCommitTimeoutError](err); !ok || submission == nil || submission.TransactionID != "first" {
			t.Fatalf("wait submission=%+v error=%v", submission, err)
		}
		timeout.Submission.PieceCIDs[0] = cid.Undef
		if !submission.PieceCIDs[0].Equals(piece.PieceCID) {
			t.Fatal("timeout diagnostics alias the caller's submission")
		}
	})
}

func TestUploadBatcherCommitDeadlineOutcomes(t *testing.T) {
	for _, mode := range []string{"disabled", "provider deadline", "confirmed at deadline", "rejected at deadline", "close"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				identity := serviceTestIdentity()
				opts := []UploadBatcherOption{WithUploadIdleWait(0), WithUploadCommitTimeout(time.Minute)}
				if mode == "disabled" {
					opts = append(opts, WithoutUploadCommitTimeout())
				}
				batcher := mustUploadBatcher(t, identity, mustTestSigner(t), opts...)
				defer func() { _ = batcher.Close() }()
				ref := testCommitDataSetRef(1, 11)
				target := batchTestTarget(identity, ref)
				target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
					return &CommitSubmission{TransactionID: "pending", PieceCIDs: pieceCIDs(req.Pieces)}, nil
				}
				target.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
					switch mode {
					case "disabled":
						if _, ok := ctx.Deadline(); ok {
							return nil, errors.New("disabled deadline still present")
						}
						time.Sleep(6 * time.Minute)
					case "provider deadline":
						return nil, context.DeadlineExceeded
					case "confirmed at deadline":
						<-ctx.Done()
					case "rejected at deadline":
						<-ctx.Done()
						return nil, &CommitRejectedError{ProviderID: target.id}
					case "close":
						_ = batcher.Close()
						return nil, ctx.Err()
					}
					return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
				}
				enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, mode))
				err := batcher.Flush(context.Background())
				if _, ok := errors.AsType[*UploadCommitTimeoutError](err); ok {
					t.Fatalf("%s misclassified as confirmation timeout: %v", mode, err)
				}
				switch mode {
				case "provider deadline":
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("error=%v, want provider deadline", err)
					}
				case "rejected at deadline":
					if !errors.Is(err, pdp.ErrTxRejected) {
						t.Fatalf("error=%v, want rejection", err)
					}
				case "close":
					if !errors.Is(err, ErrClosed) {
						t.Fatalf("error=%v, want ErrClosed", err)
					}
				default:
					if err != nil {
						t.Fatal(err)
					}
				}
			})
		})
	}
}

func TestUploadBatcherCommitTimeoutSurvivesCanceledFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0), WithUploadCommitTimeout(time.Minute))
		defer func() { _ = batcher.Close() }()
		for _, providerID := range []uint64{1, 2} {
			target := batchTestTarget(identity, testCommitDataSetRef(providerID, providerID+10))
			target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
				return &CommitSubmission{TransactionID: target.id.String(), ProviderID: target.id, PieceCIDs: pieceCIDs(req.Pieces)}, nil
			}
			target.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
				<-ctx.Done()
				return nil, ctx.Err()
			}
			enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, target.id.String()))
		}
		synctest.Wait()
		flushCtx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := batcher.Flush(flushCtx); !errors.Is(err, context.Canceled) {
			t.Fatalf("Flush error=%v, want caller cancellation", err)
		}
		time.Sleep(time.Minute)
		synctest.Wait()
		err := batcher.Flush(context.Background())
		joined, ok := err.(interface{ Unwrap() []error })
		if !ok || len(joined.Unwrap()) != 2 {
			t.Fatalf("Flush error=%v, want two retained confirmation timeouts", err)
		}
		seen := make(map[string]bool)
		for _, failure := range joined.Unwrap() {
			timeout, ok := errors.AsType[*UploadCommitTimeoutError](failure)
			if !ok {
				t.Fatalf("failure=%v, want a recoverable confirmation timeout", failure)
			}
			seen[timeout.Submission.ProviderID.String()] = true
		}
		if !seen["1"] || !seen["2"] {
			t.Fatalf("timeout handles=%v, want both providers", seen)
		}
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatalf("consumed confirmation failures reported again: %v", err)
		}
	})
}

func TestUploadBatcherCommitDeadlineStartsAfterSubmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0), WithUploadCommitTimeout(time.Minute))
		defer func() { _ = batcher.Close() }()
		ref := testCommitDataSetRef(1, 11)
		target := batchTestTarget(identity, ref)
		target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
			submission := &CommitSubmission{TransactionID: "slow-submit", PieceCIDs: pieceCIDs(req.Pieces)}
			req.OnSubmitted(*submission)
			time.Sleep(10 * time.Minute)
			return submission, nil
		}
		target.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
			deadline, ok := ctx.Deadline()
			if !ok || time.Until(deadline) != time.Minute {
				return nil, fmt.Errorf("confirmation deadline=%v, want one minute after submission", deadline)
			}
			return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
		}
		enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "slow-submit"))
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestUploadBatcherCallerCancellationPreservesSharedConfirmation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithoutUploadIdleWait(), WithoutUploadMaxWait(), WithUploadCommitTimeout(time.Minute))
		defer func() { _ = batcher.Close() }()
		ref := testCommitDataSetRef(1, 11)
		target := batchTestTarget(identity, ref)
		release := make(chan struct{})
		target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
			return &CommitSubmission{TransactionID: "shared", PieceCIDs: pieceCIDs(req.Pieces)}, nil
		}
		target.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
			select {
			case <-release:
				return &CommitResult{DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1), types.NewBigInt(2)}}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		first := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "shared-first"))
		second := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "shared-second"))
		flushed := make(chan error, 1)
		go func() { flushed <- batcher.Flush(context.Background()) }()
		synctest.Wait()
		caller, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := first.wait(caller, nil); !errors.Is(err, context.Canceled) {
			t.Fatalf("first wait=%v, want caller cancellation", err)
		}
		time.Sleep(30 * time.Second)
		close(release)
		result, _, err := second.wait(context.Background(), nil)
		if err != nil || result == nil || !result.PieceIDs[0].Equal(types.NewBigInt(2)) {
			t.Fatalf("second result=%+v error=%v", result, err)
		}
		if err := <-flushed; err != nil {
			t.Fatal(err)
		}
	})
}

func TestUploadBatcherRecoversCreateAfterConfirmationTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0), WithUploadCommitTimeout(time.Minute))
		defer func() { _ = batcher.Close() }()
		target := sharedBatchTestTarget(identity, 1)
		commits := recordSharedBatchCommits(target)
		commits.waitHook = func(ctx context.Context, commit sharedBatchCommit) error {
			if commit.create {
				<-ctx.Done()
				return ctx.Err()
			}
			return nil
		}
		enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "timeout-create"))
		if _, ok := errors.AsType[*UploadCommitTimeoutError](batcher.Flush(context.Background())); !ok {
			t.Fatal("create did not return a confirmation timeout")
		}
		first := commits.snapshot()[0]
		ref := testCommitDataSetRefWithClientID(t, 1, 555, first.clientDataSetID)
		lookups := 0
		target.findDataSetFn = func(_ context.Context, id types.BigInt) (DataSetRef, bool, error) {
			lookups++
			if !id.Equal(first.clientDataSetID) {
				return DataSetRef{}, false, errors.New("lookup changed client data-set ID")
			}
			return ref, true, nil
		}
		commits.useDataSet(ref)
		task := enqueueBatchTestPiece(t, batcher, target, batchTestPiece(t, "after-timeout"))
		if err := batcher.Flush(context.Background()); err != nil {
			t.Fatal(err)
		}
		result, _, err := task.wait(context.Background(), nil)
		got := commits.snapshot()
		if err != nil || result == nil || !result.DataSet.Equal(ref) || len(got) != 2 || got[1].create || lookups == 0 {
			t.Fatalf("recovery result=%+v error=%v commits=%+v lookups=%d", result, err, got, lookups)
		}
	})
}

func TestUploadBatcherTimeoutKeepsSuccessfulCopies(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		identity := serviceTestIdentity()
		batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0), WithUploadCommitTimeout(time.Minute))
		defer func() { _ = batcher.Close() }()
		input := batchTestPiece(t, "partial-timeout")
		primaryRef, secondaryRef := testCommitDataSetRef(1, 11), testCommitDataSetRef(2, 22)
		primary, secondary := batchTestTarget(identity, primaryRef), batchTestTarget(identity, secondaryRef)
		primary.storeFn = func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: input.PieceCID, Size: 127}, nil
		}
		secondary.pullFn = func(context.Context, PullRequest) (*PullResult, error) {
			return &PullResult{Status: PullStatusComplete, Pieces: []PullPieceResult{{PieceCID: input.PieceCID, Status: PullStatusComplete}}}, nil
		}
		for _, target := range []*fakeUploadContext{primary, secondary} {
			target.submitCommitFn = func(_ context.Context, req CommitRequest) (*CommitSubmission, error) {
				return &CommitSubmission{TransactionID: target.id.String(), ProviderID: target.id, PieceCIDs: pieceCIDs(req.Pieces)}, nil
			}
		}
		primary.waitCommitFn = func(context.Context, CommitSubmission) (*CommitResult, error) {
			return &CommitResult{DataSet: primaryRef, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
		}
		secondary.waitCommitFn = func(ctx context.Context, _ CommitSubmission) (*CommitResult, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		service := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary, secondary}}, UploadBatcher: batcher})
		result, err := service.Upload(context.Background(), bytes.NewReader([]byte("partial-timeout")), &UploadOptions{Copies: 2})
		if err != nil || result == nil || result.Complete || len(result.Copies) != 1 || len(result.FailedAttempts) != 1 {
			t.Fatalf("Upload result=%+v error=%v, want one successful copy", result, err)
		}
		failure := result.FailedAttempts[0]
		timeout, ok := errors.AsType[*UploadCommitTimeoutError](failure.Err)
		if !ok || failure.Stage != CopyStageCommit || failure.Submission == nil || timeout.Submission.TransactionID != "2" || failure.Submission.TransactionID != "2" {
			t.Fatalf("failed attempt=%+v, want recoverable secondary commit timeout", failure)
		}
	})
}
