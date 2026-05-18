package upstream

import (
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

var tsSDKCommitRE = regexp.MustCompile(`^[a-f0-9]{40}$`)

func TestTSSDKBaselineRefFormat(t *testing.T) {
	t.Parallel()

	if !tsSDKCommitRE.MatchString(TSSDKRef) {
		t.Fatalf("TSSDKRef = %q, want 40-character lowercase hex commit", TSSDKRef)
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
