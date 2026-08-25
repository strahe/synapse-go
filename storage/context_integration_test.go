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

	"github.com/ethereum/go-ethereum/common"
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

// TestIntegration_ContextCreateDataSetStagedFlow directly exercises the
// immutable storage context surface. Store, create/add, pull, delete and provider
// termination also provide real delegated coverage of the corresponding PDP
// HTTP writes. Direct FWSS termination and both terminated-rail settlement
// routes are asserted from their receipts and on-chain state. Direct termination
// retains a long lockup period, so an empty provider-terminated fixture supplies
// the second mature rail needed to exercise both settlement entry points.
func TestIntegration_ContextCreateDataSetStagedFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
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
	primary, ok := contexts[0].(*storage.ProviderContext)
	if !ok {
		t.Fatalf("primary context type = %T, want *storage.ProviderContext", contexts[0])
	}
	secondary, ok := contexts[1].(*storage.ProviderContext)
	if !ok {
		t.Fatalf("secondary context type = %T, want *storage.ProviderContext", contexts[1])
	}

	var cleanupIDs []types.BigInt
	terminatedIDs := make(map[string]struct{})
	t.Cleanup(func() {
		for _, id := range cleanupIDs {
			if _, terminated := terminatedIDs[id.String()]; terminated {
				continue
			}
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
		if prepare == nil {
			t.Fatalf("%s returned nil prepare result", label)
		}
		if prepare.Transaction == nil {
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
	t.Log("start storage staged primary Store")
	primaryStore, err := primary.Store(ctx, bytes.NewReader(data), nil)
	t.Logf("done storage staged primary Store elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("primary Store: %v", err)
	}
	if !primaryStore.PieceCID.Defined() {
		t.Fatal("primary Store returned undefined PieceCID")
	}
	if primaryStore.Size != int64(len(data)) {
		t.Fatalf("primary Store size = %d, want %d", primaryStore.Size, len(data))
	}
	primaryPiece := storage.PieceInput{PieceCID: primaryStore.PieceCID}
	primaryExtra, err := primary.PresignForCommit(ctx, []storage.PieceInput{primaryPiece})
	if err != nil {
		t.Fatalf("primary PresignForCommit: %v", err)
	}
	start = time.Now()
	t.Log("start storage staged primary Commit")
	primaryCommit, err := primary.Commit(ctx, storage.CommitRequest{
		Pieces:    []storage.PieceInput{primaryPiece},
		ExtraData: primaryExtra,
	})
	t.Logf("done storage staged primary Commit elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("primary Commit: %v", err)
	}
	if primaryCommit.DataSetID.IsZero() || !primaryCommit.IsNewDataSet {
		t.Fatalf("primary Commit = %+v, want a new non-zero data set", primaryCommit)
	}
	if len(primaryCommit.PieceIDs) != 1 {
		t.Fatalf("primary Commit PieceIDs = %d, want 1", len(primaryCommit.PieceIDs))
	}
	primaryURL := primary.PieceURL(primaryStore.PieceCID)
	if primaryURL == "" {
		t.Fatal("primary PieceURL returned empty URL")
	}
	cleanupIDs = append(cleanupIDs, primaryCommit.DataSetID)

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

	start = time.Now()
	t.Log("start storage staged WaitForDataSetCreated")
	created, err := secondary.WaitForDataSetCreated(ctx, submission)
	t.Logf("done storage staged WaitForDataSetCreated elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("WaitForDataSetCreated: %v", err)
	}
	if created.DataSet.DataSetID.IsZero() {
		t.Fatal("WaitForDataSetCreated returned zero DataSetID")
	}
	if !created.DataSet.ClientDataSetID.Equal(*submission.ClientDataSetID) {
		t.Fatalf("ClientDataSetID mismatch: got %v want %v", created.DataSet.ClientDataSetID, submission.ClientDataSetID)
	}
	recovered, err := secondary.ForDataSet(created.DataSet)
	if err != nil {
		t.Fatalf("ForDataSet(recovery): %v", err)
	}
	if got := recovered.DataSetID(); !got.Equal(created.DataSet.DataSetID) {
		t.Fatalf("recovered DataSetID = %v, want %s", got, created.DataSet.DataSetID)
	}
	cleanupIDs = append(cleanupIDs, created.DataSet.DataSetID)

	beforeCount, err := client.WarmStorage().GetActivePieceCount(ctx, created.DataSet.DataSetID)
	if err != nil {
		t.Fatalf("GetActivePieceCount(before): %v", err)
	}
	if beforeCount == nil || beforeCount.Sign() != 0 {
		t.Fatalf("active piece count before commit = %v, want 0", beforeCount)
	}
	beforeActive, err := client.WarmStorage().HasActivePieces(ctx, created.DataSet.DataSetID)
	if err != nil {
		t.Fatalf("HasActivePieces(before): %v", err)
	}
	if beforeActive {
		t.Fatal("HasActivePieces(before) = true, want false")
	}

	pieceInput := storage.PieceInput{PieceCID: primaryStore.PieceCID}
	extraData, err := recovered.PresignForCommit(ctx, []storage.PieceInput{pieceInput})
	if err != nil {
		t.Fatalf("PresignForCommit: %v", err)
	}
	start = time.Now()
	t.Log("start storage staged Pull")
	pull, err := recovered.Pull(ctx, storage.PullRequest{
		Pieces: []cid.Cid{primaryStore.PieceCID},
		From: func(cid.Cid) string {
			return primaryURL
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
	if !commit.DataSetID.Equal(created.DataSet.DataSetID) {
		t.Fatalf("Commit DataSetID = %s, want %s", commit.DataSetID, created.DataSet.DataSetID)
	}
	if commit.IsNewDataSet {
		t.Fatal("Commit unexpectedly used create-and-add path")
	}
	if len(commit.PieceIDs) != 1 {
		t.Fatalf("Commit PieceIDs = %d, want 1", len(commit.PieceIDs))
	}

	afterCount, err := client.WarmStorage().GetActivePieceCount(ctx, created.DataSet.DataSetID)
	if err != nil {
		t.Fatalf("GetActivePieceCount(after): %v", err)
	}
	wantAfter := new(big.Int).Add(beforeCount, big.NewInt(1))
	if afterCount == nil || afterCount.Cmp(wantAfter) != 0 {
		t.Fatalf("active piece count after commit = %v, want %v", afterCount, wantAfter)
	}
	afterActive, err := client.WarmStorage().HasActivePieces(ctx, created.DataSet.DataSetID)
	if err != nil {
		t.Fatalf("HasActivePieces(after): %v", err)
	}
	if !afterActive {
		t.Fatal("HasActivePieces(after) = false, want true")
	}

	t.Run("DeletePieceByID", func(t *testing.T) {
		start := time.Now()
		t.Log("start storage staged DeletePieceByID")
		deleted, err := recovered.DeletePieceByID(ctx, commit.PieceIDs[0])
		t.Logf("done storage staged DeletePieceByID elapsed=%s", time.Since(start).Round(time.Second))
		if err != nil {
			t.Fatalf("DeletePieceByID: %v", err)
		}
		if deleted == nil || deleted.Hash == (common.Hash{}) {
			t.Fatalf("DeletePieceByID result = %+v, want non-zero transaction hash", deleted)
		}

		deadline := time.Now().Add(3 * time.Minute)
		for {
			removals, err := recovered.GetScheduledRemovals(ctx)
			if err == nil {
				for _, removalID := range removals {
					if removalID.Equal(commit.PieceIDs[0]) {
						t.Logf("scheduled removal observed for piece %s", removalID)
						return
					}
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("scheduled removal for piece %s was not indexed within 3m (last error: %v)", commit.PieceIDs[0], err)
			}
			select {
			case <-ctx.Done():
				t.Fatalf("wait for scheduled removal: %v", ctx.Err())
			case <-time.After(2 * time.Second):
			}
		}
	})

	ws := client.WarmStorage()
	primaryInfo, err := ws.GetDataSet(ctx, primaryCommit.DataSetID)
	if err != nil {
		t.Fatalf("GetDataSet(primary before terminate): %v", err)
	}
	secondaryInfo, err := ws.GetDataSet(ctx, created.DataSet.DataSetID)
	if err != nil {
		t.Fatalf("GetDataSet(secondary before terminate): %v", err)
	}
	if primaryInfo.PDPRailID.IsZero() || secondaryInfo.PDPRailID.IsZero() {
		t.Fatalf("termination rails must be non-zero: primary=%s secondary=%s", primaryInfo.PDPRailID, secondaryInfo.PDPRailID)
	}
	primaryTarget, err := sm.CreateContext(ctx, &storage.CreateContextOptions{DataSetID: &primaryCommit.DataSetID})
	if err != nil {
		t.Fatalf("CreateContext(primary data set): %v", err)
	}
	primaryDataSet, ok := primaryTarget.(*storage.DataSetContext)
	if !ok {
		t.Fatalf("primary data-set context type = %T, want *storage.DataSetContext", primaryTarget)
	}

	start = time.Now()
	t.Log("start storage staged primary DataSetContext.Terminate")
	directTermination, err := primaryDataSet.Terminate(ctx, warmstorage.WithWait(contextIntegrationTxWait))
	t.Logf("done storage staged primary DataSetContext.Terminate elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("primary DataSetContext.Terminate: %v", err)
	}
	if directTermination == nil || directTermination.Receipt == nil || directTermination.Receipt.Status != 1 {
		t.Fatalf("primary DataSetContext.Terminate receipt = %+v", directTermination)
	}
	directEvent, err := warmstorage.ExtractPDPPaymentTerminatedEvent(directTermination.Receipt)
	if err != nil {
		t.Fatalf("ExtractPDPPaymentTerminatedEvent(primary): %v", err)
	}
	if !directEvent.DataSetID.Equal(primaryCommit.DataSetID) || !directEvent.PDPRailID.Equal(primaryInfo.PDPRailID) || directEvent.EndEpoch == 0 {
		t.Fatalf("primary termination event = %+v, want dataSet=%s rail=%s and non-zero end epoch", directEvent, primaryCommit.DataSetID, primaryInfo.PDPRailID)
	}
	terminatedIDs[primaryCommit.DataSetID.String()] = struct{}{}

	start = time.Now()
	t.Log("start storage staged secondary DataSetContext.TerminateService")
	providerTermination, err := recovered.TerminateService(ctx, &storage.TerminateServiceOptions{
		ProviderWaitTimeout: 5 * time.Minute,
		PollInterval:        2 * time.Second,
	})
	t.Logf("done storage staged secondary DataSetContext.TerminateService elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("secondary DataSetContext.TerminateService: %v", err)
	}
	if providerTermination == nil || providerTermination.TxHash == nil || providerTermination.DataSetID.IsZero() || providerTermination.EndEpoch == 0 {
		t.Fatalf("secondary DataSetContext.TerminateService result = %+v", providerTermination)
	}
	if !providerTermination.DataSetID.Equal(created.DataSet.DataSetID) {
		t.Fatalf("secondary termination DataSetID = %s, want %s", providerTermination.DataSetID, created.DataSet.DataSetID)
	}
	terminatedIDs[created.DataSet.DataSetID.String()] = struct{}{}

	settlementProviderID := secondary.ProviderID()
	settlementTarget, err := sm.CreateContext(ctx, &storage.CreateContextOptions{
		ProviderID:      &settlementProviderID,
		DataSetMetadata: map[string]string{"staged-settlement": metadata["staged"]},
		WithCDN:         &withCDN,
	})
	if err != nil {
		t.Fatalf("CreateContext(settlement fixture): %v", err)
	}
	settlementPrepare, err := sm.Prepare(ctx, &storage.PrepareOptions{
		DataSize: 1,
		Contexts: []storage.UploadContext{settlementTarget},
	})
	if err != nil {
		t.Fatalf("Prepare(settlement fixture): %v", err)
	}
	executePrepare("Prepare(settlement fixture).Execute", settlementPrepare)
	settlementProvider, ok := settlementTarget.(*storage.ProviderContext)
	if !ok {
		t.Fatalf("settlement context type = %T, want *storage.ProviderContext", settlementTarget)
	}

	start = time.Now()
	t.Log("start storage staged settlement fixture CreateDataSet")
	settlementCreated, err := settlementProvider.CreateDataSet(ctx, nil)
	t.Logf("done storage staged settlement fixture CreateDataSet elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("settlement fixture CreateDataSet: %v", err)
	}
	if settlementCreated == nil || settlementCreated.DataSet.DataSetID.IsZero() {
		t.Fatalf("settlement fixture CreateDataSet result = %+v", settlementCreated)
	}
	cleanupIDs = append(cleanupIDs, settlementCreated.DataSet.DataSetID)
	settlementDataSet, err := settlementProvider.ForDataSet(settlementCreated.DataSet)
	if err != nil {
		t.Fatalf("ForDataSet(settlement fixture): %v", err)
	}

	settlementInfo, err := ws.GetDataSet(ctx, settlementCreated.DataSet.DataSetID)
	if err != nil {
		t.Fatalf("GetDataSet(settlement fixture): %v", err)
	}
	if settlementInfo.PDPRailID.IsZero() {
		t.Fatal("settlement fixture has zero PDP rail ID")
	}

	start = time.Now()
	t.Log("start storage staged settlement fixture DataSetContext.TerminateService")
	settlementTermination, err := settlementDataSet.TerminateService(ctx, &storage.TerminateServiceOptions{
		ProviderWaitTimeout: 5 * time.Minute,
		PollInterval:        2 * time.Second,
	})
	t.Logf("done storage staged settlement fixture DataSetContext.TerminateService elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("settlement fixture DataSetContext.TerminateService: %v", err)
	}
	if settlementTermination == nil || settlementTermination.TxHash == nil || settlementTermination.EndEpoch == 0 ||
		!settlementTermination.DataSetID.Equal(settlementCreated.DataSet.DataSetID) {
		t.Fatalf("settlement fixture termination result = %+v", settlementTermination)
	}
	terminatedIDs[settlementCreated.DataSet.DataSetID.String()] = struct{}{}

	directRail, err := client.Payments().GetRail(ctx, primaryInfo.PDPRailID)
	if err != nil {
		t.Fatalf("GetRail(direct termination): %v", err)
	}
	if directRail.EndEpoch == nil {
		t.Fatalf("direct terminated rail has no end epoch: %+v", directRail)
	}
	if directRail.EndEpoch.Cmp(new(big.Int).SetUint64(uint64(directEvent.EndEpoch))) != 0 {
		t.Fatalf("direct rail end epoch = %s, termination event = %d", directRail.EndEpoch, directEvent.EndEpoch)
	}

	providerRail, err := client.Payments().GetRail(ctx, secondaryInfo.PDPRailID)
	if err != nil {
		t.Fatalf("GetRail(provider termination): %v", err)
	}
	if providerRail.EndEpoch == nil {
		t.Fatalf("provider terminated rail has no end epoch: %+v", providerRail)
	}
	if providerRail.EndEpoch.Cmp(new(big.Int).SetUint64(uint64(providerTermination.EndEpoch))) != 0 {
		t.Fatalf("provider rail end epoch = %s, termination result = %d", providerRail.EndEpoch, providerTermination.EndEpoch)
	}

	settlementRail, err := client.Payments().GetRail(ctx, settlementInfo.PDPRailID)
	if err != nil {
		t.Fatalf("GetRail(settlement fixture): %v", err)
	}
	if settlementRail.EndEpoch == nil || settlementRail.EndEpoch.Cmp(new(big.Int).SetUint64(uint64(settlementTermination.EndEpoch))) != 0 {
		t.Fatalf("settlement fixture rail end epoch = %v, termination result = %d", settlementRail.EndEpoch, settlementTermination.EndEpoch)
	}

	var summary *payments.AccountSummary
	maturityDeadline := time.Now().Add(3 * time.Minute)
	for {
		summary, err = client.Payments().AccountSummary(ctx, client.Address())
		if err != nil {
			t.Fatalf("AccountSummary(after termination): %v", err)
		}
		if summary == nil || summary.CurrentEpoch == nil {
			t.Fatalf("AccountSummary(after termination) returned no current epoch: %+v", summary)
		}
		if summary.CurrentEpoch.Cmp(providerRail.EndEpoch) > 0 && summary.CurrentEpoch.Cmp(settlementRail.EndEpoch) > 0 {
			break
		}
		if time.Now().After(maturityDeadline) {
			t.Fatalf(
				"provider-terminated rails did not mature within 3m: current=%s providerEnd=%s fixtureEnd=%s",
				summary.CurrentEpoch,
				providerRail.EndEpoch,
				settlementRail.EndEpoch,
			)
		}
		select {
		case <-ctx.Done():
			t.Fatalf("wait for provider-terminated rail maturity: %v", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
	if directRail.EndEpoch.Sign() <= 0 {
		t.Fatalf("direct terminated rail has invalid end epoch: %+v", directRail)
	}
	t.Logf("direct termination confirmed; settlement unlocks at epoch %s (current %s)", directRail.EndEpoch, summary.CurrentEpoch)

	assertMatureRail := func(label string, rail *payments.RailView) {
		t.Helper()
		if rail.From != client.Address() || rail.Token != client.ResolvedAddresses().USDFC ||
			rail.EndEpoch == nil || rail.SettledUpTo == nil || rail.PaymentRate == nil ||
			rail.PaymentRate.Sign() < 0 || rail.SettledUpTo.Cmp(rail.EndEpoch) >= 0 ||
			summary.CurrentEpoch.Cmp(rail.EndEpoch) <= 0 {
			t.Fatalf("%s is not a mature unsettled terminated rail: current=%s rail=%+v", label, summary.CurrentEpoch, rail)
		}
	}
	assertMatureRail("provider termination", providerRail)
	assertMatureRail("settlement fixture", settlementRail)

	directSettle, err := client.Payments().SettleTerminatedRail(
		ctx,
		secondaryInfo.PDPRailID,
		payments.WithWait(contextIntegrationTxWait),
	)
	if err != nil {
		t.Fatalf("SettleTerminatedRail(%s): %v", secondaryInfo.PDPRailID, err)
	}
	if directSettle == nil || directSettle.Receipt == nil || directSettle.Receipt.Status != 1 {
		t.Fatalf("SettleTerminatedRail(%s) receipt = %+v", secondaryInfo.PDPRailID, directSettle.Receipt)
	}
	t.Logf("SettleTerminatedRail succeeded for provider-terminated rail %s: tx=%s", secondaryInfo.PDPRailID, directSettle.Hash)

	autoSettle, err := client.Payments().SettleAuto(
		ctx,
		settlementInfo.PDPRailID,
		nil,
		payments.WithWait(contextIntegrationTxWait),
	)
	if err != nil {
		t.Fatalf("SettleAuto(terminated rail %s): %v", settlementInfo.PDPRailID, err)
	}
	if autoSettle == nil || autoSettle.Receipt == nil || autoSettle.Receipt.Status != 1 {
		t.Fatalf("SettleAuto(terminated rail %s) receipt = %+v", settlementInfo.PDPRailID, autoSettle.Receipt)
	}
	t.Logf("SettleAuto routed settlement fixture rail %s: tx=%s", settlementInfo.PDPRailID, autoSettle.Hash)
}
