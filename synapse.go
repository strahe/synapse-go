package synapse

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	ethcrypto "github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/filbeam"
	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

// Client is the root entry point for the Filecoin Onchain Cloud SDK.
// It composes all sub-services and provides lazy-initialized access to
// each one. Create with [New]; release resources with [Close].
//
// All methods are safe for concurrent use.
type Client struct {
	ethClient     *ethclient.Client
	ownsClient    bool
	evmSigner     signer.EVMSigner
	selectedChain chain.Chain
	addresses     iabi.ResolvedAddresses
	nonces        *txutil.NonceManager
	logger        *slog.Logger
	httpClient    *http.Client
	source        string

	closeOnce sync.Once

	warmStorageOnce sync.Once
	warmStorage     *warmstorage.Service

	spRegistryOnce sync.Once
	spRegistry     *spregistry.Service

	paymentsOnce sync.Once
	payments     *payments.Service

	sessionKeyOnce sync.Once
	sessionKey     *sessionkey.Service

	costsOnce sync.Once
	costs     *costs.Service

	filbeamOnce sync.Once
	filbeam     *filbeam.Service

	storageOnce sync.Once
	storage     *storage.Manager
}

// New creates a Client, connecting to the given RPC endpoint and
// resolving the chain and contract addresses.
//
// Required options: a private key ([WithPrivateKey] or [WithPrivateKeyHex])
// and an RPC source ([WithRPCURL] or [WithEthClient]).
func New(ctx context.Context, opts ...ClientOption) (*Client, error) {
	var cfg clientConfig
	for _, o := range opts {
		o(&cfg)
	}

	// --- resolve private key ---
	if cfg.privateKey == nil && cfg.privateKeyHex != "" {
		key, err := parsePrivateKeyHex(cfg.privateKeyHex)
		if err != nil {
			return nil, fmt.Errorf("synapse.New: %w", err)
		}
		cfg.privateKey = key
	}
	if cfg.privateKey == nil {
		return nil, errors.New("synapse.New: missing private key (use WithPrivateKey or WithPrivateKeyHex)")
	}

	// --- resolve ethclient ---
	var (
		ec         *ethclient.Client
		ownsClient bool
	)
	switch {
	case cfg.ethClient != nil:
		ec = cfg.ethClient
	case cfg.rpcURL != "":
		c, err := ethclient.DialContext(ctx, cfg.rpcURL)
		if err != nil {
			return nil, fmt.Errorf("synapse.New: dial RPC: %w", err)
		}
		ec = c
		ownsClient = true
	default:
		return nil, errors.New("synapse.New: missing RPC source (use WithRPCURL or WithEthClient)")
	}

	// --- resolve chain ---
	var selectedChain chain.Chain
	if cfg.chain != nil {
		selectedChain = *cfg.chain
	} else {
		chainID, err := ec.ChainID(ctx)
		if err != nil {
			if ownsClient {
				ec.Close()
			}
			return nil, fmt.Errorf("synapse.New: detect chain: %w", err)
		}
		if !chainID.IsInt64() {
			if ownsClient {
				ec.Close()
			}
			return nil, fmt.Errorf("synapse.New: chain ID %s exceeds int64 range", chainID)
		}
		detected, err := chain.FromID(chainID.Int64())
		if err != nil {
			if ownsClient {
				ec.Close()
			}
			return nil, fmt.Errorf("synapse.New: unsupported chain %d: %w", chainID.Int64(), err)
		}
		selectedChain = detected
	}

	// --- validate chain addresses ---
	addresses := iabi.ResolvedAddressesFromChain(selectedChain)
	zeroAddr := common.Address{}
	for name, addr := range map[string]common.Address{
		"FWSS":               addresses.FWSS,
		"Payments":           addresses.Payments,
		"PDPVerifier":        addresses.PDPVerifier,
		"SPRegistry":         addresses.SPRegistry,
		"USDFC":              addresses.USDFC,
		"ViewContract":       addresses.ViewContract,
		"SessionKeyRegistry": addresses.SessionKeyRegistry,
	} {
		if addr == zeroAddr {
			if ownsClient {
				ec.Close()
			}
			return nil, fmt.Errorf("synapse.New: no %s address for chain %s", name, selectedChain)
		}
	}

	// --- build signer + nonce manager ---
	evmSigner, err := signer.NewSecp256k1Signer(cfg.privateKey)
	if err != nil {
		if ownsClient {
			ec.Close()
		}
		return nil, fmt.Errorf("synapse.New: create signer: %w", err)
	}
	nonces := txutil.NewNonceManager(ec, evmSigner.EVMAddress())

	return &Client{
		ethClient:     ec,
		ownsClient:    ownsClient,
		evmSigner:     evmSigner,
		selectedChain: selectedChain,
		addresses:     addresses,
		nonces:        nonces,
		logger:        cfg.logger,
		httpClient:    cfg.httpClient,
		source:        cfg.source,
	}, nil
}

// Close releases resources held by the Client. If the ethclient was
// created internally (via [WithRPCURL]), it is closed. User-provided
// clients (via [WithEthClient]) are left open. Safe to call concurrently
// or multiple times.
//
// After Close returns, the Client and all services obtained from it must
// not be used.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		if s, ok := c.evmSigner.(*signer.Secp256k1Signer); ok {
			s.Zero()
		}
		if c.ownsClient && c.ethClient != nil {
			c.ethClient.Close()
		}
	})
	return nil
}

// Chain returns the resolved [chain.Chain].
func (c *Client) Chain() chain.Chain {
	return c.selectedChain
}

// Address returns the EVM address derived from the private key.
func (c *Client) Address() common.Address {
	return c.evmSigner.EVMAddress()
}

func parsePrivateKeyHex(raw string) (*ecdsa.PrivateKey, error) {
	hexStr := strings.TrimPrefix(strings.TrimSpace(raw), "0x")
	decoded, err := hex.DecodeString(hexStr)
	if err != nil {
		return nil, fmt.Errorf("decode private key hex: %w", err)
	}
	key, err := ethcrypto.ToECDSA(decoded)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return key, nil
}
