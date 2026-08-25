package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func mustNewService(t *testing.T, opts Options) *Service {
	t.Helper()
	if opts.SignerAddress == (common.Address{}) && opts.Signer == nil {
		opts.SignerAddress = testPayer()
	}
	if !opts.ChainID.IsValid() {
		opts.ChainID = types.ChainID(314159)
	}
	if opts.RecordKeeper == (common.Address{}) {
		opts.RecordKeeper = testRecordKeeper()
	}
	s, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestManagerNewDataSetContext_AllowsEndedDataSet(t *testing.T) {
	dataSetID := types.NewBigInt(13269)
	providerID := types.NewBigInt(2)
	clientDataSetID := types.NewBigInt(99)
	fixture := serviceResolverFixture{
		dataSetsByID: map[string]*warmstorage.DataSetInfo{
			testIDKey(13269): {
				DataSetID:       dataSetID,
				ProviderID:      providerID,
				Payer:           testPayer(),
				PDPEndEpoch:     3778900,
				ClientDataSetID: clientDataSetID,
			},
		},
		providersByID: map[string]*spregistry.PDPProvider{
			testIDKey(2): ptrPDPProvider(testPDPProvider(providerID, "https://sp-2.example.com")),
		},
		validatorEnabled: true,
		validatorErr:     errors.New("not live"),
	}
	catalog := &fakeEnhancedDataSetCatalog{fakeDataSetCatalog: fakeDataSetCatalog{fixture: fixture}}
	resolver, err := NewServiceResolver(ServiceResolverOptions{
		Payer:       testPayer(),
		SPRegistry:  &fakePDPProviderSource{fixture: fixture},
		WarmStorage: catalog,
		NewContext: func(provider Provider, opts ContextFactoryOptions) (*ProviderContext, error) {
			return newTestContextForSelection(t, provider, opts, &fakePDPProviderClient{},
				WithPayer(testPayer()),
				WithChainID(types.ChainID(314159)),
				WithRecordKeeper(testRecordKeeper()),
			)
		},
	})
	if err != nil {
		t.Fatalf("NewServiceResolver: %v", err)
	}
	mgr := mustNewService(t, Options{Resolver: resolver})

	got, err := mgr.NewDataSetContext(context.Background(), dataSetID, NewDataSetContextOptions{})
	if err != nil {
		t.Fatalf("NewDataSetContext: %v", err)
	}
	ref, ok := got.DataSetRef()
	if !ok || !ref.DataSetID().Equal(dataSetID) {
		t.Fatalf("DataSetRef()=(%+v, %t) want dataSetID %s", ref, ok, dataSetID.String())
	}
}

func TestManagerUpload_SafetyNetRejectsEndedDataSetBeforeStore(t *testing.T) {
	dataSetID := types.NewBigInt(88)
	storeCalled := false
	ctxWithEndedDataSet := &fakeUploadContext{
		id:        types.NewBigInt(7),
		endpoint:  "https://sp-7.example.com",
		dataSetID: &dataSetID,
		storeFn: func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
			storeCalled = true
			t.Fatal("Store must not run when FWSS data set is ended")
			return nil, nil
		},
	}
	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{
			DataSetID:   dataSetID,
			PDPEndEpoch: 1234,
		},
	}
	mgr := mustNewService(t, Options{
		Resolver:          &fakeResolver{contexts: []StorageContext{ctxWithEndedDataSet}},
		FWSSDataSetReader: reader,
	})

	_, err := mgr.Upload(context.Background(), bytes.NewReader([]byte("payload")), &UploadOptions{Copies: 1})
	requireDataSetPDPPaymentTerminated(t, err, dataSetID, 1234)
	if storeCalled {
		t.Fatal("Store was called")
	}
	if reader.calls != 1 || !reader.gotID.Equal(dataSetID) {
		t.Fatalf("reader calls=%d gotID=%s want one call for %s", reader.calls, reader.gotID.String(), dataSetID.String())
	}
}

func TestManagerUpload_SafetyNetReaderErrorFailsBeforeStore(t *testing.T) {
	data := bytes.Repeat([]byte("up"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	dataSetID := types.NewBigInt(88)
	boom := errors.New("fwss unavailable")
	storeCalled := false
	ctxWithDataSet := &fakeUploadContext{
		id:        types.NewBigInt(7),
		endpoint:  "https://sp-7.example.com",
		dataSetID: &dataSetID,
		storeFn: func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
			storeCalled = true
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
	}
	reader := &fakeFWSSDataSetReader{infoErr: boom}
	mgr := mustNewService(t, Options{
		Resolver:          &fakeResolver{contexts: []StorageContext{ctxWithDataSet}},
		FWSSDataSetReader: reader,
	})

	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if !errors.Is(err, boom) {
		t.Fatalf("Upload error=%v want wrap of %v", err, boom)
	}
	if storeCalled {
		t.Fatal("Store was called")
	}
}

func TestManagerUpload_SafetyNetRejectsEndedReplacementBeforePull(t *testing.T) {
	data := bytes.Repeat([]byte("rp"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	dataSetID := types.NewBigInt(88)
	replacementPresignCalled := false
	primary := &fakeUploadContext{
		id:       types.NewBigInt(1),
		endpoint: "https://sp-1.example.com",
		storeFn: func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(context.Context, CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(10), PieceIDs: []types.BigInt{types.NewBigInt(100)}, IsNewDataSet: true}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.NewBigInt(2),
		endpoint: "https://sp-2.example.com",
		presignFn: func(context.Context, []PieceInput) ([]byte, error) {
			return nil, errors.New("presign failed")
		},
	}
	replacement := &fakeUploadContext{
		id:        types.NewBigInt(3),
		endpoint:  "https://sp-3.example.com",
		dataSetID: &dataSetID,
		presignFn: func(context.Context, []PieceInput) ([]byte, error) {
			replacementPresignCalled = true
			t.Fatal("replacement PresignForCommit must not run for an ended data set")
			return nil, nil
		},
	}
	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{
			DataSetID:   dataSetID,
			PDPEndEpoch: 1234,
		},
	}
	mgr := mustNewService(t, Options{
		Resolver: &fakeResolver{
			contexts:     []StorageContext{primary, failedSecondary},
			replacements: []StorageContext{replacement},
		},
		FWSSDataSetReader: reader,
	})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if replacementPresignCalled {
		t.Fatal("replacement PresignForCommit was called")
	}
	if reader.calls != 1 || !reader.gotID.Equal(dataSetID) {
		t.Fatalf("reader calls=%d gotID=%s want one call for %s", reader.calls, reader.gotID.String(), dataSetID.String())
	}
	if got.SuccessCount() != 1 || got.Complete {
		t.Fatalf("result success=%d complete=%v want one partial primary success", got.SuccessCount(), got.Complete)
	}
	if len(got.FailedAttempts) < 2 {
		t.Fatalf("failed attempts=%+v want replacement failure", got.FailedAttempts)
	}
	requireDataSetPDPPaymentTerminated(t, got.FailedAttempts[1].Err, dataSetID, 1234)
}

func TestManagerUpload_ReplacementPreflightFailureConsumesAttemptBudget(t *testing.T) {
	data := bytes.Repeat([]byte("rs"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	replacementDataSetID := types.NewBigInt(88)
	replacementPresignCalled := false
	primary := &fakeUploadContext{
		id:       types.NewBigInt(1),
		endpoint: "https://sp-1.example.com",
		pieceURL: "https://sp-1.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(context.Context, CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(10), PieceIDs: []types.BigInt{types.NewBigInt(100)}, IsNewDataSet: true}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.NewBigInt(2),
		endpoint: "https://sp-2.example.com",
		presignFn: func(context.Context, []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(context.Context, PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}
	badReplacement := &fakeUploadContext{
		id:        types.NewBigInt(3),
		endpoint:  "https://sp-3.example.com",
		dataSetID: &replacementDataSetID,
		presignFn: func(context.Context, []PieceInput) ([]byte, error) {
			replacementPresignCalled = true
			return nil, errors.New("replacement presign should not run")
		},
	}
	goodReplacement := &fakeUploadContext{
		id:       types.NewBigInt(4),
		endpoint: "https://sp-4.example.com",
		presignFn: func(context.Context, []PieceInput) ([]byte, error) {
			t.Fatal("second replacement must not run after attempt budget is exhausted")
			return nil, nil
		},
		pullFn: func(context.Context, PullRequest) (*PullResult, error) {
			return &PullResult{
				Status: PullStatusComplete,
				Pieces: []PullPieceResult{{PieceCID: info.CIDv2, Status: PullStatusComplete}},
			}, nil
		},
		commitFn: func(context.Context, CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(20), PieceIDs: []types.BigInt{types.NewBigInt(200)}}, nil
		},
	}
	reader := &fakeFWSSDataSetReader{infoErr: errors.New("fwss unavailable")}
	mgr := mustNewService(t, Options{
		Resolver: &fakeResolver{
			contexts:     []StorageContext{primary, failedSecondary},
			replacements: []StorageContext{badReplacement, goodReplacement},
		},
		MaxSecondaryAttempts: 2,
		FWSSDataSetReader:    reader,
	})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got.Copies) != 1 || !got.Copies[0].ProviderID.Equal(primary.id) {
		t.Fatalf("copies=%+v want only primary after replacement preflight exhausts budget", got.Copies)
	}
	if replacementPresignCalled {
		t.Fatal("bad replacement PresignForCommit was called")
	}
	if got.Complete {
		t.Fatal("upload should be partial after exhausting secondary attempt budget")
	}
	if len(got.FailedAttempts) != 2 {
		t.Fatalf("failedAttempts=%+v want initial secondary and one replacement failure", got.FailedAttempts)
	}
}

func TestManagerUpload_ExplicitCopiesAndPresignReuse(t *testing.T) {
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
		id:       types.NewBigInt(101),
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
				DataSetID:     types.NewBigInt(1001),
				PieceIDs:      []types.BigInt{types.NewBigInt(2001)},
				IsNewDataSet:  true,
				TransactionID: "0xprimary",
			}, nil
		},
	}

	secondaryExtra := []byte{0xde, 0xad, 0xbe, 0xef}
	secondary := &fakeUploadContext{
		id:       types.NewBigInt(202),
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
				DataSetID:     types.NewBigInt(1002),
				PieceIDs:      []types.BigInt{types.NewBigInt(2002)},
				IsNewDataSet:  true,
				TransactionID: "0xsecondary",
			}, nil
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary, secondary}}})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
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
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return nil, want
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})

	_, err := mgr.Upload(context.Background(), bytes.NewReader(bytes.Repeat([]byte("ab"), 128)), &UploadOptions{Copies: 1})
	if err == nil {
		t.Fatal("expected StoreError")
	}
	got, ok := errors.AsType[*StoreError](err)
	if !ok {
		t.Fatalf("want StoreError, got %T", err)
	}
	if !got.ProviderID.Equal(primary.id) {
		t.Fatalf("providerID=%s want %s", got.ProviderID.String(), primary.id.String())
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
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1001), PieceIDs: []types.BigInt{types.NewBigInt(2001)}}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.NewBigInt(202),
		endpoint: "https://secondary.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary, secondary}}})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
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

	_, err := svc.Upload(context.Background(), bytes.NewReader(bytes.Repeat([]byte("ab"), 128)), &UploadOptions{Copies: 1})
	if !errors.Is(err, ErrUninitialized) || !strings.Contains(err.Error(), "upload resolver not configured") {
		t.Fatalf("err=%v want ErrUninitialized for missing upload resolver", err)
	}
}

func TestManagerUpload_AllCommitsFailReturnsCommitError(t *testing.T) {
	data := bytes.Repeat([]byte("ef"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(101),
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
		id:       types.NewBigInt(202),
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

	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary, secondary}}})

	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
	if err == nil {
		t.Fatal("expected CommitError")
	}
	got, ok := errors.AsType[*CommitError](err)
	if !ok {
		t.Fatalf("want CommitError, got %T", err)
	}
	if !got.ProviderID.Equal(primary.id) {
		t.Fatalf("providerID=%s want %s", got.ProviderID.String(), primary.id.String())
	}
}

func TestManagerUpload_ImplicitSecondaryReplacement(t *testing.T) {
	data := bytes.Repeat([]byte("gh"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1001), PieceIDs: []types.BigInt{types.NewBigInt(2001)}}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.NewBigInt(202),
		endpoint: "https://secondary-a.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}
	replacement := &fakeUploadContext{
		id:       types.NewBigInt(303),
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
			return &CommitResult{DataSetID: types.NewBigInt(1002), PieceIDs: []types.BigInt{types.NewBigInt(2002)}}, nil
		},
	}

	resolver := &fakeResolver{
		contexts:     []StorageContext{primary, failedSecondary},
		replacements: []StorageContext{replacement},
	}
	mgr := mustNewService(t, Options{Resolver: resolver})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
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
	if !got.Copies[1].ProviderID.Equal(replacement.id) {
		t.Fatalf("replacement provider=%s want %s", got.Copies[1].ProviderID.String(), replacement.id.String())
	}
}

func TestManagerUpload_ReplacementKeepsImmutableClientDataSetID(t *testing.T) {
	data := bytes.Repeat([]byte("ij"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1001), PieceIDs: []types.BigInt{types.NewBigInt(2001)}}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.NewBigInt(202),
		endpoint: "https://secondary-a.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}

	dsID := types.NewBigInt(404)
	replacementClient := &fakePDPProviderClient{
		pullPiecesFn: func(_ context.Context, req pdp.PullRequest) (*pdp.PullResult, error) {
			if req.DataSetID == nil || !req.DataSetID.Equal(dsID) {
				t.Fatalf("pull dataSetID=%v want %s", req.DataSetID, dsID.String())
			}
			if len(req.ExtraData) == 0 {
				t.Fatal("pull extraData should be populated for replacement existing dataset")
			}
			return &pdp.PullResult{
				Status: pdp.PullStatusComplete,
				Pieces: []pdp.PullPieceStatus{{PieceCID: info.CIDv2.String(), Status: pdp.PullStatusComplete}},
			}, nil
		},
		addPiecesFn: func(_ context.Context, gotDataSetID types.BigInt, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.AddPiecesResult, error) {
			if !gotDataSetID.Equal(dsID) {
				t.Fatalf("commit dataSetID=%s want %s", gotDataSetID.String(), dsID.String())
			}
			if len(pieces) != 1 || pieces[0].PieceCID != info.CIDv2 {
				t.Fatalf("unexpected pieces: %+v", pieces)
			}
			if len(extraData) == 0 {
				t.Fatal("commit extraData should be populated for replacement existing dataset")
			}
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x02"), StatusURL: "https://secondary-b.example.com/status"}, nil
		},
		waitForAddedFn: func(_ context.Context, statusURL string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			if statusURL == "" {
				t.Fatal("empty statusURL")
			}
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x02"),
				DataSetID:         dsID,
				PieceCount:        1,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(2002)},
			}, nil
		},
	}
	clientDataSetID := types.NewBigInt(0xFEED)
	replacement, err := NewDataSetContext(testProvider(), replacementClient, mustTestSigner(t), testDataSetRef(dsID, clientDataSetID),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{ClientDataSetID: types.NewBigInt(0xFEED)},
	}
	resolver := &fakeResolver{
		contexts:     []StorageContext{primary, failedSecondary},
		replacements: []StorageContext{replacement},
	}
	mgr := mustNewService(t, Options{Resolver: resolver, FWSSDataSetReader: reader})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if reader.calls != 1 {
		t.Fatalf("reader.calls=%d want 1 (replacement existing dataset should be validated once)", reader.calls)
	}
	ref, ok := replacement.DataSetRef()
	if !ok || !ref.ClientDataSetID().Equal(clientDataSetID) {
		t.Fatalf("replacement DataSetRef()=(%+v, %t)", ref, ok)
	}
	if len(got.Copies) != 2 || !got.Copies[1].ProviderID.Equal(replacement.ProviderID()) {
		t.Fatalf("copies=%+v want replacement provider %s in second slot", got.Copies, replacement.ProviderID())
	}
}

func TestManagerUpload_ReplacementReaderFailureAdvancesToNextProvider(t *testing.T) {
	data := bytes.Repeat([]byte("kl"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1001), PieceIDs: []types.BigInt{types.NewBigInt(2001)}}, nil
		},
	}
	failedSecondary := &fakeUploadContext{
		id:       types.NewBigInt(202),
		endpoint: "https://secondary-a.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, errors.New("pull failed")
		},
	}

	replacementProvider := testProvider()
	replacementProvider.ID = types.NewBigInt(404)
	replacementRef, err := NewDataSetRef(replacementProvider.ID, types.NewBigInt(404), types.NewBigInt(7))
	if err != nil {
		t.Fatalf("NewDataSetRef: %v", err)
	}
	replacementCtx, err := NewDataSetContext(replacementProvider, &fakePDPProviderClient{}, mustTestSigner(t), replacementRef,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	replacement2 := &fakeUploadContext{
		id:       types.NewBigInt(303),
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
			return &CommitResult{DataSetID: types.NewBigInt(1002), PieceIDs: []types.BigInt{types.NewBigInt(2002)}}, nil
		},
	}

	readerErr := errors.New("fwss unavailable")
	reader := &fakeFWSSDataSetReader{infoErr: readerErr}
	resolver := &fakeResolver{
		contexts:     []StorageContext{primary, failedSecondary},
		replacements: []StorageContext{replacementCtx, replacement2},
	}
	mgr := mustNewService(t, Options{
		Resolver:             resolver,
		MaxSecondaryAttempts: 3,
		FWSSDataSetReader:    reader,
	})

	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 2})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got.Copies) != 2 || !got.Copies[1].ProviderID.Equal(replacement2.id) {
		t.Fatalf("copies=%+v want replacement provider %s in second slot", got.Copies, replacement2.id)
	}
	foundReplacementFailure := false
	for _, attempt := range got.FailedAttempts {
		if attempt.ProviderID.Equal(replacementProvider.ID) && errors.Is(attempt.Err, readerErr) {
			foundReplacementFailure = true
			break
		}
	}
	if !foundReplacementFailure {
		t.Fatalf("FailedAttempts=%+v want replacement provider %s reader failure", got.FailedAttempts, replacementProvider.ID)
	}
}

type fakeResolver struct {
	contexts         []StorageContext
	explicit         bool
	replacements     []StorageContext
	replacementCalls int
	captureFn        func(*UploadOptions)
}

func (r *fakeResolver) ResolveUploadContexts(_ context.Context, opts *UploadOptions) ([]StorageContext, bool, error) {
	if r.captureFn != nil {
		r.captureFn(opts)
	}
	return r.contexts, r.explicit, nil
}

func (r *fakeResolver) SelectReplacement(_ context.Context, _ map[string]types.BigInt, _ *UploadOptions) (StorageContext, error) {
	r.replacementCalls++
	if len(r.replacements) == 0 {
		return nil, errors.New("no replacement")
	}
	next := r.replacements[0]
	r.replacements = r.replacements[1:]
	return next, nil
}

type fakeUploadContext struct {
	id              types.BigInt
	endpoint        string
	pieceURL        string
	dataSetID       *types.BigInt
	clientDataSetID *types.BigInt
	dataSetMetadata map[string]string
	storeFn         func(context.Context, io.Reader, *StoreOptions) (*StoreResult, error)
	presignFn       func(context.Context, []PieceInput) ([]byte, error)
	pullFn          func(context.Context, PullRequest) (*PullResult, error)
	commitFn        func(context.Context, CommitRequest) (*CommitResult, error)
	identity        *ContextIdentity
}

func (c *fakeUploadContext) ProviderID() types.BigInt  { return c.id }
func (c *fakeUploadContext) ServiceURL() string        { return c.endpoint }
func (c *fakeUploadContext) PieceURL(_ cid.Cid) string { return c.pieceURL }

func (c *fakeUploadContext) DataSetRef() (DataSetRef, bool) {
	if c.dataSetID == nil {
		return DataSetRef{}, false
	}
	clientDataSetID := types.BigInt{}
	if c.clientDataSetID != nil {
		clientDataSetID = *c.clientDataSetID
	}
	ref, err := NewDataSetRef(c.id, *c.dataSetID, clientDataSetID)
	if err != nil {
		return DataSetRef{}, false
	}
	return ref, true
}

func (c *fakeUploadContext) ContextIdentity() ContextIdentity {
	if c.identity != nil {
		return *c.identity
	}
	return ContextIdentity{Payer: testPayer(), ChainID: types.ChainID(314159), RecordKeeper: testRecordKeeper()}
}

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

func (c *fakeUploadContext) Upload(context.Context, io.Reader, *UploadOptions) (*UploadResult, error) {
	return nil, errors.New("unexpected Upload")
}

func (c *fakeUploadContext) Download(context.Context, cid.Cid) (io.ReadCloser, error) {
	return nil, errors.New("unexpected Download")
}

type readCountingReader struct {
	reads int
}

func (r *readCountingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, io.EOF
}

func TestServiceUploadToContextsValidatesBeforeReading(t *testing.T) {
	valid := &fakeUploadContext{id: types.NewBigInt(1)}
	wrongIdentity := serviceTestIdentity()
	wrongIdentity.ChainID = types.ChainID(1)
	wrong := &fakeUploadContext{id: types.NewBigInt(2), identity: &wrongIdentity}

	tests := map[string][]StorageContext{
		"empty":              nil,
		"typed nil":          {(*fakeUploadContext)(nil)},
		"duplicate provider": {valid, valid},
		"identity mismatch":  {wrong},
	}
	for name, contexts := range tests {
		t.Run(name, func(t *testing.T) {
			reader := &readCountingReader{}
			svc := mustNewService(t, Options{})
			result, err := svc.UploadToContexts(context.Background(), reader, contexts, nil)
			if result != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
			}
			if reader.reads != 0 {
				t.Fatalf("reader reads=%d want 0", reader.reads)
			}
		})
	}
}

func TestServiceUploadToContextsRejectsSelectionOptionsBeforeReading(t *testing.T) {
	withCDN := true
	tests := map[string]*UploadOptions{
		"Copies":             {Copies: 1},
		"ExcludeProviderIDs": {ExcludeProviderIDs: []types.BigInt{types.NewBigInt(2)}},
		"DataSetMetadata":    {DataSetMetadata: map[string]string{"source": "app"}},
		"WithCDN":            {WithCDN: &withCDN},
	}
	for name, opts := range tests {
		t.Run(name, func(t *testing.T) {
			reader := &readCountingReader{}
			svc := mustNewService(t, Options{})
			result, err := svc.UploadToContexts(context.Background(), reader, []StorageContext{&fakeUploadContext{id: types.NewBigInt(1)}}, opts)
			if result != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
			}
			if reader.reads != 0 {
				t.Fatalf("reader reads=%d want 0", reader.reads)
			}
		})
	}
}

func TestServiceUploadValidatesContextIdentityBeforeReading(t *testing.T) {
	identity := serviceTestIdentity()
	identity.RecordKeeper = common.HexToAddress("0x9999")
	storageCtx := &fakeUploadContext{id: types.NewBigInt(1), identity: &identity}
	reader := &readCountingReader{}
	svc := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{storageCtx}}})

	result, err := svc.Upload(context.Background(), reader, &UploadOptions{Copies: 1})
	if result != nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
	}
	if reader.reads != 0 {
		t.Fatalf("reader reads=%d want 0", reader.reads)
	}
}

func TestServiceUploadRejectsInvalidResolverTargetsBeforeReading(t *testing.T) {
	provider1 := &fakeUploadContext{id: types.NewBigInt(1)}
	provider2 := &fakeUploadContext{id: types.NewBigInt(2)}
	tests := map[string]struct {
		contexts []StorageContext
		opts     *UploadOptions
	}{
		"too many contexts": {
			contexts: []StorageContext{provider1, provider2},
			opts:     &UploadOptions{Copies: 1},
		},
		"excluded provider": {
			contexts: []StorageContext{provider1},
			opts: &UploadOptions{
				Copies:             1,
				ExcludeProviderIDs: []types.BigInt{types.NewBigInt(1)},
			},
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			reader := &readCountingReader{}
			svc := mustNewService(t, Options{Resolver: &fakeResolver{contexts: tt.contexts}})
			result, err := svc.Upload(context.Background(), reader, tt.opts)
			if result != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
			}
			if reader.reads != 0 {
				t.Fatalf("reader reads=%d want 0", reader.reads)
			}
		})
	}
}

func TestContextUploadRejectsMultiTargetOptionsBeforeReading(t *testing.T) {
	withCDN := true
	tests := map[string]*UploadOptions{
		"Copies":             {Copies: 1},
		"ExcludeProviderIDs": {ExcludeProviderIDs: []types.BigInt{types.NewBigInt(2)}},
		"DataSetMetadata":    {DataSetMetadata: map[string]string{"source": "app"}},
		"WithCDN":            {WithCDN: &withCDN},
		"OnCopyComplete":     {OnCopyComplete: func(types.BigInt, cid.Cid) {}},
		"OnCopyFailed":       {OnCopyFailed: func(types.BigInt, cid.Cid, error) {}},
		"OnPullProgress":     {OnPullProgress: func(types.BigInt, cid.Cid, PullStatus) {}},
	}
	storageCtx := mustProviderContext(t, &fakePDPProviderClient{})
	for name, opts := range tests {
		t.Run(name, func(t *testing.T) {
			reader := &readCountingReader{}
			result, err := storageCtx.Upload(context.Background(), reader, opts)
			if result != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("result=%v error=%v want ErrInvalidArgument", result, err)
			}
			if reader.reads != 0 {
				t.Fatalf("reader reads=%d want 0", reader.reads)
			}
		})
	}
}

func containsCall(calls []string, want string) bool {
	return slices.Contains(calls, want)
}

// TestManagerUpload_RequestedCopiesIsCallerRequested proves that
// UploadResult.RequestedCopies reflects opts.Copies, not the number of
// contexts the resolver happened to return.
// When fewer contexts are available the result must have Complete=false.
func TestManagerUpload_RequestedCopiesIsCallerRequested(t *testing.T) {
	data := bytes.Repeat([]byte("rc"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}, IsNewDataSet: true}, nil
		},
	}

	// Resolver returns only 1 context even though caller requests 3 copies.
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})

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
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}, IsNewDataSet: true}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.NewBigInt(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		// Returns (nil, nil) — the nil-deref case.
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return nil, nil
		},
	}

	mgr := mustNewService(t, Options{})

	// Should not panic; primary copy should still succeed.
	got, err := mgr.UploadToContexts(context.Background(), bytes.NewReader(data), []StorageContext{primary, secondary}, nil)
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
// error is recorded with CopyStagePresign, not CopyStageCommit, and does not
// trigger OnCopyFailed because no SP-to-SP copy was attempted yet.
func TestManagerUpload_PresignFailureUsesPresignStage(t *testing.T) {
	data := bytes.Repeat([]byte("ps"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}, IsNewDataSet: true}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.NewBigInt(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return nil, errors.New("presign failed: no signer")
		},
	}

	mgr := mustNewService(t, Options{})

	copyFailedCalled := false
	got, err := mgr.UploadToContexts(context.Background(), bytes.NewReader(data), []StorageContext{primary, secondary}, &UploadOptions{
		OnCopyFailed: func(types.BigInt, cid.Cid, error) {
			copyFailedCalled = true
		},
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(got.FailedAttempts) != 1 {
		t.Fatalf("FailedAttempts=%d want 1", len(got.FailedAttempts))
	}
	if got.FailedAttempts[0].Stage != CopyStagePresign {
		t.Fatalf("Stage=%s want %s", got.FailedAttempts[0].Stage, CopyStagePresign)
	}
	if copyFailedCalled {
		t.Fatal("OnCopyFailed should not fire for presign failures")
	}
}

func TestManagerUpload_NilReader(t *testing.T) {
	mgr := mustNewService(t, Options{})
	_, err := mgr.Upload(context.Background(), nil, &UploadOptions{Copies: 1})
	if err == nil {
		t.Fatal("expected error for nil reader")
	}
}

func TestManagerUpload_ReadError(t *testing.T) {
	readErr := errors.New("read boom")
	ctx := &fakeUploadContext{
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			_, err := io.ReadAll(r)
			if err != nil {
				return nil, err
			}
			return nil, errors.New("unexpected: reader should have errored")
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{ctx}}})
	_, err := mgr.Upload(context.Background(), iotest.ErrReader(readErr), &UploadOptions{Copies: 1})
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
		id:       types.NewBigInt(1),
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
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})

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
		id:       types.NewBigInt(1),
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
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})

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
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			if opts != nil {
				gotPC = opts.PieceCID
			}
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})
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
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			if opts != nil && opts.OnProgress != nil {
				cbSeen = true
			}
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})
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
		id:       types.NewBigInt(1),
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
	newSecondary := func(id types.BigInt, name string) *fakeUploadContext {
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
		Resolver:          &fakeResolver{contexts: []StorageContext{primary, newSecondary(types.NewBigInt(2), "secondary-1"), newSecondary(types.NewBigInt(3), "secondary-2")}},
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

func TestNewSetsUploadResolverWithoutInventingContextCapabilities(t *testing.T) {
	r := &fakeResolver{}
	mgr := mustNewService(t, Options{Resolver: r})
	if mgr.resolver != r {
		t.Fatal("New did not set upload resolver")
	}
	if mgr.contextResolver != nil || mgr.contextSelector != nil {
		t.Fatal("New inherited context capabilities that the resolver does not implement")
	}
}

func TestNewRejectsSignerAddressMismatch(t *testing.T) {
	s := mustTestSigner(t)
	_, err := New(Options{
		Signer:        s,
		SignerAddress: common.HexToAddress("0x9999"),
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("New error=%v want ErrInvalidArgument", err)
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

func TestManagerUpload_SourceInjectedIntoMetadata(t *testing.T) {
	data := bytes.Repeat([]byte("src"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}}, nil
		},
	}

	var capturedOpts *UploadOptions
	resolver := &fakeResolver{
		contexts: []StorageContext{primary},
		captureFn: func(opts *UploadOptions) {
			capturedOpts = opts
		},
	}

	mgr := mustNewService(t, Options{Resolver: resolver, Source: "test-app"})
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
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(0), PieceIDs: nil}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})
	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if err == nil {
		t.Fatal("expected CommitError when identifiers missing")
	}
	if _, ok := errors.AsType[*CommitError](err); !ok {
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
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(0), PieceIDs: []types.BigInt{types.NewBigInt(10)}}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})
	_, err = mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
	if err == nil {
		t.Fatal("expected CommitError when dataSetID is zero")
	}
	if _, ok := errors.AsType[*CommitError](err); !ok {
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
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(0)}}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})
	got, err := mgr.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{Copies: 1})
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

// TestManagerUpload_PullStatusNotComplete proves that a non-complete pull
// status with nil error is recorded as a failed pull attempt.
func TestManagerUpload_PullStatusNotComplete(t *testing.T) {
	data := bytes.Repeat([]byte("nc"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(1),
		endpoint: "https://p.example.com",
		storeFn: func(_ context.Context, _ io.Reader, _ *StoreOptions) (*StoreResult, error) {
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{DataSetID: types.NewBigInt(1), PieceIDs: []types.BigInt{types.NewBigInt(10)}}, nil
		},
	}
	secondary := &fakeUploadContext{
		id:       types.NewBigInt(2),
		endpoint: "https://s.example.com",
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x01}, nil
		},
		pullFn: func(_ context.Context, _ PullRequest) (*PullResult, error) {
			return &PullResult{Status: PullStatusFailed}, nil
		},
	}

	mgr := mustNewService(t, Options{})
	got, err := mgr.UploadToContexts(context.Background(), bytes.NewReader(data), []StorageContext{primary, secondary}, nil)
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
