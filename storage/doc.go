// Package storage provides the multi-copy upload orchestration service.
//
// The central types are [*Service] (high-level upload/download operations),
// [*Context] (per-provider store/pull/commit/download operations), and
// [*ServiceResolver] (selection + dataset-reuse wiring against warmstorage and
// spregistry services).
//
// # Upload Flow
//
// The multi-copy upload follows a store → pull → commit pipeline:
//
//  1. Store: Upload data to the primary storage provider.
//  2. Pull: Secondary providers fetch data from the primary (SP-to-SP).
//  3. Commit: All providers call AddPieces on-chain with EIP-712 signatures.
//
// The Service handles orchestration of the full multi-copy flow, while
// ServiceResolver reuses provider-local datasets only when metadata matches
// exactly and the warmstorage-approved provider set intersects active PDP
// providers from spregistry.
//
// Downloads are validated as they stream so callers can keep io.Reader-style
// boundaries without skipping PieceCID verification. By default the HTTP
// download client refuses to dial loopback, link-local, or private (RFC1918 /
// ULA) addresses to guard against SSRF, and it ignores environment-variable
// proxies for the same reason; set [Options.AllowPrivateNetworks] when
// connecting to trusted private infrastructure, or provide [Options.HTTPClient]
// if you need explicit proxy control. Bound the number of bytes accepted per
// URL-based Service.Download call via [Options.DownloadMaxBytes];
// Context.Download (curio-backed) is not subject to this cap.
package storage
