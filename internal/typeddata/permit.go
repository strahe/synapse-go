package typeddata

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// permitTypes is the EIP-712 type schema for ERC-2612 Permit signatures.
// Maintained separately from the global FWSS Types because the token domain
// (name/version/verifyingContract) is per-token and the only primary type
// used is Permit.
//
// Mirrors synapse-core/src/typed-data/type-definitions.ts (EIP712Types).
var permitTypes = apitypes.Types{
	"EIP712Domain": {
		{Name: "name", Type: "string"},
		{Name: "version", Type: "string"},
		{Name: "chainId", Type: "uint256"},
		{Name: "verifyingContract", Type: "address"},
	},
	"Permit": {
		{Name: "owner", Type: "address"},
		{Name: "spender", Type: "address"},
		{Name: "value", Type: "uint256"},
		{Name: "nonce", Type: "uint256"},
		{Name: "deadline", Type: "uint256"},
	},
}

// NewERC20PermitDomain builds an ERC-2612 EIP-712 domain for the given
// token. The name and version must be fetched from the token itself at
// runtime (see payments.fetchPermitInputs); the chainID and token address
// complete the domain.
func NewERC20PermitDomain(chainID *big.Int, tokenName, tokenVersion string, token common.Address) apitypes.TypedDataDomain {
	return apitypes.TypedDataDomain{
		Name:              tokenName,
		Version:           tokenVersion,
		ChainId:           (*math.HexOrDecimal256)(chainID),
		VerifyingContract: token.Hex(),
	}
}

// SignERC20Permit signs an ERC-2612 Permit message and returns v/r/s suitable
// for direct use with Solidity `permit(owner, spender, value, deadline, v, r, s)`
// or Filecoin Pay `depositWithPermit(token, to, amount, deadline, v, r, s)`.
//
// Ports synapse-core/src/typed-data/sign-erc20-permit.ts.
func SignERC20Permit(
	signHash func([]byte) ([]byte, error),
	domain apitypes.TypedDataDomain,
	owner, spender common.Address,
	value, nonce, deadline *big.Int,
) (*Signature, error) {
	if value == nil {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: nil value")
	}
	if nonce == nil {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: nil nonce")
	}
	if deadline == nil {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: nil deadline")
	}

	message := apitypes.TypedDataMessage{
		"owner":    owner.Hex(),
		"spender":  spender.Hex(),
		"value":    (*math.HexOrDecimal256)(value),
		"nonce":    (*math.HexOrDecimal256)(nonce),
		"deadline": (*math.HexOrDecimal256)(deadline),
	}

	typedData := apitypes.TypedData{
		Types:       permitTypes,
		PrimaryType: "Permit",
		Domain:      domain,
		Message:     message,
	}

	domainSeparator, err := typedData.HashStruct("EIP712Domain", typedData.Domain.Map())
	if err != nil {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: hash domain: %w", err)
	}
	messageHash, err := typedData.HashStruct("Permit", message)
	if err != nil {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: hash message: %w", err)
	}

	rawData := []byte{0x19, 0x01}
	rawData = append(rawData, domainSeparator...)
	rawData = append(rawData, messageHash...)
	digest := crypto.Keccak256(rawData)

	sig, err := signHash(digest)
	if err != nil {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: %w", err)
	}
	if len(sig) != 65 {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: %w", ErrInvalidSignatureLength)
	}

	v := sig[64]
	if v < 27 {
		v += 27
	}
	if v != 27 && v != 28 {
		return nil, fmt.Errorf("typeddata.SignERC20Permit: %w", ErrInvalidRecoveryID)
	}

	var r, s [32]byte
	copy(r[:], sig[:32])
	copy(s[:], sig[32:64])

	sBig := new(big.Int).SetBytes(s[:])
	if sBig.Cmp(secp256k1HalfN) > 0 {
		sBig.Sub(secp256k1.S256().Params().N, sBig)
		sBytes := sBig.Bytes()
		s = [32]byte{}
		copy(s[32-len(sBytes):], sBytes)
		if v == 27 {
			v = 28
		} else {
			v = 27
		}
	}

	return &Signature{V: v, R: r, S: s}, nil
}
