package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/big"
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
	providerID types.ProviderID
	txHash     string
	pieces     []SubmittedPiece
}

type confirmedEvent struct {
	providerID types.ProviderID
	dataSetID  types.DataSetID
	pieces     []ConfirmedPiece
}

type copyEvent struct {
	providerID types.ProviderID
	pieceCID   cid.Cid
}

type copyFailureEvent struct {
	providerID types.ProviderID
	pieceCID   cid.Cid
	err        error
}

type pullProgressEvent struct {
	providerID types.ProviderID
	pieceCID   cid.Cid
	status     PullStatus
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
		if got[i].PieceID != want[i].PieceID || got[i].PieceCID != want[i].PieceCID {
			return false
		}
	}
	return true
}

func hasSubmittedEvent(events []submittedEvent, want submittedEvent) bool {
	for _, got := range events {
		if got.providerID == want.providerID && got.txHash == want.txHash && submittedPiecesEqual(got.pieces, want.pieces) {
			return true
		}
	}
	return false
}

func hasConfirmedEvent(events []confirmedEvent, want confirmedEvent) bool {
	for _, got := range events {
		if got.providerID == want.providerID && got.dataSetID == want.dataSetID && confirmedPiecesEqual(got.pieces, want.pieces) {
			return true
		}
	}
	return false
}

func hasCopyEvent(events []copyEvent, want copyEvent) bool {
	for _, got := range events {
		if got == want {
			return true
		}
	}
	return false
}

func hasPullProgressEvent(events []pullProgressEvent, want pullProgressEvent) bool {
	for _, got := range events {
		if got == want {
			return true
		}
	}
	return false
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
				DataSetID:         55,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(77)},
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
		storedProviderID        types.ProviderID
		storedPieceCID          cid.Cid
		piecesAddedTxHash       string
		piecesAddedProviderID   types.ProviderID
		piecesAddedPieces       []SubmittedPiece
		piecesConfirmedDSID     types.DataSetID
		piecesConfirmedProvider types.ProviderID
		piecesConfirmedPieces   []ConfirmedPiece
	)

	opts := &UploadOptions{
		OnStored: func(providerID types.ProviderID, pieceCID cid.Cid) {
			storedProviderID = providerID
			storedPieceCID = pieceCID
		},
		OnPiecesAdded: func(txHash string, providerID types.ProviderID, pieces []SubmittedPiece) {
			piecesAddedTxHash = txHash
			piecesAddedProviderID = providerID
			piecesAddedPieces = append([]SubmittedPiece(nil), pieces...)
		},
		OnPiecesConfirmed: func(dataSetID types.DataSetID, providerID types.ProviderID, pieces []ConfirmedPiece) {
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

	if storedProviderID != provider.ID {
		t.Errorf("OnStored providerID=%d, want %d", storedProviderID, provider.ID)
	}
	if storedPieceCID != info.CIDv2 {
		t.Errorf("OnStored pieceCID=%s, want %s", storedPieceCID, info.CIDv2)
	}
	// TxHash.Hex() zero-pads to the full 32-byte (64-char) form.
	wantTxHash := common.HexToHash("0xabc").Hex()
	if piecesAddedTxHash != wantTxHash {
		t.Errorf("OnPiecesAdded txHash=%q, want %q", piecesAddedTxHash, wantTxHash)
	}
	if piecesAddedProviderID != provider.ID {
		t.Errorf("OnPiecesAdded providerID=%d, want %d", piecesAddedProviderID, provider.ID)
	}
	if !submittedPiecesEqual(piecesAddedPieces, []SubmittedPiece{{PieceCID: info.CIDv2}}) {
		t.Errorf("OnPiecesAdded pieces=%v, want [{%s}]", piecesAddedPieces, info.CIDv2)
	}
	if piecesConfirmedDSID != types.DataSetID(55) {
		t.Errorf("OnPiecesConfirmed dataSetID=%d, want 55", piecesConfirmedDSID)
	}
	if piecesConfirmedProvider != provider.ID {
		t.Errorf("OnPiecesConfirmed providerID=%d, want %d", piecesConfirmedProvider, provider.ID)
	}
	if !confirmedPiecesEqual(piecesConfirmedPieces, []ConfirmedPiece{{
		PieceID:  types.PieceID(77),
		PieceCID: info.CIDv2,
	}}) {
		t.Errorf("OnPiecesConfirmed pieces=%v, want [{77 %s}]", piecesConfirmedPieces, info.CIDv2)
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
				DataSetID:         55,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []*big.Int{big.NewInt(0)},
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
		OnPiecesConfirmed: func(_ types.DataSetID, _ types.ProviderID, pieces []ConfirmedPiece) {
			confirmed = append([]ConfirmedPiece(nil), pieces...)
		},
	}

	if _, err := ctx.Upload(context.Background(), bytes.NewReader(data), opts); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if !confirmedPiecesEqual(confirmed, []ConfirmedPiece{{
		PieceID:  0,
		PieceCID: info.CIDv2,
	}}) {
		t.Fatalf("OnPiecesConfirmed pieces=%v, want [{0 %s}]", confirmed, info.CIDv2)
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
		id:       types.ProviderID(101),
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
				DataSetID:     types.DataSetID(1001),
				PieceIDs:      []types.PieceID{types.PieceID(2001)},
				IsNewDataSet:  true,
			}, nil
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
				DataSetID:     types.DataSetID(1002),
				PieceIDs:      []types.PieceID{types.PieceID(2002)},
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
		OnStored: func(providerID types.ProviderID, pieceCID cid.Cid) {
			mu.Lock()
			defer mu.Unlock()
			storedEvents = append(storedEvents, copyEvent{providerID: providerID, pieceCID: pieceCID})
		},
		OnPiecesAdded: func(txHash string, providerID types.ProviderID, pieces []SubmittedPiece) {
			mu.Lock()
			defer mu.Unlock()
			piecesAddedEvents = append(piecesAddedEvents, submittedEvent{
				providerID: providerID,
				txHash:     txHash,
				pieces:     append([]SubmittedPiece(nil), pieces...),
			})
		},
		OnPiecesConfirmed: func(dataSetID types.DataSetID, providerID types.ProviderID, pieces []ConfirmedPiece) {
			mu.Lock()
			defer mu.Unlock()
			piecesConfirmedEvt = append(piecesConfirmedEvt, confirmedEvent{
				providerID: providerID,
				dataSetID:  dataSetID,
				pieces:     append([]ConfirmedPiece(nil), pieces...),
			})
		},
		OnCopyComplete: func(providerID types.ProviderID, pieceCID cid.Cid) {
			mu.Lock()
			defer mu.Unlock()
			copyCompleteEvents = append(copyCompleteEvents, copyEvent{providerID: providerID, pieceCID: pieceCID})
		},
		OnCopyFailed: func(providerID types.ProviderID, pieceCID cid.Cid, err error) {
			mu.Lock()
			defer mu.Unlock()
			copyFailedEvents = append(copyFailedEvents, copyFailureEvent{providerID: providerID, pieceCID: pieceCID, err: err})
		},
		OnPullProgress: func(providerID types.ProviderID, pieceCID cid.Cid, status PullStatus) {
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

	if len(storedEvents) != 1 || !hasCopyEvent(storedEvents, copyEvent{providerID: primary.id, pieceCID: info.CIDv2}) {
		t.Errorf("OnStored: got %v, want [{provider=%d pieceCID=%s}]", storedEvents, primary.id, info.CIDv2)
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
			t.Errorf("OnPiecesAdded missing event %+v in %v", want, piecesAddedEvents)
		}
	}

	wantConfirmed := []confirmedEvent{
		{providerID: primary.id, dataSetID: types.DataSetID(1001), pieces: []ConfirmedPiece{{PieceID: types.PieceID(2001), PieceCID: info.CIDv2}}},
		{providerID: replacement.id, dataSetID: types.DataSetID(1002), pieces: []ConfirmedPiece{{PieceID: types.PieceID(2002), PieceCID: info.CIDv2}}},
	}
	if len(piecesConfirmedEvt) != len(wantConfirmed) {
		t.Errorf("OnPiecesConfirmed: got %d events, want %d", len(piecesConfirmedEvt), len(wantConfirmed))
	}
	for _, want := range wantConfirmed {
		if !hasConfirmedEvent(piecesConfirmedEvt, want) {
			t.Errorf("OnPiecesConfirmed missing event %+v in %v", want, piecesConfirmedEvt)
		}
	}

	// OnCopyComplete fires when a secondary's SP-to-SP pull completes, not on
	// commit. In this scenario the replacement's pull succeeds; the primary has
	// no pull step and the failedSecondary's pull fails before OnCopyComplete fires.
	if len(copyCompleteEvents) != 1 || !hasCopyEvent(copyCompleteEvents, copyEvent{providerID: replacement.id, pieceCID: info.CIDv2}) {
		t.Errorf("OnCopyComplete: got %v, want [{provider=%d pieceCID=%s}]", copyCompleteEvents, replacement.id, info.CIDv2)
	}

	if len(copyFailedEvents) != 1 {
		t.Errorf("OnCopyFailed: got %d events, want 1", len(copyFailedEvents))
	} else {
		got := copyFailedEvents[0]
		if got.providerID != failedSecondary.id || got.pieceCID != info.CIDv2 || got.err == nil {
			t.Errorf("OnCopyFailed: got %+v, want {provider=%d pieceCID=%s err!=nil}", got, failedSecondary.id, info.CIDv2)
		}
	}

	if !hasPullProgressEvent(pullProgressEvents, pullProgressEvent{
		providerID: replacement.id,
		pieceCID:   info.CIDv2,
		status:     PullStatusComplete,
	}) {
		t.Errorf("OnPullProgress missing {provider=%d pieceCID=%s status=%s} in %v", replacement.id, info.CIDv2, PullStatusComplete, pullProgressEvents)
	}
}

func TestManagerUpload_CallbacksAllowZeroPieceID(t *testing.T) {
	data := bytes.Repeat([]byte("mz"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}

	primary := &fakeUploadContext{
		id:       types.ProviderID(101),
		endpoint: "https://primary.example.com",
		pieceURL: "https://primary.example.com/piece/" + info.CIDv2.String(),
		storeFn: func(_ context.Context, r io.Reader, _ *StoreOptions) (*StoreResult, error) {
			_, _ = io.Copy(io.Discard, r)
			return &StoreResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		commitFn: func(_ context.Context, _ CommitRequest) (*CommitResult, error) {
			return &CommitResult{
				TransactionID: "0xprimary",
				DataSetID:     types.DataSetID(1001),
				PieceIDs:      []types.PieceID{0},
				IsNewDataSet:  true,
			}, nil
		},
	}
	mgr := mustNewService(t, Options{Resolver: &fakeResolver{contexts: []UploadContext{primary}}})

	var confirmed []confirmedEvent
	opts := &UploadOptions{
		Copies: 1,
		OnPiecesConfirmed: func(dataSetID types.DataSetID, providerID types.ProviderID, pieces []ConfirmedPiece) {
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
		dataSetID:  types.DataSetID(1001),
		pieces:     []ConfirmedPiece{{PieceID: 0, PieceCID: info.CIDv2}},
	}}
	if len(confirmed) != 1 || !hasConfirmedEvent(confirmed, want[0]) {
		t.Fatalf("OnPiecesConfirmed=%v, want %v", confirmed, want)
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
