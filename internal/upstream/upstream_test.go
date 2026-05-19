package upstream

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var (
	gitCommitRE   = regexp.MustCompile(`^[a-f0-9]{40}$`)
	semverRE      = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
	wagmiGitRefRE = regexp.MustCompile(`(?m)^\s*(?:export\s+)?const\s+GIT_REF\s*=\s*['"]([^'"]+)['"]`)
)

func TestTSSDKBaselineRefFormat(t *testing.T) {
	t.Parallel()

	if !gitCommitRE.MatchString(TSSDKRef) {
		t.Fatalf("TSSDKRef = %q, want 40-character lowercase hex commit", TSSDKRef)
	}
	if !gitCommitRE.MatchString(FilecoinServicesRef) {
		t.Fatalf("FilecoinServicesRef = %q, want 40-character lowercase hex commit", FilecoinServicesRef)
	}
}

func TestLocalTSSDKBaseline(t *testing.T) {
	if os.Getenv("CHECK_TS_SDK_BASELINE") != "1" {
		t.Skip("set CHECK_TS_SDK_BASELINE=1 to check the local TS SDK checkout")
	}

	root := repoRoot(t)
	sdkPath := filepath.Join(root, TSSDKLocalDir)
	actual, err := gitHead(sdkPath)
	if err != nil {
		t.Fatalf("local TS SDK baseline mismatch: repo=%s path=%s expected=%s actual=unavailable error=%v", TSSDKRepo, sdkPath, TSSDKRef, err)
	}
	if actual != TSSDKRef {
		t.Fatalf("local TS SDK baseline mismatch: repo=%s path=%s expected=%s actual=%s", TSSDKRepo, sdkPath, TSSDKRef, actual)
	}

	sdkPackage := readPackageJSON(t, filepath.Join(sdkPath, "packages", "synapse-sdk", "package.json"))
	if sdkPackage.Name != "@filoz/synapse-sdk" {
		t.Fatalf("synapse-sdk package name = %q, want @filoz/synapse-sdk", sdkPackage.Name)
	}
	if !semverRE.MatchString(sdkPackage.Version) {
		t.Fatalf("synapse-sdk package version = %q, want valid semver", sdkPackage.Version)
	}
	coreDep := sdkPackage.Dependencies["@filoz/synapse-core"]
	if coreDep == "" {
		t.Fatal("synapse-sdk package is missing @filoz/synapse-core dependency")
	}
	if !strings.HasPrefix(coreDep, "workspace:") {
		t.Fatalf("synapse-sdk @filoz/synapse-core dependency = %q, want workspace: dependency in local source checkout", coreDep)
	}

	corePackage := readPackageJSON(t, filepath.Join(sdkPath, "packages", "synapse-core", "package.json"))
	if corePackage.Name != "@filoz/synapse-core" {
		t.Fatalf("synapse-core package name = %q, want @filoz/synapse-core", corePackage.Name)
	}
	if !semverRE.MatchString(corePackage.Version) {
		t.Fatalf("synapse-core package version = %q, want valid semver", corePackage.Version)
	}
	t.Logf("local TS baseline packages: %s@%s depends on %s@%s via %s", sdkPackage.Name, sdkPackage.Version, corePackage.Name, corePackage.Version, coreDep)

	wagmiPath := filepath.Join(sdkPath, "packages", "synapse-core", "wagmi.config.ts")
	wagmiData, err := os.ReadFile(wagmiPath)
	if err != nil {
		t.Fatalf("read %s: %v", wagmiPath, err)
	}
	wagmiConfig := string(wagmiData)
	matches := wagmiGitRefRE.FindStringSubmatch(wagmiConfig)
	if len(matches) != 2 {
		t.Fatalf("locate GIT_REF in %s", wagmiPath)
	}
	if matches[1] != FilecoinServicesRef {
		t.Fatalf("synapse-core wagmi GIT_REF = %q, want %s", matches[1], FilecoinServicesRef)
	}
	wantBaseURL := "const BASE_URL = `https://raw.githubusercontent.com/" + FilecoinServicesRepo + "/${GIT_REF"
	if !strings.Contains(wagmiConfig, wantBaseURL) || !strings.Contains(wagmiConfig, "}/service_contracts/abi`") {
		t.Fatalf("synapse-core wagmi BASE_URL must derive raw.githubusercontent.com/%s from GIT_REF", FilecoinServicesRepo)
	}
}

type packageJSON struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
}

func readPackageJSON(t *testing.T, path string) packageJSON {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return pkg
}

func TestGitHeadIncludesCommandOutputOnError(t *testing.T) {
	const stderr = "fatal: baseline repo missing"

	withFakeGit(t, "#!/bin/sh\nprintf '%s\\n' '"+stderr+"' >&2\nexit 128\n")
	_, err := gitHead(t.TempDir())
	if err == nil {
		t.Fatal("gitHead() error = nil, want error")
	}
	if !strings.Contains(err.Error(), stderr) {
		t.Fatalf("gitHead() error = %q, want %q", err, stderr)
	}
}

func TestGitHeadIgnoresStderrOnSuccess(t *testing.T) {
	withFakeGit(t, "#!/bin/sh\nprintf '%s\\n' '"+TSSDKRef+"'\nprintf '%s\\n' 'warning: ignored stderr' >&2\n")

	actual, err := gitHead(t.TempDir())
	if err != nil {
		t.Fatalf("gitHead() error = %v", err)
	}
	if actual != TSSDKRef {
		t.Fatalf("gitHead() = %q, want %q", actual, TSSDKRef)
	}
}

func withFakeGit(t *testing.T, script string) {
	t.Helper()

	binDir := t.TempDir()
	gitPath := filepath.Join(binDir, "git")
	if err := os.WriteFile(gitPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate repo root: runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func gitHead(dir string) (string, error) {
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := strings.TrimSpace(string(exitErr.Stderr))
			if stderr != "" {
				return "", fmt.Errorf("git rev-parse HEAD failed: %w: %s", err, stderr)
			}
		}
		return "", fmt.Errorf("git rev-parse HEAD failed: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
