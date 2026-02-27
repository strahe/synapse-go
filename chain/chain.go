package chain

import (
	"errors"
	"fmt"
	"math/big"
)

// ErrUnknownChain is returned when an unrecognized chain or chain ID is used.
var ErrUnknownChain = errors.New("chain: unknown chain")

// Chain identifies a Filecoin network.
type Chain uint8

const (
	// Mainnet is the Filecoin production network (chain ID 314).
	Mainnet Chain = iota
	// Calibration is the Filecoin test network (chain ID 314159).
	Calibration

	chainCount // sentinel for array sizing; unexported
)

var chainIDs = [chainCount]int64{
	Mainnet:     314,
	Calibration: 314159,
}

var chainNames = [chainCount]string{
	Mainnet:     "mainnet",
	Calibration: "calibration",
}

// ChainID returns the EIP-155 chain ID for this chain.
func (c Chain) ChainID() int64 {
	if c < chainCount {
		return chainIDs[c]
	}
	return 0
}

// BigChainID returns the chain ID as a *big.Int, convenient for go-ethereum calls.
func (c Chain) BigChainID() *big.Int {
	return big.NewInt(c.ChainID())
}

// String returns the human-readable network name.
func (c Chain) String() string {
	if c < chainCount {
		return chainNames[c]
	}
	return fmt.Sprintf("chain(%d)", c)
}

// FromID returns the Chain for the given EIP-155 chain ID.
func FromID(id int64) (Chain, error) {
	for i := Chain(0); i < chainCount; i++ {
		if chainIDs[i] == id {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%w: chain ID %d", ErrUnknownChain, id)
}
