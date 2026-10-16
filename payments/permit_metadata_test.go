package payments

import (
	"context"
	"errors"
	"io"
	"math/big"
	"slices"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"
)

func TestFetchPermitInputs_Metadata(t *testing.T) {
	for _, tc := range []struct {
		name    string
		token   string
		version string
		nonce   int64
	}{
		{name: "populated", token: "USDFC", version: "1", nonce: 42},
		{name: "empty strings and zero nonce"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mb := newTestService(t)
			for method, value := range map[string]any{
				"name": tc.token, "version": tc.version, "nonces": big.NewInt(tc.nonce),
			} {
				data, err := permitERC20ABI.Methods[method].Outputs.Pack(value)
				if err != nil {
					t.Fatal(err)
				}
				mb.replies[tokenAddr.Hex()+":"+method] = data
			}
			got, err := s.fetchPermitInputs(context.Background(), tokenAddr, s.Account())
			if err != nil || got == nil {
				t.Fatalf("fetchPermitInputs = %v, %v", got, err)
			}
			if got.Name != tc.token || got.Version != tc.version || got.Nonce.Cmp(big.NewInt(tc.nonce)) != 0 {
				t.Fatalf("metadata = %+v, want name=%q version=%q nonce=%d", got, tc.token, tc.version, tc.nonce)
			}
		})
	}
}

func TestFetchPermitInputs_DecodeErrors(t *testing.T) {
	for _, method := range []string{"name", "version", "nonces"} {
		for _, tc := range []struct {
			name string
			raw  []byte
		}{
			{name: "empty"},
			{name: "malformed", raw: []byte{1}},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				s, mb := newTestService(t)
				seedPermitInputs(t, mb)
				mb.replies[tokenAddr.Hex()+":"+method] = tc.raw
				got, err := s.fetchPermitInputs(context.Background(), tokenAddr, s.Account())
				if got != nil || !errors.Is(err, ErrPermitUnsupported) {
					t.Fatalf("fetchPermitInputs = %v, %v, want nil and ErrPermitUnsupported", got, err)
				}
				_, decodeErr := permitERC20ABI.Unpack(method, tc.raw)
				if decodeErr == nil {
					t.Fatal("fixture must fail ABI decoding")
				}
				causes, ok := errors.AsType[interface {
					error
					Unwrap() []error
				}](err)
				if !ok || !slices.ContainsFunc(causes.Unwrap(), func(cause error) bool {
					return !errors.Is(cause, ErrPermitUnsupported) && cause.Error() == decodeErr.Error()
				}) {
					t.Fatalf("decode cause is missing from error chain: %v", err)
				}
			})
		}
	}
}

func TestFetchPermitInputs_CallErrors(t *testing.T) {
	for _, method := range []string{"name", "version", "nonces"} {
		for _, tc := range []struct {
			name  string
			cause error
		}{
			{name: "canceled", cause: context.Canceled},
			{name: "deadline", cause: context.DeadlineExceeded},
			{name: "transport", cause: io.EOF},
			{name: "RPC", cause: &rpc.HTTPError{StatusCode: 503}},
			{name: "revert", cause: &summaryRPCError{3, "execution reverted: version unavailable"}},
		} {
			t.Run(method+"/"+tc.name, func(t *testing.T) {
				s, mb := newTestService(t)
				seedPermitInputs(t, mb)
				mb.errs[tokenAddr.Hex()+":"+method] = tc.cause
				got, err := s.fetchPermitInputs(context.Background(), tokenAddr, s.Account())
				if got != nil || !errors.Is(err, tc.cause) || errors.Is(err, ErrPermitUnsupported) {
					t.Fatalf("fetchPermitInputs = %v, %v, want original cause without ErrPermitUnsupported", got, err)
				}
			})
		}
	}
}

type permitCodeBackend struct {
	*mockBackend
	codeErr error
}

func (b *permitCodeBackend) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return nil, b.codeErr
}

func TestFetchPermitInputs_CodeCheck(t *testing.T) {
	lookupErr := &rpc.HTTPError{StatusCode: 503}
	for _, tc := range []struct {
		name        string
		codeErr     error
		cause       error
		unsupported bool
	}{
		{name: "no code", cause: bind.ErrNoCode, unsupported: true},
		{name: "code lookup failed", codeErr: lookupErr, cause: lookupErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mb := newTestService(t)
			s.backend = &permitCodeBackend{mockBackend: mb, codeErr: tc.codeErr}
			got, err := s.fetchPermitInputs(context.Background(), tokenAddr, s.Account())
			if got != nil || !errors.Is(err, tc.cause) || errors.Is(err, ErrPermitUnsupported) != tc.unsupported {
				t.Fatalf("fetchPermitInputs = %v, %v, want cause=%v unsupported=%t", got, err, tc.cause, tc.unsupported)
			}
		})
	}
}

func TestDepositWithPermit_MetadataFailureStopsWriteAndReleasesLock(t *testing.T) {
	callErr := errors.New("permit metadata call failed")
	for _, tc := range []struct {
		name        string
		cause       error
		unsupported bool
	}{
		{name: "undecodable metadata", unsupported: true},
		{name: "call failure", cause: callErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sg := &validationSigner{EVMSigner: newTestSigner(t)}
			s, mb := newTestServiceWith(t, sg)
			seedPermitInputs(t, mb)
			mb.replies[tokenAddr.Hex()+":version"] = []byte{1}
			if tc.cause != nil {
				mb.errs[tokenAddr.Hex()+":version"] = tc.cause
			}
			nonces := &recordingNonceManager{}
			s.nonces = nonces
			result, err := s.DepositWithPermit(context.Background(), tokenAddr, common.Address{}, big.NewInt(1), nil, WithSkipPrecheck())
			if result != nil || err == nil || errors.Is(err, ErrPermitUnsupported) != tc.unsupported {
				t.Fatalf("DepositWithPermit = %v, %v, want nil result and unsupported=%t", result, err, tc.unsupported)
			}
			if tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Fatalf("DepositWithPermit error lost its cause: %v", err)
			}
			if sg.transactors != 0 || sg.signatures != 0 || sg.hashes != 0 || nonces.acquired != 0 || sentCount(mb) != 0 {
				t.Fatalf("metadata failure signed or sent a transaction: transactors=%d signatures=%d hashes=%d nonces=%d sent=%d",
					sg.transactors, sg.signatures, sg.hashes, nonces.acquired, sentCount(mb))
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			release, err := s.permits.acquire(ctx, permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()})
			if err != nil {
				t.Fatalf("permit lock was not released after metadata failure: %v", err)
			}
			release()
		})
	}
}
