package typeddata

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

// NewDomain creates an EIP-712 domain for FilecoinWarmStorageService.
func NewDomain(chainID *big.Int, fwssAddr common.Address) apitypes.TypedDataDomain {
	return apitypes.TypedDataDomain{
		Name:              "FilecoinWarmStorageService",
		Version:           "1",
		ChainId:           (*math.HexOrDecimal256)(chainID),
		VerifyingContract: fwssAddr.Hex(),
	}
}
