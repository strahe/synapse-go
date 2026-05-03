package types

import (
	"bytes"
	"fmt"
	"math/big"
	"strconv"
)

var uint256Limit = new(big.Int).Lsh(big.NewInt(1), 256)

// BigInt holds a uint256 value used by on-chain identifiers.
type BigInt struct {
	n *big.Int
}

// NewBigInt returns a BigInt from a uint64 value.
func NewBigInt(v uint64) BigInt {
	return BigInt{n: new(big.Int).SetUint64(v)}
}

// BigIntFromBig returns a BigInt after validating that v is a uint256.
func BigIntFromBig(v *big.Int) (BigInt, error) {
	if v == nil {
		return BigInt{}, fmt.Errorf("bigint: nil")
	}
	if v.Sign() < 0 {
		return BigInt{}, fmt.Errorf("bigint: negative: %s", v.String())
	}
	if v.Cmp(uint256Limit) >= 0 {
		return BigInt{}, fmt.Errorf("bigint: exceeds uint256: %s", v.String())
	}
	return BigInt{n: new(big.Int).Set(v)}, nil
}

// ParseBigInt parses a decimal uint256.
func ParseBigInt(s string) (BigInt, error) {
	if s == "" {
		return BigInt{}, fmt.Errorf("bigint: empty")
	}
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return BigInt{}, fmt.Errorf("bigint: invalid decimal: %q", s)
		}
	}
	v, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return BigInt{}, fmt.Errorf("bigint: invalid decimal: %q", s)
	}
	return BigIntFromBig(v)
}

// Big returns a defensive copy of id.
func (id BigInt) Big() *big.Int {
	if id.n == nil {
		return new(big.Int)
	}
	return new(big.Int).Set(id.n)
}

// Copy returns an independent copy of id.
func (id BigInt) Copy() BigInt {
	if id.n == nil {
		return BigInt{}
	}
	return BigInt{n: new(big.Int).Set(id.n)}
}

// Bytes32 returns id as a 32-byte big-endian uint256 value.
func (id BigInt) Bytes32() [32]byte {
	var out [32]byte
	if id.n != nil {
		id.n.FillBytes(out[:])
	}
	return out
}

// String returns the decimal form of id.
func (id BigInt) String() string {
	if id.n == nil {
		return "0"
	}
	return id.n.String()
}

// IsZero reports whether id is zero.
func (id BigInt) IsZero() bool {
	return id.n == nil || id.n.Sign() == 0
}

// Uint64 returns id as uint64 when it fits.
func (id BigInt) Uint64() (uint64, bool) {
	if id.n == nil {
		return 0, true
	}
	if !id.n.IsUint64() {
		return 0, false
	}
	return id.n.Uint64(), true
}

// Equal reports whether id and other hold the same numeric value.
func (id BigInt) Equal(other BigInt) bool {
	return id.Cmp(other) == 0
}

// Cmp compares id and other.
func (id BigInt) Cmp(other BigInt) int {
	switch {
	case id.n == nil && other.n == nil:
		return 0
	case id.n == nil:
		if other.n.Sign() == 0 {
			return 0
		}
		return -1
	case other.n == nil:
		return id.n.Sign()
	default:
		return id.n.Cmp(other.n)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (id BigInt) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *BigInt) UnmarshalText(text []byte) error {
	parsed, err := ParseBigInt(string(text))
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}

// MarshalJSON implements json.Marshaler.
func (id BigInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(id.String())), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (id *BigInt) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if bytes.Equal(data, []byte("null")) {
		return fmt.Errorf("bigint: null")
	}

	var raw string
	if len(data) > 0 && data[0] == '"' {
		var err error
		raw, err = strconv.Unquote(string(data))
		if err != nil {
			return err
		}
	} else {
		raw = string(data)
	}

	parsed, err := ParseBigInt(raw)
	if err != nil {
		return err
	}
	*id = parsed
	return nil
}
