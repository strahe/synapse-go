//go:build integration

package integration_test

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// loadEnvFile reads a KEY=VALUE file and sets any variables not already
// present in the environment. Lines starting with # and empty lines are
// skipped. Values may optionally be quoted with double quotes.
func loadEnvFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil // .env file is optional
		}
		return err
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// Strip optional surrounding double-quotes.
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		// Only set if not already present in environment.
		if _, ok := os.LookupEnv(k); !ok {
			_ = os.Setenv(k, v)
		}
	}
	return sc.Err()
}
