// Package upstream records pinned upstream references.
package upstream

const (
	// TSSDKRepo is the upstream TypeScript SDK repository.
	TSSDKRepo = "FilOzone/synapse-sdk"
	// TSSDKLocalDir is the local TypeScript SDK checkout path.
	TSSDKLocalDir = "synapse-sdk"
	// TSSDKRef is the pinned TypeScript SDK commit for synapse-sdk v2.0.2
	// with the final Filecoin services v1.4.0 deployment snapshot.
	TSSDKRef = "8f47ccd01dfef28a51697c96d98f4d1cab665aed"
	// FilecoinServicesRepo is the upstream contract ABI repository.
	FilecoinServicesRepo = "FilOzone/filecoin-services"
	// FilecoinServicesRef is the pinned contract ABI commit.
	FilecoinServicesRef = "d53b16b4cd258c11f5c18ea1432958511426bdc1"
)
