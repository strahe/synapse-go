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
	start := time.Now()
	t.Log("start storage staged CreateContexts")
	contexts, err := sm.CreateContexts(ctx, &storage.CreateContextsOptions{
		Copies:          2,
		DataSetMetadata: metadata,
		WithCDN:         &withCDN,
	})
	t.Logf("done storage staged CreateContexts elapsed=%s", time.Since(start).Round(time.Second))
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
			start := time.Now()
			t.Logf("start storage staged cleanup TerminateDataSet(%s)", id)
			_, err := sm.TerminateDataSet(cctx, id, &storage.TerminateDataSetOptions{
				WriteOptions: []warmstorage.WriteOption{warmstorage.WithWait(contextIntegrationTxWait)},
			})
			t.Logf("done storage staged cleanup TerminateDataSet(%s) elapsed=%s", id, time.Since(start).Round(time.Second))
			cancel()
			if err != nil {
				t.Logf("cleanup TerminateDataSet(%s): %v", id, err)
			}
		}
	})

	executePrepare := func(label string, prepare *storage.PrepareResult) {
		t.Helper()
		if prepare == nil || prepare.Transaction == nil {
			return
		}
		start := time.Now()
		t.Logf("start %s", label)
		res, err := prepare.Transaction.Execute(ctx, payments.WithWait(contextIntegrationTxWait))
		t.Logf("done %s elapsed=%s", label, time.Since(start).Round(time.Second))
		if err != nil {
			if errors.Is(err, payments.ErrPermitUnsupported) {
				t.Skip("needs-usdfc-permit-support: storage.Prepare funding requires permit support")
			}
			t.Fatalf("%s: %v", label, err)
		}
		if res.Receipt == nil || res.Receipt.Status != 1 {
			t.Fatalf("%s receipt = %+v", label, res.Receipt)
		}
	}

	start = time.Now()
	t.Log("start storage staged Prepare")
	prepare, err := sm.Prepare(ctx, &storage.PrepareOptions{
		DataSize: uint64(len(data)),
		Contexts: []storage.UploadContext{
			primary,
			secondary,
		},
	})
	t.Logf("done storage staged Prepare elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	executePrepare("Prepare.Execute", prepare)

	start = time.Now()
	t.Log("start storage staged primary Upload")
	primaryUpload, err := primary.Upload(ctx, bytes.NewReader(data), nil)
	t.Logf("done storage staged primary Upload elapsed=%s", time.Since(start).Round(time.Second))
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
	start = time.Now()
	t.Log("start storage staged secondary CreateDataSet")
	_, err = secondary.CreateDataSet(submitCtx, &storage.CreateDataSetOptions{
		OnSubmitted: func(s storage.CreateDataSetSubmission) {
			submission = s
			cancelSubmit()
		},
	})
	t.Logf("done storage staged secondary CreateDataSet elapsed=%s", time.Since(start).Round(time.Second))
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
	start = time.Now()
	t.Log("start storage staged WaitForDataSetCreated")
	created, err := recovered.WaitForDataSetCreated(ctx, submission)
	t.Logf("done storage staged WaitForDataSetCreated elapsed=%s", time.Since(start).Round(time.Second))
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
		t.Fatalf("recovered DataSetID = %v, want %s", got, created.DataSetID)
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
	start = time.Now()
	t.Log("start storage staged Pull")
	pull, err := recovered.Pull(ctx, storage.PullRequest{
		Pieces: []cid.Cid{primaryUpload.PieceCID},
		From: func(cid.Cid) string {
			return primaryCopy.RetrievalURL
		},
		ExtraData: extraData,
	})
	t.Logf("done storage staged Pull elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	if pull.Status != storage.PullStatusComplete {
		t.Fatalf("Pull status = %s, want %s", pull.Status, storage.PullStatusComplete)
	}
	if len(pull.Pieces) != 1 || pull.Pieces[0].Status != storage.PullStatusComplete {
		t.Fatalf("Pull pieces = %+v, want one complete piece", pull.Pieces)
	}

	start = time.Now()
	t.Log("start storage staged Prepare(commit)")
	commitPrepare, err := sm.Prepare(ctx, &storage.PrepareOptions{
		DataSize: uint64(len(data)),
		Contexts: []storage.UploadContext{
			recovered,
		},
	})
	t.Logf("done storage staged Prepare(commit) elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("Prepare(commit): %v", err)
	}
	executePrepare("Prepare(commit).Execute", commitPrepare)

	start = time.Now()
	t.Log("start storage staged Commit")
	commit, err := recovered.Commit(ctx, storage.CommitRequest{
		Pieces:    []storage.PieceInput{pieceInput},
		ExtraData: extraData,
	})
	t.Logf("done storage staged Commit elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !commit.DataSetID.Equal(created.DataSetID) {
		t.Fatalf("Commit DataSetID = %s, want %s", commit.DataSetID, created.DataSetID)
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
