package spregistry

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/types"
)

// ProductType mirrors the uint8 product-type enum used by the
// ServiceProviderRegistry contract.
type ProductType uint8

const (
	// ProductTypePDP is the Proof-of-Data-Possession product (id 0).
	ProductTypePDP ProductType = 0
)

// ProviderInfo is the canonical on-chain provider record.
type ProviderInfo struct {
	ID              types.ProviderID
	ServiceProvider common.Address
	Payee           common.Address
	Name            string
	Description     string
	IsActive        bool
}

// PDPOffering is the decoded PDP product offering, derived from the
// capability key/value pairs returned by getProviderWithProduct.
//
// Canonical capability keys are defined by filecoin-services'
// ServiceProviderRegistry.sol REQUIRED_PDP_KEYS Bloom filter.
type PDPOffering struct {
	ServiceURL               string
	MinPieceSizeInBytes      *big.Int
	MaxPieceSizeInBytes      *big.Int
	StoragePricePerTiBPerDay *big.Int
	MinProvingPeriodInEpochs *big.Int
	Location                 string
	PaymentTokenAddress      common.Address

	// Optional capabilities (present only if the provider set them).
	IPNIPiece  bool
	IPNIIPFS   bool
	IPNIPeerID string // base58btc-encoded peer id; empty if absent

	// Any additional capabilities declared by the provider. Keys are
	// the raw on-chain strings; values are the raw bytes (NOT decoded).
	ExtraCapabilities map[string][]byte
}

// PDPProvider is a provider record together with its decoded PDP offering.
// This is the high-level view that consumers (storage package) need.
type PDPProvider struct {
	Info     ProviderInfo
	Product  ServiceProduct
	Offering PDPOffering
}

// ServiceProduct is the on-chain product record (pre-decoding of
// capability values).
type ServiceProduct struct {
	ProductType    ProductType
	CapabilityKeys []string
	IsActive       bool
}

// PaginatedPDPProviders is the paginated result from GetPDPProviders.
type PaginatedPDPProviders struct {
	Providers []PDPProvider
	HasMore   bool
}

// ProviderFilter describes criteria for selecting active PDP providers.
// A zero-value ProviderFilter accepts all active providers.
//
// All non-nil / non-zero fields are applied as AND conditions.
type ProviderFilter struct {
	// PieceSizeBytes, when non-nil, retains only providers whose
	// minPieceSizeInBytes ≤ PieceSizeBytes ≤ maxPieceSizeInBytes.
	PieceSizeBytes *big.Int

	// PaymentToken, when non-nil, retains only providers that accept
	// this payment token. The zero address is a valid value here and
	// means FIL, matching the on-chain/TS semantics.
	PaymentToken *common.Address

	// ExcludeIDs is a set of provider IDs to skip. This supports the
	// replacement-with-exclusion-set pattern required for secondary
	// provider selection during multi-copy upload.
	ExcludeIDs []types.ProviderID
}
