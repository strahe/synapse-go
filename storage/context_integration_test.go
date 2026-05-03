//go:build integration

package storage_test

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"errors"
	"math/big"
	"strconv"
	"testing"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

const (
	contextIntegrationDataSize = 64 * 1024
	contextIntegrationTxWait   = 180 * time.Second
)

func TestIntegration_ContextCreateDataSetStagedFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	client := integrationtest.NewDefaultClient(t, ctx)
	sm := client.Storage()

	data := make([]byte, contextIntegrationDataSize)
	if _, err := crypto_rand.Read(data); err != nil {
		t.Fatalf("generate test data: %v", err)
	}

	withCDN := false
	metadata := map[string]string{
		"staged": strconv.FormatInt(time.Now().UnixNano(), 10),
	}
	contexts, err := sm.CreateContexts(ctx, &storage.CreateContextsOptions{
		Copies:          2,
		DataSetMetadata: metadata,
		WithCDN:         &withCDN,
	})
	if err != nil {
		t.Fatalf("CreateContexts: %v", err)
	}
	if len(contexts) < 2 {
		t.Skipf("need at least two storage contexts, got %d", len(contexts))
	}
	primary := contexts[0]
	secondary := contexts[1]

	var cleanupIDs []types.BigInt
	t.Cleanup(func() {
		for _, id := range cleanupIDs {
			cctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			_, err := sm.TerminateDataSet(cctx, id, &storage.TerminateDataSetOptions{
				WriteOptions: []warmstorage.WriteOption{warmstorage.WithWait(contextIntegrationTxWait)},
			})
			cancel()
			if err != nil {
				t.Logf("cleanup TerminateDataSet(%d): %v", id, err)
			}
		}
	})

	prepare, err := sm.Prepare(ctx, &storage.PrepareOptions{
		DataSize: uint64(len(data)),
		Contexts: []storage.UploadContext{
			primary,
			secondary,
		},
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prepare.Transaction != nil {
		res, err := prepare.Transaction.Execute(ctx, payments.WithWait(contextIntegrationTxWait))
		if err != nil {
			if errors.Is(err, payments.ErrPermitUnsupported) {
				t.Skip("needs-usdfc-permit-support: storage.Prepare funding requires permit support")
			}
			t.Fatalf("Prepare.Execute: %v", err)
		}
		if res.Receipt == nil || res.Receipt.Status != 1 {
			t.Fatalf("Prepare.Execute receipt = %+v", res.Receipt)
		}
	}

	primaryUpload, err := primary.Upload(ctx, bytes.NewReader(data), nil)
	if err != nil {
		t.Fatalf("primary Upload: %v", err)
	}
	if !primaryUpload.PieceCID.Defined() {
		t.Fatal("primary Upload returned undefined PieceCID")
	}
	if len(primaryUpload.Copies) != 1 {
		t.Fatalf("primary Upload copies = %d, want 1", len(primaryUpload.Copies))
	}
	primaryCopy := primaryUpload.Copies[0]
	if primaryCopy.DataSetID.IsZero() {
		t.Fatal("primary Upload returned zero DataSetID")
	}
	if primaryCopy.RetrievalURL == "" {
		t.Fatal("primary Upload returned empty RetrievalURL")
	}
	cleanupIDs = append(cleanupIDs, primaryCopy.DataSetID)

	submitCtx, cancelSubmit := context.WithCancel(ctx)
	defer cancelSubmit()
	var submission storage.CreateDataSetSubmission
	_, err = secondary.CreateDataSet(submitCtx, &storage.CreateDataSetOptions{
		OnSubmitted: func(s storage.CreateDataSetSubmission) {
			submission = s
			cancelSubmit()
		},
	})
	if err == nil {
		t.Fatal("CreateDataSet returned nil error after submit context cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CreateDataSet error = %v, want context.Canceled", err)
	}
	if submission.TransactionID == "" {
		t.Fatal("CreateDataSet submission missing TransactionID")
	}
	if submission.StatusURL == "" {
		t.Fatal("CreateDataSet submission missing StatusURL")
	}
	if submission.ClientDataSetID == nil || submission.ClientDataSetID.IsZero() {
		t.Fatal("CreateDataSet submission missing ClientDataSetID")
	}

	recovered, err := sm.CreateContext(ctx, &storage.CreateContextOptions{
		ProviderIDs:     []types.BigInt{secondary.ProviderID()},
		DataSetMetadata: metadata,
		WithCDN:         &withCDN,
	})
	if err != nil {
		t.Fatalf("CreateContext(recovery): %v", err)
	}
	created, err := recovered.WaitForDataSetCreated(ctx, submission)
	if err != nil {
		t.Fatalf("WaitForDataSetCreated: %v", err)
	}
	if created.DataSetID.IsZero() {
		t.Fatal("WaitForDataSetCreated returned zero DataSetID")
	}
	if !created.ClientDataSetID.Equal(*submission.ClientDataSetID) {
		t.Fatalf("ClientDataSetID mismatch: got %v want %v", created.ClientDataSetID, submission.ClientDataSetID)
	}
	if got := recovered.DataSetID(); got == nil || !got.Equal(created.DataSetID) {
		t.Fatalf("recovered DataSetID = %v, want %d", got, created.DataSetID)
	}
	cleanupIDs = append(cleanupIDs, created.DataSetID)

	beforeCount, err := client.WarmStorage().GetActivePieceCount(ctx, created.DataSetID)
	if err != nil {
		t.Fatalf("GetActivePieceCount(before): %v", err)
	}
	if beforeCount == nil || beforeCount.Sign() != 0 {
		t.Fatalf("active piece count before commit = %v, want 0", beforeCount)
	}

	pieceInput := storage.PieceInput{PieceCID: primaryUpload.PieceCID}
	extraData, err := recovered.PresignForCommit(ctx, []storage.PieceInput{pieceInput})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	pull, err := recovered.Pull(ctx, storage.PullRequest{
		Pieces: []cid.Cid{primaryUpload.PieceCID},
		From: func(cid.Cid) string {
			return primaryCopy.RetrievalURL
		},
		ExtraData: extraData,
	})
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if pull.Status != storage.PullStatusComplete {
		t.Fatalf("Pull status = %s, want %s", pull.Status, storage.PullStatusComplete)
	}
	if len(pull.Pieces) != 1 || pull.Pieces[0].Status != storage.PullStatusComplete {
		t.Fatalf("Pull pieces = %+v, want one complete piece", pull.Pieces)
	}

	commit, err := recovered.Commit(ctx, storage.CommitRequest{
		Pieces:    []storage.PieceInput{pieceInput},
		ExtraData: extraData,
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !commit.DataSetID.Equal(created.DataSetID) {
		t.Fatalf("Commit DataSetID = %d, want %d", commit.DataSetID, created.DataSetID)
	}
	if commit.IsNewDataSet {
		t.Fatal("Commit unexpectedly used create-and-add path")
	}
	if len(commit.PieceIDs) != 1 {
		t.Fatalf("Commit PieceIDs = %d, want 1", len(commit.PieceIDs))
	}

	afterCount, err := client.WarmStorage().GetActivePieceCount(ctx, created.DataSetID)
	if err != nil {
		t.Fatalf("GetActivePieceCount(after): %v", err)
	}
	wantAfter := new(big.Int).Add(beforeCount, big.NewInt(1))
	if afterCount == nil || afterCount.Cmp(wantAfter) != 0 {
		t.Fatalf("active piece count after commit = %v, want %v", afterCount, wantAfter)
	}
}
