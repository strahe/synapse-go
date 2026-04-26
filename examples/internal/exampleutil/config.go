package exampleutil

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
)

const (
	// DefaultRPCURL points at Filecoin Calibration, the network targeted by
	// these runnable examples.
	DefaultRPCURL       = "https://api.calibration.node.glif.io/rpc/v1"
	DefaultTimeout      = 30 * time.Minute
	MinUploadBytes      = 127
	PrivateKeyEnvVar    = "SYNAPSE_PRIVATE_KEY"
	legacyPrivateKeyVar = "PRIVATE_KEY"
)

// EnvConfig contains the common wallet/RPC settings shared by examples.
type EnvConfig struct {
	PrivateKeyHex string
	RPCURL        string
	Chain         *chain.Chain
}

// LoadEnv reads the standard example environment. SYNAPSE_PRIVATE_KEY is
// required; RPC_URL defaults to the public Calibration endpoint.
func LoadEnv(getenv func(string) string) (EnvConfig, error) {
	privateKeyHex := strings.TrimSpace(getenv(PrivateKeyEnvVar))
	if privateKeyHex == "" {
		if strings.TrimSpace(getenv(legacyPrivateKeyVar)) != "" {
			return EnvConfig{}, fmt.Errorf("%s is ignored; set %s for these examples", legacyPrivateKeyVar, PrivateKeyEnvVar)
		}
		return EnvConfig{}, fmt.Errorf("%s is required", PrivateKeyEnvVar)
	}

	rpcURL := strings.TrimSpace(getenv("RPC_URL"))
	if rpcURL == "" {
		rpcURL = DefaultRPCURL
	} else {
		rpcURL = NormalizeRPCURL(rpcURL)
	}

	selectedChain, err := ParseChain(getenv("CHAIN"))
	if err != nil {
		return EnvConfig{}, err
	}

	return EnvConfig{
		PrivateKeyHex: privateKeyHex,
		RPCURL:        rpcURL,
		Chain:         selectedChain,
	}, nil
}

// NormalizeRPCURL accepts the GLIF host-only URLs that older examples used and
// converts them to the JSON-RPC endpoint expected by Filecoin clients.
func NormalizeRPCURL(raw string) string {
	rpcURL := strings.TrimRight(strings.TrimSpace(raw), "/")
	switch rpcURL {
	case "https://api.calibration.node.glif.io":
		return "https://api.calibration.node.glif.io/rpc/v1"
	case "https://api.node.glif.io":
		return "https://api.node.glif.io/rpc/v1"
	default:
		return strings.TrimSpace(raw)
	}
}

// ParseChain maps CHAIN to a known chain. Empty means auto-detect from RPC.
func ParseChain(raw string) (*chain.Chain, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return nil, nil
	case "calibration":
		c := chain.Calibration
		return &c, nil
	case "mainnet":
		c := chain.Mainnet
		return &c, nil
	default:
		return nil, fmt.Errorf("unsupported CHAIN %q (use calibration or mainnet)", raw)
	}
}

// ClientOptions returns synapse.New options for the common environment.
func (c EnvConfig) ClientOptions(extra ...synapse.ClientOption) []synapse.ClientOption {
	opts := []synapse.ClientOption{
		synapse.WithPrivateKeyHex(c.PrivateKeyHex),
		synapse.WithRPCURL(c.RPCURL),
		synapse.WithAllowPrivateNetworks(true),
	}
	if c.Chain != nil {
		opts = append(opts, synapse.WithChain(*c.Chain))
	}
	opts = append(opts, extra...)
	return opts
}

// NewClient creates a root SDK client from EnvConfig.
func NewClient(ctx context.Context, cfg EnvConfig, extra ...synapse.ClientOption) (*synapse.Client, error) {
	client, err := synapse.New(ctx, cfg.ClientOptions(extra...)...)
	if err != nil {
		return nil, fmt.Errorf("create synapse client: %w", err)
	}
	return client, nil
}

// WithTimeout applies a consistent timeout to example commands.
func WithTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(parent, timeout)
}

// OpenFile opens a file path for reading with a contextual error.
func OpenFile(path string) (*os.File, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("file path is required")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return f, nil
}

// OpenRegularFile opens a regular file for upload examples.
func OpenRegularFile(path string) (*os.File, os.FileInfo, error) {
	f, err := OpenFile(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, fmt.Errorf("stat %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, fmt.Errorf("%s is not a regular file", path)
	}
	return f, info, nil
}

// ValidateUploadSize rejects payloads too small to produce a PieceCIDv2.
func ValidateUploadSize(label string, size int64) error {
	if size <= 0 {
		return fmt.Errorf("%s is empty", label)
	}
	if size < MinUploadBytes {
		return fmt.Errorf("%s is too small: FOC uploads need at least %d bytes for a PieceCIDv2, got %d", label, MinUploadBytes, size)
	}
	return nil
}

// PrintPrepare writes the funding summary returned by storage.Prepare.
func PrintPrepare(w io.Writer, result *storage.PrepareResult) error {
	if result == nil || result.Costs == nil {
		return errors.New("prepare returned no cost summary")
	}
	if err := WriteKV(w, "ready", result.Costs.Ready); err != nil {
		return err
	}
	if err := WriteKV(w, "depositNeeded", result.Costs.DepositNeeded); err != nil {
		return err
	}
	return WriteKV(w, "needsFWSSApproval", result.Costs.NeedsFWSSMaxApproval)
}

// WriteTx writes the transaction hash from an executed SDK write.
func WriteTx(w io.Writer, prefix string, result *types.WriteResult) error {
	if result == nil {
		return WriteKV(w, prefix+".txHash", "<nil>")
	}
	return WriteKV(w, prefix+".txHash", result.Hash.Hex())
}

// WriteKV writes a stable key=value line.
func WriteKV(w io.Writer, key string, value any) error {
	_, err := fmt.Fprintf(w, "%s=%v\n", key, value)
	return err
}

// WriteMap writes a string map in sorted key order as prefix.key=value lines.
func WriteMap(w io.Writer, prefix string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := WriteKV(w, prefix+"."+key, values[key]); err != nil {
			return err
		}
	}
	return nil
}
