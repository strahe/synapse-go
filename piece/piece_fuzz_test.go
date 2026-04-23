package piece

import (
	"testing"

	"github.com/ipfs/go-cid"
)

// FuzzValidate exercises Validate with arbitrary byte sequences passed
// through cid.Cast. Cast may reject the bytes outright; what we care about
// is that neither Cast nor Validate panics.
func FuzzValidate(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte("not-a-cid"))
	// Valid CIDv0 and CIDv1 prefixes (smallest known shapes).
	f.Add([]byte{0x01, 0x55, 0x12, 0x20})
	// Long, mostly-zero buffer.
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, raw []byte) {
		c, err := cid.Cast(raw)
		if err != nil {
			return
		}
		_ = Validate(c)
	})
}

// FuzzCalculateFromBytes fuzzes the piece commitment calculator against
// arbitrary byte inputs. It asserts the function never panics; errors are
// expected for degenerate inputs.
func FuzzCalculateFromBytes(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte("hello"))
	f.Add(make([]byte, 1024))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Cap input size to keep fuzz runs cheap; commP padding has known
		// growth bounds.
		if len(data) > 64*1024 {
			data = data[:64*1024]
		}
		_, _ = CalculateFromBytes(data)
	})
}
