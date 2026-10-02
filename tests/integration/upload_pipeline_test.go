//go:build integration

package integration_test

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
)

type pipelineGateTransport struct {
	base     http.RoundTripper
	mu       sync.Mutex
	endpoint string
	entered  chan struct{}
	release  <-chan struct{}
	once     *sync.Once
}

func (tr *pipelineGateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	endpoint, entered, release, once := tr.endpoint, tr.entered, tr.release, tr.once
	tr.mu.Unlock()
	if req.Method == http.MethodPost && req.URL.String() == endpoint {
		once.Do(func() { close(entered) })
		select {
		case <-release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	return tr.base.RoundTrip(req)
}

func (tr *pipelineGateTransport) arm(t *testing.T, baseURL string) (<-chan struct{}, func()) {
	t.Helper()
	endpoint, err := url.JoinPath(baseURL, "pdp/piece/pull")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	tr.mu.Lock()
	tr.endpoint, tr.entered, tr.release, tr.once = endpoint, entered, release, &sync.Once{}
	tr.mu.Unlock()
	return entered, sync.OnceFunc(func() { close(release) })
}

func TestIntegration_UploadPipelines(t *testing.T) {
	for _, batched := range []bool{false, true} {
		t.Run(fmt.Sprintf("batched=%t", batched), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
			defer cancel()
			base := http.DefaultTransport.(*http.Transport).Clone()
			base.Proxy = nil
			t.Cleanup(base.CloseIdleConnections)
			transport := &pipelineGateTransport{base: base}
			opts := []synapse.ClientOption{
				synapse.WithHTTPClient(&http.Client{Transport: transport, Timeout: 10 * time.Minute}),
				synapse.WithUploadPullConcurrency(2),
			}
			if batched {
				opts = append(opts, synapse.WithUploadBatching(storage.WithUploadIdleWait(0)))
			}
			client := integrationtest.NewDefaultClient(t, ctx, opts...)
			if client.Chain().ChainID() != calibrationChainID {
				t.Fatal("upload pipeline acceptance requires calibration")
			}
			run := fmt.Sprintf("pipeline-%d", time.Now().UnixNano())
			metadata := map[string]string{"source": "integration-upload-pipeline", "run": run}
			selection, err := client.Storage().SelectUploadContexts(ctx, storage.SelectUploadContextsOptions{
				Copies: 3, DataSetMetadata: metadata, AllowUnendorsedPrimary: true,
			})
			if err != nil {
				t.Fatalf("three healthy providers required for P0 pipeline acceptance: %v", err)
			}
			if selection == nil || len(selection.Contexts) != 3 {
				t.Fatalf("selection=%+v, want three contexts", selection)
			}
			for _, target := range selection.Contexts {
				if _, bound := target.DataSetRef(); bound {
					t.Fatal("unique pipeline metadata unexpectedly reused a dataset")
				}
				t.Logf("selected provider=%s endpoint=%s", target.ProviderID(), target.ServiceURL())
			}
			if selection.Contexts[1].ServiceURL() == selection.Contexts[2].ServiceURL() {
				t.Fatal("pipeline gate requires distinct secondary endpoints")
			}
			created := make(map[string]types.BigInt)
			remember := func(result *storage.UploadResult) {
				if result != nil {
					for _, cp := range result.Copies {
						created[cp.DataSetID.String()] = cp.DataSetID
					}
				}
			}
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cleanupCancel()
				if err := client.Storage().Flush(cleanupCtx); err != nil {
					t.Logf("pipeline cleanup Flush: %v", err)
				}
				if len(created) < 3 {
					dataSets, err := client.Storage().FindDataSets(cleanupCtx, &storage.FindDataSetsOptions{OnlyManaged: true})
					if err != nil {
						t.Logf("reconcile pipeline datasets: %v", err)
					}
					for _, ds := range dataSets {
						if ds == nil {
							continue
						}
						meta, err := client.WarmStorage().GetAllDataSetMetadata(cleanupCtx, ds.DataSetID)
						if err == nil && meta["run"] == run {
							created[ds.DataSetID.String()] = ds.DataSetID
						}
					}
				}
				for _, id := range created {
					terminateDataSetOnCleanup(t, client, id, "UploadPipeline")
				}
			})
			contexts := selection.Contexts
			for _, stage := range []string{"new-data-sets", "existing-data-sets"} {
				payload := make([]byte, 128*1024)
				if _, err := crypto_rand.Read(payload); err != nil {
					t.Fatal(err)
				}
				prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{
					Contexts: contexts, PieceSizes: []uint64{uint64(len(payload))},
					ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
					BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
				})
				if err != nil {
					t.Fatalf("%s Prepare: %v", stage, err)
				}
				if prep.Transaction != nil {
					result, err := prep.Transaction.Execute(ctx, payments.WithWait(txWaitTimeout))
					if err != nil {
						t.Fatalf("%s Prepare.Execute: %v", stage, err)
					}
					if result.Receipt == nil || result.Receipt.Status != 1 {
						t.Fatalf("%s funding receipt=%+v", stage, result.Receipt)
					}
				}
				result := uploadPipelineWithGate(t, ctx, client, transport, contexts, payload, stage, remember)
				remember(result)
				if result == nil || !result.Complete || len(result.Copies) != 3 {
					t.Fatalf("%s result=%+v, want three confirmed copies", stage, result)
				}
				for i, cp := range result.Copies {
					if !cp.ProviderID.Equal(contexts[i].ProviderID()) {
						t.Fatalf("%s result order changed at slot %d", stage, i)
					}
					bound, err := client.Storage().NewDataSetContext(ctx, cp.DataSetID, storage.NewDataSetContextOptions{})
					if err != nil {
						t.Fatalf("open confirmed pipeline dataset: %v", err)
					}
					contexts[i] = bound
				}
			}
		})
	}
}

func uploadPipelineWithGate(t *testing.T, ctx context.Context, client *synapse.Client, transport *pipelineGateTransport, contexts []storage.StorageContext, payload []byte, stage string, remember func(*storage.UploadResult)) *storage.UploadResult {
	t.Helper()
	uploadCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	entered, release := transport.arm(t, contexts[1].ServiceURL())
	defer release()
	added := make(chan types.BigInt, len(contexts))
	type outcome struct {
		result *storage.UploadResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := client.Storage().UploadToContexts(uploadCtx, bytes.NewReader(payload), contexts, &storage.UploadToContextsOptions{
			OnPiecesAdded: func(txHash string, providerID types.BigInt, _ []storage.SubmittedPiece) {
				t.Logf("%s submitted provider=%s tx=%s", stage, providerID, txHash)
				added <- providerID
			},
		})
		done <- outcome{result, err}
	}()
	consumed := false
	defer func() {
		if consumed {
			return
		}
		release()
		cancel()
		select {
		case got := <-done:
			remember(got.result)
		case <-time.After(10 * time.Second):
			t.Error("pipeline upload did not stop after cancellation")
		}
	}()
	proofCtx, proofCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer proofCancel()
	seen := make(map[string]bool)
	blocked := false
	for !blocked || !seen[contexts[0].ProviderID().String()] || !seen[contexts[2].ProviderID().String()] {
		select {
		case <-entered:
			blocked = true
			entered = nil
		case id := <-added:
			seen[id.String()] = true
		case got := <-done:
			consumed = true
			remember(got.result)
			t.Fatalf("%s upload ended before independent submission proof: result=%+v error=%v", stage, got.result, got.err)
		case <-proofCtx.Done():
			t.Fatalf("%s ready copies did not submit while slow pull was blocked: %v", stage, proofCtx.Err())
		}
	}
	t.Logf("%s primary and fast secondary submitted before releasing provider=%s", stage, contexts[1].ProviderID())
	release()
	select {
	case got := <-done:
		consumed = true
		remember(got.result)
		if got.err != nil {
			t.Fatalf("%s upload: %v", stage, got.err)
		}
		return got.result
	case <-ctx.Done():
		t.Fatalf("%s confirmations: %v", stage, ctx.Err())
		return nil
	}
}
