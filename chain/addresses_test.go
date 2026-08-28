package chain

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestKnownEndorsementsAddresses(t *testing.T) {
	tests := []struct {
		chain Chain
		want  common.Address
	}{
		{chain: Mainnet, want: common.HexToAddress("0x59eFa2e8324E1551d46010d7B0B140eE2F5c726b")},
		{chain: Calibration, want: common.HexToAddress("0xAA2f7CfC7ecAc616EC9C1f6d700fAd19087FAC84")},
	}
	for _, tt := range tests {
		t.Run(tt.chain.String(), func(t *testing.T) {
			if got := tt.chain.Addresses().Endorsements; got != tt.want {
				t.Fatalf("Endorsements = %s, want %s", got, tt.want)
			}
		})
	}
}
