package synapse_test

import (
	"context"
	"fmt"
	"log"

	synapse "github.com/strahe/synapse-go"
)

// Example shows the minimal setup for creating a synapse.Client connected to
// an RPC endpoint with a signing key.
func Example() {
	ctx := context.Background()

	client, err := synapse.New(ctx,
		synapse.WithRPCURL("https://api.calibration.node.glif.io/rpc/v1"),
		synapse.WithPrivateKeyHex("0x..."),
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	fmt.Println(client.Address())
}
