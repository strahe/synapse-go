// Package sessionkey provides SessionKeyRegistry authorization management on
// Filecoin EVM.
//
// Session keys are authorized by a root account with specific EIP-712
// permissions (e.g., CreateDataSet, AddPieces) and a time-bounded expiry. This
// package manages those on-chain authorizations and expiry checks. A root
// Client can remain the storage payer and direct transaction signer while a
// separately authorized key signs Storage EIP-712 messages.
//
// This is separate from the signer package because session keys represent
// a higher-level authorization concept, not just a signing primitive.
//
// # Permission values
//
// [CreateDataSetPermission], [AddPiecesPermission],
// [SchedulePieceRemovalsPermission], and [TerminateServicePermission] return
// independent [Permission] values. [DefaultFWSSPermissions] returns a new slice
// on each call. Changing these values or slices does not change SDK defaults.
//
// Nil permissions select all four standard permissions for login, revocation,
// and expiry queries. An explicit empty slice selects none. Pass a custom
// slice through [LoginOptions], [RevokeOptions], or [Service.GetExpirations]
// to choose permissions for an individual operation.
//
// # Migrating permission variables
//
// The standard permissions and DefaultFWSSPermissions are now functions.
// Replace variable reads with calls, for example AddPiecesPermission(). To
// take a permission's address or slice its bytes, first assign it to a local
// variable:
//
//	permission := AddPiecesPermission()
//	bytes := permission[:]
//	pointer := &permission
//
// # Stability
//
// 0.x phase: public API may change between minor releases.
package sessionkey
