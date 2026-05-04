//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	crypto_rand "crypto/rand"
	"errors"
	"io"
	"math/big"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/filbeam"
	filpaybind "github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/idconv"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/internal/retry"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/sessionkey"
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

func tracedUploadOptions(t *testing.T, label string, opts *storage.UploadOptions) *storage.UploadOptions {
	t.Helper()
	cloned := &storage.UploadOptions{}
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

	addrs := client.Chain().Addresses()
	usdfc := addrs.USDFC
	filPay := addrs.Payments
	fwss := addrs.FWSS

	// Shared state across subtests.
	var (
		uploadedCID       cid.Cid
		uploadedURL       string
		testData          []byte
		uploadedDataSetID types.BigInt
		uploadedRailID    types.BigInt
	)

	// --- Costs subtest ---
	t.Run("Costs", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		dataSize := big.NewInt(testDataSize)
		uploadCosts, err := client.Costs().GetUploadCosts(cctx, addr, dataSize, nil)
		if err != nil {
			t.Fatalf("GetUploadCosts: %v", err)
		}

		if uploadCosts.Rate.RatePerEpoch == nil || uploadCosts.Rate.RatePerEpoch.Sign() <= 0 {
			t.Errorf("RatePerEpoch should be positive, got %v", uploadCosts.Rate.RatePerEpoch)
		}
		if uploadCosts.Rate.RatePerMonth == nil || uploadCosts.Rate.RatePerMonth.Sign() <= 0 {
			t.Errorf("RatePerMonth should be positive, got %v", uploadCosts.Rate.RatePerMonth)
		}
		if uploadCosts.Lockup.TotalLockup == nil || uploadCosts.Lockup.TotalLockup.Sign() < 0 {
			t.Errorf("TotalLockup should be >= 0, got %v", uploadCosts.Lockup.TotalLockup)
		}
		t.Logf("rate/epoch=%s, rate/month=%s, lockup=%s, depositNeeded=%s, ready=%v",
			uploadCosts.Rate.RatePerEpoch, uploadCosts.Rate.RatePerMonth,
			uploadCosts.Lockup.TotalLockup, uploadCosts.DepositNeeded, uploadCosts.Ready)

		summary, err := client.Costs().GetAccountSummary(cctx, addr)
		if err != nil {
			t.Fatalf("GetAccountSummary: %v", err)
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
		uploadCosts, err := client.Costs().GetUploadCosts(cctx, addr, dataSize,
			&costs.UploadCostOptions{IsNewDataSet: true})
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
		if uploadCosts.Lockup.TotalLockup == nil {
			t.Fatal("uploadCosts.Lockup.TotalLockup is nil")
		}
		if !svcApproval.IsApproved {
			maxRate := new(big.Int).Mul(uploadCosts.Rate.RatePerEpoch, big.NewInt(100))
			maxLockup := new(big.Int).Mul(uploadCosts.Lockup.TotalLockup, big.NewInt(100))
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
		res, err := client.Payments().Deposit(cctx, usdfc, addr, depositAmount,
			payments.WithWait(txWaitTimeout))
		if err != nil {
			t.Fatalf("Deposit: %v", err)
		}
		if res.Receipt != nil && res.Receipt.Status != 1 {
			t.Fatalf("Deposit tx failed: status=%d", res.Receipt.Status)
		}
		t.Logf("deposited %s USDFC, tx=%s", depositAmount, res.Hash)

		balAfter, err := client.Payments().Balance(cctx, usdfc, addr)
		if err != nil {
			t.Fatalf("Balance (after): %v", err)
		}

		diff := new(big.Int).Sub(balAfter, balBefore)
		if diff.Cmp(depositAmount) != 0 {
			t.Errorf("balance diff = %s, want %s", diff, depositAmount)
		}
		t.Logf("balance: before=%s, after=%s, diff=%s", balBefore, balAfter, diff)
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
			Copies: 1,
		}))
		t.Logf("done Upload Storage.Upload elapsed=%s", time.Since(start).Round(time.Second))
		if err != nil {
			t.Fatalf("Upload: %v", err)
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
		for attempt := 0; attempt < 5; attempt++ {
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
	})

	// --- Multicopy subtest ---
	t.Run("Multicopy", func(t *testing.T) {
		cctx, cancel := context.WithTimeout(ctx, 6*time.Minute)
		defer cancel()

		perCopyCosts, err := client.Costs().GetUploadCosts(cctx, addr, big.NewInt(testDataSize), &costs.UploadCostOptions{
			EnableCDN:    true,
			IsNewDataSet: true,
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
			multiCosts.DepositNeeded, multiCosts.Lockup.TotalLockup, multiCosts.Rate.RatePerEpoch, multiCosts.Ready)
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
			cctx, addr, sessionKeyAddr, sessionkey.CreateDataSetPermission)
		if err != nil {
			t.Fatalf("AuthorizationExpiry: %v", err)
		}
		if expiry == 0 {
			t.Error("expiry = 0, want > 0 after login")
		}
		t.Logf("CreateDataSet permission expiry: %d", expiry)

		// Verify not expired.
		isExpired, err := client.SessionKey().IsExpired(
			cctx, addr, sessionKeyAddr, sessionkey.CreateDataSetPermission)
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
			cctx, addr, sessionKeyAddr, sessionkey.CreateDataSetPermission)
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
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()

		sm := client.Storage()

		sets, err := sm.FindDataSets(cctx, nil)
		if err != nil {
			t.Fatalf("FindDataSets(default): %v", err)
		}
		t.Logf("FindDataSets(default): %d", len(sets))

		setsManaged, err := sm.FindDataSets(cctx, &storage.FindDataSetsOptions{OnlyManaged: true})
		if err != nil {
			t.Fatalf("FindDataSets(onlyManaged): %v", err)
		}
		if len(setsManaged) > len(sets) {
			t.Errorf("OnlyManaged returned %d > total %d", len(setsManaged), len(sets))
		}

		info, err := sm.GetStorageInfo(cctx, nil)
		if err != nil {
			t.Fatalf("GetStorageInfo: %v", err)
		}
		if len(info.Providers) == 0 {
			t.Errorf("StorageInfo.Providers empty")
		}
		if info.Allowances == nil {
			t.Errorf("StorageInfo.Allowances nil for configured signer")
		}

		prep, err := sm.Prepare(cctx, &storage.PrepareOptions{DataSize: 64 * 1024})
		if err != nil {
			t.Fatalf("Prepare: %v", err)
		}
		if prep == nil || prep.Costs == nil {
			t.Fatal("Prepare returned nil result/costs")
		}
		if prep.Transaction != nil {
			t.Logf("Prepare reports funding required: deposit=%s includesApproval=%v",
				prep.Transaction.DepositAmount, prep.Transaction.IncludesApproval)
		}

		mc, err := sm.CalculateMultiContextCosts(cctx, 64*1024, []storage.ContextCostRef{
			{Provider: storage.Provider{ID: info.Providers[0].Info.ID}, WithCDN: false},
		}, storage.MultiCostOptions{}, addr)
		if err != nil {
			t.Fatalf("CalculateMultiContextCosts: %v", err)
		}
		if mc == nil || mc.RatePerEpoch == nil {
			t.Fatal("MultiContextCosts missing RatePerEpoch")
		}

		def, err := sm.GetDefaultContext(cctx)
		if err != nil {
			t.Fatalf("GetDefaultContext: %v", err)
		}
		if def == nil || def.ProviderID().IsZero() {
			t.Fatal("GetDefaultContext returned invalid context")
		}

		ctxs, err := sm.CreateContexts(cctx, &storage.CreateContextsOptions{Copies: 1})
		if err != nil {
			t.Fatalf("CreateContexts: %v", err)
		}
		if len(ctxs) != 1 {
			t.Errorf("CreateContexts returned %d contexts, want 1", len(ctxs))
		}

		single, err := sm.CreateContext(cctx, nil)
		if err != nil {
			t.Fatalf("CreateContext(nil): %v", err)
		}
		if single == nil || single.ProviderID().IsZero() {
			t.Fatal("CreateContext returned invalid context")
		}
	})

	// --- ContextInspection: read methods on the Context produced by Upload. ---
	t.Run("ContextInspection", func(t *testing.T) {
		if uploadedDataSetID.IsZero() || !uploadedCID.Defined() {
			t.Skip("Upload subtest did not produce dataset/cid; skipping ContextInspection")
		}
		cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()

		uctx, err := client.Storage().CreateContext(cctx, &storage.CreateContextOptions{
			DataSetIDs: []types.BigInt{uploadedDataSetID},
		})
		if err != nil {
			t.Fatalf("CreateContext(uploadedDataSetID): %v", err)
		}

		if uctx.ProviderID().IsZero() {
			t.Errorf("ProviderID == 0")
		}
		if uctx.ServiceURL() == "" {
			t.Errorf("ServiceURL empty")
		}
		if got := uctx.DataSetID(); got == nil || !got.Equal(uploadedDataSetID) {
			t.Errorf("DataSetID = %v, want %s", got, uploadedDataSetID)
		}
		_ = uctx.WithCDN()
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

		removals, err := uctx.GetScheduledRemovals(cctx)
		if err != nil {
			t.Fatalf("GetScheduledRemovals: %v", err)
		}
		t.Logf("scheduled removals: %d", len(removals))

		// Context-level Download.
		start := time.Now()
		t.Log("start ContextInspection Download")
		rc, err := uctx.Download(cctx, uploadedCID)
		if err != nil {
			t.Logf("done ContextInspection Download elapsed=%s", time.Since(start).Round(time.Second))
			t.Fatalf("Context.Download: %v", err)
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

	// --- ContextUploadExistingDataSet: explicit evidence that the existing-dataset
	// AddPieces typed-data path accepts the current PieceCID encoding end-to-end. ---
	t.Run("ContextUploadExistingDataSet", func(t *testing.T) {
		if uploadedDataSetID.IsZero() {
			t.Skip("Upload subtest did not produce dataset id; skipping existing-dataset AddPieces evidence")
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

		uctx, err := client.Storage().CreateContext(cctx, &storage.CreateContextOptions{
			DataSetIDs: []types.BigInt{uploadedDataSetID},
		})
		if err != nil {
			t.Fatalf("CreateContext(uploadedDataSetID): %v", err)
		}

		extraData := make([]byte, 128*1024)
		if _, err := crypto_rand.Read(extraData); err != nil {
			t.Fatalf("generate existing-dataset upload data: %v", err)
		}

		t.Log("start ExistingDataSet Prepare")
		prep, err := client.Storage().Prepare(cctx, &storage.PrepareOptions{
			DataSize: uint64(len(extraData)),
			Contexts: []storage.UploadContext{
				uctx,
			},
		})
		if err != nil {
			t.Fatalf("Prepare(existing dataset): %v", err)
		}
		if prep.Transaction != nil {
			t.Log("start ExistingDataSet Prepare.Execute")
			res, err := prep.Transaction.Execute(cctx, payments.WithWait(txWaitTimeout))
			if err != nil {
				if errors.Is(err, payments.ErrPermitUnsupported) {
					t.Skip("needs-usdfc-permit-support: existing-dataset funding requires permit support")
				}
				t.Fatalf("Prepare(existing dataset).Execute: %v", err)
			}
			if res.Receipt == nil || res.Receipt.Status != 1 {
				t.Fatalf("Prepare(existing dataset).Execute receipt = %+v", res.Receipt)
			}
		}

		start := time.Now()
		t.Log("start ExistingDataSet Context.Upload")
		result, err := uctx.Upload(cctx, bytes.NewReader(extraData), tracedUploadOptions(t, "ExistingDataSet", nil))
		t.Logf("done ExistingDataSet Context.Upload elapsed=%s", time.Since(start).Round(time.Second))
		if err != nil {
			t.Fatalf("Context.Upload(existing dataset): %v", err)
		}
		if !result.PieceCID.Defined() {
			t.Fatal("Context.Upload(existing dataset) returned undefined PieceCID")
		}
		info, err := piece.ParseV2(result.PieceCID)
		if err != nil {
			t.Fatalf("Context.Upload(existing dataset) PieceCID must be v2: %v", err)
		}
		if info.RawSize != uint64(len(extraData)) {
			t.Fatalf("Context.Upload(existing dataset) raw size = %d, want %d", info.RawSize, len(extraData))
		}
		if len(result.Copies) != 1 {
			t.Fatalf("Context.Upload(existing dataset) copies = %d, want 1", len(result.Copies))
		}
		copy0 := result.Copies[0]
		if !copy0.DataSetID.Equal(uploadedDataSetID) {
			t.Fatalf("Context.Upload(existing dataset) dataSetID = %s, want %s", copy0.DataSetID, uploadedDataSetID)
		}
		if copy0.IsNewDataSet {
			t.Fatal("Context.Upload(existing dataset) unexpectedly created a new dataset")
		}

		if err := ws.ValidateDataSet(cctx, uploadedDataSetID); err != nil {
			t.Fatalf("ValidateDataSet(existing dataset after addPieces): %v", err)
		}
		afterCount, err := ws.GetActivePieceCount(cctx, uploadedDataSetID)
		if err != nil {
			t.Fatalf("GetActivePieceCount(after existing-dataset upload): %v", err)
		}
		wantAfter := new(big.Int).Add(new(big.Int).Set(beforeCount), big.NewInt(1))
		if afterCount == nil || afterCount.Cmp(wantAfter) != 0 {
			t.Fatalf("active piece count after existing-dataset upload = %v, want %v", afterCount, wantAfter)
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

		// PieceID 0 is always queryable. "label" is not a key we set on
		// upload, so (exists, value) must be (false, "") — assert both.
		exists, value, err := ws.GetPieceMetadata(cctx, uploadedDataSetID, types.NewBigInt(0), "label")
		if err != nil {
			t.Fatalf("GetPieceMetadata(0,label): %v", err)
		}
		if exists || value != "" {
			t.Errorf("GetPieceMetadata(0,label): want (false, \"\"), got (%v, %q)", exists, value)
		}
		all, err := ws.GetAllPieceMetadata(cctx, uploadedDataSetID, types.NewBigInt(0))
		if err != nil {
			t.Fatalf("GetAllPieceMetadata(0): %v", err)
		}
		if all == nil {
			t.Errorf("GetAllPieceMetadata(0) returned nil map")
		}
		t.Logf("piece 0 metadata keys: %d", len(all))
	})

	// --- PaymentsRails: rail read methods + Settle on the upload's PDP rail. ---
	t.Run("PaymentsRails", func(t *testing.T) {
		if uploadedRailID.IsZero() {
			t.Skip("no uploaded rail id; skipping PaymentsRails")
		}
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		rv, err := client.Payments().GetRail(cctx, uploadedRailID)
		if err != nil {
			t.Fatalf("GetRail(%s): %v", uploadedRailID, err)
		}
		if rv == nil {
			t.Fatal("GetRail returned nil")
		}
		t.Logf("rail %s: token=%s payer=%s payee=%s", uploadedRailID, rv.Token, rv.From, rv.To)

		payee := rv.To
		page, err := client.Payments().GetRailsAsPayee(cctx, payee, usdfc)
		if err != nil {
			t.Fatalf("GetRailsAsPayee: %v", err)
		}
		t.Logf("payee %s has %d rails (USDFC)", payee, len(page.Rails))

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

		// Settle is always safe to call; it's a no-op if nothing accrued.
		t.Log("start PaymentsRails Settle")
		settleRes, err := client.Payments().Settle(cctx, uploadedRailID, nil, payments.WithWait(txWaitTimeout))
		if err != nil {
			t.Fatalf("Settle: %v", err)
		}
		if settleRes.Receipt == nil || settleRes.Receipt.Status != 1 {
			t.Errorf("Settle tx not successful: %+v", settleRes.Receipt)
		}
		t.Logf("Settle tx=%s", settleRes.Hash)

		// SettleAuto commonly reverts when the rail state has nothing new
		// to auto-settle (e.g. already settled up to current epoch). The
		// call path itself is what we want to cover, so a gas-estimate
		// revert is logged and tolerated.
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

		// SettleTerminatedRail requires a rail in Terminated state with
		// settled epoch beyond endEpoch — outside the scope of this test
		// run.
		t.Run("SettleTerminatedRail", func(t *testing.T) {
			t.Skip("needs-terminated-rail")
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

	// --- StorageLifecycle: DeletePiece + storage.Service.TerminateDataSet. ---
	t.Run("StorageLifecycle", func(t *testing.T) {
		if uploadedDataSetID.IsZero() || !uploadedCID.Defined() {
			t.Skip("no uploaded dataset/cid; skipping StorageLifecycle")
		}
		cctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()

		uctx, err := client.Storage().CreateContext(cctx, &storage.CreateContextOptions{
			DataSetIDs: []types.BigInt{uploadedDataSetID},
		})
		if err != nil {
			t.Fatalf("CreateContext: %v", err)
		}

		t.Log("start StorageLifecycle DeletePiece")
		delRes, err := uctx.DeletePiece(cctx, uploadedCID)
		if err != nil {
			t.Fatalf("DeletePiece: %v", err)
		}
		// DeletePiece intentionally returns only a Hash (no on-chain wait),
		// matching TS behaviour; the server schedules the removal.
		t.Logf("DeletePiece tx=%s", delRes.Hash)

		t.Log("start StorageLifecycle TerminateDataSet")
		termRes, err := client.Storage().TerminateDataSet(cctx, uploadedDataSetID, &storage.TerminateDataSetOptions{
			WriteOptions: []warmstorage.WriteOption{warmstorage.WithWait(txWaitTimeout)},
		})
		if err != nil {
			t.Fatalf("TerminateDataSet: %v", err)
		}
		if termRes.Receipt == nil || termRes.Receipt.Status != 1 {
			t.Errorf("TerminateDataSet tx failed: %+v", termRes.Receipt)
		}
		t.Logf("TerminateDataSet tx=%s", termRes.Hash)
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
