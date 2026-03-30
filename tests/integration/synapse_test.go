//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	crypto_rand "crypto/rand"
	"io"
	"math/big"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/storage"
)

const (
	defaultRPCURL      = "https://api.calibration.node.glif.io/rpc/v1"
	calibrationChainID = 314159
	testDataSize       = 256 * 1024 // 256 KB
	txWaitTimeout      = 180 * time.Second
)

func TestIntegration(t *testing.T) {
	// Load .env from project root (two levels up from tests/integration/).
	if err := loadEnvFile("../../.env"); err != nil {
		t.Logf("warning: failed to load .env file: %v", err)
	}

	privateKeyHex := os.Getenv("INTEGRATION_PRIVATE_KEY")
	if privateKeyHex == "" {
		t.Skip("INTEGRATION_PRIVATE_KEY not set; skipping integration tests")
	}

	rpcURL := os.Getenv("INTEGRATION_RPC_URL")
	if rpcURL == "" {
		rpcURL = defaultRPCURL
	}

	ctx := context.Background()

	client, err := synapse.New(ctx,
		synapse.WithPrivateKeyHex(privateKeyHex),
		synapse.WithRPCURL(rpcURL),
	)
	if err != nil {
		t.Fatalf("synapse.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

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
		uploadedCID cid.Cid
		uploadedURL string
		testData    []byte
	)

	// Register cleanup to withdraw available funds at the end.
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), txWaitTimeout)
		defer cancel()
		acct, err := client.Payments().AccountInfo(cctx, usdfc, addr)
		if err != nil {
			t.Logf("cleanup: AccountInfo: %v", err)
			return
		}
		avail := acct.AvailableFunds()
		if avail == nil || avail.Sign() <= 0 {
			t.Log("cleanup: no available funds to withdraw")
			return
		}
		// Try withdrawing half of available to avoid contract revert on locked funds.
		half := new(big.Int).Div(avail, big.NewInt(2))
		if half.Sign() <= 0 {
			t.Log("cleanup: available funds too small to withdraw")
			return
		}
		_, err = client.Payments().Withdraw(cctx, usdfc, half,
			payments.WithWait(txWaitTimeout))
		if err != nil {
			t.Logf("cleanup: Withdraw %s: %v", half, err)
		} else {
			t.Logf("cleanup: withdrew %s USDFC", half)
		}
	})

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

		// Calculate deposit amount: 4x the needed deposit for 256KB upload.
		dataSize := big.NewInt(testDataSize)
		uploadCosts, err := client.Costs().GetUploadCosts(cctx, addr, dataSize,
			&costs.UploadCostOptions{IsNewDataSet: true})
		if err != nil {
			t.Fatalf("GetUploadCosts for deposit calculation: %v", err)
		}
		depositAmount := new(big.Int).Mul(uploadCosts.DepositNeeded, big.NewInt(4))
		if depositAmount.Sign() <= 0 {
			// DepositNeeded may be 0 if account is already well-funded.
			// Use a minimum deposit of 1 USDFC (18 decimals).
			depositAmount = new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
		}
		t.Logf("deposit amount: %s (depositNeeded=%s)", depositAmount, uploadCosts.DepositNeeded)

		// Ensure ERC20 allowance for the Payments contract.
		allowance, err := client.Payments().Allowance(cctx, usdfc, addr, filPay)
		if err != nil {
			t.Fatalf("Allowance: %v", err)
		}
		if allowance.Cmp(depositAmount) < 0 {
			approveAmount := new(big.Int).Mul(depositAmount, big.NewInt(10))
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

		// Deposit funds.
		balBefore, err := client.Payments().Balance(cctx, usdfc, addr)
		if err != nil {
			t.Fatalf("Balance (before): %v", err)
		}

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
		cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		testData = make([]byte, testDataSize)
		if _, err := crypto_rand.Read(testData); err != nil {
			t.Fatalf("generate test data: %v", err)
		}

		result, err := client.Storage().UploadBytes(cctx, testData, &storage.UploadOptions{
			Copies: 1,
		})
		if err != nil {
			t.Fatalf("UploadBytes: %v", err)
		}

		// Verify PieceCID is a valid piece CID.
		if !result.PieceCID.Defined() {
			t.Fatal("PieceCID is undefined")
		}
		if err := piece.Validate(result.PieceCID); err != nil {
			if _, err2 := piece.ParseV2(result.PieceCID); err2 != nil {
				t.Errorf("PieceCID is not a valid piece CID (v1: %v, v2: %v)", err, err2)
			}
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
			if cp.ProviderID == nil || cp.ProviderID.Sign() <= 0 {
				t.Errorf("Copy[%d].ProviderID invalid: %v", i, cp.ProviderID)
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
		t.Logf("uploaded: cid=%s, size=%d, copies=%d, url=%s",
			result.PieceCID, result.Size, len(result.Copies), uploadedURL)

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

			rc, err := client.Storage().Download(cctx, uploadedCID, &storage.DownloadOptions{
				URL: uploadedURL,
			})
			if err != nil {
				lastErr = err
				continue
			}

			data, err := io.ReadAll(rc)
			_ = rc.Close()
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

		result, err := client.Storage().UploadBytes(cctx, multiData, &storage.UploadOptions{
			Copies:  2,
			WithCDN: true,
		})
		if err != nil {
			t.Fatalf("UploadBytes (multicopy) failed: %v", err)
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
			if cp.ProviderID == nil {
				t.Errorf("Copy[%d].ProviderID is nil", i)
				continue
			}
			pid := cp.ProviderID.String()
			if _, dup := seen[pid]; dup {
				t.Errorf("Copy[%d] has duplicate ProviderID: %s", i, pid)
			}
			seen[pid] = struct{}{}

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
}
