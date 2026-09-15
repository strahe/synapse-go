package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ipfs/go-cid"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestProviderContextSubmitCommitUsesRequestedClientDataSetIDInBothSignatures(t *testing.T) {
	maxValue := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	requested, err := types.BigIntFromBig(maxValue)
	if err != nil {
		t.Fatalf("BigIntFromBig: %v", err)
	}
	want := requested.Copy()
	pieceInfo := mustPieceInfo(t)
	storageSigner := mustTestSigner(t)
	var submittedExtraData []byte
	client := &fakePDPProviderClient{
		createAndAddFn: func(_ context.Context, recordKeeper common.Address, _ []pdp.AddPieceInput, extraData []byte) (*pdp.CreateDataSetResult, error) {
			if recordKeeper != testRecordKeeper() {
				t.Fatalf("recordKeeper=%s want %s", recordKeeper, testRecordKeeper())
			}
			submittedExtraData = append([]byte(nil), extraData...)
			requested = types.NewBigInt(9)
			return &pdp.CreateDataSetResult{
				TxHash:    common.HexToHash("0x1234"),
				StatusURL: "https://sp.example.com/status/create",
			}, nil
		},
	}
	ctx, err := NewProviderContext(
		testProvider(),
		client,
		storageSigner,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}

	submission, err := ctx.SubmitCommit(context.Background(), CommitRequest{
		Pieces:          []PieceInput{{PieceCID: pieceInfo.CIDv2}},
		ClientDataSetID: &requested,
	})
	if err != nil {
		t.Fatalf("SubmitCommit: %v", err)
	}
	if submission.ClientDataSetID == nil || !submission.ClientDataSetID.Equal(want) {
		t.Fatalf("submission clientDataSetID=%v want %s", submission.ClientDataSetID, want.String())
	}
	if requested.Equal(want) {
		t.Fatal("test did not mutate the caller-owned input")
	}

	outer, err := createAndAddArgs.Unpack(submittedExtraData)
	if err != nil {
		t.Fatalf("unpack create-and-add: %v", err)
	}
	createValues, err := createDataSetArgs.Unpack(outer[0].([]byte))
	if err != nil {
		t.Fatalf("unpack create payload: %v", err)
	}
	if got := createValues[1].(*big.Int); got.Cmp(maxValue) != 0 {
		t.Fatalf("create clientDataSetID=%s want %s", got, maxValue)
	}
	domain := ityped.NewDomain(big.NewInt(314159), testRecordKeeper())
	createMessage := ityped.CreateDataSetMessage(
		want.Big(),
		testProvider().Payee,
		decodedMetadataEntries(createValues[2].([]string), createValues[3].([]string)),
	)
	if recovered := recoverRawTypedDataSigner(t, domain, "CreateDataSet", createMessage, createValues[4].([]byte)); recovered != storageSigner.EVMAddress() {
		t.Fatalf("CreateDataSet signer=%s want %s", recovered, storageSigner.EVMAddress())
	}

	addValues, err := addPiecesArgs.Unpack(outer[1].([]byte))
	if err != nil {
		t.Fatalf("unpack add payload: %v", err)
	}
	addMessage, err := ityped.AddPiecesMessage(
		want.Big(),
		addValues[0].(*big.Int),
		[]cid.Cid{pieceInfo.CIDv2},
		nil,
	)
	if err != nil {
		t.Fatalf("build AddPieces message: %v", err)
	}
	if recovered := recoverRawTypedDataSigner(t, domain, "AddPieces", addMessage, addValues[3].([]byte)); recovered != storageSigner.EVMAddress() {
		t.Fatalf("AddPieces signer=%s want %s", recovered, storageSigner.EVMAddress())
	}
}

func TestProviderContextRecoversAmbiguousCreateWithoutSecondPOST(t *testing.T) {
	type handledRequest struct {
		body []byte
		err  error
	}
	requests := make(chan handledRequest, 1)
	var postCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		postCalls.Add(1)
		body, err := io.ReadAll(request.Body)
		if err == nil && (request.Method != http.MethodPost || request.URL.Path != "/pdp/data-sets") {
			err = fmt.Errorf("request=%s %s", request.Method, request.URL.Path)
		}
		requests <- handledRequest{body: body, err: err}
		panic(http.ErrAbortHandler)
	}))
	defer server.Close()

	provider := testProvider()
	provider.ServiceURL = server.URL
	pdpClient, err := pdp.New(server.URL, pdp.WithHTTPClient(server.Client()))
	if err != nil {
		t.Fatalf("pdp.New: %v", err)
	}
	storageSigner := mustTestSigner(t)
	creator, err := NewProviderContext(
		provider,
		pdpClient,
		storageSigner,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	clientDataSetID := types.NewBigInt(0)
	savedProviderID := creator.ProviderID()
	savedIdentity := creator.ContextIdentity()

	_, err = creator.CreateDataSet(context.Background(), &CreateDataSetOptions{ClientDataSetID: &clientDataSetID})
	if err == nil {
		t.Fatal("CreateDataSet succeeded after the provider aborted its response")
	}
	handled := <-requests
	if handled.err != nil {
		t.Fatalf("provider did not receive expected create request: %v", handled.err)
	}
	if postCalls.Load() != 1 {
		t.Fatalf("POST calls=%d want 1", postCalls.Load())
	}
	var request pdp.CreateDataSetRequest
	if err := json.Unmarshal(handled.body, &request); err != nil {
		t.Fatalf("decode create request: %v", err)
	}
	if request.RecordKeeper != testRecordKeeper() {
		t.Fatalf("recordKeeper=%s want %s", request.RecordKeeper, testRecordKeeper())
	}
	extraData, err := hexutil.Decode(request.ExtraData)
	if err != nil {
		t.Fatalf("decode extraData: %v", err)
	}
	createValues, err := createDataSetArgs.Unpack(extraData)
	if err != nil {
		t.Fatalf("unpack create payload: %v", err)
	}
	if createValues[0].(common.Address) != testPayer() || createValues[1].(*big.Int).Sign() != 0 {
		t.Fatalf("create identity=%v", createValues[:2])
	}
	createMessage := ityped.CreateDataSetMessage(
		clientDataSetID.Big(),
		provider.Payee,
		decodedMetadataEntries(createValues[2].([]string), createValues[3].([]string)),
	)
	domain := ityped.NewDomain(savedIdentity.ChainID.BigInt(), savedIdentity.RecordKeeper)
	if recovered := recoverRawTypedDataSigner(t, domain, "CreateDataSet", createMessage, createValues[4].([]byte)); recovered != storageSigner.EVMAddress() {
		t.Fatalf("CreateDataSet signer=%s want %s", recovered, storageSigner.EVMAddress())
	}

	dataSetID := types.NewBigInt(42)
	reader := &fakeFWSSDataSetReader{}
	reader.findFn = func(context.Context, common.Address, types.BigInt) (*warmstorage.DataSetInfo, error) {
		if reader.findCalls == 1 {
			return nil, fmt.Errorf("chain state not visible: %w", warmstorage.ErrNotFound)
		}
		return matchingRecoveryDataSetInfo(provider, dataSetID, clientDataSetID), nil
	}
	fresh, err := NewProviderContext(
		provider,
		pdpClient,
		nil,
		WithPayer(savedIdentity.Payer),
		WithRecordKeeper(savedIdentity.RecordKeeper),
		WithChainID(savedIdentity.ChainID),
		WithFWSSDataSetReader(reader),
	)
	if err != nil {
		t.Fatalf("rebuild ProviderContext: %v", err)
	}
	if !fresh.ProviderID().Equal(savedProviderID) || fresh.ContextIdentity() != savedIdentity {
		t.Fatalf("rebuilt context identity changed: provider=%s identity=%+v", fresh.ProviderID().String(), fresh.ContextIdentity())
	}

	ref, found, err := fresh.FindDataSetByClientDataSetID(context.Background(), clientDataSetID)
	if err != nil || found {
		t.Fatalf("first Find=(%+v, %t, %v), want unknown", ref, found, err)
	}
	ref, found, err = fresh.FindDataSetByClientDataSetID(context.Background(), clientDataSetID)
	if err != nil || !found {
		t.Fatalf("second Find=(%+v, %t, %v), want found", ref, found, err)
	}
	if !ref.ProviderID().Equal(savedProviderID) || !ref.DataSetID().Equal(dataSetID) || !ref.ClientDataSetID().Equal(clientDataSetID) {
		t.Fatalf("recovered ref=%+v", ref)
	}
	if _, bound := fresh.DataSetRef(); bound {
		t.Fatal("FindDataSetByClientDataSetID bound the ProviderContext")
	}
	if postCalls.Load() != 1 {
		t.Fatalf("recovery performed a second POST: calls=%d", postCalls.Load())
	}
}

func TestProviderContextFindDataSetByClientDataSetIDRejectsStableIdentityConflicts(t *testing.T) {
	clientDataSetID := types.NewBigInt(7)
	dataSetID := types.NewBigInt(42)
	tests := []struct {
		name   string
		mutate func(*warmstorage.DataSetInfo) *warmstorage.DataSetInfo
	}{
		{name: "nil record", mutate: func(*warmstorage.DataSetInfo) *warmstorage.DataSetInfo { return nil }},
		{name: "zero data set ID", mutate: func(info *warmstorage.DataSetInfo) *warmstorage.DataSetInfo {
			info.DataSetID = types.BigInt{}
			return info
		}},
		{name: "wrong payer", mutate: func(info *warmstorage.DataSetInfo) *warmstorage.DataSetInfo {
			info.Payer = common.HexToAddress("0x9999")
			return info
		}},
		{name: "wrong client ID", mutate: func(info *warmstorage.DataSetInfo) *warmstorage.DataSetInfo {
			info.ClientDataSetID = types.NewBigInt(8)
			return info
		}},
		{name: "wrong provider ID", mutate: func(info *warmstorage.DataSetInfo) *warmstorage.DataSetInfo {
			info.ProviderID = types.NewBigInt(2)
			return info
		}},
		{name: "wrong service provider", mutate: func(info *warmstorage.DataSetInfo) *warmstorage.DataSetInfo {
			info.ServiceProvider = common.HexToAddress("0x8888")
			return info
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := testProvider()
			info := matchingRecoveryDataSetInfo(provider, dataSetID, clientDataSetID)
			returned := test.mutate(info)
			reader := &fakeFWSSDataSetReader{findInfo: returned}
			ctx := mustWritableProviderContext(t, &fakePDPProviderClient{}, WithFWSSDataSetReader(reader))

			_, found, err := ctx.FindDataSetByClientDataSetID(context.Background(), clientDataSetID)
			if !errors.Is(err, ErrDataSetCorrelationConflict) || found {
				t.Fatalf("Find found=%t error=%v want stable correlation conflict", found, err)
			}
			if reader.findCalls != 2 {
				t.Fatalf("reader calls=%d want initial read plus one confirmation", reader.findCalls)
			}
		})
	}
}

func TestProviderContextFindDataSetByClientDataSetIDTreatsChangedStateAsUnknown(t *testing.T) {
	clientDataSetID := types.NewBigInt(7)
	wrong := matchingRecoveryDataSetInfo(testProvider(), types.NewBigInt(42), clientDataSetID)
	wrong.ProviderID = types.NewBigInt(2)
	reader := &fakeFWSSDataSetReader{}
	reader.findFn = func(context.Context, common.Address, types.BigInt) (*warmstorage.DataSetInfo, error) {
		if reader.findCalls == 1 {
			return wrong, nil
		}
		return nil, fmt.Errorf("mapping changed: %w", warmstorage.ErrNotFound)
	}
	ctx := mustWritableProviderContext(t, &fakePDPProviderClient{}, WithFWSSDataSetReader(reader))

	_, found, err := ctx.FindDataSetByClientDataSetID(context.Background(), clientDataSetID)
	if err != nil || found {
		t.Fatalf("Find found=%t error=%v want unknown after state change", found, err)
	}
	if reader.findCalls != 2 {
		t.Fatalf("reader calls=%d want 2", reader.findCalls)
	}
}

func TestProviderContextFindDataSetByClientDataSetIDPropagatesReadErrorsAndCancellation(t *testing.T) {
	t.Run("read error", func(t *testing.T) {
		readErr := errors.New("rpc unavailable")
		reader := &fakeFWSSDataSetReader{findErr: readErr}
		ctx := mustWritableProviderContext(t, &fakePDPProviderClient{}, WithFWSSDataSetReader(reader))

		_, _, err := ctx.FindDataSetByClientDataSetID(context.Background(), types.NewBigInt(7))
		if !errors.Is(err, readErr) {
			t.Fatalf("error=%v want wrapped read error", err)
		}
	})

	t.Run("pre-canceled context", func(t *testing.T) {
		reader := &fakeFWSSDataSetReader{}
		ctx := mustWritableProviderContext(t, &fakePDPProviderClient{}, WithFWSSDataSetReader(reader))
		canceled, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := ctx.FindDataSetByClientDataSetID(canceled, types.NewBigInt(7))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v want context.Canceled", err)
		}
		if reader.findCalls != 0 {
			t.Fatalf("reader calls=%d want 0", reader.findCalls)
		}
	})
}

func matchingRecoveryDataSetInfo(provider Provider, dataSetID, clientDataSetID types.BigInt) *warmstorage.DataSetInfo {
	return &warmstorage.DataSetInfo{
		DataSetID:       dataSetID,
		Payer:           testPayer(),
		ServiceProvider: provider.ServiceProvider,
		ClientDataSetID: clientDataSetID,
		ProviderID:      provider.ID,
	}
}
