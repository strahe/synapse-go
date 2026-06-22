package testutil

import (
	"strings"
	"testing"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/contracts/fwss"
)

// FWSSAddressResolutionResultHex returns an ABI-encoded Multicall3 aggregate3
// result for the FWSS address-resolution calls used by synapse.New tests.
func FWSSAddressResolutionResultHex(t testing.TB, c chain.Chain) string {
	t.Helper()
	return FWSSAddressResolutionResultHexFor(t, c.Addresses(), common.Address{})
}

// FWSSAddressResolutionResultHexFor returns an ABI-encoded Multicall3
// aggregate3 result for the supplied child-contract addresses.
func FWSSAddressResolutionResultHexFor(t testing.TB, addresses chain.ContractAddresses, filBeamBeneficiary common.Address) string {
	t.Helper()

	fwssABI, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatalf("parse fwss abi: %v", err)
	}
	multicallABI, err := gethabi.JSON(strings.NewReader(`[{"inputs":[{"components":[{"internalType":"address","name":"target","type":"address"},{"internalType":"bool","name":"allowFailure","type":"bool"},{"internalType":"bytes","name":"callData","type":"bytes"}],"internalType":"struct Multicall3.Call3[]","name":"calls","type":"tuple[]"}],"name":"aggregate3","outputs":[{"components":[{"internalType":"bool","name":"success","type":"bool"},{"internalType":"bytes","name":"returnData","type":"bytes"}],"internalType":"struct Multicall3.Result[]","name":"returnData","type":"tuple[]"}],"stateMutability":"payable","type":"function"}]`))
	if err != nil {
		t.Fatalf("parse multicall abi: %v", err)
	}

	pack := func(name string, addr common.Address) []byte {
		t.Helper()
		b, err := fwssABI.Methods[name].Outputs.Pack(addr)
		if err != nil {
			t.Fatalf("pack %s: %v", name, err)
		}
		return b
	}
	type result struct {
		Success    bool
		ReturnData []byte
	}
	out, err := multicallABI.Methods["aggregate3"].Outputs.Pack([]result{
		{Success: true, ReturnData: pack("pdpVerifierAddress", addresses.PDPVerifier)},
		{Success: true, ReturnData: pack("serviceProviderRegistry", addresses.SPRegistry)},
		{Success: true, ReturnData: pack("usdfcTokenAddress", addresses.USDFC)},
		{Success: true, ReturnData: pack("paymentsContractAddress", addresses.Payments)},
		{Success: true, ReturnData: pack("viewContractAddress", addresses.StateView)},
		{Success: true, ReturnData: pack("filBeamBeneficiaryAddress", filBeamBeneficiary)},
		{Success: true, ReturnData: pack("sessionKeyRegistry", addresses.SessionKeyRegistry)},
	})
	if err != nil {
		t.Fatalf("pack aggregate3 output: %v", err)
	}
	return hexutil.Encode(out)
}
