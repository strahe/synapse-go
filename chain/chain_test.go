package chain

import (
	"errors"
	"math"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

func TestChainID(t *testing.T) {
	tests := []struct {
		chain  Chain
		wantID int64
	}{
		{Mainnet, 314},
		{Calibration, 314159},
	}
	for _, tt := range tests {
		if got := tt.chain.ChainID(); got != tt.wantID {
			t.Errorf("%s.ChainID() = %d, want %d", tt.chain, got, tt.wantID)
		}
	}
}

func TestChainID_Unknown(t *testing.T) {
	if got := Chain(99).ChainID(); got != 0 {
		t.Errorf("Chain(99).ChainID() = %d, want 0", got)
	}
}

func TestBigChainID(t *testing.T) {
	want := big.NewInt(314)
	if got := Mainnet.BigChainID(); got.Cmp(want) != 0 {
		t.Errorf("Mainnet.BigChainID() = %v, want %v", got, want)
	}
}

func TestChainString(t *testing.T) {
	if got := Mainnet.String(); got != "mainnet" {
		t.Errorf("Mainnet.String() = %q, want %q", got, "mainnet")
	}
	if got := Calibration.String(); got != "calibration" {
		t.Errorf("Calibration.String() = %q, want %q", got, "calibration")
	}
	// Unknown chain should not panic and should include the numeric value
	got := Chain(99).String()
	if got == "" {
		t.Error("Chain(99).String() should not be empty")
	}
}

func TestFromID(t *testing.T) {
	c, err := FromID(314)
	if err != nil || c != Mainnet {
		t.Errorf("FromID(314) = %v, %v; want Mainnet, nil", c, err)
	}

	c, err = FromID(314159)
	if err != nil || c != Calibration {
		t.Errorf("FromID(314159) = %v, %v; want Calibration, nil", c, err)
	}
}

func TestFromID_Unknown(t *testing.T) {
	_, err := FromID(999)
	if err == nil {
		t.Fatal("FromID(999) should return error")
	}
	if !errors.Is(err, ErrUnknownChain) {
		t.Errorf("error should wrap ErrUnknownChain, got %v", err)
	}
}

func TestFromID_RoundTrip(t *testing.T) {
	for _, c := range []Chain{Mainnet, Calibration} {
		got, err := FromID(c.ChainID())
		if err != nil || got != c {
			t.Errorf("FromID(%d) round-trip: got %v, %v", c.ChainID(), got, err)
		}
	}
}

func TestAddresses_NonZero(t *testing.T) {
	zero := common.Address{}
	for _, c := range []Chain{Mainnet, Calibration} {
		addrs := c.Addresses()
		if addrs.FWSS == zero {
			t.Errorf("%s: FWSS address should not be zero", c)
		}
		if addrs.Payments == zero {
			t.Errorf("%s: Payments address should not be zero", c)
		}
		if addrs.PDPVerifier == zero {
			t.Errorf("%s: PDPVerifier address should not be zero", c)
		}
		if addrs.SPRegistry == zero {
			t.Errorf("%s: SPRegistry address should not be zero", c)
		}
		if addrs.USDFC == zero {
			t.Errorf("%s: USDFC address should not be zero", c)
		}
		if addrs.Multicall3 == zero {
			t.Errorf("%s: Multicall3 address should not be zero", c)
		}
		if addrs.SessionKeyRegistry == zero {
			t.Errorf("%s: SessionKeyRegistry address should not be zero", c)
		}
	}
}

func TestAddresses_Multicall3Same(t *testing.T) {
	if Mainnet.Addresses().Multicall3 != Calibration.Addresses().Multicall3 {
		t.Error("Multicall3 address should be the same on Mainnet and Calibration")
	}
}

func TestAddresses_Unknown(t *testing.T) {
	addrs := Chain(99).Addresses()
	if addrs.FWSS != (common.Address{}) {
		t.Error("unknown chain should return zero addresses")
	}
}

func TestGenesisTimestamp(t *testing.T) {
	if got := Mainnet.GenesisTimestamp(); got != 1598306400 {
		t.Errorf("Mainnet genesis = %d, want 1598306400", got)
	}
	if got := Calibration.GenesisTimestamp(); got != 1667326380 {
		t.Errorf("Calibration genesis = %d, want 1667326380", got)
	}
	if got := Chain(99).GenesisTimestamp(); got != 0 {
		t.Errorf("unknown chain genesis = %d, want 0", got)
	}
}

func TestCurrentEpoch_Positive(t *testing.T) {
	for _, c := range []Chain{Mainnet, Calibration} {
		epoch := CurrentEpoch(c)
		if epoch.Sign() <= 0 {
			t.Errorf("%s: CurrentEpoch() = %v, want > 0", c, epoch)
		}
	}
}

func TestCurrentEpoch_Unknown(t *testing.T) {
	epoch := CurrentEpoch(Chain(99))
	if epoch.Sign() != 0 {
		t.Errorf("unknown chain CurrentEpoch() = %v, want 0", epoch)
	}
}

func TestEpochTimeRoundTrip(t *testing.T) {
	for _, c := range []Chain{Mainnet, Calibration} {
		now := time.Now()
		epoch := TimeToEpoch(c, now)
		got := EpochToTime(c, epoch)

		// The round-trip should be within one epoch duration
		diff := now.Sub(got)
		if diff < 0 {
			diff = -diff
		}
		if diff > EpochDuration {
			t.Errorf("%s: round-trip error %v exceeds epoch duration", c, diff)
		}
	}
}

func TestEpochToTime_KnownValue(t *testing.T) {
	// Epoch 0 on mainnet should equal genesis time
	genesis := time.Unix(1598306400, 0)
	got := EpochToTime(Mainnet, big.NewInt(0))
	if !got.Equal(genesis) {
		t.Errorf("EpochToTime(Mainnet, 0) = %v, want %v", got, genesis)
	}

	// Epoch 1 should be 30 seconds after genesis
	got = EpochToTime(Mainnet, big.NewInt(1))
	want := genesis.Add(30 * time.Second)
	if !got.Equal(want) {
		t.Errorf("EpochToTime(Mainnet, 1) = %v, want %v", got, want)
	}
}

func TestEpochToTime_Unknown(t *testing.T) {
	got := EpochToTime(Chain(99), big.NewInt(100))
	if !got.IsZero() {
		t.Errorf("unknown chain EpochToTime should be zero, got %v", got)
	}
}

func TestEpochToTime_Nil(t *testing.T) {
	got := EpochToTime(Mainnet, nil)
	if !got.IsZero() {
		t.Errorf("EpochToTime with nil epoch should be zero, got %v", got)
	}
}

func TestEpochToTime_TooLarge(t *testing.T) {
	maxEpoch := (math.MaxInt64 - Mainnet.GenesisTimestamp()) / EpochDurationSeconds
	epoch := big.NewInt(maxEpoch + 1)

	got := EpochToTime(Mainnet, epoch)
	if !got.IsZero() {
		t.Errorf("EpochToTime with overflowing epoch should be zero, got %v", got)
	}
}

func TestEpochConstants(t *testing.T) {
	if got := int64(EpochDuration / time.Second); got != EpochDurationSeconds {
		t.Errorf("EpochDurationSeconds = %d, want %d", EpochDurationSeconds, got)
	}

	if got := int64((24 * time.Hour) / EpochDuration); got != EpochsPerDay {
		t.Errorf("EpochsPerDay = %d, want %d", EpochsPerDay, got)
	}

	if got := int64(time.Hour / EpochDuration); got != EpochsPerHour {
		t.Errorf("EpochsPerHour = %d, want %d", EpochsPerHour, got)
	}

	if got := 30 * EpochsPerDay; got != EpochsPerMonth {
		t.Errorf("EpochsPerMonth = %d, want %d", EpochsPerMonth, got)
	}
}

func TestEpochConversions(t *testing.T) {
	if got := EpochsToHours(big.NewInt(240)); got.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("EpochsToHours(240) = %s, want 2", got)
	}
	if got := EpochsToDays(big.NewInt(8640)); got.Cmp(big.NewInt(3)) != 0 {
		t.Fatalf("EpochsToDays(8640) = %s, want 3", got)
	}
	if got := EpochsToHours(nil); got != nil {
		t.Fatalf("EpochsToHours(nil) = %v, want nil", got)
	}

	maxUint256 := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 256), big.NewInt(1))
	got := EpochsToDays(maxUint256)
	if got.Cmp(maxUint256) != 0 {
		t.Fatalf("EpochsToDays(maxUint256) = %s, want unchanged", got)
	}
	got.SetInt64(0)
	if maxUint256.Sign() == 0 {
		t.Fatal("EpochsToDays returned aliased maxUint256")
	}
}

func TestTimeToEpoch_Unknown(t *testing.T) {
	got := TimeToEpoch(Chain(99), time.Now())
	if got.Sign() != 0 {
		t.Errorf("unknown chain TimeToEpoch should be zero, got %v", got)
	}
}

func TestDefaultRPCURL(t *testing.T) {
	if got := Mainnet.DefaultRPCURL(); got == "" {
		t.Error("Mainnet RPC URL should not be empty")
	}
	if got := Calibration.DefaultRPCURL(); got == "" {
		t.Error("Calibration RPC URL should not be empty")
	}
	if got := Chain(99).DefaultRPCURL(); got != "" {
		t.Errorf("unknown chain RPC URL should be empty, got %q", got)
	}
}

func TestSizes(t *testing.T) {
	if KiB != 1024 {
		t.Errorf("KiB = %d, want 1024", KiB)
	}
	if MiB != 1024*1024 {
		t.Errorf("MiB = %d, want %d", MiB, 1024*1024)
	}
	if GiB != 1024*1024*1024 {
		t.Errorf("GiB = %d, want %d", GiB, 1024*1024*1024)
	}
	if MaxUploadSize <= 0 {
		t.Error("MaxUploadSize should be positive")
	}
	if MaxUploadSize >= GiB {
		t.Error("MaxUploadSize should be less than 1 GiB due to fr32 padding")
	}
	if MinUploadSize != 127 {
		t.Errorf("MinUploadSize = %d, want 127", MinUploadSize)
	}
	if BytesPerLeaf != 32 {
		t.Errorf("BytesPerLeaf = %d, want 32", BytesPerLeaf)
	}
}
