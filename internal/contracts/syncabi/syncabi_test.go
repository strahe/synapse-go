package syncabi

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestSyncWritesMergedABIs(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/service_contracts/abi/Errors.abi.json":
			_, _ = w.Write([]byte(`[{"type":"error","name":"Boom","inputs":[]}]`))
		case "/service_contracts/abi/FilecoinWarmStorageService.abi.json":
			_, _ = w.Write([]byte(`[{"type":"function","name":"store","inputs":[],"outputs":[],"stateMutability":"nonpayable"}]`))
		case "/service_contracts/abi/ServiceProviderRegistry.abi.json":
			_, _ = w.Write([]byte(`[{"type":"function","name":"register","inputs":[],"outputs":[],"stateMutability":"nonpayable"}]`))
		case "/service_contracts/abi/FilecoinPayV1.abi.json":
			_, _ = w.Write([]byte(`[]`))
		case "/service_contracts/abi/PDPVerifier.abi.json":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	cfg := Config{
		BaseURL: server.URL + "/service_contracts/abi",
		Ref:     "test-ref",
		RootDir: root,
		Client:  server.Client(),
	}

	if err := Sync(context.Background(), cfg); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	fwssData, err := os.ReadFile(filepath.Join(root, "fwss", "abi.json"))
	if err != nil {
		t.Fatalf("read fwss abi: %v", err)
	}
	if !bytes.Contains(fwssData, []byte(`"name": "store"`)) {
		t.Fatalf("fwss abi missing primary contract entry: %s", fwssData)
	}
	if !bytes.Contains(fwssData, []byte(`"name": "Boom"`)) {
		t.Fatalf("fwss abi missing merged errors entry: %s", fwssData)
	}

	spregistryData, err := os.ReadFile(filepath.Join(root, "spregistry", "abi.json"))
	if err != nil {
		t.Fatalf("read spregistry abi: %v", err)
	}
	if !bytes.Contains(spregistryData, []byte(`"name": "register"`)) {
		t.Fatalf("spregistry abi missing primary contract entry: %s", spregistryData)
	}
	if !bytes.Contains(spregistryData, []byte(`"name": "Boom"`)) {
		t.Fatalf("spregistry abi missing merged errors entry: %s", spregistryData)
	}
}

func TestDefaultBaseURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "commit hash stays unchanged",
			ref:  "ed85348ebad54196b5bfefc5cb0dbe7e8bfd6f7c",
			want: "https://raw.githubusercontent.com/FilOzone/filecoin-services/ed85348ebad54196b5bfefc5cb0dbe7e8bfd6f7c/service_contracts/abi",
		},
		{
			name: "tag ref gets refs prefix",
			ref:  "tags/v1.2.0",
			want: "https://raw.githubusercontent.com/FilOzone/filecoin-services/refs/tags/v1.2.0/service_contracts/abi",
		},
		{
			name: "branch head gets refs prefix",
			ref:  "heads/main",
			want: "https://raw.githubusercontent.com/FilOzone/filecoin-services/refs/heads/main/service_contracts/abi",
		},
		{
			name: "fully qualified ref is preserved",
			ref:  "refs/heads/main",
			want: "https://raw.githubusercontent.com/FilOzone/filecoin-services/refs/heads/main/service_contracts/abi",
		},
		{
			name: "bare branch name uses bare path",
			ref:  "main",
			want: "https://raw.githubusercontent.com/FilOzone/filecoin-services/main/service_contracts/abi",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DefaultBaseURL(tc.ref)
			if got != tc.want {
				t.Fatalf("DefaultBaseURL(%q) = %q, want %q", tc.ref, got, tc.want)
			}
		})
	}
}

func TestSyncUsesRefDerivedBaseURLWhenBaseURLEmpty(t *testing.T) {
	t.Parallel()

	var gotURLs []string
	client := &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotURLs = append(gotURLs, req.URL.String())

			body := "[]"
			if strings.HasSuffix(req.URL.Path, "/Errors.abi.json") {
				body = `[{"type":"error","name":"Boom","inputs":[]}]`
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       io.NopCloser(strings.NewReader(body)),
				Header:     make(http.Header),
			}, nil
		}),
	}

	if err := Sync(context.Background(), Config{
		Ref:     "tags/v1.2.0",
		RootDir: t.TempDir(),
		Client:  client,
	}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}

	if len(gotURLs) == 0 {
		t.Fatal("Sync() did not issue any requests")
	}
	if !strings.HasPrefix(gotURLs[0], "https://raw.githubusercontent.com/FilOzone/filecoin-services/refs/tags/v1.2.0/service_contracts/abi/") {
		t.Fatalf("first request URL = %q", gotURLs[0])
	}
}

func TestSyncDoesNotWriteAnyFilesWhenFetchFails(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	existingFWSS := []byte("[{\"name\":\"old-fwss\"}]\n")
	existingSPRegistry := []byte("[{\"name\":\"old-spregistry\"}]\n")

	for path, content := range map[string][]byte{
		filepath.Join(root, "fwss", "abi.json"):       existingFWSS,
		filepath.Join(root, "spregistry", "abi.json"): existingSPRegistry,
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/service_contracts/abi/Errors.abi.json":
			_, _ = w.Write([]byte(`[{"type":"error","name":"Boom","inputs":[]}]`))
		case "/service_contracts/abi/FilecoinWarmStorageService.abi.json":
			_, _ = w.Write([]byte(`[{"type":"function","name":"store","inputs":[],"outputs":[],"stateMutability":"nonpayable"}]`))
		case "/service_contracts/abi/ServiceProviderRegistry.abi.json":
			_, _ = w.Write([]byte(`[{"type":"function","name":"register","inputs":[],"outputs":[],"stateMutability":"nonpayable"}]`))
		case "/service_contracts/abi/FilecoinPayV1.abi.json":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "/service_contracts/abi/PDPVerifier.abi.json":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	err := Sync(context.Background(), Config{
		BaseURL: server.URL + "/service_contracts/abi",
		Ref:     "test-ref",
		RootDir: root,
		Client:  server.Client(),
	})
	if err == nil {
		t.Fatal("Sync() error = nil, want fetch failure")
	}

	gotFWSS, err := os.ReadFile(filepath.Join(root, "fwss", "abi.json"))
	if err != nil {
		t.Fatalf("read fwss abi: %v", err)
	}
	if !bytes.Equal(gotFWSS, existingFWSS) {
		t.Fatalf("fwss abi changed on failed sync: got %s want %s", gotFWSS, existingFWSS)
	}

	gotSPRegistry, err := os.ReadFile(filepath.Join(root, "spregistry", "abi.json"))
	if err != nil {
		t.Fatalf("read spregistry abi: %v", err)
	}
	if !bytes.Equal(gotSPRegistry, existingSPRegistry) {
		t.Fatalf("spregistry abi changed on failed sync: got %s want %s", gotSPRegistry, existingSPRegistry)
	}
}
