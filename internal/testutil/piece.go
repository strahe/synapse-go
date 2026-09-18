package testutil

import (
	"encoding/binary"
	"testing"

	commcid "github.com/filecoin-project/go-fil-commcid"
	"github.com/ipfs/go-cid"
	"github.com/multiformats/go-multicodec"
	mh "github.com/multiformats/go-multihash"

	"github.com/strahe/synapse-go/chain"
)

// PieceCIDv2WithRawSize returns a PieceCIDv2 for v1's root that encodes
// rawSize. Unlike the PieceCIDv2 constructors, it also encodes sizes below
// chain.MinUploadSize, which decoders accept but providers reject.
func PieceCIDv2WithRawSize(t testing.TB, v1 cid.Cid, rawSize uint64) cid.Cid {
	t.Helper()
	if rawSize >= chain.MinUploadSize {
		pieceCID, err := commcid.PieceCidV2FromV1(v1, rawSize)
		if err != nil {
			t.Fatalf("PieceCidV2FromV1(%d): %v", rawSize, err)
		}
		return pieceCID
	}
	root, err := commcid.CIDToDataCommitmentV1(v1)
	if err != nil {
		t.Fatalf("CIDToDataCommitmentV1: %v", err)
	}
	// A height-2 tree holds up to 127 payload bytes; padding records the rest.
	digest := binary.AppendUvarint(nil, chain.MinUploadSize-rawSize)
	digest = append(digest, 2)
	digest = append(digest, root...)
	encoded, err := mh.Encode(digest, uint64(multicodec.Fr32Sha256Trunc254Padbintree))
	if err != nil {
		t.Fatalf("encode PieceCIDv2 multihash: %v", err)
	}
	return cid.NewCidV1(cid.Raw, encoded)
}
