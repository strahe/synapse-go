package costs

import (
	"math/big"

	"github.com/strahe/synapse-go/chain"
)

const (
	// DefaultExtraRunwayEpochs matches synapse-sdk: no extra runway above lockup period.
	DefaultExtraRunwayEpochs int64 = 0
	// DefaultBufferEpochs is a 5-epoch deposit cushion for transaction execution latency.
	DefaultBufferEpochs int64 = 5
	// DefaultLockupPeriod is the standard lockup horizon (30 days in epochs).
	DefaultLockupPeriod int64 = chain.EpochsPerMonth
)

// CDNFixedLockupValue returns the fixed lockup amount for new CDN-enabled datasets (1.0 USDFC).
// The caller owns the returned value and may modify it freely.
func CDNFixedLockupValue() *big.Int {
	return new(big.Int).Set(cdnFixedLockup)
}

var (
	// cdnFixedLockup is the fixed lockup for new CDN-enabled datasets (1.0 USDFC).
	// Access via CDNFixedLockupValue() to prevent in-place mutation of the global.
	cdnFixedLockup = big.NewInt(1_000_000_000_000_000_000)

	// maxUint256 is 2^256-1.
	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	// halfMaxUint256 is maxUint256 >> 1.
	halfMaxUint256 = new(big.Int).Rsh(maxUint256, 1)

	bigOne = big.NewInt(1)
	bigTiB = big.NewInt(chain.TiB)
)
