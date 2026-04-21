package sessionkey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"

	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/contracts/sessionkeyregistry"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/signer"
	sdktypes "github.com/strahe/synapse-go/types"
)

// Backend is the minimal RPC surface used by the session key service. It is
// satisfied by *ethclient.Client. Tests can substitute a mock.
type Backend interface {
	bind.ContractBackend
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	BlockNumber(ctx context.Context) (uint64, error)
}

// Service provides session key management against the SessionKeyRegistry
// contract. It handles login (authorization), revocation, and expiry queries.
//
// It is safe for concurrent use. All state-changing calls return a
// [types.WriteResult] whose Receipt is only populated when WithWait is
// supplied.
type Service struct {
	backend      Backend
	chainID      *big.Int
	registryAddr common.Address
	registryCall *sessionkeyregistry.SessionKeyRegistryCaller
	registryTx   *sessionkeyregistry.SessionKeyRegistryTransactor
	signer       signer.EVMSigner
	nonces       *txutil.NonceManager
	logger       *slog.Logger
	receiptWait  time.Duration
}

// Options bundles the dependencies for constructing a Service.
type Options struct {
	// Backend is the Ethereum RPC client. Required.
	Backend Backend
	// ChainID of the target FEVM chain. Required.
	ChainID *big.Int
	// RegistryAddress is the SessionKeyRegistry contract address. Required.
	RegistryAddress common.Address
	// Signer is used to sign transactions. Required for write methods
	// (Login, Revoke); may be nil when the Service is used for reads only.
	Signer signer.EVMSigner
	// Logger is optional. When nil, logging is disabled.
	Logger *slog.Logger
	// NonceManager is optional. When nil, one is created from Backend.
	NonceManager *txutil.NonceManager
	// ReceiptWait overrides the default receipt polling timeout.
	ReceiptWait time.Duration
}

// New constructs a Service.
func New(opts Options) (*Service, error) {
	if opts.Backend == nil {
		return nil, errors.New("sessionkey.New: nil Backend")
	}
	if opts.ChainID == nil || opts.ChainID.Sign() <= 0 {
		return nil, errors.New("sessionkey.New: invalid ChainID")
	}
	if (opts.RegistryAddress == common.Address{}) {
		return nil, errors.New("sessionkey.New: zero RegistryAddress")
	}

	caller, err := sessionkeyregistry.NewSessionKeyRegistryCaller(opts.RegistryAddress, opts.Backend)
	if err != nil {
		return nil, fmt.Errorf("sessionkey.New: bind caller: %w", err)
	}
	writer, err := sessionkeyregistry.NewSessionKeyRegistryTransactor(opts.RegistryAddress, opts.Backend)
	if err != nil {
		return nil, fmt.Errorf("sessionkey.New: bind transactor: %w", err)
	}

	s := &Service{
		backend:      opts.Backend,
		chainID:      new(big.Int).Set(opts.ChainID),
		registryAddr: opts.RegistryAddress,
		registryCall: caller,
		registryTx:   writer,
		signer:       opts.Signer,
		logger:       opts.Logger,
		nonces:       opts.NonceManager,
		receiptWait:  opts.ReceiptWait,
	}
	if s.nonces == nil && s.signer != nil {
		s.nonces = txutil.NewNonceManager(opts.Backend, s.signer.EVMAddress())
	}
	return s, nil
}

// RegistryAddress returns the configured SessionKeyRegistry contract address.
func (s *Service) RegistryAddress() common.Address { return s.registryAddr }

// ---------- write operations ----------

// Login authorises the given session key address with default options
// (DefaultFWSSPermissions, 1 hour expiry, origin "synapse").
func (s *Service) Login(ctx context.Context, sessionKeyAddr common.Address, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	return s.LoginWithOptions(ctx, sessionKeyAddr, nil, opts...)
}

// LoginWithOptions authorises the given session key address with custom
// login options.
func (s *Service) LoginWithOptions(ctx context.Context, sessionKeyAddr common.Address, loginOpts *LoginOptions, writeOpts ...WriteOption) (*sdktypes.WriteResult, error) {
	if s.signer == nil {
		return nil, errors.New("sessionkey.Login: nil signer")
	}
	if (sessionKeyAddr == common.Address{}) {
		return nil, errors.New("sessionkey.Login: zero session key address")
	}

	lo := resolveLoginOptions(loginOpts)
	if lo.ExpiresAt <= uint64(time.Now().Unix()) {
		return nil, errors.New("sessionkey.Login: ExpiresAt must be in the future")
	}
	perms := dedup(lo.Permissions)

	txOpts, err := s.txOpts(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sessionkey.Login: %w", err)
	}

	tx, err := s.registryTx.Login(txOpts, sessionKeyAddr, new(big.Int).SetUint64(lo.ExpiresAt), perms, lo.Origin)
	if err != nil {
		if s.nonces != nil {
			s.nonces.MarkFailed(txOpts.Nonce.Uint64())
		}
		return nil, fmt.Errorf("sessionkey.Login: %w", err)
	}
	s.log("login tx broadcast", "hash", tx.Hash().Hex(), "sessionKey", sessionKeyAddr.Hex())
	return s.finalize(ctx, tx, writeOpts)
}

// LoginAndFund authorises a session key and transfers value (native FIL)
// in the same transaction using the payable loginAndFund method.
func (s *Service) LoginAndFund(ctx context.Context, sessionKeyAddr common.Address, value *big.Int, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	return s.LoginAndFundWithOptions(ctx, sessionKeyAddr, value, nil, opts...)
}

// LoginAndFundWithOptions is the full-option variant of LoginAndFund.
func (s *Service) LoginAndFundWithOptions(ctx context.Context, sessionKeyAddr common.Address, value *big.Int, loginOpts *LoginOptions, writeOpts ...WriteOption) (*sdktypes.WriteResult, error) {
	if s.signer == nil {
		return nil, errors.New("sessionkey.LoginAndFund: nil signer")
	}
	if (sessionKeyAddr == common.Address{}) {
		return nil, errors.New("sessionkey.LoginAndFund: zero session key address")
	}
	if value == nil || value.Sign() < 0 {
		return nil, errors.New("sessionkey.LoginAndFund: nil or negative value")
	}

	lo := resolveLoginOptions(loginOpts)
	if lo.ExpiresAt <= uint64(time.Now().Unix()) {
		return nil, errors.New("sessionkey.LoginAndFund: ExpiresAt must be in the future")
	}
	perms := dedup(lo.Permissions)

	txOpts, err := s.txOpts(ctx, value)
	if err != nil {
		return nil, fmt.Errorf("sessionkey.LoginAndFund: %w", err)
	}

	tx, err := s.registryTx.LoginAndFund(txOpts, sessionKeyAddr, new(big.Int).SetUint64(lo.ExpiresAt), perms, lo.Origin)
	if err != nil {
		if s.nonces != nil {
			s.nonces.MarkFailed(txOpts.Nonce.Uint64())
		}
		return nil, fmt.Errorf("sessionkey.LoginAndFund: %w", err)
	}
	s.log("loginAndFund tx broadcast", "hash", tx.Hash().Hex(), "sessionKey", sessionKeyAddr.Hex(), "value", value.String())
	return s.finalize(ctx, tx, writeOpts)
}

// Revoke revokes default FWSS permissions from a session key.
func (s *Service) Revoke(ctx context.Context, sessionKeyAddr common.Address, opts ...WriteOption) (*sdktypes.WriteResult, error) {
	return s.RevokeWithOptions(ctx, sessionKeyAddr, nil, opts...)
}

// RevokeWithOptions revokes specific permissions from a session key.
func (s *Service) RevokeWithOptions(ctx context.Context, sessionKeyAddr common.Address, revokeOpts *RevokeOptions, writeOpts ...WriteOption) (*sdktypes.WriteResult, error) {
	if s.signer == nil {
		return nil, errors.New("sessionkey.Revoke: nil signer")
	}
	if (sessionKeyAddr == common.Address{}) {
		return nil, errors.New("sessionkey.Revoke: zero session key address")
	}

	ro := resolveRevokeOptions(revokeOpts)
	perms := dedup(ro.Permissions)

	txOpts, err := s.txOpts(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sessionkey.Revoke: %w", err)
	}

	tx, err := s.registryTx.Revoke(txOpts, sessionKeyAddr, perms, ro.Origin)
	if err != nil {
		if s.nonces != nil {
			s.nonces.MarkFailed(txOpts.Nonce.Uint64())
		}
		return nil, fmt.Errorf("sessionkey.Revoke: %w", err)
	}
	s.log("revoke tx broadcast", "hash", tx.Hash().Hex(), "sessionKey", sessionKeyAddr.Hex())
	return s.finalize(ctx, tx, writeOpts)
}

// ---------- read operations ----------

// AuthorizationExpiry returns the Unix timestamp at which a specific
// permission for a session key expires. Returns 0 if not authorised.
func (s *Service) AuthorizationExpiry(ctx context.Context, rootAddr, sessionKeyAddr common.Address, permission Permission) (uint64, error) {
	raw, err := s.registryCall.AuthorizationExpiry(&bind.CallOpts{Context: ctx}, rootAddr, sessionKeyAddr, permission)
	if err != nil {
		return 0, fmt.Errorf("sessionkey.AuthorizationExpiry: %w", err)
	}
	if !raw.IsUint64() {
		return 0, fmt.Errorf("sessionkey.AuthorizationExpiry: expiry overflows uint64: %s", raw.String())
	}
	return raw.Uint64(), nil
}

// IsExpired returns true when the given permission for a session key has
// expired (i.e., the on-chain expiry is in the past).
func (s *Service) IsExpired(ctx context.Context, rootAddr, sessionKeyAddr common.Address, permission Permission) (bool, error) {
	exp, err := s.AuthorizationExpiry(ctx, rootAddr, sessionKeyAddr, permission)
	if err != nil {
		return false, err
	}
	return exp <= uint64(time.Now().Unix()), nil
}

// GetExpirations queries the on-chain authorization expiry for each of the
// given permissions in a single Multicall3 batch. Falls back to sequential
// reads if the batch call fails.
func (s *Service) GetExpirations(ctx context.Context, rootAddr, sessionKeyAddr common.Address, permissions []Permission) (Expirations, error) {
	if len(permissions) == 0 {
		permissions = DefaultFWSSPermissions
	}

	result := make(Expirations, len(permissions))
	for _, p := range permissions {
		result[p] = 0
	}

	batchResult, err := s.getExpirationsBatch(ctx, rootAddr, sessionKeyAddr, permissions, result)
	if err == nil {
		return batchResult, nil
	}
	s.logWarn("multicall batch failed, falling back to sequential", "err", err)
	return s.getExpirationsSequential(ctx, rootAddr, sessionKeyAddr, permissions, result)
}

func (s *Service) getExpirationsBatch(ctx context.Context, rootAddr, sessionKeyAddr common.Address, permissions []Permission, result Expirations) (Expirations, error) {
	regABI, err := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("sessionkey.GetExpirations: parse ABI: %w", err)
	}

	calls := make([]iabi.Call3, len(permissions))
	for i, perm := range permissions {
		data, err := regABI.Pack("authorizationExpiry", rootAddr, sessionKeyAddr, perm)
		if err != nil {
			return nil, fmt.Errorf("sessionkey.GetExpirations: pack call %d: %w", i, err)
		}
		calls[i] = iabi.Call3{
			Target:       s.registryAddr,
			AllowFailure: true,
			CallData:     data,
		}
	}

	results, err := iabi.BatchCall(ctx, s.backend, calls)
	if err != nil {
		return nil, fmt.Errorf("sessionkey.GetExpirations: batch call: %w", err)
	}

	uint256Type, err := abi.NewType("uint256", "", nil)
	if err != nil {
		return nil, fmt.Errorf("sessionkey.GetExpirations: build uint256 type: %w", err)
	}
	args := abi.Arguments{{Type: uint256Type}}
	for i, r := range results {
		if !r.Success || len(r.ReturnData) == 0 {
			continue
		}
		vals, err := args.Unpack(r.ReturnData)
		if err != nil {
			return nil, fmt.Errorf("sessionkey.GetExpirations: unpack sub-call %d: %w", i, err)
		}
		raw, ok := vals[0].(*big.Int)
		if !ok || !raw.IsUint64() {
			return nil, fmt.Errorf("sessionkey.GetExpirations: sub-call %d: value out of uint64 range", i)
		}
		result[permissions[i]] = raw.Uint64()
	}
	return result, nil
}

func (s *Service) getExpirationsSequential(ctx context.Context, rootAddr, sessionKeyAddr common.Address, permissions []Permission, result Expirations) (Expirations, error) {
	for _, p := range permissions {
		exp, err := s.AuthorizationExpiry(ctx, rootAddr, sessionKeyAddr, p)
		if err != nil {
			s.logWarn("sequential expiry lookup failed", "permission", p, "err", err)
			continue
		}
		result[p] = exp
	}
	return result, nil
}

// ---------- internal helpers ----------

func (s *Service) txOpts(ctx context.Context, value *big.Int) (*bind.TransactOpts, error) {
	txOpts, err := s.signer.Transactor(s.chainID)
	if err != nil {
		return nil, fmt.Errorf("transactor: %w", err)
	}
	txOpts.Context = ctx
	if value != nil {
		txOpts.Value = value
	}
	if s.nonces != nil {
		nonce, nErr := s.nonces.Get(ctx)
		if nErr != nil {
			return nil, fmt.Errorf("nonce: %w", nErr)
		}
		txOpts.Nonce = new(big.Int).SetUint64(nonce)
	}
	return txOpts, nil
}

func (s *Service) finalize(ctx context.Context, tx *types.Transaction, opts []WriteOption) (*sdktypes.WriteResult, error) {
	cfg := newWriteConfig(opts)
	res := &sdktypes.WriteResult{Hash: tx.Hash()}

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
		waitCfg.Timeout = cfg.waitTimeout
		receipt, err = txutil.WaitForReceiptWithConfig(ctx, s.backend, tx.Hash(), waitCfg, cfg.confirmations)
	} else {
		receipt, err = txutil.WaitForReceipt(ctx, s.backend, tx.Hash(), cfg.waitTimeout)
	}
	if err != nil {
		if s.nonces != nil {
			s.nonces.MarkConfirmed(tx.Nonce())
		}
		if errors.Is(err, txutil.ErrTxFailed) {
			res.Receipt = receipt
		}
		return res, fmt.Errorf("sessionkey: wait receipt: %w", err)
	}
	if s.nonces != nil {
		s.nonces.MarkConfirmed(tx.Nonce())
	}
	res.Receipt = receipt
	return res, nil
}

func (s *Service) log(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Info(msg, args...)
	}
}

func (s *Service) logWarn(msg string, args ...any) {
	if s.logger != nil {
		s.logger.Warn(msg, args...)
	}
}

func resolveLoginOptions(opts *LoginOptions) LoginOptions {
	var lo LoginOptions
	if opts != nil {
		lo = *opts
	}
	if len(lo.Permissions) == 0 {
		lo.Permissions = DefaultFWSSPermissions
	}
	if lo.ExpiresAt == 0 {
		lo.ExpiresAt = uint64(time.Now().Unix()) + 3600
	}
	if lo.Origin == "" {
		lo.Origin = "synapse"
	}
	return lo
}

func resolveRevokeOptions(opts *RevokeOptions) RevokeOptions {
	var ro RevokeOptions
	if opts != nil {
		ro = *opts
	}
	if len(ro.Permissions) == 0 {
		ro.Permissions = DefaultFWSSPermissions
	}
	if ro.Origin == "" {
		ro.Origin = "synapse"
	}
	return ro
}

// dedup removes duplicate permissions while preserving order.
func dedup(perms []Permission) [][32]byte {
	seen := make(map[Permission]struct{}, len(perms))
	out := make([][32]byte, 0, len(perms))
	for _, p := range perms {
		if _, exists := seen[p]; exists {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, [32]byte(p))
	}
	return out
}
