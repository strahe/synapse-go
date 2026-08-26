package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/types"
)

func TestAddCommitLifecycleCanResumeFromJSON(t *testing.T) {
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
		OnSubmitted: func(txHash string) {
			callbackCalls++
			if txHash != originalTx.Hex() {
				t.Fatalf("callback tx=%s", txHash)
			}
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
	if submission.Identity != (ContextIdentity{}) {
		t.Fatalf("identity=%+v want exact zero receiver identity", submission.Identity)
	}

	persisted, err := json.Marshal(submission)
	if err != nil {
		t.Fatal(err)
	}
	var restored CommitSubmission
	if err := json.Unmarshal(persisted, &restored); err != nil {
		t.Fatal(err)
	}
	validator := &fakeDataSetValidator{}
	fresh, err := NewDataSetContext(testProvider(), client, nil, ref, WithDataSetValidator(validator))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := fresh.GetCommitStatus(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != CommitStatePending || pending.DataSet == nil || !pending.DataSet.Equal(ref) {
		t.Fatalf("pending=%+v", pending)
	}
	if statusCalls != 1 {
		t.Fatalf("one-shot status calls=%d want 1", statusCalls)
	}

	confirmed, err := fresh.GetCommitStatus(context.Background(), restored)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.State != CommitStateConfirmed || confirmed.ConfirmedTransactionID != confirmedTx.Hex() {
		t.Fatalf("confirmed=%+v", confirmed)
	}
	result, err := fresh.WaitForCommit(context.Background(), restored)
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
			_, err := providerContext.SubmitCommit(context.Background(), CommitRequest{
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

func TestGetCommitStatusRejectsPersistedDuplicateCIDBeforeNetwork(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	statusCalls := 0
	client := &fakePDPProviderClient{
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			statusCalls++
			return nil, errors.New("unexpected status request")
		},
	}
	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	submission := CommitSubmission{
		Kind:          CommitKindAddPieces,
		TransactionID: common.HexToHash("0x11").Hex(),
		StatusURL:     "https://sp.example.com/status/add",
		ProviderID:    testProvider().ID,
		DataSet:       &ref,
		PieceCIDs:     []cid.Cid{pieceCID, pieceCID},
	}
	if _, err := ctx.GetCommitStatus(context.Background(), submission); !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "duplicate pieceCID") {
		t.Fatalf("error=%v want duplicate ErrInvalidArgument", err)
	}
	if statusCalls != 0 {
		t.Fatalf("statusCalls=%d want 0", statusCalls)
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
	submission, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: pieceCID}},
		ExtraData: extraData,
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

	mismatched := mustWritableProviderContext(t, client, WithPayer(common.HexToAddress("0x9999")))
	_, err = mismatched.SubmitCommit(context.Background(), CommitRequest{
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
	submission, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: pieceCID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := ctx.GetCommitStatus(context.Background(), *submission)
	if err != nil {
		t.Fatal(err)
	}
	if pending.State != CommitStatePending || pending.DataSet != nil {
		t.Fatalf("pending exposed data set: %+v", pending)
	}
	result, err := ctx.WaitForCommit(context.Background(), *submission)
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
	status, err := ctx.GetCommitStatus(context.Background(), *submission)
	if err != nil || status.State != CommitStateRejected || status.DataSet == nil || !status.DataSet.Equal(ref) {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	_, err = ctx.WaitForCommit(context.Background(), *submission)
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
	submission.PieceCIDs[0] = cid.Undef
	if rejected.Submission.PieceCIDs[0] != pieceCID || rejected.Status.DataSet == nil || !rejected.Status.DataSet.Equal(ref) {
		t.Fatalf("rejection snapshots were not copied: %+v", rejected)
	}
}

func TestRejectedCommitRejectsNonzeroPieceCountMismatch(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
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
	submission := CommitSubmission{
		Kind:          CommitKindAddPieces,
		TransactionID: originalTx.Hex(),
		StatusURL:     "https://sp.example.com/status/rejected",
		ProviderID:    testProvider().ID,
		DataSet:       &ref,
		PieceCIDs:     []cid.Cid{pieceCID},
	}
	if _, err := ctx.GetCommitStatus(context.Background(), submission); !errors.Is(err, pdp.ErrInvalidStatus) {
		t.Fatalf("error=%v want ErrInvalidStatus", err)
	}
}

func TestWaitForCommitTreatsReorgedSuccessSnapshotAsRejected(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
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
	submission := CommitSubmission{
		Kind:          CommitKindAddPieces,
		TransactionID: originalTx.Hex(),
		StatusURL:     "https://sp.example.com/status/reorged",
		ProviderID:    testProvider().ID,
		DataSet:       &ref,
		PieceCIDs:     []cid.Cid{pieceCID},
	}
	status, err := ctx.GetCommitStatus(context.Background(), submission)
	if err != nil || status.State != CommitStateRejected || len(status.PieceIDs) != 0 {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	_, err = ctx.WaitForCommit(context.Background(), submission)
	var rejected *CommitRejectedError
	if !errors.As(err, &rejected) || !errors.Is(err, pdp.ErrTxRejected) {
		t.Fatalf("error=%v want CommitRejectedError", err)
	}
}

func TestCreateStageRejectionDoesNotRequireAddSnapshot(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
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
	submission := CommitSubmission{
		Kind:            CommitKindCreateAndAdd,
		TransactionID:   originalTx.Hex(),
		StatusURL:       "https://sp.example.com/status/create",
		ProviderID:      testProvider().ID,
		Identity:        ctx.ContextIdentity(),
		ClientDataSetID: &clientDataSetID,
		PieceCIDs:       []cid.Cid{pieceCID},
	}
	status, err := ctx.GetCommitStatus(context.Background(), submission)
	if err != nil || status.State != CommitStateRejected || status.DataSet != nil {
		t.Fatalf("status=%+v error=%v", status, err)
	}
	_, err = ctx.WaitForCommit(context.Background(), submission)
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
	if _, err := ctx.WaitForCommit(cancelled, *submission); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait error=%v", err)
	}
	confirmed = true
	result, err := ctx.WaitForCommit(context.Background(), *submission)
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
		getAddedFn: func(context.Context, string) (*pdp.AddPiecesStatus, error) {
			statusCalls++
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
	if _, err := ctx.GetCommitStatus(context.Background(), *submission); !errors.Is(err, pdp.ErrInvalidStatus) || errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("provider mismatch error=%v", err)
	}
	wrongDataSet = false
	if _, err := ctx.GetCommitStatus(context.Background(), *submission); !errors.Is(err, pdp.ErrInvalidStatus) || errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("missing confirmed pieceCount error=%v", err)
	}
	invalid := copyCommitSubmission(*submission)
	invalid.StatusURL = "https://other.example/status?token=secret"
	if _, err := ctx.GetCommitStatus(context.Background(), invalid); !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, pdp.ErrStatusURLOrigin) {
		t.Fatalf("caller URL error=%v", err)
	}
	if statusCalls != 2 {
		t.Fatalf("invalid caller submission reached network: statusCalls=%d", statusCalls)
	}
	invalid = copyCommitSubmission(*submission)
	invalid.Identity.Payer = common.HexToAddress("0x9999")
	if _, err := ctx.GetCommitStatus(context.Background(), invalid); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("caller identity error=%v", err)
	}
	if statusCalls != 2 {
		t.Fatalf("identity mismatch reached network: statusCalls=%d", statusCalls)
	}
}

func TestSubmitCommitRejectsInvalidProviderHandleBeforeCallback(t *testing.T) {
	pieceCID := mustPieceInfo(t).CIDv2
	ref := testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))
	callbackCalls := 0
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{
				TxHash:    common.HexToHash("0x88"),
				StatusURL: "https://other.example/status?token=secret",
			}, nil
		},
	}
	ctx, err := NewDataSetContext(testProvider(), client, nil, ref)
	if err != nil {
		t.Fatal(err)
	}
	_, err = ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:      []PieceInput{{PieceCID: pieceCID}},
		ExtraData:   []byte{1},
		OnSubmitted: func(string) { callbackCalls++ },
	})
	if !errors.Is(err, pdp.ErrStatusURLOrigin) || errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("error=%v", err)
	}
	if callbackCalls != 0 || strings.Contains(err.Error(), "secret") {
		t.Fatalf("callbackCalls=%d error=%v", callbackCalls, err)
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
				_, err := providerContext.SubmitCommit(ctx, CommitRequest{})
				return err
			},
		},
		{
			name: "provider get",
			call: func() error {
				_, err := providerContext.GetCommitStatus(ctx, CommitSubmission{})
				return err
			},
		},
		{
			name: "provider wait",
			call: func() error {
				_, err := providerContext.WaitForCommit(ctx, CommitSubmission{})
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
				_, err := dataSetContext.GetCommitStatus(ctx, CommitSubmission{})
				return err
			},
		},
		{
			name: "data set wait",
			call: func() error {
				_, err := dataSetContext.WaitForCommit(ctx, CommitSubmission{})
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
