//go:build integration

package sessionkey_test

import (
	"context"
	"crypto/ecdsa"
	cryptorand "crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

// TestIntegration_SessionKey covers custom permissions, authorization reads,
// and funded login followed by revocation.
//
// Each subtest uses a freshly generated ephemeral session key so we can
// assert strict invariants on each lifecycle step without interference
// from prior runs.
func TestIntegration_SessionKey(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
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
			sessionkey.CreateDataSetPermission(),
			sessionkey.AddPiecesPermission(),
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
		// default FWSS permission set. TerminateService was not authorised
		// here so its expiry should be 0.
		defaultExps, err := sk.GetExpirations(ctx, rootAddr, skAddr, nil)
		if err != nil {
			t.Fatalf("GetExpirations(default): %v", err)
		}
		if defaultExps[sessionkey.TerminateServicePermission()] != 0 {
			t.Errorf("TerminateService expiry = %d, want 0 (not authorised)",
				defaultExps[sessionkey.TerminateServicePermission()])
		}
		if defaultExps[sessionkey.CreateDataSetPermission()] == 0 {
			t.Error("CreateDataSet expiry = 0, want > 0 after login")
		}

		// RevokeWithOptions for just the AddPieces permission.
		revRes, err := sk.RevokeWithOptions(ctx, skAddr, &sessionkey.RevokeOptions{
			Permissions: []sessionkey.Permission{sessionkey.AddPiecesPermission()},
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
		expiredAddPieces, err := sk.IsExpired(ctx, rootAddr, skAddr, sessionkey.AddPiecesPermission())
		if err != nil {
			t.Fatalf("IsExpired(AddPieces): %v", err)
		}
		if !expiredAddPieces {
			t.Error("AddPieces should be expired after partial revoke")
		}
		exp, err := sk.AuthorizationExpiry(ctx, rootAddr, skAddr, sessionkey.CreateDataSetPermission())
		if err != nil {
			t.Fatalf("AuthorizationExpiry(CreateDataSet): %v", err)
		}
		if exp == 0 {
			t.Error("CreateDataSet should still be authorised after partial revoke")
		}
	})

	t.Run("LoginAndFund+LoginAndFundWithOptions", func(t *testing.T) {
		rpc, err := ethclient.DialContext(ctx, integrationtest.RPCURL())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(rpc.Close)
		chainID, err := rpc.ChainID(ctx)
		if err != nil || chainID.Cmp(big.NewInt(integrationtest.CalibrationChainID)) != 0 {
			t.Fatalf("session key RPC must be Calibration: chain=%v error=%v", chainID, err)
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
		writer, err := sessionkey.New(sessionkey.Options{
			Backend: rpc, Signer: &cachedTransactorSigner{EVMSigner: sg},
			ChainID:         types.ChainID(integrationtest.CalibrationChainID),
			RegistryAddress: sk.RegistryAddress(),
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, explicit := range []bool{false, true} {
			name := "DefaultOptions"
			if explicit {
				name = "ExplicitOptions"
			}
			t.Run(name, func(t *testing.T) {
				skPriv, err := ecdsa.GenerateKey(crypto.S256(), cryptorand.Reader)
				if err != nil {
					t.Fatalf("generate session key: %v", err)
				}
				skAddr := crypto.PubkeyToAddress(skPriv.PublicKey)
				perms := sessionkey.DefaultFWSSPermissions()
				origin := "synapse"
				if explicit {
					perms = []sessionkey.Permission{sessionkey.CreateDataSetPermission()}
					origin = "integration-test"
				}
				revoked := false
				verifyRevoked := func(checkCtx context.Context) error {
					for _, permission := range perms {
						expired, err := sk.IsExpired(checkCtx, rootAddr, skAddr, permission)
						if err != nil {
							return fmt.Errorf("IsExpired(%s): %w", permission.Hex(), err)
						}
						if !expired {
							return fmt.Errorf("permission %s is still authorized", permission.Hex())
						}
					}
					return nil
				}
				t.Cleanup(func() {
					if revoked {
						return
					}
					cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 3*time.Minute)
					defer cleanupCancel()
					res, err := sk.RevokeWithOptions(cleanupCtx, skAddr, &sessionkey.RevokeOptions{
						Permissions: perms, Origin: origin,
					}, sessionkey.WithWait(txWait))
					if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
						t.Errorf("cleanup RevokeWithOptions(session=%s): result=%v error=%v", skAddr, res, err)
						return
					}
					if err := verifyRevoked(cleanupCtx); err != nil {
						t.Errorf("cleanup authorization check(session=%s): %v", skAddr, err)
					}
				})

				one := big.NewInt(1)
				var res *types.WriteResult
				if explicit {
					res, err = writer.LoginAndFundWithOptions(ctx, skAddr, one, &sessionkey.LoginOptions{
						Permissions: perms,
						ExpiresAt:   uint64(time.Now().Add(10 * time.Minute).Unix()),
						Origin:      origin,
					}, sessionkey.WithWait(txWait))
				} else {
					res, err = writer.LoginAndFund(ctx, skAddr, one, sessionkey.WithWait(txWait))
				}
				if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
					t.Fatalf("funded login: result=%v error=%v", res, err)
				}
				t.Logf("funded login tx=%s", res.Hash)
				bal, err := client.Payments().WalletBalance(ctx, common.Address{}, skAddr)
				if err != nil {
					t.Fatalf("WalletBalance(session key): %v", err)
				}
				if bal == nil || bal.Cmp(one) != 0 {
					t.Fatalf("session key FIL balance = %v, want 1 attoFIL", bal)
				}
				for _, permission := range perms {
					expired, err := sk.IsExpired(ctx, rootAddr, skAddr, permission)
					if err != nil || expired {
						t.Fatalf("permission %s after login: expired=%v error=%v", permission.Hex(), expired, err)
					}
				}
				if explicit {
					exp, expiryErr := sk.AuthorizationExpiry(ctx, rootAddr, skAddr, sessionkey.AddPiecesPermission())
					if expiryErr != nil || exp != 0 {
						t.Fatalf("unrequested AddPieces expiry = %d error=%v, want 0", exp, expiryErr)
					}
					res, err = writer.RevokeWithOptions(ctx, skAddr, &sessionkey.RevokeOptions{
						Permissions: perms, Origin: origin,
					}, sessionkey.WithWait(txWait))
				} else {
					res, err = writer.Revoke(ctx, skAddr, sessionkey.WithWait(txWait))
				}
				if err != nil || res == nil || res.Receipt == nil || res.Receipt.Status != 1 {
					t.Fatalf("revoke after funded login: result=%v error=%v", res, err)
				}
				if err := verifyRevoked(ctx); err != nil {
					t.Fatal(err)
				}
				revoked = true
				t.Logf("revoke tx=%s", res.Hash)
			})
		}
	})
}

type cachedTransactorSigner struct {
	signer.EVMSigner
	opts *bind.TransactOpts
}

func (s *cachedTransactorSigner) Transactor(chainID *big.Int) (*bind.TransactOpts, error) {
	if s.opts == nil {
		opts, err := s.EVMSigner.Transactor(chainID)
		if err != nil {
			return nil, err
		}
		s.opts = opts
	}
	return s.opts, nil
}
