//go:build integration

package integration_test

import (
	"context"
	"errors"
	"math/big"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/payments"
)

// TestNegatives runs negative integration scenarios against a calibration
// network. Unlike the happy-path TestIntegration, each subtest deliberately
// triggers a failure mode and asserts the SDK surfaces a recognizable error.
//
// All scenarios are gated on INTEGRATION_PRIVATE_KEY (calibration tFIL +
// USDFC). When unset the function skips, mirroring TestIntegration.
func TestNegatives(t *testing.T) {
	if err := loadEnvFile("../../.env"); err != nil {
		t.Logf("warning: failed to load .env file: %v", err)
	}

	privateKeyHex := os.Getenv("INTEGRATION_PRIVATE_KEY")
	if privateKeyHex == "" {
		t.Skip("INTEGRATION_PRIVATE_KEY not set; skipping negative integration tests")
	}

	rpcURL := os.Getenv("INTEGRATION_RPC_URL")
	if rpcURL == "" {
		rpcURL = defaultRPCURL
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	client, err := synapse.New(ctx,
		synapse.WithPrivateKeyHex(privateKeyHex),
		synapse.WithRPCURL(rpcURL),
	)
	if err != nil {
		t.Fatalf("synapse.New: %v", err)
	}
	defer client.Close()

	t.Run("InsufficientFundsDeposit", func(t *testing.T) {
		testInsufficientFundsDeposit(ctx, t, client)
	})

	t.Run("NonceConflictRecovery", func(t *testing.T) {
		testNonceConflictRecovery(ctx, t, client)
	})

	t.Run("ExpiredSessionKeyRejected", func(t *testing.T) {
		t.Skip("placeholder: requires WarmStorage write path with expired session-key permit; tracked for follow-up")
	})
}

// testInsufficientFundsDeposit attempts to deposit an amount far in excess
// of the account's USDFC balance and asserts the returned error is either
// detectable by Is(err, payments.ErrTxFailed) or by a substring match on
// known revert reasons.
func testInsufficientFundsDeposit(ctx context.Context, t *testing.T, client *synapse.Client) {
	pm := client.Payments()
	if pm == nil {
		t.Skip("payments service not available on client")
	}

	addrs := client.Chain().Addresses()
	usdfc := addrs.USDFC
	to := client.Address()

	// Choose an obviously-unfundable amount: max uint128.
	huge := new(big.Int).Lsh(big.NewInt(1), 128)
	huge.Sub(huge, big.NewInt(1))

	depositCtx, cancel := context.WithTimeout(ctx, txWaitTimeout)
	defer cancel()

	_, err := pm.Deposit(depositCtx, usdfc, to, huge)
	if err == nil {
		t.Fatalf("Deposit(%v) succeeded; expected insufficient-funds error", huge)
	}
	low := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, payments.ErrTxFailed):
	case strings.Contains(low, "insufficient"):
	case strings.Contains(low, "exceeds balance"):
	case strings.Contains(low, "transfer amount exceeds"):
	case strings.Contains(low, "allowance"):
	default:
		t.Fatalf("Deposit error did not look like insufficient funds: %v", err)
	}
}

// testNonceConflictRecovery fires several small deposits concurrently and
// asserts the embedded NonceManager serialises them: every call must either
// succeed or fail with a non-nonce error.
func testNonceConflictRecovery(ctx context.Context, t *testing.T, client *synapse.Client) {
	pm := client.Payments()
	if pm == nil {
		t.Skip("payments service not available on client")
	}

	addrs := client.Chain().Addresses()
	usdfc := addrs.USDFC
	to := client.Address()

	const concurrent = 5
	amount := big.NewInt(1) // dust deposit; insufficient-allowance is fine, nonce-too-low is not
	type res struct {
		idx int
		err error
	}
	results := make([]res, concurrent)
	var wg sync.WaitGroup
	wg.Add(concurrent)
	for i := 0; i < concurrent; i++ {
		i := i
		go func() {
			defer wg.Done()
			subCtx, cancel := context.WithTimeout(ctx, txWaitTimeout)
			defer cancel()
			_, err := pm.Deposit(subCtx, usdfc, to, amount)
			results[i] = res{idx: i, err: err}
		}()
	}
	wg.Wait()

	for _, r := range results {
		if r.err == nil {
			continue
		}
		low := strings.ToLower(r.err.Error())
		if strings.Contains(low, "nonce too low") || strings.Contains(low, "nonce already used") {
			t.Fatalf("Deposit #%d hit nonce conflict: %v", r.idx, r.err)
		}
	}
}
