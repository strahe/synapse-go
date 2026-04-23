// Package abi provides helpers for working with contract ABIs beyond what
// generated bindings offer: address auto-discovery via the FWSS root of
// trust, generic event extraction from receipts, and PieceCID <-> contract
// struct conversions.
package abi

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/contracts/fwss"
)

// ContractCaller is the subset of bind.ContractCaller used by helpers here.
// It is satisfied by *ethclient.Client.
type ContractCaller interface {
	bind.ContractCaller
}

// ResolvedAddresses holds the set of contract addresses that FWSS points to,
// plus FWSS itself.
type ResolvedAddresses struct {
	FWSS               common.Address
	PDPVerifier        common.Address
	SPRegistry         common.Address
	USDFC              common.Address
	Payments           common.Address
	ViewContract       common.Address
	FilBeamBeneficiary common.Address
	SessionKeyRegistry common.Address
	Multicall3         common.Address // propagated from chain fallback, if known
}

// ResolveAddresses reads address pointers directly from the FWSS contract.
// FWSS is the "root of trust" in the TS SDK — every other address is
// derived from it. fwssAddr must be non-zero; callers that want a registry
// fallback should use ResolvedAddressesFromChain separately.
//
// On any RPC/call failure the function returns an error; callers may choose
// to fall back to ResolvedAddressesFromChain.
func ResolveAddresses(ctx context.Context, caller ContractCaller, fwssAddr common.Address) (*ResolvedAddresses, error) {
	if caller == nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: nil caller")
	}
	if fwssAddr == (common.Address{}) {
		return nil, fmt.Errorf("abi.ResolveAddresses: fwss address is zero")
	}
	fwssABI, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: parse fwss abi: %w", err)
	}
	callData := func(name string) []byte {
		b, err := fwssABI.Pack(name)
		if err != nil {
			panic(err) //nolint:forbidigo // names below are hardcoded methods; Pack failure implies an ABI/codegen mismatch at build time
		}
		return b
	}
	methods := []string{
		"pdpVerifierAddress",
		"serviceProviderRegistry",
		"usdfcTokenAddress",
		"paymentsContractAddress",
		"viewContractAddress",
		"filBeamBeneficiaryAddress",
		"sessionKeyRegistry",
	}
	calls := make([]Call3, 0, len(methods))
	for _, name := range methods {
		calls = append(calls, Call3{
			Target:       fwssAddr,
			AllowFailure: true,
			CallData:     callData(name),
		})
	}
	results, err := BatchCall(ctx, caller, calls)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: batch call: %w", err)
	}
	if len(results) != len(methods) {
		return nil, fmt.Errorf("abi.ResolveAddresses: expected %d results, got %d", len(methods), len(results))
	}
	decodeAddress := func(i int) (common.Address, error) {
		if !results[i].Success {
			return common.Address{}, fmt.Errorf("%s returned unsuccessful result", methods[i])
		}
		values, err := fwssABI.Methods[methods[i]].Outputs.Unpack(results[i].ReturnData)
		if err != nil {
			return common.Address{}, fmt.Errorf("%s: unpack: %w", methods[i], err)
		}
		if len(values) != 1 {
			return common.Address{}, fmt.Errorf("%s: expected 1 output, got %d", methods[i], len(values))
		}
		addr, ok := values[0].(common.Address)
		if !ok {
			return common.Address{}, fmt.Errorf("%s: unexpected output type %T", methods[i], values[0])
		}
		return addr, nil
	}
	pdp, err := decodeAddress(0)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: %w", err)
	}
	reg, err := decodeAddress(1)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: %w", err)
	}
	usdfc, err := decodeAddress(2)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: %w", err)
	}
	pay, err := decodeAddress(3)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: %w", err)
	}
	view, err := decodeAddress(4)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: %w", err)
	}
	beam, err := decodeAddress(5)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: %w", err)
	}
	skr, err := decodeAddress(6)
	if err != nil {
		return nil, fmt.Errorf("abi.ResolveAddresses: %w", err)
	}

	return &ResolvedAddresses{
		FWSS:               fwssAddr,
		PDPVerifier:        pdp,
		SPRegistry:         reg,
		USDFC:              usdfc,
		Payments:           pay,
		ViewContract:       view,
		FilBeamBeneficiary: beam,
		SessionKeyRegistry: skr,
		Multicall3:         multicall3Address,
	}, nil
}

// ResolvedAddressesFromChain builds a ResolvedAddresses from the hard-coded
// chain registry. Intended as a fallback when on-chain resolution fails.
// ViewContract and FilBeamBeneficiary are left zero when unknown.
func ResolvedAddressesFromChain(c chain.Chain) ResolvedAddresses {
	a := c.Addresses()
	return ResolvedAddresses{
		FWSS:               a.FWSS,
		PDPVerifier:        a.PDPVerifier,
		SPRegistry:         a.SPRegistry,
		USDFC:              a.USDFC,
		Payments:           a.Payments,
		ViewContract:       a.StateView,
		SessionKeyRegistry: a.SessionKeyRegistry,
		Multicall3:         a.Multicall3,
	}
}
