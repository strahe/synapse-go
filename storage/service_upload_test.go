package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

func pipelineTestTargets(t *testing.T, count int) ([]StorageContext, PieceInput) {
	t.Helper()
	piece := batchTestPiece(t, "pipeline")
	contexts := make([]StorageContext, count)
	for i := range contexts {
		ref := testCommitDataSetRef(uint64(i+1), uint64(i+11))
		target := batchTestTarget(serviceTestIdentity(), ref)
		target.storeFn = func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: piece.PieceCID, Size: 1}, nil
		}
		target.presignFn = func(context.Context, []PieceInput) ([]byte, error) { return []byte{byte(i + 1)}, nil }
		target.pullFn = func(context.Context, PullRequest) (*PullResult, error) {
			return &PullResult{Status: PullStatusComplete}, nil
		}
		configureBatchTargetCommit(target, ref, nil)
		contexts[i] = target
	}
	return contexts, piece
}

func awaitPipeline[T any](t *testing.T, events <-chan T) T {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(5 * time.Second):
		t.Fatal("pipeline did not reach the expected barrier")
		var zero T
		return zero
	}
}

type cancelAfterSigning struct {
	signer.StorageSigner
	cancel context.CancelFunc
}

func (s cancelAfterSigning) SignHash(hash []byte) ([]byte, error) {
	signature, err := s.StorageSigner.SignHash(hash)
	if err == nil {
		s.cancel()
	}
	return signature, err
}

func TestUploadCancellationAfterPresign(t *testing.T) {
	contexts, piece := pipelineTestTargets(t, 2)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		http.Error(w, "unexpected request after cancellation", http.StatusInternalServerError)
	}))
	defer server.Close()
	pdpClient, err := pdp.New(server.URL, pdp.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatal(err)
	}
	identity := serviceTestIdentity()
	provider, err := NewProviderContext(Provider{ID: types.NewBigInt(2), ServiceURL: server.URL}, pdpClient,
		cancelAfterSigning{StorageSigner: mustTestSigner(t), cancel: cancel},
		WithPayer(identity.Payer), WithChainID(identity.ChainID), WithRecordKeeper(identity.RecordKeeper))
	if err != nil {
		t.Fatal(err)
	}
	ref, _ := contexts[1].DataSetRef()
	contexts[1], err = provider.ForDataSet(ref)
	if err != nil {
		t.Fatal(err)
	}
	service := mustNewService(t, Options{})
	result, err := service.UploadToContexts(ctx, bytes.NewReader([]byte("data")), contexts, nil)
	var failures []FailedAttempt
	switch {
	case err == nil && result != nil:
		failures = result.FailedAttempts
	case errors.Is(err, context.Canceled):
		commitErr, ok := errors.AsType[*CommitError](err)
		if !ok {
			t.Fatalf("error=%v, want CommitError", err)
		}
		failures = commitErr.FailedAttempts
	default:
		t.Fatalf("result=%+v error=%v, want cancellation diagnostics", result, err)
	}
	if requests.Load() != 0 {
		t.Fatalf("HTTP requests=%d, want no pull or commit after cancellation", requests.Load())
	}
	if !slices.ContainsFunc(failures, func(failure FailedAttempt) bool {
		return failure.ProviderID.Equal(contexts[1].ProviderID()) && failure.Role == CopyRoleSecondary &&
			failure.Stage == CopyStagePull && failure.Explicit && errors.Is(failure.Err, context.Canceled)
	}) {
		t.Fatalf("failures=%+v, want canceled secondary pull after successful presign for %s", failures, piece.PieceCID)
	}
}

func TestUploadCancellationAfterCompletedPull(t *testing.T) {
	for _, batched := range []bool{false, true} {
		t.Run(fmt.Sprintf("batched=%t", batched), func(t *testing.T) {
			contexts, piece := pipelineTestTargets(t, 2)
			primary := contexts[0].(*fakeUploadContext)
			primary.pieceURL = "https://primary.example.com/piece/" + piece.PieceCID.String()
			primaryConfirmed := make(chan struct{})
			waitCommit := primary.waitCommitFn
			primary.waitCommitFn = func(ctx context.Context, submission CommitSubmission) (*CommitResult, error) {
				defer close(primaryConfirmed)
				return waitCommit(ctx, submission)
			}
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.Method != http.MethodPost || r.URL.Path != "/pdp/piece/pull" {
					http.Error(w, "unexpected secondary commit", http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				if _, err := fmt.Fprintf(w, `{"status":"complete","pieces":[{"pieceCid":%q,"status":"complete"}]}`, piece.PieceCID.String()); err != nil {
					t.Errorf("write pull response: %v", err)
				}
			}))
			defer server.Close()
			pdpClient, err := pdp.New(server.URL, pdp.WithHTTPClient(server.Client()))
			if err != nil {
				t.Fatal(err)
			}
			identity := serviceTestIdentity()
			provider, err := NewProviderContext(Provider{ID: types.NewBigInt(2), ServiceURL: server.URL}, pdpClient,
				mustTestSigner(t), WithPayer(identity.Payer), WithChainID(identity.ChainID), WithRecordKeeper(identity.RecordKeeper))
			if err != nil {
				t.Fatal(err)
			}
			ref, _ := contexts[1].DataSetRef()
			contexts[1], err = provider.ForDataSet(ref)
			if err != nil {
				t.Fatal(err)
			}
			opts := Options{}
			if batched {
				opts.UploadBatcher = mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
			}
			service := mustNewService(t, opts)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			pullCompleted, releaseProgress := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(releaseProgress) })
			defer release()
			var completeCallbacks atomic.Int32
			type outcome struct {
				result *UploadResult
				err    error
			}
			done := make(chan outcome, 1)
			go func() {
				result, err := service.UploadToContexts(ctx, bytes.NewReader([]byte("data")), contexts, &UploadToContextsOptions{
					OnPullProgress: func(_ types.BigInt, _ cid.Cid, status PullStatus) {
						if status == PullStatusComplete {
							close(pullCompleted)
							<-releaseProgress
							cancel()
						}
					},
					OnCopyComplete: func(types.BigInt, cid.Cid) { completeCallbacks.Add(1) },
				})
				done <- outcome{result: result, err: err}
			}()
			awaitPipeline(t, pullCompleted)
			awaitPipeline(t, primaryConfirmed)
			if batched {
				waitForNoUploadBatchFlights(t, opts.UploadBatcher)
			}
			release()
			got := awaitPipeline(t, done)
			if got.err != nil || got.result == nil || got.result.Complete || got.result.RequestedCopies != 2 ||
				len(got.result.Copies) != 1 || !got.result.Copies[0].ProviderID.Equal(primary.ProviderID()) {
				t.Fatalf("result=%+v error=%v, want incomplete confirmed primary result", got.result, got.err)
			}
			failures := got.result.FailedAttempts
			if len(failures) != 1 || !failures[0].ProviderID.Equal(contexts[1].ProviderID()) ||
				failures[0].Role != CopyRoleSecondary || failures[0].Stage != CopyStageCommit ||
				!failures[0].Explicit || !errors.Is(failures[0].Err, context.Canceled) {
				t.Fatalf("failures=%+v, want canceled secondary commit after completed pull", failures)
			}
			if requests.Load() != 1 || completeCallbacks.Load() != 0 {
				t.Fatalf("HTTP requests=%d complete callbacks=%d, want one pull and no callback after cancellation",
					requests.Load(), completeCallbacks.Load())
			}
			if batched {
				assertNoUploadBatchTransfers(t, opts.UploadBatcher)
			}
		})
	}
}

func TestUploadPipelinesReadyCopiesBeforeSlowPull(t *testing.T) {
	for _, batched := range []bool{false, true} {
		t.Run(fmt.Sprintf("batched=%t", batched), func(t *testing.T) {
			contexts, _ := pipelineTestTargets(t, 3)
			slowStarted := make(chan struct{})
			release := make(chan struct{})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			contexts[1].(*fakeUploadContext).pullFn = func(ctx context.Context, _ PullRequest) (*PullResult, error) {
				close(slowStarted)
				select {
				case <-release:
					return &PullResult{Status: PullStatusComplete}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			opts := Options{}
			if batched {
				opts.UploadBatcher = mustUploadBatcher(t, serviceTestIdentity(), mustTestSigner(t), WithUploadIdleWait(0))
			}
			service := mustNewService(t, opts)
			added := make(chan string, 3)
			var confirmed []string
			done := make(chan *UploadResult, 1)
			errCh := make(chan error, 1)
			go func() {
				result, err := service.UploadToContexts(ctx, bytes.NewReader([]byte("data")), contexts, &UploadToContextsOptions{
					OnPiecesAdded:     func(_ string, id types.BigInt, _ []SubmittedPiece) { added <- id.String() },
					OnPiecesConfirmed: func(_, id types.BigInt, _ []ConfirmedPiece) { confirmed = append(confirmed, id.String()) },
				})
				errCh <- err
				done <- result
			}()
			awaitPipeline(t, slowStarted)
			ready := []string{awaitPipeline(t, added), awaitPipeline(t, added)}
			slices.Sort(ready)
			if !slices.Equal(ready, []string{"1", "3"}) {
				t.Fatalf("submitted providers=%v before slow pull release, want primary and fast secondary", ready)
			}
			close(release)
			result := awaitPipeline(t, done)
			if err := awaitPipeline(t, errCh); err != nil || result == nil || !result.Complete {
				t.Fatalf("result=%+v error=%v", result, err)
			}
			if !slices.Equal(confirmed, []string{"1", "2", "3"}) {
				t.Fatalf("confirmation order=%v, want original target order", confirmed)
			}
			for i, cp := range result.Copies {
				if !cp.ProviderID.Equal(contexts[i].ProviderID()) {
					t.Fatalf("copy %d provider=%s, want %s", i, cp.ProviderID, contexts[i].ProviderID())
				}
			}
			if batched {
				assertNoUploadBatchTransfers(t, opts.UploadBatcher)
			}
		})
	}
}

func TestUploadPullAndCommitConcurrencyLimits(t *testing.T) {
	for _, pullLimit := range []int{-1, 0, 1, 2} {
		t.Run(fmt.Sprintf("pull=%d", pullLimit), func(t *testing.T) {
			contexts, _ := pipelineTestTargets(t, 6)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			pullRelease, commitRelease := make(chan struct{}), make(chan struct{})
			pullStarted, commitStarted := make(chan struct{}, 5), make(chan struct{}, 6)
			var mu sync.Mutex
			var pulls, commits, peakPulls, peakCommits int
			for i, target := range contexts {
				ref, _ := target.DataSetRef()
				fake := target.(*fakeUploadContext)
				fake.waitCommitFn = func(ctx context.Context, submission CommitSubmission) (*CommitResult, error) {
					mu.Lock()
					commits++
					peakCommits = max(peakCommits, commits)
					mu.Unlock()
					defer func() { mu.Lock(); commits--; mu.Unlock() }()
					commitStarted <- struct{}{}
					select {
					case <-commitRelease:
						return &CommitResult{TransactionID: submission.TransactionID, DataSet: ref, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				if i == 0 {
					continue
				}
				fake.pullFn = func(ctx context.Context, _ PullRequest) (*PullResult, error) {
					mu.Lock()
					pulls++
					peakPulls = max(peakPulls, pulls)
					mu.Unlock()
					defer func() { mu.Lock(); pulls--; mu.Unlock() }()
					pullStarted <- struct{}{}
					select {
					case <-pullRelease:
						return &PullResult{Status: PullStatusComplete}, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
			}
			service := mustNewService(t, Options{PullConcurrency: pullLimit, CommitConcurrency: 2})
			done := make(chan error, 1)
			go func() {
				result, err := service.UploadToContexts(ctx, bytes.NewReader([]byte("data")), contexts, nil)
				if err == nil && (result == nil || !result.Complete) {
					err = fmt.Errorf("incomplete result: %+v", result)
				}
				done <- err
			}()
			wantPulls := pullLimit
			if wantPulls <= 0 {
				wantPulls = 4
			}
			for range wantPulls {
				awaitPipeline(t, pullStarted)
			}
			awaitPipeline(t, commitStarted)
			close(pullRelease)
			awaitPipeline(t, commitStarted)
			close(commitRelease)
			if err := awaitPipeline(t, done); err != nil {
				t.Fatal(err)
			}
			if peakPulls != wantPulls || peakCommits != 2 {
				t.Fatalf("peak pulls=%d commits=%d, want %d and 2", peakPulls, peakCommits, wantPulls)
			}
		})
	}
}

func TestUploadConcurrentReplacementReservations(t *testing.T) {
	contexts, _ := pipelineTestTargets(t, 5)
	failed := make(chan struct{}, 2)
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for _, target := range contexts[1:3] {
		target.(*fakeUploadContext).pullFn = func(ctx context.Context, _ PullRequest) (*PullResult, error) {
			failed <- struct{}{}
			select {
			case <-release:
				return nil, errors.New("pull failed")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	resolver := &fakeResolver{contexts: contexts[:3], replacements: []StorageContext{contexts[3], contexts[3], contexts[4]}}
	service := mustNewService(t, Options{Resolver: resolver, MaxSecondaryAttempts: 3})
	type outcome struct {
		result *UploadResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := service.Upload(ctx, bytes.NewReader([]byte("data")), &UploadOptions{Copies: 3})
		done <- outcome{result, err}
	}()
	awaitPipeline(t, failed)
	awaitPipeline(t, failed)
	close(release)
	got := awaitPipeline(t, done)
	if got.err != nil || got.result == nil || !got.result.Complete {
		t.Fatalf("result=%+v error=%v", got.result, got.err)
	}
	ids := []string{got.result.Copies[1].ProviderID.String(), got.result.Copies[2].ProviderID.String()}
	slices.Sort(ids)
	if !slices.Equal(ids, []string{"4", "5"}) || resolver.replacementCalls != 3 {
		t.Fatalf("replacement providers=%v calls=%d, want distinct providers 4 and 5 in three attempts", ids, resolver.replacementCalls)
	}
	if len(got.result.FailedAttempts) != 3 || !got.result.FailedAttempts[0].ProviderID.Equal(types.NewBigInt(2)) {
		t.Fatalf("failed attempts=%+v, want stable slot order including duplicate candidate", got.result.FailedAttempts)
	}
}

func TestUploadPullCallbacksOverlap(t *testing.T) {
	contexts, piece := pipelineTestTargets(t, 3)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	for _, target := range contexts[1:] {
		target.(*fakeUploadContext).pullFn = func(_ context.Context, req PullRequest) (*PullResult, error) {
			req.OnProgress(piece.PieceCID, PullStatusComplete)
			return &PullResult{Status: PullStatusComplete}, nil
		}
	}
	service := mustNewService(t, Options{})
	done := make(chan error, 1)
	go func() {
		_, err := service.UploadToContexts(ctx, bytes.NewReader([]byte("data")), contexts, &UploadToContextsOptions{
			OnPullProgress: func(types.BigInt, cid.Cid, PullStatus) {
				entered <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
				}
			},
		})
		done <- err
	}()
	awaitPipeline(t, entered)
	awaitPipeline(t, entered)
	close(release)
	if err := awaitPipeline(t, done); err != nil {
		t.Fatal(err)
	}
}

func TestUploadAbortKeepsAcceptedBatchWorkAndOtherUpload(t *testing.T) {
	for _, callbackPanic := range []bool{false, true} {
		t.Run(fmt.Sprintf("callback-panic=%t", callbackPanic), func(t *testing.T) {
			contexts, _ := pipelineTestTargets(t, 3)
			batcher := mustUploadBatcher(t, serviceTestIdentity(), mustTestSigner(t), WithUploadIdleWait(0))
			service := mustNewService(t, Options{UploadBatcher: batcher})
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			pullStarted := make(chan struct{})
			contexts[1].(*fakeUploadContext).pullFn = func(ctx context.Context, _ PullRequest) (*PullResult, error) {
				close(pullStarted)
				<-ctx.Done()
				return nil, ctx.Err()
			}
			primaryWaiting, releasePrimary := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(releasePrimary) })
			defer release()
			primaryRef, _ := contexts[0].DataSetRef()
			contexts[0].(*fakeUploadContext).waitCommitFn = func(ctx context.Context, submission CommitSubmission) (*CommitResult, error) {
				close(primaryWaiting)
				select {
				case <-releasePrimary:
					return &CommitResult{TransactionID: submission.TransactionID, DataSet: primaryRef, PieceIDs: []types.BigInt{types.NewBigInt(1)}}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			added, triggerPanic := make(chan struct{}), make(chan struct{})
			sentinel := &uploadCallbackPanic{callback: "OnPiecesAdded"}
			type outcome struct {
				result     *UploadResult
				err        error
				panicValue any
			}
			done := make(chan outcome, 1)
			go func() {
				var got outcome
				got.panicValue = recoverPanic(func() {
					got.result, got.err = service.UploadToContexts(ctx, bytes.NewReader([]byte("first")), contexts[:2], &UploadToContextsOptions{
						OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) {
							close(added)
							if callbackPanic {
								select {
								case <-triggerPanic:
									panic(sentinel)
								case <-ctx.Done():
								}
							}
						},
					})
				})
				done <- got
			}()
			awaitPipeline(t, pullStarted)
			awaitPipeline(t, added)
			awaitPipeline(t, primaryWaiting)
			other := make(chan outcome, 1)
			go func() {
				result, err := service.UploadToContexts(t.Context(), bytes.NewReader([]byte("other")), contexts[2:], nil)
				other <- outcome{result: result, err: err}
			}()
			if callbackPanic {
				close(triggerPanic)
			} else {
				cancel()
			}
			got := awaitPipeline(t, done)
			if callbackPanic {
				if got.panicValue != sentinel {
					t.Fatalf("panic=%v, want original sentinel", got.panicValue)
				}
			} else if got.result != nil || !errors.Is(got.err, context.Canceled) {
				t.Fatalf("result=%+v error=%v, want cancellation before any confirmation", got.result, got.err)
			}
			if another := awaitPipeline(t, other); another.err != nil || another.result == nil || !another.result.Complete {
				t.Fatalf("other upload affected: result=%+v error=%v", another.result, another.err)
			}
			release()
			flushCtx, flushCancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer flushCancel()
			if err := service.Flush(flushCtx); err != nil {
				t.Fatalf("accepted task did not survive abort: %v", err)
			}
			assertNoUploadBatchTransfers(t, batcher)
		})
	}
}
