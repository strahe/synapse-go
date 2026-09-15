// Package upstream records pinned upstream references.
package upstream

const (
	// TSSDKRepo is the upstream TypeScript SDK repository.
	TSSDKRepo = "FilOzone/synapse-sdk"
	// TSSDKLocalDir is the local TypeScript SDK checkout path.
	TSSDKLocalDir = "synapse-sdk"
	// TSSDKRef is the pinned TypeScript SDK commit for synapse-sdk v2.0.0
	// with the final Filecoin services v1.4.0 deployment snapshot.
	TSSDKRef = "44fecae5af68754bae62be29dbaba89f3fc844a2"
	// FilecoinServicesRepo is the upstream contract ABI repository.
	FilecoinServicesRepo = "FilOzone/filecoin-services"
	// FilecoinServicesRef is the pinned contract ABI commit.
	FilecoinServicesRef = "d53b16b4cd258c11f5c18ea1432958511426bdc1"
)
