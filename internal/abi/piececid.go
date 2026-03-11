package abi

import (
	"fmt"

	"github.com/ipfs/go-cid"

	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
)

// EncodePieceCID converts a cid.Cid to the on-chain Cids.Cid struct shape
// (the bindings alias it as CidsCid). The stored bytes are the raw CID
// byte representation (cid.Bytes()).
func EncodePieceCID(c cid.Cid) pdpverifier.CidsCid {
	if !c.Defined() {
		return pdpverifier.CidsCid{Data: nil}
	}
	b := c.Bytes()
	out := make([]byte, len(b))
	copy(out, b)
	return pdpverifier.CidsCid{Data: out}
}

// DecodePieceCID parses a CidsCid back into a cid.Cid.
func DecodePieceCID(raw pdpverifier.CidsCid) (cid.Cid, error) {
	if len(raw.Data) == 0 {
		return cid.Undef, fmt.Errorf("abi.DecodePieceCID: empty bytes")
	}
	c, err := cid.Cast(raw.Data)
	if err != nil {
		return cid.Undef, fmt.Errorf("abi.DecodePieceCID: %w", err)
	}
	return c, nil
}

// EncodePieceCIDs batches EncodePieceCID over a slice.
func EncodePieceCIDs(cids []cid.Cid) []pdpverifier.CidsCid {
	out := make([]pdpverifier.CidsCid, len(cids))
	for i, c := range cids {
		out[i] = EncodePieceCID(c)
	}
	return out
}

// DecodePieceCIDs batches DecodePieceCID over a slice. Returns the first
// decoding error encountered.
func DecodePieceCIDs(raw []pdpverifier.CidsCid) ([]cid.Cid, error) {
	out := make([]cid.Cid, len(raw))
	for i, r := range raw {
		c, err := DecodePieceCID(r)
		if err != nil {
			return nil, fmt.Errorf("abi.DecodePieceCIDs[%d]: %w", i, err)
		}
		out[i] = c
	}
	return out, nil
}
