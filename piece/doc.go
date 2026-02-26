// Package piece provides PieceCID utilities for Filecoin content addressing.
//
// This is a leaf package. It wraps IPFS CID operations with Filecoin-specific
// piece commitment (commp) calculation, merkle root extraction, and format
// conversion utilities.
//
// PieceCID format: uvarint padding | uint8 height | 32-byte merkle root.
// Smart contracts require the last 32 bytes (the merkle root) extracted
// from the full PieceCID.
package piece
