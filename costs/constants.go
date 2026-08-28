package costs

import (
	"math/big"

	"github.com/strahe/synapse-go/chain"
)

const (
	// DefaultExtraRunwayEpochs adds no runway above the lockup period.
	DefaultExtraRunwayEpochs int64 = 0
	// DefaultBufferEpochs is a 5-epoch deposit cushion for transaction execution latency.
	DefaultBufferEpochs int64 = 5
	// DefaultLockupPeriod is the standard lockup horizon (30 days in epochs).
	DefaultLockupPeriod int64 = chain.EpochsPerMonth
)

var (
	// maxUint256 is 2^256-1.
	maxUint256 = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	// halfMaxUint256 is maxUint256 >> 1.
	halfMaxUint256 = new(big.Int).Rsh(maxUint256, 1)

	bigOne = big.NewInt(1)
	bigTiB = big.NewInt(chain.TiB)
)
