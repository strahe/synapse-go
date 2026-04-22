package payments

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/strahe/synapse-go/internal/typeddata"
	"github.com/strahe/synapse-go/signer"
	sdktypes "github.com/strahe/synapse-go/types"
)

// PermitDeadlineDuration is the default validity window appended to the
// current wall-clock time when the caller does not provide an explicit
// permit deadline. Mirrors synapse-core `PERMIT_DEADLINE_DURATION`
// (1 hour; utils/constants.ts:34).
const PermitDeadlineDuration = time.Hour

// permitERC20ABI is the subset of the ERC-2612 / OpenZeppelin ERC20Permit
// interface required to fetch the metadata needed to sign a permit:
// - name() -> string
// - version() -> string
// - nonces(owner) -> uint256
//
// Maintained as a parsed ABI literal so we do not have to regenerate the
// erc20 bindings just to expose three extra reads. USDFC and every other
// OpenZeppelin-derived ERC20Permit implementation expose these methods.
var permitERC20ABI = mustParsePermitABI()

func mustParsePermitABI() abi.ABI {
	const src = `[
{"type":"function","name":"name","inputs":[],"outputs":[{"name":"","type":"string"}],"stateMutability":"view"},
{"type":"function","name":"version","inputs":[],"outputs":[{"name":"","type":"string"}],"stateMutability":"view"},
{"type":"function","name":"nonces","inputs":[{"name":"owner","type":"address"}],"outputs":[{"name":"","type":"uint256"}],"stateMutability":"view"}
]`
	parsed, err := abi.JSON(strings.NewReader(src))
	if err != nil {
		panic(fmt.Sprintf("payments: parse permit ABI: %v", err))
	}
	return parsed
}

type permitInputs struct {
	Name    string
	Version string
	Nonce   *big.Int
}

// fetchPermitInputs queries `name()`, `version()`, and `nonces(owner)` on the
// token contract so the caller can assemble an ERC-2612 EIP-712 domain
// and message.
//
// Mirrors synapse-core/src/erc20/index.ts:127 (balanceForPermit) except it
// does not co-fetch the balance — callers that need a pre-check can use
// [Service.WalletBalance] instead.
func (s *Service) fetchPermitInputs(ctx context.Context, token, owner common.Address) (*permitInputs, error) {
	bound := bind.NewBoundContract(token, permitERC20ABI, s.backend, nil, nil)
	call := &bind.CallOpts{Context: ctx}

	var nameOut []interface{}
	if err := bound.Call(call, &nameOut, "name"); err != nil {
		return nil, fmt.Errorf("name(): %w", err)
	}
	name, ok := nameOut[0].(string)
	if !ok {
		return nil, fmt.Errorf("name(): unexpected type %T", nameOut[0])
	}

	var versionOut []interface{}
	if err := bound.Call(call, &versionOut, "version"); err != nil {
		return nil, fmt.Errorf("version(): %w", err)
	}
	version, ok := versionOut[0].(string)
	if !ok {
		return nil, fmt.Errorf("version(): unexpected type %T", versionOut[0])
	}

	var nonceOut []interface{}
	if err := bound.Call(call, &nonceOut, "nonces", owner); err != nil {
		return nil, fmt.Errorf("nonces(): %w", err)
	}
	nonce, ok := nonceOut[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("nonces(): unexpected type %T", nonceOut[0])
	}

	return &permitInputs{Name: name, Version: version, Nonce: nonce}, nil
}

// PermitOption is reserved for future nonce / salt overrides. Currently
// unused; keep the type to preserve API stability once options land.
type PermitOption func(*permitConfig)

type permitConfig struct{}

// DepositWithPermit deposits `amount` of `token` into the Filecoin Pay
// contract on behalf of the signer in a single on-chain transaction by
// attaching an ERC-2612 permit signature. No prior ERC20 approval is
// required.
//
// `to` is the credited depositor; when zero, defaults to the signer EOA.
// `deadline` is the permit expiry (unix seconds); when nil, now +
// [PermitDeadlineDuration] is used. `token` must be an ERC-2612 /
// ERC20Permit-compliant token (e.g. USDFC).
//
// Mirrors synapse-core/src/pay/deposit-with-permit.ts:78 (depositWithPermit).
func (s *Service) DepositWithPermit(
	ctx context.Context,
	token, to common.Address,
	amount, deadline *big.Int,
	opts ...WriteOption,
) (*sdktypes.WriteResult, error) {
	return s.depositWithPermit(ctx, token, to, amount, deadline, nil, nil, nil, nil, opts)
}

// DepositWithPermitAndApproveOperator combines DepositWithPermit with a
// SetOperatorApproval in the same transaction. Use this to onboard a new
// client: deposit + grant WarmStorage (or another operator) the allowances
// it needs in a single click.
//
// Mirrors synapse-core/src/pay/payments.ts:73 (depositAndApprove).
func (s *Service) DepositWithPermitAndApproveOperator(
	ctx context.Context,
	token, to common.Address,
	amount, deadline *big.Int,
	operator common.Address,
	rateAllowance, lockupAllowance, maxLockupPeriod *big.Int,
	opts ...WriteOption,
) (*sdktypes.WriteResult, error) {
	return s.depositWithPermit(
		ctx, token, to, amount, deadline,
		&operator, rateAllowance, lockupAllowance, maxLockupPeriod,
		opts,
	)
}

func (s *Service) depositWithPermit(
	ctx context.Context,
	token, to common.Address,
	amount, deadline *big.Int,
	operator *common.Address,
	rateAllowance, lockupAllowance, maxLockupPeriod *big.Int,
	writeOpts []WriteOption,
) (*sdktypes.WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	method := "DepositWithPermit"
	if operator != nil {
		method = "DepositWithPermitAndApproveOperator"
	}
	if err := s.requireSigner(method); err != nil {
		return nil, err
	}
	if (token == common.Address{}) {
		return nil, invalidZeroAddressError("payments."+method, "token")
	}
	if err := validatePositive("payments."+method+" amount", amount); err != nil {
		return nil, err
	}
	if operator != nil {
		if (*operator == common.Address{}) {
			return nil, invalidZeroAddressError("payments."+method, "operator")
		}
		for name, v := range map[string]*big.Int{
			"rateAllowance":   rateAllowance,
			"lockupAllowance": lockupAllowance,
			"maxLockupPeriod": maxLockupPeriod,
		} {
			if err := validateNonNegative("payments."+method+" "+name, v); err != nil {
				return nil, err
			}
		}
	}

	owner := s.signer.EVMAddress()
	recipient := to
	if (recipient == common.Address{}) {
		recipient = owner
	}

	// Resolve permit deadline: default now + PermitDeadlineDuration.
	if deadline == nil || deadline.Sign() <= 0 {
		deadline = big.NewInt(time.Now().Add(PermitDeadlineDuration).Unix())
	}

	cfg := newWriteConfig(writeOpts)
	if !cfg.skipPrecheck {
		bal, err := s.WalletBalance(ctx, token, owner)
		if err != nil {
			return nil, fmt.Errorf("payments.%s: precheck balance: %w", method, err)
		}
		if bal.Cmp(amount) < 0 {
			return nil, fmt.Errorf("payments.%s: %w (have %s, want %s)", method, ErrInsufficientBalance, bal, amount)
		}
	}

	inputs, err := s.fetchPermitInputs(ctx, token, owner)
	if err != nil {
		return nil, fmt.Errorf("payments.%s: fetch permit inputs: %w", method, err)
	}

	domain := typeddata.NewERC20PermitDomain(s.chainID.BigInt(), inputs.Name, inputs.Version, token)
	sig, err := typeddata.SignERC20Permit(
		func(h []byte) ([]byte, error) { return signer.SignHash(s.signer, h) },
		domain,
		owner,
		s.filPayAddr,
		amount,
		inputs.Nonce,
		deadline,
	)
	if err != nil {
		return nil, fmt.Errorf("payments.%s: sign permit: %w", method, err)
	}

	txOpts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("payments.%s: %w", method, err)
	}
	defer release()

	var tx *ethtypes.Transaction
	if operator == nil {
		tx, err = s.filPayWrite.DepositWithPermit(txOpts, token, recipient, amount, deadline, sig.V, sig.R, sig.S)
	} else {
		tx, err = s.filPayWrite.DepositWithPermitAndApproveOperator(
			txOpts, token, recipient, amount, deadline, sig.V, sig.R, sig.S,
			*operator, rateAllowance, lockupAllowance, maxLockupPeriod,
		)
	}
	release()
	if err != nil {
		return nil, fmt.Errorf("payments.%s: %w", method, err)
	}
	return s.finalize(ctx, tx, writeOpts)
}

func validatePositive(name string, v *big.Int) error {
	if v == nil {
		return fmt.Errorf("%s: %w: nil", name, ErrInvalidArgument)
	}
	if v.Sign() <= 0 {
		return fmt.Errorf("%s: %w: must be > 0", name, ErrInvalidArgument)
	}
	return nil
}
