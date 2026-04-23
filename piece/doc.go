// Package piece provides PieceCID utilities for Filecoin content addressing.
//
// This is a leaf package. It provides Filecoin-specific piece commitment (commp)
// calculation, CID format conversion, merkle root extraction, and padded size
// helpers.
//
// # CID formats
//
// PieceCIDv1 (codec=0xf101, multihash=sha2-256-trunc254-padded) is the
// traditional Filecoin piece commitment. It is used by smart contracts for
// on-chain piece verification.
//
// PieceCIDv2 (codec=0x55, multihash=fr32-sha256-trunc254-padbintree, per
// FRC-0069) encodes the original raw byte count inside its multihash digest.
// It is the format exposed at the upload/download API boundary so that the
// exact raw size is always carried losslessly without requiring callers to
// recompute it from the original data.
//
// # PieceInfo
//
// [Calculate] and [CalculateFromBytes] return a [PieceInfo] value that holds
// both CID formats together with the exact raw byte count (RawSize). Storage
// consumers can rely on PieceInfo directly without maintaining separate v1 /
// rawSize fields.
//
// CIDv2 requires a minimum raw payload of 127 bytes (the unpadded capacity of
// the smallest valid Filecoin piece, 128 padded bytes). For inputs smaller than
// 127 bytes CIDv2 is left as [cid.Undef] while CIDv1 and RawSize are still
// populated.
//
// # Other utilities
//
//   - [Validate] checks that a CID is a valid v1 piece commitment.
//   - [ExtractRoot] extracts the 32-byte merkle root from a v1 CID; smart
//     contracts receive this root for piece verification.
//   - [ParseV2] decodes a PieceCIDv2 back into a PieceInfo.
//   - [PieceInfoFromV1] reconstructs a PieceInfo from a stored v1 CID and raw size.
//   - [PaddedSize] computes the smallest power-of-two padded piece size for a
//     given raw byte count.
//
// # Stability
//
// 0.x phase: public API may change between minor releases. Mirrors the
// TS SDK piece utilities at synapse-sdk/packages/synapse-sdk/src/piece.
package piece
