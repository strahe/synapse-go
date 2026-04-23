package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	icurio "github.com/strahe/synapse-go/internal/curio"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func mustNewService(t *testing.T, opts Options) *Service {
	t.Helper()
	s, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestManagerUpload_DefaultCopiesAndPresignReuse(t *testing.T) {
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
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			appendCall("primary.store")
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
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
				DataSetID:     types.DataSetID(1001),
				PieceIDs:      []types.PieceID{types.PieceID(2001)},
				IsNewDataSet:  true,
				TransactionID: "0xprimary",
			}, nil
		},
	}

	secondaryExtra := []byte{0xde, 0xad, 0xbe, 0xef}
	secondary := &fakeUploadContext{
		id:       types.ProviderID(202),
		endpoint: "https://secondary.example.com",
		pieceURL: "https://secondary.example.com/piece/" + info.CIDv2.String(),
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
				DataSetID:     types.DataSetID(1002),
				PieceIDs:      []types.PieceID{types.PieceID(2002)},
				IsNewDataSet:  true,
				TransactionID: "0xsecondary",
			}, nil
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}}})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
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

func TestManagerUpload_PrimaryStoreFailureReturnsStoreError(t *testing.T) {
	want := errors.New("store failed")
	primary := &fakeUploadContext{
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return nil, want
		},
	}

	mgr := &Service{
		httpClient: &http.Client{},
		resolver:   &fakeResolver{contexts: []UploadContext{primary}},
	}

	_, err := mgr.Upload(context.Background(), bytes.NewReader(bytes.Repeat([]byte("ab"), 128)), nil)
	if err == nil {
		t.Fatal("expected StoreError")
	}
	var got *StoreError
	if !errors.As(err, &got) {
		t.Fatalf("want StoreError, got %T", err)
	}
	if got.ProviderID != primary.id {
		t.Fatalf("providerID=%d want %d", got.ProviderID, primary.id)
	}
	if !errors.Is(err, want) {
		t.Fatalf("error should wrap original cause: %v", err)
	}
}

func TestManagerUpload_PartialSuccessReturnsIncompleteResult(t *testing.T) {
	data := bytes.Repeat([]byte("cd"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1001), PieceIDs: []types.PieceID{types.PieceID(2001)}}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.ProviderID(202),
		endpoint: "https://secondary.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}}})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
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

func TestNew_TypedNilResolverIsTreatedAsUnset(t *testing.T) {
	var resolver *fakeResolver

	svc := mustNewService(t, Options{Resolver: resolver})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Upload panicked with typed-nil resolver: %v", r)
		}
	}()

	_, err := svc.Upload(context.Background(), bytes.NewReader(bytes.Repeat([]byte("ab"), 128)), nil)
	if err == nil || !strings.Contains(err.Error(), "no upload resolver configured") {
		t.Fatalf("err=%v want no upload resolver configured", err)
	}
}

func TestManagerUpload_AllCommitsFailReturnsCommitError(t *testing.T) {
	data := bytes.Repeat([]byte("ef"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return nil, errors.New("primary commit failed")
		},
	}
	secondary := &fakeUploadContext{
		id:       types.ProviderID(202),
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

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}}})

	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err == nil {
		t.Fatal("expected CommitError")
	}
	var got *CommitError
	if !errors.As(err, &got) {
		t.Fatalf("want CommitError, got %T", err)
	}
	if got.ProviderID != primary.id {
		t.Fatalf("providerID=%d want %d", got.ProviderID, primary.id)
	}
}

func TestManagerUpload_ImplicitSecondaryReplacement(t *testing.T) {
	data := bytes.Repeat([]byte("gh"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1001), PieceIDs: []types.PieceID{types.PieceID(2001)}}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.ProviderID(202),
		endpoint: "https://secondary-a.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}
	replacement := &fakeUploadContext{
		id:       types.ProviderID(303),
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
			return &CommitResult{DataSetID: types.DataSetID(1002), PieceIDs: []types.PieceID{types.PieceID(2002)}}, nil
		},
	}

	resolver := &fakeResolver{
		contexts:     []UploadContext{primary, failedSecondary},
		replacements: []UploadContext{replacement},
	}
	mgr := mustNewService(t, Options{Resolver: resolver})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
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
	if got.Copies[1].ProviderID != replacement.id {
		t.Fatalf("replacement provider=%d want %d", got.Copies[1].ProviderID, replacement.id)
	}
}

func TestManagerUpload_ReplacementAutoFetchesClientDataSetID(t *testing.T) {
	data := bytes.Repeat([]byte("ij"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1001), PieceIDs: []types.PieceID{types.PieceID(2001)}}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.ProviderID(202),
		endpoint: "https://secondary-a.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}

	const dsID types.DataSetID = 404
	replacementClient := &fakeCurioClient{
		pullPiecesFn: func(_ context.Context, req icurio.PullRequest) (*icurio.PullResult, error) {
			if req.DataSetID != uint64(dsID) {
				t.Fatalf("pull dataSetID=%d want %d", req.DataSetID, uint64(dsID))
			}
			if len(req.ExtraData) == 0 {
				t.Fatal("pull extraData should be populated for replacement existing dataset")
			}
			return &icurio.PullResult{
				Status: icurio.PullStatusComplete,
				Pieces: []icurio.PullPieceStatus{{PieceCID: info.CIDv2.String(), Status: icurio.PullStatusComplete}},
			}, nil
		},
		addPiecesFn: func(_ context.Context, gotDataSetID uint64, pieces []icurio.AddPieceInput, extraData []byte) (*icurio.AddPiecesResult, error) {
			if gotDataSetID != uint64(dsID) {
				t.Fatalf("commit dataSetID=%d want %d", gotDataSetID, uint64(dsID))
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("unexpected pieces: %+v", pieces)
			}
			if len(extraData) == 0 {
				t.Fatal("commit extraData should be populated for replacement existing dataset")
			}
			return &icurio.AddPiecesResult{TxHash: common.HexToHash("0x02"), StatusURL: "https://secondary-b.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, statusURL string, _ time.Duration) (*icurio.AddPiecesStatus, error) {
			if statusURL == "" {
				t.Fatal("empty statusURL")
			}
			return &icurio.AddPiecesStatus{
				TxHash:            common.HexToHash("0x02"),
				DataSetID:         uint64(dsID),
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(2002)},
			}, nil
		},
	}
	replacement, err := NewContext(testProvider(), replacementClient, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(dsID),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: big.NewInt(0xFEED)},
	}
	resolver := &fakeResolver{
		contexts:     []UploadContext{primary, failedSecondary},
		replacements: []UploadContext{replacement},
	}
	mgr := mustNewService(t, Options{Resolver: resolver, FWSSDataSetReader: reader})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("reader.calls=%d want 1 (replacement existing dataset should auto-fetch once)", reader.calls)
	}
	if replacement.clientDataSetID == nil {
		t.Fatal("replacement clientDataSetID should be backfilled")
	}
	if len(got.Copies) != 2 || got.Copies[1].ProviderID != replacement.ProviderID() {
		t.Fatalf("copies=%+v want replacement provider %d in second slot", got.Copies, replacement.ProviderID())
	}
}

func TestManagerUpload_ReplacementPopulateFailureAdvancesCurrentProvider(t *testing.T) {
	data := bytes.Repeat([]byte("kl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1001), PieceIDs: []types.PieceID{types.PieceID(2001)}}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.ProviderID(202),
		endpoint: "https://secondary-a.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}

	replacementProvider := testProvider()
	replacementProvider.ID = types.ProviderID(404)
	replacementCtx, err := NewContext(replacementProvider, &fakeCurioClient{}, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithDataSetID(types.DataSetID(404)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	replacement2 := &fakeUploadContext{
		id:       types.ProviderID(303),
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
			return &CommitResult{DataSetID: types.DataSetID(1002), PieceIDs: []types.PieceID{types.PieceID(2002)}}, nil
		},
	}

	reader := &fakeFWSSDataSetReader{infoErr: errors.New("fwss unavailable")}
	resolver := &fakeResolver{
		contexts:     []UploadContext{primary, failedSecondary},
		replacements: []UploadContext{replacementCtx, replacement2},
	}
	mgr := mustNewService(t, Options{
		Resolver:             resolver,
		MaxSecondaryAttempts: 3,
		FWSSDataSetReader:    reader,
	})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got.Copies) != 2 || got.Copies[1].ProviderID != replacement2.id {
		t.Fatalf("copies=%+v want replacement provider %d in second slot", got.Copies, replacement2.id)
	}
	if len(got.FailedAttempts) != 3 {
		t.Fatalf("FailedAttempts=%+v want 3 entries", got.FailedAttempts)
	}
	if got.FailedAttempts[2].ProviderID != replacementProvider.ID {
		t.Fatalf("last failed provider=%d want replacement provider %d (retry should advance to replacement after populate failure)", got.FailedAttempts[2].ProviderID, replacementProvider.ID)
	}
}

type fakeResolver struct {
	contexts         []UploadContext
	explicit         bool
	replacements     []UploadContext
	replacementCalls int
	captureFn        func(*UploadOptions)
}

func (r *fakeResolver) ResolveUploadContexts(_ context.Context, opts *UploadOptions) ([]UploadContext, bool, error) {
	if r.captureFn != nil {
		r.captureFn(opts)
	}
	return r.contexts, r.explicit, nil
}

func (r *fakeResolver) SelectReplacement(_ context.Context, _ map[types.ProviderID]struct{}, _ *UploadOptions) (UploadContext, error) {
	r.replacementCalls++
	if len(r.replacements) == 0 {
		return nil, errors.New("no replacement")
	}
	next := r.replacements[0]
	r.replacements = r.replacements[1:]
	return next, nil
}

type fakeUploadContext struct {
	id              types.ProviderID
	endpoint        string
	pieceURL        string
	dataSetID       *types.DataSetID
	clientDataSetID types.ClientDataSetID
	dataSetMetadata map[string]string
	storeFn         func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error)
	presignFn       func(context.Context, []PieceInput) ([]byte, error)
	pullFn          func(context.Context, PullRequest) (*PullResult, error)
	commitFn        func(context.Context, CommitRequest) (*CommitResult, error)
}

func (c *fakeUploadContext) ProviderID() types.ProviderID { return c.id }
func (c *fakeUploadContext) ServiceURL() string           { return c.endpoint }
func (c *fakeUploadContext) PieceURL(_ cid.Cid) string    { return c.pieceURL }

func (c *fakeUploadContext) Store(ctx context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
	if c.storeFn == nil {
		return nil, fmt.Errorf("unexpected store")
	}
	return c.storeFn(ctx, r, opts)
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

// TestManagerUpload_RequestedCopiesIsCallerRequested proves that
// UploadResult.RequestedCopies reflects the caller's intent (opts.Copies,
// default 2), not the number of contexts the resolver happened to return.
// When fewer contexts are available the result must have Complete=false.
func TestManagerUpload_RequestedCopiesIsCallerRequested(t *testing.T) {
	data := bytes.Repeat([]byte("rc"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}, IsNewDataSet: true}, nil
		},
	}

	// Resolver returns only 1 context even though caller requests 3 copies.
	mgr := &Service{
		resolver:   &fakeResolver{contexts: []UploadContext{primary}},
		httpClient: &http.Client{},
	}

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 3})
	if err != nil {
		t.Fatalf("Upload: %v", err)
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

// TestManagerUpload_NilPullResultNoNilDeref proves that a nil PullResult
// returned alongside a nil error is handled gracefully (no panic).
func TestManagerUpload_NilPullResultNoNilDeref(t *testing.T) {
	data := bytes.Repeat([]byte("np"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}, IsNewDataSet: true}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.ProviderID(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		// Returns (nil, nil) — the nil-deref case.
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, nil
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}, explicit: true}})

	// Should not panic; primary copy should still succeed.
	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got.Copies) != 1 || got.Copies[0].Role != CopyRolePrimary {
		t.Fatalf("expected only primary copy, got %+v", got.Copies)
	}
	if len(got.FailedAttempts) != 1 || got.FailedAttempts[0].Stage != CopyStagePull {
		t.Fatalf("expected one pull failure, got %+v", got.FailedAttempts)
	}
}

// TestManagerUpload_PresignFailureUsesPresignStage proves that a presign
// error is recorded with CopyStagePresign, not CopyStageCommit.
func TestManagerUpload_PresignFailureUsesPresignStage(t *testing.T) {
	data := bytes.Repeat([]byte("ps"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}, IsNewDataSet: true}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.ProviderID(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return nil, errors.New("presign failed: no signer")
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}, explicit: true}})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got.FailedAttempts) != 1 {
		t.Fatalf("FailedAttempts=%d want 1", len(got.FailedAttempts))
	}
	if got.FailedAttempts[0].Stage != CopyStagePresign {
		t.Fatalf("Stage=%s want %s", got.FailedAttempts[0].Stage, CopyStagePresign)
	}
}

func TestManagerUpload_NilReader(t *testing.T) {
	mgr := mustNewService(t, Options{})
	_, err := mgr.Upload(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil reader")
	}
}

func TestManagerUpload_ReadError(t *testing.T) {
	readErr := errors.New("read boom")
	ctx := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			_, err := io.ReadAll(r)
			if err != nil {
				return nil, err
			}
			return nil, errors.New("unexpected: reader should have errored")
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{ctx}}}
	_, err := mgr.Upload(context.Background(), iotest.ErrReader(readErr), nil)
	if err == nil {
		t.Fatal("expected error for failing reader")
	}
	if !errors.Is(err, readErr) {
		t.Fatalf("expected error wrapping %q, got %q", readErr, err)
	}
}

func TestManagerUpload_StreamsToPrimary(t *testing.T) {
	data := bytes.Repeat([]byte("rd"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			got, err := io.ReadAll(r)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if !bytes.Equal(got, data) {
				t.Fatal("data mismatch")
			}
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}}, nil
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{primary}}}

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got.PieceCID != info.CIDv2 {
		t.Fatalf("PieceCID=%s want %s", got.PieceCID, info.CIDv2)
	}
}

// TestManagerUpload_LargeReader verifies that a large, non-Seekable reader
// is streamed to the primary Store without being buffered in memory by
// the Manager itself. We use a 32 MiB io.LimitReader wrapped in an
// anonymous struct to hide any Seek/Len method set.
func TestManagerUpload_LargeReader(t *testing.T) {
	const size = 32 << 20
	src := struct{ io.Reader }{io.LimitReader(zeroReader{}, size)}

	info, err := piece.CalculateFromBytes(bytes.Repeat([]byte{0}, 256))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	var read int64
	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			n, err := io.Copy(io.Discard, r)
			if err != nil {
				t.Fatalf("copy: %v", err)
			}
			read = n
			return &StoreResult{PieceCID: info.CIDv2, Size: n}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}}, nil
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{primary}}}

	if _, err := mgr.Upload(context.Background(), src, &UploadOptions{Copies: 1}); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if read != size {
		t.Fatalf("read=%d want %d", read, size)
	}
}

// TestManagerUpload_WithPieceCIDPrefill verifies opts.PieceCID flows
// through to StoreOptions.
func TestManagerUpload_WithPieceCIDPrefill(t *testing.T) {
	data := bytes.Repeat([]byte("pf"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	var gotPC cid.Cid
	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			if opts != nil {
				gotPC = opts.PieceCID
			}
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}}, nil
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{primary}}}
	_, err = mgr.Upload(context.Background(), bytes.NewReader(data),
		&UploadOptions{Copies: 1, PieceCID: info.CIDv2})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotPC != info.CIDv2 {
		t.Fatalf("StoreOptions.PieceCID=%s want %s", gotPC, info.CIDv2)
	}
}

// TestManagerUpload_OnProgress verifies opts.OnProgress flows through.
func TestManagerUpload_OnProgress(t *testing.T) {
	data := bytes.Repeat([]byte("op"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	var cbSeen bool
	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			if opts != nil && opts.OnProgress != nil {
				cbSeen = true
			}
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}}, nil
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{primary}}}
	_, err = mgr.Upload(context.Background(), bytes.NewReader(data),
		&UploadOptions{Copies: 1, OnProgress: func(int64) {}})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !cbSeen {
		t.Fatal("expected StoreOptions.OnProgress forwarded")
	}
}

func TestManagerUpload_CtxCancelSkipsQueuedCommits(t *testing.T) {
	data := bytes.Repeat([]byte("cq"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	started := make(chan struct{})
	var (
		mu          sync.Mutex
		commitCalls []string
	)
	recordCommit := func(name string) {
		mu.Lock()
		first := len(commitCalls) == 0
		commitCalls = append(commitCalls, name)
		mu.Unlock()
		if first {
			close(started)
		}
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		pieceURL: "https://p.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(ctx context.Context, _ CommitRequest) (*CommitResult, error) {
			recordCommit("primary")
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	newSecondary := func(id types.ProviderID, name string) *fakeUploadContext {
		return &fakeUploadContext{
			id:       id,
			endpoint: "https://s.example.com",
			presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
				return []byte{0x01}, nil
			},
			pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
				return &PullResult{
					Status: PullStatusComplete,
					Pieces: []PullPieceResult{{PieceCID: info.CIDv2, Status: PullStatusComplete}},
				}, nil
			},
			commitFn: func(ctx context.Context, _ CommitRequest) (*CommitResult, error) {
				recordCommit(name)
				<-ctx.Done()
				return nil, ctx.Err()
			},
		}
	}

	mgr := mustNewService(t, Options{
		Resolver:          &fakeResolver{contexts: []UploadContext{primary, newSecondary(2, "secondary-1"), newSecondary(3, "secondary-2")}},
		CommitConcurrency: 1,
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := mgr.Upload(ctx, bytes.NewReader(data), &UploadOptions{Copies: 3})
		done <- err
	}()

	<-started
	cancel()

	err = <-done
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v want context.Canceled", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(commitCalls) != 1 {
		t.Fatalf("queued commits should not start after cancel; calls=%v", commitCalls)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

func TestWithUploadResolver(t *testing.T) {
	r := &fakeResolver{}
	mgr := mustNewService(t, Options{Resolver: r})
	if mgr.resolver != r {
		t.Fatal("WithUploadResolver did not set resolver")
	}
}

func TestWithSource(t *testing.T) {
	mgr := mustNewService(t, Options{Source: "my-app"})
	if mgr.source != "my-app" {
		t.Fatalf("source=%q want my-app", mgr.source)
	}
}

func TestWithSourceMetadata(t *testing.T) {
	m := &Service{httpClient: &http.Client{}, source: "app"}

	// nil opts → creates new opts with source
	got := m.withSourceMetadata(nil)
	if got == nil || got.DataSetMetadata["source"] != "app" {
		t.Fatalf("nil opts: got=%+v", got)
	}

	// existing source key → caller wins
	existing := &UploadOptions{DataSetMetadata: map[string]string{"source": "override"}}
	got = m.withSourceMetadata(existing)
	if got.DataSetMetadata["source"] != "override" {
		t.Fatalf("caller override: source=%q want override", got.DataSetMetadata["source"])
	}
	if got != existing {
		t.Fatal("should return same pointer when caller overrides")
	}

	// no source key → injects source
	noSource := &UploadOptions{DataSetMetadata: map[string]string{"env": "prod"}}
	got = m.withSourceMetadata(noSource)
	if got.DataSetMetadata["source"] != "app" {
		t.Fatalf("inject source: source=%q want app", got.DataSetMetadata["source"])
	}
	if got.DataSetMetadata["env"] != "prod" {
		t.Fatal("existing keys should be preserved")
	}
	// original should not be mutated
	if _, ok := noSource.DataSetMetadata["source"]; ok {
		t.Fatal("original opts should not be mutated")
	}
}

// TestResolveWithCDN_TriState asserts the tri-state resolution between
// Service.defaultWithCDN and UploadOptions.WithCDN: caller-provided non-nil
// always wins; nil inherits the Service default.
func TestResolveWithCDN_TriState(t *testing.T) {
	bTrue, bFalse := true, false

	cases := []struct {
		name    string
		defCDN  bool
		inOpts  *UploadOptions
		want    bool
		wantPtr bool // ensure returned WithCDN is non-nil (normalized)
	}{
		{"nil opts, default false", false, nil, false, true},
		{"nil opts, default true", true, nil, true, true},
		{"nil WithCDN inherits false", false, &UploadOptions{Copies: 1}, false, true},
		{"nil WithCDN inherits true", true, &UploadOptions{Copies: 1}, true, true},
		{"explicit true overrides false default", false, &UploadOptions{WithCDN: &bTrue}, true, true},
		{"explicit false overrides true default", true, &UploadOptions{WithCDN: &bFalse}, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{defaultWithCDN: tc.defCDN}
			got := s.resolveWithCDN(tc.inOpts)
			if got == nil {
				t.Fatal("resolveWithCDN returned nil")
			}
			if tc.wantPtr && got.WithCDN == nil {
				t.Fatal("WithCDN should be non-nil after resolution")
			}
			if got.WithCDN != nil && *got.WithCDN != tc.want {
				t.Fatalf("WithCDN=%v want %v", *got.WithCDN, tc.want)
			}
			// Must not mutate caller-provided opts.
			if tc.inOpts != nil && tc.inOpts.WithCDN == nil && got == tc.inOpts {
				t.Fatal("resolveWithCDN mutated caller opts (should clone when setting default)")
			}
		})
	}
}

// TestNew_ZeroOptions asserts that Service works with zero Options:
// default HTTP client with non-zero timeout is installed, MaxSecondaryAttempts
// falls back to the package default, and Upload fails cleanly (no panic)
// because no resolver is configured.
func TestNew_ZeroOptions(t *testing.T) {
	s, err := New(Options{})
	if err != nil {
		t.Fatalf("New(Options{}): %v", err)
	}
	if s.httpClient == nil {
		t.Fatal("default HTTPClient should be installed")
	}
	if s.httpClient.Timeout == 0 {
		t.Fatal("default HTTP client must have a non-zero timeout")
	}
	if s.maxSecondaryAttempts != maxSecondaryAttemptsDefault {
		t.Fatalf("maxSecondaryAttempts = %d, want default %d", s.maxSecondaryAttempts, maxSecondaryAttemptsDefault)
	}
}

// TestNew_ZeroOptions_UploadReturnsError locks in that a zero-Options
// Service does not panic on Upload; it returns a clean validation error
// because no resolver was provided.
func TestNew_ZeroOptions_UploadReturnsError(t *testing.T) {
	s, err := New(Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = s.Upload(context.Background(), bytes.NewReader([]byte("hi")), nil)
	if err == nil {
		t.Fatal("expected error from Upload without resolver")
	}
}

// TestNew_ExplicitHTTPClient asserts a caller-provided HTTP client is kept
// verbatim (not overwritten by the default).
func TestNew_ExplicitHTTPClient(t *testing.T) {
	custom := &http.Client{Timeout: time.Second}
	s, err := New(Options{HTTPClient: custom})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.httpClient != custom {
		t.Fatal("caller-supplied HTTPClient should be kept")
	}
}

func TestCloneMetadata(t *testing.T) {
	// nil opts
	if got := cloneMetadata(nil); got != nil {
		t.Fatalf("nil opts: got=%v want nil", got)
	}
	// empty metadata
	if got := cloneMetadata(&UploadOptions{}); got != nil {
		t.Fatalf("empty metadata: got=%v want nil", got)
	}
	// non-empty: returns clone
	orig := &UploadOptions{PieceMetadata: map[string]string{"k": "v"}}
	cloned := cloneMetadata(orig)
	if cloned["k"] != "v" {
		t.Fatalf("cloned[k]=%q want v", cloned["k"])
	}
	// mutating clone must not affect original
	cloned["k"] = "changed"
	if orig.PieceMetadata["k"] != "v" {
		t.Fatal("clone mutated original")
	}
}

func TestRequestedCopiesForUpload(t *testing.T) {
	tests := []struct {
		name string
		opts *UploadOptions
		want int
	}{
		{"nil opts defaults to 2", nil, 2},
		{"explicit Copies", &UploadOptions{Copies: 5}, 5},
		{"DataSetIDs count", &UploadOptions{DataSetIDs: []types.DataSetID{1, 2}}, 2},
		{"ProviderIDs count", &UploadOptions{ProviderIDs: []types.ProviderID{10}}, 1},
		{"zero Copies, no IDs defaults to 2", &UploadOptions{}, 2},
		{"DataSetIDs deduplicated to 1 copy", &UploadOptions{DataSetIDs: []types.DataSetID{1, 1}}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestedCopiesForUpload(tt.opts); got != tt.want {
				t.Fatalf("requestedCopiesForUpload()=%d want %d", got, tt.want)
			}
		})
	}
}

func TestManagerUpload_SourceInjectedIntoMetadata(t *testing.T) {
	data := bytes.Repeat([]byte("src"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}}, nil
		},
	}

	var capturedOpts *UploadOptions
	resolver := &fakeResolver{
		contexts: []UploadContext{primary},
		captureFn: func(opts *UploadOptions) {
			capturedOpts = opts
		},
	}

	mgr := &Service{httpClient: &http.Client{}, resolver: resolver, source: "test-app"}
	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if capturedOpts == nil {
		t.Fatal("resolver did not receive opts")
	}
	if capturedOpts.DataSetMetadata["source"] != "test-app" {
		t.Fatalf("source=%q want test-app", capturedOpts.DataSetMetadata["source"])
	}
}

// TestManagerUpload_CommitResultMissingIdentifiers proves that a commit
// result with missing identifiers (nil DataSetID or empty PieceIDs) is treated
// as a failed attempt.
func TestManagerUpload_CommitResultMissingIdentifiers(t *testing.T) {
	data := bytes.Repeat([]byte("mi"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(0), PieceIDs: nil}, nil
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{primary}}}
	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if err == nil {
		t.Fatal("expected CommitError when identifiers missing")
	}
	var ce *CommitError
	if !errors.As(err, &ce) {
		t.Fatalf("want CommitError, got %T", err)
	}
}

// TestManagerUpload_CommitResultZeroDataSetID proves that a commit result with
// confirmed piece IDs but no assigned data set ID is still treated as invalid.
func TestManagerUpload_CommitResultZeroDataSetID(t *testing.T) {
	data := bytes.Repeat([]byte("zd"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: 0, PieceIDs: []types.PieceID{types.PieceID(10)}}, nil
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{primary}}}
	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if err == nil {
		t.Fatal("expected CommitError when dataSetID is zero")
	}
	var ce *CommitError
	if !errors.As(err, &ce) {
		t.Fatalf("want CommitError, got %T", err)
	}
}

// TestManagerUpload_CommitResultAllowsZeroPieceID proves TS-compatible piece
// ID 0 is accepted as a successful confirmation.
func TestManagerUpload_CommitResultAllowsZeroPieceID(t *testing.T) {
	data := bytes.Repeat([]byte("mid"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{0}}, nil
		},
	}
	mgr := &Service{httpClient: &http.Client{}, resolver: &fakeResolver{contexts: []UploadContext{primary}}}
	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if got.SuccessCount() != 1 {
		t.Fatalf("success count=%d want 1", got.SuccessCount())
	}
	if len(got.Copies) != 1 || got.Copies[0].PieceID != 0 {
		t.Fatalf("copies=%+v want pieceID 0", got.Copies)
	}
}

// TestManagerUpload_PullStatusNotComplete proves that a non-complete pull
// status with nil error is recorded as a failed pull attempt.
func TestManagerUpload_PullStatusNotComplete(t *testing.T) {
	data := bytes.Repeat([]byte("nc"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.DataSetID(1), PieceIDs: []types.PieceID{types.PieceID(10)}}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.ProviderID(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return &PullResult{Status: PullStatusFailed}, nil
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary, secondary}, explicit: true}})
	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got.FailedAttempts) != 1 || got.FailedAttempts[0].Stage != CopyStagePull {
		t.Fatalf("FailedAttempts=%+v, want 1 pull failure", got.FailedAttempts)
	}
}

func TestWithMaxSecondaryAttempts(t *testing.T) {
	// Positive value is applied.
	mgr := mustNewService(t, Options{MaxSecondaryAttempts: 3})
	if mgr.maxSecondaryAttempts != 3 {
		t.Fatalf("maxSecondaryAttempts = %d, want 3", mgr.maxSecondaryAttempts)
	}

	// Zero is ignored; default of 5 is preserved.
	mgr = mustNewService(t, Options{MaxSecondaryAttempts: 0})
	if mgr.maxSecondaryAttempts != maxSecondaryAttemptsDefault {
		t.Fatalf("maxSecondaryAttempts = %d after n=0, want default %d", mgr.maxSecondaryAttempts, maxSecondaryAttemptsDefault)
	}

	// Negative value is ignored; default is preserved.
	mgr = mustNewService(t, Options{MaxSecondaryAttempts: -1})
	if mgr.maxSecondaryAttempts != maxSecondaryAttemptsDefault {
		t.Fatalf("maxSecondaryAttempts = %d after n=-1, want default %d", mgr.maxSecondaryAttempts, maxSecondaryAttemptsDefault)
	}

	// Boundary: n=1 is accepted.
	mgr = mustNewService(t, Options{MaxSecondaryAttempts: 1})
	if mgr.maxSecondaryAttempts != 1 {
		t.Fatalf("maxSecondaryAttempts = %d after n=1, want 1", mgr.maxSecondaryAttempts)
	}
}
