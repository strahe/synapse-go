package storage

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// DataSetInfo aliases warmstorage.EnhancedDataSetInfo, the richer record
// returned by Service.FindDataSets. Kept as an alias (not a copy) so that
// callers that already hold *warmstorage.EnhancedDataSetInfo can pass
// values through without conversion.
type DataSetInfo = warmstorage.EnhancedDataSetInfo

// PricePerTiB holds the effective price per TiB for one provisioning
// profile at three time granularities.
type PricePerTiB struct {
	PerMonth *big.Int
	PerDay   *big.Int
	PerEpoch *big.Int
}

// PricingInfo is the pricing sub-structure of StorageInfo.
type PricingInfo struct {
	NoCDN        PricePerTiB
	WithCDN      PricePerTiB
	TokenAddress common.Address
	TokenSymbol  string
}

// ServiceParameters captures the immutable chain/service geometry that
// StorageInfo callers want to show alongside pricing.
type ServiceParameters struct {
	EpochsPerMonth int64
	EpochsPerDay   int64
	EpochDuration  int64 // seconds per epoch
	MinUploadSize  int64
	MaxUploadSize  int64
}

// Allowances is the client-specific operator-approval view embedded in
// StorageInfo. Nil when the caller has no wallet or the lookup failed.
type Allowances struct {
	Service         common.Address
	IsApproved      bool
	RateAllowance   *big.Int
	LockupAllowance *big.Int
	RateUsed        *big.Int
	LockupUsed      *big.Int
	MaxLockupPeriod *big.Int
}

// StorageInfo aggregates the chain-wide storage view a client needs
// before authoring an upload.
type StorageInfo struct {
	Pricing           PricingInfo
	Providers         []spregistry.PDPProvider
	ServiceParameters ServiceParameters
	Allowances        *Allowances
}

// ContextCostRef references one prospective upload target for
// multi-context cost aggregation. A nil DataSetID means "a new data set
// will be created on this provider". CurrentDataSetSizeBytes is used
// only when DataSetID is non-nil; zero means the caller does not have
// the current size handy and wants the floor-price rate. WithCDN
// determines whether the CDN-fixed lockup is included for this ref.
type ContextCostRef struct {
	DataSetID               *types.BigInt
	Provider                Provider
	CurrentDataSetSizeBytes *big.Int
	WithCDN                 bool
}
