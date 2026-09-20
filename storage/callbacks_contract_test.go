package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

type submittedEvent struct {
	providerID types.BigInt
	txHash     string
	pieces     []SubmittedPiece
}

type confirmedEvent struct {
	providerID types.BigInt
	dataSetID  types.BigInt
	pieces     []ConfirmedPiece
}

type copyEvent struct {
	providerID types.BigInt
	pieceCID   cid.Cid
}

type copyFailureEvent struct {
	providerID types.BigInt
	pieceCID   cid.Cid
	err        error
}

type pullProgressEvent struct {
	providerID types.BigInt
	pieceCID   cid.Cid
	status     PullStatus
}

type recordingSlogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingSlogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingSlogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *recordingSlogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *recordingSlogHandler) WithGroup(string) slog.Handler      { return h }

// errorAttrs returns the attributes of each Error-level record.
func (h *recordingSlogHandler) errorAttrs() []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]string
	for _, record := range h.records {
		if record.Level != slog.LevelError {
			continue
		}
		attrs := make(map[string]string)
		record.Attrs(func(attr slog.Attr) bool {
			attrs[attr.Key] = attr.Value.String()
			return true
		})
		out = append(out, attrs)
	}
	return out
}

type uploadCallbackPanic struct{ callback string }

func recoverPanic(fn func()) (recovered any) {
	defer func() { recovered = recover() }()
	fn()
	return nil
}

// assertCallbackPanicLogged checks that the original panic stack, which
// includes the test's callback frame, was logged once.
func assertCallbackPanicLogged(t *testing.T, logs *recordingSlogHandler, callback, testName string) {
	t.Helper()
	records := logs.errorAttrs()
	if len(records) != 1 {
		t.Fatalf("error logs=%v, want one callback panic record", records)
	}
	if records[0]["callback"] != callback || !strings.Contains(records[0]["stack"], testName) {
		t.Fatalf("callback panic log=%v, want callback %s and a stack through %s", records[0], callback, testName)
	}
}

func formatSubmittedEvent(e submittedEvent) string {
	return fmt.Sprintf("{provider=%s txHash=%s pieces=%s}", e.providerID, e.txHash, formatSubmittedPieces(e.pieces))
}

func formatSubmittedEvents(events []submittedEvent) string {
	return formatEvents(events, formatSubmittedEvent)
}

func formatConfirmedEvent(e confirmedEvent) string {
	return fmt.Sprintf("{provider=%s dataSet=%s pieces=%s}", e.providerID, e.dataSetID, formatConfirmedPieces(e.pieces))
}

func formatConfirmedEvents(events []confirmedEvent) string {
	return formatEvents(events, formatConfirmedEvent)
}

func formatCopyEvent(e copyEvent) string {
	return fmt.Sprintf("{provider=%s pieceCID=%s}", e.providerID, e.pieceCID)
}

func formatCopyEvents(events []copyEvent) string {
	return formatEvents(events, formatCopyEvent)
}

func formatPullProgressEvent(e pullProgressEvent) string {
	return fmt.Sprintf("{provider=%s pieceCID=%s status=%s}", e.providerID, e.pieceCID, e.status)
}

func formatPullProgressEvents(events []pullProgressEvent) string {
	return formatEvents(events, formatPullProgressEvent)
}

func formatSubmittedPieces(pieces []SubmittedPiece) string {
	return formatEvents(pieces, func(p SubmittedPiece) string {
		return fmt.Sprintf("{pieceCID=%s}", p.PieceCID)
	})
}

func formatConfirmedPieces(pieces []ConfirmedPiece) string {
	return formatEvents(pieces, func(p ConfirmedPiece) string {
		return fmt.Sprintf("{pieceID=%s pieceCID=%s}", p.PieceID, p.PieceCID)
	})
}

func formatEvents[T any](events []T, formatOne func(T) string) string {
	parts := make([]string, len(events))
	for i, event := range events {
		parts[i] = formatOne(event)
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func submittedPiecesEqual(got, want []SubmittedPiece) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i].PieceCID != want[i].PieceCID {
			return false
		}
	}
	return true
}

func confirmedPiecesEqual(got, want []ConfirmedPiece) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if !got[i].PieceID.Equal(want[i].PieceID) || got[i].PieceCID != want[i].PieceCID {
			return false
		}
	}
	return true
}

func hasSubmittedEvent(events []submittedEvent, want submittedEvent) bool {
	for _, got := range events {
		if got.providerID.Equal(want.providerID) && got.txHash == want.txHash && submittedPiecesEqual(got.pieces, want.pieces) {
			return true
		}
	}
	return false
}

func hasConfirmedEvent(events []confirmedEvent, want confirmedEvent) bool {
	for _, got := range events {
		if got.providerID.Equal(want.providerID) && got.dataSetID.Equal(want.dataSetID) && confirmedPiecesEqual(got.pieces, want.pieces) {
			return true
		}
	}
	return false
}

func hasCopyEvent(events []copyEvent, want copyEvent) bool {
	for _, got := range events {
		if got.providerID.Equal(want.providerID) && got.pieceCID == want.pieceCID {
			return true
		}
	}
	return false
}

func hasPullProgressEvent(events []pullProgressEvent, want pullProgressEvent) bool {
	for _, got := range events {
		if got.providerID.Equal(want.providerID) && got.pieceCID == want.pieceCID && got.status == want.status {
			return true
		}
	}
	return false
}

// callbackPanicUploadFixture returns a two-copy upload that invokes every
// UploadOptions callback: the first secondary fails its pull and a replacement
// succeeds.
func callbackPanicUploadFixture(t *testing.T) ([]byte, *fakeResolver) {
	t.Helper()
	data := bytes.Repeat([]byte("mp"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, r io.Reader, opts *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			if opts.OnProgress != nil {
				opts.OnProgress(1)
				opts.OnProgress(2)
			}
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, req CommitRequest) (*CommitResult, error) {
			if req.OnSubmitted != nil {
				req.OnSubmitted("0xprimary")
			}
			return &CommitResult{
				TransactionID: "0xprimary",
				DataSet:       testCommitDataSetRef(101, 1001),
				PieceIDs:      []types.BigInt{types.NewBigInt(2001)},
				IsNewDataSet:  true,
			}, nil
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
		pieceURL: "https://secondary-b.example.com/piece/" + info.CIDv2.String(),
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x02}, nil
		},
		pullFn: func(_ context.Context, req PullRequest) (*PullResult, error) {
			if req.OnProgress != nil {
				req.OnProgress(info.CIDv2, PullStatusInProgress)
				req.OnProgress(info.CIDv2, PullStatusComplete)
			}
			return &PullResult{
				Status: PullStatusComplete,
				Pieces: []PullPieceResult{{PieceCID: info.CIDv2, Status: PullStatusComplete}},
			}, nil
		},
		commitFn: func(_ context.Context, req CommitRequest) (*CommitResult, error) {
			if req.OnSubmitted != nil {
				req.OnSubmitted("0xreplacement")
			}
			return &CommitResult{
				TransactionID: "0xreplacement",
				DataSet:       testCommitDataSetRef(303, 1002),
				PieceIDs:      []types.BigInt{types.NewBigInt(2002)},
				IsNewDataSet:  true,
			}, nil
		},
	}
	return data, &fakeResolver{
		contexts:     []StorageContext{primary, failedSecondary},
		replacements: []StorageContext{replacement},
	}
}

func TestManagerUpload_CallbackPanicReachesCaller(t *testing.T) {
	for _, name := range []string{
		"OnProgress",
		"OnStored",
		"OnPullProgress",
		"OnCopyComplete",
		"OnCopyFailed",
		"OnPiecesAdded",
		"OnPiecesConfirmed",
	} {
		t.Run(name, func(t *testing.T) {
			data, resolver := callbackPanicUploadFixture(t)
			logs := &recordingSlogHandler{}
			mgr := mustNewService(t, Options{Resolver: resolver, Logger: slog.New(logs)})
			sentinel := &uploadCallbackPanic{callback: name}
			var (
				panicked         atomic.Bool
				mu               sync.Mutex
				calledAfterPanic []string
			)
			hit := func(callback string) {
				if callback == name {
					panicked.Store(true)
					panic(sentinel)
				}
				if panicked.Load() {
					mu.Lock()
					calledAfterPanic = append(calledAfterPanic, callback)
					mu.Unlock()
				}
			}
			opts := &UploadOptions{
				Copies:            2,
				OnProgress:        func(int64) { hit("OnProgress") },
				OnStored:          func(types.BigInt, cid.Cid) { hit("OnStored") },
				OnPullProgress:    func(types.BigInt, cid.Cid, PullStatus) { hit("OnPullProgress") },
				OnCopyComplete:    func(types.BigInt, cid.Cid) { hit("OnCopyComplete") },
				OnCopyFailed:      func(types.BigInt, cid.Cid, error) { hit("OnCopyFailed") },
				OnPiecesAdded:     func(string, types.BigInt, []SubmittedPiece) { hit("OnPiecesAdded") },
				OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) { hit("OnPiecesConfirmed") },
			}

			recovered := recoverPanic(func() {
				_, _ = mgr.Upload(context.Background(), bytes.NewReader(data), opts)
			})
			if recovered != sentinel {
				t.Fatalf("recovered %v, want the %s panic value", recovered, name)
			}
			// Concurrent commits can each start OnPiecesAdded before either
			// panic is recorded; every other callback runs sequentially.
			if name != "OnPiecesAdded" && len(calledAfterPanic) != 0 {
				t.Fatalf("callbacks invoked after the panic: %v", calledAfterPanic)
			}
			assertCallbackPanicLogged(t, logs, name, "TestManagerUpload_CallbackPanicReachesCaller")
		})
	}
}

func TestServiceUpload_BatchedCallbackPanicReachesCaller(t *testing.T) {
	identity := serviceTestIdentity()
	batcher := mustUploadBatcher(t, identity, mustTestSigner(t), WithUploadIdleWait(0))
	target := batchTestTarget(identity, testCommitDataSetRef(1, 11))
	target.storeFn = func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		info, err := piece.CalculateFromBytes(data)
		if err != nil {
			return nil, err
		}
		return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
	}
	_ = captureBatchTestSubmissions(target)
	service := mustNewService(t, Options{UploadBatcher: batcher})

	sentinel := &uploadCallbackPanic{callback: "OnPiecesAdded"}
	recovered := recoverPanic(func() {
		_, _ = service.UploadToContexts(context.Background(), bytes.NewReader(bytes.Repeat([]byte("panic"), 128)), []StorageContext{target}, &UploadToContextsOptions{
			OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) { panic(sentinel) },
		})
	})
	if recovered != sentinel {
		t.Fatalf("recovered %v, want the OnPiecesAdded panic value", recovered)
	}
	if err := service.Flush(context.Background()); err != nil {
		t.Fatalf("Flush after callback panic: %v", err)
	}
	result, err := service.UploadToContexts(context.Background(), bytes.NewReader(bytes.Repeat([]byte("after"), 128)), []StorageContext{target}, nil)
	if err != nil {
		t.Fatalf("UploadToContexts after callback panic: %v", err)
	}
	if len(result.Copies) != 1 {
		t.Fatalf("result=%+v, want one copy", result)
	}
	assertNoUploadBatchTransfers(t, batcher)
}

func TestContextUpload_CallbackPanicReachesCaller(t *testing.T) {
	for _, name := range []string{"OnStored", "OnPiecesAdded"} {
		t.Run(name, func(t *testing.T) {
			data := bytes.Repeat([]byte("cp"), 128)
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
					return &pdp.CreateDataSetResult{
						TxHash:    common.HexToHash("0xabc"),
						StatusURL: "https://sp.example.com/status",
					}, nil
				},
				waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
					return &pdp.AddPiecesStatus{
						TxHash:            common.HexToHash("0xabc"),
						DataSetID:         types.NewBigInt(55),
						PiecesAdded:       true,
						ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(77)},
					}, nil
				},
			}
			logs := &recordingSlogHandler{}
			ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t),
				WithPayer(testPayer()),
				WithRecordKeeper(testRecordKeeper()),
				WithChainID(types.ChainID(314159)),
				WithLogger(slog.New(logs)),
			)
			if err != nil {
				t.Fatalf("NewContext: %v", err)
			}
			sentinel := &uploadCallbackPanic{callback: name}
			var confirmed atomic.Bool
			opts := &ContextUploadOptions{
				OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) { confirmed.Store(true) },
			}
			switch name {
			case "OnStored":
				opts.OnStored = func(types.BigInt, cid.Cid) { panic(sentinel) }
			case "OnPiecesAdded":
				opts.OnPiecesAdded = func(string, types.BigInt, []SubmittedPiece) { panic(sentinel) }
			}

			recovered := recoverPanic(func() {
				_, _ = ctx.Upload(context.Background(), bytes.NewReader(data), opts)
			})
			if recovered != sentinel {
				t.Fatalf("recovered %v, want the %s panic value", recovered, name)
			}
			if confirmed.Load() {
				t.Fatal("OnPiecesConfirmed invoked after an earlier callback panicked")
			}
			assertCallbackPanicLogged(t, logs, name, "TestContextUpload_CallbackPanicReachesCaller")
		})
	}
}

func TestContextStore_LowLevelProgressPanicPropagates(t *testing.T) {
	data := bytes.Repeat([]byte("lp"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	fake := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, opts pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			_, _ = io.Copy(io.Discard, r)
			if opts.OnProgress != nil {
				opts.OnProgress(1)
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(_ context.Context, _ cid.Cid, _ time.Duration) error { return nil },
	}
	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("StoreOptions.OnProgress panic did not propagate")
		}
	}()
	_, _ = ctx.Store(context.Background(), bytes.NewReader(data), &StoreOptions{
		OnProgress: func(int64) {
			panic("low-level progress panic")
		},
	})
}

// TestContextUpload_Callbacks exercises Context.Upload with all lifecycle
// callbacks set and verifies that a successful upload triggers the expected
// callback invocations throughout the upload lifecycle.
func TestContextUpload_Callbacks(t *testing.T) {
	data := bytes.Repeat([]byte("cb"), 128)
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
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xabc"),
				StatusURL: "https://sp.example.com/status",
			}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xabc"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(77)},
			}, nil
		},
	}

	provider := testProvider()
	ctx, err := NewProviderContext(provider, fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var (
		storedProviderID        types.BigInt
		storedPieceCID          cid.Cid
		piecesAddedTxHash       string
		piecesAddedProviderID   types.BigInt
		piecesAddedPieces       []SubmittedPiece
		piecesConfirmedDSID     types.BigInt
		piecesConfirmedProvider types.BigInt
		piecesConfirmedPieces   []ConfirmedPiece
	)

	opts := &ContextUploadOptions{
		OnStored: func(providerID types.BigInt, pieceCID cid.Cid) {
			storedProviderID = providerID
			storedPieceCID = pieceCID
		},
		OnPiecesAdded: func(txHash string, providerID types.BigInt, pieces []SubmittedPiece) {
			piecesAddedTxHash = txHash
			piecesAddedProviderID = providerID
			piecesAddedPieces = append([]SubmittedPiece(nil), pieces...)
		},
		OnPiecesConfirmed: func(dataSetID, providerID types.BigInt, pieces []ConfirmedPiece) {
			piecesConfirmedDSID = dataSetID
			piecesConfirmedProvider = providerID
			piecesConfirmedPieces = append([]ConfirmedPiece(nil), pieces...)
		},
	}

	result, err := ctx.Upload(context.Background(), bytes.NewReader(data), opts)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if result.SuccessCount() != 1 {
		t.Fatalf("SuccessCount=%d, want 1", result.SuccessCount())
	}

	if !storedProviderID.Equal(provider.ID) {
		t.Errorf("OnStored providerID=%s, want %s", storedProviderID.String(), provider.ID.String())
	}
	if storedPieceCID != info.CIDv2 {
		t.Errorf("OnStored pieceCID=%s, want %s", storedPieceCID, info.CIDv2)
	}
	// TxHash.Hex() zero-pads to the full 32-byte (64-char) form.
	wantTxHash := common.HexToHash("0xabc").Hex()
	if piecesAddedTxHash != wantTxHash {
		t.Errorf("OnPiecesAdded txHash=%q, want %q", piecesAddedTxHash, wantTxHash)
	}
	if !piecesAddedProviderID.Equal(provider.ID) {
		t.Errorf("OnPiecesAdded providerID=%s, want %s", piecesAddedProviderID.String(), provider.ID.String())
	}
	wantSubmittedPieces := []SubmittedPiece{{PieceCID: info.CIDv2}}
	if !submittedPiecesEqual(piecesAddedPieces, wantSubmittedPieces) {
		t.Errorf("OnPiecesAdded pieces=%s, want %s", formatSubmittedPieces(piecesAddedPieces), formatSubmittedPieces(wantSubmittedPieces))
	}
	if !piecesConfirmedDSID.Equal(types.NewBigInt(55)) {
		t.Errorf("OnPiecesConfirmed dataSetID=%s, want 55", piecesConfirmedDSID.String())
	}
	if !piecesConfirmedProvider.Equal(provider.ID) {
		t.Errorf("OnPiecesConfirmed providerID=%s, want %s", piecesConfirmedProvider.String(), provider.ID.String())
	}
	wantConfirmedPieces := []ConfirmedPiece{{
		PieceID:  types.NewBigInt(77),
		PieceCID: info.CIDv2,
	}}
	if !confirmedPiecesEqual(piecesConfirmedPieces, wantConfirmedPieces) {
		t.Errorf("OnPiecesConfirmed pieces=%s, want %s", formatConfirmedPieces(piecesConfirmedPieces), formatConfirmedPieces(wantConfirmedPieces))
	}
}

func TestContextUpload_CallbacksAllowZeroPieceID(t *testing.T) {
	data := bytes.Repeat([]byte("cz"), 128)
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
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0xabc"),
				StatusURL: "https://sp.example.com/status",
			}, nil
		},
		waitForCreateAndAddFn: func(_ context.Context, _ string, _ time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0xabc"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(0)},
			}, nil
		},
	}

	ctx, err := NewProviderContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var confirmed []ConfirmedPiece
	opts := &ContextUploadOptions{
		OnPiecesConfirmed: func(_, _ types.BigInt, pieces []ConfirmedPiece) {
			confirmed = append([]ConfirmedPiece(nil), pieces...)
		},
	}

	if _, err := ctx.Upload(context.Background(), bytes.NewReader(data), opts); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	wantConfirmedPieces := []ConfirmedPiece{{
		PieceID:  types.NewBigInt(0),
		PieceCID: info.CIDv2,
	}}
	if !confirmedPiecesEqual(confirmed, wantConfirmedPieces) {
		t.Fatalf("OnPiecesConfirmed pieces=%s, want %s", formatConfirmedPieces(confirmed), formatConfirmedPieces(wantConfirmedPieces))
	}
}

// TestManagerUpload_CallbacksAcrossPrimaryAndReplacement exercises Service.Upload
// through a primary + failed-secondary + replacement scenario and verifies that
// all six UploadOptions callbacks fire with the correct payloads.
func TestManagerUpload_CallbacksAcrossPrimaryAndReplacement(t *testing.T) {
	data := bytes.Repeat([]byte("mg"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, req CommitRequest) (*CommitResult, error) {
			if req.OnSubmitted != nil {
				req.OnSubmitted("0xprimary")
			}
			return &CommitResult{
				TransactionID: "0xprimary",
				DataSet:       testCommitDataSetRef(101, 1001),
				PieceIDs:      []types.BigInt{types.NewBigInt(2001)},
				IsNewDataSet:  true,
			}, nil
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
		pieceURL: "https://secondary-b.example.com/piece/" + info.CIDv2.String(),
		presignFn: func(_ context.Context, _ []PieceInput) ([]byte, error) {
			return []byte{0x02}, nil
		},
		pullFn: func(_ context.Context, req PullRequest) (*PullResult, error) {
			if req.OnProgress != nil {
				req.OnProgress(info.CIDv2, PullStatusComplete)
			}
			return &PullResult{
				Status: PullStatusComplete,
				Pieces: []PullPieceResult{{PieceCID: info.CIDv2, Status: PullStatusComplete}},
			}, nil
		},
		commitFn: func(_ context.Context, req CommitRequest) (*CommitResult, error) {
			if req.OnSubmitted != nil {
				req.OnSubmitted("0xreplacement")
			}
			return &CommitResult{
				TransactionID: "0xreplacement",
				DataSet:       testCommitDataSetRef(303, 1002),
				PieceIDs:      []types.BigInt{types.NewBigInt(2002)},
			}, nil
		},
	}

	resolver := &fakeResolver{
		contexts:     []StorageContext{primary, failedSecondary},
		replacements: []StorageContext{replacement},
	}
	mgr := mustNewService(t, Options{Resolver: resolver})

	var (
		mu                 sync.Mutex
		storedEvents       []copyEvent
		piecesAddedEvents  []submittedEvent
		piecesConfirmedEvt []confirmedEvent
		copyCompleteEvents []copyEvent
		copyFailedEvents   []copyFailureEvent
		pullProgressEvents []pullProgressEvent
	)

	opts := &UploadOptions{
		Copies: 2,
		OnStored: func(providerID types.BigInt, pieceCID cid.Cid) {
			mu.Lock()
			defer mu.Unlock()
			storedEvents = append(storedEvents, copyEvent{providerID: providerID, pieceCID: pieceCID})
		},
		OnPiecesAdded: func(txHash string, providerID types.BigInt, pieces []SubmittedPiece) {
			mu.Lock()
			defer mu.Unlock()
			piecesAddedEvents = append(piecesAddedEvents, submittedEvent{
				providerID: providerID,
				txHash:     txHash,
				pieces:     append([]SubmittedPiece(nil), pieces...),
			})
		},
		OnPiecesConfirmed: func(dataSetID, providerID types.BigInt, pieces []ConfirmedPiece) {
			mu.Lock()
			defer mu.Unlock()
			piecesConfirmedEvt = append(piecesConfirmedEvt, confirmedEvent{
				providerID: providerID,
				dataSetID:  dataSetID,
				pieces:     append([]ConfirmedPiece(nil), pieces...),
			})
		},
		OnCopyComplete: func(providerID types.BigInt, pieceCID cid.Cid) {
			mu.Lock()
			defer mu.Unlock()
			copyCompleteEvents = append(copyCompleteEvents, copyEvent{providerID: providerID, pieceCID: pieceCID})
		},
		OnCopyFailed: func(providerID types.BigInt, pieceCID cid.Cid, err error) {
			mu.Lock()
			defer mu.Unlock()
			copyFailedEvents = append(copyFailedEvents, copyFailureEvent{providerID: providerID, pieceCID: pieceCID, err: err})
		},
		OnPullProgress: func(providerID types.BigInt, pieceCID cid.Cid, status PullStatus) {
			mu.Lock()
			defer mu.Unlock()
			pullProgressEvents = append(pullProgressEvents, pullProgressEvent{
				providerID: providerID,
				pieceCID:   pieceCID,
				status:     status,
			})
		},
	}

	result, err := mgr.Upload(context.Background(), bytes.NewReader(data), opts)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !result.Complete {
		t.Fatalf("Complete=false, want true")
	}

	mu.Lock()
	defer mu.Unlock()

	wantStored := copyEvent{providerID: primary.id, pieceCID: info.CIDv2}
	if len(storedEvents) != 1 || !hasCopyEvent(storedEvents, wantStored) {
		t.Errorf("OnStored: got %s, want %s", formatCopyEvents(storedEvents), formatCopyEvents([]copyEvent{wantStored}))
	}

	wantSubmitted := []submittedEvent{
		{providerID: primary.id, txHash: "0xprimary", pieces: []SubmittedPiece{{PieceCID: info.CIDv2}}},
		{providerID: replacement.id, txHash: "0xreplacement", pieces: []SubmittedPiece{{PieceCID: info.CIDv2}}},
	}
	if len(piecesAddedEvents) != len(wantSubmitted) {
		t.Errorf("OnPiecesAdded: got %d events, want %d", len(piecesAddedEvents), len(wantSubmitted))
	}
	for _, want := range wantSubmitted {
		if !hasSubmittedEvent(piecesAddedEvents, want) {
			t.Errorf("OnPiecesAdded missing event %s in %s", formatSubmittedEvent(want), formatSubmittedEvents(piecesAddedEvents))
		}
	}

	wantConfirmed := []confirmedEvent{
		{providerID: primary.id, dataSetID: types.NewBigInt(1001), pieces: []ConfirmedPiece{{PieceID: types.NewBigInt(2001), PieceCID: info.CIDv2}}},
		{providerID: replacement.id, dataSetID: types.NewBigInt(1002), pieces: []ConfirmedPiece{{PieceID: types.NewBigInt(2002), PieceCID: info.CIDv2}}},
	}
	if len(piecesConfirmedEvt) != len(wantConfirmed) {
		t.Errorf("OnPiecesConfirmed: got %d events, want %d", len(piecesConfirmedEvt), len(wantConfirmed))
	}
	for _, want := range wantConfirmed {
		if !hasConfirmedEvent(piecesConfirmedEvt, want) {
			t.Errorf("OnPiecesConfirmed missing event %s in %s", formatConfirmedEvent(want), formatConfirmedEvents(piecesConfirmedEvt))
		}
	}

	// OnCopyComplete fires when a secondary's SP-to-SP pull completes, not on
	// commit. In this scenario the replacement's pull succeeds; the primary has
	// no pull step and the failedSecondary's pull fails before OnCopyComplete fires.
	wantCopyComplete := copyEvent{providerID: replacement.id, pieceCID: info.CIDv2}
	if len(copyCompleteEvents) != 1 || !hasCopyEvent(copyCompleteEvents, wantCopyComplete) {
		t.Errorf("OnCopyComplete: got %s, want %s", formatCopyEvents(copyCompleteEvents), formatCopyEvents([]copyEvent{wantCopyComplete}))
	}

	if len(copyFailedEvents) != 1 {
		t.Errorf("OnCopyFailed: got %d events, want 1", len(copyFailedEvents))
	} else {
		got := copyFailedEvents[0]
		if !got.providerID.Equal(failedSecondary.id) || got.pieceCID != info.CIDv2 || got.err == nil {
			t.Errorf("OnCopyFailed: got {provider=%s pieceCID=%s err=%v}, want {provider=%s pieceCID=%s err!=nil}", got.providerID, got.pieceCID, got.err, failedSecondary.id, info.CIDv2)
		}
	}

	wantPullProgress := pullProgressEvent{
		providerID: replacement.id,
		pieceCID:   info.CIDv2,
		status:     PullStatusComplete,
	}
	if !hasPullProgressEvent(pullProgressEvents, wantPullProgress) {
		t.Errorf("OnPullProgress missing %s in %s", formatPullProgressEvent(wantPullProgress), formatPullProgressEvents(pullProgressEvents))
	}
}

func TestManagerUpload_CallbacksAllowZeroPieceID(t *testing.T) {
	data := bytes.Repeat([]byte("mz"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.NewBigInt(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{
				TransactionID: "0xprimary",
				DataSet:       testCommitDataSetRef(101, 1001),
				PieceIDs:      []types.BigInt{types.NewBigInt(0)},
				IsNewDataSet:  true,
			}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []StorageContext{primary}}})

	var confirmed []confirmedEvent
	opts := &UploadOptions{
		Copies: 1,
		OnPiecesConfirmed: func(dataSetID, providerID types.BigInt, pieces []ConfirmedPiece) {
			confirmed = append(confirmed, confirmedEvent{
				providerID: providerID,
				dataSetID:  dataSetID,
				pieces:     append([]ConfirmedPiece(nil), pieces...),
			})
		},
	}

	if _, err := mgr.Upload(context.Background(), bytes.NewReader(data), opts); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	want := []confirmedEvent{{
		providerID: primary.id,
		dataSetID:  types.NewBigInt(1001),
		pieces:     []ConfirmedPiece{{PieceID: types.NewBigInt(0), PieceCID: info.CIDv2}},
	}}
	if len(confirmed) != 1 || !hasConfirmedEvent(confirmed, want[0]) {
		t.Fatalf("OnPiecesConfirmed=%s, want %s", formatConfirmedEvents(confirmed), formatConfirmedEvents(want))
	}
}

// Compile-time contract check: PullRequest.OnProgress and
// CommitRequest.OnSubmitted must exist with the expected signatures.
// Keep these low-level hooks pinned in the public surface even if future test
// refactors stop exercising them through the higher-level upload flows.
var (
	_ = PullRequest{OnProgress: func(cid.Cid, PullStatus) {}}
	_ = CommitRequest{OnSubmitted: func(string) {}}
)
