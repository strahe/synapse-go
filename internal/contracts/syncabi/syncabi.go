package syncabi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ContractSpec struct {
	PackageName string
	UpstreamABI string
	MergeErrors bool
}

type Config struct {
	BaseURL string
	Ref     string
	RootDir string
	Client  *http.Client
}

const (
	// DefaultRepo is the upstream ABI source of truth for generated bindings.
	DefaultRepo = "FilOzone/filecoin-services"
	// DefaultRef must track synapse-sdk/packages/synapse-core/wagmi.config.ts.
	// Upgrade flow: update this ref, run `make generate-contracts`, then commit abi.json and bindings.go.
	DefaultRef = "ed85348ebad54196b5bfefc5cb0dbe7e8bfd6f7c"
)

var manifest = []ContractSpec{
	{PackageName: "pdpverifier", UpstreamABI: "PDPVerifier.abi.json"},
	{PackageName: "fwss", UpstreamABI: "FilecoinWarmStorageService.abi.json", MergeErrors: true},
	{PackageName: "fwssview", UpstreamABI: "FilecoinWarmStorageServiceStateView.abi.json", MergeErrors: true},
	{PackageName: "spregistry", UpstreamABI: "ServiceProviderRegistry.abi.json", MergeErrors: true},
	{PackageName: "filpay", UpstreamABI: "FilecoinPayV1.abi.json"},
}

func Sync(ctx context.Context, cfg Config) error {
	if cfg.BaseURL == "" && cfg.Ref == "" {
		return fmt.Errorf("syncabi.Sync: either Config.BaseURL or Config.Ref must be set")
	}
	client := cfg.Client
	if client == nil {
		client = http.DefaultClient
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL(cfg.Ref)
	}

	errorsABI, err := fetchABI(ctx, client, baseURL, "Errors.abi.json")
	if err != nil {
		return fmt.Errorf("syncabi.Sync: fetch Errors.abi.json: %w", err)
	}

	pendingWrites := make(map[string][]json.RawMessage, len(manifest))
	for _, spec := range manifest {
		entries, err := fetchABI(ctx, client, baseURL, spec.UpstreamABI)
		if err != nil {
			return fmt.Errorf("syncabi.Sync: fetch %s: %w", spec.UpstreamABI, err)
		}
		if spec.MergeErrors {
			entries = append(entries, errorsABI...)
		}
		pendingWrites[spec.PackageName] = entries
	}

	for pkg, entries := range pendingWrites {
		if err := writeABI(cfg.RootDir, pkg, entries); err != nil {
			return fmt.Errorf("syncabi.Sync: write %s: %w", pkg, err)
		}
	}

	return nil
}

func fetchABI(ctx context.Context, client *http.Client, baseURL, name string) (_ []json.RawMessage, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/"+name, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err = errors.Join(err, resp.Body.Close())
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}

	var entries []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// DefaultBaseURL builds a raw.githubusercontent.com URL for the given git
// ref. Supported ref forms:
//   - 40-char lower-hex commit SHA (passed verbatim)
//   - ref fragment like "heads/<branch>" or "tags/<tag>"
//   - fully-qualified ref like "refs/heads/<branch>" or "refs/tags/<tag>"
//   - bare branch name (anything else)
//
// Note: a bare ref such as "refs/main" is invalid on GitHub's raw server, so
// a plain branch name must NOT be prefixed with refs/.
func DefaultBaseURL(ref string) string {
	if matched, _ := regexp.MatchString("^[a-f0-9]{40}$", ref); matched {
		return "https://raw.githubusercontent.com/" + DefaultRepo + "/" + ref + "/service_contracts/abi"
	}
	trimmed := strings.TrimPrefix(ref, "refs/")
	// Fragments rooted under heads/ or tags/ are mapped to refs/<fragment>.
	// Anything else is treated as a bare branch name.
	if strings.HasPrefix(trimmed, "heads/") || strings.HasPrefix(trimmed, "tags/") {
		return "https://raw.githubusercontent.com/" + DefaultRepo + "/refs/" + trimmed + "/service_contracts/abi"
	}
	return "https://raw.githubusercontent.com/" + DefaultRepo + "/" + trimmed + "/service_contracts/abi"
}

func writeABI(rootDir, pkg string, entries []json.RawMessage) error {
	path := filepath.Join(rootDir, pkg, "abi.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tempFile, err := os.CreateTemp(filepath.Dir(path), ".abi-*.json")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}
