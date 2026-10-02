//go:build integration

package integration_test_test

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ipfs/go-cid"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

func TestIntegration_CompactDataSetBatching(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 24*time.Minute)
	defer cancel()
	client := integrationtest.NewDefaultClient(t, ctx, synapse.WithUploadBatching(storage.WithoutUploadIdleWait(), storage.WithoutUploadMaxWait()))
	cutoff := chain.Calibration.LegacyPieceStorageIDLimit()
	var legacy *storage.DataSetContext
	var metadata map[string]string
	for _, candidate := range []uint64{30264, 30265} {
		id := types.NewBigInt(candidate)
		info, err := client.WarmStorage().GetDataSet(ctx, id)
		if err != nil || info.Payer != client.Address() || info.PDPEndEpoch != 0 || id.Cmp(types.NewBigInt(cutoff)) >= 0 {
			t.Logf("fixture %d unavailable or not owned/writable", candidate)
			continue
		}
		meta, err := client.WarmStorage().GetAllDataSetMetadata(ctx, id)
		if err != nil {
			t.Logf("fixture %d metadata: %v", candidate, err)
			continue
		}
		marked := false
		for key, value := range meta {
			marked = marked || strings.Contains(strings.ToLower(key+value), "test") || strings.Contains(strings.ToLower(key+value), "integration")
		}
		if !marked {
			t.Logf("fixture %d lacks a test marker", candidate)
			continue
		}
		if err := client.WarmStorage().ValidateDataSet(ctx, id); err != nil {
			t.Logf("fixture %d validation: %v", candidate, err)
			continue
		}
		opened, err := client.Storage().NewDataSetContext(ctx, id, storage.NewDataSetContextOptions{})
		if err != nil {
			t.Logf("fixture %d context: %v", candidate, err)
			continue
		}
		httpClient, err := pdp.New(opened.ServiceURL())
		if err != nil {
			t.Fatal(err)
		}
		if err := httpClient.Ping(ctx); err != nil {
			t.Logf("fixture %d provider health: %v", candidate, err)
			continue
		}
		legacy, metadata = opened, meta
		break
	}
	if legacy == nil {
		t.Fatal("no owned, marked, live legacy fixture with a healthy provider; P0 acceptance blocked")
	}
	legacyRef, _ := legacy.DataSetRef()
	t.Logf("using legacy fixture=%s provider=%s cutoff=%d", legacyRef.DataSetID(), legacy.ProviderID(), cutoff)
	providers, err := client.SPRegistry().SelectActivePDPProviders(ctx, spregistry.ProviderFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var excluded []types.BigInt
	for _, provider := range providers {
		if !provider.Info.ID.Equal(legacy.ProviderID()) {
			excluded = append(excluded, provider.Info.ID)
		}
	}
	selectTarget := func() storage.StorageContext {
		selection, err := client.Storage().SelectUploadContexts(ctx, storage.SelectUploadContextsOptions{Copies: 1, DataSetMetadata: metadata, ExcludeProviderIDs: excluded, AllowUnendorsedPrimary: true})
		if err != nil || !selection.Complete || len(selection.Contexts) != 1 {
			t.Fatalf("controlled selection=%+v err=%v", selection, err)
		}
		return selection.Contexts[0]
	}
	fallback := selectTarget()
	fallbackRef, bound := fallback.DataSetRef()
	if !bound || !fallbackRef.Equal(legacyRef) {
		t.Fatalf("legacy fallback=%+v want=%+v", fallbackRef, legacyRef)
	}
	withCDN := false
	if _, ok := metadata["withCDN"]; ok {
		withCDN = true
		delete(metadata, "withCDN")
	}
	provider, err := client.Storage().NewProviderContext(ctx, legacy.ProviderID(), storage.NewProviderContextOptions{DataSetMetadata: metadata, WithCDN: &withCDN})
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the full matching metadata for subsequent directory selection.
	metadata = provider.DataSetMetadata()
	if withCDN {
		metadata["withCDN"] = ""
	}
	prepare := func(target storage.StorageContext, count int) {
		sizes := make([]uint64, count)
		for i := range sizes {
			sizes[i] = chain.MinUploadSize
		}
		prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{Contexts: []storage.StorageContext{target}, PieceSizes: sizes, ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs, BufferEpochs: new(int64(integrationtest.FundingBufferEpochs))})
		if err != nil {
			t.Fatalf("Prepare(%d): %v", count, err)
		}
		if prep.Transaction != nil {
			res, err := prep.Transaction.Execute(ctx, payments.WithWait(180*time.Second))
			if err != nil || res.Receipt == nil || res.Receipt.Status != 1 {
				t.Fatalf("Prepare.Execute: result=%+v err=%v", res, err)
			}
		}
	}
	payloads := func(count int) [][]byte {
		result := make([][]byte, count)
		for i := range result {
			result[i] = make([]byte, int(chain.MinUploadSize))
			if _, err := crypto_rand.Read(result[i]); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}
	store := func(target storage.StorageContext, data [][]byte) []storage.PieceInput {
		pieces := make([]storage.PieceInput, len(data))
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		errs := make(chan error, len(data))
		for i, content := range data {
			wg.Go(func() {
				sem <- struct{}{}
				defer func() { <-sem }()
				result, err := target.Store(ctx, bytes.NewReader(content), nil)
				if err != nil {
					errs <- err
					return
				}
				pieces[i] = storage.PieceInput{PieceCID: result.PieceCID}
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("Store: %v", err)
		}
		return pieces
	}
	var legacyAdded []types.BigInt
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cancel()
		rpc, err := ethclient.DialContext(cleanupCtx, integrationtest.RPCURL())
		if err != nil {
			t.Errorf("legacy cleanup dial: %v", err)
			return
		}
		defer rpc.Close()
		if len(legacyAdded) == 0 {
			return
		}
		{
			chunk := legacyAdded
			res, err := legacy.DeletePiecesByID(cleanupCtx, chunk)
			if err != nil {
				t.Errorf("legacy cleanup delete: %v", err)
				return
			}
			for {
				receipt, err := rpc.TransactionReceipt(cleanupCtx, res.Hash)
				if err == nil {
					if receipt.Status != 1 {
						t.Errorf("legacy cleanup tx %s reverted", res.Hash)
					}
					break
				}
				if !errors.Is(err, ethereum.NotFound) {
					t.Errorf("legacy cleanup receipt: %v", err)
					return
				}
				select {
				case <-cleanupCtx.Done():
					t.Errorf("legacy cleanup: %v", cleanupCtx.Err())
					return
				case <-time.After(time.Second):
				}
			}
			t.Logf("legacy cleanup confirmed: %d added pieces tx=%s", len(chunk), res.Hash)
		}
	})
	prepare(provider, 81)
	pieces := store(provider, payloads(81))
	submission, err := provider.SubmitCreateAndAdd(ctx, storage.CreateAndAddRequest{Pieces: pieces})
	if err != nil {
		t.Fatalf("compact create 81: %v", err)
	}
	created, err := provider.WaitForCreateAndAdd(ctx, submission.StatusURL, *submission.ClientDataSetID)
	if err != nil || len(created.PieceIDs) != 81 {
		t.Fatalf("compact create confirmation=%+v err=%v", created, err)
	}
	if created.DataSet.DataSetID().Cmp(types.NewBigInt(cutoff)) < 0 {
		t.Fatalf("new data set %s is not compact", created.DataSet.DataSetID())
	}
	t.Logf("compact create: dataset=%s pieces=81 tx=%s", created.DataSet.DataSetID(), created.TransactionID)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		res, err := client.WarmStorage().TerminateDataSet(cleanupCtx, created.DataSet.DataSetID(), warmstorage.WithWait(180*time.Second))
		if err != nil || res.Receipt == nil || res.Receipt.Status != 1 {
			t.Errorf("new compact cleanup result=%+v err=%v", res, err)
			return
		}
		t.Logf("compact cleanup confirmed: dataset=%s tx=%s", created.DataSet.DataSetID(), res.Hash)
	})
	compact, err := client.Storage().NewDataSetContext(ctx, created.DataSet.DataSetID(), storage.NewDataSetContextOptions{})
	if err != nil {
		t.Fatal(err)
	}
	preferred := selectTarget()
	preferredRef, bound := preferred.DataSetRef()
	if !bound || !preferredRef.Equal(created.DataSet) {
		t.Fatalf("compact preference=%+v want=%+v", preferredRef, created.DataSet)
	}
	batchUpload := func(target *storage.DataSetContext, count int) map[string][]types.BigInt {
		prepare(target, count)
		data := payloads(count)
		stored := make(chan struct{}, count)
		type outcome struct {
			result *storage.UploadResult
			tx     string
			err    error
		}
		outcomes := make(chan outcome, count)
		sem := make(chan struct{}, 6)
		for _, content := range data {
			go func() {
				sem <- struct{}{}
				var released sync.Once
				release := func() { released.Do(func() { <-sem }) }
				defer release()
				var tx string
				result, err := target.Upload(ctx, bytes.NewReader(content), &storage.ContextUploadOptions{OnStored: func(types.BigInt, cid.Cid) { stored <- struct{}{}; release() }, OnPiecesAdded: func(hash string, _ types.BigInt, _ []storage.SubmittedPiece) { tx = hash }})
				outcomes <- outcome{result, tx, err}
			}()
		}
		finished := make([]outcome, 0, count)
		for ready := 0; ready < count; {
			select {
			case <-stored:
				ready++
			case out := <-outcomes:
				finished = append(finished, out)
				if out.err != nil {
					t.Fatalf("batched Upload: %v", out.err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
		if err := client.Storage().Flush(ctx); err != nil {
			t.Fatalf("batch Flush: %v", err)
		}
		for len(finished) < count {
			select {
			case out := <-outcomes:
				finished = append(finished, out)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		}
		byTx := make(map[string][]types.BigInt)
		for _, out := range finished {
			if out.err != nil || len(out.result.Copies) != 1 || out.tx == "" {
				t.Fatalf("upload result=%+v tx=%s err=%v", out.result, out.tx, out.err)
			}
			copy := out.result.Copies[0]
			ref, _ := target.DataSetRef()
			if !copy.DataSetID.Equal(ref.DataSetID()) {
				t.Fatalf("batch created an extra data set: %s want %s", copy.DataSetID, ref.DataSetID())
			}
			byTx[out.tx] = append(byTx[out.tx], copy.PieceID)
			if target == legacy {
				legacyAdded = append(legacyAdded, copy.PieceID)
			}
		}
		return byTx
	}
	compactTx := batchUpload(compact, 81)
	if len(compactTx) != 1 {
		t.Fatalf("compact 81 produced %d transactions", len(compactTx))
	}
	for tx := range compactTx {
		url := strings.TrimRight(compact.ServiceURL(), "/") + fmt.Sprintf("/pdp/data-sets/%s/pieces/added/%s", created.DataSet.DataSetID(), tx)
		status, err := compact.GetCommitStatus(ctx, url)
		if err != nil || len(status.PieceIDs) != 81 {
			t.Fatalf("compact status recovery=%+v err=%v", status, err)
		}
		result, err := compact.WaitForCommit(ctx, url)
		if err != nil || len(result.PieceIDs) != 81 {
			t.Fatalf("compact wait recovery=%+v err=%v", result, err)
		}
		t.Logf("compact high-level batch and recovery: pieces=81 tx=%s", tx)
	}
	prepare(legacy, 80)
	legacyPieces := store(legacy, payloads(81))
	if _, err := legacy.SubmitCommit(ctx, storage.CommitRequest{Pieces: legacyPieces}); !errors.Is(err, pdp.ErrTooManyPieces) {
		t.Fatalf("legacy 81 rejection: %v", err)
	}
	legacySubmission, err := legacy.SubmitCommit(ctx, storage.CommitRequest{Pieces: legacyPieces[:80]})
	if err != nil {
		t.Fatalf("legacy 80 submit: %v", err)
	}
	legacyResult, err := legacy.WaitForCommit(ctx, legacySubmission.StatusURL)
	if err != nil || len(legacyResult.PieceIDs) != 80 {
		t.Fatalf("legacy 80 confirmation=%+v err=%v", legacyResult, err)
	}
	legacyAdded = append(legacyAdded, legacyResult.PieceIDs...)
	t.Logf("legacy direct: pieces=80 tx=%s; 81 rejected locally", legacyResult.TransactionID)
	legacyTx := batchUpload(legacy, 81)
	counts := make([]int, 0, len(legacyTx))
	for _, ids := range legacyTx {
		counts = append(counts, len(ids))
	}
	slices.Sort(counts)
	if !slices.Equal(counts, []int{1, 80}) {
		t.Fatalf("legacy batches=%v want [1 80]", counts)
	}
	t.Logf("legacy automatic batches=%v; Prepare retained one target per layout", counts)
}
