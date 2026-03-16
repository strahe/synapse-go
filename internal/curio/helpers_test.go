package curio

import "github.com/ipfs/go-cid"

// emptyCID returns an undefined (zero-value) CID for testing error paths.
func emptyCID() cid.Cid {
	return cid.Cid{}
}
