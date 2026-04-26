package integrationtest

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/strahe/synapse-go/piece"
	"github.com/strahe/synapse-go/storage"
)

func TestNewClientDefaultsAllowPrivateNetworkDownloads(t *testing.T) {
	t.Setenv(EnvRPCURL, fakeChainRPCServer(t).URL)

	client := NewClient(t, context.Background(), generateTestPrivateKeyHex(t))

	data := bytes.Repeat([]byte("private-network"), 32)
	info, err := piece.CalculateFromBytes(data)
	if err != nil {
		t.Fatalf("CalculateFromBytes: %v", err)
	}
	download := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	defer download.Close()

	reader, err := client.Storage().Download(context.Background(), info.CIDv2, &storage.DownloadOptions{URL: download.URL})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer func() { _ = reader.Close() }()
	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatal("downloaded bytes mismatch")
	}
}

func generateTestPrivateKeyHex(t *testing.T) string {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return "0x" + hex.EncodeToString(crypto.FromECDSA(key))
}

func fakeChainRPCServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode JSON-RPC request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if req.Method != "eth_chainId" {
			t.Errorf("unexpected JSON-RPC method %q", req.Method)
			http.Error(w, "unexpected method", http.StatusBadRequest)
			return
		}
		id := req.ID
		if len(id) == 0 {
			id = []byte("null")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":"0x4cb2f"}`, id)
	}))
	t.Cleanup(server.Close)
	return server
}
