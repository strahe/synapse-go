//go:build integration

package sessionkey_test

import (
	"context"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/sessionkey"
)

// TestIntegration_SessionKey exercises the WithOptions / AndFund / GetExpirations
// surface of sessionkey.Service that the existing cross-package SessionKey
// subtest does not already cover (which uses the simpler Login / Revoke).
//
// Each subtest uses a freshly generated ephemeral session key so we can
// assert strict invariants on each lifecycle step without interference
// from prior runs.
func TestIntegration_SessionKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	client := integrationtest.NewDefaultClient(t, ctx)
	sk := client.SessionKey()

	// RegistryAddress is trivially covered but assert non-zero.
	if (sk.RegistryAddress() == common.Address{}) {
		t.Fatal("RegistryAddress is zero")
	}

	txWait := 90 * time.Second
	rootAddr := client.Address()

	t.Run("LoginWithOptions+GetExpirations+RevokeWithOptions", func(t *testing.T) {
		skPriv, err := ecdsa.GenerateKey(crypto.S256(), cryptorand.Reader)
		if err != nil {
			t.Fatalf("generate session key: %v", err)
		}
		skAddr := crypto.PubkeyToAddress(skPriv.PublicKey)

		// Short expiry on just CreateDataSet + AddPieces keeps any residual
		// authorization bounded without an extra cleanup transaction.
		expiresAt := uint64(time.Now().Add(10 * time.Minute).Unix())
		perms := []sessionkey.Permission{
			sessionkey.CreateDataSetPermission,
			sessionkey.AddPiecesPermission,
		}

		res, err := sk.LoginWithOptions(ctx, skAddr, &sessionkey.LoginOptions{
			Permissions: perms,
			ExpiresAt:   expiresAt,
			Origin:      "integration-test",
		}, sessionkey.WithWait(txWait))
		if err != nil {
			t.Fatalf("LoginWithOptions: %v", err)
		}
		if res.Receipt == nil || res.Receipt.Status != 1 {
			t.Fatalf("LoginWithOptions tx not successful: %+v", res.Receipt)
		}
		t.Logf("LoginWithOptions tx=%s", res.Hash)

		// GetExpirations with explicit permission slice.
		exps, err := sk.GetExpirations(ctx, rootAddr, skAddr, perms)
		if err != nil {
			t.Fatalf("GetExpirations(perms): %v", err)
		}
		for _, p := range perms {
			got, ok := exps[p]
			if !ok {
				t.Errorf("GetExpirations missing permission %s", p.Hex())
				continue
			}
			// On-chain expiry should be >= our requested ExpiresAt - 10s jitter.
			if got+10 < expiresAt {
				t.Errorf("permission %s expiry %d < expected %d", p.Hex(), got, expiresAt)
			}
		}

		// GetExpirations with default (empty) slice — should return the
		// default FWSS permission set. DeleteDataSet was not authorised
		// here so its expiry should be 0.
		defaultExps, err := sk.GetExpirations(ctx, rootAddr, skAddr, nil)
		if err != nil {
			t.Fatalf("GetExpirations(default): %v", err)
		}
		if defaultExps[sessionkey.DeleteDataSetPermission] != 0 {
			t.Errorf("DeleteDataSet expiry = %d, want 0 (not authorised)",
				defaultExps[sessionkey.DeleteDataSetPermission])
		}
		if defaultExps[sessionkey.CreateDataSetPermission] == 0 {
			t.Error("CreateDataSet expiry = 0, want > 0 after login")
		}

		// RevokeWithOptions for just the AddPieces permission.
		revRes, err := sk.RevokeWithOptions(ctx, skAddr, &sessionkey.RevokeOptions{
			Permissions: []sessionkey.Permission{sessionkey.AddPiecesPermission},
			Origin:      "integration-test",
		}, sessionkey.WithWait(txWait))
		if err != nil {
			t.Fatalf("RevokeWithOptions: %v", err)
		}
		if revRes.Receipt == nil || revRes.Receipt.Status != 1 {
			t.Fatalf("RevokeWithOptions tx not successful")
		}
		t.Logf("RevokeWithOptions tx=%s", revRes.Hash)

		// AddPieces should now be expired, CreateDataSet still active.
		expiredAddPieces, err := sk.IsExpired(ctx, rootAddr, skAddr, sessionkey.AddPiecesPermission)
		if err != nil {
			t.Fatalf("IsExpired(AddPieces): %v", err)
		}
		if !expiredAddPieces {
			t.Error("AddPieces should be expired after partial revoke")
		}
		exp, err := sk.AuthorizationExpiry(ctx, rootAddr, skAddr, sessionkey.CreateDataSetPermission)
		if err != nil {
			t.Fatalf("AuthorizationExpiry(CreateDataSet): %v", err)
		}
		if exp == 0 {
			t.Error("CreateDataSet should still be authorised after partial revoke")
		}

	})

	t.Run("LoginAndFund+LoginAndFundWithOptions", func(t *testing.T) {
		// Generate a session key and fund it with 1 attoFIL via LoginAndFund.
		skPriv1, err := ecdsa.GenerateKey(crypto.S256(), cryptorand.Reader)
		if err != nil {
			t.Fatalf("generate session key: %v", err)
		}
		skAddr1 := crypto.PubkeyToAddress(skPriv1.PublicKey)

		one := big.NewInt(1)
		res, err := sk.LoginAndFund(ctx, skAddr1, one, sessionkey.WithWait(txWait))
		if err != nil {
			t.Fatalf("LoginAndFund: %v", err)
		}
		if res.Receipt == nil || res.Receipt.Status != 1 {
			t.Fatalf("LoginAndFund tx not successful")
		}
		t.Logf("LoginAndFund tx=%s", res.Hash)

		bal, err := client.Payments().WalletBalance(ctx, common.Address{}, skAddr1)
		if err != nil {
			t.Fatalf("WalletBalance(sk1): %v", err)
		}
		if bal.Sign() <= 0 {
			t.Errorf("session key should have > 0 FIL after LoginAndFund, got %s", bal)
		}

		// LoginAndFundWithOptions — custom permissions + funding.
		skPriv2, err := ecdsa.GenerateKey(crypto.S256(), cryptorand.Reader)
		if err != nil {
			t.Fatalf("generate session key: %v", err)
		}
		skAddr2 := crypto.PubkeyToAddress(skPriv2.PublicKey)

		res2, err := sk.LoginAndFundWithOptions(ctx, skAddr2, one, &sessionkey.LoginOptions{
			Permissions: []sessionkey.Permission{sessionkey.CreateDataSetPermission},
			ExpiresAt:   uint64(time.Now().Add(10 * time.Minute).Unix()),
			Origin:      "integration-test",
		}, sessionkey.WithWait(txWait))
		if err != nil {
			t.Fatalf("LoginAndFundWithOptions: %v", err)
		}
		if res2.Receipt == nil || res2.Receipt.Status != 1 {
			t.Fatalf("LoginAndFundWithOptions tx not successful")
		}

		// Only CreateDataSet should be authorised.
		expAdd, err := sk.AuthorizationExpiry(ctx, rootAddr, skAddr2, sessionkey.AddPiecesPermission)
		if err != nil {
			t.Fatalf("AuthorizationExpiry(AddPieces): %v", err)
		}
		if expAdd != 0 {
			t.Errorf("AddPieces expiry = %d, want 0 (only CreateDataSet was requested)", expAdd)
		}

	})
}
