package typeddata

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// FuzzNewDomain ensures the EIP-712 domain builder is total over arbitrary
// chainID magnitudes and 20-byte FWSS addresses, and produces a stable
// representation (same inputs → same domain field values).
func FuzzNewDomain(f *testing.F) {
	f.Add(int64(1), []byte("0123456789abcdef0123"))
	f.Add(int64(0), make([]byte, 20))
	f.Add(int64(-1), make([]byte, 20))
	f.Add(int64(314_159), []byte("\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff\xff"))

	f.Fuzz(func(t *testing.T, chainID int64, addrBytes []byte) {
		// Pad/truncate to 20 bytes for an EVM address.
		var a common.Address
		switch {
		case len(addrBytes) >= 20:
			copy(a[:], addrBytes[:20])
		default:
			copy(a[:], addrBytes)
		}
		cid := big.NewInt(chainID)
		d1 := NewDomain(cid, a)
		d2 := NewDomain(cid, a)
		if d1.Name != d2.Name || d1.Version != d2.Version {
			t.Fatalf("NewDomain not deterministic: %+v vs %+v", d1, d2)
		}
		if (d1.ChainId == nil) != (d2.ChainId == nil) {
			t.Fatalf("NewDomain chainID nilness inconsistent")
		}
		if d1.ChainId != nil && (*big.Int)(d1.ChainId).Cmp((*big.Int)(d2.ChainId)) != 0 {
			t.Fatalf("NewDomain chainID mismatch: %v vs %v", d1.ChainId, d2.ChainId)
		}
		if d1.VerifyingContract != d2.VerifyingContract {
			t.Fatalf("NewDomain verifyingContract mismatch")
		}
	})
}

// FuzzCreateDataSetMessage ensures the message builder is total over
// arbitrary metadata key/value pairs.
func FuzzCreateDataSetMessage(f *testing.F) {
	f.Add(int64(0), "", "")
	f.Add(int64(1), "k", "v")
	f.Add(int64(-1), "with-non-ascii-\x00\xff", "value")

	f.Fuzz(func(t *testing.T, clientDSID int64, k, v string) {
		var payee common.Address
		md := []MetadataEntry{{Key: k, Value: v}}
		_ = CreateDataSetMessage(big.NewInt(clientDSID), payee, md)
	})
}
