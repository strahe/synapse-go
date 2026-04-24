package integrationtest

import (
	"context"
	"testing"

	synapse "github.com/strahe/synapse-go"
)

// NewClient builds a synapse.Client using the supplied private key hex
// and the shared RPC URL. The client is automatically closed via
// t.Cleanup; a failed dial is reported with t.Fatalf so callers can
// treat the returned value as non-nil.
func NewClient(t *testing.T, ctx context.Context, privateKeyHex string) *synapse.Client {
	t.Helper()
	client, err := synapse.New(ctx,
		synapse.WithPrivateKeyHex(privateKeyHex),
		synapse.WithRPCURL(RPCURL()),
	)
	if err != nil {
		t.Fatalf("integrationtest: synapse.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// NewDefaultClient resolves INTEGRATION_PRIVATE_KEY (skipping when
// absent) and returns a ready client.
func NewDefaultClient(t *testing.T, ctx context.Context) *synapse.Client {
	t.Helper()
	return NewClient(t, ctx, RequirePrivateKey(t))
}
