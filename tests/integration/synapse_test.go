//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	crypto_rand "crypto/rand"
	"errors"
	"fmt"
	"io"
	"maps"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/filbeam"
	filpaybind "github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/internal/retry"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/pdp"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

const (
	calibrationChainID = integrationtest.CalibrationChainID
	testDataSize       = 256 * 1024 // 256 KB
	txWaitTimeout      = 180 * time.Second
)

var noProgressInSettlementSelector = filPayErrorSelector("NoProgressInSettlement")

// ABI selector lookup panics during test init because ABI drift is a codegen bug.
func filPayErrorSelector(name string) string {
	parsed, err := filpaybind.FilPayMetaData.GetAbi()
	if err != nil {
		panic(err)
	}
	def, ok := parsed.Errors[name]
	if !ok {
		panic("missing FilPay error: " + name)
	}
	return strings.ToLower(def.ID.Hex()[:10])
}

func isNoProgressInSettlement(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), noProgressInSettlementSelector)
}

func isExecutionRevert(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, payments.ErrTxFailed) {
		return true
	}
	low := strings.ToLower(err.Error())
	if strings.Contains(low, "execution reverted") || strings.Contains(low, "contract reverted") {
		return true
	}
	return strings.Contains(low, "message execution failed") && strings.Contains(low, "revert reason")
}

func isIntegrationReadTransient(err error) bool {
	if err == nil {
		return false
	}
	if isExecutionRevert(err) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	low := strings.ToLower(err.Error())
	for _, marker := range []string{
		"context deadline exceeded",
		"client.timeout exceeded",
		"timeout",
		"timed out",
		"i/o timeout",
		"connection reset",
		"connection refused",
		"broken pipe",
		"temporary failure",
		"try again",
		"eof",
	} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

func retryIntegrationRead[T any](ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	return retry.Do(
		ctx,
		fn,
		retry.WithMaxRetries(2),
		retry.WithInitialDelay(time.Second),
		retry.WithMaxDelay(3*time.Second),
		retry.WithRetryIf(isIntegrationReadTransient),
	)
}

type recordedFilBeamRequest struct {
	Method string
	Host   string
	Path   string
}

type filbeamRecordingTransport struct {
	inner http.RoundTripper

	mu       sync.Mutex
	requests []recordedFilBeamRequest
}

func (rt *filbeamRecordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if strings.HasSuffix(strings.ToLower(req.URL.Hostname()), "filbeam.io") {
		rt.mu.Lock()
		rt.requests = append(rt.requests, recordedFilBeamRequest{
			Method: req.Method,
			Host:   req.URL.Host,
			Path:   req.URL.Path,
		})
		rt.mu.Unlock()
	}
	return rt.inner.RoundTrip(req)
}

func (rt *filbeamRecordingTransport) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.requests = nil
}

func (rt *filbeamRecordingTransport) Snapshot() []recordedFilBeamRequest {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	out := make([]recordedFilBeamRequest, len(rt.requests))
	copy(out, rt.requests)
	return out
}

func hasFilBeamMethod(requests []recordedFilBeamRequest, method string) bool {
	for _, req := range requests {
		if req.Method == method {
			return true
		}
	}
	return false
}

func tracedContextUploadOptions(t *testing.T, label string, opts *storage.ContextUploadOptions) *storage.ContextUploadOptions {
	t.Helper()
	cloned := &storage.ContextUploadOptions{}
	if opts != nil {
		v := *opts
		cloned = &v
	}

	prevStored := cloned.OnStored
	cloned.OnStored = func(providerID types.BigInt, pieceCID cid.Cid) {
		t.Logf("%s stored: provider=%s cid=%s", label, providerID, pieceCID)
		if prevStored != nil {
			prevStored(providerID, pieceCID)
		}
	}

	prevPiecesAdded := cloned.OnPiecesAdded
	cloned.OnPiecesAdded = func(txHash string, providerID types.BigInt, pieces []storage.SubmittedPiece) {
		t.Logf("%s commit submitted: provider=%s tx=%s pieces=%d", label, providerID, txHash, len(pieces))
		if prevPiecesAdded != nil {
			prevPiecesAdded(txHash, providerID, pieces)
		}
	}

	prevPiecesConfirmed := cloned.OnPiecesConfirmed
	cloned.OnPiecesConfirmed = func(dataSetID, providerID types.BigInt, pieces []storage.ConfirmedPiece) {
		t.Logf("%s commit confirmed: provider=%s dataSet=%s pieces=%d", label, providerID, dataSetID, len(pieces))
		if prevPiecesConfirmed != nil {
			prevPiecesConfirmed(dataSetID, providerID, pieces)
		}
	}

	return cloned
}

func tracedUploadOptions(t *testing.T, label string, opts *storage.UploadOptions) *storage.UploadOptions {
	t.Helper()
	cloned := &storage.UploadOptions{}
	if opts != nil {
		v := *opts
		cloned = &v
	}

	lifecycle := tracedContextUploadOptions(t, label, &storage.ContextUploadOptions{
		OnStored:          cloned.OnStored,
		OnPiecesAdded:     cloned.OnPiecesAdded,
		OnPiecesConfirmed: cloned.OnPiecesConfirmed,
	})
	cloned.OnStored = lifecycle.OnStored
	cloned.OnPiecesAdded = lifecycle.OnPiecesAdded
	cloned.OnPiecesConfirmed = lifecycle.OnPiecesConfirmed

	prevCopyComplete := cloned.OnCopyComplete
	cloned.OnCopyComplete = func(providerID types.BigInt, pieceCID cid.Cid) {
		t.Logf("%s copy complete: provider=%s cid=%s", label, providerID, pieceCID)
		if prevCopyComplete != nil {
			prevCopyComplete(providerID, pieceCID)
		}
	}

	prevCopyFailed := cloned.OnCopyFailed
	cloned.OnCopyFailed = func(providerID types.BigInt, pieceCID cid.Cid, err error) {
		t.Logf("%s copy failed: provider=%s cid=%s err=%v", label, providerID, pieceCID, err)
		if prevCopyFailed != nil {
			prevCopyFailed(providerID, pieceCID, err)
		}
	}

	return cloned
}

func TestIntegration_CDNContextDownload(t *testing.T) {
	ctx := context.Background()
	recorder := &filbeamRecordingTransport{inner: http.DefaultTransport}
	httpClient := &http.Client{
		Transport: recorder,
		Timeout:   5 * time.Minute,
	}
	client, err := synapse.New(ctx,
		synapse.WithPrivateKeyHex(integrationtest.RequirePrivateKey(t)),
		synapse.WithRPCURL(integrationtest.RPCURL()),
		synapse.WithHTTPClient(httpClient),
		synapse.WithCDN(true),
		synapse.WithoutUploadBatching(),
	)
	if err != nil {
		t.Fatalf("synapse.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	cctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	data := make([]byte, testDataSize)
	if _, err := crypto_rand.Read(data); err != nil {
		t.Fatalf("generate CDN test data: %v", err)
	}

	withCDN := true
	metadata := map[string]string{
		"source": "integration-cdn-context-download",
		"run":    fmt.Sprintf("%d", time.Now().UnixNano()),
	}
	t.Log("start CDNContextDownload SelectUploadContexts")
	selection, err := client.Storage().SelectUploadContexts(cctx, storage.SelectUploadContextsOptions{
		Copies:          1,
		DataSetMetadata: metadata,
		WithCDN:         &withCDN,
	})
	if err != nil {
		t.Fatalf("SelectUploadContexts: %v", err)
	}
	if selection == nil || len(selection.Contexts) != 1 || selection.Contexts[0] == nil {
		t.Fatalf("SelectUploadContexts returned %+v", selection)
	}
	uploadCtx := selection.Contexts[0]

	t.Log("start CDNContextDownload Prepare")
	prep, err := client.Storage().Prepare(cctx, &storage.PrepareOptions{
		PieceSizes:        []uint64{uint64(len(data))},
		Contexts:          []storage.StorageContext{uploadCtx},
		ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
		BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
	})
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	if prep.Transaction != nil {
		t.Log("start CDNContextDownload Prepare.Execute")
		res, err := prep.Transaction.Execute(cctx, payments.WithWait(txWaitTimeout))
		if err != nil {
			if errors.Is(err, payments.ErrPermitUnsupported) {
				t.Skip("needs-usdfc-permit-support: CDN integration funding requires permit support")
			}
			t.Fatalf("Prepare.Execute: %v", err)
		}
		if res.Receipt == nil || res.Receipt.Status != 1 {
			t.Fatalf("Prepare.Execute receipt = %+v", res.Receipt)
		}
	}

	start := time.Now()
	t.Log("start CDNContextDownload Upload")
	var result *storage.UploadResult
	switch concrete := uploadCtx.(type) {
	case *storage.ProviderContext:
		result, err = concrete.Upload(cctx, bytes.NewReader(data), tracedContextUploadOptions(t, "CDNContextDownload", nil))
	case *storage.DataSetContext:
		result, err = concrete.Upload(cctx, bytes.NewReader(data), tracedContextUploadOptions(t, "CDNContextDownload", nil))
	default:
		t.Fatalf("selected context has unsupported type %T", uploadCtx)
	}
	t.Logf("done CDNContextDownload Upload elapsed=%s", time.Since(start).Round(time.Second))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if len(result.Copies) == 0 {
		t.Fatal("Upload returned no copies")
	}
	cleanupDataSetID := result.Copies[0].DataSetID
	terminated := false
	if !cleanupDataSetID.IsZero() {
		t.Cleanup(func() {
			if terminated {
				return
			}
			cctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()

			t.Logf("start CDNContextDownload cleanup TerminateDataSet(%s)", cleanupDataSetID)
			termRes, err := client.WarmStorage().TerminateDataSet(cctx, cleanupDataSetID, warmstorage.WithWait(txWaitTimeout))
			if err != nil {
				t.Logf("cleanup TerminateDataSet(%s): %v", cleanupDataSetID, err)
				return
			}
			if termRes == nil {
				t.Logf("cleanup TerminateDataSet(%s) returned nil result", cleanupDataSetID)
				return
			}
			if termRes.Receipt != nil && termRes.Receipt.Status != 1 {
				t.Logf("cleanup TerminateDataSet(%s) receipt = %+v", cleanupDataSetID, termRes.Receipt)
			}
			t.Logf("done CDNContextDownload cleanup TerminateDataSet(%s) tx=%s", cleanupDataSetID, termRes.Hash)
		})
	}
	if !result.PieceCID.Defined() {
		t.Fatal("Upload returned undefined PieceCID")
	}

	downloadDataSetID := result.Copies[0].DataSetID
	uctx, err := client.Storage().NewDataSetContext(cctx, downloadDataSetID, storage.NewDataSetContextOptions{
		WithCDN: &withCDN,
	})
	if err != nil {
		t.Fatalf("NewDataSetContext: %v", err)
	}
	if !uctx.CDNEnabled() {
		t.Fatal("created context has WithCDN=false")
	}

	var lastErr error
	for attempt := 1; attempt <= 10; attempt++ {
		if attempt > 1 {
			delay := 30 * time.Second
			t.Logf("CDN context download attempt %d retrying in %s: %v", attempt, delay, lastErr)
			select {
			case <-time.After(delay):
			case <-cctx.Done():
				t.Fatalf("context expired waiting for CDN download: %v", cctx.Err())
			}
		}

		recorder.Reset()
		start = time.Now()
		t.Logf("start CDNContextDownload StorageContext.Download attempt %d", attempt)
		rc, err := uctx.Download(cctx, result.PieceCID)
		if err != nil {
			lastErr = err
			t.Logf("done CDNContextDownload StorageContext.Download attempt %d elapsed=%s requests=%+v err=%v",
				attempt, time.Since(start).Round(time.Second), recorder.Snapshot(), err)
			continue
		}
		got, readErr := io.ReadAll(rc)
		_ = rc.Close()
		requests := recorder.Snapshot()
		t.Logf("done CDNContextDownload StorageContext.Download attempt %d elapsed=%s requests=%+v",
			attempt, time.Since(start).Round(time.Second), requests)
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if !bytes.Equal(got, data) {
			lastErr = errors.New("downloaded data mismatch")
			continue
		}
		if !hasFilBeamMethod(requests, http.MethodHead) || !hasFilBeamMethod(requests, http.MethodGet) {
			lastErr = fmt.Errorf("download succeeded without observed FilBeam HEAD+GET: %+v", requests)
			continue
		}
		t.Logf("CDN context download verified through FilBeam: cid=%s dataset=%s", result.PieceCID, result.Copies[0].DataSetID)

		recorder.Reset()
		byID, err := client.Storage().Download(cctx, result.PieceCID, &storage.DownloadOptions{DataSetID: &downloadDataSetID})
		if err != nil {
			t.Fatalf("data-set-ID download: %v", err)
		}
		got, readErr = io.ReadAll(byID)
		_ = byID.Close()
		requests = recorder.Snapshot()
		if readErr != nil || !bytes.Equal(got, data) {
			t.Fatalf("data-set-ID download bytes=%d error=%v", len(got), readErr)
		}
		if !hasFilBeamMethod(requests, http.MethodHead) || !hasFilBeamMethod(requests, http.MethodGet) {
			t.Fatalf("data-set-ID download did not use FilBeam HEAD+GET: %+v", requests)
		}
		t.Logf("data-set-ID CDN download verified: dataset=%s", downloadDataSetID)

		t.Logf("start CDNContextDownload TerminateDataSet(%s)", cleanupDataSetID)
		termRes, err := client.WarmStorage().TerminateDataSet(cctx, cleanupDataSetID, warmstorage.WithWait(txWaitTimeout))
		if err != nil {
			t.Fatalf("TerminateDataSet(%s): %v", cleanupDataSetID, err)
		}
		if termRes == nil || termRes.Receipt == nil || termRes.Receipt.Status != 1 {
			t.Fatalf("TerminateDataSet(%s) receipt = %+v", cleanupDataSetID, termRes)
		}
		terminated = true
		t.Logf("done CDNContextDownload TerminateDataSet(%s) tx=%s", cleanupDataSetID, termRes.Hash)
		return
	}
	t.Fatalf("CDN context download did not complete through FilBeam: %v", lastErr)
}

func TestIntegration_LargePieceStreaming(t *testing.T) {
	const rawSize int64 = 2 << 30
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Minute)
	defer cancel()
	client := integrationtest.NewDefaultClient(t, ctx)
	if client.Chain().ChainID() != calibrationChainID {
		t.Fatal("large-piece acceptance requires calibration")
	}

	eligible, err := client.SPRegistry().SelectActivePDPProviders(ctx, spregistry.ProviderFilter{
		PieceSizeBytes: big.NewInt(rawSize),
	})
	if err != nil {
		t.Fatalf("read providers accepting %d bytes: %v", rawSize, err)
	}
	if len(eligible) < 2 {
		t.Fatalf("large-piece acceptance requires two providers offering %d bytes; found %d", rawSize, len(eligible))
	}
	eligibleIDs := make(map[string]struct{}, len(eligible))
	for _, provider := range eligible {
		eligibleIDs[idconv.Key(provider.Info.ID)] = struct{}{}
	}
	providers, err := client.SPRegistry().SelectActivePDPProviders(ctx, spregistry.ProviderFilter{})
	if err != nil {
		t.Fatalf("read active providers: %v", err)
	}
	var excluded []types.BigInt
	for _, provider := range providers {
		if _, ok := eligibleIDs[idconv.Key(provider.Info.ID)]; !ok {
			excluded = append(excluded, provider.Info.ID)
		}
	}
	run := fmt.Sprintf("large-piece-%d", time.Now().UnixNano())
	withCDN := false
	selection, err := client.Storage().SelectUploadContexts(ctx, storage.SelectUploadContextsOptions{
		Copies:             2,
		DataSetMetadata:    map[string]string{"run": run},
		ExcludeProviderIDs: excluded,
		WithCDN:            &withCDN,
	})
	if err != nil {
		t.Fatalf("large-piece acceptance requires two approved, healthy providers and an endorsed primary: %v", err)
	}
	if selection == nil || !selection.Complete || len(selection.Contexts) != 2 {
		t.Fatalf("large-piece selection=%+v, want two complete contexts", selection)
	}
	for _, target := range selection.Contexts {
		if _, ok := eligibleIDs[idconv.Key(target.ProviderID())]; !ok {
			t.Fatalf("selected provider %s does not offer %d bytes", target.ProviderID(), rawSize)
		}
		if _, bound := target.DataSetRef(); bound {
			t.Fatal("unique large-piece metadata unexpectedly reused a data set")
		}
		t.Logf("large-piece provider=%s endpoint=%s", target.ProviderID(), target.ServiceURL())
	}

	created := make(map[string]types.BigInt)
	t.Cleanup(func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), time.Minute)
		if err := client.Storage().Flush(flushCtx); err != nil {
			t.Logf("large-piece cleanup Flush: %v", err)
		}
		flushCancel()
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 8*time.Minute)
		defer cleanupCancel()
		if len(created) < 2 {
			dataSets, err := client.Storage().FindDataSets(cleanupCtx, &storage.FindDataSetsOptions{OnlyManaged: true})
			if err != nil {
				t.Errorf("reconcile large-piece data sets: %v", err)
			}
			for _, dataSet := range dataSets {
				if dataSet == nil {
					continue
				}
				metadata, err := client.WarmStorage().GetAllDataSetMetadata(cleanupCtx, dataSet.DataSetID)
				if err == nil && metadata["run"] == run {
					created[dataSet.DataSetID.String()] = dataSet.DataSetID
				}
			}
		}
		for _, id := range created {
			dataSet, err := client.WarmStorage().GetDataSet(cleanupCtx, id)
			if err != nil {
				t.Errorf("read large-piece cleanup data set %s: %v", id, err)
				continue
			}
			metadata, err := client.WarmStorage().GetAllDataSetMetadata(cleanupCtx, id)
			if err != nil || dataSet.Payer != client.Address() || metadata["run"] != run {
				t.Errorf("cannot verify ownership of large-piece cleanup data set %s: %v", id, err)
				continue
			}
			result, err := client.WarmStorage().TerminateDataSet(cleanupCtx, id, warmstorage.WithWait(txWaitTimeout))
			if err != nil || result == nil || result.Receipt == nil || result.Receipt.Status != 1 {
				t.Errorf("terminate large-piece data set %s: result=%+v error=%v", id, result, err)
			}
		}
	})

	file, err := os.CreateTemp(t.TempDir(), "large-piece-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if err := file.Truncate(rawSize); err != nil {
		t.Fatalf("create sparse upload file: %v", err)
	}
	var prefix [4096]byte
	if _, err := crypto_rand.Read(prefix[:]); err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt(prefix[:], 0); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	prep, err := client.Storage().Prepare(ctx, &storage.PrepareOptions{
		Contexts:          selection.Contexts,
		PieceSizes:        []uint64{uint64(rawSize)},
		ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
		BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
	})
	if err != nil || prep == nil {
		t.Fatalf("prepare large-piece upload: result=%+v error=%v", prep, err)
	}
	if prep.Transaction != nil {
		result, err := prep.Transaction.Execute(ctx, payments.WithWait(txWaitTimeout))
		if err != nil || result == nil || result.Receipt == nil || result.Receipt.Status != 1 {
			t.Fatalf("fund large-piece upload: result=%+v error=%v", result, err)
		}
	}
	start := time.Now()
	result, err := client.Storage().UploadToContexts(ctx, file, selection.Contexts, nil)
	if result != nil {
		for _, copy := range result.Copies {
			if copy.IsNewDataSet && !copy.DataSetID.IsZero() {
				created[copy.DataSetID.String()] = copy.DataSetID
			}
		}
	}
	if err != nil {
		t.Fatalf("upload large piece: %v", err)
	}
	if result == nil || !result.Complete || result.Size != rawSize || len(result.Copies) != 2 {
		t.Fatalf("large-piece upload=%+v, want %d bytes and two complete copies", result, rawSize)
	}
	info, err := piece.ParseV2(result.PieceCID)
	if err != nil || info.RawSize != uint64(rawSize) {
		t.Fatalf("large-piece CID info=%+v error=%v", info, err)
	}
	for _, copy := range result.Copies {
		if !copy.IsNewDataSet || copy.DataSetID.IsZero() {
			t.Fatalf("large-piece copy=%+v, want a new data set", copy)
		}
		body, err := client.Storage().Download(ctx, result.PieceCID, &storage.DownloadOptions{URL: copy.RetrievalURL})
		if err != nil {
			t.Fatalf("download large piece from provider %s: %v", copy.ProviderID, err)
		}
		count, readErr := io.Copy(io.Discard, body)
		closeErr := body.Close()
		if readErr != nil || closeErr != nil || count != rawSize {
			t.Fatalf("provider %s large-piece download bytes=%d read=%v close=%v", copy.ProviderID, count, readErr, closeErr)
		}
	}
	t.Logf("verified large-piece upload and both downloads: cid=%s rawBytes=%d elapsed=%s", result.PieceCID, rawSize, time.Since(start).Round(time.Second))
}

// TestIntegration classifies network coverage as follows: service and context
// methods called below are direct coverage; PDP create/add/pull/delete/
// terminate writes reached through storage are real delegated coverage; SP
// owner writes, CDN rail debt and destructive permit scenarios remain explicit
// external-condition coverage in their dedicated subtests.
func TestIntegration(t *testing.T) {
	ctx := context.Background()
	client := integrationtest.NewDefaultClient(t, ctx)

	// Verify auto-detected chain is Calibration.
	if got := client.Chain().ChainID(); got != calibrationChainID {
		t.Fatalf("chain ID = %d, want %d (calibration)", got, calibrationChainID)
	}

	addr := client.Address()
	if addr == (common.Address{}) {
		t.Fatal("client address is zero")
	}
	t.Logf("client address: %s, chain: %s (ID %d)", addr, client.Chain(), client.Chain().ChainID())

	addrs := client.ResolvedAddresses()
	usdfc := addrs.USDFC
	filPay := addrs.Payments
	fwss := addrs.FWSS

	// Shared state across subtests.
	var (
		uploadedCID        cid.Cid
		uploadedURL        string
		testData           []byte
		uploadedDataSetID  types.BigInt
		uploadedProviderID types.BigInt
		uploadedRailID     types.BigInt
	)
	run := fmt.Sprintf("storage-http-%d", time.Now().UnixNano())
	uploadMetadata := map[string]string{"run": run, "emptyFlag": ""}
	dataSetTerminated := make(map[string]bool)
	suite := t

	t.Run("ClientNetwork", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		matched := integrationtest.NewDefaultClient(t, cctx, synapse.WithChain(chain.Calibration))
		if matched.Chain() != chain.Calibration {
			t.Fatalf("explicit chain = %v, want Calibration", matched.Chain())
		}
		defaulted := integrationtest.NewDefaultClient(t, cctx, synapse.WithRPCURL(""))
		if defaulted.Chain() != chain.Calibration {
			t.Fatalf("default chain = %v, want Calibration", defaulted.Chain())
		}
		mismatched, err := synapse.New(cctx,
			synapse.WithPrivateKeyHex(integrationtest.RequirePrivateKey(t)),
			synapse.WithRPCURL(integrationtest.RPCURL()),
			synapse.WithChain(chain.Mainnet),
		)
		if mismatched != nil {
			_ = mismatched.Close()
			t.Fatal("Calibration RPC accepted as Mainnet")
		}
		if !errors.Is(err, synapse.ErrInvalidArgument) {
			t.Fatalf("network mismatch = %v, want ErrInvalidArgument", err)
		}
	})

	// --- Costs subtest ---
	t.Run("Costs", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		dataSize := big.NewInt(testDataSize)
		uploadCosts, err := client.Costs().GetUploadCosts(cctx, addr, []uint64{dataSize.Uint64()}, &costs.UploadCostOptions{IsNewDataSet: true})
		if err != nil {
			t.Fatalf("GetUploadCosts: %v", err)
		}

		if uploadCosts.Rate.RatePerEpoch == nil || uploadCosts.Rate.RatePerEpoch.Sign() <= 0 {
			t.Errorf("RatePerEpoch should be positive, got %v", uploadCosts.Rate.RatePerEpoch)
		}
		if uploadCosts.Rate.RatePerMonth == nil || uploadCosts.Rate.RatePerMonth.Sign() <= 0 {
			t.Errorf("RatePerMonth should be positive, got %v", uploadCosts.Rate.RatePerMonth)
		}
		if uploadCosts.Lockup.Total == nil || uploadCosts.Lockup.Total.Sign() < 0 {
			t.Errorf("Total should be >= 0, got %v", uploadCosts.Lockup.Total)
		}
		t.Logf("rate/epoch=%s, rate/month=%s, lockup=%s, depositNeeded=%s, ready=%v",
			uploadCosts.Rate.RatePerEpoch, uploadCosts.Rate.RatePerMonth,
			uploadCosts.Lockup.Total, uploadCosts.DepositNeeded, uploadCosts.Ready)

		summary, err := client.Payments().AccountSummary(cctx, addr)
		if err != nil {
			t.Fatalf("AccountSummary: %v", err)
		}
		if summary.Funds == nil {
			t.Error("AccountSummary.Funds is nil")
		}
		if summary.AvailableFunds == nil {
			t.Error("AccountSummary.AvailableFunds is nil")
		}
		if summary.CurrentEpoch == nil || summary.CurrentEpoch.Sign() <= 0 {
			t.Errorf("CurrentEpoch should be positive, got %v", summary.CurrentEpoch)
		}
		t.Logf("funds=%s, available=%s, debt=%s, currentEpoch=%s",
			summary.Funds, summary.AvailableFunds, summary.Debt, summary.CurrentEpoch)
	})

	// --- Payments subtest ---
	t.Run("Payments", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		key, err := crypto.HexToECDSA(strings.TrimPrefix(strings.TrimSpace(integrationtest.RequirePrivateKey(t)), "0x"))
		if err != nil {
			t.Fatal("invalid integration private key")
		}
		client := integrationtest.NewDefaultClient(t, cctx, synapse.WithPrivateKey(key))
		key.D.SetInt64(0) //nolint:staticcheck // secp256k1 scalar mutation exercises caller-owned key isolation.
		key.X.SetInt64(0)
		key.Y.SetInt64(0)
		if client.Address() != addr {
			t.Fatalf("client address = %s, want %s", client.Address(), addr)
		}

		// Query initial account state.
		acct, err := client.Payments().AccountInfo(cctx, usdfc, addr)
		if err != nil {
			t.Fatalf("AccountInfo: %v", err)
		}
		if acct.Funds == nil || acct.Funds.Sign() < 0 {
			t.Errorf("Funds should be >= 0, got %v", acct.Funds)
		}
		t.Logf("initial: funds=%s, lockup=%s, available=%s",
			acct.Funds, acct.LockupCurrent, acct.AvailableFunds())

		// Calculate deposit amount for the upload. Well-funded accounts still
		// deposit 1 atto-USDFC so this full flow keeps direct Deposit coverage.
		dataSize := big.NewInt(testDataSize)
		uploadCosts, err := client.Costs().GetUploadCosts(cctx, addr, []uint64{dataSize.Uint64()},
			&costs.UploadCostOptions{
				ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
				BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
				IsNewDataSet:      true,
			})
		if err != nil {
			t.Fatalf("GetUploadCosts for deposit calculation: %v", err)
		}
		depositAmount := new(big.Int).Set(uploadCosts.DepositNeeded)
		if depositAmount.Sign() <= 0 {
			depositAmount = big.NewInt(1)
		}
		t.Logf("deposit amount: %s (depositNeeded=%s)", depositAmount, uploadCosts.DepositNeeded)

		// Ensure FWSS service approval.
		svcApproval, err := client.Payments().ServiceApproval(cctx, usdfc, addr, fwss)
		if err != nil {
			t.Fatalf("ServiceApproval: %v", err)
		}
		if uploadCosts.Rate.RatePerEpoch == nil {
			t.Fatal("uploadCosts.Rate.RatePerEpoch is nil")
		}
		if uploadCosts.Lockup.Total == nil {
			t.Fatal("uploadCosts.Lockup.Total is nil")
		}
		if !svcApproval.IsApproved {
			maxRate := new(big.Int).Mul(uploadCosts.Rate.RatePerEpoch, big.NewInt(100))
			maxLockup := new(big.Int).Mul(uploadCosts.Lockup.Total, big.NewInt(100))
			maxPeriod := big.NewInt(365 * 24 * 60 * 2) // ~1 year in epochs
			t.Log("start Payments ApproveService")
			res, err := client.Payments().ApproveService(cctx, usdfc, fwss,
				maxRate, maxLockup, maxPeriod,
				payments.WithWait(txWaitTimeout))
			if err != nil {
				t.Fatalf("ApproveService: %v", err)
			}
			if res.Receipt != nil && res.Receipt.Status != 1 {
				t.Fatalf("ApproveService tx failed: status=%d", res.Receipt.Status)
			}
			t.Logf("approved FWSS service operator, tx=%s", res.Hash)
		}

		// Ensure ERC20 allowance for the Payments contract before depositing.
		allowance, err := client.Payments().Allowance(cctx, usdfc, addr, filPay)
		if err != nil {
			t.Fatalf("Allowance: %v", err)
		}
		if allowance.Cmp(depositAmount) < 0 {
			approveAmount := new(big.Int).Mul(depositAmount, big.NewInt(10))
			t.Log("start Payments Approve")
			res, err := client.Payments().Approve(cctx, usdfc, filPay, approveAmount,
				payments.WithWait(txWaitTimeout))
			if err != nil {
				t.Fatalf("Approve: %v", err)
			}
			if res.Receipt != nil && res.Receipt.Status != 1 {
				t.Fatalf("Approve tx failed: status=%d", res.Receipt.Status)
			}
			t.Logf("approved %s USDFC for payments contract, tx=%s", approveAmount, res.Hash)
		}

		balBefore, err := client.Payments().Balance(cctx, usdfc, addr)
		if err != nil {
			t.Fatalf("Balance (before): %v", err)
		}

		t.Log("start Payments Deposit")
		var submitted common.Hash
		res, err := client.Payments().Deposit(cctx, usdfc, addr, depositAmount,
			payments.WithOnSubmitted(func(hash common.Hash) { submitted = hash }))
		if err != nil {
			t.Fatalf("Deposit: %v", err)
		}
		if res == nil || res.Receipt != nil || submitted == (common.Hash{}) || res.Hash != submitted {
			t.Fatalf("Deposit submission = %+v, callback=%s", res, submitted)
		}
		res, err = client.Payments().WaitForTransaction(cctx, submitted, payments.WithWait(txWaitTimeout))
		if err != nil {
			t.Fatalf("WaitForTransaction(Deposit): %v", err)
		}
		if res == nil || res.Receipt == nil || res.Hash != submitted || res.Receipt.TxHash != submitted || res.Receipt.Status != 1 {
			t.Fatalf("Deposit confirmation = %+v, want successful receipt for %s", res, submitted)
		}
		t.Logf("deposited %s USDFC, tx=%s", depositAmount, res.Hash)

		balAfter, err := client.Payments().Balance(cctx, usdfc, addr)
		if err != nil {
			t.Fatalf("Balance (after): %v", err)
		}

		diff := new(big.Int).Sub(balAfter, balBefore)
		shortfall := new(big.Int).Sub(depositAmount, diff)
		maxAccrued := new(big.Int)
		if acct.LockupRate != nil {
			maxEpochs := int64(txWaitTimeout/chain.EpochDuration) + 2
			maxAccrued.Mul(acct.LockupRate, big.NewInt(maxEpochs))
		}
		minDiff := new(big.Int).Sub(depositAmount, maxAccrued)
		if diff.Cmp(minDiff) < 0 {
			t.Errorf("balance diff = %s, want >= deposit minus accrued lockup bound %s", diff, minDiff)
		}
		if diff.Cmp(depositAmount) > 0 {
			t.Errorf("balance diff = %s, want <= deposit %s", diff, depositAmount)
		}
		if shortfall.Sign() > 0 && shortfall.Cmp(maxAccrued) > 0 {
			t.Errorf("balance diff shortfall = %s, want <= accrued lockup bound %s", shortfall, maxAccrued)
		}
		t.Logf("balance: before=%s, after=%s, diff=%s, deposit=%s, shortfall=%s, maxAccrued=%s",
			balBefore, balAfter, diff, depositAmount, shortfall, maxAccrued)
	})

	// --- Upload subtest ---
	t.Run("Upload", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		testData = make([]byte, testDataSize)
		if _, err := crypto_rand.Read(testData); err != nil {
			t.Fatalf("generate test data: %v", err)
		}

		start := time.Now()
		t.Log("start Upload Storage.Upload")
		result, err := client.Storage().Upload(cctx, bytes.NewReader(testData), tracedUploadOptions(t, "Upload", &storage.UploadOptions{
			Copies:          1,
			DataSetMetadata: maps.Clone(uploadMetadata),
			WithCDN:         new(false),
		}))
		t.Logf("done Upload Storage.Upload elapsed=%s", time.Since(start).Round(time.Second))
		if err != nil {
			t.Fatalf("Upload: %v", err)
		}
		if result == nil {
			t.Fatal("Upload returned nil result; no dataset registered for cleanup")
		}
		for _, cp := range result.Copies {
			if !cp.IsNewDataSet || cp.DataSetID.IsZero() {
				t.Fatalf("Upload reused or omitted dataset %s; refusing lifecycle operations", cp.DataSetID)
			}
			metadata, err := retryIntegrationRead(cctx, func(ctx context.Context) (map[string]string, error) {
				return client.WarmStorage().GetAllDataSetMetadata(ctx, cp.DataSetID)
			})
			if err != nil || metadata["run"] != run {
				t.Fatalf("cannot verify dataset %s belongs to run %s: metadata=%v error=%v; refusing cleanup", cp.DataSetID, run, metadata, err)
			}
			id := cp.DataSetID
			if _, registered := dataSetTerminated[id.String()]; registered {
				continue
			}
			dataSetTerminated[id.String()] = false
			suite.Cleanup(func() {
				if dataSetTerminated[id.String()] {
					return
				}
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				res, err := client.WarmStorage().TerminateDataSet(cleanupCtx, id, warmstorage.WithWait(txWaitTimeout))
				if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
					suite.Errorf("cleanup verified run dataset %s: result=%+v error=%v", id, res, err)
					return
				}
				dataSetTerminated[id.String()] = true
				suite.Logf("cleaned up verified run dataset %s tx=%s", id, res.Hash)
			})
		}

		// Upload returns the Curio piece CID boundary value, which is PieceCIDv2.
		if !result.PieceCID.Defined() {
			t.Fatal("PieceCID is undefined")
		}
		if _, err := piece.ParseV2(result.PieceCID); err != nil {
			t.Fatalf("Upload PieceCID must be v2: %v", err)
		}

		if result.Size != int64(testDataSize) {
			t.Errorf("Size = %d, want %d", result.Size, testDataSize)
		}
		if !result.Complete {
			t.Errorf("Complete = false, want true")
		}
		if len(result.Copies) < 1 {
			t.Fatalf("Copies = %d, want >= 1", len(result.Copies))
		}

		for i, cp := range result.Copies {
			if cp.ProviderID.IsZero() {
				t.Errorf("Copy[%d].ProviderID invalid: %s", i, cp.ProviderID)
			}
			if _, err := url.Parse(cp.RetrievalURL); err != nil {
				t.Errorf("Copy[%d].RetrievalURL invalid: %v", i, err)
			}
			if cp.RetrievalURL == "" {
				t.Errorf("Copy[%d].RetrievalURL is empty", i)
			}
		}

		uploadedCID = result.PieceCID
		uploadedURL = result.Copies[0].RetrievalURL
		uploadedDataSetID = result.Copies[0].DataSetID
		uploadedProviderID = result.Copies[0].ProviderID
		t.Logf("uploaded: cid=%s, size=%d, copies=%d, url=%s, dataSet=%s",
			result.PieceCID, result.Size, len(result.Copies), uploadedURL, uploadedDataSetID)

		if !uploadedDataSetID.IsZero() {
			dsInfo, derr := client.WarmStorage().GetDataSet(cctx, uploadedDataSetID)
			if derr != nil {
				t.Logf("GetDataSet(%s): %v", uploadedDataSetID, derr)
			} else {
				uploadedRailID = dsInfo.PDPRailID
				t.Logf("dataSet rails: pdp=%s cacheMiss=%s cdn=%s", dsInfo.PDPRailID, dsInfo.CacheMissRailID, dsInfo.CDNRailID)
			}
		}

		if len(result.FailedAttempts) > 0 {
			for _, fa := range result.FailedAttempts {
				t.Logf("  failed attempt: provider=%s, role=%s, stage=%s, err=%v",
					fa.ProviderID, fa.Role, fa.Stage, fa.Err)
			}
		}
	})

	t.Run("MetadataSelection", func(t *testing.T) {
		if uploadedDataSetID.IsZero() || uploadedProviderID.IsZero() {
			t.Fatal("Upload did not produce a data set and provider; metadata selection cannot be verified")
		}
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		dataSet, err := retryIntegrationRead(cctx, func(ctx context.Context) (*warmstorage.DataSetInfo, error) {
			return client.WarmStorage().GetDataSet(ctx, uploadedDataSetID)
		})
		if err != nil {
			t.Fatalf("GetDataSet(%s): %v", uploadedDataSetID, err)
		}
		if dataSet == nil || !dataSet.DataSetID.Equal(uploadedDataSetID) || dataSet.Payer != addr || !dataSet.ProviderID.Equal(uploadedProviderID) {
			t.Fatalf("uploaded data set identity = %+v, want dataSet=%s payer=%s provider=%s", dataSet, uploadedDataSetID, addr, uploadedProviderID)
		}
		metadata, err := retryIntegrationRead(cctx, func(ctx context.Context) (map[string]string, error) {
			return client.WarmStorage().GetAllDataSetMetadata(ctx, uploadedDataSetID)
		})
		if err != nil || !maps.Equal(metadata, uploadMetadata) {
			t.Fatalf("uploaded metadata = %v, want %v: %v", metadata, uploadMetadata, err)
		}
		providers, err := retryIntegrationRead(cctx, func(ctx context.Context) ([]spregistry.PDPProvider, error) {
			return client.SPRegistry().SelectActivePDPProviders(ctx, spregistry.ProviderFilter{})
		})
		if err != nil {
			t.Fatalf("SelectActivePDPProviders: %v", err)
		}
		var excluded []types.BigInt
		for _, provider := range providers {
			if !provider.Info.ID.Equal(uploadedProviderID) {
				excluded = append(excluded, provider.Info.ID)
			}
		}
		selectTarget := func(t *testing.T, ctx context.Context, requested map[string]string, withCDN bool) storage.StorageContext {
			t.Helper()
			selection, err := retryIntegrationRead(ctx, func(ctx context.Context) (*storage.UploadContextSelection, error) {
				return client.Storage().SelectUploadContexts(ctx, storage.SelectUploadContextsOptions{
					Copies:                 1,
					DataSetMetadata:        maps.Clone(requested),
					ExcludeProviderIDs:     excluded,
					AllowUnendorsedPrimary: true,
					WithCDN:                &withCDN,
				})
			})
			if err != nil || selection == nil || !selection.Complete || len(selection.Contexts) != 1 || selection.Contexts[0] == nil {
				t.Fatalf("SelectUploadContexts(metadata=%v, withCDN=%t) = %+v: %v", requested, withCDN, selection, err)
			}
			target := selection.Contexts[0]
			expected := maps.Clone(requested)
			if withCDN {
				expected["withCDN"] = ""
			}
			if !target.ProviderID().Equal(uploadedProviderID) || target.CDNEnabled() != withCDN || !maps.Equal(target.DataSetMetadata(), expected) {
				t.Fatalf("selected provider=%s CDN=%t metadata=%v, want provider=%s CDN=%t metadata=%v", target.ProviderID(), target.CDNEnabled(), target.DataSetMetadata(), uploadedProviderID, withCDN, expected)
			}
			return target
		}

		matchCtx, cancelMatch := context.WithTimeout(cctx, 60*time.Second)
		defer cancelMatch()
		for {
			target := selectTarget(t, matchCtx, uploadMetadata, false)
			if ref, bound := target.DataSetRef(); bound {
				if !ref.DataSetID().Equal(uploadedDataSetID) {
					t.Fatalf("matching metadata selected data set %s, want %s", ref.DataSetID(), uploadedDataSetID)
				}
				break
			}
			select {
			case <-matchCtx.Done():
				t.Fatalf("uploaded data set %s was not selected within 60 seconds: %v", uploadedDataSetID, matchCtx.Err())
			case <-time.After(2 * time.Second):
			}
		}

		for _, tc := range []struct {
			name    string
			key     string
			withCDN bool
		}{
			{name: "DifferentEmptyKey", key: "otherEmpty"},
			{name: "CDNRequiresMatchingKey", withCDN: true},
		} {
			t.Run(tc.name, func(t *testing.T) {
				requested := maps.Clone(uploadMetadata)
				delete(requested, "emptyFlag")
				if tc.key != "" {
					requested[tc.key] = ""
				}
				target := selectTarget(t, cctx, requested, tc.withCDN)
				if ref, bound := target.DataSetRef(); bound {
					t.Fatalf("nonmatching metadata reused data set %s, want an unbound provider context", ref.DataSetID())
				}
			})
		}
	})

	// --- Download subtest ---
	t.Run("Download", func(t *testing.T) {
		if !uploadedCID.Defined() || uploadedURL == "" {
			t.Skip("Upload subtest did not produce CID/URL; skipping Download")
		}

		cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()

		// Piece may not be immediately available after upload; retry with backoff.
		var downloaded []byte
		var lastErr error
		for attempt := range 5 {
			if attempt > 0 {
				delay := 30 * time.Second
				t.Logf("download attempt %d failed, retrying in %s: %v", attempt, delay, lastErr)
				select {
				case <-time.After(delay):
				case <-cctx.Done():
					t.Fatalf("context expired waiting for download: %v", cctx.Err())
				}
			}

			start := time.Now()
			t.Logf("start Download attempt %d", attempt+1)
			rc, err := client.Storage().Download(cctx, uploadedCID, &storage.DownloadOptions{
				URL: uploadedURL,
			})
			if err != nil {
				t.Logf("done Download attempt %d elapsed=%s", attempt+1, time.Since(start).Round(time.Second))
				lastErr = err
				continue
			}

			data, err := io.ReadAll(rc)
			_ = rc.Close()
			t.Logf("done Download attempt %d elapsed=%s", attempt+1, time.Since(start).Round(time.Second))
			if err != nil {
				lastErr = err
				continue
			}
			downloaded = data
			lastErr = nil
			break
		}
		if lastErr != nil {
			t.Fatalf("Download not available after retries: %v", lastErr)
		}

		if len(downloaded) != len(testData) {
			t.Fatalf("downloaded size = %d, want %d", len(downloaded), len(testData))
		}
		if !bytes.Equal(downloaded, testData) {
			t.Fatal("downloaded data does not match uploaded data")
		}
		t.Logf("download verified: %d bytes match", len(downloaded))
		limited, err := storage.New(storage.Options{DownloadMaxBytes: testDataSize / 2, AllowPrivateNetworks: true})
		if err != nil {
			t.Fatalf("storage.New with URL limit: %v", err)
		}
		body, err := limited.Download(cctx, uploadedCID, &storage.DownloadOptions{URL: uploadedURL})
		var got []byte
		if body != nil {
			got, lastErr = io.ReadAll(body)
			_ = body.Close()
			if err == nil {
				err = lastErr
			}
		}
		if !errors.Is(err, storage.ErrMaxBytesExceeded) || len(got) > testDataSize/2 {
			t.Fatalf("limited URL download bytes=%d error=%v, want ErrMaxBytesExceeded and at most %d bytes", len(got), err, testDataSize/2)
		}
	})

	// --- Multicopy subtest ---
	t.Run("Multicopy", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 12*time.Minute)
		defer cancel()

		perCopyCosts, err := client.Costs().GetUploadCosts(cctx, addr, []uint64{testDataSize}, &costs.UploadCostOptions{
			ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
			BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
			EnableCDN:         true,
			IsNewDataSet:      true,
		})
		if err != nil {
			t.Fatalf("GetUploadCosts (multicopy): %v", err)
		}
		acct, err := client.Payments().AccountInfo(cctx, usdfc, addr)
		if err != nil {
			t.Fatalf("AccountInfo (multicopy): %v", err)
		}
		multiCosts := aggregateNewUploadCosts(perCopyCosts, acct, 2)
		t.Logf("multicopy funding: depositNeeded=%s, totalLockup=%s, rate/epoch=%s, ready=%v",
			multiCosts.DepositNeeded, multiCosts.Lockup.Total, multiCosts.Rate.RatePerEpoch, multiCosts.Ready)
		if multiCosts.DepositNeeded.Sign() > 0 {
			allowance, err := client.Payments().Allowance(cctx, usdfc, addr, filPay)
			if err != nil {
				t.Fatalf("Allowance (multicopy): %v", err)
			}
			if allowance.Cmp(multiCosts.DepositNeeded) < 0 {
				approveAmount := new(big.Int).Mul(multiCosts.DepositNeeded, big.NewInt(10))
				t.Log("start Multicopy Approve")
				res, err := client.Payments().Approve(cctx, usdfc, filPay, approveAmount,
					payments.WithWait(txWaitTimeout))
				if err != nil {
					t.Fatalf("Approve (multicopy): %v", err)
				}
				if res.Receipt != nil && res.Receipt.Status != 1 {
					t.Fatalf("Approve tx failed (multicopy): status=%d", res.Receipt.Status)
				}
				t.Logf("approved %s USDFC for multicopy, tx=%s", approveAmount, res.Hash)
			}

			t.Log("start Multicopy Deposit")
			res, err := client.Payments().Deposit(cctx, usdfc, addr, multiCosts.DepositNeeded,
				payments.WithWait(txWaitTimeout))
			if err != nil {
				t.Fatalf("Deposit (multicopy): %v", err)
			}
			if res.Receipt != nil && res.Receipt.Status != 1 {
				t.Fatalf("Deposit tx failed (multicopy): status=%d", res.Receipt.Status)
			}
			t.Logf("deposited %s USDFC for multicopy, tx=%s", multiCosts.DepositNeeded, res.Hash)
		}

		multiData := make([]byte, testDataSize)
		if _, err := crypto_rand.Read(multiData); err != nil {
			t.Fatalf("generate multicopy data: %v", err)
		}

		withCDN := true
		start := time.Now()
		t.Log("start Multicopy Storage.Upload")
		result, err := client.Storage().Upload(cctx, bytes.NewReader(multiData), tracedUploadOptions(t, "Multicopy", &storage.UploadOptions{
			Copies:  2,
			WithCDN: &withCDN,
		}))
		t.Logf("done Multicopy Storage.Upload elapsed=%s", time.Since(start).Round(time.Second))
		if err != nil {
			t.Fatalf("Upload (multicopy) failed: %v", err)
		}

		if !result.PieceCID.Defined() {
			t.Fatal("PieceCID is undefined")
		}

		if len(result.Copies) < 2 {
			t.Errorf("Copies = %d, want >= 2 (requested 2)", len(result.Copies))
		}

		// Verify all providers are distinct.
		seen := make(map[string]struct{})
		for i, cp := range result.Copies {
			if cp.ProviderID.IsZero() {
				t.Errorf("Copy[%d].ProviderID is zero", i)
				continue
			}
			pid := cp.ProviderID
			key := idconv.Key(pid)
			if _, dup := seen[key]; dup {
				t.Errorf("Copy[%d] has duplicate ProviderID: %s", i, pid)
			}
			seen[key] = struct{}{}

			if cp.RetrievalURL == "" {
				t.Errorf("Copy[%d].RetrievalURL is empty", i)
			}
			if _, err := url.Parse(cp.RetrievalURL); err != nil {
				t.Errorf("Copy[%d].RetrievalURL invalid: %v", i, err)
			}
		}

		t.Logf("multicopy: cid=%s, copies=%d/%d",
			result.PieceCID, len(result.Copies), result.RequestedCopies)
		for i, cp := range result.Copies {
			t.Logf("  copy[%d]: provider=%s, role=%s, url=%s",
				i, cp.ProviderID, cp.Role, cp.RetrievalURL)
		}
		if len(result.FailedAttempts) > 0 {
			for _, fa := range result.FailedAttempts {
				t.Logf("  failed: provider=%s, role=%s, stage=%s, err=%v",
					fa.ProviderID, fa.Role, fa.Stage, fa.Err)
			}
		}
	})

	// --- SessionKey subtest ---
	t.Run("SessionKey", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		// Generate an ephemeral session key pair.
		sessionKeyPriv, err := ecdsa.GenerateKey(crypto.S256(), crypto_rand.Reader)
		if err != nil {
			t.Fatalf("generate session key: %v", err)
		}
		sessionKeyAddr := crypto.PubkeyToAddress(sessionKeyPriv.PublicKey)
		t.Logf("session key address: %s", sessionKeyAddr)

		// Login (authorize session key without funding).
		t.Log("start SessionKey Login")
		loginRes, err := client.SessionKey().Login(cctx, sessionKeyAddr,
			sessionkey.WithWait(txWaitTimeout))
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if loginRes.Receipt != nil && loginRes.Receipt.Status != 1 {
			t.Fatalf("Login tx failed: status=%d", loginRes.Receipt.Status)
		}
		t.Logf("login tx=%s", loginRes.Hash)

		// Verify the session key is authorized.
		expiry, err := client.SessionKey().AuthorizationExpiry(
			cctx, addr, sessionKeyAddr, sessionkey.CreateDataSetPermission(),
		)
		if err != nil {
			t.Fatalf("AuthorizationExpiry: %v", err)
		}
		if expiry == 0 {
			t.Error("expiry = 0, want > 0 after login")
		}
		t.Logf("CreateDataSet permission expiry: %d", expiry)

		// Verify not expired.
		isExpired, err := client.SessionKey().IsExpired(
			cctx, addr, sessionKeyAddr, sessionkey.CreateDataSetPermission(),
		)
		if err != nil {
			t.Fatalf("IsExpired (before revoke): %v", err)
		}
		if isExpired {
			t.Error("session key should not be expired immediately after login")
		}

		// Revoke the session key.
		t.Log("start SessionKey Revoke")
		revokeRes, err := client.SessionKey().Revoke(cctx, sessionKeyAddr,
			sessionkey.WithWait(txWaitTimeout))
		if err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		if revokeRes.Receipt != nil && revokeRes.Receipt.Status != 1 {
			t.Fatalf("Revoke tx failed: status=%d", revokeRes.Receipt.Status)
		}
		t.Logf("revoke tx=%s", revokeRes.Hash)

		// Verify the session key is now expired.
		isExpiredAfter, err := client.SessionKey().IsExpired(
			cctx, addr, sessionKeyAddr, sessionkey.CreateDataSetPermission(),
		)
		if err != nil {
			t.Fatalf("IsExpired (after revoke): %v", err)
		}
		if !isExpiredAfter {
			t.Error("session key should be expired after revoke")
		}
		t.Log("session key lifecycle: login → verify → revoke → confirm expired ✓")
	})

	// --- ClientSmoke: every synapse.Client service getter + provider lookups. ---
	t.Run("ClientSmoke", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		if client.Chain().ChainID() != calibrationChainID {
			t.Errorf("ChainID = %d, want %d", client.Chain().ChainID(), calibrationChainID)
		}
		if client.Address() != addr {
			t.Errorf("Address mismatch")
		}
		if client.WarmStorage() == nil || client.SPRegistry() == nil ||
			client.Payments() == nil || client.SessionKey() == nil ||
			client.Costs() == nil || client.FilBeam() == nil ||
			client.Storage() == nil {
			t.Fatal("one or more service getters returned nil")
		}

		page, err := client.SPRegistry().GetPDPProviders(cctx, true, types.ListOptions{Limit: 1})
		if err != nil {
			t.Fatalf("GetPDPProviders: %v", err)
		}
		if page == nil || len(page.Providers) == 0 {
			t.Skip("no active PDP providers on calibration; skipping provider lookup")
		}
		first := page.Providers[0]
		info, err := client.GetProviderInfoByID(cctx, first.Info.ID)
		if err != nil {
			t.Fatalf("GetProviderInfoByID(%s): %v", first.Info.ID, err)
		}
		if !info.ID.Equal(first.Info.ID) {
			t.Errorf("provider id mismatch: got %s want %s", info.ID, first.Info.ID)
		}
		info2, err := client.GetProviderInfoByAddress(cctx, info.ServiceProvider)
		if err != nil {
			t.Fatalf("GetProviderInfoByAddress(%s): %v", info.ServiceProvider, err)
		}
		if !info2.ID.Equal(info.ID) {
			t.Errorf("address-lookup mismatch: got %s want %s", info2.ID, info.ID)
		}
	})

	// --- StorageManagerSurface: every storage.Service read method. ---
	t.Run("StorageManagerSurface", func(t *testing.T) {
		sm := client.Storage()
		checkRead := func(t *testing.T, label string, err error) {
			t.Helper()
			if err == nil {
				return
			}
			t.Fatalf("%s: %v", label, err)
		}

		t.Run("FindDataSets", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			sets, err := retryIntegrationRead(cctx, func(ctx context.Context) ([]*storage.DataSetDetails, error) {
				return sm.FindDataSets(ctx, nil)
			})
			checkRead(t, "FindDataSets(default)", err)
			t.Logf("FindDataSets(default): %d", len(sets))

			start := time.Now()
			setsManaged, err := retryIntegrationRead(cctx, func(ctx context.Context) ([]*storage.DataSetDetails, error) {
				return sm.FindDataSets(ctx, &storage.FindDataSetsOptions{OnlyManaged: true})
			})
			t.Logf("done FindDataSets(onlyManaged) elapsed=%s", time.Since(start).Round(time.Second))
			checkRead(t, "FindDataSets(onlyManaged)", err)
			if len(setsManaged) > len(sets) {
				t.Errorf("OnlyManaged returned %d > total %d", len(setsManaged), len(sets))
			}
		})

		t.Run("GetStorageInfo", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			info, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.StorageInfo, error) {
				return sm.GetStorageInfo(ctx, nil)
			})
			checkRead(t, "GetStorageInfo", err)
			if len(info.Providers) == 0 {
				t.Error("StorageInfo.Providers empty")
			}
			if info.Allowances == nil {
				t.Error("StorageInfo.Allowances nil for configured signer")
			}
		})

		t.Run("Prepare", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			selection, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.UploadContextSelection, error) {
				return sm.SelectUploadContexts(ctx, storage.SelectUploadContextsOptions{Copies: 1})
			})
			checkRead(t, "SelectUploadContexts(Prepare prerequisite)", err)
			prep, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.PrepareResult, error) {
				return sm.Prepare(ctx, &storage.PrepareOptions{
					PieceSizes: []uint64{64 * 1024},
					Contexts:   selection.Contexts,
				})
			})
			checkRead(t, "Prepare", err)
			if prep == nil || prep.Costs == nil {
				t.Fatal("Prepare returned nil result/costs")
			}
			if prep.Transaction != nil {
				t.Logf("Prepare reports funding required: deposit=%s includesApproval=%v",
					prep.Transaction.DepositAmount, prep.Transaction.IncludesApproval)
			}
		})

		t.Run("CalculateMultiContextCosts", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			info, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.StorageInfo, error) {
				return sm.GetStorageInfo(ctx, nil)
			})
			checkRead(t, "GetStorageInfo(prerequisite)", err)
			if len(info.Providers) == 0 {
				t.Fatal("StorageInfo.Providers empty")
			}
			mc, err := retryIntegrationRead(cctx, func(ctx context.Context) (*costs.MultiContextCosts, error) {
				return sm.CalculateMultiContextCosts(ctx, []uint64{64 * 1024}, []storage.ContextCostRef{
					{Provider: storage.Provider{ID: info.Providers[0].Info.ID}, WithCDN: false},
				}, storage.MultiCostOptions{}, addr)
			})
			checkRead(t, "CalculateMultiContextCosts", err)
			if mc == nil || mc.RatePerEpoch == nil {
				t.Fatal("MultiContextCosts missing RatePerEpoch")
			}
		})

		t.Run("SelectProviderContext", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			def, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.ProviderContext, error) {
				return sm.SelectProviderContext(ctx, storage.SelectProviderContextOptions{})
			})
			checkRead(t, "SelectProviderContext", err)
			if def == nil || def.ProviderID().IsZero() {
				t.Fatal("SelectProviderContext returned invalid context")
			}
		})

		t.Run("SelectUploadContexts", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			selection, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.UploadContextSelection, error) {
				return sm.SelectUploadContexts(ctx, storage.SelectUploadContextsOptions{Copies: 1})
			})
			checkRead(t, "SelectUploadContexts", err)
			if selection == nil || len(selection.Contexts) != 1 {
				t.Errorf("SelectUploadContexts returned %+v, want one context", selection)
			}
		})

		t.Run("NewProviderContext", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()
			selected, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.ProviderContext, error) {
				return sm.SelectProviderContext(ctx, storage.SelectProviderContextOptions{})
			})
			checkRead(t, "SelectProviderContext(NewProviderContext prerequisite)", err)
			single, err := retryIntegrationRead(cctx, func(ctx context.Context) (*storage.ProviderContext, error) {
				return sm.NewProviderContext(ctx, selected.ProviderID(), storage.NewProviderContextOptions{})
			})
			checkRead(t, "NewProviderContext", err)
			if single == nil || single.ProviderID().IsZero() {
				t.Fatal("NewProviderContext returned invalid context")
			}
		})
	})

	// --- ContextInspection: read methods on the DataSetContext produced for the upload. ---
	t.Run("ContextInspection", func(t *testing.T) {
		if uploadedDataSetID.IsZero() || !uploadedCID.Defined() {
			t.Skip("Upload subtest did not produce dataset/cid; skipping ContextInspection")
		}
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()

		uctx, err := client.Storage().NewDataSetContext(cctx, uploadedDataSetID, storage.NewDataSetContextOptions{})
		if err != nil {
			t.Fatalf("NewDataSetContext(uploadedDataSetID): %v", err)
		}

		if uctx.ProviderID().IsZero() {
			t.Errorf("ProviderID == 0")
		}
		if uctx.ServiceURL() == "" {
			t.Errorf("ServiceURL empty")
		}
		if got := uctx.DataSetID(); !got.Equal(uploadedDataSetID) {
			t.Errorf("DataSetID = %v, want %s", got, uploadedDataSetID)
		}
		_ = uctx.CDNEnabled()
		prov := uctx.GetProviderInfo()
		if prov.ID.IsZero() {
			t.Errorf("Provider.ID == 0")
		}
		if u := uctx.PieceURL(uploadedCID); u == "" {
			t.Errorf("PieceURL empty")
		}

		ps, err := uctx.PieceStatus(cctx, uploadedCID)
		if err != nil {
			t.Fatalf("PieceStatus: %v", err)
		}
		if ps == nil {
			t.Fatal("PieceStatus returned nil")
		}
		if !ps.Exists {
			t.Fatal("confirmed upload is absent from PieceStatus")
		}
		if ps.HoursUntilNextChallenge < 0 {
			t.Fatalf("HoursUntilNextChallenge = %v", ps.HoursUntilNextChallenge)
		}
		if !ps.DataSetEstimatedProofDeadline.IsZero() &&
			!ps.DataSetEstimatedProofDeadline.After(ps.DataSetNextChallengeAt) {
			t.Fatalf("estimated deadline %s is not after challenge %s", ps.DataSetEstimatedProofDeadline, ps.DataSetNextChallengeAt)
		}

		removals, err := uctx.GetScheduledRemovals(cctx)
		if err != nil {
			t.Fatalf("GetScheduledRemovals: %v", err)
		}
		t.Logf("scheduled removals: %d", len(removals))

		// DataSetContext-level Download.
		start := time.Now()
		t.Log("start ContextInspection Download")
		rc, err := uctx.Download(cctx, uploadedCID)
		if err != nil {
			t.Logf("done ContextInspection Download elapsed=%s", time.Since(start).Round(time.Second))
			t.Fatalf("DataSetContext.Download: %v", err)
		}
		defer func() { _ = rc.Close() }()
		got, err := io.ReadAll(rc)
		t.Logf("done ContextInspection Download elapsed=%s", time.Since(start).Round(time.Second))
		if err != nil {
			t.Fatalf("read downloaded: %v", err)
		}
		if int64(len(got)) != int64(len(testData)) {
			t.Errorf("downloaded size = %d, want %d", len(got), len(testData))
		}
	})

	// --- PDPDirect: direct provider HTTP reads for the uploaded fixture. ---
	t.Run("PDPDirect", func(t *testing.T) {
		if uploadedDataSetID.IsZero() || !uploadedCID.Defined() {
			t.Skip("Upload subtest did not produce dataset/cid; skipping direct PDP reads")
		}
		cctx, cancel := context.WithTimeout(ctx, 4*time.Minute)
		defer cancel()

		uctx, err := client.Storage().NewDataSetContext(cctx, uploadedDataSetID, storage.NewDataSetContextOptions{})
		if err != nil {
			t.Fatalf("NewDataSetContext(uploadedDataSetID): %v", err)
		}
		provider, err := pdp.New(uctx.ServiceURL())
		if err != nil {
			t.Fatalf("pdp.New: %v", err)
		}
		if err := provider.Ping(cctx); err != nil {
			t.Fatalf("pdp.Ping: %v", err)
		}

		deadline := time.Now().Add(3 * time.Minute)
		var dataSet *pdp.DataSet
		for {
			dataSet, err = provider.GetDataSet(cctx, uploadedDataSetID)
			if err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("pdp.GetDataSet(%s) was not indexed within 3m: %v", uploadedDataSetID, err)
			}
			select {
			case <-cctx.Done():
				t.Fatalf("wait for pdp.GetDataSet: %v", cctx.Err())
			case <-time.After(2 * time.Second):
			}
		}
		if dataSet == nil || !dataSet.ID.Equal(uploadedDataSetID) {
			t.Fatalf("pdp.GetDataSet returned %+v, want id %s", dataSet, uploadedDataSetID)
		}
		foundInDataSet := false
		for _, dataSetPiece := range dataSet.Pieces {
			if dataSetPiece.PieceCID == uploadedCID.String() {
				foundInDataSet = true
				break
			}
		}
		if !foundInDataSet {
			t.Fatalf("pdp.GetDataSet(%s) does not contain piece %s", uploadedDataSetID, uploadedCID)
		}

		deadline = time.Now().Add(3 * time.Minute)
		var found *pdp.FindPieceResult
		for {
			found, err = provider.FindPiece(cctx, uploadedCID)
			if err == nil {
				break
			}
			if !errors.Is(err, pdp.ErrPieceNotFound) && !errors.Is(err, pdp.ErrPieceProcessing) && !isIntegrationReadTransient(err) {
				t.Fatalf("pdp.FindPiece(%s): %v", uploadedCID, err)
			}
			if time.Now().After(deadline) {
				t.Fatalf("pdp.FindPiece(%s) was not ready within 3m: %v", uploadedCID, err)
			}
			select {
			case <-cctx.Done():
				t.Fatalf("wait for pdp.FindPiece: %v", cctx.Err())
			case <-time.After(2 * time.Second):
			}
		}
		if found == nil || found.PieceCID != uploadedCID.String() {
			t.Fatalf("pdp.FindPiece returned %+v, want %s", found, uploadedCID)
		}

		rc, contentLength, err := provider.DownloadPiece(cctx, uploadedCID)
		if err != nil {
			t.Fatalf("pdp.DownloadPiece(%s): %v", uploadedCID, err)
		}
		downloaded, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			t.Fatalf("read pdp.DownloadPiece: %v", readErr)
		}
		if closeErr != nil {
			t.Fatalf("close pdp.DownloadPiece: %v", closeErr)
		}
		if contentLength >= 0 && contentLength != int64(len(testData)) {
			t.Errorf("pdp.DownloadPiece content length = %d, want %d", contentLength, len(testData))
		}
		if !bytes.Equal(downloaded, testData) {
			t.Fatal("pdp.DownloadPiece data mismatch")
		}
	})

	// --- ContextUploadBatchingNewDataSet: the managed default batcher combines
	// concurrent ProviderContext uploads into one create-and-add transaction,
	// and a later upload to the same target adds to that data set. ---
	t.Run("ContextUploadBatchingNewDataSet", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		defer cancel()

		batchClient := integrationtest.NewDefaultClient(t, cctx, synapse.WithUploadBatching())
		metadata := map[string]string{
			"source": "integration-new-dataset-batch",
			"run":    fmt.Sprintf("%d", time.Now().UnixNano()),
		}
		providerCtx, err := batchClient.Storage().SelectProviderContext(cctx, storage.SelectProviderContextOptions{
			DataSetMetadata: metadata,
		})
		if err != nil {
			t.Fatalf("SelectProviderContext(new dataset batch): %v", err)
		}
		if providerCtx == nil || providerCtx.ProviderID().IsZero() {
			t.Fatalf("SelectProviderContext(new dataset batch) returned %+v", providerCtx)
		}
		if _, bound := providerCtx.DataSetRef(); bound {
			t.Fatal("selected ProviderContext unexpectedly started bound")
		}

		payloads := make([][]byte, 2)
		for i := range payloads {
			payloads[i] = make([]byte, 128*1024)
			if _, err := crypto_rand.Read(payloads[i]); err != nil {
				t.Fatalf("generate new-dataset batch upload data %d: %v", i, err)
			}
		}
		sequentialPayload := make([]byte, 128*1024)
		if _, err := crypto_rand.Read(sequentialPayload); err != nil {
			t.Fatalf("generate sequential new-dataset batch upload data: %v", err)
		}

		t.Log("start NewDataSetBatch Prepare")
		prep, err := batchClient.Storage().Prepare(cctx, &storage.PrepareOptions{
			PieceSizes:        []uint64{uint64(len(payloads[0])), uint64(len(payloads[1])), uint64(len(sequentialPayload))},
			ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
			BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
			Contexts:          []storage.StorageContext{providerCtx},
		})
		if err != nil {
			t.Fatalf("Prepare(new dataset batch): %v", err)
		}
		if prep.Transaction != nil {
			t.Log("start NewDataSetBatch Prepare.Execute")
			res, err := prep.Transaction.Execute(cctx, payments.WithWait(txWaitTimeout))
			if err != nil {
				if errors.Is(err, payments.ErrPermitUnsupported) {
					t.Skip("needs-usdfc-permit-support: new-dataset batch funding requires permit support")
				}
				t.Fatalf("Prepare(new dataset batch).Execute: %v", err)
			}
			if res.Receipt == nil || res.Receipt.Status != 1 {
				t.Fatalf("Prepare(new dataset batch).Execute receipt = %+v", res.Receipt)
			}
		}

		type uploadOutcome struct {
			index  int
			result *storage.UploadResult
			err    error
		}
		stored := make(chan int, len(payloads))
		releaseStored := make(chan struct{})
		var releaseStoredOnce sync.Once
		releaseStores := func() { releaseStoredOnce.Do(func() { close(releaseStored) }) }
		defer releaseStores()
		outcomes := make(chan uploadOutcome, len(payloads))
		txHashes := make([]string, len(payloads))
		start := time.Now()
		t.Log("start NewDataSetBatch concurrent ProviderContext.Upload")
		for i := range payloads {
			index := i
			opts := tracedContextUploadOptions(t, fmt.Sprintf("NewDataSetBatch[%d]", index), &storage.ContextUploadOptions{
				OnStored: func(types.BigInt, cid.Cid) {
					stored <- index
					<-releaseStored
				},
				OnPiecesAdded: func(txHash string, _ types.BigInt, pieces []storage.SubmittedPiece) {
					txHashes[index] = txHash
					if len(pieces) != 1 {
						t.Errorf("upload %d OnPiecesAdded pieces=%d, want 1", index, len(pieces))
					}
				},
			})
			go func() {
				result, err := providerCtx.Upload(cctx, bytes.NewReader(payloads[index]), opts)
				outcomes <- uploadOutcome{index: index, result: result, err: err}
			}()
		}
		seenStored := make(map[int]struct{}, len(payloads))
		for len(seenStored) < len(payloads) {
			select {
			case index := <-stored:
				seenStored[index] = struct{}{}
			case outcome := <-outcomes:
				if outcome.err != nil {
					t.Fatalf("ProviderContext.Upload(new dataset batch %d) before barrier: %v", outcome.index, outcome.err)
				}
				t.Fatalf("ProviderContext.Upload(new dataset batch %d) returned before OnStored barrier", outcome.index)
			case <-cctx.Done():
				t.Fatalf("wait for concurrent new-dataset stores: %v", cctx.Err())
			}
		}
		releaseStores()

		results := make([]*storage.UploadResult, len(payloads))
		for range payloads {
			select {
			case outcome := <-outcomes:
				if outcome.err != nil {
					t.Fatalf("ProviderContext.Upload(new dataset batch %d): %v", outcome.index, outcome.err)
				}
				results[outcome.index] = outcome.result
			case <-cctx.Done():
				t.Fatalf("wait for concurrent new-dataset uploads: %v", cctx.Err())
			}
		}
		t.Logf("done NewDataSetBatch concurrent ProviderContext.Upload elapsed=%s", time.Since(start).Round(time.Second))

		var dataSetID types.BigInt
		for i, result := range results {
			if result == nil || !result.PieceCID.Defined() || len(result.Copies) != 1 {
				t.Fatalf("ProviderContext.Upload(new dataset batch %d) result=%+v", i, result)
			}
			info, err := piece.ParseV2(result.PieceCID)
			if err != nil || info.RawSize != uint64(len(payloads[i])) {
				t.Fatalf("upload %d PieceCID info=%+v error=%v, want raw size %d", i, info, err, len(payloads[i]))
			}
			copy0 := result.Copies[0]
			if !copy0.ProviderID.Equal(providerCtx.ProviderID()) || !copy0.IsNewDataSet || copy0.DataSetID.IsZero() {
				t.Fatalf("upload %d copy=%+v, want new data set on provider %s", i, copy0, providerCtx.ProviderID())
			}
			if i == 0 {
				dataSetID = copy0.DataSetID
				cleanupDataSetID := dataSetID
				t.Cleanup(func() {
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
					defer cleanupCancel()
					t.Logf("start NewDataSetBatch cleanup TerminateDataSet(%s)", cleanupDataSetID)
					termRes, err := batchClient.WarmStorage().TerminateDataSet(cleanupCtx, cleanupDataSetID, warmstorage.WithWait(txWaitTimeout))
					if err != nil {
						t.Logf("cleanup TerminateDataSet(%s): %v", cleanupDataSetID, err)
						return
					}
					if termRes == nil {
						t.Logf("cleanup TerminateDataSet(%s) returned nil result", cleanupDataSetID)
						return
					}
					if termRes.Receipt != nil && termRes.Receipt.Status != 1 {
						t.Logf("cleanup TerminateDataSet(%s) receipt = %+v", cleanupDataSetID, termRes.Receipt)
					}
					t.Logf("done NewDataSetBatch cleanup TerminateDataSet(%s) tx=%s", cleanupDataSetID, termRes.Hash)
				})
			} else if !copy0.DataSetID.Equal(dataSetID) {
				t.Fatalf("batch DataSetIDs=%s,%s, want one shared data set", dataSetID, copy0.DataSetID)
			}
		}
		if txHashes[0] == "" || txHashes[0] != txHashes[1] {
			t.Fatalf("batch transaction IDs=%q,%q, want one shared non-empty transaction", txHashes[0], txHashes[1])
		}
		if results[0].Copies[0].PieceID.Equal(results[1].Copies[0].PieceID) {
			t.Fatalf("batch PieceIDs=%s,%s, want distinct IDs", results[0].Copies[0].PieceID, results[1].Copies[0].PieceID)
		}

		start = time.Now()
		t.Log("start NewDataSetBatch sequential ProviderContext.Upload")
		sequential, err := providerCtx.Upload(cctx, bytes.NewReader(sequentialPayload), tracedContextUploadOptions(t, "NewDataSetBatchSequential", nil))
		if err != nil {
			t.Fatalf("ProviderContext.Upload(sequential new dataset batch): %v", err)
		}
		t.Logf("done NewDataSetBatch sequential ProviderContext.Upload elapsed=%s", time.Since(start).Round(time.Second))
		if sequential == nil || len(sequential.Copies) != 1 {
			t.Fatalf("ProviderContext.Upload(sequential new dataset batch) result=%+v", sequential)
		}
		if copy0 := sequential.Copies[0]; !copy0.DataSetID.Equal(dataSetID) || copy0.IsNewDataSet {
			t.Fatalf("sequential copy=%+v, want pieces added to shared data set %s", copy0, dataSetID)
		}

		providerID := providerCtx.ProviderID()
		resolved, err := batchClient.Storage().NewDataSetContext(cctx, dataSetID, storage.NewDataSetContextOptions{
			ProviderID: &providerID,
		})
		if err != nil {
			t.Fatalf("NewDataSetContext(new dataset batch): %v", err)
		}
		ref, ok := resolved.DataSetRef()
		if !ok || !ref.ProviderID().Equal(providerCtx.ProviderID()) || !ref.DataSetID().Equal(dataSetID) {
			t.Fatalf("resolved DataSetRef=(%+v, %t), want provider=%s dataSet=%s", ref, ok, providerCtx.ProviderID(), dataSetID)
		}
		t.Logf("NewDataSetBatch resolved DataSetRef: provider=%s dataSet=%s clientDataSet=%s", ref.ProviderID(), ref.DataSetID(), ref.ClientDataSetID())
		if _, bound := providerCtx.DataSetRef(); bound {
			t.Fatal("ProviderContext was mutated into a bound context")
		}
	})

	// --- UploadBatchingMultiCopyNewDataSets: concurrent multi-copy uploads with
	// no matching data set share one new data set on each provider. ---
	t.Run("UploadBatchingMultiCopyNewDataSets", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
		defer cancel()

		batchClient := integrationtest.NewDefaultClient(t, cctx, synapse.WithUploadBatching())
		metadata := map[string]string{
			"source": "integration-multicopy-shared-dataset",
			"run":    fmt.Sprintf("%d", time.Now().UnixNano()),
		}
		const uploads = 3
		payloads := make([][]byte, uploads)
		pieceSizes := make([]uint64, uploads)
		for i := range payloads {
			payloads[i] = make([]byte, 128*1024)
			if _, err := crypto_rand.Read(payloads[i]); err != nil {
				t.Fatalf("generate multi-copy shared dataset upload data %d: %v", i, err)
			}
			pieceSizes[i] = uint64(len(payloads[i]))
		}

		selection, err := batchClient.Storage().SelectUploadContexts(cctx, storage.SelectUploadContextsOptions{
			Copies:          2,
			DataSetMetadata: metadata,
		})
		if err != nil {
			t.Fatalf("SelectUploadContexts(multi-copy shared dataset): %v", err)
		}
		if selection == nil || len(selection.Contexts) != 2 {
			t.Fatalf("SelectUploadContexts(multi-copy shared dataset) returned %+v, want two contexts", selection)
		}
		for i, target := range selection.Contexts {
			if _, bound := target.DataSetRef(); bound {
				t.Fatalf("selected context %d is already bound to a data set", i)
			}
		}

		t.Log("start MultiCopySharedDataSet Prepare")
		prep, err := batchClient.Storage().Prepare(cctx, &storage.PrepareOptions{
			PieceSizes:        pieceSizes,
			ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
			BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
			Contexts:          selection.Contexts,
		})
		if err != nil {
			t.Fatalf("Prepare(multi-copy shared dataset): %v", err)
		}
		if prep.Transaction != nil {
			t.Log("start MultiCopySharedDataSet Prepare.Execute")
			res, err := prep.Transaction.Execute(cctx, payments.WithWait(txWaitTimeout))
			if err != nil {
				if errors.Is(err, payments.ErrPermitUnsupported) {
					t.Skip("needs-usdfc-permit-support: multi-copy shared dataset funding requires permit support")
				}
				t.Fatalf("Prepare(multi-copy shared dataset).Execute: %v", err)
			}
			if res.Receipt == nil || res.Receipt.Status != 1 {
				t.Fatalf("Prepare(multi-copy shared dataset).Execute receipt = %+v", res.Receipt)
			}
		}

		type uploadOutcome struct {
			index  int
			result *storage.UploadResult
			err    error
		}
		outcomes := make(chan uploadOutcome, uploads)
		start := time.Now()
		t.Log("start MultiCopySharedDataSet concurrent Storage.Upload")
		for i := range payloads {
			opts := tracedUploadOptions(t, fmt.Sprintf("MultiCopySharedDataSet[%d]", i), &storage.UploadOptions{
				Copies:          2,
				DataSetMetadata: metadata,
			})
			go func() {
				result, err := batchClient.Storage().Upload(cctx, bytes.NewReader(payloads[i]), opts)
				outcomes <- uploadOutcome{index: i, result: result, err: err}
			}()
		}
		results := make([]*storage.UploadResult, uploads)
		errs := make([]error, uploads)
		for range payloads {
			select {
			case outcome := <-outcomes:
				results[outcome.index] = outcome.result
				errs[outcome.index] = outcome.err
			case <-cctx.Done():
				t.Fatalf("wait for concurrent multi-copy uploads: %v", cctx.Err())
			}
		}
		t.Logf("done MultiCopySharedDataSet concurrent Storage.Upload elapsed=%s", time.Since(start).Round(time.Second))

		cleaned := make(map[string]struct{})
		for _, result := range results {
			if result == nil {
				continue
			}
			for _, cp := range result.Copies {
				key := idconv.Key(cp.DataSetID)
				if _, ok := cleaned[key]; ok || cp.DataSetID.IsZero() {
					continue
				}
				cleaned[key] = struct{}{}
				terminateDataSetOnCleanup(t, batchClient, cp.DataSetID, "MultiCopySharedDataSet")
			}
		}

		dataSetByProvider := make(map[string]types.BigInt, 2)
		for i, result := range results {
			if errs[i] != nil {
				t.Fatalf("Storage.Upload(multi-copy shared dataset %d): %v", i, errs[i])
			}
			if result == nil || !result.Complete || len(result.Copies) != 2 {
				t.Fatalf("Storage.Upload(multi-copy shared dataset %d) result=%+v, want two copies", i, result)
			}
			for _, cp := range result.Copies {
				key := idconv.Key(cp.ProviderID)
				if existing, ok := dataSetByProvider[key]; !ok {
					dataSetByProvider[key] = cp.DataSetID
				} else if !existing.Equal(cp.DataSetID) {
					t.Fatalf("provider %s DataSetIDs=%s,%s, want one shared data set", cp.ProviderID, existing, cp.DataSetID)
				}
			}
		}
		if len(dataSetByProvider) != 2 {
			t.Fatalf("data sets by provider=%v, want two providers", dataSetByProvider)
		}
		for _, cp := range results[0].Copies {
			t.Logf("MultiCopySharedDataSet provider=%s role=%s dataSet=%s", cp.ProviderID, cp.Role, cp.DataSetID)
		}
	})

	// --- ContextUploadBatchingExistingDataSet: explicit evidence that two
	// concurrent high-level uploads share one existing-dataset transaction. ---
	t.Run("ContextUploadBatchingExistingDataSet", func(t *testing.T) {
		if uploadedDataSetID.IsZero() {
			t.Skip("Upload subtest did not produce dataset id; skipping upload batching evidence")
		}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		ws := client.WarmStorage()
		beforeCount, err := ws.GetActivePieceCount(cctx, uploadedDataSetID)
		if err != nil {
			t.Fatalf("GetActivePieceCount(before existing-dataset upload): %v", err)
		}
		if beforeCount == nil || beforeCount.Sign() < 0 {
			t.Fatalf("active piece count before existing-dataset upload invalid: %v", beforeCount)
		}

		batchClient := integrationtest.NewDefaultClient(t, cctx, synapse.WithUploadBatching(
			storage.WithoutUploadIdleWait(),
			storage.WithoutUploadMaxWait(),
		))
		uctx, err := batchClient.Storage().NewDataSetContext(cctx, uploadedDataSetID, storage.NewDataSetContextOptions{})
		if err != nil {
			t.Fatalf("NewDataSetContext(uploadedDataSetID): %v", err)
		}

		payloads := make([][]byte, 2)
		for i := range payloads {
			payloads[i] = make([]byte, 128*1024)
			if _, err := crypto_rand.Read(payloads[i]); err != nil {
				t.Fatalf("generate existing-dataset upload data %d: %v", i, err)
			}
		}

		t.Log("start ExistingDataSetBatch Prepare")
		prep, err := batchClient.Storage().Prepare(cctx, &storage.PrepareOptions{
			PieceSizes:        []uint64{uint64(len(payloads[0])), uint64(len(payloads[1]))},
			ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
			BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
			Contexts: []storage.StorageContext{
				uctx,
			},
		})
		if err != nil {
			t.Fatalf("Prepare(existing dataset batch): %v", err)
		}
		if prep.Transaction != nil {
			t.Log("start ExistingDataSetBatch Prepare.Execute")
			res, err := prep.Transaction.Execute(cctx, payments.WithWait(txWaitTimeout))
			if err != nil {
				if errors.Is(err, payments.ErrPermitUnsupported) {
					t.Skip("needs-usdfc-permit-support: existing-dataset batch funding requires permit support")
				}
				t.Fatalf("Prepare(existing dataset batch).Execute: %v", err)
			}
			if res.Receipt == nil || res.Receipt.Status != 1 {
				t.Fatalf("Prepare(existing dataset batch).Execute receipt = %+v", res.Receipt)
			}
		}

		type uploadOutcome struct {
			index  int
			result *storage.UploadResult
			err    error
		}
		stored := make(chan struct{}, len(payloads))
		outcomes := make(chan uploadOutcome, len(payloads))
		txHashes := make([]string, len(payloads))
		start := time.Now()
		t.Log("start ExistingDataSetBatch concurrent DataSetContext.Upload")
		for i := range payloads {
			opts := tracedContextUploadOptions(t, fmt.Sprintf("ExistingDataSetBatch[%d]", i), &storage.ContextUploadOptions{
				OnStored: func(types.BigInt, cid.Cid) { stored <- struct{}{} },
				OnPiecesAdded: func(txHash string, _ types.BigInt, pieces []storage.SubmittedPiece) {
					txHashes[i] = txHash
					if len(pieces) != 1 {
						t.Errorf("upload %d OnPiecesAdded pieces=%d, want 1", i, len(pieces))
					}
				},
			})
			go func() {
				result, err := uctx.Upload(cctx, bytes.NewReader(payloads[i]), opts)
				outcomes <- uploadOutcome{index: i, result: result, err: err}
			}()
		}
		for range payloads {
			select {
			case <-stored:
			case <-cctx.Done():
				t.Fatalf("wait for concurrent stores: %v", cctx.Err())
			}
		}
		if err := batchClient.Storage().Flush(cctx); err != nil {
			t.Fatalf("Flush(existing dataset batch): %v", err)
		}
		t.Logf("done ExistingDataSetBatch concurrent DataSetContext.Upload elapsed=%s", time.Since(start).Round(time.Second))

		results := make([]*storage.UploadResult, len(payloads))
		for range payloads {
			outcome := <-outcomes
			if outcome.err != nil {
				t.Fatalf("DataSetContext.Upload(existing dataset batch %d): %v", outcome.index, outcome.err)
			}
			results[outcome.index] = outcome.result
		}
		for i, result := range results {
			if result == nil || !result.PieceCID.Defined() || len(result.Copies) != 1 {
				t.Fatalf("DataSetContext.Upload(existing dataset batch %d) result=%+v", i, result)
			}
			info, err := piece.ParseV2(result.PieceCID)
			if err != nil || info.RawSize != uint64(len(payloads[i])) {
				t.Fatalf("upload %d PieceCID info=%+v error=%v, want raw size %d", i, info, err, len(payloads[i]))
			}
			copy0 := result.Copies[0]
			if !copy0.DataSetID.Equal(uploadedDataSetID) || copy0.IsNewDataSet {
				t.Fatalf("upload %d copy=%+v, want existing dataSetID %s", i, copy0, uploadedDataSetID)
			}
		}
		if txHashes[0] == "" || txHashes[0] != txHashes[1] {
			t.Fatalf("batch transaction IDs=%q,%q, want one shared non-empty transaction", txHashes[0], txHashes[1])
		}
		if results[0].Copies[0].PieceID.Equal(results[1].Copies[0].PieceID) {
			t.Fatalf("batch PieceIDs=%s,%s, want distinct IDs", results[0].Copies[0].PieceID, results[1].Copies[0].PieceID)
		}

		if err := ws.ValidateDataSet(cctx, uploadedDataSetID); err != nil {
			t.Fatalf("ValidateDataSet(existing dataset after batch): %v", err)
		}
		afterCount, err := ws.GetActivePieceCount(cctx, uploadedDataSetID)
		if err != nil {
			t.Fatalf("GetActivePieceCount(after existing-dataset batch): %v", err)
		}
		wantAfter := new(big.Int).Add(new(big.Int).Set(beforeCount), big.NewInt(int64(len(payloads))))
		if afterCount == nil || afterCount.Cmp(wantAfter) != 0 {
			t.Fatalf("active piece count after existing-dataset batch = %v, want %v", afterCount, wantAfter)
		}
	})

	// --- ContextUploadBatchingWaitsForInProgressStore: with default timers, a
	// piece that finishes storing well after the idle wait still joins the
	// window of an upload that was ready earlier. ---
	t.Run("ContextUploadBatchingWaitsForInProgressStore", func(t *testing.T) {
		if uploadedDataSetID.IsZero() {
			t.Skip("Upload subtest did not produce dataset id; skipping in-progress store batching evidence")
		}
		cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		// Default batching timers: no maximum wait and a 3-second idle wait.
		batchClient := integrationtest.NewDefaultClient(t, cctx, synapse.WithUploadBatching())
		uctx, err := batchClient.Storage().NewDataSetContext(cctx, uploadedDataSetID, storage.NewDataSetContextOptions{})
		if err != nil {
			t.Fatalf("NewDataSetContext(uploadedDataSetID): %v", err)
		}

		payloads := make([][]byte, 2)
		for i := range payloads {
			payloads[i] = make([]byte, 128*1024)
			if _, err := crypto_rand.Read(payloads[i]); err != nil {
				t.Fatalf("generate in-progress store upload data %d: %v", i, err)
			}
		}

		t.Log("start InProgressStoreBatch Prepare")
		prep, err := batchClient.Storage().Prepare(cctx, &storage.PrepareOptions{
			PieceSizes:        []uint64{uint64(len(payloads[0])), uint64(len(payloads[1]))},
			ExtraRunwayEpochs: integrationtest.FundingExtraRunwayEpochs,
			BufferEpochs:      new(int64(integrationtest.FundingBufferEpochs)),
			Contexts:          []storage.StorageContext{uctx},
		})
		if err != nil {
			t.Fatalf("Prepare(in-progress store batch): %v", err)
		}
		if prep.Transaction != nil {
			t.Log("start InProgressStoreBatch Prepare.Execute")
			res, err := prep.Transaction.Execute(cctx, payments.WithWait(txWaitTimeout))
			if err != nil {
				if errors.Is(err, payments.ErrPermitUnsupported) {
					t.Skip("needs-usdfc-permit-support: in-progress store batch funding requires permit support")
				}
				t.Fatalf("Prepare(in-progress store batch).Execute: %v", err)
			}
			if res.Receipt == nil || res.Receipt.Status != 1 {
				t.Fatalf("Prepare(in-progress store batch).Execute receipt = %+v", res.Receipt)
			}
		}

		// The second upload stays in its Store step (OnStored runs before the
		// piece joins a batch) for longer than the default 3-second idle wait
		// after the first piece is ready.
		const lateDelay = 5 * time.Second
		firstStored := make(chan struct{})
		type uploadOutcome struct {
			index  int
			result *storage.UploadResult
			err    error
		}
		outcomes := make(chan uploadOutcome, len(payloads))
		txHashes := make([]string, len(payloads))
		start := time.Now()
		t.Log("start InProgressStoreBatch concurrent DataSetContext.Upload")
		for i := range payloads {
			opts := tracedContextUploadOptions(t, fmt.Sprintf("InProgressStoreBatch[%d]", i), &storage.ContextUploadOptions{
				OnStored: func(types.BigInt, cid.Cid) {
					if i == 0 {
						close(firstStored)
						return
					}
					select {
					case <-firstStored:
					case <-cctx.Done():
						return
					}
					time.Sleep(lateDelay)
				},
				OnPiecesAdded: func(txHash string, _ types.BigInt, pieces []storage.SubmittedPiece) {
					txHashes[i] = txHash
					if len(pieces) != 1 {
						t.Errorf("upload %d OnPiecesAdded pieces=%d, want 1", i, len(pieces))
					}
				},
			})
			go func() {
				result, err := uctx.Upload(cctx, bytes.NewReader(payloads[i]), opts)
				outcomes <- uploadOutcome{index: i, result: result, err: err}
			}()
		}
		results := make([]*storage.UploadResult, len(payloads))
		for range payloads {
			select {
			case outcome := <-outcomes:
				if outcome.err != nil {
					t.Fatalf("DataSetContext.Upload(in-progress store batch %d): %v", outcome.index, outcome.err)
				}
				results[outcome.index] = outcome.result
			case <-cctx.Done():
				t.Fatalf("wait for in-progress store batch uploads: %v", cctx.Err())
			}
		}
		t.Logf("done InProgressStoreBatch concurrent DataSetContext.Upload elapsed=%s", time.Since(start).Round(time.Second))

		for i, result := range results {
			if result == nil || len(result.Copies) != 1 {
				t.Fatalf("DataSetContext.Upload(in-progress store batch %d) result=%+v", i, result)
			}
			if copy0 := result.Copies[0]; !copy0.DataSetID.Equal(uploadedDataSetID) || copy0.IsNewDataSet {
				t.Fatalf("upload %d copy=%+v, want existing dataSetID %s", i, copy0, uploadedDataSetID)
			}
		}
		if txHashes[0] == "" || txHashes[0] != txHashes[1] {
			t.Fatalf("transaction IDs=%q,%q, want one shared transaction after the late store", txHashes[0], txHashes[1])
		}
		if results[0].Copies[0].PieceID.Equal(results[1].Copies[0].PieceID) {
			t.Fatalf("PieceIDs=%s,%s, want distinct IDs", results[0].Copies[0].PieceID, results[1].Copies[0].PieceID)
		}
	})

	// --- WarmStorageInspection: per-dataset/per-piece warm-storage methods. ---
	t.Run("WarmStorageInspection", func(t *testing.T) {
		if uploadedDataSetID.IsZero() {
			t.Skip("no uploaded dataset id; skipping WarmStorageInspection")
		}
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		ws := client.WarmStorage()

		ds, err := ws.GetDataSet(cctx, uploadedDataSetID)
		if err != nil {
			t.Fatalf("GetDataSet: %v", err)
		}
		if ds.PDPRailID.IsZero() {
			t.Errorf("PDPRailID == 0")
		}

		md, err := ws.GetAllDataSetMetadata(cctx, uploadedDataSetID)
		if err != nil {
			t.Fatalf("GetAllDataSetMetadata: %v", err)
		}
		t.Logf("dataset metadata keys: %d", len(md))

		if err := ws.ValidateDataSet(cctx, uploadedDataSetID); err != nil {
			t.Fatalf("ValidateDataSet: %v", err)
		}

		count, err := ws.GetActivePieceCount(cctx, uploadedDataSetID)
		if err != nil {
			t.Fatalf("GetActivePieceCount: %v", err)
		}
		if count == nil || count.Sign() < 0 {
			t.Errorf("active piece count invalid: %v", count)
		}
	})

	// --- PaymentsRails: rail read methods + Settle on the upload's PDP rail. ---
	t.Run("PaymentsRails", func(t *testing.T) {
		if uploadedRailID.IsZero() {
			t.Skip("no uploaded rail id; skipping PaymentsRails")
		}

		t.Run("RailReads", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			rv, err := client.Payments().GetRail(cctx, uploadedRailID)
			if err != nil {
				t.Fatalf("GetRail(%s): %v", uploadedRailID, err)
			}
			if rv == nil {
				t.Fatal("GetRail returned nil")
			}
			t.Logf("rail %s: token=%s payer=%s payee=%s", uploadedRailID, rv.Token, rv.From, rv.To)
			page, err := client.Payments().GetRailsAsPayee(cctx, rv.To, usdfc, types.ListOptions{Limit: 100})
			if err != nil {
				t.Fatalf("GetRailsAsPayee: %v", err)
			}
			t.Logf("payee %s first page contains %d rails (USDFC)", rv.To, len(page.Rails))
		})

		t.Run("GetSettlementAmounts", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			amts, err := client.Payments().GetSettlementAmounts(cctx, uploadedRailID, nil)
			if err != nil {
				if isNoProgressInSettlement(err) {
					t.Skipf("settlement preview has no progress for rail %s: %v", uploadedRailID, err)
				}
				t.Fatalf("GetSettlementAmounts: %v", err)
			}
			t.Logf("settlement preview: settled=%s netPayee=%s commission=%s networkFee=%s finalEpoch=%s note=%q",
				amts.TotalSettledAmount, amts.TotalNetPayeeAmount, amts.TotalOperatorCommission,
				amts.TotalNetworkFee, amts.FinalSettledEpoch, amts.Note)
		})

		t.Run("Settle", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, txWaitTimeout+time.Minute)
			defer cancel()
			t.Log("start PaymentsRails Settle")
			settleRes, err := client.Payments().Settle(cctx, uploadedRailID, nil, payments.WithWait(txWaitTimeout))
			if err != nil {
				t.Fatalf("Settle: %v", err)
			}
			if settleRes.Receipt == nil || settleRes.Receipt.Status != 1 {
				t.Fatalf("Settle tx not successful: %+v", settleRes.Receipt)
			}
			t.Logf("Settle tx=%s", settleRes.Hash)
		})

		t.Run("SettleAutoActive", func(t *testing.T) {
			cctx, cancel := context.WithTimeout(ctx, txWaitTimeout+time.Minute)
			defer cancel()
			t.Log("start PaymentsRails SettleAuto")
			autoRes, err := client.Payments().SettleAuto(cctx, uploadedRailID, nil, payments.WithWait(txWaitTimeout))
			if err != nil {
				if !isExecutionRevert(err) {
					t.Fatalf("SettleAuto: %v", err)
				}
				t.Logf("SettleAuto reverted (tolerated, likely nothing to auto-settle): %v", err)
				return
			}
			if autoRes.Receipt == nil || autoRes.Receipt.Status != 1 {
				t.Fatalf("SettleAuto tx not successful: %+v", autoRes.Receipt)
			}
			t.Logf("SettleAuto tx=%s", autoRes.Hash)
		})
	})

	// --- FilBeam: stats endpoint for the uploaded dataset. ---
	t.Run("FilBeam", func(t *testing.T) {
		if uploadedDataSetID.IsZero() {
			t.Skip("no uploaded dataset id; skipping FilBeam")
		}
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		stats, err := client.FilBeam().GetDataSetStats(cctx, uploadedDataSetID)
		if err != nil {
			// FilBeam aggregator may not have indexed brand-new uploads yet;
			// log but do not fail the suite.
			if errors.Is(err, filbeam.ErrDataSetNotFound) {
				t.Skipf("FilBeam has no stats yet for dataset %s (eventual consistency)", uploadedDataSetID)
			}
			if errors.Is(err, retry.ErrMaxRetries) {
				t.Skipf("FilBeam stats endpoint transient failure after retries for dataset %s: %v", uploadedDataSetID, err)
			}
			t.Fatalf("GetDataSetStats: %v", err)
		}
		if stats == nil {
			t.Fatal("GetDataSetStats returned nil")
		}
		t.Logf("filbeam stats: %+v", stats)
	})

	// --- StorageLifecycle: DeletePiece + storage.Service.TerminateService. ---
	t.Run("StorageLifecycle", func(t *testing.T) {
		if uploadedDataSetID.IsZero() || !uploadedCID.Defined() {
			t.Skip("no uploaded dataset/cid; skipping StorageLifecycle")
		}
		if _, verified := dataSetTerminated[uploadedDataSetID.String()]; !verified {
			t.Fatal("dataset ownership was not verified; refusing lifecycle operations")
		}
		cctx, cancel := context.WithTimeout(ctx, 7*time.Minute)
		defer cancel()

		uctx, err := client.Storage().NewDataSetContext(cctx, uploadedDataSetID, storage.NewDataSetContextOptions{})
		if err != nil {
			t.Fatalf("NewDataSetContext: %v", err)
		}

		t.Log("start StorageLifecycle DeletePiece")
		delRes, err := uctx.DeletePiece(cctx, uploadedCID)
		if err != nil {
			t.Fatalf("DeletePiece: %v", err)
		}
		// DeletePiece intentionally returns only a Hash (no on-chain wait),
		// matching TS behaviour; the server schedules the removal.
		t.Logf("DeletePiece tx=%s", delRes.Hash)

		t.Log("start StorageLifecycle SubmitTerminateService")
		var notified *storage.TerminateServiceSubmission
		submission, err := client.Storage().SubmitTerminateService(cctx, uploadedDataSetID, &storage.TerminateServiceOptions{
			ProviderWaitTimeout: 5 * time.Minute,
			OnSubmitted: func(sub storage.TerminateServiceSubmission) {
				notified = &sub
				t.Logf("TerminateService accepted provider=%s dataset=%s", sub.ProviderID, sub.DataSetID)
			},
		})
		if err != nil {
			t.Fatalf("SubmitTerminateService: %v", err)
		}
		if submission == nil || submission.Direct || !submission.Accepted || submission.ProviderID.IsZero() ||
			!submission.DataSetID.Equal(uploadedDataSetID) || notified == nil || !notified.DataSetID.Equal(submission.DataSetID) {
			t.Fatalf("SubmitTerminateService submission=%+v callback=%+v", submission, notified)
		}
		termRes, err := client.Storage().WaitForTerminateService(cctx, *submission, &storage.TerminateServiceWaitOptions{
			Timeout:      5 * time.Minute,
			PollInterval: 2 * time.Second,
		})
		if err != nil {
			t.Fatalf("WaitForTerminateService: %v", err)
		}
		if termRes == nil || !termRes.DataSetID.Equal(uploadedDataSetID) || termRes.EndEpoch == 0 {
			t.Fatalf("TerminateService result=%+v, want a confirmed termination epoch", termRes)
		}
		dataSetTerminated[uploadedDataSetID.String()] = true
		t.Logf("TerminateService result tx=%v endEpoch=%d", termRes.TxHash, termRes.EndEpoch)
	})

	// --- DestructiveSuite: gated on INTEGRATION_DESTRUCTIVE_KEY. ---
	t.Run("DestructiveSuite", func(t *testing.T) {
		ok, destKey := integrationtest.DestructiveKey(t)
		if !ok {
			t.Skip("needs-destructive-account: INTEGRATION_DESTRUCTIVE_KEY not set")
		}
		dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		dclient := integrationtest.NewClient(t, dctx, destKey)
		daddr := dclient.Address()

		// RevokeService — reset operator approval to zero (idempotent).
		t.Log("start DestructiveSuite RevokeService")
		revRes, err := dclient.Payments().RevokeService(dctx, usdfc, fwss, payments.WithWait(txWaitTimeout))
		if err != nil {
			t.Fatalf("RevokeService: %v", err)
		}
		if revRes.Receipt == nil || revRes.Receipt.Status != 1 {
			t.Errorf("RevokeService tx failed: %+v", revRes.Receipt)
		}
		t.Logf("RevokeService tx=%s", revRes.Hash)

		// DepositWithPermit — 1 unit USDFC. Requires EIP-2612 support
		// from the USDFC token contract on calibration; if the contract
		// lacks one of the permit ABI methods, skip with the documented
		// reason rather than failing.
		amount := big.NewInt(1)
		t.Log("start DestructiveSuite DepositWithPermit")
		depRes, err := dclient.Payments().DepositWithPermit(dctx, usdfc, daddr, amount, nil, payments.WithWait(txWaitTimeout))
		if err != nil {
			if errors.Is(err, payments.ErrPermitUnsupported) {
				t.Skip("needs-usdfc-permit-support: USDFC contract does not implement EIP-2612 permit")
			}
			t.Fatalf("DepositWithPermit: %v", err)
		}
		if depRes.Receipt == nil || depRes.Receipt.Status != 1 {
			t.Errorf("DepositWithPermit tx failed: %+v", depRes.Receipt)
		}
		t.Logf("DepositWithPermit tx=%s", depRes.Hash)

		// DepositWithPermitAndApproveOperator — combine deposit with
		// FWSS operator approval bump.
		rate := big.NewInt(1)
		lockup := big.NewInt(1)
		maxPeriod := big.NewInt(86400)
		t.Log("start DestructiveSuite DepositWithPermitAndApproveOperator")
		dpaRes, err := dclient.Payments().DepositWithPermitAndApproveOperator(
			dctx, usdfc, daddr, amount, nil, fwss, rate, lockup, maxPeriod,
			payments.WithWait(txWaitTimeout),
		)
		if err != nil {
			if errors.Is(err, payments.ErrPermitUnsupported) {
				t.Skip("needs-usdfc-permit-support: USDFC contract does not implement EIP-2612 permit")
			}
			t.Fatalf("DepositWithPermitAndApproveOperator: %v", err)
		}
		if dpaRes.Receipt == nil || dpaRes.Receipt.Status != 1 {
			t.Errorf("DepositWithPermitAndApproveOperator tx failed: %+v", dpaRes.Receipt)
		}
		t.Logf("DepositWithPermitAndApproveOperator tx=%s", dpaRes.Hash)
	})
}

func terminateDataSetOnCleanup(t *testing.T, client *synapse.Client, dataSetID types.BigInt, label string) {
	t.Helper()
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		t.Logf("start %s cleanup TerminateDataSet(%s)", label, dataSetID)
		termRes, err := client.WarmStorage().TerminateDataSet(cleanupCtx, dataSetID, warmstorage.WithWait(txWaitTimeout))
		if err != nil {
			t.Logf("cleanup TerminateDataSet(%s): %v", dataSetID, err)
			return
		}
		if termRes == nil {
			t.Logf("cleanup TerminateDataSet(%s) returned nil result", dataSetID)
			return
		}
		if termRes.Receipt != nil && termRes.Receipt.Status != 1 {
			t.Logf("cleanup TerminateDataSet(%s) receipt = %+v", dataSetID, termRes.Receipt)
		}
		t.Logf("done %s cleanup TerminateDataSet(%s) tx=%s", label, dataSetID, termRes.Hash)
	})
}
