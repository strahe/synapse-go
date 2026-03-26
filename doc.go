// Package synapse provides the root [Client] for the Filecoin Onchain Cloud
// (FOC) Go SDK.
//
// Create a client with [New], passing a private key and an RPC endpoint (or
// an existing [ethclient.Client]). The client auto-detects the chain, resolves
// contract addresses, and provides lazy-initialized access to every sub-service.
//
//	client, err := synapse.New(ctx,
//	    synapse.WithPrivateKeyHex(os.Getenv("PRIVATE_KEY")),
//	    synapse.WithRPCURL(os.Getenv("RPC_URL")),
//	)
//	if err != nil { ... }
//	defer client.Close()
//
//	result, err := client.Storage().Upload(ctx, data, nil)
//
// Sub-services are accessed via getters: [Client.Storage], [Client.Payments],
// [Client.WarmStorage], [Client.SPRegistry], [Client.Costs], [Client.FilBeam],
// and [Client.SessionKey]. Each getter initializes its service on first call
// and returns the same instance thereafter.
//
// Lower-level packages ([chain], [signer], [piece], [storage], [payments], etc.)
// can still be used independently without the root client.
package synapse
