// Package sessionkey provides session key management for delegated signing.
//
// Session keys allow applications to sign storage operations without
// exposing the main wallet private key. A session key is authorized by
// the root account and can perform a limited set of operations on its behalf.
//
// This is separate from the signer package because session keys represent
// a higher-level authorization concept, not just a signing primitive.
package sessionkey
