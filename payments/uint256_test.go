package payments

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/signer"
	sdktypes "github.com/strahe/synapse-go/types"
)

type validationSigner struct {
	signer.EVMSigner
	transactors int
	signatures  int
	hashes      int
}

func (s *validationSigner) Transactor(chainID *big.Int) (*bind.TransactOpts, error) {
	s.transactors++
	opts, err := s.EVMSigner.Transactor(chainID)
	if err != nil {
		return nil, err
	}
	sign := opts.Signer
	opts.Signer = func(address common.Address, tx *types.Transaction) (*types.Transaction, error) {
		s.signatures++
		return sign(address, tx)
	}
	return opts, nil
}

func (s *validationSigner) SignHash(hash []byte) ([]byte, error) {
	s.hashes++
	return signer.SignHash(s.EVMSigner, hash)
}

func TestPermitRejectsOtherRecipientsBeforeSigning(t *testing.T) {
	for _, combined := range []bool{false, true} {
		for _, skipPrecheck := range []bool{false, true} {
			name := "deposit"
			if combined {
				name = "deposit and approve"
			}
			if skipPrecheck {
				name += "/skip precheck"
			}
			t.Run(name, func(t *testing.T) {
				sg := &validationSigner{EVMSigner: newTestSigner(t)}
				s, backend := newTestServiceWith(t, sg)
				nonces := &recordingNonceManager{}
				s.nonces = nonces
				release, err := s.permits.acquire(context.Background(), permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()})
				if err != nil {
					t.Fatal(err)
				}
				defer release()
				ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
				defer cancel()
				var opts []WriteOption
				if skipPrecheck {
					opts = append(opts, WithSkipPrecheck())
				}
				var result *sdktypes.WriteResult
				if combined {
					result, err = s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, otherAddr, big.NewInt(1), nil,
						operatorAddr, new(big.Int), new(big.Int), new(big.Int), opts...)
				} else {
					result, err = s.DepositWithPermit(ctx, tokenAddr, otherAddr, big.NewInt(1), nil, opts...)
				}
				if result != nil || !errors.Is(err, ErrInvalidArgument) {
					t.Fatalf("result=%v error=%v, want invalid recipient before waiting for the permit lock", result, err)
				}
				if nonces.acquired != 0 || sg.transactors != 0 || sg.signatures != 0 || sg.hashes != 0 || len(backend.sent) != 0 || len(backend.lastIn) != 0 {
					t.Fatal("invalid recipient reached contract reads, signing, nonce acquisition, or broadcasting")
				}
			})
		}
	}
}

func TestWritesRejectUint256OverflowBeforeSigning(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		field    string
		precheck bool
		call     func(*Service, *big.Int, ...WriteOption) (*sdktypes.WriteResult, error)
	}{
		{name: "Approve", field: "amount", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.Approve(ctx, tokenAddr, filPayAddr, v, opts...)
		}},
		{name: "Deposit", field: "amount", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.Deposit(ctx, tokenAddr, otherAddr, v, opts...)
		}},
		{name: "Withdraw", field: "amount", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.Withdraw(ctx, tokenAddr, v, opts...)
		}},
		{name: "ApproveService rate", field: "rateAllowance", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.ApproveService(ctx, tokenAddr, operatorAddr, v, big.NewInt(1), big.NewInt(1), opts...)
		}},
		{name: "ApproveService lockup", field: "lockupAllowance", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.ApproveService(ctx, tokenAddr, operatorAddr, big.NewInt(1), v, big.NewInt(1), opts...)
		}},
		{name: "ApproveService period", field: "maxLockupPeriod", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.ApproveService(ctx, tokenAddr, operatorAddr, big.NewInt(1), big.NewInt(1), v, opts...)
		}},
		{name: "Permit amount", field: "amount", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermit(ctx, tokenAddr, s.Account(), v, nil, opts...)
		}},
		{name: "Permit deadline", field: "deadline", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermit(ctx, tokenAddr, s.Account(), big.NewInt(1), v, opts...)
		}},
		{name: "Combined permit amount", field: "amount", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, s.Account(), v, nil, operatorAddr, big.NewInt(1), big.NewInt(1), big.NewInt(1), opts...)
		}},
		{name: "Combined permit deadline", field: "deadline", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, s.Account(), big.NewInt(1), v, operatorAddr, big.NewInt(1), big.NewInt(1), big.NewInt(1), opts...)
		}},
		{name: "Combined permit rate", field: "rateAllowance", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, s.Account(), big.NewInt(1), nil, operatorAddr, v, big.NewInt(1), big.NewInt(1), opts...)
		}},
		{name: "Combined permit lockup", field: "lockupAllowance", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, s.Account(), big.NewInt(1), nil, operatorAddr, big.NewInt(1), v, big.NewInt(1), opts...)
		}},
		{name: "Combined permit period", field: "maxLockupPeriod", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, s.Account(), big.NewInt(1), nil, operatorAddr, big.NewInt(1), big.NewInt(1), v, opts...)
		}},
		{name: "Fund amount", field: "amount", precheck: true, call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.Fund(ctx, v, opts...)
		}},
		{name: "FundSync amount", field: "amount", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.FundSync(ctx, v, opts...)
		}},
		{name: "Fund period", field: "approval lockup period", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			opts = append(opts, WithFundNeedsFwssApproval(true), WithFundApprovalLockupPeriod(v))
			return s.Fund(ctx, new(big.Int), opts...)
		}},
		{name: "FundSync period", field: "approval lockup period", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			opts = append(opts, WithFundNeedsFwssApproval(true), WithFundApprovalLockupPeriod(v))
			return s.FundSync(ctx, new(big.Int), opts...)
		}},
		{name: "Settle", field: "untilEpoch", call: func(s *Service, v *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
			return s.Settle(ctx, sdktypes.NewBigInt(1), v, opts...)
		}},
	}
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	for _, tt := range tests {
		for _, delta := range []int64{0, 7} {
			checks := []bool{false}
			if tt.precheck {
				checks = append(checks, true)
			}
			for _, skip := range checks {
				name := tt.name + "/overflow+" + big.NewInt(delta).String()
				if skip {
					name += "/skip precheck"
				}
				t.Run(name, func(t *testing.T) {
					sg := &validationSigner{EVMSigner: newTestSigner(t)}
					s, backend := newTestServiceWith(t, sg)
					nonces := &recordingNonceManager{}
					s.nonces = nonces
					s.warmStorage, s.usdfcToken = operatorAddr, tokenAddr
					var opts []WriteOption
					if skip {
						opts = append(opts, WithSkipPrecheck())
					}
					res, err := tt.call(s, new(big.Int).Add(limit, big.NewInt(delta)), opts...)
					if res != nil || !errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), tt.field) {
						t.Fatalf("result=%v error=%v, want nil result and invalid %s", res, err, tt.field)
					}
					if nonces.acquired != 0 || sg.transactors != 0 || sg.signatures != 0 || sg.hashes != 0 || len(backend.sent) != 0 {
						t.Fatalf("nonce=%d transactors=%d signatures=%d hashes=%d sent=%d", nonces.acquired, sg.transactors, sg.signatures, sg.hashes, len(backend.sent))
					}
					if len(backend.lastIn) != 0 {
						t.Fatalf("contract reads before rejection: %v", backend.lastIn)
					}
				})
			}
		}
	}
}

func TestWritesPreserveMaximumUint256Calldata(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name    string
		method  string
		indices []int
		call    func(*Service, *big.Int) (*sdktypes.WriteResult, error)
	}{
		{name: "Approve", method: "approve", indices: []int{1}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.Approve(ctx, tokenAddr, filPayAddr, v)
		}},
		{name: "Deposit", method: "deposit", indices: []int{2}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.Deposit(ctx, tokenAddr, otherAddr, v, WithSkipPrecheck())
		}},
		{name: "Withdraw", method: "withdraw", indices: []int{1}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.Withdraw(ctx, tokenAddr, v, WithSkipPrecheck())
		}},
		{name: "ApproveService", method: "setOperatorApproval", indices: []int{3, 4, 5}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.ApproveService(ctx, tokenAddr, operatorAddr, v, v, v)
		}},
		{name: "Permit", method: "depositWithPermit", indices: []int{2, 3}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermit(ctx, tokenAddr, s.Account(), v, v, WithSkipPrecheck(), WithWait(time.Second))
		}},
		{name: "Combined permit", method: "depositWithPermitAndApproveOperator", indices: []int{2, 3, 8, 9, 10}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.DepositWithPermitAndApproveOperator(ctx, tokenAddr, s.Account(), v, v, operatorAddr, v, v, v, WithSkipPrecheck(), WithWait(time.Second))
		}},
		{name: "Fund approval only", method: "setOperatorApproval", indices: []int{3, 4, 5}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.Fund(ctx, new(big.Int), WithFundNeedsFwssApproval(true), WithFundApprovalLockupPeriod(v))
		}},
		{name: "Fund combined", method: "depositWithPermitAndApproveOperator", indices: []int{2, 8, 9, 10}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.Fund(ctx, v, WithFundNeedsFwssApproval(true), WithFundApprovalLockupPeriod(v), WithSkipPrecheck(), WithWait(time.Second))
		}},
		{name: "Settle", method: "settleRail", indices: []int{1}, call: func(s *Service, v *big.Int) (*sdktypes.WriteResult, error) {
			return s.Settle(ctx, sdktypes.NewBigInt(1), v)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, backend := newTestService(t)
			s.warmStorage, s.usdfcToken = operatorAddr, tokenAddr
			seedPermitInputs(t, backend)
			backend.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
				return &types.Receipt{Status: types.ReceiptStatusSuccessful}, nil
			}
			maximum := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
			before := new(big.Int).Set(maximum)
			res, err := tt.call(s, maximum)
			if err != nil || res == nil || len(backend.sent) != 1 {
				t.Fatalf("result=%v error=%v sent=%d", res, err, len(backend.sent))
			}
			method := backend.filPayABI.Methods[tt.method]
			if tt.method == "approve" {
				method = backend.erc20ABI.Methods[tt.method]
			}
			args, err := method.Inputs.Unpack(backend.sent[0].Data()[4:])
			if err != nil {
				t.Fatal(err)
			}
			for _, index := range tt.indices {
				if got := args[index].(*big.Int); got.Cmp(before) != 0 {
					t.Errorf("argument %d=%s, want %s", index, got, before)
				}
			}
			if maximum.Cmp(before) != 0 {
				t.Fatal("input amount changed")
			}
		})
	}
}

func TestGetSettlementAmountsRejectsUint256OverflowBeforeCall(t *testing.T) {
	s, backend := newTestService(t)
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	res, err := s.GetSettlementAmounts(context.Background(), sdktypes.NewBigInt(1), limit)
	if res != nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("result=%v error=%v, want invalid untilEpoch", res, err)
	}
	if len(backend.lastIn) != 0 {
		t.Fatalf("contract reads before rejection: %v", backend.lastIn)
	}
}

func TestFundRejectsReaderUint256OverflowBeforeSigning(t *testing.T) {
	sg := &validationSigner{EVMSigner: newTestSigner(t)}
	s, backend := newTestServiceWith(t, sg)
	s.warmStorage, s.usdfcToken = operatorAddr, tokenAddr
	s.lockups = &fakeApprovalLockupReader{period: new(big.Int).Lsh(big.NewInt(1), 256)}
	nonces := &recordingNonceManager{}
	s.nonces = nonces
	res, err := s.Fund(context.Background(), new(big.Int), WithFundNeedsFwssApproval(true))
	if res != nil || err == nil || errors.Is(err, ErrInvalidArgument) || !strings.Contains(err.Error(), "approval lockup period reader") {
		t.Fatalf("result=%v error=%v, want invalid reader result", res, err)
	}
	if nonces.acquired != 0 || sg.transactors != 0 || sg.signatures != 0 || sg.hashes != 0 || len(backend.sent) != 0 {
		t.Fatalf("nonce=%d transactors=%d signatures=%d hashes=%d sent=%d", nonces.acquired, sg.transactors, sg.signatures, sg.hashes, len(backend.sent))
	}
}

func TestPermitRejectsOverflowBeforeWaitingForLock(t *testing.T) {
	s, _ := newTestService(t)
	release, err := s.permits.acquire(context.Background(), permitKey{chainID: s.chainID, token: tokenAddr, owner: s.Account()})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	limit := new(big.Int).Lsh(big.NewInt(1), 256)
	for _, tt := range []struct {
		name             string
		amount, deadline *big.Int
	}{
		{name: "amount", amount: limit},
		{name: "deadline", amount: big.NewInt(1), deadline: limit},
	} {
		t.Run(tt.name, func(t *testing.T) {
			res, err := s.DepositWithPermit(ctx, tokenAddr, s.Account(), tt.amount, tt.deadline)
			if res != nil || !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("result=%v error=%v, want invalid argument before permit-lock cancellation", res, err)
			}
		})
	}
}

func TestPermitNonpositiveDeadlineUsesDefault(t *testing.T) {
	for _, tt := range []struct {
		name     string
		deadline *big.Int
	}{
		{name: "nil"},
		{name: "zero", deadline: new(big.Int)},
		{name: "negative", deadline: big.NewInt(-1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, backend := newTestService(t)
			seedPermitInputs(t, backend)
			backend.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
				return &types.Receipt{Status: types.ReceiptStatusSuccessful}, nil
			}
			before := time.Now().Add(PermitDeadlineDuration).Unix()
			if _, err := s.DepositWithPermit(context.Background(), tokenAddr, s.Account(), big.NewInt(1), tt.deadline, WithSkipPrecheck(), WithWait(time.Second)); err != nil {
				t.Fatal(err)
			}
			after := time.Now().Add(PermitDeadlineDuration).Unix()
			deadline := permitDeadlineFromSent(t, backend, 0)
			if !deadline.IsInt64() || deadline.Int64() < before || deadline.Int64() > after {
				t.Fatalf("deadline=%s, want between %d and %d", deadline, before, after)
			}
		})
	}
}
