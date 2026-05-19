package spregistry

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/multiformats/go-multibase"
)

// Canonical PDP capability keys (must match
// filecoin-services/service_contracts/src/ServiceProviderRegistry.sol
// REQUIRED_PDP_KEYS Bloom filter).
const (
	CapServiceURL         = "serviceURL"
	CapMinPieceSize       = "minPieceSizeInBytes"
	CapMaxPieceSize       = "maxPieceSizeInBytes"
	CapStoragePrice       = "storagePricePerTibPerDay"
	CapMinProvingPeriod   = "minProvingPeriodInEpochs"
	CapLocation           = "location"
	CapPaymentToken       = "paymentTokenAddress"
	CapIPNIPiece          = "ipniPiece"
	CapIPNIIPFS           = "ipniIpfs"
	CapIPNIPeerID         = "ipniPeerId"
	CapIPNIPeerIDLegacyUC = "IPNIPeerID" // legacy key, still read

	maxProviderNameBytes        = 128
	maxProviderDescriptionBytes = 256
	maxLocationBytes            = 128
	maxCapabilityKeyBytes       = 32
	maxCapabilityValueBytes     = 128
	maxCapabilities             = 24
)

func registrationFeeWei() *big.Int {
	return big.NewInt(5_000_000_000_000_000_000)
}

// knownCaps is the set of keys that map into typed PDPOffering fields;
// everything else ends up in ExtraCapabilities.
var knownCaps = map[string]struct{}{
	CapServiceURL:         {},
	CapMinPieceSize:       {},
	CapMaxPieceSize:       {},
	CapStoragePrice:       {},
	CapMinProvingPeriod:   {},
	CapLocation:           {},
	CapPaymentToken:       {},
	CapIPNIPiece:          {},
	CapIPNIIPFS:           {},
	CapIPNIPeerID:         {},
	CapIPNIPeerIDLegacyUC: {},
}

// CapabilitiesListToMap zips parallel keys/values slices returned by the
// contract into a map.
func CapabilitiesListToMap(keys []string, values [][]byte) map[string][]byte {
	m := make(map[string][]byte, len(keys))
	n := len(keys)
	if len(values) < n {
		n = len(values)
	}
	for i := 0; i < n; i++ {
		m[keys[i]] = values[i]
	}
	return m
}

// DecodePDPOffering decodes the capability map into a typed PDPOffering.
//
// Semantics:
//   - Required numeric fields (sizes, price, proving period) are big-endian
//     unsigned integers; missing entries decode to 0.
//   - Boolean flags (ipniPiece/ipniIpfs) are true only when the value is
//     exactly 0x01 — presence alone is not sufficient.
//   - paymentTokenAddress is the last 20 bytes of the value; if the value
//     has fewer than 20 bytes it is left-padded (common.BytesToAddress).
//   - ipniPeerId is base58btc-encoded for display.
//     The uppercase legacy key "IPNIPeerID" is accepted for read only.
//
// Missing required fields do NOT produce an error; they are zero-valued.
// Callers that need strict validation should check individual fields.
func DecodePDPOffering(caps map[string][]byte) (PDPOffering, error) {
	off := PDPOffering{
		MinPieceSizeInBytes:      big.NewInt(0),
		MaxPieceSizeInBytes:      big.NewInt(0),
		StoragePricePerTiBPerDay: big.NewInt(0),
		MinProvingPeriodInEpochs: big.NewInt(0),
		ExtraCapabilities:        map[string][]byte{},
	}

	if v, ok := caps[CapServiceURL]; ok {
		off.ServiceURL = string(v)
	}
	if v, ok := caps[CapLocation]; ok {
		off.Location = string(v)
	}
	if v, ok := caps[CapMinPieceSize]; ok {
		off.MinPieceSizeInBytes = new(big.Int).SetBytes(v)
	}
	if v, ok := caps[CapMaxPieceSize]; ok {
		off.MaxPieceSizeInBytes = new(big.Int).SetBytes(v)
	}
	if v, ok := caps[CapStoragePrice]; ok {
		off.StoragePricePerTiBPerDay = new(big.Int).SetBytes(v)
	}
	if v, ok := caps[CapMinProvingPeriod]; ok {
		off.MinProvingPeriodInEpochs = new(big.Int).SetBytes(v)
	}
	if v, ok := caps[CapPaymentToken]; ok {
		off.PaymentTokenAddress = common.BytesToAddress(v)
	}
	if v, ok := caps[CapIPNIPiece]; ok {
		off.IPNIPiece = len(v) == 1 && v[0] == 0x01
	}
	if v, ok := caps[CapIPNIIPFS]; ok {
		off.IPNIIPFS = len(v) == 1 && v[0] == 0x01
	}

	// Prefer the canonical key; fall back to the legacy uppercase key.
	if v, ok := caps[CapIPNIPeerID]; ok && len(v) > 0 {
		enc, err := multibase.Encode(multibase.Base58BTC, v)
		if err != nil {
			return PDPOffering{}, fmt.Errorf("spregistry: decode ipniPeerId: %w", err)
		}
		off.IPNIPeerID = enc
	} else if v, ok := caps[CapIPNIPeerIDLegacyUC]; ok && len(v) > 0 {
		enc, err := multibase.Encode(multibase.Base58BTC, v)
		if err != nil {
			return PDPOffering{}, fmt.Errorf("spregistry: decode legacy IPNIPeerID: %w", err)
		}
		off.IPNIPeerID = enc
	}

	for k, v := range caps {
		if _, known := knownCaps[k]; known {
			continue
		}
		off.ExtraCapabilities[k] = v
	}
	return off, nil
}

// ValidatePDPOffering checks that required fields are present and have basic
// sane values. Optional capabilities and detailed size/range relationships
// are left to higher-level caller policy.
//
// All *big.Int fields must be non-nil and non-negative. MinProvingPeriodInEpochs
// must additionally be strictly positive (zero proving periods are rejected
// by the on-chain validation). ServiceURL and Location must be non-empty.
func ValidatePDPOffering(o PDPOffering) error {
	if o.ServiceURL == "" {
		return fmt.Errorf("%w: missing serviceURL", ErrInvalidOffering)
	}
	if o.MinPieceSizeInBytes == nil {
		return fmt.Errorf("%w: missing minPieceSizeInBytes", ErrInvalidOffering)
	}
	if o.MinPieceSizeInBytes.Sign() < 0 {
		return fmt.Errorf("%w: negative minPieceSizeInBytes", ErrInvalidOffering)
	}
	if o.MaxPieceSizeInBytes == nil {
		return fmt.Errorf("%w: missing maxPieceSizeInBytes", ErrInvalidOffering)
	}
	if o.MaxPieceSizeInBytes.Sign() < 0 {
		return fmt.Errorf("%w: negative maxPieceSizeInBytes", ErrInvalidOffering)
	}
	if o.StoragePricePerTiBPerDay == nil {
		return fmt.Errorf("%w: missing storagePricePerTibPerDay", ErrInvalidOffering)
	}
	if o.StoragePricePerTiBPerDay.Sign() < 0 {
		return fmt.Errorf("%w: negative storagePricePerTibPerDay", ErrInvalidOffering)
	}
	if o.MinProvingPeriodInEpochs == nil {
		return fmt.Errorf("%w: missing minProvingPeriodInEpochs", ErrInvalidOffering)
	}
	if o.MinProvingPeriodInEpochs.Sign() <= 0 {
		return fmt.Errorf("%w: non-positive minProvingPeriodInEpochs", ErrInvalidOffering)
	}
	if o.Location == "" {
		return fmt.Errorf("%w: missing location", ErrInvalidOffering)
	}
	if len(o.Location) > maxLocationBytes {
		return fmt.Errorf("%w: location too long", ErrInvalidOffering)
	}
	return nil
}

// EncodePDPCapabilities produces the parallel (keys, values) byte slices
// expected by the ServiceProviderRegistry contract's registerProvider,
// addProduct, and updateProduct methods.
//
// Canonical layout (positions 1-9):
//
//  1. serviceURL               (UTF-8 bytes)
//  2. minPieceSizeInBytes      (big-endian minimal; zero encodes as 0x00)
//  3. maxPieceSizeInBytes
//  4. (optional) ipniPiece     (single byte 0x01, omitted when false)
//  5. (optional) ipniIpfs      (single byte 0x01, omitted when false)
//  6. storagePricePerTibPerDay
//  7. minProvingPeriodInEpochs
//  8. location                 (required UTF-8 bytes)
//  9. paymentTokenAddress      (20 bytes)
//
// 10. any entries in extras, sorted by key for deterministic output.
//
// Note: ipniPeerId is intentionally NOT emitted even though the decoder
// accepts it for backwards compatibility.
//
// For extras values:
//   - a "0x"-prefixed string is hex-decoded;
//   - otherwise the raw UTF-8 bytes are used.
//
// Extras keys are sorted alphabetically to guarantee deterministic output.
// On-chain storage is a key/value mapping, so this ordering is not observable
// to the contract.
//
// Attempting to set a canonical PDP key through extras returns
// ErrInvalidOffering: doing so would either duplicate or shadow the
// typed field in a way the contract cannot disambiguate.
//
// The offering itself is validated via [ValidatePDPOffering] before any
// bytes are emitted.
func EncodePDPCapabilities(o PDPOffering, extras map[string]string) (keys []string, values [][]byte, err error) {
	if err := ValidatePDPOffering(o); err != nil {
		return nil, nil, err
	}
	for k := range extras {
		if _, clashes := knownCaps[k]; clashes {
			return nil, nil, fmt.Errorf("%w: extra capability %q clashes with a canonical PDP key", ErrInvalidOffering, k)
		}
	}

	keys = make([]string, 0, 9+len(extras))
	values = make([][]byte, 0, 9+len(extras))

	push := func(k string, v []byte) {
		keys = append(keys, k)
		values = append(values, v)
	}

	push(CapServiceURL, []byte(o.ServiceURL))
	push(CapMinPieceSize, bigIntBytes(o.MinPieceSizeInBytes))
	push(CapMaxPieceSize, bigIntBytes(o.MaxPieceSizeInBytes))
	if o.IPNIPiece {
		push(CapIPNIPiece, []byte{0x01})
	}
	if o.IPNIIPFS {
		push(CapIPNIIPFS, []byte{0x01})
	}
	push(CapStoragePrice, bigIntBytes(o.StoragePricePerTiBPerDay))
	push(CapMinProvingPeriod, bigIntBytes(o.MinProvingPeriodInEpochs))
	push(CapLocation, []byte(o.Location))
	push(CapPaymentToken, o.PaymentTokenAddress.Bytes())

	if len(extras) > 0 {
		sortedKeys := make([]string, 0, len(extras))
		for k := range extras {
			sortedKeys = append(sortedKeys, k)
		}
		sort.Strings(sortedKeys)
		for _, k := range sortedKeys {
			encoded, err := encodeExtraValue(extras[k])
			if err != nil {
				return nil, nil, fmt.Errorf("%w: capability %q: %w", ErrInvalidOffering, k, err)
			}
			push(k, encoded)
		}
	}

	if err := validateCapabilityLists(keys, values); err != nil {
		return nil, nil, err
	}
	return keys, values, nil
}

func bigIntBytes(v *big.Int) []byte {
	if v == nil {
		return nil
	}
	if v.Sign() == 0 {
		// big.Int.Bytes() returns an empty slice for zero, but the on-chain
		// capability wire format uses a single zero byte.
		return []byte{0x00}
	}
	return v.Bytes()
}

func encodeExtraValue(v string) ([]byte, error) {
	if v == "" {
		return nil, fmt.Errorf("empty capability value")
	}
	if strings.HasPrefix(v, "0x") || strings.HasPrefix(v, "0X") {
		decoded, err := hex.DecodeString(v[2:])
		if err != nil {
			return nil, fmt.Errorf("invalid hex value: %w", err)
		}
		return decoded, nil
	}
	return []byte(v), nil
}

func validateProviderInfo(name, description string) error {
	if len(name) > maxProviderNameBytes {
		return fmt.Errorf("%w: provider name too long", ErrInvalidArgument)
	}
	if len(description) > maxProviderDescriptionBytes {
		return fmt.Errorf("%w: provider description too long", ErrInvalidArgument)
	}
	return nil
}

func validateCapabilityLists(keys []string, values [][]byte) error {
	if len(keys) != len(values) {
		return fmt.Errorf("%w: capability keys and values length mismatch", ErrInvalidOffering)
	}
	if len(keys) > maxCapabilities {
		return fmt.Errorf("%w: too many capabilities", ErrInvalidOffering)
	}
	for i := range keys {
		keyLen := len(keys[i])
		if keyLen == 0 {
			return fmt.Errorf("%w: empty capability key", ErrInvalidOffering)
		}
		if keyLen > maxCapabilityKeyBytes {
			return fmt.Errorf("%w: capability key too long", ErrInvalidOffering)
		}
		valueLen := len(values[i])
		if valueLen == 0 {
			return fmt.Errorf("%w: empty capability value", ErrInvalidOffering)
		}
		if valueLen > maxCapabilityValueBytes {
			return fmt.Errorf("%w: capability value too long", ErrInvalidOffering)
		}
	}
	return nil
}
