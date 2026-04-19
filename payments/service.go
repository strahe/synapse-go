package payments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/contracts/erc20"
	"github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/signer"
)

// Backend is the minimal RPC surface used by the payments service. It is
// satisfied by *ethclient.Client. Tests can substitute a mock.
type Backend interface {
	bind.ContractBackend
	BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error)
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// Service provides read and write access to the Filecoin Pay contract
// plus convenience wrappers around ERC20 allowance management.
//
// It is safe for concurrent use. All state-changing calls return a
// WriteResult whose Receipt is only populated when WithWait is supplied.
type Service struct {
	backend     Backend
	chainID     *big.Int
	filPayAddr  common.Address
	filPayCall  *filpay.FilPayCaller
	filPayWrite *filpay.FilPayTransactor
	signer      signer.EVMSigner
	nonces      *txutil.NonceManager
	logger      *slog.Logger
	receiptWait time.Duration
}

// Options bundles the dependencies for constructing a Service.
type Options struct {
	// Backend is the Ethereum RPC client. Required.
	Backend Backend
	// ChainID of the target FEVM chain. Required.
	ChainID *big.Int
	// FilPayAddress is the Filecoin Pay contract address. Required.
	FilPayAddress common.Address
	// Signer is used to sign transactions. Required for write methods;
	// may be nil when the Service is used for reads only.
	Signer signer.EVMSigner
	// Logger is optional. When nil, logging is disabled.
	Logger *slog.Logger
	// NonceManager is optional. When nil, one is created from Backend.
	NonceManager *txutil.NonceManager
	// ReceiptWait overrides the default receipt polling timeout used by
	// WithConfirmations when the call waits for a receipt but does not
	// provide a more specific WithWait(timeout). Zero uses
	// txutil.DefaultReceiptWaitConfig.
	ReceiptWait time.Duration
}

// New constructs a Service.
func New(opts Options) (*Service, error) {
	if opts.Backend == nil {
		return nil, errors.New("payments.New: nil Backend")
	}
	if opts.ChainID == nil || opts.ChainID.Sign() <= 0 {
		return nil, errors.New("payments.New: invalid ChainID")
	}
	if (opts.FilPayAddress == common.Address{}) {
		return nil, errors.New("payments.New: zero FilPayAddress")
	}
	caller, err := filpay.NewFilPayCaller(opts.FilPayAddress, opts.Backend)
	if err != nil {
		return nil, fmt.Errorf("payments.New: bind caller: %w", err)
	}
	writer, err := filpay.NewFilPayTransactor(opts.FilPayAddress, opts.Backend)
	if err != nil {
		return nil, fmt.Errorf("payments.New: bind transactor: %w", err)
	}
	s := &Service{
		backend:     opts.Backend,
		chainID:     new(big.Int).Set(opts.ChainID),
		filPayAddr:  opts.FilPayAddress,
		filPayCall:  caller,
		filPayWrite: writer,
		signer:      opts.Signer,
		logger:      opts.Logger,
		nonces:      opts.NonceManager,
		receiptWait: opts.ReceiptWait,
	}
	if s.nonces == nil && s.signer != nil {
		s.nonces = txutil.NewNonceManager(opts.Backend, s.signer.EVMAddress())
	}
	return s, nil
}

// Address returns the FilPay contract address.
func (s *Service) Address() common.Address { return s.filPayAddr }

// ChainID returns the configured chain id (copy).
func (s *Service) ChainID() *big.Int { return new(big.Int).Set(s.chainID) }

// Account returns the EOA address used for writes, or the zero address
// when the service was constructed without a signer.
func (s *Service) Account() common.Address {
	if s.signer == nil {
		return common.Address{}
	}
	return s.signer.EVMAddress()
}

// ---------- reads ----------

// AccountInfo returns the on-chain account record for (token, owner).
//
// When token is ZeroAddress, native FIL is queried via BalanceAt and returned
// as AccountState.Funds. All lockup fields are zero because native FIL is not
// tracked by the FilPay contract.
func (s *Service) AccountInfo(ctx context.Context, token, owner common.Address) (*AccountState, error) {
	if (owner == common.Address{}) {
		return nil, fmt.Errorf("payments.AccountInfo: %w (owner)", ErrZeroAddress)
	}
	if (token == common.Address{}) {
		bal, err := s.backend.BalanceAt(ctx, owner, nil)
		if err != nil {
			return nil, fmt.Errorf("payments.AccountInfo (FIL): %w", err)
		}
		if bal == nil {
			bal = new(big.Int)
		}
		return &AccountState{
			Funds:               copyBig(bal),
			LockupCurrent:       big.NewInt(0),
			LockupRate:          big.NewInt(0),
			LockupLastSettledAt: big.NewInt(0),
			FundedUntilEpoch:    big.NewInt(0),
			availableFunds:      copyBig(bal),
		}, nil
	}
	v, err := s.filPayCall.Accounts(&bind.CallOpts{Context: ctx}, token, owner)
	if err != nil {
		return nil, fmt.Errorf("payments.AccountInfo: %w", err)
	}
	settled, err := s.filPayCall.GetAccountInfoIfSettled(&bind.CallOpts{Context: ctx}, token, owner)
	if err != nil {
		return nil, fmt.Errorf("payments.AccountInfo: getAccountInfoIfSettled: %w", err)
	}
	return &AccountState{
		Funds:               copyBig(v.Funds),
		LockupCurrent:       copyBig(v.LockupCurrent),
		LockupRate:          copyBig(v.LockupRate),
		LockupLastSettledAt: copyBig(v.LockupLastSettledAt),
		FundedUntilEpoch:    copyBig(settled.FundedUntilEpoch),
		availableFunds:      copyBig(settled.AvailableFunds),
	}, nil
}

// Balance is a convenience that returns the Funds field of AccountInfo.
func (s *Service) Balance(ctx context.Context, token, owner common.Address) (*big.Int, error) {
	info, err := s.AccountInfo(ctx, token, owner)
	if err != nil {
		return nil, err
	}
	return info.Funds, nil
}

// WalletBalance returns the EOA balance of token. When token is the zero
// address the native FIL balance is returned via BalanceAt. Otherwise the
// ERC20 balanceOf(account) is queried.
func (s *Service) WalletBalance(ctx context.Context, token, account common.Address) (*big.Int, error) {
	if (account == common.Address{}) {
		return nil, fmt.Errorf("payments.WalletBalance: %w (account)", ErrZeroAddress)
	}
	if (token == common.Address{}) {
		bal, err := s.backend.BalanceAt(ctx, account, nil)
		if err != nil {
			return nil, fmt.Errorf("payments.WalletBalance (FIL): %w", err)
		}
		return bal, nil
	}
	c, err := erc20.NewERC20Caller(token, s.backend)
	if err != nil {
		return nil, fmt.Errorf("payments.WalletBalance: bind token: %w", err)
	}
	bal, err := c.BalanceOf(&bind.CallOpts{Context: ctx}, account)
	if err != nil {
		return nil, fmt.Errorf("payments.WalletBalance: %w", err)
	}
	return bal, nil
}

// Allowance returns the ERC20 allowance of owner towards spender for token.
func (s *Service) Allowance(ctx context.Context, token, owner, spender common.Address) (*big.Int, error) {
	if (token == common.Address{}) {
		return nil, fmt.Errorf("payments.Allowance: %w (token)", ErrZeroAddress)
	}
	if (owner == common.Address{}) {
		return nil, fmt.Errorf("payments.Allowance: %w (owner)", ErrZeroAddress)
	}
	if (spender == common.Address{}) {
		return nil, fmt.Errorf("payments.Allowance: %w (spender)", ErrZeroAddress)
	}
	c, err := erc20.NewERC20Caller(token, s.backend)
	if err != nil {
		return nil, fmt.Errorf("payments.Allowance: bind token: %w", err)
	}
	allow, err := c.Allowance(&bind.CallOpts{Context: ctx}, owner, spender)
	if err != nil {
		return nil, fmt.Errorf("payments.Allowance: %w", err)
	}
	return allow, nil
}

// ServiceApproval returns the operator approval record for
// (token, client, operator).
func (s *Service) ServiceApproval(ctx context.Context, token, client, operator common.Address) (*OperatorApproval, error) {
	if (token == common.Address{}) {
		return nil, fmt.Errorf("payments.ServiceApproval: %w (token)", ErrZeroAddress)
	}
	if (client == common.Address{}) {
		return nil, fmt.Errorf("payments.ServiceApproval: %w (client)", ErrZeroAddress)
	}
	if (operator == common.Address{}) {
		return nil, fmt.Errorf("payments.ServiceApproval: %w (operator)", ErrZeroAddress)
	}
	v, err := s.filPayCall.OperatorApprovals(&bind.CallOpts{Context: ctx}, token, client, operator)
	if err != nil {
		return nil, fmt.Errorf("payments.ServiceApproval: %w", err)
	}
	return &OperatorApproval{
		IsApproved:      v.IsApproved,
		RateAllowance:   copyBig(v.RateAllowance),
		LockupAllowance: copyBig(v.LockupAllowance),
		RateUsage:       copyBig(v.RateUsage),
		LockupUsage:     copyBig(v.LockupUsage),
		MaxLockupPeriod: copyBig(v.MaxLockupPeriod),
	}, nil
}

// ---------- writes ----------

// Approve calls ERC20.approve(spender, amount) on the given token.
//
// spender is typically the FilPay contract address; use service.Address()
// for that convenience.
func (s *Service) Approve(ctx context.Context, token, spender common.Address, amount *big.Int, opts ...WriteOption) (*WriteResult, error) {
	if err := s.requireSigner("Approve"); err != nil {
		return nil, err
	}
	if (token == common.Address{}) {
		return nil, fmt.Errorf("payments.Approve: %w (token)", ErrZeroAddress)
	}
	if (spender == common.Address{}) {
		return nil, fmt.Errorf("payments.Approve: %w (spender)", ErrZeroAddress)
	}
	if err := validateNonNegative("payments.Approve amount", amount); err != nil {
		return nil, err
	}
	tw, err := erc20.NewERC20Transactor(token, s.backend)
	if err != nil {
		return nil, fmt.Errorf("payments.Approve: bind token: %w", err)
	}
	txOpts, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.Approve: %w", err)
	}
	tx, err := tw.Approve(txOpts, spender, amount)
	if err != nil {
		s.releaseNonce(txOpts.Nonce)
		return nil, fmt.Errorf("payments.Approve: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// Deposit calls FilPay.deposit(token, to, amount). The caller must have
// first approved at least `amount` on the token contract for FilPay.
// When `to` is the zero address the caller's EOA is used.
func (s *Service) Deposit(ctx context.Context, token, to common.Address, amount *big.Int, opts ...WriteOption) (*WriteResult, error) {
	if err := s.requireSigner("Deposit"); err != nil {
		return nil, err
	}
	if (token == common.Address{}) {
		return nil, fmt.Errorf("payments.Deposit: %w (token)", ErrZeroAddress)
	}
	if err := validateNonNegative("payments.Deposit amount", amount); err != nil {
		return nil, err
	}
	owner := s.signer.EVMAddress()
	recipient := to
	if (recipient == common.Address{}) {
		recipient = owner
	}

	cfg := newWriteConfig(opts)
	if !cfg.skipPrecheck {
		bal, err := s.WalletBalance(ctx, token, owner)
		if err != nil {
			return nil, fmt.Errorf("payments.Deposit: precheck balance: %w", err)
		}
		if bal.Cmp(amount) < 0 {
			return nil, fmt.Errorf("payments.Deposit: %w (have %s, want %s)", ErrInsufficientBalance, bal, amount)
		}
		allow, err := s.Allowance(ctx, token, owner, s.filPayAddr)
		if err != nil {
			return nil, fmt.Errorf("payments.Deposit: precheck allowance: %w", err)
		}
		if allow.Cmp(amount) < 0 {
			return nil, fmt.Errorf("payments.Deposit: %w (have %s, want %s)", ErrInsufficientAllowance, allow, amount)
		}
	}

	txOpts, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.Deposit: %w", err)
	}
	tx, err := s.filPayWrite.Deposit(txOpts, token, recipient, amount)
	if err != nil {
		s.releaseNonce(txOpts.Nonce)
		return nil, fmt.Errorf("payments.Deposit: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// Withdraw calls FilPay.withdraw(token, amount). The amount must not
// exceed AccountInfo.AvailableFunds (pre-check can be disabled via
// WithSkipPrecheck).
func (s *Service) Withdraw(ctx context.Context, token common.Address, amount *big.Int, opts ...WriteOption) (*WriteResult, error) {
	if err := s.requireSigner("Withdraw"); err != nil {
		return nil, err
	}
	if (token == common.Address{}) {
		return nil, fmt.Errorf("payments.Withdraw: %w (token)", ErrZeroAddress)
	}
	if err := validateNonNegative("payments.Withdraw amount", amount); err != nil {
		return nil, err
	}
	cfg := newWriteConfig(opts)
	if !cfg.skipPrecheck {
		info, err := s.AccountInfo(ctx, token, s.signer.EVMAddress())
		if err != nil {
			return nil, fmt.Errorf("payments.Withdraw: precheck: %w", err)
		}
		avail := info.AvailableFunds()
		if avail == nil || avail.Cmp(amount) < 0 {
			return nil, fmt.Errorf("payments.Withdraw: %w (available %s, want %s)", ErrInsufficientBalance, avail, amount)
		}
	}

	txOpts, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.Withdraw: %w", err)
	}
	tx, err := s.filPayWrite.Withdraw(txOpts, token, amount)
	if err != nil {
		s.releaseNonce(txOpts.Nonce)
		return nil, fmt.Errorf("payments.Withdraw: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// ApproveService calls FilPay.setOperatorApproval(token, operator, true,
// rateAllowance, lockupAllowance, maxLockupPeriod). Use RevokeService to
// clear the approval.
func (s *Service) ApproveService(ctx context.Context, token, operator common.Address, rateAllowance, lockupAllowance, maxLockupPeriod *big.Int, opts ...WriteOption) (*WriteResult, error) {
	if err := s.requireSigner("ApproveService"); err != nil {
		return nil, err
	}
	if (token == common.Address{}) {
		return nil, fmt.Errorf("payments.ApproveService: %w (token)", ErrZeroAddress)
	}
	if (operator == common.Address{}) {
		return nil, fmt.Errorf("payments.ApproveService: %w (operator)", ErrZeroAddress)
	}
	for name, v := range map[string]*big.Int{
		"rateAllowance":   rateAllowance,
		"lockupAllowance": lockupAllowance,
		"maxLockupPeriod": maxLockupPeriod,
	} {
		if err := validateNonNegative("payments.ApproveService "+name, v); err != nil {
			return nil, err
		}
	}

	txOpts, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.ApproveService: %w", err)
	}
	tx, err := s.filPayWrite.SetOperatorApproval(txOpts, token, operator, true, rateAllowance, lockupAllowance, maxLockupPeriod)
	if err != nil {
		s.releaseNonce(txOpts.Nonce)
		return nil, fmt.Errorf("payments.ApproveService: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// RevokeService clears a prior ApproveService by setting approved=false
// and all allowances to zero.
func (s *Service) RevokeService(ctx context.Context, token, operator common.Address, opts ...WriteOption) (*WriteResult, error) {
	if err := s.requireSigner("RevokeService"); err != nil {
		return nil, err
	}
	if (token == common.Address{}) {
		return nil, fmt.Errorf("payments.RevokeService: %w (token)", ErrZeroAddress)
	}
	if (operator == common.Address{}) {
		return nil, fmt.Errorf("payments.RevokeService: %w (operator)", ErrZeroAddress)
	}
	zero := big.NewInt(0)
	txOpts, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.RevokeService: %w", err)
	}
	tx, err := s.filPayWrite.SetOperatorApproval(txOpts, token, operator, false, zero, zero, zero)
	if err != nil {
		s.releaseNonce(txOpts.Nonce)
		return nil, fmt.Errorf("payments.RevokeService: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// ---------- internals ----------

func (s *Service) requireSigner(method string) error {
	if s.signer == nil {
		return fmt.Errorf("payments.%s: signer required for write methods", method)
	}
	return nil
}

// newTransactOpts builds a bind.TransactOpts with a nonce reserved from
// the NonceManager. Gas estimation and pricing are left to the backend;
// this helper does not override gas fields.
func (s *Service) newTransactOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := s.signer.Transactor(s.chainID)
	if err != nil {
		return nil, fmt.Errorf("transactor: %w", err)
	}
	opts.Context = ctx
	nonce, err := s.nonces.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	opts.Nonce = new(big.Int).SetUint64(nonce)
	return opts, nil
}

func (s *Service) releaseNonce(nonce *big.Int) {
	if nonce == nil || s.nonces == nil {
		return
	}
	s.nonces.MarkFailed(nonce.Uint64())
}

func (s *Service) finalize(ctx context.Context, tx *types.Transaction, opts []WriteOption) (*WriteResult, error) {
	cfg := newWriteConfig(opts)
	res := &WriteResult{Hash: tx.Hash()}
	if cfg.waitTimeout <= 0 {
		if s.nonces != nil {
			s.nonces.MarkConfirmed(tx.Nonce())
		}
		return res, nil
	}
	var (
		receipt *types.Receipt
		err     error
	)
	if cfg.confirmations > 0 {
		waitCfg := txutil.DefaultReceiptWaitConfig()
		if s.receiptWait > 0 {
			waitCfg.Timeout = s.receiptWait
		}
		if cfg.waitTimeout > 0 {
			waitCfg.Timeout = cfg.waitTimeout
		}
		receipt, err = txutil.WaitForReceiptWithConfig(ctx, s.backend, tx.Hash(), waitCfg, cfg.confirmations)
	} else {
		receipt, err = txutil.WaitForReceipt(ctx, s.backend, tx.Hash(), cfg.waitTimeout)
	}
	if err != nil {
		if errors.Is(err, txutil.ErrTxFailed) {
			if s.nonces != nil {
				s.nonces.MarkConfirmed(tx.Nonce())
			}
			res.Receipt = receipt
		}
		return res, fmt.Errorf("wait receipt: %w", err)
	}
	// Whether the tx reverted or succeeded, the nonce is consumed on-chain,
	// so mark it confirmed to release the reservation.
	if s.nonces != nil {
		s.nonces.MarkConfirmed(tx.Nonce())
	}
	res.Receipt = receipt
	return res, nil
}

func copyBig(v *big.Int) *big.Int {
	if v == nil {
		return nil
	}
	return new(big.Int).Set(v)
}

func validateNonNegative(name string, v *big.Int) error {
	if v == nil {
		return fmt.Errorf("%s: nil", name)
	}
	if v.Sign() < 0 {
		return fmt.Errorf("%s: negative", name)
	}
	return nil
}
