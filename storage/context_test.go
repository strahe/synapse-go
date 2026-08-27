package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/ipfs/go-cid"

	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func mustPieceInfo(t *testing.T) piece.PieceInfo {
	t.Helper()
	info, err := piece.CalculateFromBytes(bytes.Repeat([]byte("pi"), 128))
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	return info
}

func mustTestSigner(t *testing.T) signer.EVMSigner {
	t.Helper()
	key, err := ethcrypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	s, err := signer.NewSecp256k1Signer(key)
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	return s
}

func recoverRawTypedDataSigner(t *testing.T, domain apitypes.TypedDataDomain, primaryType string, message apitypes.TypedDataMessage, signature []byte) common.Address {
	t.Helper()
	typedData := apitypes.TypedData{
		Types:       ityped.Types,
		PrimaryType: primaryType,
		Domain:      domain,
		Message:     message,
	}
	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		t.Fatalf("hash EIP-712 domain: %v", err)
	}
	messageHash, err := typedData.HashStruct(primaryType, message)
	if err != nil {
		t.Fatalf("hash %s: %v", primaryType, err)
	}
	digest := ethcrypto.Keccak256(append(append([]byte{0x19, 0x01}, domainSeparator...), messageHash...))
	if len(signature) != 65 {
		t.Fatalf("signature length=%d want 65", len(signature))
	}
	recoverySignature := append([]byte(nil), signature...)
	if recoverySignature[64] >= 27 {
		recoverySignature[64] -= 27
	}
	publicKey, err := ethcrypto.SigToPub(digest, recoverySignature)
	if err != nil {
		t.Fatalf("recover %s signer: %v", primaryType, err)
	}
	return ethcrypto.PubkeyToAddress(*publicKey)
}

func decodedMetadataEntries(keys, values []string) []ityped.MetadataEntry {
	entries := make([]ityped.MetadataEntry, len(keys))
	for i := range keys {
		entries[i] = ityped.MetadataEntry{Key: keys[i], Value: values[i]}
	}
	return entries
}

type fakeDataSetValidator struct {
	err   error
	calls []types.BigInt
}

func (v *fakeDataSetValidator) ValidateDataSet(_ context.Context, dataSetID types.BigInt) error {
	v.calls = append(v.calls, copyBigInt(dataSetID))
	return v.err
}

func testProvider() Provider {
	return Provider{
		ID:              types.NewBigInt(1),
		ServiceURL:      "https://sp.example.com",
		ServiceProvider: common.HexToAddress("0x1001"),
		Payee:           common.HexToAddress("0x2002"),
	}
}

func testPayer() common.Address {
	return common.HexToAddress("0x3003")
}

func testRecordKeeper() common.Address {
	return common.HexToAddress("0x4004")
}

func testDataSetRef(dataSetID, clientDataSetID types.BigInt) DataSetRef {
	ref, _ := NewDataSetRef(testProvider().ID, dataSetID, clientDataSetID)
	return ref
}

func mustProviderContext(t *testing.T, client PDPProviderClient, opts ...ContextOption) *ProviderContext {
	t.Helper()
	c, err := NewProviderContext(testProvider(), client, mustTestSigner(t), opts...)
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	return c
}

func mustWritableProviderContext(t *testing.T, client PDPProviderClient, opts ...ContextOption) *ProviderContext {
	t.Helper()
	base := []ContextOption{
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	}
	return mustProviderContext(t, client, append(base, opts...)...)
}

func mustDataSetContext(t *testing.T, client PDPProviderClient, ref DataSetRef, opts ...ContextOption) *DataSetContext {
	t.Helper()
	c, err := NewDataSetContext(testProvider(), client, mustTestSigner(t), ref, opts...)
	if err != nil {
		t.Fatalf("NewDataSetContext: %v", err)
	}
	return c
}

func mustWritableDataSetContext(t *testing.T, client PDPProviderClient, ref DataSetRef, opts ...ContextOption) *DataSetContext {
	t.Helper()
	base := []ContextOption{
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	}
	return mustDataSetContext(t, client, ref, append(base, opts...)...)
}

func newTestContextForSelection(t *testing.T, provider Provider, factoryOpts ContextFactoryOptions, client PDPProviderClient, opts ...ContextOption) (*ProviderContext, error) {
	t.Helper()
	opts = append(opts, WithDataSetMetadata(factoryOpts.DataSetMetadata), WithCDN(factoryOpts.WithCDN))
	return NewProviderContext(provider, client, mustTestSigner(t), opts...)
}

type fakePDPProviderClient struct {
	uploadStreamingFn     func(context.Context, io.Reader, pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error)
	downloadPieceFn       func(context.Context, cid.Cid) (io.ReadCloser, int64, error)
	waitForPieceFn        func(context.Context, cid.Cid, time.Duration) error
	pullPiecesFn          func(context.Context, pdp.PullRequest) (*pdp.PullResult, error)
	pullPiecesFnWithCb    func(context.Context, pdp.PullRequest, func(*pdp.PullResult)) (*pdp.PullResult, error)
	addPiecesFn           func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error)
	getAddedFn            func(context.Context, string) (*pdp.AddPiecesStatus, error)
	waitForAddedFn        func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error)
	createDataSetFn       func(context.Context, common.Address, []byte) (*pdp.CreateDataSetResult, error)
	getCreatedFn          func(context.Context, string) (*pdp.CreateDataSetStatus, error)
	waitForCreatedFn      func(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error)
	createAndAddFn        func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error)
	getCreateAndAddFn     func(context.Context, string) (*pdp.CreateAndAddPiecesStatus, error)
	waitForCreateAndAddFn func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error)
	scheduleDeletionFn    func(context.Context, types.BigInt, types.BigInt, []byte) (common.Hash, error)
	terminateServiceFn    func(context.Context, pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error)
	waitTerminateFn       func(context.Context, types.BigInt, time.Duration, func(common.Hash)) (*pdp.TerminateServiceStatus, error)
}

func (f *fakePDPProviderClient) UploadPieceStreaming(ctx context.Context, r io.Reader, opts pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
	if f.uploadStreamingFn == nil {
		return nil, errors.New("unexpected UploadPieceStreaming")
	}
	return f.uploadStreamingFn(ctx, r, opts)
}

func (f *fakePDPProviderClient) DownloadPiece(ctx context.Context, pieceCID cid.Cid) (io.ReadCloser, int64, error) {
	if f.downloadPieceFn == nil {
		return nil, 0, errors.New("unexpected DownloadPiece")
	}
	return f.downloadPieceFn(ctx, pieceCID)
}

func (f *fakePDPProviderClient) WaitForPieceParked(ctx context.Context, pieceCID cid.Cid, pollInterval time.Duration) error {
	if f.waitForPieceFn == nil {
		return errors.New("unexpected WaitForPieceParked")
	}
	return f.waitForPieceFn(ctx, pieceCID, pollInterval)
}

func (f *fakePDPProviderClient) WaitForPullComplete(ctx context.Context, req pdp.PullRequest, pollInterval time.Duration, cb func(*pdp.PullResult)) (*pdp.PullResult, error) {
	if f.pullPiecesFnWithCb != nil {
		return f.pullPiecesFnWithCb(ctx, req, cb)
	}
	if f.pullPiecesFn == nil {
		return nil, errors.New("unexpected WaitForPullComplete")
	}
	return f.pullPiecesFn(ctx, req)
}

func (f *fakePDPProviderClient) AddPieces(ctx context.Context, dataSetID types.BigInt, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.AddPiecesResult, error) {
	if f.addPiecesFn == nil {
		return nil, errors.New("unexpected AddPieces")
	}
	return f.addPiecesFn(ctx, dataSetID, pieces, extraData)
}

func (f *fakePDPProviderClient) GetAddPiecesStatus(ctx context.Context, statusURL string) (*pdp.AddPiecesStatus, error) {
	if f.getAddedFn != nil {
		return f.getAddedFn(ctx, statusURL)
	}
	if f.waitForAddedFn != nil {
		status, err := f.waitForAddedFn(ctx, statusURL, 0)
		if status != nil {
			status = copyTestAddPiecesStatus(status)
			if status.PiecesAdded && status.PieceCount == 0 {
				status.PieceCount = len(status.ConfirmedPieceIDs)
			}
		}
		return status, err
	}
	return nil, errors.New("unexpected GetAddPiecesStatus")
}

func (f *fakePDPProviderClient) WaitForPiecesAdded(ctx context.Context, statusURL string, pollInterval time.Duration) (*pdp.AddPiecesStatus, error) {
	if f.waitForAddedFn == nil {
		return nil, errors.New("unexpected WaitForPiecesAdded")
	}
	return f.waitForAddedFn(ctx, statusURL, pollInterval)
}

func (f *fakePDPProviderClient) CreateDataSet(ctx context.Context, recordKeeper common.Address, extraData []byte) (*pdp.CreateDataSetResult, error) {
	if f.createDataSetFn == nil {
		return nil, errors.New("unexpected CreateDataSet")
	}
	return f.createDataSetFn(ctx, recordKeeper, extraData)
}

func (f *fakePDPProviderClient) GetDataSetCreationStatus(ctx context.Context, statusURL string) (*pdp.CreateDataSetStatus, error) {
	if f.getCreatedFn != nil {
		return f.getCreatedFn(ctx, statusURL)
	}
	if f.waitForCreatedFn != nil {
		return f.waitForCreatedFn(ctx, statusURL, 0)
	}
	return nil, errors.New("unexpected GetDataSetCreationStatus")
}

func (f *fakePDPProviderClient) WaitForDataSetCreated(ctx context.Context, statusURL string, pollInterval time.Duration) (*pdp.CreateDataSetStatus, error) {
	if f.waitForCreatedFn == nil {
		return nil, errors.New("unexpected WaitForDataSetCreated")
	}
	return f.waitForCreatedFn(ctx, statusURL, pollInterval)
}

func (f *fakePDPProviderClient) CreateDataSetAndAddPieces(ctx context.Context, recordKeeper common.Address, pieces []pdp.AddPieceInput, extraData []byte) (*pdp.CreateDataSetResult, error) {
	if f.createAndAddFn == nil {
		return nil, errors.New("unexpected CreateDataSetAndAddPieces")
	}
	return f.createAndAddFn(ctx, recordKeeper, pieces, extraData)
}

func (f *fakePDPProviderClient) GetCreateDataSetAndAddPiecesStatus(ctx context.Context, statusURL string) (*pdp.CreateAndAddPiecesStatus, error) {
	if f.getCreateAndAddFn != nil {
		return f.getCreateAndAddFn(ctx, statusURL)
	}
	if f.waitForCreateAndAddFn != nil {
		add, err := f.waitForCreateAndAddFn(ctx, statusURL, 0)
		if add == nil {
			return nil, err
		}
		add = copyTestAddPiecesStatus(add)
		if add.PiecesAdded && add.PieceCount == 0 {
			add.PieceCount = len(add.ConfirmedPieceIDs)
		}
		dataSetID := add.DataSetID.Copy()
		return &pdp.CreateAndAddPiecesStatus{
			Create: &pdp.CreateDataSetStatus{
				CreateMessageHash: add.TxHash,
				TxStatus:          "confirmed",
				DataSetCreated:    true,
				OK:                new(true),
				DataSetID:         &dataSetID,
			},
			Add: add,
		}, err
	}
	return nil, errors.New("unexpected GetCreateDataSetAndAddPiecesStatus")
}

func copyTestAddPiecesStatus(in *pdp.AddPiecesStatus) *pdp.AddPiecesStatus {
	out := *in
	out.DataSetID = in.DataSetID.Copy()
	out.ConfirmedPieceIDs = copyBigInts(in.ConfirmedPieceIDs)
	return &out
}

func (f *fakePDPProviderClient) WaitForCreateDataSetAndAddPieces(ctx context.Context, statusURL string, pollInterval time.Duration) (*pdp.AddPiecesStatus, error) {
	if f.waitForCreateAndAddFn == nil {
		return nil, errors.New("unexpected WaitForCreateDataSetAndAddPieces")
	}
	return f.waitForCreateAndAddFn(ctx, statusURL, pollInterval)
}

func (f *fakePDPProviderClient) SchedulePieceDeletion(ctx context.Context, dataSetID, pieceID types.BigInt, extraData []byte) (common.Hash, error) {
	if f.scheduleDeletionFn == nil {
		return common.Hash{}, errors.New("unexpected SchedulePieceDeletion")
	}
	return f.scheduleDeletionFn(ctx, dataSetID, pieceID, extraData)
}

func (f *fakePDPProviderClient) TerminateService(ctx context.Context, req pdp.TerminateServiceRequest) (*pdp.TerminateServiceResult, error) {
	if f.terminateServiceFn == nil {
		return nil, errors.New("unexpected TerminateService")
	}
	return f.terminateServiceFn(ctx, req)
}

func (f *fakePDPProviderClient) WaitForTerminateService(ctx context.Context, dataSetID types.BigInt, pollInterval time.Duration, onHash func(common.Hash)) (*pdp.TerminateServiceStatus, error) {
	if f.waitTerminateFn == nil {
		return nil, errors.New("unexpected WaitForTerminateService")
	}
	return f.waitTerminateFn(ctx, dataSetID, pollInterval, onHash)
}

func TestDataSetContextConstructionAndDefensiveCopies(t *testing.T) {
	provider := testProvider()
	metadata := map[string]string{"job": "original"}
	providerCtx, err := NewProviderContext(provider, &fakePDPProviderClient{}, mustTestSigner(t), WithDataSetMetadata(metadata))
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	provider.ID = types.NewBigInt(99)
	metadata["job"] = "mutated"
	if !providerCtx.ProviderID().Equal(testProvider().ID) || providerCtx.core.dataSetMetadata["job"] != "original" {
		t.Fatal("NewProviderContext retained mutable constructor inputs")
	}
	providerInfo := providerCtx.GetProviderInfo()
	providerInfo.ID = types.NewBigInt(100)
	if !providerCtx.ProviderID().Equal(testProvider().ID) {
		t.Fatal("GetProviderInfo exposed mutable provider state")
	}
	if _, ok := providerCtx.DataSetRef(); ok {
		t.Fatal("ProviderContext unexpectedly reports a data-set target")
	}

	t.Run("provider mismatch", func(t *testing.T) {
		ref, err := NewDataSetRef(types.NewBigInt(2), types.NewBigInt(42), types.NewBigInt(7))
		if err != nil {
			t.Fatalf("NewDataSetRef: %v", err)
		}
		_, err = providerCtx.ForDataSet(ref)
		if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "does not match") {
			t.Fatalf("ForDataSet error=%v", err)
		}
	})

	t.Run("zero data set", func(t *testing.T) {
		_, err := providerCtx.ForDataSet(testDataSetRef(types.BigInt{}, types.NewBigInt(7)))
		if !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("ForDataSet error=%v, want ErrInvalidArgument", err)
		}
	})

	input := testDataSetRef(types.NewBigInt(42), types.BigInt{})
	dataSetCtx, err := providerCtx.ForDataSet(input)
	if err != nil {
		t.Fatalf("ForDataSet: %v", err)
	}
	input.providerID = types.NewBigInt(98)
	input.dataSetID = types.NewBigInt(99)
	input.clientDataSetID = types.NewBigInt(97)
	ref, ok := dataSetCtx.DataSetRef()
	if !ok || !ref.ProviderID().Equal(testProvider().ID) || !ref.DataSetID().Equal(types.NewBigInt(42)) || !ref.ClientDataSetID().IsZero() {
		t.Fatalf("DataSetRef()=(%+v, %t), want dataSetID 42 and legal zero clientDataSetID", ref, ok)
	}
	ref.providerID = types.NewBigInt(101)
	ref.dataSetID = types.NewBigInt(100)
	ref.clientDataSetID = types.NewBigInt(102)
	again, _ := dataSetCtx.DataSetRef()
	if !again.ProviderID().Equal(testProvider().ID) || !again.DataSetID().Equal(types.NewBigInt(42)) || !again.ClientDataSetID().IsZero() {
		t.Fatalf("DataSetRef mutated through returned copy: %+v", again)
	}
	if _, ok := providerCtx.DataSetRef(); ok {
		t.Fatal("ForDataSet mutated its ProviderContext receiver")
	}

	logged := mustProviderContext(t, &fakePDPProviderClient{}, WithLogger(slog.New(slog.DiscardHandler)), WithCDN(true))
	dataSetLogged, err := logged.ForDataSet(testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)))
	if err != nil {
		t.Fatalf("ForDataSet: %v", err)
	}
	if logged.ServiceURL() != testProvider().ServiceURL || dataSetLogged.ServiceURL() != testProvider().ServiceURL {
		t.Fatalf("ServiceURL provider=%q data-set=%q", logged.ServiceURL(), dataSetLogged.ServiceURL())
	}
	if !logged.CDNEnabled() || !dataSetLogged.CDNEnabled() || !logged.WithCDN() || !dataSetLogged.WithCDN() {
		t.Fatal("CDNEnabled/WithCDN mismatch")
	}
	if dataSetLogged.GetProviderInfo().ServiceURL != testProvider().ServiceURL {
		t.Fatalf("GetProviderInfo=%+v", dataSetLogged.GetProviderInfo())
	}
	if !dataSetLogged.ClientDataSetID().Equal(types.NewBigInt(7)) || !dataSetLogged.DataSetID().Equal(types.NewBigInt(42)) {
		t.Fatalf("DataSetContext IDs dataSet=%s client=%s", dataSetLogged.DataSetID(), dataSetLogged.ClientDataSetID())
	}
	pieceCID := mustPieceInfo(t).CIDv2
	if logged.PieceURL(pieceCID) == "" || dataSetLogged.PieceURL(pieceCID) == "" {
		t.Fatal("PieceURL empty")
	}
}

func TestProviderContextForDataSetConcurrent(t *testing.T) {
	providerCtx := mustProviderContext(t, &fakePDPProviderClient{})
	const count = 32
	results := make(chan *DataSetContext, count)
	errs := make(chan error, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx, err := providerCtx.ForDataSet(testDataSetRef(types.NewBigInt(uint64(i+1)), types.NewBigInt(uint64(i))))
			results <- ctx
			errs <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("ForDataSet: %v", err)
		}
	}
	for result := range results {
		if result == nil {
			t.Fatal("ForDataSet returned nil context")
		}
	}
	if _, ok := providerCtx.DataSetRef(); ok {
		t.Fatal("concurrent ForDataSet calls mutated the source context")
	}
}

func TestContextCommitRoutesByConcreteTypeAndPreservesExtraData(t *testing.T) {
	info := mustPieceInfo(t)
	dataSetID := types.NewBigInt(42)
	extraData := []byte{0x01, 0x02, 0x03}
	var addCalls, createCalls int
	client := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, got types.BigInt, _ []pdp.AddPieceInput, payload []byte) (*pdp.AddPiecesResult, error) {
			addCalls++
			if !got.Equal(dataSetID) {
				t.Fatalf("AddPieces dataSetID=%s want %s", got.String(), dataSetID.String())
			}
			if !bytes.Equal(payload, extraData) {
				t.Fatalf("AddPieces extraData=%x want %x", payload, extraData)
			}
			payload[0] = 0xff
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x11"), StatusURL: "https://sp.example.com/add/1"}, nil
		},
		waitForAddedFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x11"),
				DataSetID:         dataSetID,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(8)},
			}, nil
		},
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			createCalls++
			return &pdp.CreateDataSetResult{TxHash: common.HexToHash("0x22"), StatusURL: "https://sp.example.com/create/1"}, nil
		},
		waitForCreateAndAddFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x22"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(9)},
			}, nil
		},
	}

	dataSetCtx := mustWritableDataSetContext(t, client, testDataSetRef(dataSetID, types.NewBigInt(7)))
	result, err := dataSetCtx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: extraData,
	})
	if err != nil {
		t.Fatalf("DataSetContext.Commit: %v", err)
	}
	if result.IsNewDataSet || !result.DataSet.DataSetID().Equal(dataSetID) || addCalls != 1 || createCalls != 0 {
		t.Fatalf("data-set result=%+v addCalls=%d createCalls=%d", result, addCalls, createCalls)
	}
	if !bytes.Equal(extraData, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("caller ExtraData was mutated: %x", extraData)
	}

	providerCtx := mustWritableProviderContext(t, client)
	providerExtraData, err := providerCtx.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	result, err = providerCtx.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: providerExtraData,
	})
	if err != nil {
		t.Fatalf("ProviderContext.Commit: %v", err)
	}
	if !result.IsNewDataSet || addCalls != 1 || createCalls != 1 {
		t.Fatalf("provider result=%+v addCalls=%d createCalls=%d", result, addCalls, createCalls)
	}
	if _, ok := providerCtx.DataSetRef(); ok {
		t.Fatal("successful create-and-add mutated ProviderContext")
	}
}

func TestDataSetContextCommitsEnterAddPiecesConcurrently(t *testing.T) {
	info := mustPieceInfo(t)
	dataSetID := types.NewBigInt(42)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	client := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, _ types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			entered <- struct{}{}
			<-release
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x11"), StatusURL: "https://sp.example.com/add"}, nil
		},
		waitForAddedFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x11"),
				DataSetID:         dataSetID,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)},
			}, nil
		},
	}
	c := mustWritableDataSetContext(t, client, testDataSetRef(dataSetID, types.NewBigInt(7)))
	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := c.Commit(context.Background(), CommitRequest{
				Pieces: []PieceInput{{PieceCID: info.CIDv2}},
			})
			errCh <- err
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("commits did not enter AddPieces concurrently")
		}
	}
	close(release)
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
}

func TestProviderContextConcurrentCommitsCreateIndependently(t *testing.T) {
	info := mustPieceInfo(t)
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	client := &fakePDPProviderClient{
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			entered <- struct{}{}
			<-release
			return &pdp.CreateDataSetResult{TxHash: common.HexToHash("0x22"), StatusURL: "https://sp.example.com/create"}, nil
		},
		waitForCreateAndAddFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x22"),
				DataSetID:         types.NewBigInt(55),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)},
			}, nil
		},
	}
	c := mustWritableProviderContext(t, client)
	errCh := make(chan error, 2)
	for range 2 {
		go func() {
			_, err := c.Commit(context.Background(), CommitRequest{
				Pieces: []PieceInput{{PieceCID: info.CIDv2}},
			})
			errCh <- err
		}()
	}
	for range 2 {
		select {
		case <-entered:
		case <-time.After(2 * time.Second):
			t.Fatal("commits did not create independently")
		}
	}
	close(release)
	for range 2 {
		if err := <-errCh; err != nil {
			t.Fatalf("Commit: %v", err)
		}
	}
	if _, ok := c.DataSetRef(); ok {
		t.Fatal("concurrent commits mutated ProviderContext")
	}
}

func TestProviderContextCreateDataSetReturnsRecoverableRefWithoutBinding(t *testing.T) {
	txHash := common.HexToHash("0x1234")
	confirmedTxHash := common.HexToHash("0x5678")
	dataSetID := types.NewBigInt(77)
	client := &fakePDPProviderClient{
		createDataSetFn: func(_ context.Context, recordKeeper common.Address, extraData []byte) (*pdp.CreateDataSetResult, error) {
			if recordKeeper != testRecordKeeper() || len(extraData) == 0 {
				t.Fatalf("CreateDataSet recordKeeper=%s extraData=%x", recordKeeper, extraData)
			}
			return &pdp.CreateDataSetResult{TxHash: txHash, StatusURL: "https://sp.example.com/create/1234"}, nil
		},
		waitForCreatedFn: func(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error) {
			id := copyBigInt(dataSetID)
			return &pdp.CreateDataSetStatus{CreateMessageHash: txHash, ConfirmedTxHash: confirmedTxHash, DataSetID: &id}, nil
		},
	}
	providerCtx := mustWritableProviderContext(t, client)
	var submission CreateDataSetSubmission
	result, err := providerCtx.CreateDataSet(context.Background(), &CreateDataSetOptions{
		OnSubmitted: func(got CreateDataSetSubmission) { submission = got },
	})
	if err != nil {
		t.Fatalf("CreateDataSet: %v", err)
	}
	if !submission.ProviderID.Equal(testProvider().ID) || submission.ClientDataSetID == nil {
		t.Fatalf("submission=%+v", submission)
	}
	if !result.DataSet.ProviderID().Equal(testProvider().ID) ||
		!result.DataSet.DataSetID().Equal(dataSetID) ||
		!result.DataSet.ClientDataSetID().Equal(*submission.ClientDataSetID) ||
		result.ConfirmedTransactionID != confirmedTxHash.Hex() {
		t.Fatalf("result=%+v submission=%+v", result, submission)
	}
	if _, ok := providerCtx.DataSetRef(); ok {
		t.Fatal("CreateDataSet mutated ProviderContext")
	}

	fresh := mustWritableProviderContext(t, client)
	recovered, err := fresh.WaitForDataSetCreated(context.Background(), submission)
	if err != nil {
		t.Fatalf("WaitForDataSetCreated: %v", err)
	}
	if !recovered.DataSet.ProviderID().Equal(result.DataSet.ProviderID()) ||
		!recovered.DataSet.DataSetID().Equal(result.DataSet.DataSetID()) ||
		!recovered.DataSet.ClientDataSetID().Equal(result.DataSet.ClientDataSetID()) {
		t.Fatalf("recovered ref=%+v want %+v", recovered.DataSet, result.DataSet)
	}
	if _, ok := fresh.DataSetRef(); ok {
		t.Fatal("WaitForDataSetCreated mutated fresh ProviderContext")
	}
	bound, err := fresh.ForDataSet(recovered.DataSet)
	if err != nil {
		t.Fatalf("ForDataSet: %v", err)
	}
	if ref, ok := bound.DataSetRef(); !ok ||
		!ref.ProviderID().Equal(recovered.DataSet.ProviderID()) ||
		!ref.DataSetID().Equal(recovered.DataSet.DataSetID()) ||
		!ref.ClientDataSetID().Equal(recovered.DataSet.ClientDataSetID()) {
		t.Fatalf("bound ref=(%+v, %t) want %+v", ref, ok, recovered.DataSet)
	}
}

func TestProviderContextWaitForDataSetCreatedRejectsWrongProvider(t *testing.T) {
	waitCalls := 0
	client := &fakePDPProviderClient{
		waitForCreatedFn: func(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error) {
			waitCalls++
			return nil, nil
		},
	}
	c := mustWritableProviderContext(t, client)
	clientID := types.NewBigInt(7)
	_, err := c.WaitForDataSetCreated(context.Background(), CreateDataSetSubmission{
		ProviderID:      types.NewBigInt(2),
		TransactionID:   common.HexToHash("0x1234").Hex(),
		StatusURL:       "https://sp.example.com/status",
		ClientDataSetID: &clientID,
	})
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "providerID") {
		t.Fatalf("WaitForDataSetCreated error=%v", err)
	}
	if waitCalls != 0 {
		t.Fatalf("waitCalls=%d want 0", waitCalls)
	}
}

func TestProviderContextWaitForDataSetCreatedAcceptsZeroProviderID(t *testing.T) {
	txHash := common.HexToHash("0x1234")
	dataSetID := types.NewBigInt(77)
	clientID := types.NewBigInt(7)
	waitCalls := 0
	client := &fakePDPProviderClient{
		waitForCreatedFn: func(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error) {
			waitCalls++
			id := copyBigInt(dataSetID)
			return &pdp.CreateDataSetStatus{CreateMessageHash: txHash, DataSetID: &id}, nil
		},
	}
	c := mustWritableProviderContext(t, client)
	result, err := c.WaitForDataSetCreated(context.Background(), CreateDataSetSubmission{
		TransactionID:   txHash.Hex(),
		StatusURL:       "https://sp.example.com/status",
		ClientDataSetID: &clientID,
	})
	if err != nil {
		t.Fatalf("WaitForDataSetCreated: %v", err)
	}
	if waitCalls != 1 {
		t.Fatalf("waitCalls=%d want 1", waitCalls)
	}
	if !result.DataSet.ProviderID().Equal(testProvider().ID) ||
		!result.DataSet.DataSetID().Equal(dataSetID) ||
		!result.DataSet.ClientDataSetID().Equal(clientID) {
		t.Fatalf("result=%+v", result.DataSet)
	}
}

func TestProviderContextWaitForDataSetCreatedRejectsInvalidSubmission(t *testing.T) {
	clientID := types.NewBigInt(7)
	valid := CreateDataSetSubmission{
		TransactionID:   common.HexToHash("0x1234").Hex(),
		StatusURL:       "https://sp.example.com/status",
		ClientDataSetID: &clientID,
	}
	tests := map[string]CreateDataSetSubmission{
		"empty transaction": {TransactionID: "", StatusURL: valid.StatusURL, ClientDataSetID: valid.ClientDataSetID},
		"short transaction": {TransactionID: "0xbeef", StatusURL: valid.StatusURL, ClientDataSetID: valid.ClientDataSetID},
		"zero transaction":  {TransactionID: common.Hash{}.Hex(), StatusURL: valid.StatusURL, ClientDataSetID: valid.ClientDataSetID},
		"empty status URL":  {TransactionID: valid.TransactionID, StatusURL: "", ClientDataSetID: valid.ClientDataSetID},
		"missing client ID": {TransactionID: valid.TransactionID, StatusURL: valid.StatusURL},
		"cross-origin status URL": {
			TransactionID:   valid.TransactionID,
			StatusURL:       "https://other.example/status",
			ClientDataSetID: valid.ClientDataSetID,
		},
	}
	for name, submission := range tests {
		t.Run(name, func(t *testing.T) {
			waitCalls := 0
			client := &fakePDPProviderClient{
				waitForCreatedFn: func(context.Context, string, time.Duration) (*pdp.CreateDataSetStatus, error) {
					waitCalls++
					return nil, errors.New("wait should not be called")
				},
			}
			c := mustWritableProviderContext(t, client)
			_, err := c.WaitForDataSetCreated(context.Background(), submission)
			if !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("WaitForDataSetCreated error=%v want ErrInvalidArgument", err)
			}
			if waitCalls != 0 {
				t.Fatalf("waitCalls=%d want 0", waitCalls)
			}
		})
	}
}

func TestDataSetContextCommitRejectsEndedAndValidatorFailuresBeforeAddPieces(t *testing.T) {
	info := mustPieceInfo(t)
	dataSetID := types.NewBigInt(42)
	want := errors.New("not live")

	t.Run("ended rail", func(t *testing.T) {
		addCalls := 0
		reader := &fakeFWSSDataSetReader{
			info: &warmstorage.DataSetInfo{DataSetID: dataSetID, PDPEndEpoch: 3778900},
		}
		client := &fakePDPProviderClient{
			addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
				addCalls++
				t.Fatal("AddPieces must not run for an ended data set")
				return nil, nil
			},
		}
		c := mustWritableDataSetContext(t, client, testDataSetRef(dataSetID, types.NewBigInt(7)), WithFWSSDataSetReader(reader))
		_, err := c.Commit(context.Background(), CommitRequest{Pieces: []PieceInput{{PieceCID: info.CIDv2}}})
		requireDataSetPDPPaymentTerminated(t, err, dataSetID, 3778900)
		if addCalls != 0 {
			t.Fatalf("addCalls=%d want 0", addCalls)
		}
		if reader.calls != 1 || !reader.gotID.Equal(dataSetID) {
			t.Fatalf("reader calls=%d gotID=%s", reader.calls, reader.gotID.String())
		}
	})

	t.Run("validator failure", func(t *testing.T) {
		addCalls := 0
		validator := &fakeDataSetValidator{err: want}
		client := &fakePDPProviderClient{
			addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
				addCalls++
				t.Fatal("AddPieces must not run after validator failure")
				return nil, nil
			},
		}
		c := mustWritableDataSetContext(t, client, testDataSetRef(dataSetID, types.NewBigInt(7)), WithDataSetValidator(validator))
		_, err := c.Commit(context.Background(), CommitRequest{Pieces: []PieceInput{{PieceCID: info.CIDv2}}})
		if !errors.Is(err, want) {
			t.Fatalf("Commit error=%v want %v", err, want)
		}
		if addCalls != 0 {
			t.Fatalf("addCalls=%d want 0", addCalls)
		}
		if len(validator.calls) != 1 || !validator.calls[0].Equal(dataSetID) {
			t.Fatalf("validator calls=%v", validator.calls)
		}
	})
}

func TestDataSetContextUploadRejectsEndedExistingDataSetBeforeStore(t *testing.T) {
	dataSetID := types.NewBigInt(13269)
	storeCalled := false
	reader := &fakeFWSSDataSetReader{
		info: &warmstorage.DataSetInfo{DataSetID: dataSetID, PDPEndEpoch: 3778900},
	}
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(context.Context, io.Reader, pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			storeCalled = true
			t.Fatal("Store must not run when the existing data set cannot accept uploads")
			return nil, nil
		},
	}
	c := mustWritableDataSetContext(t, client, testDataSetRef(dataSetID, types.NewBigInt(99)), WithFWSSDataSetReader(reader))
	_, err := c.Upload(context.Background(), bytes.NewReader([]byte("payload")), nil)
	requireDataSetPDPPaymentTerminated(t, err, dataSetID, 3778900)
	if storeCalled {
		t.Fatal("Store was called")
	}
	if reader.calls != 1 || !reader.gotID.Equal(dataSetID) {
		t.Fatalf("reader calls=%d gotID=%s", reader.calls, reader.gotID.String())
	}
}

func TestContextPullRoutesByConcreteType(t *testing.T) {
	info := mustPieceInfo(t)
	dataSetID := types.NewBigInt(42)
	var requests []pdp.PullRequest
	client := &fakePDPProviderClient{
		pullPiecesFn: func(_ context.Context, req pdp.PullRequest) (*pdp.PullResult, error) {
			requests = append(requests, req)
			return &pdp.PullResult{Status: pdp.PullStatusComplete}, nil
		},
	}
	request := PullRequest{
		Pieces: []cid.Cid{info.CIDv2},
		From:   func(cid.Cid) string { return "https://source.example.com/piece" },
	}
	providerCtx := mustProviderContext(t, client, WithRecordKeeper(testRecordKeeper()))
	if _, err := providerCtx.Pull(context.Background(), request); err != nil {
		t.Fatalf("ProviderContext.Pull: %v", err)
	}
	dataSetCtx := mustDataSetContext(t, client, testDataSetRef(dataSetID, types.NewBigInt(7)), WithRecordKeeper(testRecordKeeper()))
	if _, err := dataSetCtx.Pull(context.Background(), request); err != nil {
		t.Fatalf("DataSetContext.Pull: %v", err)
	}
	if len(requests) != 2 || requests[0].DataSetID != nil || requests[1].DataSetID == nil || !requests[1].DataSetID.Equal(dataSetID) {
		t.Fatalf("pull requests=%+v", requests)
	}
}

func TestContextPresignWireShapesRemainStable(t *testing.T) {
	info := mustPieceInfo(t)
	storageSigner := mustTestSigner(t)
	providerCtx, err := NewProviderContext(testProvider(), &fakePDPProviderClient{}, storageSigner,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
		WithCDN(true),
		WithDataSetMetadata(map[string]string{"z": "last", "a": "first"}),
	)
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}
	providerPayload, err := providerCtx.PresignForCommit(context.Background(), []PieceInput{{
		PieceCID:      info.CIDv2,
		PieceMetadata: map[string]string{"z": "last", "a": "first"},
	}})
	if err != nil {
		t.Fatalf("ProviderContext.PresignForCommit: %v", err)
	}
	outer, err := createAndAddArgs.Unpack(providerPayload)
	if err != nil {
		t.Fatalf("unpack create-and-add: %v", err)
	}
	if len(outer) != 2 || len(outer[0].([]byte)) == 0 || len(outer[1].([]byte)) == 0 {
		t.Fatalf("create-and-add payload=%v", outer)
	}
	createValues, err := createDataSetArgs.Unpack(outer[0].([]byte))
	if err != nil {
		t.Fatalf("unpack create payload: %v", err)
	}
	if createValues[0].(common.Address) != testPayer() || strings.Join(createValues[2].([]string), ",") != "a,withCDN,z" {
		t.Fatalf("create values=%v", createValues)
	}
	clientDataSetID := createValues[1].(*big.Int)
	domain := ityped.NewDomain(big.NewInt(314159), testRecordKeeper())
	createMessage := ityped.CreateDataSetMessage(
		clientDataSetID,
		testProvider().Payee,
		decodedMetadataEntries(createValues[2].([]string), createValues[3].([]string)),
	)
	if recovered := recoverRawTypedDataSigner(t, domain, "CreateDataSet", createMessage, createValues[4].([]byte)); recovered != storageSigner.EVMAddress() {
		t.Fatalf("CreateDataSet signer=%s want %s", recovered, storageSigner.EVMAddress())
	}
	providerAddValues, err := addPiecesArgs.Unpack(outer[1].([]byte))
	if err != nil {
		t.Fatalf("unpack provider add payload: %v", err)
	}
	pieceMetadata := [][]ityped.MetadataEntry{
		decodedMetadataEntries(providerAddValues[1].([][]string)[0], providerAddValues[2].([][]string)[0]),
	}
	addMessage, err := ityped.AddPiecesMessage(clientDataSetID, providerAddValues[0].(*big.Int), []cid.Cid{info.CIDv2}, pieceMetadata)
	if err != nil {
		t.Fatalf("build AddPieces message: %v", err)
	}
	if recovered := recoverRawTypedDataSigner(t, domain, "AddPieces", addMessage, providerAddValues[3].([]byte)); recovered != storageSigner.EVMAddress() {
		t.Fatalf("AddPieces signer=%s want %s", recovered, storageSigner.EVMAddress())
	}

	dataSetCtx, err := NewDataSetContext(
		testProvider(),
		&fakePDPProviderClient{},
		storageSigner,
		testDataSetRef(types.NewBigInt(42), types.NewBigInt(99)),
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewDataSetContext: %v", err)
	}
	addPayload, err := dataSetCtx.PresignForCommit(context.Background(), []PieceInput{{
		PieceCID:      info.CIDv2,
		PieceMetadata: map[string]string{"k": "v"},
	}})
	if err != nil {
		t.Fatalf("DataSetContext.PresignForCommit: %v", err)
	}
	addValues, err := addPiecesArgs.Unpack(addPayload)
	if err != nil {
		t.Fatalf("unpack add payload: %v", err)
	}
	if addValues[0].(*big.Int).Sign() == 0 || len(addValues[3].([]byte)) != 65 {
		t.Fatalf("add values=%v", addValues)
	}
	if keys := addValues[1].([][]string); len(keys) != 1 || len(keys[0]) != 1 || keys[0][0] != "k" {
		t.Fatalf("piece metadata keys=%v", keys)
	}

	bytesType, err := abi.NewType("bytes", "", nil)
	if err != nil || bytesArgs[0].Type.String() != bytesType.String() {
		t.Fatalf("bytes ABI type changed: %v", err)
	}
}

func TestContextPresignRejectsEquivalentDuplicatePieceCIDs(t *testing.T) {
	info := mustPieceInfo(t)
	pieces := []PieceInput{{PieceCID: info.CIDv1}, {PieceCID: info.CIDv2}}
	tests := []struct {
		name    string
		presign func(context.Context, []PieceInput) ([]byte, error)
	}{
		{
			name:    "provider context",
			presign: mustProviderContext(t, &fakePDPProviderClient{}).PresignForCommit,
		},
		{
			name: "data set context",
			presign: mustDataSetContext(
				t,
				&fakePDPProviderClient{},
				testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)),
			).PresignForCommit,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := test.presign(context.Background(), pieces); !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "duplicate pieceCID") {
				t.Fatalf("error=%v want duplicate ErrInvalidArgument", err)
			}
		})
	}
}

func TestProviderContextPresignUsesFullWidthClientDataSetID(t *testing.T) {
	oldReader := randReader
	t.Cleanup(func() { randReader = oldReader })
	random := bytes.Repeat([]byte{0xff}, 64)
	randReader = bytes.NewReader(random)

	info := mustPieceInfo(t)
	c := mustWritableProviderContext(t, &fakePDPProviderClient{})
	payload, err := c.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	outer, err := createAndAddArgs.Unpack(payload)
	if err != nil {
		t.Fatalf("unpack create-and-add: %v", err)
	}
	createValues, err := createDataSetArgs.Unpack(outer[0].([]byte))
	if err != nil {
		t.Fatalf("unpack create: %v", err)
	}
	clientDataSetID := createValues[1].(*big.Int)
	if clientDataSetID.BitLen() != 256 {
		t.Fatalf("clientDataSetID bit length=%d want 256", clientDataSetID.BitLen())
	}
}

func TestDataSetContextCommitPreservesUint256DataSetID(t *testing.T) {
	info := mustPieceInfo(t)
	large, err := types.BigIntFromBig(new(big.Int).Lsh(big.NewInt(1), 255))
	if err != nil {
		t.Fatalf("BigIntFromBig: %v", err)
	}
	client := &fakePDPProviderClient{
		addPiecesFn: func(_ context.Context, got types.BigInt, _ []pdp.AddPieceInput, _ []byte) (*pdp.AddPiecesResult, error) {
			if !got.Equal(large) {
				t.Fatalf("AddPieces dataSetID=%s want %s", got.String(), large.String())
			}
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x11"), StatusURL: "https://sp.example.com/add"}, nil
		},
		waitForAddedFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x11"),
				DataSetID:         large,
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)},
			}, nil
		},
	}
	c := mustWritableDataSetContext(t, client, testDataSetRef(large, types.NewBigInt(7)))
	result, err := c.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !result.DataSet.DataSetID().Equal(large) {
		t.Fatalf("result dataSetID=%s want %s", result.DataSet.DataSetID().String(), large.String())
	}
}

func TestContextStoreUploadsAndWaits(t *testing.T) {
	data := bytes.Repeat([]byte("store"), 64)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	client := &fakePDPProviderClient{
		uploadStreamingFn: func(_ context.Context, r io.Reader, _ pdp.UploadPieceStreamingOptions) (*pdp.UploadStreamingResult, error) {
			got, err := io.ReadAll(r)
			if err != nil || !bytes.Equal(got, data) {
				t.Fatalf("uploaded data=%x err=%v", got, err)
			}
			return &pdp.UploadStreamingResult{PieceCID: info.CIDv2, Size: int64(len(data))}, nil
		},
		waitForPieceFn: func(_ context.Context, got cid.Cid, _ time.Duration) error {
			if got != info.CIDv2 {
				t.Fatalf("WaitForPieceParked CID=%s want %s", got, info.CIDv2)
			}
			return nil
		},
	}
	for _, name := range []string{"provider", "data-set"} {
		t.Run(name, func(t *testing.T) {
			var (
				result *StoreResult
				err    error
			)
			switch name {
			case "provider":
				result, err = mustProviderContext(t, client).Store(context.Background(), bytes.NewReader(data), nil)
			default:
				result, err = mustDataSetContext(t, client, testDataSetRef(types.NewBigInt(42), types.NewBigInt(7))).Store(context.Background(), bytes.NewReader(data), nil)
			}
			if err != nil {
				t.Fatalf("Store: %v", err)
			}
			if result.PieceCID != info.CIDv2 || result.Size != int64(len(data)) {
				t.Fatalf("Store result=%+v", result)
			}
		})
	}
}

func TestContextValidationErrors(t *testing.T) {
	_, err := NewProviderContext(Provider{}, &fakePDPProviderClient{}, nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("zero provider error=%v", err)
	}
	_, err = NewProviderContext(testProvider(), nil, nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil client error=%v", err)
	}
	c := mustProviderContext(t, &fakePDPProviderClient{})
	_, err = c.Commit(context.Background(), CommitRequest{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty commit error=%v", err)
	}
	_, err = c.Pull(context.Background(), PullRequest{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty pull error=%v", err)
	}
	_, err = c.Store(context.Background(), nil, nil)
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil store error=%v", err)
	}
}

func TestContextPresignAndPullRejectInvalidInputs(t *testing.T) {
	info := mustPieceInfo(t)
	c := mustWritableProviderContext(t, &fakePDPProviderClient{})

	tooMany := make([]PieceInput, pdp.MaxAddPiecesBatchSize+1)
	for i := range tooMany {
		tooMany[i] = PieceInput{PieceCID: info.CIDv2}
	}
	_, err := c.PresignForCommit(context.Background(), tooMany)
	if !errors.Is(err, ErrInvalidArgument) || !errors.Is(err, pdp.ErrTooManyPieces) {
		t.Fatalf("too many pieces error=%v", err)
	}

	_, err = c.PresignForCommit(context.Background(), []PieceInput{{
		PieceCID:      info.CIDv2,
		PieceMetadata: map[string]string{strings.Repeat("k", maxMetadataKeyLength+1): "v"},
	}})
	if err == nil || !strings.Contains(err.Error(), "exceeds max length") {
		t.Fatalf("long metadata key error=%v", err)
	}

	_, err = c.PresignForCommit(context.Background(), []PieceInput{{}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("undefined pieceCID error=%v", err)
	}

	unsigned := mustProviderContext(t, &fakePDPProviderClient{})
	_, err = unsigned.PresignForCommit(context.Background(), []PieceInput{{PieceCID: info.CIDv2}})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("missing payer/chain/recordKeeper error=%v", err)
	}

	_, err = c.Pull(context.Background(), PullRequest{
		Pieces: []cid.Cid{info.CIDv2},
		From:   func(cid.Cid) string { return "" },
	})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("empty source URL error=%v", err)
	}
}

func TestNewProviderContextNormalizesTypedNilSigner(t *testing.T) {
	var typedNil *signer.Secp256k1Signer
	c, err := NewProviderContext(
		testProvider(),
		&fakePDPProviderClient{},
		typedNil,
		WithPayer(testPayer()),
		WithRecordKeeper(testRecordKeeper()),
		WithChainID(types.ChainID(314159)),
	)
	if err != nil {
		t.Fatalf("NewProviderContext: %v", err)
	}

	_, err = c.PresignForCommit(context.Background(), []PieceInput{{PieceCID: mustPieceInfo(t).CIDv2}})
	if !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "nil signer") {
		t.Fatalf("PresignForCommit error=%v want nil signer ErrInvalidArgument", err)
	}
}

func TestDataSetContextCommitRejectsMismatchedConfirmation(t *testing.T) {
	info := mustPieceInfo(t)
	client := &fakePDPProviderClient{
		addPiecesFn: func(context.Context, types.BigInt, []pdp.AddPieceInput, []byte) (*pdp.AddPiecesResult, error) {
			return &pdp.AddPiecesResult{TxHash: common.HexToHash("0x11"), StatusURL: "https://sp.example.com/add"}, nil
		},
		waitForAddedFn: func(context.Context, string, time.Duration) (*pdp.AddPiecesStatus, error) {
			return &pdp.AddPiecesStatus{
				TxHash:            common.HexToHash("0x11"),
				DataSetID:         types.NewBigInt(99),
				PiecesAdded:       true,
				ConfirmedPieceIDs: []types.BigInt{types.NewBigInt(1)},
			}, nil
		},
	}
	c := mustWritableDataSetContext(t, client, testDataSetRef(types.NewBigInt(42), types.NewBigInt(7)))
	_, err := c.Commit(context.Background(), CommitRequest{
		Pieces:    []PieceInput{{PieceCID: info.CIDv2}},
		ExtraData: []byte{0x01},
	})
	if !errors.Is(err, pdp.ErrInvalidStatus) {
		t.Fatalf("Commit error=%v", err)
	}
}

func TestContextErrorWrappingRetainsCause(t *testing.T) {
	boom := fmt.Errorf("provider unavailable")
	client := &fakePDPProviderClient{
		createAndAddFn: func(context.Context, common.Address, []pdp.AddPieceInput, []byte) (*pdp.CreateDataSetResult, error) {
			return nil, boom
		},
	}
	c := mustWritableProviderContext(t, client)
	_, err := c.Commit(context.Background(), CommitRequest{
		Pieces: []PieceInput{{PieceCID: mustPieceInfo(t).CIDv2}},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Commit error=%v want wrapped cause", err)
	}
}
