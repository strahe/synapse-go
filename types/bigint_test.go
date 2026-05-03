package types

import (
	"encoding/json"
	"math"
	"math/big"
	"testing"
)

func TestBigIntZeroValue(t *testing.T) {
	var id BigInt
	if !id.IsZero() {
		t.Fatal("zero value should be zero")
	}
	if id.String() != "0" {
		t.Fatalf("String=%q", id.String())
	}
	if got := id.Big(); got.Sign() != 0 {
		t.Fatalf("Big=%s", got)
	}
	got, ok := id.Uint64()
	if !ok || got != 0 {
		t.Fatalf("Uint64=%d ok=%v", got, ok)
	}
}

func TestBigIntFromBigValidation(t *testing.T) {
	if _, err := BigIntFromBig(nil); err == nil {
		t.Fatal("nil should fail")
	}
	if _, err := BigIntFromBig(big.NewInt(-1)); err == nil {
		t.Fatal("negative should fail")
	}

	max := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	if _, err := BigIntFromBig(max); err != nil {
		t.Fatalf("max uint256 should pass: %v", err)
	}

	overflow := new(big.Int).Lsh(big.NewInt(1), 256)
	if _, err := BigIntFromBig(overflow); err == nil {
		t.Fatal("2^256 should fail")
	}
}

func TestBigIntDefensiveCopy(t *testing.T) {
	src := big.NewInt(42)
	id, err := BigIntFromBig(src)
	if err != nil {
		t.Fatalf("BigIntFromBig: %v", err)
	}
	src.SetUint64(99)
	if id.String() != "42" {
		t.Fatalf("source mutation changed id: %s", id.String())
	}

	out := id.Big()
	out.SetUint64(100)
	if id.String() != "42" {
		t.Fatalf("Big mutation changed id: %s", id.String())
	}

	cp := id.Copy()
	if !cp.Equal(id) {
		t.Fatalf("Copy=%s want %s", cp.String(), id.String())
	}
	cp.n.SetUint64(7)
	if id.String() != "42" {
		t.Fatalf("Copy mutation changed id: %s", id.String())
	}
}

func TestBigIntEqualAndCmp(t *testing.T) {
	var zero BigInt
	explicitZero := NewBigInt(0)
	one := NewBigInt(1)
	alsoOne, err := BigIntFromBig(big.NewInt(1))
	if err != nil {
		t.Fatalf("BigIntFromBig: %v", err)
	}
	two := NewBigInt(2)

	if !zero.Equal(explicitZero) || zero.Cmp(explicitZero) != 0 {
		t.Fatalf("zero comparison failed: zero/explicit=%d", zero.Cmp(explicitZero))
	}
	if zero.Cmp(one) >= 0 || one.Cmp(zero) <= 0 {
		t.Fatalf("zero ordering failed: zero/one=%d one/zero=%d", zero.Cmp(one), one.Cmp(zero))
	}
	if !one.Equal(alsoOne) {
		t.Fatal("equal values should compare equal")
	}
	if one.Equal(two) {
		t.Fatal("different values should not compare equal")
	}
	if one.Cmp(two) >= 0 || two.Cmp(one) <= 0 || one.Cmp(alsoOne) != 0 {
		t.Fatalf("bad ordering: one/two=%d two/one=%d one/also=%d", one.Cmp(two), two.Cmp(one), one.Cmp(alsoOne))
	}
}

func TestBigIntBytes32(t *testing.T) {
	var zero BigInt
	if got := zero.Bytes32(); got != ([32]byte{}) {
		t.Fatalf("zero Bytes32=%x", got)
	}

	one := NewBigInt(1)
	got := one.Bytes32()
	if got[31] != 1 {
		t.Fatalf("one Bytes32=%x", got)
	}
	for i := 0; i < 31; i++ {
		if got[i] != 0 {
			t.Fatalf("one Bytes32 not left-padded: %x", got)
		}
	}

	max, err := BigIntFromBig(new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1)))
	if err != nil {
		t.Fatalf("BigIntFromBig: %v", err)
	}
	for i, b := range max.Bytes32() {
		if b != 0xff {
			t.Fatalf("max Bytes32[%d]=%x want ff", i, b)
		}
	}
}

func TestBigIntUint64Overflow(t *testing.T) {
	large, err := BigIntFromBig(new(big.Int).Add(new(big.Int).SetUint64(math.MaxUint64), big.NewInt(1)))
	if err != nil {
		t.Fatalf("BigIntFromBig: %v", err)
	}
	if got, ok := large.Uint64(); ok || got != 0 {
		t.Fatalf("Uint64=%d ok=%v", got, ok)
	}
}

func TestBigIntTextAndJSON(t *testing.T) {
	id := NewBigInt(12345)

	text, err := id.MarshalText()
	if err != nil {
		t.Fatalf("MarshalText: %v", err)
	}
	if string(text) != "12345" {
		t.Fatalf("text=%q", text)
	}

	var fromText BigInt
	if err := fromText.UnmarshalText([]byte("12345")); err != nil {
		t.Fatalf("UnmarshalText: %v", err)
	}
	if !fromText.Equal(id) {
		t.Fatalf("fromText=%s want %s", fromText.String(), id.String())
	}

	jsonBytes, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	if string(jsonBytes) != `"12345"` {
		t.Fatalf("json=%s", jsonBytes)
	}

	for _, raw := range []string{`"12345"`, `12345`} {
		var got BigInt
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", raw, err)
		}
		if !got.Equal(id) {
			t.Fatalf("UnmarshalJSON(%s)=%s want %s", raw, got.String(), id.String())
		}
	}
}

func TestBigIntRejectsNonDecimalInput(t *testing.T) {
	for _, raw := range []string{"", " 1", "1 ", "+1", "-1", "0x10", "1.0"} {
		if _, err := ParseBigInt(raw); err == nil {
			t.Fatalf("ParseBigInt(%q) should fail", raw)
		}

		var id BigInt
		if err := id.UnmarshalText([]byte(raw)); err == nil {
			t.Fatalf("UnmarshalText(%q) should fail", raw)
		}
	}

	for _, raw := range []string{`"0x10"`, `"1.0"`, `1.0`, `-1`} {
		var id BigInt
		if err := json.Unmarshal([]byte(raw), &id); err == nil {
			t.Fatalf("UnmarshalJSON(%s) should fail", raw)
		}
	}
}
