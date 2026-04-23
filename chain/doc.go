// Package chain defines chain identifiers, contract addresses, and epoch
// utilities for Filecoin networks (Mainnet, Calibration, Devnet).
//
// This is a leaf package with no dependencies on other synapse-go packages.
// All chain-specific constants (chain IDs, contract addresses, genesis
// timestamps) are defined here to keep them in a single source of truth.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Contract
// addresses track the canonical FOC deployments and may be updated when
// upstream contracts redeploy.
package chain
