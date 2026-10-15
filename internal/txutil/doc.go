// Package txutil provides transaction lifecycle utilities for Ethereum/FEVM
// transactions.
//
// Key components:
//   - NonceManager: cancellable serialization of nonce allocation and submission
//   - Gas estimation: configurable gas limits with FEVM workarounds
//   - Receipt polling: bounded RPC retries and block confirmation checks
package txutil
