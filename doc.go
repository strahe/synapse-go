// Package synapse provides the root [Client] for the Filecoin Onchain Cloud
// (FOC) Go SDK.
//
// Create a client with [New], passing a private key and an RPC endpoint (or
// an existing [ethclient.Client]). The client auto-detects the chain, resolves
// contract addresses, and eagerly initialises all sub-services before returning.
//
//	client, err := synapse.New(ctx,
//	    synapse.WithPrivateKeyHex("0x..."),
//	    synapse.WithRPCURL("https://api.calibration.node.glif.io/rpc/v1"),
//	    synapse.WithSource("my-app"),
//	)
//	if err != nil {
//	    // handle error
//	}
//	defer client.Close()
//
//	// data must contain uploadable content; PieceCIDv2 requires at least
//	// 127 raw bytes.
//	result, err := client.Storage().Upload(ctx, data, &storage.UploadOptions{Copies: 2})
//
// [WithStorageSigner] can delegate Storage EIP-712 authorization signatures to
// an authorized key while the root key continues to determine [Client.Address]
// and remains the payer and transaction signer. Authorize the delegated address
// through [Client.SessionKey] before the first storage write; [New] does not
// check that authorization during client construction.
//
// Sub-services are accessed via getters: [Client.Storage], [Client.Payments],
// [Client.WarmStorage], [Client.SPRegistry], [Client.Costs], [Client.FilBeam],
// and [Client.SessionKey]. Each getter returns the service instance created by [New].
// [Client.ResolvedAddresses] returns the address snapshot used by those
// services. Read-only callers can use [ResolveAddresses] directly without a
// private key.
//
// # HTTP security
//
// HTTP clients assembled by [New] reject private and reserved network
// destinations by default. Match rejected requests with [ErrPrivateNetwork].
// Use [WithAllowPrivateNetworks] only for trusted private infrastructure.
// Supplying [WithHTTPClient] replaces these safeguards with the caller's
// transport policy. These root-client defaults do not change standalone
// [pdp.New] or [filbeam.New] clients.
//
// Lower-level packages ([chain], [signer], [piece], [storage], [payments], etc.)
// can still be used independently without the root client.
//
// # Stability
//
// This SDK is in its 0.x phase. Public APIs may change between minor
// releases; breaking changes are called out in release notes. Pin to a
// specific minor version in production. The implementation tracks the
// Filecoin Onchain Cloud protocol.
//
// [piece]: https://pkg.go.dev/github.com/strahe/synapse-go/piece
package synapse
