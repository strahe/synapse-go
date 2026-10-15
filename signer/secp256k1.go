package signer

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/crypto"

	blake2b "github.com/minio/blake2b-simd"
)

// Secp256k1Signer implements [EVMSigner] backed by a secp256k1 private key.
// It can sign both Filecoin messages (blake2b hash) and Ethereum/FEVM
// transactions (keccak256 hash) from a single key.
//
// Private-key lifecycle is the caller's responsibility: the SDK does not
// provide an in-place wipe primitive. To bound the in-memory exposure of the
// key, hold the signer for as short a lifetime as the workload allows and let
// it be garbage-collected.
type Secp256k1Signer struct {
	ecdsaKey *ecdsa.PrivateKey
	filAddr  address.Address
	ethAddr  common.Address
}

var (
	_ EVMSigner     = (*Secp256k1Signer)(nil)
	_ StorageSigner = (*Secp256k1Signer)(nil)
)

// NewSecp256k1Signer creates a dual-protocol signer from a go-ethereum ECDSA
// private key. The key must use the go-ethereum secp256k1 curve and its public
// key must match its private scalar; invalid keys return an error.
// The caller must not modify the key until this call returns. The signer owns
// an independent copy, so the caller may then clear or mutate the original.
func NewSecp256k1Signer(key *ecdsa.PrivateKey) (*Secp256k1Signer, error) {
	keyCopy, err := cloneSecp256k1PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return newSecp256k1(keyCopy)
}

// NewSecp256k1SignerFromBytes creates a dual-protocol signer from a raw
// 32-byte private key scalar. Shorter inputs are left-padded to 32 bytes
// to handle big.Int.Bytes() output that may drop leading zeros.
func NewSecp256k1SignerFromBytes(raw []byte) (*Secp256k1Signer, error) {
	if len(raw) == 0 || len(raw) > 32 {
		return nil, fmt.Errorf("signer: invalid key length %d", len(raw))
	}
	var padded [32]byte
	defer func() {
		for i := range padded {
			padded[i] = 0
		}
	}()
	copy(padded[32-len(raw):], raw)

	key, err := ethcrypto.ToECDSA(padded[:])
	if err != nil {
		return nil, fmt.Errorf("signer: invalid secp256k1 key: %w", err)
	}
	return newSecp256k1(key)
}

func newSecp256k1(ecdsaKey *ecdsa.PrivateKey) (*Secp256k1Signer, error) {
	filAddr, err := address.NewSecp256k1Address(ethcrypto.FromECDSAPub(&ecdsaKey.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("signer: deriving filecoin address: %w", err)
	}

	ethAddr := ethcrypto.PubkeyToAddress(ecdsaKey.PublicKey)

	return &Secp256k1Signer{
		ecdsaKey: ecdsaKey,
		filAddr:  filAddr,
		ethAddr:  ethAddr,
	}, nil
}

// FilecoinAddress returns the Filecoin secp256k1 protocol address.
func (s *Secp256k1Signer) FilecoinAddress() address.Address { return s.filAddr }

// EVMAddress returns the Ethereum address derived from the public key.
func (s *Secp256k1Signer) EVMAddress() common.Address { return s.ethAddr }

// Sign produces a Filecoin-native secp256k1 signature.
// The message is hashed with blake2b-256 before signing, and the result
// is in R|S|V format (65 bytes).
func (s *Secp256k1Signer) Sign(msg []byte) (*crypto.Signature, error) {
	hash := blake2b.Sum256(msg)
	sig, err := ethcrypto.Sign(hash[:], s.ecdsaKey)
	if err != nil {
		return nil, fmt.Errorf("signer.Sign: %w", err)
	}

	return &crypto.Signature{
		Type: crypto.SigTypeSecp256k1,
		Data: sig,
	}, nil
}

// Transactor returns bind.TransactOpts for signing Ethereum/FEVM transactions
// on the given chain. The returned opts embed their own key copy so they
// remain valid for the lifetime of the opts independent of the signer.
func (s *Secp256k1Signer) Transactor(chainID *big.Int) (*bind.TransactOpts, error) {
	return bind.NewKeyedTransactorWithChainID(copySecp256k1PrivateKey(s.ecdsaKey), chainID)
}

//nolint:staticcheck // secp256k1 needs raw key fields; stdlib encoding does not support this curve.
func cloneSecp256k1PrivateKey(key *ecdsa.PrivateKey) (*ecdsa.PrivateKey, error) {
	if key == nil {
		return nil, fmt.Errorf("signer: nil private key")
	}
	if key.D == nil {
		return nil, fmt.Errorf("signer: nil private scalar")
	}
	curve := ethcrypto.S256()
	if key.Curve != curve {
		return nil, fmt.Errorf("signer: private key must use the go-ethereum secp256k1 curve")
	}
	params := curve.Params()
	if key.D.Sign() <= 0 || key.D.Cmp(params.N) >= 0 {
		return nil, fmt.Errorf("signer: private scalar must be greater than zero and less than the curve order")
	}
	if key.X == nil || key.Y == nil {
		return nil, fmt.Errorf("signer: missing public key coordinates")
	}
	if key.X.Sign() < 0 || key.X.Cmp(params.P) >= 0 || key.Y.Sign() < 0 || key.Y.Cmp(params.P) >= 0 {
		return nil, fmt.Errorf("signer: public key coordinates out of range")
	}
	var raw [32]byte
	defer clear(raw[:])
	key.D.FillBytes(raw[:])
	keyCopy, err := ethcrypto.ToECDSA(raw[:])
	if err != nil {
		return nil, fmt.Errorf("signer: invalid secp256k1 key: %w", err)
	}
	if key.X.Cmp(keyCopy.X) != 0 || key.Y.Cmp(keyCopy.Y) != 0 {
		return nil, fmt.Errorf("signer: public key does not match private scalar")
	}
	return keyCopy, nil
}

//nolint:staticcheck // secp256k1 copies need raw key fields; stdlib encoding does not support this curve.
func copySecp256k1PrivateKey(key *ecdsa.PrivateKey) *ecdsa.PrivateKey {
	return &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{
			Curve: ethcrypto.S256(),
			X:     new(big.Int).Set(key.X),
			Y:     new(big.Int).Set(key.Y),
		},
		D: new(big.Int).Set(key.D),
	}
}

// SignHash signs a pre-computed 32-byte hash using the secp256k1 key.
// Returns 65-byte R‖S‖V signature.
//
// SignHash implements [HashSigner]. Raw hash signing bypasses domain
// separation, so callers should prefer a higher-level API that constructs the
// message and use a dedicated key or access policy for this capability.
func (s *Secp256k1Signer) SignHash(hash []byte) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("signer.SignHash: hash must be 32 bytes, got %d", len(hash))
	}
	return ethcrypto.Sign(hash, s.ecdsaKey)
}
