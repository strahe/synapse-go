package signer

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"
)

// Signer signs native Filecoin messages. Every key type implements this.
type Signer interface {
	// FilecoinAddress returns the Filecoin protocol address for this key.
	FilecoinAddress() address.Address

	// Sign produces a Filecoin-native signature over msg.
	Sign(msg []byte) (*crypto.Signature, error)
}

// EVMSigner extends Signer with Ethereum/FEVM transaction signing.
// Only secp256k1 keys support this.
type EVMSigner interface {
	Signer

	// EVMAddress returns the Ethereum address derived from this key.
	EVMAddress() common.Address

	// SignHash signs a pre-computed 32-byte hash and returns a 65-byte
	// signature in R‖S‖V format. This is used for EIP-712 typed data
	// signing and other raw-hash signing operations.
	SignHash(hash []byte) ([]byte, error)

	// Transactor returns go-ethereum TransactOpts bound to the given chain ID.
	Transactor(chainID *big.Int) (*bind.TransactOpts, error)
}

// AsEVM checks whether a Signer can also sign EVM transactions.
// Returns nil, false for key types that don't support EVM (e.g., BLS).
func AsEVM(s Signer) (EVMSigner, bool) {
	e, ok := s.(EVMSigner)
	return e, ok
}
