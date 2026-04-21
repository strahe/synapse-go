package sessionkey

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestErrInvalidArgument_Detection exercises every entry point that returns
// ErrInvalidArgument and verifies errors.Is detects the sentinel.
func TestErrInvalidArgument_Detection(t *testing.T) {
	t.Run("New: nil Backend", func(t *testing.T) {
		_, err := New(Options{ChainID: testChainID, RegistryAddress: testRegistryAddr})
		requireInvalidArgument(t, err)
	})
	t.Run("New: invalid ChainID", func(t *testing.T) {
		_, err := New(Options{Backend: newMockBackend(t), RegistryAddress: testRegistryAddr})
		requireInvalidArgument(t, err)
	})
	t.Run("New: zero RegistryAddress", func(t *testing.T) {
		_, err := New(Options{Backend: newMockBackend(t), ChainID: testChainID})
		requireInvalidArgument(t, err)
	})

	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)
	ctx := context.Background()

	t.Run("Login: zero session key address", func(t *testing.T) {
		_, err := svc.Login(ctx, common.Address{})
		requireInvalidArgument(t, err)
	})
	t.Run("Login: past ExpiresAt", func(t *testing.T) {
		_, err := svc.LoginWithOptions(ctx, common.HexToAddress("0xBEEF"),
			&LoginOptions{ExpiresAt: 1})
		requireInvalidArgument(t, err)
	})
	t.Run("LoginAndFund: zero session key address", func(t *testing.T) {
		_, err := svc.LoginAndFund(ctx, common.Address{}, big.NewInt(1))
		requireInvalidArgument(t, err)
	})
	t.Run("LoginAndFund: nil value", func(t *testing.T) {
		_, err := svc.LoginAndFund(ctx, common.HexToAddress("0xBEEF"), nil)
		requireInvalidArgument(t, err)
	})
	t.Run("LoginAndFund: negative value", func(t *testing.T) {
		_, err := svc.LoginAndFund(ctx, common.HexToAddress("0xBEEF"), big.NewInt(-1))
		requireInvalidArgument(t, err)
	})
	t.Run("LoginAndFund: past ExpiresAt", func(t *testing.T) {
		_, err := svc.LoginAndFundWithOptions(ctx, common.HexToAddress("0xBEEF"),
			big.NewInt(0), &LoginOptions{ExpiresAt: 1})
		requireInvalidArgument(t, err)
	})
	t.Run("Revoke: zero session key address", func(t *testing.T) {
		_, err := svc.Revoke(ctx, common.Address{})
		requireInvalidArgument(t, err)
	})

	// Nil-signer paths (construct service without signer).
	readOnly, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("Login: nil signer", func(t *testing.T) {
		_, err := readOnly.Login(ctx, common.HexToAddress("0xBEEF"))
		requireInvalidArgument(t, err)
	})
	t.Run("LoginAndFund: nil signer", func(t *testing.T) {
		_, err := readOnly.LoginAndFund(ctx, common.HexToAddress("0xBEEF"), big.NewInt(1))
		requireInvalidArgument(t, err)
	})
	t.Run("Revoke: nil signer", func(t *testing.T) {
		_, err := readOnly.Revoke(ctx, common.HexToAddress("0xBEEF"))
		requireInvalidArgument(t, err)
	})
}

// TestErrInvalidArgument_NegativeMatch verifies ErrInvalidArgument does NOT
// match unrelated sentinels.
func TestErrInvalidArgument_NegativeMatch(t *testing.T) {
	if errors.Is(ErrTxFailed, ErrInvalidArgument) {
		t.Fatal("ErrTxFailed must not match ErrInvalidArgument")
	}
	if errors.Is(errors.New("unrelated"), ErrInvalidArgument) {
		t.Fatal("unrelated error must not match ErrInvalidArgument")
	}
}

func requireInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("errors.Is(err, ErrInvalidArgument)=false; err=%v", err)
	}
}
