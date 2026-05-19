package spregistry

import (
	"encoding/hex"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("hex.DecodeString(%q): %v", s, err)
	}
	return b
}

// sampleOffering returns a fully-populated PDPOffering whose encoded layout
// is used by several tests.
func sampleOffering() PDPOffering {
	return PDPOffering{
		ServiceURL:               "https://example.com/pdp",
		MinPieceSizeInBytes:      big.NewInt(1024),
		MaxPieceSizeInBytes:      big.NewInt(1073741824),
		StoragePricePerTiBPerDay: big.NewInt(1000000),
		MinProvingPeriodInEpochs: big.NewInt(2880),
		Location:                 "US-WEST",
		PaymentTokenAddress:      common.HexToAddress("0x1234567890123456789012345678901234567890"),
		IPNIPiece:                false,
		IPNIIPFS:                 false,
	}
}

func TestEncodePDPCapabilities_RequiredOrder(t *testing.T) {
	keys, values, err := EncodePDPCapabilities(sampleOffering(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	wantKeys := []string{
		CapServiceURL,
		CapMinPieceSize,
		CapMaxPieceSize,
		CapStoragePrice,
		CapMinProvingPeriod,
		CapLocation,
		CapPaymentToken,
	}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("keys mismatch:\n  got  %v\n  want %v", keys, wantKeys)
	}

	wantValues := [][]byte{
		[]byte("https://example.com/pdp"),
		big.NewInt(1024).Bytes(),
		big.NewInt(1073741824).Bytes(),
		big.NewInt(1000000).Bytes(),
		big.NewInt(2880).Bytes(),
		[]byte("US-WEST"),
		common.HexToAddress("0x1234567890123456789012345678901234567890").Bytes(),
	}
	if len(values) != len(wantValues) {
		t.Fatalf("values length mismatch: got %d want %d", len(values), len(wantValues))
	}
	for i, want := range wantValues {
		if !reflect.DeepEqual(values[i], want) {
			t.Errorf("values[%d] %s: got %x want %x", i, keys[i], values[i], want)
		}
	}
	if got, want := len(values[6]), 20; got != want {
		t.Errorf("paymentTokenAddress must be 20 bytes, got %d", got)
	}
}

func TestEncodePDPCapabilities_IPNIFlags(t *testing.T) {
	off := sampleOffering()
	off.IPNIPiece = true
	off.IPNIIPFS = true

	keys, values, err := EncodePDPCapabilities(off, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Flags sit between maxPieceSize and storagePrice per the TS encoder.
	wantKeys := []string{
		CapServiceURL,
		CapMinPieceSize,
		CapMaxPieceSize,
		CapIPNIPiece,
		CapIPNIIPFS,
		CapStoragePrice,
		CapMinProvingPeriod,
		CapLocation,
		CapPaymentToken,
	}
	if !reflect.DeepEqual(keys, wantKeys) {
		t.Fatalf("keys mismatch:\n  got  %v\n  want %v", keys, wantKeys)
	}
	for i, k := range keys {
		if k == CapIPNIPiece || k == CapIPNIIPFS {
			if len(values[i]) != 1 || values[i][0] != 0x01 {
				t.Errorf("%s: expected single byte 0x01, got %x", k, values[i])
			}
		}
	}
}

func TestEncodePDPCapabilities_IPNIPeerIDNotEncoded(t *testing.T) {
	off := sampleOffering()
	off.IPNIPeerID = "ignored-on-encode"

	keys, _, err := EncodePDPCapabilities(off, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, k := range keys {
		if k == CapIPNIPeerID || k == CapIPNIPeerIDLegacyUC {
			t.Errorf("ipniPeerId unexpectedly encoded at key %q", k)
		}
	}
}

func TestEncodePDPCapabilities_ExtrasSortedAndValueFormats(t *testing.T) {
	extras := map[string]string{
		"zCustom":  "hello",
		"aPresent": "0x01",
		"bHex":     "0xdeadbeef",
		"cText":    "plain",
	}
	keys, values, err := EncodePDPCapabilities(sampleOffering(), extras)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The last four entries must be the extras in lexical order.
	extraKeys := keys[len(keys)-4:]
	want := []string{"aPresent", "bHex", "cText", "zCustom"}
	if !reflect.DeepEqual(extraKeys, want) {
		t.Fatalf("extras keys mismatch: got %v want %v", extraKeys, want)
	}
	extraValues := values[len(values)-4:]
	wantValues := [][]byte{
		{0x01},
		mustHex(t, "deadbeef"),
		[]byte("plain"),
		[]byte("hello"),
	}
	for i, want := range wantValues {
		if !reflect.DeepEqual(extraValues[i], want) {
			t.Errorf("extra %q: got %x want %x", extraKeys[i], extraValues[i], want)
		}
	}
}

func TestEncodePDPCapabilities_RejectsEmptyExtraValue(t *testing.T) {
	_, _, err := EncodePDPCapabilities(sampleOffering(), map[string]string{
		"aPresent": "",
	})
	if !errors.Is(err, ErrInvalidOffering) {
		t.Fatalf("expected ErrInvalidOffering, got %v", err)
	}
}

func TestEncodePDPCapabilities_ExtrasCannotShadowCanonical(t *testing.T) {
	_, _, err := EncodePDPCapabilities(sampleOffering(), map[string]string{
		CapServiceURL: "https://evil.example",
	})
	if !errors.Is(err, ErrInvalidOffering) {
		t.Fatalf("expected ErrInvalidOffering, got %v", err)
	}
}

func TestEncodePDPCapabilities_RejectsInvalidHexExtra(t *testing.T) {
	_, _, err := EncodePDPCapabilities(sampleOffering(), map[string]string{
		"weird": "0xZZZZ",
	})
	if !errors.Is(err, ErrInvalidOffering) {
		t.Fatalf("expected ErrInvalidOffering, got %v", err)
	}
}

func TestEncodePDPCapabilities_RejectsOversizedCapabilityFields(t *testing.T) {
	longLocation := strings.Repeat("x", 129)
	off := sampleOffering()
	off.Location = longLocation
	if _, _, err := EncodePDPCapabilities(off, nil); !errors.Is(err, ErrInvalidOffering) {
		t.Fatalf("long location: expected ErrInvalidOffering, got %v", err)
	}

	if _, _, err := EncodePDPCapabilities(sampleOffering(), map[string]string{
		"": "0x01",
	}); !errors.Is(err, ErrInvalidOffering) {
		t.Fatalf("empty key: expected ErrInvalidOffering, got %v", err)
	}

	if _, _, err := EncodePDPCapabilities(sampleOffering(), map[string]string{
		strings.Repeat("k", 33): "0x01",
	}); !errors.Is(err, ErrInvalidOffering) {
		t.Fatalf("long key: expected ErrInvalidOffering, got %v", err)
	}

	if _, _, err := EncodePDPCapabilities(sampleOffering(), map[string]string{
		"large": "0x" + strings.Repeat("00", 129),
	}); !errors.Is(err, ErrInvalidOffering) {
		t.Fatalf("long value: expected ErrInvalidOffering, got %v", err)
	}
}

func TestValidatePDPOffering_RejectsInvalidRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*PDPOffering)
	}{
		{"nil MinPieceSize", func(o *PDPOffering) { o.MinPieceSizeInBytes = nil }},
		{"negative MinPieceSize", func(o *PDPOffering) { o.MinPieceSizeInBytes = big.NewInt(-1) }},
		{"nil MaxPieceSize", func(o *PDPOffering) { o.MaxPieceSizeInBytes = nil }},
		{"negative MaxPieceSize", func(o *PDPOffering) { o.MaxPieceSizeInBytes = big.NewInt(-1) }},
		{"nil StoragePrice", func(o *PDPOffering) { o.StoragePricePerTiBPerDay = nil }},
		{"negative StoragePrice", func(o *PDPOffering) { o.StoragePricePerTiBPerDay = big.NewInt(-1) }},
		{"nil MinProvingPeriod", func(o *PDPOffering) { o.MinProvingPeriodInEpochs = nil }},
		{"zero MinProvingPeriod", func(o *PDPOffering) { o.MinProvingPeriodInEpochs = big.NewInt(0) }},
		{"empty ServiceURL", func(o *PDPOffering) { o.ServiceURL = "" }},
		{"empty Location", func(o *PDPOffering) { o.Location = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			off := sampleOffering()
			tc.mut(&off)
			if err := ValidatePDPOffering(off); !errors.Is(err, ErrInvalidOffering) {
				t.Errorf("expected ErrInvalidOffering, got %v", err)
			}
			if _, _, err := EncodePDPCapabilities(off, nil); !errors.Is(err, ErrInvalidOffering) {
				t.Errorf("Encode: expected ErrInvalidOffering, got %v", err)
			}
		})
	}
}

func TestEncodePDPCapabilities_RoundtripViaDecode(t *testing.T) {
	off := sampleOffering()
	off.IPNIPiece = true
	off.IPNIIPFS = true

	keys, values, err := EncodePDPCapabilities(off, map[string]string{"extra": "value"})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	decoded, err := DecodePDPOffering(CapabilitiesListToMap(keys, values))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if decoded.ServiceURL != off.ServiceURL {
		t.Errorf("ServiceURL mismatch")
	}
	if decoded.MinPieceSizeInBytes.Cmp(off.MinPieceSizeInBytes) != 0 {
		t.Errorf("MinPieceSize mismatch")
	}
	if decoded.MaxPieceSizeInBytes.Cmp(off.MaxPieceSizeInBytes) != 0 {
		t.Errorf("MaxPieceSize mismatch")
	}
	if decoded.StoragePricePerTiBPerDay.Cmp(off.StoragePricePerTiBPerDay) != 0 {
		t.Errorf("StoragePrice mismatch")
	}
	if decoded.MinProvingPeriodInEpochs.Cmp(off.MinProvingPeriodInEpochs) != 0 {
		t.Errorf("MinProvingPeriod mismatch")
	}
	if decoded.PaymentTokenAddress != off.PaymentTokenAddress {
		t.Errorf("PaymentTokenAddress mismatch")
	}
	if !decoded.IPNIPiece || !decoded.IPNIIPFS {
		t.Errorf("IPNI flags lost on roundtrip")
	}
	if got := string(decoded.ExtraCapabilities["extra"]); got != "value" {
		t.Errorf("extra roundtrip mismatch: %q", got)
	}
}
