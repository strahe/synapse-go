// Package curio provides the HTTP client for the Curio storage provider
// API (PDP endpoints).
//
// Key operations:
//   - Store: upload piece data to a storage provider
//   - Pull: trigger SP-to-SP data transfer
//   - Commit: on-chain piece confirmation
//   - Query: check pull status, list pieces, manage data sets
//
// All HTTP requests include EIP-712 authentication headers.
package curio
