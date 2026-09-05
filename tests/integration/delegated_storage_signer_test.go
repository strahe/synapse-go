//go:build integration

package integration_test_test

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/ipfs/go-cid"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/integrationtest"
	ityped "github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

const (
	delegatedTestDataSize             = 256 * 1024
	delegatedTxWaitTimeout            = 180 * time.Second
	delegatedFundingExtraRunwayEpochs = chain.EpochsPerDay
	delegatedFundingBufferEpochs      = 120
)

var (
	delegatedCreateDataSetArgs = mustDelegatedABIArguments("address", "uint256", "string[]", "string[]", "bytes")
	delegatedAddPiecesArgs     = mustDelegatedABIArguments("uint256", "string[][]", "string[][]", "bytes")
	delegatedCreateAndAddArgs  = mustDelegatedABIArguments("bytes", "bytes")
)

func mustDelegatedABIArguments(typeNames ...string) abi.Arguments {
	args := make(abi.Arguments, len(typeNames))
	for i, typeName := range typeNames {
		t, err := abi.NewType(typeName, "", nil)
		if err != nil {
			panic("integration: parse ABI type " + typeName + ": " + err.Error())
		}
		args[i] = abi.Argument{Type: t}
	}
	return args
}

func delegatedMetadataEntries(keys, values []string) []ityped.MetadataEntry {
	entries := make([]ityped.MetadataEntry, len(keys))
	for i := range keys {
		entries[i] = ityped.MetadataEntry{Key: keys[i], Value: values[i]}
	}
	return entries
}

func recoverDelegatedTypedDataSigner(t *testing.T, domain apitypes.TypedDataDomain, primaryType string, message apitypes.TypedDataMessage, signature []byte) common.Address {
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
	digest := crypto.Keccak256(append(append([]byte{0x19, 0x01}, domainSeparator...), messageHash...))
	if len(signature) != 65 {
		t.Fatalf("%s signature length=%d want 65", primaryType, len(signature))
	}
	recoverySignature := append([]byte(nil), signature...)
	if recoverySignature[64] >= 27 {
		recoverySignature[64] -= 27
	}
	publicKey, err := crypto.SigToPub(digest, recoverySignature)
	if err != nil {
		t.Fatalf("recover %s signer: %v", primaryType, err)
	}
	return crypto.PubkeyToAddress(*publicKey)
}

func recoverDelegatedPresigners(t *testing.T, client *synapse.Client, uploadCtx storage.StorageContext, pieceCID cid.Cid, extraData []byte) (common.Address, common.Address, common.Address) {
	t.Helper()
	outer, err := delegatedCreateAndAddArgs.Unpack(extraData)
	if err != nil {
		t.Fatalf("unpack create-and-add extraData: %v", err)
	}
	createValues, err := delegatedCreateDataSetArgs.Unpack(outer[0].([]byte))
	if err != nil {
		t.Fatalf("unpack CreateDataSet extraData: %v", err)
	}
	addValues, err := delegatedAddPiecesArgs.Unpack(outer[1].([]byte))
	if err != nil {
		t.Fatalf("unpack AddPieces extraData: %v", err)
	}

	clientDataSetID := createValues[1].(*big.Int)
	domain := ityped.NewDomain(big.NewInt(int64(client.Chain().ChainID())), client.ResolvedAddresses().FWSS)
	createMessage := ityped.CreateDataSetMessage(
		clientDataSetID,
		uploadCtx.GetProviderInfo().Payee,
		delegatedMetadataEntries(createValues[2].([]string), createValues[3].([]string)),
	)
	createSigner := recoverDelegatedTypedDataSigner(t, domain, "CreateDataSet", createMessage, createValues[4].([]byte))

	metadataKeys := addValues[1].([][]string)
	metadataValues := addValues[2].([][]string)
	pieceMetadata := make([][]ityped.MetadataEntry, len(metadataKeys))
	for i := range metadataKeys {
		pieceMetadata[i] = delegatedMetadataEntries(metadataKeys[i], metadataValues[i])
	}
	addMessage, err := ityped.AddPiecesMessage(
		clientDataSetID,
		addValues[0].(*big.Int),
		[]cid.Cid{pieceCID},
		pieceMetadata,
	)
	if err != nil {
		t.Fatalf("build AddPieces message: %v", err)
	}
	addSigner := recoverDelegatedTypedDataSigner(t, domain, "AddPieces", addMessage, addValues[3].([]byte))
	return createValues[0].(common.Address), createSigner, addSigner
}

func selectUnboundDelegatedContext(t *testing.T, ctx context.Context, client *synapse.Client, run string) storage.StorageContext {
	t.Helper()
	selection, err := client.Storage().SelectUploadContexts(ctx, storage.SelectUploadContextsOptions{
		Copies: 1,
		DataSetMetadata: map[string]string{
			"run": run,
		},
	})
	if err != nil {
		t.Fatalf("SelectUploadContexts: %v", err)
	}
	if selection == nil || len(selection.Contexts) != 1 || selection.Contexts[0] == nil {
		t.Fatalf("SelectUploadContexts returned %+v", selection)
	}
	uploadCtx := selection.Contexts[0]
	if _, bound := uploadCtx.DataSetRef(); bound {
		t.Fatal("selected upload context unexpectedly reused an existing data set")
	}
	return uploadCtx
}

func TestIntegration_DelegatedStorageSigner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	privateKeyHex := integrationtest.RequirePrivateKey(t)

	data := make([]byte, delegatedTestDataSize)
	if _, err := crypto_rand.Read(data); err != nil {
		t.Fatalf("generate upload payload: %v", err)
	}
	pieceInfo, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("calculate PieceCID: %v", err)
	}

	var typedNil *signer.Secp256k1Signer
	fallbackCases := []struct {
		name string
		opt  synapse.ClientOption
	}{
		{name: "omitted"},
		{name: "nil", opt: synapse.WithStorageSigner(nil)},
		{name: "typed-nil", opt: synapse.WithStorageSigner(typedNil)},
	}
	for _, tc := range fallbackCases {
		t.Run("fallback-"+tc.name, func(t *testing.T) {
			source := fmt.Sprintf("integration-delegated-storage-signer-%s-%d", tc.name, time.Now().UnixNano())
			opts := []synapse.ClientOption{synapse.WithSource(source)}
			if tc.opt != nil {
				opts = append(opts, tc.opt)
			}
			fallbackClient := integrationtest.NewClient(t, ctx, privateKeyHex, opts...)
			rootAddress := fallbackClient.Address()
			uploadCtx := selectUnboundDelegatedContext(t, ctx, fallbackClient, fmt.Sprintf("fallback-%d", time.Now().UnixNano()))
			extraData, err := uploadCtx.PresignForCommit(ctx, []storage.PieceInput{{PieceCID: pieceInfo.CIDv2}})
			if err != nil {
				t.Fatalf("PresignForCommit: %v", err)
			}
			payer, createSigner, addSigner := recoverDelegatedPresigners(t, fallbackClient, uploadCtx, pieceInfo.CIDv2, extraData)
			if payer != rootAddress || uploadCtx.ContextIdentity().Payer != rootAddress {
				t.Fatalf("payer payload=%s context=%s want root %s", payer, uploadCtx.ContextIdentity().Payer, rootAddress)
			}
			if createSigner != rootAddress || addSigner != rootAddress {
				t.Fatalf("fallback signers create=%s add=%s want root %s", createSigner, addSigner, rootAddress)
			}
		})
	}

	delegatedKey, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("generate delegated key: %v", err)
	}
	storageSigner, err := signer.NewSecp256k1Signer(delegatedKey)
	if err != nil {
		t.Fatalf("NewSecp256k1Signer: %v", err)
	}
	delegatedAddress := storageSigner.EVMAddress()
	source := fmt.Sprintf("integration-delegated-storage-signer-%d", time.Now().UnixNano())
	client := integrationtest.NewClient(t, ctx, privateKeyHex,
		synapse.WithStorageSigner(storageSigner),
		synapse.WithSource(source),
	)
	rootAddress := client.Address()
	if rootAddress == delegatedAddress {
		t.Fatalf("generated delegated signer unexpectedly matches root %s", rootAddress)
	}

	permissions := []sessionkey.Permission{
		sessionkey.CreateDataSetPermission(),
		sessionkey.AddPiecesPermission(),
	}
	login, err := client.SessionKey().LoginWithOptions(
		ctx,
		delegatedAddress,
		&sessionkey.LoginOptions{
			Permissions: permissions,
			ExpiresAt:   uint64(time.Now().Add(time.Hour).Unix()),
			Origin:      "integration-test",
		},
		sessionkey.WithWait(delegatedTxWaitTimeout),
	)
	if login != nil {
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cleanupCancel()
			result, revokeErr := client.SessionKey().RevokeWithOptions(
				cleanupCtx,
				delegatedAddress,
				&sessionkey.RevokeOptions{
					Permissions: permissions,
					Origin:      "integration-test",
				},
				sessionkey.WithWait(delegatedTxWaitTimeout),
			)
			if revokeErr != nil {
				t.Errorf("cleanup RevokeWithOptions(session=%s): %v", delegatedAddress, revokeErr)
				return
			}
			if result == nil || result.Receipt == nil || result.Receipt.Status != 1 {
				t.Errorf("cleanup RevokeWithOptions(session=%s) result=%+v", delegatedAddress, result)
			}
		})
	}
	if err != nil {
		t.Fatalf("LoginWithOptions: %v", err)
	}
	if login == nil || login.Receipt == nil || login.Receipt.Status != 1 {
		t.Fatalf("LoginWithOptions result=%+v", login)
	}

	uploadCtx := selectUnboundDelegatedContext(t, ctx, client, fmt.Sprintf("delegated-%d", time.Now().UnixNano()))
	if uploadCtx.ContextIdentity().Payer != rootAddress {
		t.Fatalf("context payer=%s want root %s", uploadCtx.ContextIdentity().Payer, rootAddress)
	}
	extraData, err := uploadCtx.PresignForCommit(ctx, []storage.PieceInput{{PieceCID: pieceInfo.CIDv2}})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	payer, createSigner, addSigner := recoverDelegatedPresigners(t, client, uploadCtx, pieceInfo.CIDv2, extraData)
	if payer != rootAddress {
		t.Fatalf("create payload payer=%s want root %s", payer, rootAddress)
	}
	if createSigner != delegatedAddress || addSigner != delegatedAddress {
		t.Fatalf("delegated signers create=%s add=%s want %s", createSigner, addSigner, delegatedAddress)
	}

	prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{
		DataSize:          uint64(len(data)),
		Contexts:          []storage.StorageContext{uploadCtx},
		ExtraRunwayEpochs: delegatedFundingExtraRunwayEpochs,
		BufferEpochs:      new(int64(delegatedFundingBufferEpochs)),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prep.Transaction != nil {
		prepareResult, err := prep.Transaction.Execute(ctx, payments.WithWait(delegatedTxWaitTimeout))
		if errors.Is(err, payments.ErrPermitUnsupported) {
			t.Skip("needs-usdfc-permit-support: delegated storage signer funding requires permit support")
		}
		if err != nil {
			t.Fatalf("Prepare.Execute: %v", err)
		}
		if prepareResult == nil || prepareResult.Receipt == nil || prepareResult.Receipt.Status != 1 {
			t.Fatalf("Prepare.Execute result=%+v", prepareResult)
		}
	}

	providerCtx, ok := uploadCtx.(*storage.ProviderContext)
	if !ok {
		t.Fatalf("selected unbound context has type %T, want *storage.ProviderContext", uploadCtx)
	}
	storeResult, err := providerCtx.Store(ctx, bytes.NewReader(data), &storage.StoreOptions{PieceCID: pieceInfo.CIDv2})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if storeResult == nil || !storeResult.PieceCID.Equals(pieceInfo.CIDv2) {
		t.Fatalf("Store PieceCID=%v want %s", storeResult, pieceInfo.CIDv2)
	}
	commitSubmission, err := providerCtx.SubmitCommit(ctx, storage.CommitRequest{
		Pieces: []storage.PieceInput{{PieceCID: storeResult.PieceCID}},
	})
	if err != nil {
		t.Fatalf("SubmitCommit: %v", err)
	}
	if commitSubmission == nil {
		t.Fatal("SubmitCommit returned nil submission")
	}
	var dataSetID types.BigInt
	t.Cleanup(func() {
		cleanupDataSetID := dataSetID
		if cleanupDataSetID.IsZero() {
			waitCtx, waitCancel := context.WithTimeout(context.Background(), 3*time.Minute)
			commitResult, waitErr := providerCtx.WaitForCommit(waitCtx, *commitSubmission)
			waitCancel()
			if waitErr != nil {
				t.Errorf("cleanup WaitForCommit(submission=%+v session=%s): %v", *commitSubmission, delegatedAddress, waitErr)
				return
			}
			if commitResult == nil || commitResult.DataSet.DataSetID().IsZero() {
				t.Errorf("cleanup WaitForCommit(submission=%+v session=%s) result=%+v", *commitSubmission, delegatedAddress, commitResult)
				return
			}
			cleanupDataSetID = commitResult.DataSet.DataSetID()
		}
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer terminateCancel()
		terminateResult, terminateErr := client.Storage().TerminateDataSet(terminateCtx, cleanupDataSetID, &storage.TerminateDataSetOptions{
			WriteOptions: []warmstorage.WriteOption{
				warmstorage.WithWait(delegatedTxWaitTimeout),
			},
		})
		if terminateErr != nil {
			t.Errorf("cleanup TerminateDataSet(dataset=%s session=%s): %v", cleanupDataSetID, delegatedAddress, terminateErr)
			return
		}
		if terminateResult == nil || terminateResult.Receipt == nil || terminateResult.Receipt.Status != 1 {
			t.Errorf("cleanup TerminateDataSet(dataset=%s session=%s) result=%+v", cleanupDataSetID, delegatedAddress, terminateResult)
		}
	})

	commitResult, err := providerCtx.WaitForCommit(ctx, *commitSubmission)
	if err != nil {
		t.Fatalf("WaitForCommit: %v", err)
	}
	if commitResult == nil || commitResult.DataSet.DataSetID().IsZero() {
		t.Fatalf("WaitForCommit returned no data set: %+v", commitResult)
	}
	dataSetID = commitResult.DataSet.DataSetID()
	if !commitResult.IsNewDataSet {
		t.Fatalf("WaitForCommit IsNewDataSet=false for data set %s", dataSetID)
	}
	dataSet, err := client.WarmStorage().GetDataSet(ctx, dataSetID)
	if err != nil {
		t.Fatalf("WarmStorage.GetDataSet(%s): %v", dataSetID, err)
	}
	if dataSet.Payer != rootAddress {
		t.Fatalf("on-chain payer=%s want root %s", dataSet.Payer, rootAddress)
	}
	t.Logf("delegated storage acceptance passed: dataset=%s payer=%s signer=%s", dataSetID, rootAddress, delegatedAddress)
}
