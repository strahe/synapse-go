package integrationtest

import (
	"os"
	"path/filepath"
	"testing"
)

// Env var names and defaults shared by integration tests.
const (
	EnvPrivateKey      = "INTEGRATION_PRIVATE_KEY"
	EnvDestructiveKey  = "INTEGRATION_DESTRUCTIVE_KEY"
	EnvRPCURL          = "INTEGRATION_RPC_URL"
	DefaultRPCURL      = "https://api.calibration.node.glif.io/rpc/v1"
	CalibrationChainID = 314159
)

// EnsureEnv loads the module-root .env (if present). It anchors the search
// at the directory containing go.mod so the helper works regardless of which
// package `go test` is invoked from, without accidentally loading a .env
// from a parent workspace that belongs to an unrelated project. Safe to
// call multiple times.
func EnsureEnv(t *testing.T) {
	t.Helper()
	root, err := findModuleRoot()
	if err != nil {
		return
	}
	path := filepath.Join(root, ".env")
	if _, err := os.Stat(path); err != nil {
		return
	}
	if err := LoadDotEnv(path); err != nil {
		t.Logf("integrationtest: load %s: %v", path, err)
	}
}

// findModuleRoot walks up from the current working directory until it finds
// a go.mod file and returns that directory. Returns an error if the walk
// reaches the filesystem root without finding one.
func findModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// RequirePrivateKey returns the integration signing key, skipping the
// test when the env var is absent.
func RequirePrivateKey(t *testing.T) string {
	t.Helper()
	EnsureEnv(t)
	key := os.Getenv(EnvPrivateKey)
	if key == "" {
		t.Skipf("%s not set; skipping integration test", EnvPrivateKey)
	}
	return key
}

// DestructiveKey returns (true, key) only when the caller supplied a
// dedicated destructive-test private key. Callers should t.Skip with a
// "needs-destructive-account" reason when ok==false.
func DestructiveKey(t *testing.T) (bool, string) {
	t.Helper()
	EnsureEnv(t)
	key := os.Getenv(EnvDestructiveKey)
	return key != "", key
}

// RPCURL returns the configured RPC endpoint, falling back to the
// Calibration default.
func RPCURL() string {
	if v := os.Getenv(EnvRPCURL); v != "" {
		return v
	}
	return DefaultRPCURL
}
