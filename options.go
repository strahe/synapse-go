package synapse

import (
	"crypto/ecdsa"
	"log/slog"
	"net/http"

	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/strahe/synapse-go/chain"
)

type clientConfig struct {
	privateKey    *ecdsa.PrivateKey
	privateKeyHex string
	rpcURL        string
	ethClient     *ethclient.Client
	chain         *chain.Chain
	logger        *slog.Logger
	httpClient    *http.Client
	source        string
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
//   - internal/curio.Client (piece uploads, pull / RPC, constructed per
//     UploadContext inside the storage resolver)
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
// same source. Mirrors the TS SDK's Synapse.create({ source }) option.
func WithSource(s string) ClientOption {
	return func(cfg *clientConfig) { cfg.source = s }
}
