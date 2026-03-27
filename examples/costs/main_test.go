package main

import (
	"bytes"
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/costs"
)

func TestParseConfig_Defaults(t *testing.T) {
	cfg, err := parseConfig(nil, func(key string) string {
		switch key {
		case "RPC_URL":
			return "https://api.calibration.node.glif.io"
		case "PRIVATE_KEY":
			return "0xabc"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.DataSizeBytes.Int64() != defaultDataSize {
		t.Fatalf("DataSizeBytes=%s want %d", cfg.DataSizeBytes, defaultDataSize)
	}
	if cfg.Chain != nil {
		t.Fatalf("Chain=%v want nil", cfg.Chain)
	}
}

func TestParseConfig_CustomSize(t *testing.T) {
	cfg, err := parseConfig([]string{"2048"}, func(key string) string {
		switch key {
		case "RPC_URL":
			return "https://rpc.example.com"
		case "PRIVATE_KEY":
			return "0x1"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.DataSizeBytes.Int64() != 2048 {
		t.Fatalf("DataSizeBytes=%s want 2048", cfg.DataSizeBytes)
	}
}

func TestParseConfig_InvalidSize(t *testing.T) {
	_, err := parseConfig([]string{"notanumber"}, func(key string) string {
		switch key {
		case "RPC_URL":
			return "https://rpc.example.com"
		case "PRIVATE_KEY":
			return "0x1"
		default:
			return ""
		}
	})
	if err == nil || !strings.Contains(err.Error(), "invalid data size") {
		t.Fatalf("err=%v want invalid data size error", err)
	}
}

func TestParseConfig_MissingRPCURL(t *testing.T) {
	_, err := parseConfig(nil, func(key string) string {
		if key == "PRIVATE_KEY" {
			return "0x1"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "RPC_URL") {
		t.Fatalf("err=%v want RPC_URL error", err)
	}
}

func TestParseConfig_MissingPrivateKey(t *testing.T) {
	_, err := parseConfig(nil, func(key string) string {
		if key == "RPC_URL" {
			return "https://rpc.example.com"
		}
		return ""
	})
	if err == nil || !strings.Contains(err.Error(), "PRIVATE_KEY") {
		t.Fatalf("err=%v want PRIVATE_KEY error", err)
	}
}

func TestRunCosts_PrintsOutput(t *testing.T) {
	var stdout bytes.Buffer
	addr := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")

	cfg := costsConfig{
		DataSizeBytes: big.NewInt(1073741824),
	}

	fc := &fakeCostQuerier{
		result: &costs.UploadCosts{
			Rate: costs.EffectiveRate{
				RatePerEpoch: big.NewInt(100),
				RatePerMonth: big.NewInt(86400_00),
			},
			Lockup: costs.AdditionalLockup{
				RateDelta:      big.NewInt(100),
				RateLockup:     big.NewInt(50000),
				CDNFixedLockup: big.NewInt(1000000),
				SybilFee:       big.NewInt(500),
				TotalLockup:    big.NewInt(1050500),
			},
			DepositNeeded: big.NewInt(2000000),
			Ready:         false,
		},
	}
	fa := &fakeAccountQuerier{
		result: &costs.AccountSummary{
			Funds:            big.NewInt(5000000),
			AvailableFunds:   big.NewInt(3000000),
			Debt:             big.NewInt(0),
			FundedUntilEpoch: big.NewInt(999999),
		},
	}

	err := runCosts(context.Background(), cfg, addr, fc, fa, &stdout)
	if err != nil {
		t.Fatalf("runCosts: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"Upload Cost Estimate",
		"ratePerEpoch=100",
		"ratePerMonth=8640000",
		"totalLockup=1050500",
		"depositNeeded=2000000",
		"ready=false",
		"Account Summary",
		"funds=5000000",
		"available=3000000",
		"fundedUntilEpoch=999999",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

type fakeCostQuerier struct {
	result *costs.UploadCosts
}

func (f *fakeCostQuerier) GetUploadCosts(_ context.Context, _ common.Address, _ *big.Int, _ *costs.UploadCostOptions) (*costs.UploadCosts, error) {
	return f.result, nil
}

type fakeAccountQuerier struct {
	result *costs.AccountSummary
}

func (f *fakeAccountQuerier) GetAccountSummary(_ context.Context, _ common.Address) (*costs.AccountSummary, error) {
	return f.result, nil
}
