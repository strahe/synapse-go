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
	"github.com/strahe/synapse-go/internal/adapters"
	"github.com/strahe/synapse-go/internal/lifecycle"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

// Client is the root entry point for the Filecoin Onchain Cloud SDK.
// It composes all sub-services, all of which are initialised eagerly by [New].
// Create with [New]; release resources with [Client.Close].
//
// All methods are safe for concurrent use.
type Client struct {
	ethClient              *ethclient.Client
	ownsClient             bool
	evmSigner              signer.EVMSigner
	selectedChain          chain.Chain
	addresses              iabi.ResolvedAddresses
	nonces                 *txutil.NonceManager
	logger                 *slog.Logger
	httpClient             *http.Client
	source                 string
	withCDN                bool
	filbeamRetrievalDomain string

	lifecycle *lifecycle.Lifecycle
	closeOnce sync.Once

	warmStorage *warmstorage.Service
	spRegistry  *spregistry.Service
	payments    *payments.Service
	sessionKey  *sessionkey.Service
	costs       *costs.Service
	filbeam     *filbeam.Service
	storage     *storage.Service
	pdpReader   adapters.PDPReader

	allowPrivateNetworks bool
}

type clientConfig struct {
	privateKey             *ecdsa.PrivateKey
	privateKeyHex          string
	rpcURL                 string
	ethClient              *ethclient.Client
	chain                  *chain.Chain
	logger                 *slog.Logger
	httpClient             *http.Client
	source                 string
	withCDN                bool
	filbeamRetrievalDomain string
	allowPrivateNetworks   bool
}

// ClientOption configures a [Client] via [New].
type ClientOption func(*clientConfig)

// WithPrivateKey sets the ECDSA private key used for transaction signing.
// If both WithPrivateKey and [WithPrivateKeyHex] are provided,
// WithPrivateKey takes precedence.
func WithPrivateKey(key *ecdsa.PrivateKey) ClientOption {
	return func(cfg *clientConfig) { cfg.privateKey = key }
}

// WithPrivateKeyHex sets the private key from a hex-encoded string.
// Both "0x"-prefixed and raw hex are accepted. Ignored when
// [WithPrivateKey] is also provided.
func WithPrivateKeyHex(hex string) ClientOption {
	return func(cfg *clientConfig) { cfg.privateKeyHex = hex }
}

// WithRPCURL sets the JSON-RPC endpoint URL. An ethclient is dialed
// during [New] and closed by [Client.Close].
func WithRPCURL(url string) ClientOption {
	return func(cfg *clientConfig) { cfg.rpcURL = url }
}

// WithEthClient provides a pre-created [ethclient.Client].
// When set, [WithRPCURL] is ignored and the client is NOT closed
// by [Client.Close].
func WithEthClient(c *ethclient.Client) ClientOption {
	return func(cfg *clientConfig) { cfg.ethClient = c }
}

// WithChain overrides automatic chain detection. When omitted, [New]
// calls eth_chainId on the RPC endpoint.
func WithChain(c chain.Chain) ClientOption {
	return func(cfg *clientConfig) {
		cc := c
		cfg.chain = &cc
	}
}

// WithLogger sets the structured logger passed to sub-services.
// Nil disables logging (the default).
func WithLogger(l *slog.Logger) ClientOption {
	return func(cfg *clientConfig) { cfg.logger = l }
}

// WithHTTPClient sets the HTTP client used by every service that makes HTTP
// calls:
//
//   - filbeam.Service (stats API)
//   - storage.Service (URL-based downloads via Service.HTTPClient)
//   - provider HTTP clients constructed by the storage resolver for upload,
//     pull, and provider RPC calls
//
// Services communicating over Ethereum JSON-RPC (payments, sessionkey,
// warmstorage, spregistry, costs) reuse the chain client instead and are not
// affected by this option. If nil, each HTTP service uses its own default.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cfg *clientConfig) { cfg.httpClient = c }
}

// WithSource sets the application-level source identifier used for
// dataset namespace isolation. Datasets with different source values
// are treated as distinct namespaces; reuse only occurs within the
// same source.
func WithSource(s string) ClientOption {
	return func(cfg *clientConfig) { cfg.source = s }
}

// WithCDN sets the Client-wide default for CDN-first context downloads
// and the withCDN dataset-metadata flag used during provider selection.
//
// This is a default only: each [storage.UploadOptions] and
// [storage.CreateContextsOptions] carries its own *bool WithCDN that
// overrides the Client default when non-nil. Leaving the per-op field
// nil inherits this Client default.
//
// Example — override per upload:
//
//	b := false
//	_, err := client.Storage().Upload(ctx, r, &storage.UploadOptions{WithCDN: &b})
func WithCDN(enabled bool) ClientOption {
	return func(cfg *clientConfig) { cfg.withCDN = enabled }
}

// WithFilBeamRetrievalDomain overrides the chain default FilBeam retrieval
// domain. Leave unset for the built-in Mainnet / Calibration defaults.
func WithFilBeamRetrievalDomain(domain string) ClientOption {
	return func(cfg *clientConfig) { cfg.filbeamRetrievalDomain = domain }
}

// WithAllowPrivateNetworks disables the built-in SSRF guard for
// URL-based [storage.Service.Download] calls. When false (the default),
// the storage service refuses to dial loopback, RFC1918, link-local,
// ULA, multicast, and unspecified addresses, returning
// [storage.ErrPrivateNetwork].
//
// Set to true only when you knowingly need to fetch content from a
// private network (e.g. in-cluster storage nodes). This is an explicit
// SSRF opt-in for trusted private infrastructure; do not enable it for
// untrusted user-supplied URLs.
//
// This option has no effect when [WithHTTPClient] is also provided:
// the custom client's transport is responsible for any SSRF safeguards.
func WithAllowPrivateNetworks(allow bool) ClientOption {
	return func(cfg *clientConfig) { cfg.allowPrivateNetworks = allow }
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

	cleanup, err := resolvePrivateKey(&cfg)
	if err != nil {
		return nil, fmt.Errorf("synapse.New: %w", err)
	}
	defer cleanup()

	ec, ownsClient, err := resolveEthClient(ctx, &cfg)
	if err != nil {
		return nil, fmt.Errorf("synapse.New: %w", err)
	}

	success := false
	defer func() {
		if !success && ownsClient {
			ec.Close()
		}
	}()

	selectedChain, err := resolveChain(ctx, ec, &cfg)
	if err != nil {
		return nil, fmt.Errorf("synapse.New: %w", err)
	}

	addresses, err := resolveAddresses(selectedChain)
	if err != nil {
		return nil, fmt.Errorf("synapse.New: %w", err)
	}

	client, err := newClient(&cfg, ec, ownsClient, selectedChain, addresses)
	if err != nil {
		return nil, fmt.Errorf("synapse.New: %w", err)
	}
	success = true
	return client, nil
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
		if c.lifecycle != nil {
			c.lifecycle.Close()
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
	defer func() {
		for i := range decoded {
			decoded[i] = 0
		}
	}()
	if err != nil {
		return nil, fmt.Errorf("decode private key hex: %w", err)
	}
	key, err := ethcrypto.ToECDSA(decoded)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}
	return key, nil
}

func resolvePrivateKey(cfg *clientConfig) (func(), error) {
	if cfg.privateKey == nil && cfg.privateKeyHex != "" {
		key, err := parsePrivateKeyHex(cfg.privateKeyHex)
		if err != nil {
			return nil, err
		}
		cfg.privateKey = key
		// Return a cleanup that zeros only this intermediate key.
		return func() { zeroPrivateKey(cfg.privateKey) }, nil
	}
	if cfg.privateKey == nil {
		return nil, errors.New("missing private key (use WithPrivateKey or WithPrivateKeyHex)")
	}
	// No cleanup for user-provided key.
	return func() {}, nil
}

// zeroPrivateKey clears the private key D scalar. The signer deep-copies D,
// so this does not affect the created signer. The backing words are cleared in
// place before SetInt64 truncates the logical value; SetInt64 alone only changes
// the length and can leave key bits in the heap.
func zeroPrivateKey(key *ecdsa.PrivateKey) {
	if key != nil && key.D != nil {
		bits := key.D.Bits()
		clear(bits[:cap(bits)])
		key.D.SetInt64(0)
	}
}

func resolveEthClient(ctx context.Context, cfg *clientConfig) (*ethclient.Client, bool, error) {
	switch {
	case cfg.ethClient != nil:
		return cfg.ethClient, false, nil
	case cfg.rpcURL != "":
		c, err := ethclient.DialContext(ctx, cfg.rpcURL)
		if err != nil {
			return nil, false, fmt.Errorf("dial RPC: %w", err)
		}
		return c, true, nil
	default:
		return nil, false, errors.New("missing RPC source (use WithRPCURL or WithEthClient)")
	}
}

func resolveChain(ctx context.Context, ec *ethclient.Client, cfg *clientConfig) (chain.Chain, error) {
	if cfg.chain != nil {
		return *cfg.chain, nil
	}
	chainID, err := ec.ChainID(ctx)
	if err != nil {
		return 0, fmt.Errorf("detect chain: %w", err)
	}
	if !chainID.IsInt64() {
		return 0, fmt.Errorf("chain ID %s exceeds int64 range", chainID)
	}
	detected, err := chain.FromID(chainID.Int64())
	if err != nil {
		return 0, fmt.Errorf("unsupported chain %d: %w", chainID.Int64(), err)
	}
	return detected, nil
}

func resolveAddresses(c chain.Chain) (iabi.ResolvedAddresses, error) {
	addresses := iabi.ResolvedAddressesFromChain(c)
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
			return iabi.ResolvedAddresses{}, fmt.Errorf("no %s address for chain %s", name, c)
		}
	}
	return addresses, nil
}

func newClient(cfg *clientConfig, ec *ethclient.Client, ownsClient bool, selectedChain chain.Chain, addresses iabi.ResolvedAddresses) (*Client, error) {
	evmSigner, err := signer.NewSecp256k1Signer(cfg.privateKey)
	if err != nil {
		return nil, fmt.Errorf("create signer: %w", err)
	}
	nonces := txutil.NewNonceManager(ec, evmSigner.EVMAddress())

	c := &Client{
		ethClient:              ec,
		ownsClient:             ownsClient,
		evmSigner:              evmSigner,
		selectedChain:          selectedChain,
		addresses:              addresses,
		nonces:                 nonces,
		logger:                 cfg.logger,
		httpClient:             cfg.httpClient,
		source:                 cfg.source,
		withCDN:                cfg.withCDN,
		filbeamRetrievalDomain: cfg.filbeamRetrievalDomain,
		allowPrivateNetworks:   cfg.allowPrivateNetworks,
		lifecycle:              lifecycle.New(),
	}
	if err := c.initServices(); err != nil {
		return nil, err
	}
	return c, nil
}
