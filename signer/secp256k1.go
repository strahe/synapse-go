package signer

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"

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
type Secp256k1Signer struct {
	mu       sync.RWMutex
	ecdsaKey *ecdsa.PrivateKey
	filAddr  address.Address
	ethAddr  common.Address
}

var _ EVMSigner = (*Secp256k1Signer)(nil)

// NewSecp256k1Signer creates a dual-protocol signer from a go-ethereum ECDSA
// private key. The key is deep-copied so that [Secp256k1Signer.Zero] does not
// mutate the caller's key material.
func NewSecp256k1Signer(key *ecdsa.PrivateKey) (*Secp256k1Signer, error) {
	if key == nil {
		return nil, fmt.Errorf("signer: nil private key")
	}
	// Deep-copy: caller retains ownership of the original key.
	keyCopy := *key
	keyCopy.D = new(big.Int).Set(key.D)
	return newSecp256k1(&keyCopy)
}

// NewSecp256k1SignerFromBytes creates a dual-protocol signer from a raw
// 32-byte private key scalar. Shorter inputs are left-padded to 32 bytes
// to handle big.Int.Bytes() output that may drop leading zeros.
func NewSecp256k1SignerFromBytes(raw []byte) (*Secp256k1Signer, error) {
	if len(raw) == 0 || len(raw) > 32 {
		return nil, fmt.Errorf("signer: invalid key length %d", len(raw))
	}
	var padded [32]byte
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
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ecdsaKey == nil {
		return nil, fmt.Errorf("signer.Sign: signer has been zeroed")
	}
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
// on the given chain. The returned opts embed their own key copy so they remain
// valid even if [Zero] is called after Transactor returns.
func (s *Secp256k1Signer) Transactor(chainID *big.Int) (*bind.TransactOpts, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ecdsaKey == nil {
		return nil, fmt.Errorf("signer.Transactor: signer has been zeroed")
	}
	// Copy the key so the returned TransactOpts closure is independent of Zero().
	keyCopy := *s.ecdsaKey
	keyCopy.D = new(big.Int).Set(s.ecdsaKey.D)
	return bind.NewKeyedTransactorWithChainID(&keyCopy, chainID)
}

// SignHash signs a pre-computed 32-byte hash using the secp256k1 key.
// Returns 65-byte R‖S‖V signature.
func (s *Secp256k1Signer) SignHash(hash []byte) ([]byte, error) {
	if len(hash) != 32 {
		return nil, fmt.Errorf("signer.SignHash: hash must be 32 bytes, got %d", len(hash))
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.ecdsaKey == nil {
		return nil, fmt.Errorf("signer.SignHash: signer has been zeroed")
	}
	return ethcrypto.Sign(hash, s.ecdsaKey)
}

// Zero clears the private key material from memory.
// It blocks until any in-progress [Sign], [SignHash], or [Transactor] call
// completes, then prevents further signing. The signer must not be used after
// Zero is called.
func (s *Secp256k1Signer) Zero() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ecdsaKey != nil {
		// Best-effort overwrite: SetBytes forces a new backing array write
		// before SetInt64 truncates it. Go's GC does not guarantee the old
		// array bytes are wiped, but this minimises the exposure window.
		s.ecdsaKey.D.SetBytes(make([]byte, 32))
		s.ecdsaKey.D.SetInt64(0)
		s.ecdsaKey = nil
	}
}
