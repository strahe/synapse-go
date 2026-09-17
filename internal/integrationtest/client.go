package integrationtest

import (
	"context"
	"testing"

	synapse "github.com/strahe/synapse-go"
)

// NewClient builds a synapse.Client using the supplied private key hex and the
// shared RPC URL. Provider HTTP allows private and reserved destinations so
// calibration tests can reach public providers when local DNS or a transparent
// proxy maps their hostnames onto intercepted addresses such as 198.18.0.0/15.
// Pass synapse.WithAllowPrivateNetworks(false) to restore the SDK default
// denial. Upload batching is disabled unless opts re-enable it. The client is
// closed via t.Cleanup; a failed dial is a fatal test error so callers can treat
// the returned value as non-nil.
func NewClient(t *testing.T, ctx context.Context, privateKeyHex string, opts ...synapse.ClientOption) *synapse.Client {
	t.Helper()
	clientOpts := []synapse.ClientOption{
		synapse.WithPrivateKeyHex(privateKeyHex),
		synapse.WithRPCURL(RPCURL()),
		synapse.WithAllowPrivateNetworks(true),
		synapse.WithoutUploadBatching(),
	}
	clientOpts = append(clientOpts, opts...)
	client, err := synapse.New(ctx, clientOpts...)
	if err != nil {
		t.Fatalf("integrationtest: synapse.New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// NewDefaultClient resolves INTEGRATION_PRIVATE_KEY (skipping when absent) and
// returns a ready client. Upload batching is disabled unless opts re-enable it,
// keeping existing integration flows immediate and independently timed. Private-
// network HTTP follows NewClient so later opts can still restore the SDK default.
func NewDefaultClient(t *testing.T, ctx context.Context, opts ...synapse.ClientOption) *synapse.Client {
	t.Helper()
	return NewClient(t, ctx, RequirePrivateKey(t), opts...)
}
