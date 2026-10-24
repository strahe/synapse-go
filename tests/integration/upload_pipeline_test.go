//go:build integration

package integration_test

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ipfs/go-cid"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

type pipelineGateTransport struct {
	base         http.RoundTripper
	mu           sync.Mutex
	endpoint     string
	entered      chan struct{}
	release      <-chan struct{}
	once         *sync.Once
	putDeadlines []time.Duration
}

func (tr *pipelineGateTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	endpoint, entered, release, once := tr.endpoint, tr.entered, tr.release, tr.once
	if req.Method == http.MethodPut {
		remaining := time.Duration(0)
		if deadline, ok := req.Context().Deadline(); ok {
			remaining = time.Until(deadline)
		}
		tr.putDeadlines = append(tr.putDeadlines, remaining)
	}
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
			transport.mu.Lock()
			putDeadlines := slices.Clone(transport.putDeadlines)
			transport.mu.Unlock()
			if len(putDeadlines) == 0 {
				t.Fatal("no streaming PUT observed")
			}
			for _, remaining := range putDeadlines {
				if remaining <= 0 || remaining > 10*time.Minute {
					t.Fatalf("streaming PUT deadline remaining=%s, want within custom HTTP timeout of 10m", remaining)
				}
			}
			t.Run("cancel-after-completed-pull", func(t *testing.T) {
				payload := make([]byte, 128*1024)
				if _, err := crypto_rand.Read(payload); err != nil {
					t.Fatal(err)
				}
				uploadCtx, cancelUpload := context.WithCancel(ctx)
				defer cancelUpload()
				var completedPulls, completedCallbacks int
				result, err := client.Storage().UploadToContexts(uploadCtx, bytes.NewReader(payload), contexts[:2], &storage.UploadToContextsOptions{
					OnPullProgress: func(_ types.BigInt, _ cid.Cid, status storage.PullStatus) {
						if status == storage.PullStatusComplete {
							completedPulls++
							cancelUpload()
						}
					},
					OnCopyComplete: func(types.BigInt, cid.Cid) { completedCallbacks++ },
				})
				remember(result)
				var failures []storage.FailedAttempt
				switch {
				case err == nil && result != nil:
					if result.Complete {
						t.Fatalf("result=%+v, want incomplete upload after cancellation", result)
					}
					failures = result.FailedAttempts
				case errors.Is(err, context.Canceled):
					commitErr, ok := errors.AsType[*storage.CommitError](err)
					if !ok {
						t.Fatalf("error=%v, want CommitError", err)
					}
					failures = commitErr.FailedAttempts
				default:
					t.Fatalf("result=%+v error=%v, want cancellation diagnostics", result, err)
				}
				if completedPulls != 1 || completedCallbacks != 0 {
					t.Fatalf("completed pulls=%d copy callbacks=%d, want completed pull and suppressed callback", completedPulls, completedCallbacks)
				}
				if !slices.ContainsFunc(failures, func(failure storage.FailedAttempt) bool {
					return failure.ProviderID.Equal(contexts[1].ProviderID()) && failure.Stage == storage.CopyStageCommit &&
						errors.Is(failure.Err, context.Canceled)
				}) {
					t.Fatalf("failures=%+v, want cancellation recorded for completed secondary pull", failures)
				}
			})
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

type commitTimeoutTransport struct {
	base             http.RoundTripper
	mu               sync.Mutex
	endpoint         string
	statusURL        string
	hold             bool
	submissions      int
	pendingResponses int
}

func (tr *commitTimeoutTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	tr.mu.Lock()
	submission := req.Method == http.MethodPost && req.URL.String() == tr.endpoint
	if submission {
		tr.submissions++
	}
	tr.mu.Unlock()
	resp, err := tr.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	tr.mu.Lock()
	if submission && (resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusAccepted) {
		if location := resp.Header.Get("Location"); location != "" {
			if statusURL, err := req.URL.Parse(location); err == nil && statusURL.Scheme == req.URL.Scheme && statusURL.Host == req.URL.Host {
				tr.statusURL = statusURL.String()
			}
		}
	}
	hold := tr.hold && req.Method == http.MethodGet && req.URL.String() == tr.statusURL
	tr.mu.Unlock()
	if !hold || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted) {
		return resp, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, pdp.MaxControlResponseBytes+1))
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) == nil {
		var status, hash string
		_ = json.Unmarshal(fields["txStatus"], &status)
		_ = json.Unmarshal(fields["createMessageHash"], &hash)
		if (status == "pending" || status == "confirmed") && common.IsHexHash(hash) && common.HexToHash(hash) != (common.Hash{}) {
			fields["txStatus"] = json.RawMessage(`"pending"`)
			fields["dataSetCreated"] = json.RawMessage(`false`)
			delete(fields, "dataSetId")
			delete(fields, "ok")
			delete(fields, "confirmedTxHash")
			if pending, err := json.Marshal(fields); err == nil {
				body = pending
				tr.mu.Lock()
				tr.pendingResponses++
				tr.mu.Unlock()
			}
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
	return resp, nil
}

func (tr *commitTimeoutTransport) restore() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.hold = false
}

func (tr *commitTimeoutTransport) snapshot() (string, int, int) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.statusURL, tr.submissions, tr.pendingResponses
}

func TestIntegration_UploadCommitTimeoutRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Minute)
	defer cancel()
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	t.Cleanup(base.CloseIdleConnections)
	transport := &commitTimeoutTransport{base: base, hold: true}
	httpClient := &http.Client{Transport: transport, Timeout: 2 * time.Minute}
	client := integrationtest.NewDefaultClient(t, ctx,
		synapse.WithHTTPClient(httpClient),
		synapse.WithUploadBatching(storage.WithUploadIdleWait(0), storage.WithUploadCommitTimeout(15*time.Second)),
	)
	if client.Chain().ChainID() != calibrationChainID {
		t.Fatal("upload commit timeout acceptance requires Calibration")
	}
	run := fmt.Sprintf("commit-timeout-%d", time.Now().UnixNano())
	metadata := map[string]string{"source": "integration-commit-timeout", "run": run}
	var target *storage.ProviderContext
	var createdID types.BigInt
	t.Cleanup(func() {
		transport.restore()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cleanupCancel()
		flushCtx, flushCancel := context.WithTimeout(cleanupCtx, 30*time.Second)
		if err := client.Storage().Flush(flushCtx); err != nil {
			t.Logf("commit timeout cleanup Flush: %v", err)
		}
		flushCancel()
		statusURL, submissions, _ := transport.snapshot()
		if createdID.IsZero() && target != nil && statusURL != "" {
			provider, err := pdp.New(target.ServiceURL(), pdp.WithHTTPClient(httpClient))
			if err != nil {
				t.Errorf("commit timeout cleanup client: %v", err)
			} else {
				statusCtx, statusCancel := context.WithTimeout(cleanupCtx, 2*time.Minute)
				status, err := provider.WaitForDataSetCreated(statusCtx, statusURL, 4*time.Second)
				statusCancel()
				if err == nil && status != nil && status.DataSetID != nil {
					createdID = *status.DataSetID
				} else {
					t.Logf("commit timeout cleanup creation status: %v", err)
				}
			}
		}
		if createdID.IsZero() && submissions > 0 {
			discoveryCtx, discoveryCancel := context.WithTimeout(cleanupCtx, 2*time.Minute)
			defer discoveryCancel()
			for createdID.IsZero() && discoveryCtx.Err() == nil {
				dataSets, err := client.Storage().FindDataSets(discoveryCtx, &storage.FindDataSetsOptions{Payer: client.Address(), OnlyManaged: true})
				if err != nil {
					t.Logf("commit timeout cleanup discovery: %v", err)
				}
				for _, ds := range dataSets {
					if ds != nil && ds.Payer == client.Address() && ds.Metadata["run"] == run && ds.Metadata["source"] == metadata["source"] {
						createdID = ds.DataSetID
						break
					}
				}
				if createdID.IsZero() {
					select {
					case <-time.After(4 * time.Second):
					case <-discoveryCtx.Done():
					}
				}
			}
			if createdID.IsZero() {
				t.Error("commit timeout cleanup could not reconcile the submitted data set")
				return
			}
		}
		if createdID.IsZero() {
			return
		}
		info, err := retryIntegrationRead(cleanupCtx, func(ctx context.Context) (*warmstorage.DataSetInfo, error) {
			return client.WarmStorage().GetDataSet(ctx, createdID)
		})
		if err != nil || info == nil || info.Payer != client.Address() {
			t.Errorf("commit timeout cleanup ownership: dataSet=%s info=%+v err=%v", createdID, info, err)
			return
		}
		meta, err := retryIntegrationRead(cleanupCtx, func(ctx context.Context) (map[string]string, error) {
			return client.WarmStorage().GetAllDataSetMetadata(ctx, createdID)
		})
		if err != nil || meta["run"] != run || meta["source"] != metadata["source"] {
			t.Errorf("commit timeout cleanup metadata mismatch: dataSet=%s err=%v", createdID, err)
			return
		}
		if info.PDPEndEpoch != 0 {
			return
		}
		res, err := client.WarmStorage().TerminateDataSet(cleanupCtx, createdID, warmstorage.WithWait(txWaitTimeout))
		if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
			t.Errorf("commit timeout cleanup termination: result=%+v err=%v", res, err)
		}
	})
	var err error
	target, err = client.Storage().SelectProviderContext(ctx, storage.SelectProviderContextOptions{DataSetMetadata: metadata})
	if err != nil {
		t.Fatalf("SelectProviderContext: %v", err)
	}
	endpoint, err := url.JoinPath(target.ServiceURL(), "pdp/data-sets/create-and-add")
	if err != nil {
		t.Fatal(err)
	}
	transport.mu.Lock()
	transport.endpoint = endpoint
	transport.mu.Unlock()
	payload := make([]byte, 128*1024)
	if _, err := crypto_rand.Read(payload); err != nil {
		t.Fatal(err)
	}
	pieceInfo, err := piece.CalculateFromBytes(payload)
	if err != nil {
		t.Fatal(err)
	}
	prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{
		Contexts: []storage.StorageContext{target}, PieceSizes: []uint64{uint64(len(payload))},
		ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
		BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prep.Transaction != nil {
		res, err := prep.Transaction.Execute(ctx, payments.WithWait(txWaitTimeout))
		if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
			t.Fatalf("Prepare.Execute: result=%+v err=%v", res, err)
		}
	}
	result, err := target.Upload(ctx, bytes.NewReader(payload), nil)
	timeout, ok := errors.AsType[*storage.UploadCommitTimeoutError](err)
	if !ok || result != nil || !errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
		t.Fatalf("Upload result=%+v err=%v callerErr=%v, want recoverable commit timeout", result, err, ctx.Err())
	}
	transport.restore()
	submission := timeout.Submission
	statusURL, submissions, pendingResponses := transport.snapshot()
	parsedStatus, parseErr := url.Parse(submission.StatusURL)
	if submission.Kind != storage.CommitKindCreateAndAdd || submission.StatusURL != statusURL || parseErr != nil ||
		!common.IsHexHash(submission.TransactionID) || common.HexToHash(submission.TransactionID) != common.HexToHash(path.Base(parsedStatus.Path)) ||
		!submission.ProviderID.Equal(target.ProviderID()) || submission.Identity.Payer != client.Address() ||
		submission.Identity.ChainID != types.ChainID(calibrationChainID) || submission.Identity.RecordKeeper != client.ResolvedAddresses().FWSS ||
		submission.DataSet != nil ||
		submission.ClientDataSetID == nil || submission.ClientDataSetID.IsZero() ||
		len(submission.PieceCIDs) != 1 || !submission.PieceCIDs[0].Equals(pieceInfo.CIDv2) {
		t.Fatalf("incomplete timeout submission: %+v", submission)
	}
	if submissions != 1 || pendingResponses == 0 {
		t.Fatalf("submissions=%d pending responses=%d, want one real submission with pending confirmation", submissions, pendingResponses)
	}
	recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 6*time.Minute)
	defer recoveryCancel()
	recovered, err := target.WaitForCreateAndAdd(recoveryCtx, submission.StatusURL, *submission.ClientDataSetID)
	if err != nil || recovered == nil {
		t.Fatalf("WaitForCreateAndAdd: result=%+v err=%v", recovered, err)
	}
	createdID = recovered.DataSet.DataSetID()
	if recovered.TransactionID != submission.TransactionID || !recovered.DataSet.ProviderID().Equal(submission.ProviderID) ||
		!recovered.DataSet.ClientDataSetID().Equal(*submission.ClientDataSetID) || len(recovered.PieceIDs) != 1 {
		t.Fatalf("recovered result=%+v does not match original submission=%+v", recovered, submission)
	}
	if _, got, _ := transport.snapshot(); got != 1 {
		t.Fatalf("create-and-add submissions=%d after recovery, want one", got)
	}
	t.Logf("commit timeout recovered: provider=%s dataSet=%s tx=%s", submission.ProviderID, createdID, submission.TransactionID)
}
