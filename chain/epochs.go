package chain

import (
	"math/big"
	"time"
)

const (
	// EpochDuration is the time between Filecoin epochs.
	EpochDuration = 30 * time.Second

	// EpochDurationSeconds is the epoch duration in seconds.
	EpochDurationSeconds int64 = int64(EpochDuration / time.Second)

	// EpochsPerHour is the number of epochs in a 60-minute hour.
	EpochsPerHour int64 = int64(time.Hour / EpochDuration)

	// EpochsPerDay is the number of epochs in a 24-hour day.
	EpochsPerDay int64 = int64((24 * time.Hour) / EpochDuration)

	// EpochsPerMonth is the approximate number of epochs in a 30-day month.
	EpochsPerMonth int64 = 30 * EpochsPerDay
)

var (
	bigEpochsPerHour = big.NewInt(EpochsPerHour)
	bigEpochsPerDay  = big.NewInt(EpochsPerDay)
	maxUint256Epoch  = new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
)

var genesisTimestamps = [chainCount]int64{
	Mainnet:     1598306400, // 2020-08-24T22:00:00Z
	Calibration: 1667326380, // 2022-11-01T18:13:00Z
}

// GenesisTimestamp returns the genesis Unix timestamp for this chain.
// Returns 0 for chains without a known genesis.
func (c Chain) GenesisTimestamp() int64 {
	if c < chainCount {
		return genesisTimestamps[c]
	}
	return 0
}

// CurrentEpoch returns the current Filecoin epoch for the given chain.
// Returns zero for chains without a known genesis timestamp.
func CurrentEpoch(c Chain) *big.Int {
	genesis := c.GenesisTimestamp()
	if genesis == 0 {
		return new(big.Int)
	}
	return big.NewInt((time.Now().Unix() - genesis) / EpochDurationSeconds)
}

// EpochToTime converts a Filecoin epoch number to a wall-clock time.
// Returns the zero time for chains without a known genesis, if epoch is nil,
// or if the resulting Unix timestamp does not fit in int64.
func EpochToTime(c Chain, epoch *big.Int) time.Time {
	genesis := c.GenesisTimestamp()
	if genesis == 0 || epoch == nil {
		return time.Time{}
	}

	seconds := new(big.Int).Mul(new(big.Int).Set(epoch), big.NewInt(EpochDurationSeconds))
	seconds.Add(seconds, big.NewInt(genesis))
	if !seconds.IsInt64() {
		return time.Time{}
	}

	return time.Unix(seconds.Int64(), 0)
}

// TimeToEpoch converts a wall-clock time to a Filecoin epoch number.
// Returns zero for chains without a known genesis.
func TimeToEpoch(c Chain, t time.Time) *big.Int {
	genesis := c.GenesisTimestamp()
	if genesis == 0 {
		return new(big.Int)
	}
	return big.NewInt((t.Unix() - genesis) / EpochDurationSeconds)
}

// EpochsToHours converts epochs to whole hours. A nil input returns nil.
func EpochsToHours(epochs *big.Int) *big.Int {
	return epochsToUnits(epochs, bigEpochsPerHour)
}

// EpochsToDays converts epochs to whole days. A nil input returns nil.
func EpochsToDays(epochs *big.Int) *big.Int {
	return epochsToUnits(epochs, bigEpochsPerDay)
}

func epochsToUnits(epochs, units *big.Int) *big.Int {
	if epochs == nil {
		return nil
	}
	if epochs.Cmp(maxUint256Epoch) == 0 {
		return new(big.Int).Set(maxUint256Epoch)
	}
	return new(big.Int).Quo(epochs, units)
}
