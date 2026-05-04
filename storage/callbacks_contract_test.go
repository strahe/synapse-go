package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"sync"
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

func (h *recordingSlogHandler) warningCallbacks(t *testing.T) map[string]int {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string]int)
	for _, record := range h.records {
		if record.Level != slog.LevelWarn || record.Message != "storage upload callback panic ignored" {
			continue
		}
		var callback string
		var panicValue string
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "callback" {
				callback = attr.Value.String()
			}
			if attr.Key == "panic" {
				panicValue = attr.Value.String()
			}
			return true
		})
		if callback == "" {
			t.Fatalf("warning missing callback attr: %+v", record)
		}
		if panicValue == "" {
			t.Fatalf("warning missing panic attr: %+v", record)
		}
		out[callback]++
	}
	return out
}

func assertWarnedOnce(t *testing.T, got map[string]int, callbacks ...string) {
	t.Helper()
	for _, callback := range callbacks {
		if got[callback] != 1 {
			t.Fatalf("warning count for %s = %d, want 1; all warnings=%v", callback, got[callback], got)
		}
		delete(got, callback)
	}
	if len(got) != 0 {
		t.Fatalf("unexpected callback warnings: %v", got)
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

func TestManagerUpload_CallbackPanicsAreRecoveredAndWarnOnce(t *testing.T) {
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
				DataSetID:     types.NewBigInt(1001),
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
				DataSetID:     types.NewBigInt(1002),
				PieceIDs:      []types.BigInt{types.NewBigInt(2002)},
				IsNewDataSet:  true,
			}, nil
		},
	}

	handler := &recordingSlogHandler{}
	mgr := mustNewService(t, Options{
		Resolver: &fakeResolver{
			contexts:     []UploadContext{primary, failedSecondary},
			replacements: []UploadContext{replacement},
		},
		Logger: slog.New(handler),
	})
	panicCallback := func(string) {
		panic("callback failed")
	}
	opts := &UploadOptions{
		Copies: 2,
		OnProgress: func(int64) {
			panicCallback("OnProgress")
		},
		OnStored: func(types.BigInt, cid.Cid) {
			panicCallback("OnStored")
		},
		OnPullProgress: func(types.BigInt, cid.Cid, PullStatus) {
			panicCallback("OnPullProgress")
		},
		OnCopyComplete: func(types.BigInt, cid.Cid) {
			panicCallback("OnCopyComplete")
		},
		OnCopyFailed: func(types.BigInt, cid.Cid, error) {
			panicCallback("OnCopyFailed")
		},
		OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) {
			panicCallback("OnPiecesAdded")
		},
		OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) {
			panicCallback("OnPiecesConfirmed")
		},
	}
	originalOnStored := opts.OnStored

	result, err := mgr.Upload(context.Background(), bytes.NewReader(data), opts)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !result.Complete || result.SuccessCount() != 2 {
		t.Fatalf("upload result complete=%v successCount=%d, want complete with 2 copies", result.Complete, result.SuccessCount())
	}
	if reflect.ValueOf(opts.OnStored).Pointer() != reflect.ValueOf(originalOnStored).Pointer() {
		t.Fatal("Upload mutated caller UploadOptions callback")
	}

	assertWarnedOnce(t, handler.warningCallbacks(t),
		"OnProgress",
		"OnStored",
		"OnPullProgress",
		"OnCopyComplete",
		"OnCopyFailed",
		"OnPiecesAdded",
		"OnPiecesConfirmed",
	)
}

func TestContextUpload_CallbackPanicsAreRecoveredWithNilLogger(t *testing.T) {
	data := bytes.Repeat([]byte("cp"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	fake := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, opts pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			_, _ = io.Copy(io.Discard, r)
			if opts.OnProgress != nil {
				opts.OnProgress(1)
				opts.OnProgress(2)
			}
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
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	panicCallback := func() {
		panic("callback failed")
	}

	result, err := ctx.Upload(context.Background(), bytes.NewReader(data), &UploadOptions{
		OnProgress: func(int64) {
			panicCallback()
		},
		OnStored: func(types.BigInt, cid.Cid) {
			panicCallback()
		},
		OnPiecesAdded: func(string, types.BigInt, []SubmittedPiece) {
			panicCallback()
		},
		OnPiecesConfirmed: func(types.BigInt, types.BigInt, []ConfirmedPiece) {
			panicCallback()
		},
	})
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if result.SuccessCount() != 1 {
		t.Fatalf("SuccessCount=%d, want 1", result.SuccessCount())
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
	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t))
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
	ctx, err := NewContext(provider, fake, mustTestSigner(t),
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

	opts := &UploadOptions{
		OnStored: func(providerID types.BigInt, pieceCID cid.Cid) {
			storedProviderID = providerID
			storedPieceCID = pieceCID
		},
		OnPiecesAdded: func(txHash string, providerID types.BigInt, pieces []SubmittedPiece) {
			piecesAddedTxHash = txHash
			piecesAddedProviderID = providerID
			piecesAddedPieces = append([]SubmittedPiece(nil), pieces...)
		},
		OnPiecesConfirmed: func(dataSetID types.BigInt, providerID types.BigInt, pieces []ConfirmedPiece) {
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

	ctx, err := NewContext(testProvider(), fake, mustTestSigner(t),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	var confirmed []ConfirmedPiece
	opts := &UploadOptions{
		OnPiecesConfirmed: func(_ types.BigInt, _ types.BigInt, pieces []ConfirmedPiece) {
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
				DataSetID:     types.NewBigInt(1001),
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
				DataSetID:     types.NewBigInt(1002),
				PieceIDs:      []types.BigInt{types.NewBigInt(2002)},
			}, nil
		},
	}

	resolver := &fakeResolver{
		contexts:     []UploadContext{primary, failedSecondary},
		replacements: []UploadContext{replacement},
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
		OnPiecesConfirmed: func(dataSetID types.BigInt, providerID types.BigInt, pieces []ConfirmedPiece) {
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
				DataSetID:     types.NewBigInt(1001),
				PieceIDs:      []types.BigInt{types.NewBigInt(0)},
				IsNewDataSet:  true,
			}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary}}})

	var confirmed []confirmedEvent
	opts := &UploadOptions{
		Copies: 1,
		OnPiecesConfirmed: func(dataSetID types.BigInt, providerID types.BigInt, pieces []ConfirmedPiece) {
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
