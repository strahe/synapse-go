package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/big"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

func TestAddCommitLifecycleCanResumeFromStatusURL(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	secondInfo, err := piece.CalculateFromBytes(bytes.Repeat([]byte("rho"), 128))
	if err != nil {
		t.Fatal(err)
	}
	secondPieceCID := secondInfo.CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x11")
	confirmedTx := common.HexToHash("0x22")
	externalExtraData := []byte{0x01, 0x02, 0x03}
	addCalls := 0
	statusCalls := 0
	callbackCalls := 0
	var callbackSubmission CommitSubmission

	client := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, dataSetID types.BigInt, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.AddPiecesResult, error) {
			addCalls++
			if !dataSetID.Equal(ref.DataSetID()) {
				t.Fatalf("dataSetID=%s", dataSetID)
			}
			if len(pieces) != 2 || pieces[0].PieceCID != pieceCID || pieces[1].PieceCID != secondPieceCID {
				t.Fatalf("pieces=%+v", pieces)
			}
			if !bytes.Equal(extraData, externalExtraData) {
				t.Fatalf("extraData=%x", extraData)
			}
			extraData[0] = 0xff
			return &pdp.AddPiecesResult{
				TxHash:    originalTx,
				StatusURL: "https://sp.example.com/status/add",
			}, nil
		},
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			statusCalls++
			if statusCalls == 1 {
				return &pdp.AddPiecesStatus{
					TxHash:    originalTx,
					TxStatus:  "pending",
					DataSetID: ref.DataSetID(),
				}, nil
			}
			return &pdp.AddPiecesStatus{
				TxHash:            originalTx,
				ConfirmedTxHash:   confirmedTx,
				TxStatus:          "confirmed",
				DataSetID:         ref.DataSetID(),
				PieceCount:        2,
				AddMessageOK:      new(true),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8), types.NewBigInt(9)},
			}, nil
		},
	}

	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	pieces := []PieceInput{{PieceCID: pieceCID}, {PieceCID: secondPieceCID}}
	submission, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:    pieces,
		ExtraData: externalExtraData,
		OnSubmitted: func(got CommitSubmission) {
			callbackCalls++
			callbackSubmission = copyCommitSubmission(got)
			if got.TransactionID != originalTx.Hex() || got.Kind != CommitKindAddPieces || got.DataSet == nil || !got.DataSet.Equal(ref) {
				t.Fatalf("callback submission=%+v", got)
			}
			got.PieceCIDs[0] = cid.Undef
			*got.DataSet = testDataSetRef(types.NewBigInt(99), types.NewBigInt(100))
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if addCalls != 1 || callbackCalls != 1 {
		t.Fatalf("addCalls=%d callbackCalls=%d", addCalls, callbackCalls)
	}
	if !bytes.Equal(externalExtraData, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("caller extraData mutated: %x", externalExtraData)
	}
	pieces[0].PieceCID = cid.Undef
	if submission.PieceCIDs[0] != pieceCID || submission.PieceCIDs[1] != secondPieceCID {
		t.Fatalf("submission piece order changed: %v", submission.PieceCIDs)
	}
	if submission.DataSet == nil || !submission.DataSet.Equal(ref) {
		t.Fatalf("callback mutated returned data set: %+v", submission.DataSet)
	}
	if callbackSubmission.TransactionID != submission.TransactionID || callbackSubmission.DataSet == nil || !callbackSubmission.DataSet.Equal(ref) {
		t.Fatalf("callback submission=%+v", callbackSubmission)
	}
	if submission.Identity != (ContextIdentity{}) {
		t.Fatalf("identity=%+v want exact zero receiver identity", submission.Identity)
	}

	validator := &fakeDataSetValidator{}
	fresh, err := NewDataSetContext(testProvider(), client, nil, ref, WithDataSetValidator(validator))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := fresh.GetCommitStatus(context.Background(), submission.StatusURL)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != CommitStatePending || pending.DataSet == nil || !pending.DataSet.Equal(ref) {
		t.Fatalf("pending=%+v", pending)
	}
	if statusCalls != 1 {
		t.Fatalf("one-shot status calls=%d want 1", statusCalls)
	}

	confirmed, err := fresh.GetCommitStatus(context.Background(), submission.StatusURL)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.State != CommitStateConfirmed || confirmed.ConfirmedTransactionID != confirmedTx.Hex() {
		t.Fatalf("confirmed=%+v", confirmed)
	}
	result, err := fresh.WaitForCommit(context.Background(), submission.StatusURL)
	if err != nil {
		t.Fatal(err)
	}
	if result.TransactionID != originalTx.Hex() ||
		result.ConfirmedTransactionID != confirmedTx.Hex() ||
		!result.DataSet.Equal(ref) ||
		result.IsNewDataSet ||
		len(result.PieceIDs) != 2 {
		t.Fatalf("result=%+v", result)
	}
	if addCalls != 1 {
		t.Fatalf("resuming submitted commit performed %d submissions", addCalls)
	}
	if len(validator.calls) != 0 {
		t.Fatalf("status recovery called data-set validator %d times", len(validator.calls))
	}
}

func TestContextRejectsOversizedExternalPayloadBeforeProviderCall(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	providerCalls := 0
	fromCalls := 0
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			providerCalls++
			return nil, errors.New("unexpected AddPieces")
		},
		pullPiecesFn: func(context.Context, pdp.PullRequest) (*pdp.PullResult, error) {
			providerCalls++
			return nil, errors.New("unexpected WaitForPullComplete")
		},
	}
	ctx := mustDataSetContext(t, client, testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)))
	extraData := make([]byte, pdp.MaxAddPiecesMessageSize)

	_, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: pieceCID}},
		ExtraData: extraData,
	})
	assertAddPiecesMessageTooLarge(t, err)
	_, err = ctx.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{pieceCID},
		From: func(cid.Cid) string {
			fromCalls++
			return "https://source.example.com/piece"
		},
		ExtraData: extraData,
	})
	assertAddPiecesMessageTooLarge(t, err)
	if fromCalls != 0 {
		t.Fatalf("source resolver calls=%d want 0", fromCalls)
	}
	if providerCalls != 0 {
		t.Fatalf("provider calls=%d want 0", providerCalls)
	}
}

func assertAddPiecesMessageTooLarge(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, pdp.ErrAddPiecesMessageTooLarge) {
		t.Fatalf("error=%v want ErrInvalidArgument and ErrAddPiecesMessageTooLarge", err)
	}
	var sizeError *pdp.AddPiecesMessageTooLargeError
	if !errors.As(err, &sizeError) || sizeError.Size <= sizeError.Max {
		t.Fatalf("error=%v sizeError=%+v", err, sizeError)
	}
}

func TestSubmitCommitRejectsDuplicatePieceCIDBeforeDependencies(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	addCalls := 0
	createCalls := 0
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			addCalls++
			return nil, errors.New("unexpected add")
		},
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			createCalls++
			return nil, errors.New("unexpected create")
		},
	}
	dataSetContext, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	providerContext, err := NewProviderContext(testProvider(), client, nil)
	if err != nil {
		t.Fatal(err)
	}

	for name, submit := range map[string]func() error{
		"add": func() error {
			_, err := dataSetContext.SubmitCommit(context.Background(), CommitRequest{
				Pieces:    []PieceInput{{PieceCID: pieceCID}, {PieceCID: pieceCID}},
				ExtraData: []byte{1},
			})
			return err
		},
		"create and add": func() error {
			_, err := providerContext.SubmitCreateAndAdd(context.Background(), CreateAndAddRequest{
				Pieces: []PieceInput{{PieceCID: pieceCID}, {PieceCID: pieceCID}},
			})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := submit(); !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "duplicate pieceCID") {
				t.Fatalf("error=%v want duplicate ErrInvalidArgument", err)
			}
		})
	}
	if addCalls != 0 || createCalls != 0 {
		t.Fatalf("addCalls=%d createCalls=%d", addCalls, createCalls)
	}
}

func TestCreateAndAddExternalExtraDataExtractsIdentityBeforeNetwork(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	clientDataSetID := types.NewBigInt(0)
	createPayload, err := encodeCreateDataSetExtraData(
		testPayer(),
		clientDataSetID.Big(),
		nil,
		make([]byte, 65),
	)
	if err != nil {
		t.Fatal(err)
	}
	addPayload, err := encodeAddPiecesExtraData(
		big.NewInt(1),
		[][]ityped.MetadataEntry{{}},
		make([]byte, 65),
	)
	if err != nil {
		t.Fatal(err)
	}
	extraData, err := encodeCreateAndAddExtraData(createPayload, addPayload)
	if err != nil {
		t.Fatal(err)
	}

	originalTx := common.HexToHash("0x33")
	createCalls := 0
	client := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, _ common.Address, _ []pdp.AddPieceInput, got []byte) (*pdp.CreateDataSetResult, error) {
			createCalls++
			if !bytes.Equal(got, extraData) {
				t.Fatalf("extraData changed: %x", got)
			}
			return &pdp.CreateDataSetResult{
				TxHash:    originalTx,
				StatusURL: "https://sp.example.com/status/create",
			}, nil
		},
	}
	ctx := mustWritableProviderContext(t, client)
	submission, err := ctx.SubmitCreateAndAdd(context.Background(), CreateAndAddRequest{
		Pieces:          []PieceInput{{PieceCID: pieceCID}},
		ExtraData:       extraData,
		ClientDataSetID: &clientDataSetID,
		OnSubmitted: func(got CommitSubmission) {
			if got.ClientDataSetID == nil {
				t.Fatal("callback client data-set ID is nil")
			}
			*got.ClientDataSetID = types.NewBigInt(99)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if submission.ClientDataSetID == nil || !submission.ClientDataSetID.IsZero() {
		t.Fatalf("clientDataSetID=%v want explicit zero", submission.ClientDataSetID)
	}
	if submission.DataSet != nil || submission.Kind != CommitKindCreateAndAdd {
		t.Fatalf("submission=%+v", submission)
	}

	conflictingClientDataSetID := types.NewBigInt(1)
	_, err = ctx.SubmitCreateAndAdd(context.Background(), CreateAndAddRequest{
		Pieces:          []PieceInput{{PieceCID: pieceCID}},
		ExtraData:       extraData,
		ClientDataSetID: &conflictingClientDataSetID,
	})
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("clientDataSetID mismatch error=%v", err)
	}
	if createCalls != 1 {
		t.Fatalf("clientDataSetID mismatch reached network: createCalls=%d", createCalls)
	}

	mismatched := mustWritableProviderContext(t, client, WithPayer(common.HexToAddress("0x9999")))
	_, err = mismatched.SubmitCreateAndAdd(context.Background(), CreateAndAddRequest{
		Pieces:    []PieceInput{{PieceCID: pieceCID}},
		ExtraData: extraData,
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("payer mismatch error=%v", err)
	}
	if createCalls != 1 {
		t.Fatalf("payer mismatch reached network: createCalls=%d", createCalls)
	}
}

func TestDataSetContextSubmitCommitRejectsClientDataSetIDBeforeNetwork(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	clientDataSetID := types.NewBigInt(7)
	addCalls := 0
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			addCalls++
			return nil, errors.New("unexpected add")
		},
	}
	ctx := mustWritableDataSetContext(
		t,
		client,
		testDataSetRef(types.NewBigInt(42), types.NewBigInt(9)),
	)

	_, err := ctx.submitCommit(context.Background(), commitRequest{
		CommitRequest: CommitRequest{
			Pieces:    []PieceInput{{PieceCID: pieceCID}},
			ExtraData: []byte{1},
		},
		clientDataSetID: &clientDataSetID,
	})
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "only valid when creating") {
		t.Fatalf("error=%v want ClientDataSetID ErrInvalidArgument", err)
	}
	if addCalls != 0 {
		t.Fatalf("addCalls=%d want 0", addCalls)
	}
}

func TestCreateAndAddStatusHidesDataSetUntilOverallConfirmation(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	originalTx := common.HexToHash("0x44")
	confirmedTx := common.HexToHash("0x45")
	dataSetID := types.NewBigInt(77)
	statusCalls := 0
	client := &fakePDPProviderClient{
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{TxHash: originalTx, StatusURL: "https://sp.example.com/status/create"}, nil
		},
		getCreateAndAddFn: func(context.Context, string) (*pdp.CreateAndAddPiecesStatus, error) {
			statusCalls++
			created := &pdp.CreateDataSetStatus{
				CreateMessageHash: originalTx,
				ConfirmedTxHash:   confirmedTx,
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				OK:                new(true),
				DataSetID:         copyBigIntPtr(&dataSetID),
			}
			add := &pdp.AddPiecesStatus{
				TxHash:          originalTx,
				ConfirmedTxHash: confirmedTx,
				TxStatus:        "confirmed",
				DataSetID:       dataSetID,
				PieceCount:      1,
				AddMessageOK:    new(true),
			}
			if statusCalls > 1 {
				add.PiecesAdded = true
				add.ConfirmedPieceIDs = []types.BigInt{types.NewBigInt(8)}
			}
			return &pdp.CreateAndAddPiecesStatus{Create: created, Add: add}, nil
		},
	}
	ctx := mustWritableProviderContext(t, client)
	submission, err := ctx.SubmitCreateAndAdd(context.Background(), CreateAndAddRequest{
		Pieces: []PieceInput{{PieceCID: pieceCID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := ctx.GetCreateAndAddStatus(context.Background(), submission.StatusURL, *submission.ClientDataSetID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != CommitStatePending || pending.DataSet != nil {
		t.Fatalf("pending exposed data set: %+v", pending)
	}
	result, err := ctx.WaitForCreateAndAdd(context.Background(), submission.StatusURL, *submission.ClientDataSetID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsNewDataSet ||
		!result.DataSet.DataSetID().Equal(dataSetID) ||
		result.ConfirmedTransactionID != confirmedTx.Hex() {
		t.Fatalf("result=%+v", result)
	}
	if _, ok := ctx.DataSetRef(); ok {
		t.Fatal("ProviderContext was retargeted")
	}
}

func TestWaitForCommitReturnsTypedRejectionWithDefensiveSnapshots(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x55")
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{TxHash: originalTx, StatusURL: "https://sp.example.com/status/rejected?token=secret"}, nil
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
	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: pieceCID}},
		ExtraData: []byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := ctx.GetCommitStatus(context.Background(), submission.StatusURL)
	if err != nil || status.State != CommitStateRejected || status.DataSet == nil || !status.DataSet.Equal(ref) {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	_, err = ctx.WaitForCommit(context.Background(), submission.StatusURL)
	if !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("error=%v want ErrTxRejected", err)
	}
	var rejected *CommitRejectedError
	if !errors.As(err, &rejected) {
		t.Fatalf("error type=%T", err)
	}
	if strings.Contains(rejected.Error(), submission.StatusURL) || strings.Contains(rejected.Error(), "secret") {
		t.Fatalf("rejection error leaked status URL: %v", rejected)
	}
	*status.DataSet = testDataSetRef(types.NewBigInt(99), types.NewBigInt(100))
	if !rejected.ProviderID.Equal(testProvider().ID) || rejected.Status.DataSet == nil || !rejected.Status.DataSet.Equal(ref) {
		t.Fatalf("rejection snapshots were not copied: %+v", rejected)
	}
}

func TestRejectedCommitDoesNotRequireOriginalPieceCount(t *testing.T) {
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x58")
	client := &fakePDPProviderClient{
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:       originalTx,
				TxStatus:     "failed",
				DataSetID:    ref.DataSetID(),
				PieceCount:   2,
				AddMessageOK: new(false),
			}, pdp.ErrTxRejected
		},
	}
	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	status, err := ctx.GetCommitStatus(context.Background(), "https://sp.example.com/status/rejected")
	if err != nil || status.State != CommitStateRejected {
		t.Fatalf("status=%+v error=%v", status, err)
	}
}

func TestWaitForCommitTreatsReorgedSuccessSnapshotAsRejected(t *testing.T) {
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x59")
	client := &fakePDPProviderClient{
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            originalTx,
				TxStatus:          "reorged",
				DataSetID:         ref.DataSetID(),
				PieceCount:        1,
				AddMessageOK:      new(true),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
			}, pdp.ErrTxRejected
		},
	}
	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	statusURL := "https://sp.example.com/status/reorged"
	status, err := ctx.GetCommitStatus(context.Background(), statusURL)
	if err != nil || status.State != CommitStateRejected || len(status.PieceIDs) != 0 {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	_, err = ctx.WaitForCommit(context.Background(), statusURL)
	var rejected *CommitRejectedError
	if !errors.As(err, &rejected) || !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("error=%v want CommitRejectedError", err)
	}
}

func TestCreateStageRejectionDoesNotRequireAddSnapshot(t *testing.T) {
	originalTx := common.HexToHash("0x56")
	clientDataSetID := types.NewBigInt(0)
	client := &fakePDPProviderClient{
		getCreateAndAddFn: func(context.Context, string) (*pdp.CreateAndAddPiecesStatus, error) {
			return &pdp.CreateAndAddPiecesStatus{
				Create: &pdp.CreateDataSetStatus{
					CreateMessageHash: originalTx,
					TxStatus:          "failed",
					OK:                new(false),
				},
			}, pdp.ErrTxRejected
		},
	}
	ctx := mustWritableProviderContext(t, client)
	statusURL := "https://sp.example.com/status/create"
	status, err := ctx.GetCreateAndAddStatus(context.Background(), statusURL, clientDataSetID)
	if err != nil || status.State != CommitStateRejected || status.DataSet != nil {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	_, err = ctx.WaitForCreateAndAdd(context.Background(), statusURL, clientDataSetID)
	var rejected *CommitRejectedError
	if !errors.As(err, &rejected) || !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("error=%v", err)
	}
}

func TestWaitForCommitCanResumeAfterCancellationWithoutResubmitting(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x66")
	submissions := 0
	confirmed := false
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			submissions++
			return &pdp.AddPiecesResult{TxHash: originalTx, StatusURL: "https://sp.example.com/status/add"}, nil
		},
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			status := &pdp.AddPiecesStatus{TxHash: originalTx, DataSetID: ref.DataSetID()}
			if confirmed {
				status.TxStatus = "confirmed"
				status.PieceCount = 1
				status.AddMessageOK = new(true)
				status.PiecesAdded = true
				status.ConfirmedPieceIDs = []types.BigInt{types.NewBigInt(9)}
			} else {
				status.TxStatus = "pending"
			}
			return status, nil
		},
	}
	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: pieceCID}},
		ExtraData: []byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ctx.WaitForCommit(cancelled, submission.StatusURL); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error=%v", err)
	}
	confirmed = true
	result, err := ctx.WaitForCommit(context.Background(), submission.StatusURL)
	if err != nil {
		t.Fatal(err)
	}
	if submissions != 1 || !result.DataSet.Equal(ref) {
		t.Fatalf("submissions=%d result=%+v", submissions, result)
	}
}

func TestCommitStatusSeparatesCallerAndProviderValidationErrors(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	originalTx := common.HexToHash("0x77")
	statusCalls := 0
	wrongDataSet := true
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{TxHash: originalTx, StatusURL: "https://sp.example.com/status/add"}, nil
		},
		getAddedFn: func(_ context.Context, statusURL string) (*pdp.AddPiecesStatus, error) {
			statusCalls++
			if strings.HasSuffix(statusURL, "/not-a-hash") {
				return nil, pdp.ErrInvalidStatusURL
			}
			dataSetID := ref.DataSetID()
			pieceCount := 0
			if wrongDataSet {
				dataSetID = types.NewBigInt(99)
				pieceCount = 1
			}
			return &pdp.AddPiecesStatus{
				TxHash:            originalTx,
				TxStatus:          "confirmed",
				DataSetID:         dataSetID,
				PieceCount:        pieceCount,
				AddMessageOK:      new(true),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
			}, nil
		},
	}
	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: pieceCID}},
		ExtraData: []byte{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.GetCommitStatus(context.Background(), submission.StatusURL); !errors.Is(err, pdp.ErrInvalidStatus) || errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("provider mismatch error=%v", err)
	}
	wrongDataSet = false
	if _, err := ctx.GetCommitStatus(context.Background(), submission.StatusURL); !errors.Is(err, pdp.ErrInvalidStatus) || errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("missing confirmed pieceCount error=%v", err)
	}
	if _, err := ctx.GetCommitStatus(context.Background(), "https://other.example/status?token=secret"); !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, pdp.ErrStatusURLOrigin) || !errors.Is(err, pdp.ErrInvalidStatusURL) {
		t.Fatalf("caller URL error=%v", err)
	}
	if statusCalls != 2 {
		t.Fatalf("invalid caller URL reached network: statusCalls=%d", statusCalls)
	}
	if _, err := ctx.GetCommitStatus(context.Background(), "https://sp.example.com/status/not-a-hash"); !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, pdp.ErrInvalidStatusURL) || errors.Is(err, pdp.ErrStatusURLOrigin) {
		t.Fatalf("malformed caller URL error=%v", err)
	}
	if statusCalls != 3 {
		t.Fatalf("malformed caller URL statusCalls=%d want 3", statusCalls)
	}
}

func TestSubmitCommitDoesNotInvokeCallbackWithoutValidProviderHandle(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	tests := map[string]struct {
		addPieces  func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error)
		wantOrigin bool
	}{
		"network failure": {
			addPieces: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
				return nil, errors.New("network failure")
			},
		},
		"nil result": {
			addPieces: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
				return nil, nil
			},
		},
		"invalid transaction": {
			addPieces: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
				return &pdp.AddPiecesResult{StatusURL: "https://sp.example.com/status"}, nil
			},
		},
		"invalid status URL": {
			addPieces: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
				return &pdp.AddPiecesResult{
					TxHash:    common.HexToHash("0x88"),
					StatusURL: "https://other.example/status?token=secret",
				}, nil
			},
			wantOrigin: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			callbackCalls := 0
			ctx, err := NewDataSetContext(testProvider(), &fakePDPProviderClient{addPiecesFn: test.addPieces}, nil, ref)
			if err != nil {
				t.Fatal(err)
			}
			_, err = ctx.SubmitCommit(context.Background(), CommitRequest{
				Pieces:      []PieceInput{{PieceCID: pieceCID}},
				ExtraData:   []byte{1},
				OnSubmitted: func(CommitSubmission) { callbackCalls++ },
			})
			if err == nil || test.wantOrigin != errors.Is(err, pdp.ErrStatusURLOrigin) || errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v wantOrigin=%t", err, test.wantOrigin)
			}
			if callbackCalls != 0 || strings.Contains(err.Error(), "secret") {
				t.Fatalf("callbackCalls=%d error=%v", callbackCalls, err)
			}
		})
	}
}

func TestSubmitCallbacksPropagatePanicValue(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{
				TxHash:    common.HexToHash("0x91"),
				StatusURL: "https://sp.example.com/status/add",
			}, nil
		},
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0x92"),
				StatusURL: "https://sp.example.com/status/create",
			}, nil
		},
	}
	sentinel := &struct{ label string }{label: "callback panic"}
	tests := map[string]func(){
		"provider": func() {
			_, _ = mustWritableProviderContext(t, client).SubmitCreateAndAdd(context.Background(), CreateAndAddRequest{
				Pieces:      []PieceInput{{PieceCID: pieceCID}},
				OnSubmitted: func(CommitSubmission) { panic(sentinel) },
			})
		},
		"data set": func() {
			_, _ = mustWritableDataSetContext(t, client, ref).SubmitCommit(context.Background(), CommitRequest{
				Pieces:      []PieceInput{{PieceCID: pieceCID}},
				ExtraData:   []byte{1},
				OnSubmitted: func(CommitSubmission) { panic(sentinel) },
			})
		},
	}
	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			var recovered any
			func() {
				defer func() { recovered = recover() }()
				call()
			}()
			if recovered != sentinel {
				t.Fatalf("recovered=%v want original panic value", recovered)
			}
		})
	}
}

func TestCreateAndAddCallbackHandleResumesAfterWaitFailure(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	originalTx := common.HexToHash("0x93")
	confirmedTx := common.HexToHash("0x94")
	dataSetID := types.NewBigInt(73)
	statusAvailable := false
	client := &fakePDPProviderClient{
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{
				TxHash:    originalTx,
				StatusURL: "https://sp.example.com/status/create",
			}, nil
		},
		getCreateAndAddFn: func(context.Context, string) (*pdp.CreateAndAddPiecesStatus, error) {
			if !statusAvailable {
				return nil, errors.New("status unavailable")
			}
			return &pdp.CreateAndAddPiecesStatus{
				Create: &pdp.CreateDataSetStatus{
					CreateMessageHash: originalTx,
					ConfirmedTxHash:   confirmedTx,
					TxStatus:          "confirmed",
					DataSetCreated:    true,
					OK:                new(true),
					DataSetID:         copyBigIntPtr(&dataSetID),
				},
				Add: &pdp.AddPiecesStatus{
					TxHash:            originalTx,
					ConfirmedTxHash:   confirmedTx,
					TxStatus:          "confirmed",
					DataSetID:         dataSetID,
					PieceCount:        1,
					AddMessageOK:      new(true),
					PiecesAdded:       true,
					ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
				},
			}, nil
		},
	}
	callbackCalls := 0
	var saved CommitSubmission
	_, err := mustWritableProviderContext(t, client).CreateAndAdd(context.Background(), CreateAndAddRequest{
		Pieces: []PieceInput{{PieceCID: pieceCID}},
		OnSubmitted: func(got CommitSubmission) {
			callbackCalls++
			saved = copyCommitSubmission(got)
		},
	})
	if err == nil || callbackCalls != 1 || saved.TransactionID != originalTx.Hex() {
		t.Fatalf("CreateAndAdd error=%v callbackCalls=%d saved=%+v", err, callbackCalls, saved)
	}

	statusAvailable = true
	result, err := mustWritableProviderContext(t, client).WaitForCreateAndAdd(
		context.Background(),
		saved.StatusURL,
		*saved.ClientDataSetID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsNewDataSet || !result.DataSet.DataSetID().Equal(dataSetID) || result.ConfirmedTransactionID != confirmedTx.Hex() {
		t.Fatalf("resumed result=%+v", result)
	}
}

func TestCommitLifecycleRejectsNilConcreteContexts(t *testing.T) {
	ctx := context.Background()
	var providerContext *ProviderContext
	var dataSetContext *DataSetContext

	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "provider submit",
			call: func() error {
				_, err := providerContext.SubmitCreateAndAdd(ctx, CreateAndAddRequest{})
				return err
			},
		},
		{
			name: "provider get",
			call: func() error {
				_, err := providerContext.GetCreateAndAddStatus(ctx, "", types.BigInt{})
				return err
			},
		},
		{
			name: "provider wait",
			call: func() error {
				_, err := providerContext.WaitForCreateAndAdd(ctx, "", types.BigInt{})
				return err
			},
		},
		{
			name: "data set submit",
			call: func() error {
				_, err := dataSetContext.SubmitCommit(ctx, CommitRequest{})
				return err
			},
		},
		{
			name: "data set get",
			call: func() error {
				_, err := dataSetContext.GetCommitStatus(ctx, "")
				return err
			},
		},
		{
			name: "data set wait",
			call: func() error {
				_, err := dataSetContext.WaitForCommit(ctx, "")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("error=%v want ErrInvalidArgument", err)
			}
		})
	}
}

func TestProviderContextUploadCommitFailureKeepsSubmission(t *testing.T) {
	data := bytes.Repeat([]byte("keep-context-submission"), 128)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	originalTx := common.HexToHash("0x46")
	var statusAvailable atomic.Bool
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			_, _ = io.Copy(io.Discard, r)
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(context.Context, cid.Cid, time.Duration) error { return nil },
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			return &pdp.CreateDataSetResult{TxHash: originalTx, StatusURL: "https://sp.example.com/status/create"}, nil
		},
		waitForCreateAndAddFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			if !statusAvailable.Load() {
				return nil, errors.New("status unavailable")
			}
			return &pdp.AddPiecesStatus{
				TxHash:            originalTx,
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(77)},
			}, nil
		},
	}

	_, err = mustWritableProviderContext(t, client).Upload(context.Background(), bytes.NewReader(data), nil)
	commitErr, ok := errors.AsType[*CommitError](err)
	if !ok {
		t.Fatalf("Upload error=%v, want CommitError", err)
	}
	if commitErr.PieceCID != info.CIDv2 || commitErr.Size != int64(len(data)) {
		t.Fatalf("CommitError piece=%s size=%d, want %s and %d", commitErr.PieceCID, commitErr.Size, info.CIDv2, len(data))
	}
	if len(commitErr.FailedAttempts) != 1 {
		t.Fatalf("FailedAttempts=%+v, want one", commitErr.FailedAttempts)
	}
	attempt := commitErr.FailedAttempts[0]
	if attempt.Role != CopyRolePrimary || attempt.Stage != CopyStageCommit || !attempt.Explicit || attempt.Err == nil {
		t.Fatalf("FailedAttempt=%+v, want an explicit primary commit failure", attempt)
	}
	if attempt.Submission == nil || attempt.Submission.TransactionID != originalTx.Hex() {
		t.Fatalf("Submission=%+v, want transaction %s", attempt.Submission, originalTx.Hex())
	}

	// A fresh context for the same provider resumes the kept submission.
	statusAvailable.Store(true)
	result, err := mustWritableProviderContext(t, client).WaitForCreateAndAdd(
		context.Background(),
		attempt.Submission.StatusURL,
		*attempt.Submission.ClientDataSetID,
	)
	if err != nil {
		t.Fatalf("WaitForCreateAndAdd: %v", err)
	}
	if !result.DataSet.DataSetID().Equal(types.NewBigInt(55)) || len(result.PieceIDs) != 1 || !result.PieceIDs[0].Equal(types.NewBigInt(77)) {
		t.Fatalf("resumed result=%+v, want data set 55 with piece 77", result)
	}
}
