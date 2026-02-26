// Package typeddata implements EIP-712 typed data signing for storage
// provider authentication.
//
// Supported message types:
//   - CreateDataSet: authorize dataset creation on a provider
//   - AddPieces: authorize adding pieces to a dataset
//   - DeleteDataSet: authorize dataset deletion
//   - SchedulePieceRemovals: authorize piece removal scheduling
//
// Domain: FilecoinWarmStorageService with chain-specific separation.
package typeddata
