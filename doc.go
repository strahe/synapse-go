// Package synapse provides the Go SDK for Filecoin Onchain Cloud (FOC).
//
// The primary entry point is [New], which creates a [*Client] configured
// with functional options. The Client composes all services (storage,
// payments, warm storage, SP registry, FilBeam, costs) and provides
// a simple "golden path" for common operations as well as access to
// individual services for advanced use cases.
//
// # Quick Start
//
//	client, err := synapse.New(ctx,
//	    synapse.WithPrivateKey(key),
//	    synapse.WithRPCURL("https://api.calibration.node.glif.io"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer client.Close()
//
// See ARCHITECTURE.md and ROADMAP.md for detailed design documentation.
package synapse
