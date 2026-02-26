// Package storage provides the multi-copy upload orchestration service.
//
// The central types are [*Manager] (high-level upload/download operations)
// and [*Context] (per-provider store/pull/commit operations).
//
// # Upload Flow
//
// The multi-copy upload follows a store → pull → commit pipeline:
//
//  1. Store: Upload data to the primary storage provider.
//  2. Pull: Secondary providers fetch data from the primary (SP-to-SP).
//  3. Commit: All providers call AddPieces on-chain with EIP-712 signatures.
//
// The Manager handles provider selection, context creation, and orchestration
// of the full multi-copy flow with configurable callbacks for progress tracking.
package storage
