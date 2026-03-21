package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"testing"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/piece"
)

func TestManagerUploadBytes_DefaultCopiesAndPresignReuse(t *testing.T) {
	data := bytes.Repeat([]byte("ab"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var callsMu sync.Mutex
	var calls []string
	appendCall := func(name string) {
		callsMu.Lock()
		calls = append(calls, name)
		callsMu.Unlock()
	}
	primary := &fakeUploadContext{
		id:       big.NewInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/pdp/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, got []byte, _ *StoreOptions) (*StoreResult, error) {
			appendCall("primary.store")
			if !bytes.Equal(got, data) {
				t.Fatalf("store data mismatch")
			}
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, req CommitRequest) (*CommitResult, error) {
			appendCall("primary.commit")
			if req.ExtraData != nil {
				t.Fatalf("primary commit should not receive secondary extraData")
			}
			return &CommitResult{
				DataSetID:     big.NewInt(1001),
				PieceIDs:      []*big.Int{big.NewInt(2001)},
				IsNewDataSet:  true,
				TransactionID: "0xprimary",
			}, nil
		},
	}

	secondaryExtra := []byte{0xde, 0xad, 0xbe, 0xef}
	secondary := &fakeUploadContext{
		id:       big.NewInt(202),
		endpoint: "https://secondary.example.com",
		pieceURL: "https://secondary.example.com/pdp/piece/" + info.CIDv2.String(),
		presignFn: func(_ context.Context, pieces []PieceInput) ([]byte, error) {
			appendCall("secondary.presign")
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("unexpected presign pieces: %+v", pieces)
			}
			return secondaryExtra, nil
		},
		pullFn: func(_ context.Context, req PullRequest) (*PullResult, error) {
			appendCall("secondary.pull")
			if got := req.From(info.CIDv2); got != primary.pieceURL {
				t.Fatalf("pull source=%q want %q", got, primary.pieceURL)
			}
			if !bytes.Equal(req.ExtraData, secondaryExtra) {
				t.Fatalf("pull extraData=%x want %x", req.ExtraData, secondaryExtra)
			}
			return &PullResult{
				Status: PullStatusComplete,
				Pieces: []PullPieceResult{{PieceCID: info.CIDv2, Status: PullStatusComplete}},
			}, nil
		},
		commitFn: func(_ context.Context, req CommitRequest) (*CommitResult, error) {
			appendCall("secondary.commit")
			if !bytes.Equal(req.ExtraData, secondaryExtra) {
				t.Fatalf("commit extraData=%x want %x", req.ExtraData, secondaryExtra)
			}
			return &CommitResult{
				DataSetID:     big.NewInt(1002),
				PieceIDs:      []*big.Int{big.NewInt(2002)},
				IsNewDataSet:  true,
				TransactionID: "0xsecondary",
			}, nil
		},
	}

	mgr := &Manager{
		resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}},
	}

	got, err := mgr.UploadBytes(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("UploadBytes: %v", err)
	}
	if got.RequestedCopies != 2 {
		t.Fatalf("requestedCopies=%d want 2", got.RequestedCopies)
	}
	if !got.Complete {
		t.Fatal("complete=false want true")
	}
	if len(got.Copies) != 2 {
		t.Fatalf("copies len=%d want 2", len(got.Copies))
	}
	if got.PieceCID != info.CIDv2 {
		t.Fatalf("pieceCID=%s want %s", got.PieceCID, info.CIDv2)
	}
	if len(got.FailedAttempts) != 0 {
		t.Fatalf("failedAttempts=%d want 0", len(got.FailedAttempts))
	}
	if !containsCall(calls, "primary.store") || !containsCall(calls, "secondary.presign") || !containsCall(calls, "secondary.pull") {
		t.Fatalf("missing expected calls: %v", calls)
	}
}

func TestManagerUploadBytes_PrimaryStoreFailureReturnsStoreError(t *testing.T) {
	want := errors.New("store failed")
	primary := &fakeUploadContext{
		id:       big.NewInt(101),
		endpoint: "https://primary.example.com",
		storeFn: func(_ context.Context, _ []byte, _ *StoreOptions) (*StoreResult, error) {
			return nil, want
		},
	}

	mgr := &Manager{
		resolver: &fakeResolver{contexts: []UploadContext{primary}},
	}

	_, err := mgr.UploadBytes(context.Background(), bytes.Repeat([]byte("ab"), 128), nil)
	if err == nil {
		t.Fatal("expected StoreError")
	}
	var got *StoreError
	if !errors.As(err, &got) {
		t.Fatalf("want StoreError, got %T", err)
	}
	if got.ProviderID.Cmp(primary.id) != 0 {
		t.Fatalf("providerID=%s want %s", got.ProviderID, primary.id)
	}
	if !errors.Is(err, want) {
		t.Fatalf("error should wrap original cause: %v", err)
	}
}

func TestManagerUploadBytes_PartialSuccessReturnsIncompleteResult(t *testing.T) {
	data := bytes.Repeat([]byte("cd"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       big.NewInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/pdp/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ []byte, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: big.NewInt(1001), PieceIDs: []*big.Int{big.NewInt(2001)}}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       big.NewInt(202),
		endpoint: "https://secondary.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}

	mgr := &Manager{
		resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}},
	}

	got, err := mgr.UploadBytes(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("UploadBytes: %v", err)
	}
	if got.Complete {
		t.Fatal("complete=true want false")
	}
	if len(got.Copies) != 1 {
		t.Fatalf("copies len=%d want 1", len(got.Copies))
	}
	if len(got.FailedAttempts) != 1 {
		t.Fatalf("failedAttempts len=%d want 1", len(got.FailedAttempts))
	}
	if got.FailedAttempts[0].Stage != CopyStagePull {
		t.Fatalf("failedAttempts[0].stage=%q want %q", got.FailedAttempts[0].Stage, CopyStagePull)
	}
}

func TestManagerUploadBytes_AllCommitsFailReturnsCommitError(t *testing.T) {
	data := bytes.Repeat([]byte("ef"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       big.NewInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/pdp/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ []byte, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return nil, errors.New("primary commit failed")
		},
	}
	secondary := &fakeUploadContext{
		id:       big.NewInt(202),
		endpoint: "https://secondary.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return &PullResult{
				Status: PullStatusComplete,
				Pieces: []PullPieceResult{{PieceCID: info.CIDv2, Status: PullStatusComplete}},
			}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return nil, errors.New("secondary commit failed")
		},
	}

	mgr := &Manager{
		resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}},
	}

	_, err = mgr.UploadBytes(context.Background(), data, nil)
	if err == nil {
		t.Fatal("expected CommitError")
	}
	var got *CommitError
	if !errors.As(err, &got) {
		t.Fatalf("want CommitError, got %T", err)
	}
	if got.ProviderID.Cmp(primary.id) != 0 {
		t.Fatalf("providerID=%s want %s", got.ProviderID, primary.id)
	}
}

func TestManagerUploadBytes_ImplicitSecondaryReplacement(t *testing.T) {
	data := bytes.Repeat([]byte("gh"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       big.NewInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/pdp/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ []byte, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: big.NewInt(1001), PieceIDs: []*big.Int{big.NewInt(2001)}}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       big.NewInt(202),
		endpoint: "https://secondary-a.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}
	replacement := &fakeUploadContext{
		id:       big.NewInt(303),
		endpoint: "https://secondary-b.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x02}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return &PullResult{
				Status: PullStatusComplete,
				Pieces: []PullPieceResult{{PieceCID: info.CIDv2, Status: PullStatusComplete}},
			}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: big.NewInt(1002), PieceIDs: []*big.Int{big.NewInt(2002)}}, nil
		},
	}

	resolver := &fakeResolver{
		contexts:     []UploadContext{primary, failedSecondary},
		replacements: []UploadContext{replacement},
	}
	mgr := &Manager{resolver: resolver}

	got, err := mgr.UploadBytes(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("UploadBytes: %v", err)
	}
	if !got.Complete {
		t.Fatal("complete=false want true")
	}
	if len(got.Copies) != 2 {
		t.Fatalf("copies len=%d want 2", len(got.Copies))
	}
	if resolver.replacementCalls != 1 {
		t.Fatalf("replacementCalls=%d want 1", resolver.replacementCalls)
	}
	if got.Copies[1].ProviderID.Cmp(replacement.id) != 0 {
		t.Fatalf("replacement provider=%s want %s", got.Copies[1].ProviderID, replacement.id)
	}
}

type fakeResolver struct {
	contexts         []UploadContext
	explicit         bool
	replacements     []UploadContext
	replacementCalls int
}

func (r *fakeResolver) ResolveUploadContexts(_ context.Context, _ *UploadOptions) ([]UploadContext, bool, error) {
	return r.contexts, r.explicit, nil
}

func (r *fakeResolver) SelectReplacement(_ context.Context, _ map[string]struct{}, _ *UploadOptions) (UploadContext, error) {
	r.replacementCalls++
	if len(r.replacements) == 0 {
		return nil, errors.New("no replacement")
	}
	next := r.replacements[0]
	r.replacements = r.replacements[1:]
	return next, nil
}

type fakeUploadContext struct {
	id              *big.Int
	endpoint        string
	pieceURL        string
	dataSetID       *big.Int
	clientDataSetID *big.Int
	dataSetMetadata map[string]string
	storeFn         func(context.Context, []byte, *StoreOptions) (*StoreResult, error)
	presignFn       func(context.Context, []PieceInput) ([]byte, error)
	pullFn          func(context.Context, PullRequest) (*PullResult, error)
	commitFn        func(context.Context, CommitRequest) (*CommitResult, error)
}

func (c *fakeUploadContext) ProviderID() *big.Int      { return new(big.Int).Set(c.id) }
func (c *fakeUploadContext) ServiceURL() string        { return c.endpoint }
func (c *fakeUploadContext) PieceURL(_ cid.Cid) string { return c.pieceURL }

func (c *fakeUploadContext) StoreBytes(ctx context.Context, data []byte, opts *StoreOptions) (*StoreResult, error) {
	if c.storeFn == nil {
		return nil, fmt.Errorf("unexpected storeBytes")
	}
	return c.storeFn(ctx, data, opts)
}

func (c *fakeUploadContext) PresignForCommit(ctx context.Context, pieces []PieceInput) ([]byte, error) {
	if c.presignFn == nil {
		return nil, fmt.Errorf("unexpected presignForCommit")
	}
	return c.presignFn(ctx, pieces)
}

func (c *fakeUploadContext) Pull(ctx context.Context, req PullRequest) (*PullResult, error) {
	if c.pullFn == nil {
		return nil, fmt.Errorf("unexpected pull")
	}
	return c.pullFn(ctx, req)
}

func (c *fakeUploadContext) Commit(ctx context.Context, req CommitRequest) (*CommitResult, error) {
	if c.commitFn == nil {
		return nil, fmt.Errorf("unexpected commit")
	}
	return c.commitFn(ctx, req)
}

func containsCall(calls []string, want string) bool {
	for _, call := range calls {
		if call == want {
			return true
		}
	}
	return false
}

// TestManagerUploadBytes_RequestedCopiesIsCallerRequested proves that
// UploadResult.RequestedCopies reflects the caller's intent (opts.Copies,
// default 2), not the number of contexts the resolver happened to return.
// When fewer contexts are available the result must have Complete=false.
func TestManagerUploadBytes_RequestedCopiesIsCallerRequested(t *testing.T) {
	data := bytes.Repeat([]byte("rc"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       big.NewInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ []byte, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: big.NewInt(1), PieceIDs: []*big.Int{big.NewInt(10)}, IsNewDataSet: true}, nil
		},
	}

	// Resolver returns only 1 context even though caller requests 3 copies.
	mgr := &Manager{
		resolver:   &fakeResolver{contexts: []UploadContext{primary}},
		httpClient: nil,
	}

	got, err := mgr.UploadBytes(context.Background(), data, &UploadOptions{Copies: 3})
	if err != nil {
		t.Fatalf("UploadBytes: %v", err)
	}
	if got.RequestedCopies != 3 {
		t.Fatalf("RequestedCopies=%d want 3", got.RequestedCopies)
	}
	if got.Complete {
		t.Fatalf("Complete=true want false (only 1 of 3 copies succeeded)")
	}
	if len(got.Copies) != 1 {
		t.Fatalf("Copies=%d want 1", len(got.Copies))
	}
}

// TestManagerUploadBytes_NilPullResultNoNilDeref proves that a nil PullResult
// returned alongside a nil error is handled gracefully (no panic).
func TestManagerUploadBytes_NilPullResultNoNilDeref(t *testing.T) {
	data := bytes.Repeat([]byte("np"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       big.NewInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ []byte, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: big.NewInt(1), PieceIDs: []*big.Int{big.NewInt(10)}, IsNewDataSet: true}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       big.NewInt(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		// Returns (nil, nil) — the nil-deref case.
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, nil
		},
	}

	mgr := &Manager{
		resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}, explicit: true},
	}

	// Should not panic; primary copy should still succeed.
	got, err := mgr.UploadBytes(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("UploadBytes: %v", err)
	}
	if len(got.Copies) != 1 || got.Copies[0].Role != CopyRolePrimary {
		t.Fatalf("expected only primary copy, got %+v", got.Copies)
	}
	if len(got.FailedAttempts) != 1 || got.FailedAttempts[0].Stage != CopyStagePull {
		t.Fatalf("expected one pull failure, got %+v", got.FailedAttempts)
	}
}

// TestManagerUploadBytes_PresignFailureUsesPresignStage proves that a presign
// error is recorded with CopyStagePresign, not CopyStageCommit.
func TestManagerUploadBytes_PresignFailureUsesPresignStage(t *testing.T) {
	data := bytes.Repeat([]byte("ps"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       big.NewInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ []byte, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: big.NewInt(1), PieceIDs: []*big.Int{big.NewInt(10)}, IsNewDataSet: true}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       big.NewInt(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return nil, errors.New("presign failed: no signer")
		},
	}

	mgr := &Manager{
		resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}, explicit: true},
	}

	got, err := mgr.UploadBytes(context.Background(), data, nil)
	if err != nil {
		t.Fatalf("UploadBytes: %v", err)
	}
	if len(got.FailedAttempts) != 1 {
		t.Fatalf("FailedAttempts=%d want 1", len(got.FailedAttempts))
	}
	if got.FailedAttempts[0].Stage != CopyStagePresign {
		t.Fatalf("Stage=%s want %s", got.FailedAttempts[0].Stage, CopyStagePresign)
	}
}
