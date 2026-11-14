// Package upstream records pinned upstream references.
package upstream

const (
	// TSSDKRepo is the upstream TypeScript SDK repository.
	TSSDKRepo = "FilOzone/synapse-sdk"
	// TSSDKLocalDir is the local TypeScript SDK checkout path.
	TSSDKLocalDir = "synapse-sdk"
	// TSSDKRef is the pinned TypeScript SDK commit for synapse-sdk v3.0.0,
	// which pins Filecoin services v1.5.0.
	TSSDKRef = "21778dcc88219371ce128f89d15776cfeab1bbe9"
	// FilecoinServicesRepo is the upstream contract ABI repository.
	FilecoinServicesRepo = "FilOzone/filecoin-services"
	// FilecoinServicesRef is the pinned contract ABI commit.
	FilecoinServicesRef = "649e23f3329abf6a24d01c30359958ee7d0e0948"
)
