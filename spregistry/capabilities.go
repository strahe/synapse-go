package spregistry

import (
	"fmt"
	"math/big"

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
)

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
// Semantics (aligned with filecoin-services spec and the TS SDK):
//   - Required numeric fields (sizes, price, proving period) are big-endian
//     unsigned integers; missing entries decode to 0.
//   - Boolean flags (ipniPiece/ipniIpfs) are true only when the value is
//     exactly 0x01 — presence alone is not sufficient.
//   - paymentTokenAddress is the last 20 bytes of the value; if the value
//     has fewer than 20 bytes it is left-padded (common.BytesToAddress).
//   - ipniPeerId is base58btc-encoded for display (matches TS SDK).
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
func ValidatePDPOffering(o PDPOffering) error {
	if o.ServiceURL == "" {
		return fmt.Errorf("%w: missing serviceURL", ErrInvalidOffering)
	}
	if o.StoragePricePerTiBPerDay == nil || o.StoragePricePerTiBPerDay.Sign() < 0 {
		return fmt.Errorf("%w: invalid storagePricePerTibPerDay", ErrInvalidOffering)
	}
	if o.MinProvingPeriodInEpochs == nil || o.MinProvingPeriodInEpochs.Sign() <= 0 {
		return fmt.Errorf("%w: invalid minProvingPeriodInEpochs", ErrInvalidOffering)
	}
	return nil
}
