package synapse

import (
	"context"
	"fmt"
	"math/big"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	iabi "github.com/strahe/synapse-go/internal/abi"
)

// ContractCaller is the RPC surface required to resolve contract addresses.
type ContractCaller interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
}

// ResolvedAddresses is the contract topology read from FWSS. FWSS is the
// bootstrap address supplied to [ResolveAddresses]. PDPVerifier and
// FilBeamBeneficiary may be zero when those optional services are not
// configured.
type ResolvedAddresses struct {
	FWSS               common.Address
	PDPVerifier        common.Address
	SPRegistry         common.Address
	USDFC              common.Address
	Payments           common.Address
	ViewContract       common.Address
	FilBeamBeneficiary common.Address
	SessionKeyRegistry common.Address
}

// ResolveAddresses reads the current contract topology from FWSS. It uses the
// SDK's Multicall3 deployment for supported Filecoin networks and does not
// fall back to compiled child-contract addresses.
func ResolveAddresses(ctx context.Context, caller ContractCaller, fwss common.Address) (ResolvedAddresses, error) {
	if caller == nil {
		return ResolvedAddresses{}, fmt.Errorf("synapse.ResolveAddresses: caller: %w", ErrInvalidArgument)
	}
	if fwss == (common.Address{}) {
		return ResolvedAddresses{}, fmt.Errorf("synapse.ResolveAddresses: FWSS: %w", ErrInvalidArgument)
	}

	resolved, err := iabi.ResolveAddresses(ctx, caller, fwss)
	if err != nil {
		return ResolvedAddresses{}, fmt.Errorf("synapse.ResolveAddresses: %w", err)
	}
	addresses := ResolvedAddresses(*resolved)
	for _, required := range []struct {
		name    string
		address common.Address
	}{
		{name: "SPRegistry", address: addresses.SPRegistry},
		{name: "USDFC", address: addresses.USDFC},
		{name: "Payments", address: addresses.Payments},
		{name: "ViewContract", address: addresses.ViewContract},
		{name: "SessionKeyRegistry", address: addresses.SessionKeyRegistry},
	} {
		if required.address == (common.Address{}) {
			return ResolvedAddresses{}, fmt.Errorf("synapse.ResolveAddresses: %s returned zero address", required.name)
		}
	}
	return addresses, nil
}

// ResolvedAddresses returns the contract-address snapshot used to initialise
// the Client and its services.
func (c *Client) ResolvedAddresses() ResolvedAddresses {
	return c.addresses
}
