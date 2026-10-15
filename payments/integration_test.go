//go:build integration

package payments_test

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"

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
		// Deposit a trivial amount to assert the permit-based chain path.
		one := big.NewInt(1)
		syncRes, err := p.FundSync(ctx, one, payments.WithConfirmations(2))
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
		t.Logf("FundSync tx=%s", syncRes.Hash)
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
			if loseResponse && lostResponse.accepted != nil {
				withdrawTx = lostResponse.accepted
			}
			if loseResponse {
				broadcast, ok := errors.AsType[*payments.BroadcastError](err)
				if withdraw != nil || !ok || !errors.Is(err, io.ErrUnexpectedEOF) || lostResponse.accepted == nil || lostResponse.sends != 1 {
					t.Fatalf("lost response result=%v error=%v sends=%d", withdraw, err, lostResponse.sends)
				}
				if broadcast.Transaction.Hash() != lostResponse.accepted.Hash() || broadcast.Transaction.Nonce() != lostResponse.accepted.Nonce() {
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

type lostBroadcastResponseBackend struct {
	payments.Backend
	accepted *ethtypes.Transaction
	sends    int
}

func (b *lostBroadcastResponseBackend) SendTransaction(ctx context.Context, tx *ethtypes.Transaction) error {
	b.sends++
	if err := b.Backend.SendTransaction(ctx, tx); err != nil {
		return err
	}
	b.accepted = tx
	return io.ErrUnexpectedEOF
}
