// Package txutil provides transaction lifecycle utilities for Ethereum/FEVM
// transactions.
//
// Key components:
//   - NonceManager: thread-safe nonce tracking with failure recovery
//   - Gas estimation: configurable gas limits with FEVM workarounds
//   - Receipt polling: exponential backoff receipt confirmation
package txutil
