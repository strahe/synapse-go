// Package synapse provides the root [Client] for the Filecoin Onchain Cloud
// (FOC) Go SDK.
//
// Create a client with [New], passing a private key and an RPC endpoint (or
// an existing [ethclient.Client]). The client auto-detects the chain, resolves
// contract addresses, and eagerly initialises all sub-services before returning.
//
//	client, err := synapse.New(ctx,
//	    synapse.WithPrivateKeyHex(os.Getenv("SYNAPSE_PRIVATE_KEY")),
//	    synapse.WithRPCURL(os.Getenv("RPC_URL")),
//	)
//	if err != nil { ... }
//	defer client.Close()
//
//	result, err := client.Storage().Upload(ctx, data, nil)
//
// Sub-services are accessed via getters: [Client.Storage], [Client.Payments],
// [Client.WarmStorage], [Client.SPRegistry], [Client.Costs], [Client.FilBeam],
// and [Client.SessionKey]. Each getter returns the service instance created by [New].
//
// Lower-level packages ([chain], [signer], [piece], [storage], [payments], etc.)
// can still be used independently without the root client.
//
// # Stability
//
// This SDK is in its 0.x phase. Public APIs may change between minor
// releases; breaking changes are called out in PR descriptions and
// release notes. Pin to a specific minor version in production. The
// reference implementation is the TypeScript SDK at
// https://github.com/FilOzone/synapse-sdk; behavioural divergences are
// either flagged in package docs or considered bugs.
package synapse
