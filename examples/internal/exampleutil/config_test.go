package exampleutil

import (
	"testing"

	"github.com/strahe/synapse-go/chain"
)

func TestLoadEnvDefaultsRPCURL(t *testing.T) {
	cfg, err := LoadEnv(func(key string) string {
		if key == PrivateKeyEnvVar {
			return "0xabc"
		}
		return ""
	})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.RPCURL != DefaultRPCURL {
		t.Fatalf("RPCURL=%q want %q", cfg.RPCURL, DefaultRPCURL)
	}
	if cfg.Chain != nil {
		t.Fatalf("Chain=%v want nil", cfg.Chain)
	}
}

func TestLoadEnvNormalizesGlifRootRPCURL(t *testing.T) {
	cfg, err := LoadEnv(func(key string) string {
		switch key {
		case PrivateKeyEnvVar:
			return "0xabc"
		case RPCURLEnvVar:
			return "https://api.calibration.node.glif.io/"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.RPCURL != DefaultRPCURL {
		t.Fatalf("RPCURL=%q want %q", cfg.RPCURL, DefaultRPCURL)
	}
}

func TestLoadEnvAcceptsLegacyRPCURLFallback(t *testing.T) {
	cfg, err := LoadEnv(func(key string) string {
		switch key {
		case PrivateKeyEnvVar:
			return "0xabc"
		case legacyRPCURLVar:
			return "https://api.calibration.node.glif.io/"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.RPCURL != DefaultRPCURL {
		t.Fatalf("RPCURL=%q want %q", cfg.RPCURL, DefaultRPCURL)
	}
}

func TestLoadEnvPrefersNamespacedRPCURL(t *testing.T) {
	cfg, err := LoadEnv(func(key string) string {
		switch key {
		case PrivateKeyEnvVar:
			return "0xabc"
		case RPCURLEnvVar:
			return "https://api.node.glif.io/"
		case legacyRPCURLVar:
			return "https://api.calibration.node.glif.io/"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.RPCURL != "https://api.node.glif.io/rpc/v1" {
		t.Fatalf("RPCURL=%q want mainnet GLIF RPC", cfg.RPCURL)
	}
}

func TestLoadEnvRequiresPrivateKey(t *testing.T) {
	_, err := LoadEnv(func(string) string { return "" })
	if err == nil {
		t.Fatal("LoadEnv returned nil error")
	}
}

func TestLoadEnvRejectsLegacyPrivateKey(t *testing.T) {
	_, err := LoadEnv(func(key string) string {
		if key == legacyPrivateKeyVar {
			return "0xabc"
		}
		return ""
	})
	if err == nil {
		t.Fatal("LoadEnv returned nil error")
	}
}

func TestValidateUploadSize(t *testing.T) {
	if err := ValidateUploadSize("payload", MinUploadBytes); err != nil {
		t.Fatalf("ValidateUploadSize: %v", err)
	}
	if err := ValidateUploadSize("payload", MinUploadBytes-1); err == nil {
		t.Fatal("ValidateUploadSize returned nil error for small payload")
	}
	if err := ValidateUploadSize("payload", 0); err == nil {
		t.Fatal("ValidateUploadSize returned nil error for empty payload")
	}
}

func TestParseChain(t *testing.T) {
	got, err := ParseChain("calibration")
	if err != nil {
		t.Fatalf("ParseChain: %v", err)
	}
	if got == nil || *got != chain.Calibration {
		t.Fatalf("chain=%v want calibration", got)
	}
}

func TestLoadEnvUsesNamespacedChain(t *testing.T) {
	cfg, err := LoadEnv(func(key string) string {
		switch key {
		case PrivateKeyEnvVar:
			return "0xabc"
		case ChainEnvVar:
			return "mainnet"
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if cfg.Chain == nil || *cfg.Chain != chain.Mainnet {
		t.Fatalf("chain=%v want mainnet", cfg.Chain)
	}
}

func TestMetadataFlag(t *testing.T) {
	var meta MetadataFlag
	if err := meta.Set("app=example"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := meta.Set("purpose=quickstart"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got := meta.Map()
	if got["app"] != "example" || got["purpose"] != "quickstart" {
		t.Fatalf("metadata=%v", got)
	}
	if meta.String() != "app=example,purpose=quickstart" {
		t.Fatalf("String=%q", meta.String())
	}
}
