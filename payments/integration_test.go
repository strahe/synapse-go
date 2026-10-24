//go:build integration

package payments_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

// TestIntegration_Payments covers payment reads and account writes not already
// exercised by the cross-package suite. FundSync is conditional on token
// permit support. Withdraw uses one atto-USDFC and immediately deposits it
// back; terminated-rail settlement is covered by the staged storage flow.
func TestIntegration_Payments(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	client := integrationtest.NewDefaultClient(t, ctx)
	if client.Chain() != chain.Calibration {
		t.Fatal("payment write integration tests require Calibration")
	}
	p := client.Payments()

	// Trivially covered — assert non-zero shape.
	if (p.Address() == common.Address{}) {
		t.Fatal("Payments.Address is zero")
	}
	if p.ChainID() == 0 {
		t.Fatal("Payments.ChainID is 0")
	}
	if (p.Account() == common.Address{}) {
		t.Fatal("Payments.Account is zero (expected signer EOA)")
	}
	if p.Account() != client.Address() {
		t.Errorf("Payments.Account = %s, want client.Address() = %s",
			p.Account(), client.Address())
	}

	// WalletBalance for native FIL (zero token address).
	filBal, err := p.WalletBalance(ctx, common.Address{}, client.Address())
	if err != nil {
		t.Fatalf("WalletBalance(FIL): %v", err)
	}
	if filBal == nil || filBal.Sign() <= 0 {
		t.Fatalf("WalletBalance(FIL) should be > 0, got %v", filBal)
	}
	t.Logf("wallet FIL balance: %s", filBal)

	// WalletBalance with invalid zero account → ErrInvalidArgument.
	if _, err := p.WalletBalance(ctx, common.Address{}, common.Address{}); !errors.Is(err, payments.ErrInvalidArgument) {
		t.Errorf("WalletBalance(zero account): want ErrInvalidArgument, got %v", err)
	}

	// GetRailsAsPayer — should succeed even if zero rails.
	addrs := client.ResolvedAddresses()
	page, err := p.GetRailsAsPayer(ctx, client.Address(), addrs.USDFC, types.ListOptions{Limit: 100})
	if err != nil {
		t.Fatalf("GetRailsAsPayer: %v", err)
	}
	if page == nil {
		t.Fatal("GetRailsAsPayer returned nil page")
	}
	t.Logf("rails-as-payer first page: count=%d", len(page.Rails))

	fixed, err := p.TotalAccountFixedLockup(ctx, client.Address())
	if err != nil {
		t.Fatalf("TotalAccountFixedLockup: %v", err)
	}
	if fixed == nil || fixed.Sign() < 0 {
		t.Fatalf("TotalAccountFixedLockup returned %v, want >= 0", fixed)
	}
	t.Logf("total account fixed lockup: %s", fixed)

	t.Run("Uint256Overflow", func(t *testing.T) {
		limit := new(big.Int).Lsh(big.NewInt(1), 256)
		for _, tt := range []struct {
			name string
			call func(*big.Int) (*types.WriteResult, error)
		}{
			{name: "Approve", call: func(v *big.Int) (*types.WriteResult, error) {
				return p.Approve(ctx, addrs.USDFC, addrs.Payments, v)
			}},
			{name: "Deposit", call: func(v *big.Int) (*types.WriteResult, error) {
				return p.Deposit(ctx, addrs.USDFC, client.Address(), v, payments.WithSkipPrecheck())
			}},
			{name: "Withdraw", call: func(v *big.Int) (*types.WriteResult, error) {
				return p.Withdraw(ctx, addrs.USDFC, v, payments.WithSkipPrecheck())
			}},
			{name: "Fund", call: func(v *big.Int) (*types.WriteResult, error) {
				return p.Fund(ctx, v, payments.WithSkipPrecheck())
			}},
			{name: "ApproveService", call: func(v *big.Int) (*types.WriteResult, error) {
				return p.ApproveService(ctx, addrs.USDFC, addrs.FWSS, v, big.NewInt(1), big.NewInt(1))
			}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				for _, delta := range []int64{0, 7} {
					res, err := tt.call(new(big.Int).Add(limit, big.NewInt(delta)))
					if res != nil || !errors.Is(err, payments.ErrInvalidArgument) {
						t.Fatalf("overflow+%d: result=%v error=%v, want invalid argument", delta, res, err)
					}
				}
			})
		}
	})

	t.Run("MaximumERC20Allowance", func(t *testing.T) {
		original, err := p.Allowance(ctx, addrs.USDFC, client.Address(), addrs.Payments)
		if err != nil {
			t.Fatalf("Allowance before approval: %v", err)
		}
		t.Cleanup(func() {
			restoreCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			res, err := p.Approve(restoreCtx, addrs.USDFC, addrs.Payments, original, payments.WithWait(90*time.Second))
			if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
				t.Errorf("restore allowance: result=%v error=%v", res, err)
				return
			}
			got, err := p.Allowance(restoreCtx, addrs.USDFC, client.Address(), addrs.Payments)
			if err != nil {
				t.Errorf("Allowance after restore: %v", err)
			} else if got.Cmp(original) != 0 {
				t.Errorf("restored allowance=%s, want %s", got, original)
			}
		})
		maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
		res, err := p.Approve(ctx, addrs.USDFC, addrs.Payments, maximum, payments.WithWait(90*time.Second))
		if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
			t.Fatalf("maximum allowance approval: result=%v error=%v", res, err)
		}
		got, err := p.Allowance(ctx, addrs.USDFC, client.Address(), addrs.Payments)
		if err != nil {
			t.Fatalf("Allowance after approval: %v", err)
		}
		if got.Cmp(maximum) != 0 {
			t.Fatalf("allowance=%s, want maximum %s", got, maximum)
		}
		t.Logf("maximum allowance confirmed: tx=%s", res.Hash)
	})

	t.Run("FundSync", func(t *testing.T) {
		rpcClient, err := ethclient.DialContext(ctx, integrationtest.RPCURL())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rpcClient.Close)
		chainID, err := rpcClient.ChainID(ctx)
		if err != nil || chainID.Cmp(big.NewInt(integrationtest.CalibrationChainID)) != 0 {
			t.Fatalf("funding RPC must be Calibration: chain=%v error=%v", chainID, err)
		}
		key, err := hex.DecodeString(strings.TrimPrefix(integrationtest.RequirePrivateKey(t), "0x"))
		if err != nil {
			t.Fatal("invalid integration signing key")
		}
		defer clear(key)
		sg, err := signer.NewSecp256k1SignerFromBytes(key)
		if err != nil {
			t.Fatal(err)
		}
		backend := &transientReceiptBackend{Backend: rpcClient}
		writer, err := payments.New(payments.Options{
			Backend: backend, Signer: sg,
			ChainID:       types.ChainID(integrationtest.CalibrationChainID),
			FilPayAddress: addrs.Payments, WarmStorageAddress: addrs.FWSS,
			USDFCTokenAddress: addrs.USDFC,
		})
		if err != nil {
			t.Fatal(err)
		}
		one := big.NewInt(1)
		syncRes, err := writer.FundSync(ctx, one, payments.WithConfirmations(2))
		if err != nil {
			if errors.Is(err, payments.ErrPermitUnsupported) {
				t.Skip("needs-usdfc-permit-support: USDFC contract does not implement EIP-2612 permit")
			}
			t.Fatalf("FundSync: %v", err)
		}
		if syncRes == nil || syncRes.Receipt == nil {
			t.Fatal("FundSync should return a non-nil receipt")
		}
		if syncRes.Receipt.Status != 1 {
			t.Fatalf("FundSync tx failed: status=%d", syncRes.Receipt.Status)
		}
		if sends, polls := backend.sends.Load(), backend.polls.Load(); sends != 1 || polls < 2 {
			t.Fatalf("FundSync sends=%d polls=%d, want one submission and receipt recovery after HTTP 429", sends, polls)
		}
		head := backend.head.Load()
		if block := syncRes.Receipt.BlockNumber; block == nil || !block.IsUint64() || head <= block.Uint64() {
			t.Fatalf("FundSync receipt block=%v head=%d, want at least two confirmations", block, head)
		}
		t.Logf("FundSync recovered after HTTP 429: tx=%s sends=%d polls=%d", syncRes.Hash, backend.sends.Load(), backend.polls.Load())
	})

	t.Run("PermitBroadcastRecovery", func(t *testing.T) {
		rpc, err := ethclient.DialContext(ctx, integrationtest.RPCURL())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rpc.Close)
		chainID, err := rpc.ChainID(ctx)
		if err != nil || chainID.Cmp(big.NewInt(integrationtest.CalibrationChainID)) != 0 {
			t.Fatalf("permit recovery RPC must be Calibration: chain=%v error=%v", chainID, err)
		}
		key, err := hex.DecodeString(strings.TrimPrefix(integrationtest.RequirePrivateKey(t), "0x"))
		if err != nil {
			t.Fatal("invalid integration signing key")
		}
		defer clear(key)
		sg, err := signer.NewSecp256k1SignerFromBytes(key)
		if err != nil {
			t.Fatal(err)
		}
		lostResponse := &lostBroadcastResponseBackend{Backend: rpc}
		writer, err := payments.New(payments.Options{
			Backend: lostResponse, Signer: sg,
			ChainID:           types.ChainID(integrationtest.CalibrationChainID),
			FilPayAddress:     addrs.Payments,
			PermitReceiptWait: 2 * time.Minute,
		})
		if err != nil {
			t.Fatal(err)
		}
		one := big.NewInt(1)
		wallet, err := writer.WalletBalance(ctx, addrs.USDFC, client.Address())
		if err != nil {
			t.Fatalf("WalletBalance(before permits): %v", err)
		}
		if wallet == nil || wallet.Cmp(big.NewInt(2)) < 0 {
			t.Fatalf("wallet USDFC %v is insufficient for two one atto-USDFC deposits", wallet)
		}
		allowance, err := writer.Allowance(ctx, addrs.USDFC, client.Address(), addrs.Payments)
		if err != nil || allowance == nil {
			t.Fatalf("Allowance(before permits): value=%v error=%v", allowance, err)
		}
		fwssBefore, err := writer.ServiceApproval(ctx, addrs.USDFC, client.Address(), addrs.FWSS)
		if err != nil || fwssBefore == nil {
			t.Fatalf("ServiceApproval(FWSS before): value=%v error=%v", fwssBefore, err)
		}
		var operator common.Address
		if _, err := rand.Read(operator[:]); err != nil {
			t.Fatal(err)
		}
		if operator == (common.Address{}) || operator == addrs.FWSS || operator == client.Address() {
			t.Fatal("random operator must be distinct from existing test accounts")
		}
		isZero := func(v *big.Int) bool { return v != nil && v.Sign() == 0 }
		hasZeroApproval := func(v *payments.OperatorApproval, approved bool) bool {
			return v != nil && v.IsApproved == approved && isZero(v.RateAllowance) &&
				isZero(v.LockupAllowance) && isZero(v.MaxLockupPeriod) &&
				isZero(v.RateUsage) && isZero(v.LockupUsage)
		}
		approval, err := writer.ServiceApproval(ctx, addrs.USDFC, client.Address(), operator)
		if err != nil || !hasZeroApproval(approval, false) {
			t.Fatalf("ServiceApproval(test operator before): value=%+v error=%v", approval, err)
		}
		var backgroundHashes []common.Hash
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cleanupCancel()
			cleanupCtx = context.WithValue(cleanupCtx, permitForegroundContextKey{}, true)
			defer func() {
				for _, hash := range backgroundHashes {
					if err := lostResponse.waitBackgroundReceipt(cleanupCtx, hash); err != nil {
						t.Errorf("cleanup permit receipt watcher %s: %v", hash, err)
					}
				}
			}()
			_, sent := lostResponse.snapshot()
			for _, tx := range sent {
				if _, err := bind.WaitMined(cleanupCtx, rpc, tx); err != nil {
					t.Errorf("cleanup could not determine original transaction %s outcome: %v", tx.Hash(), err)
					return
				}
			}
			waitCleanupWrite := func(result *types.WriteResult, err error) bool {
				if err == nil && result != nil && result.Receipt != nil && result.Receipt.Status == ethtypes.ReceiptStatusSuccessful {
					return true
				}
				var txHash common.Hash
				if result != nil {
					txHash = result.Hash
				}
				if broadcast, ok := errors.AsType[*payments.BroadcastError](err); ok {
					txHash = broadcast.Transaction.Hash()
				}
				_, submitted := lostResponse.snapshot()
				for _, tx := range submitted {
					if tx.Hash() != txHash {
						continue
					}
					receipt, waitErr := bind.WaitMined(cleanupCtx, rpc, tx)
					if waitErr == nil && receipt.Status == ethtypes.ReceiptStatusSuccessful {
						return true
					}
					t.Errorf("cleanup transaction %s: receipt=%+v error=%v", txHash, receipt, waitErr)
					return false
				}
				t.Errorf("cleanup write: result=%+v error=%v", result, err)
				return false
			}
			approval, err := writer.ServiceApproval(cleanupCtx, addrs.USDFC, client.Address(), operator)
			if err != nil || approval == nil {
				t.Errorf("cleanup read operator approval: %v", err)
				return
			}
			if approval.IsApproved && !waitCleanupWrite(writer.RevokeService(cleanupCtx, addrs.USDFC, operator, payments.WithWait(90*time.Second))) {
				return
			}
			approval, err = writer.ServiceApproval(cleanupCtx, addrs.USDFC, client.Address(), operator)
			if err != nil || !hasZeroApproval(approval, false) {
				t.Errorf("cleanup operator approval: value=%+v error=%v", approval, err)
			}
			currentAllowance, err := writer.Allowance(cleanupCtx, addrs.USDFC, client.Address(), addrs.Payments)
			if err != nil || currentAllowance == nil {
				t.Errorf("cleanup read allowance: value=%v error=%v", currentAllowance, err)
				return
			}
			if currentAllowance.Cmp(allowance) != 0 && !waitCleanupWrite(writer.Approve(cleanupCtx, addrs.USDFC, addrs.Payments, allowance, payments.WithWait(90*time.Second))) {
				return
			}
			currentAllowance, err = writer.Allowance(cleanupCtx, addrs.USDFC, client.Address(), addrs.Payments)
			if err != nil || currentAllowance == nil || currentAllowance.Cmp(allowance) != 0 {
				t.Errorf("cleanup allowance: value=%v want=%v error=%v", currentAllowance, allowance, err)
			}
			fwssAfter, err := writer.ServiceApproval(cleanupCtx, addrs.USDFC, client.Address(), addrs.FWSS)
			if err != nil || fwssAfter == nil || fwssAfter.IsApproved != fwssBefore.IsApproved ||
				fwssAfter.RateAllowance.Cmp(fwssBefore.RateAllowance) != 0 ||
				fwssAfter.LockupAllowance.Cmp(fwssBefore.LockupAllowance) != 0 ||
				fwssAfter.MaxLockupPeriod.Cmp(fwssBefore.MaxLockupPeriod) != 0 {
				t.Errorf("FWSS approval configuration changed: before=%+v after=%+v error=%v", fwssBefore, fwssAfter, err)
			}
		})

		writeCtx := context.WithValue(ctx, permitForegroundContextKey{}, true)
		first, err := writer.DepositWithPermit(writeCtx, addrs.USDFC, common.Address{}, one, nil)
		broadcast, ok := errors.AsType[*payments.BroadcastError](err)
		accepted, sent := lostResponse.snapshot()
		if len(sent) > 0 {
			backgroundHashes = append(backgroundHashes, sent[0].Hash())
		}
		if first != nil || !ok || !errors.Is(err, io.ErrUnexpectedEOF) || accepted == nil || len(sent) != 1 {
			t.Fatalf("lost permit response: result=%v error=%v sends=%d", first, err, len(sent))
		}
		if broadcast.Transaction.Hash() != accepted.Hash() || broadcast.Transaction.Nonce() != accepted.Nonce() {
			t.Fatal("broadcast error lost the original permit transaction identity")
		}
		zero := new(big.Int)
		second, err := writer.DepositWithPermitAndApproveOperator(writeCtx, addrs.USDFC, client.Address(), one, nil,
			operator, zero, zero, zero, payments.WithWait(90*time.Second))
		if err != nil {
			if broadcast, ok := errors.AsType[*payments.BroadcastError](err); ok {
				backgroundHashes = append(backgroundHashes, broadcast.Transaction.Hash())
			} else if second != nil && second.Receipt == nil {
				backgroundHashes = append(backgroundHashes, second.Hash)
			}
		}
		if err != nil {
			t.Fatalf("DepositWithPermitAndApproveOperator after EOF: %v", err)
		}
		if second == nil || second.Receipt == nil || second.Receipt.Status != ethtypes.ReceiptStatusSuccessful {
			t.Fatalf("second permit receipt = %+v", second)
		}
		receipt, err := bind.WaitMined(ctx, rpc, accepted)
		if err != nil || receipt == nil || receipt.Status != ethtypes.ReceiptStatusSuccessful {
			t.Fatalf("original permit receipt: value=%+v error=%v", receipt, err)
		}
		events, err := filpay.NewFilPayFilterer(addrs.Payments, rpc)
		if err != nil {
			t.Fatal(err)
		}
		for _, receipt := range []*ethtypes.Receipt{receipt, second.Receipt} {
			deposits := 0
			for _, log := range receipt.Logs {
				if log.Address != addrs.Payments {
					continue
				}
				event, err := events.ParseDepositRecorded(*log)
				if err != nil {
					continue
				}
				deposits++
				if event.Token != addrs.USDFC || event.From != client.Address() || event.To != client.Address() || event.Amount == nil || event.Amount.Cmp(one) != 0 {
					t.Fatalf("DepositRecorded = %+v, want token=%s from=%s to=%s amount=1", event, addrs.USDFC, client.Address(), client.Address())
				}
			}
			if deposits != 1 {
				t.Fatalf("permit %s DepositRecorded count = %d, want 1", receipt.TxHash, deposits)
			}
		}
		approval, err = writer.ServiceApproval(ctx, addrs.USDFC, client.Address(), operator)
		if err != nil || !hasZeroApproval(approval, true) {
			t.Fatalf("ServiceApproval(test operator after): value=%+v error=%v", approval, err)
		}
		t.Logf("recovered permit %s and completed permit %s; two atto-USDFC remain in the Filecoin Pay account", accepted.Hash(), second.Hash)
	})

	for _, loseResponse := range []bool{false, true} {
		name := "WithdrawAndRestore"
		if loseResponse {
			name = "WithdrawAcceptedThenEOFAndRestore"
		}
		t.Run(name, func(t *testing.T) {
			writer := p
			rpc, err := ethclient.DialContext(ctx, integrationtest.RPCURL())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(rpc.Close)
			chainID, err := rpc.ChainID(ctx)
			if err != nil || chainID.Cmp(big.NewInt(integrationtest.CalibrationChainID)) != 0 {
				t.Fatalf("withdrawal RPC must be Calibration: chain=%v error=%v", chainID, err)
			}
			events, err := filpay.NewFilPayFilterer(addrs.Payments, rpc)
			if err != nil {
				t.Fatal(err)
			}
			var lostResponse *lostBroadcastResponseBackend
			if loseResponse {
				key, err := hex.DecodeString(strings.TrimPrefix(integrationtest.RequirePrivateKey(t), "0x"))
				if err != nil {
					t.Fatal("invalid integration signing key")
				}
				defer clear(key)
				sg, err := signer.NewSecp256k1SignerFromBytes(key)
				if err != nil {
					t.Fatal(err)
				}
				lostResponse = &lostBroadcastResponseBackend{Backend: rpc}
				writer, err = payments.New(payments.Options{
					Backend: lostResponse, Signer: sg,
					ChainID:       types.ChainID(integrationtest.CalibrationChainID),
					FilPayAddress: addrs.Payments,
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			one := big.NewInt(1)
			before, err := p.AccountInfo(ctx, addrs.USDFC, client.Address())
			if err != nil {
				t.Fatalf("AccountInfo(before): %v", err)
			}
			if before == nil {
				t.Fatal("AccountInfo(before) returned nil")
			}
			if available := before.AvailableFunds(); available == nil || available.Cmp(one) < 0 {
				t.Fatalf("available funds %v are insufficient for a one atto-USDFC withdrawal", available)
			}
			beforeWallet, err := p.WalletBalance(ctx, addrs.USDFC, client.Address())
			if err != nil {
				t.Fatalf("WalletBalance(before): %v", err)
			}
			allowance, err := p.Allowance(ctx, addrs.USDFC, client.Address(), addrs.Payments)
			if err != nil {
				t.Fatalf("Allowance(before withdrawal): %v", err)
			}
			if allowance == nil {
				t.Fatal("Allowance(before withdrawal) returned nil")
			}
			if allowance.Cmp(one) < 0 {
				approve, err := p.Approve(ctx, addrs.USDFC, addrs.Payments, one, payments.WithWait(90*time.Second))
				if err != nil {
					t.Fatalf("Approve(restore amount): %v", err)
				}
				if approve == nil || approve.Receipt == nil || approve.Receipt.Status != 1 {
					t.Fatalf("Approve receipt = %+v", approve.Receipt)
				}
			}

			withdrawn := false
			restored := false
			var withdrawTx *ethtypes.Transaction
			var restoreTx *ethtypes.Transaction
			t.Cleanup(func() {
				if restored || !withdrawn && withdrawTx == nil {
					return
				}
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cleanupCancel()
				if restoreTx != nil {
					receipt, err := bind.WaitMined(cleanupCtx, rpc, restoreTx)
					if err != nil {
						t.Errorf("cleanup could not determine deposit outcome: %v", err)
						return
					}
					if receipt.Status == ethtypes.ReceiptStatusSuccessful {
						return
					}
				}
				if !withdrawn {
					receipt, err := bind.WaitMined(cleanupCtx, rpc, withdrawTx)
					if err != nil {
						t.Errorf("cleanup could not determine withdrawal outcome: %v", err)
						return
					}
					if receipt.Status != ethtypes.ReceiptStatusSuccessful {
						return
					}
				}
				if _, err := p.Deposit(cleanupCtx, addrs.USDFC, client.Address(), one, payments.WithWait(90*time.Second)); err != nil {
					t.Errorf("cleanup Deposit(1 atto-USDFC): %v", err)
				}
			})

			withdraw, err := writer.Withdraw(ctx, addrs.USDFC, one, payments.WithWait(90*time.Second))
			if broadcast, ok := errors.AsType[*payments.BroadcastError](err); ok {
				withdrawTx = broadcast.Transaction
			}
			if loseResponse {
				accepted, sent := lostResponse.snapshot()
				if accepted != nil {
					withdrawTx = accepted
				}
				broadcast, ok := errors.AsType[*payments.BroadcastError](err)
				if withdraw != nil || !ok || !errors.Is(err, io.ErrUnexpectedEOF) || accepted == nil || len(sent) != 1 {
					t.Fatalf("lost response result=%v error=%v sends=%d", withdraw, err, len(sent))
				}
				if broadcast.Transaction.Hash() != accepted.Hash() || broadcast.Transaction.Nonce() != accepted.Nonce() {
					t.Fatal("broadcast error lost the accepted transaction identity")
				}
				recoveryCtx, recoveryCancel := context.WithTimeout(ctx, 90*time.Second)
				defer recoveryCancel()
				receipt, waitErr := bind.WaitMined(recoveryCtx, rpc, broadcast.Transaction)
				if waitErr != nil {
					t.Fatalf("query original withdrawal: %v", waitErr)
				}
				withdraw = &types.WriteResult{Hash: broadcast.Transaction.Hash(), Receipt: receipt}
				err = nil
				t.Logf("recovered accepted withdrawal after EOF: tx=%s nonce=%d", withdraw.Hash, broadcast.Transaction.Nonce())
			}
			if err != nil {
				t.Fatalf("Withdraw(1 atto-USDFC): %v", err)
			}
			if withdraw == nil || withdraw.Receipt == nil || withdraw.Receipt.Status != 1 {
				t.Fatalf("Withdraw receipt = %+v", withdraw.Receipt)
			}
			withdrawn = true

			deposit, err := p.Deposit(ctx, addrs.USDFC, client.Address(), one, payments.WithWait(90*time.Second))
			if err != nil {
				if broadcast, ok := errors.AsType[*payments.BroadcastError](err); ok {
					restoreTx = broadcast.Transaction
				}
				t.Fatalf("Deposit(restore 1 atto-USDFC): %v", err)
			}
			if deposit == nil || deposit.Receipt == nil || deposit.Receipt.Status != 1 {
				t.Fatalf("Deposit receipt = %+v", deposit.Receipt)
			}
			restored = true
			withdrawEvents := 0
			for _, log := range withdraw.Receipt.Logs {
				if log.Address != addrs.Payments {
					continue
				}
				event, err := events.ParseWithdrawRecorded(*log)
				if err != nil {
					continue
				}
				withdrawEvents++
				if event.Token != addrs.USDFC || event.From != client.Address() || event.To != client.Address() || event.Amount == nil || event.Amount.Cmp(one) != 0 {
					t.Fatalf("WithdrawRecorded = %+v, want token=%s from=%s to=%s amount=1", event, addrs.USDFC, client.Address(), client.Address())
				}
			}
			if withdrawEvents != 1 {
				t.Fatalf("WithdrawRecorded count = %d, want 1", withdrawEvents)
			}
			depositEvents := 0
			for _, log := range deposit.Receipt.Logs {
				if log.Address != addrs.Payments {
					continue
				}
				event, err := events.ParseDepositRecorded(*log)
				if err != nil {
					continue
				}
				depositEvents++
				if event.Token != addrs.USDFC || event.From != client.Address() || event.To != client.Address() || event.Amount == nil || event.Amount.Cmp(one) != 0 {
					t.Fatalf("DepositRecorded = %+v, want token=%s from=%s to=%s amount=1", event, addrs.USDFC, client.Address(), client.Address())
				}
			}
			if depositEvents != 1 {
				t.Fatalf("DepositRecorded count = %d, want 1", depositEvents)
			}

			after, err := p.AccountInfo(ctx, addrs.USDFC, client.Address())
			if err != nil {
				t.Fatalf("AccountInfo(after): %v", err)
			}
			if after == nil {
				t.Fatal("AccountInfo(after) returned nil")
			}
			afterWallet, err := p.WalletBalance(ctx, addrs.USDFC, client.Address())
			if err != nil {
				t.Fatalf("WalletBalance(after): %v", err)
			}
			t.Logf(
				"Withdraw/Deposit restored principal: account funds delta=%s, available delta=%s, wallet delta=%s, lockup-current delta=%s",
				new(big.Int).Sub(after.Funds, before.Funds),
				new(big.Int).Sub(after.AvailableFunds(), before.AvailableFunds()),
				new(big.Int).Sub(afterWallet, beforeWallet),
				new(big.Int).Sub(after.LockupCurrent, before.LockupCurrent),
			)
		})
	}
}

type transientReceiptBackend struct {
	payments.Backend
	sends atomic.Int32
	polls atomic.Int32
	head  atomic.Uint64
}

func (b *transientReceiptBackend) SendTransaction(ctx context.Context, tx *ethtypes.Transaction) error {
	b.sends.Add(1)
	return b.Backend.SendTransaction(ctx, tx)
}

func (b *transientReceiptBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*ethtypes.Receipt, error) {
	if b.polls.Add(1) == 1 {
		return nil, rpc.HTTPError{StatusCode: 429, Status: "429 Too Many Requests"}
	}
	return b.Backend.TransactionReceipt(ctx, txHash)
}

func (b *transientReceiptBackend) BlockNumber(ctx context.Context) (uint64, error) {
	head, err := b.Backend.BlockNumber(ctx)
	if err == nil {
		b.head.Store(head)
	}
	return head, err
}

type lostBroadcastResponseBackend struct {
	payments.Backend
	mu                sync.Mutex
	accepted          *ethtypes.Transaction
	sent              []*ethtypes.Transaction
	backgroundStarted map[common.Hash]chan struct{}
	backgroundDone    map[common.Hash]<-chan struct{}
}

type permitForegroundContextKey struct{}

func (b *lostBroadcastResponseBackend) SendTransaction(ctx context.Context, tx *ethtypes.Transaction) error {
	b.mu.Lock()
	b.sent = append(b.sent, tx)
	if b.backgroundStarted == nil {
		b.backgroundStarted = make(map[common.Hash]chan struct{})
		b.backgroundDone = make(map[common.Hash]<-chan struct{})
	}
	if b.backgroundStarted[tx.Hash()] == nil {
		b.backgroundStarted[tx.Hash()] = make(chan struct{})
	}
	b.mu.Unlock()
	if err := b.Backend.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.accepted == nil {
		b.accepted = tx
		return io.ErrUnexpectedEOF
	}
	return nil
}

func (b *lostBroadcastResponseBackend) TransactionReceipt(ctx context.Context, txHash common.Hash) (*ethtypes.Receipt, error) {
	b.mu.Lock()
	if ctx.Value(permitForegroundContextKey{}) == nil && b.backgroundStarted[txHash] != nil && b.backgroundDone[txHash] == nil {
		b.backgroundDone[txHash] = ctx.Done()
		close(b.backgroundStarted[txHash])
	}
	b.mu.Unlock()
	return b.Backend.TransactionReceipt(ctx, txHash)
}

func (b *lostBroadcastResponseBackend) snapshot() (*ethtypes.Transaction, []*ethtypes.Transaction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.accepted, append([]*ethtypes.Transaction(nil), b.sent...)
}

func (b *lostBroadcastResponseBackend) waitBackgroundReceipt(ctx context.Context, hash common.Hash) error {
	b.mu.Lock()
	started := b.backgroundStarted[hash]
	b.mu.Unlock()
	select {
	case <-started:
	case <-ctx.Done():
		return ctx.Err()
	}
	b.mu.Lock()
	done := b.backgroundDone[hash]
	b.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
