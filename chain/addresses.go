package chain

import "github.com/ethereum/go-ethereum/common"

// ContractAddresses holds the built-in deployment snapshot for a chain. FWSS
// is the bootstrap root for resolving its dependent contract addresses.
// Endorsements is an independently deployed ProviderIdSet and therefore has
// no FWSS resolution path; canonical redeployments require an SDK address
// update.
type ContractAddresses struct {
	FWSS               common.Address // Filecoin Warm Storage Service (root of trust)
	Payments           common.Address
	StateView          common.Address // warm storage state view
	PDPVerifier        common.Address
	SPRegistry         common.Address
	Endorsements       common.Address // ProviderIdSet used for endorsed upload primaries
	USDFC              common.Address
	Multicall3         common.Address
	SessionKeyRegistry common.Address
}

var knownAddresses = [chainCount]ContractAddresses{
	Mainnet: {
		FWSS:               common.HexToAddress("0x8408502033C418E1bbC97cE9ac48E5528F371A9f"),
		Payments:           common.HexToAddress("0x23b1e018F08BB982348b15a86ee926eEBf7F4DAa"),
		StateView:          common.HexToAddress("0xdDd8F083a3fe9C66547D46bee24e5AaF56BCa0ab"),
		PDPVerifier:        common.HexToAddress("0xBADd0B92C1c71d02E7d520f64c0876538fa2557F"),
		SPRegistry:         common.HexToAddress("0xf55dDbf63F1b55c3F1D4FA7e339a68AB7b64A5eB"),
		Endorsements:       common.HexToAddress("0x59eFa2e8324E1551d46010d7B0B140eE2F5c726b"),
		USDFC:              common.HexToAddress("0x80B98d3aa09ffff255c3ba4A241111Ff1262F045"),
		Multicall3:         common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
		SessionKeyRegistry: common.HexToAddress("0x74FD50525A958aF5d484601E252271f9625231aB"),
	},
	Calibration: {
		FWSS:               common.HexToAddress("0x02925630df557F957f70E112bA06e50965417CA0"),
		Payments:           common.HexToAddress("0x09a0fDc2723fAd1A7b8e3e00eE5DF73841df55a0"),
		StateView:          common.HexToAddress("0x9BF9e67e83EC8613883FDdDec4D3b38AEE937177"),
		PDPVerifier:        common.HexToAddress("0x85e366Cf9DD2c0aE37E963d9556F5f4718d6417C"),
		SPRegistry:         common.HexToAddress("0x839e5c9988e4e9977d40708d0094103c0839Ac9D"),
		Endorsements:       common.HexToAddress("0xAA2f7CfC7ecAc616EC9C1f6d700fAd19087FAC84"),
		USDFC:              common.HexToAddress("0xb3042734b608a1B16e9e86B374A3f3e389B4cDf0"),
		Multicall3:         common.HexToAddress("0xcA11bde05977b3631167028862bE2a173976CA11"),
		SessionKeyRegistry: common.HexToAddress("0x518411c2062E119Aaf7A8B12A2eDf9a939347655"),
	},
}

// Addresses returns the built-in deployment snapshot for this chain. Runtime
// consumers should resolve current dependent addresses from FWSS. It returns a
// zero-value struct for chains without known addresses.
func (c Chain) Addresses() ContractAddresses {
	if c < chainCount {
		return knownAddresses[c]
	}
	return ContractAddresses{}
}
