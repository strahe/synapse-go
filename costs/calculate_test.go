package costs

import (
	"math/big"
	"testing"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/warmstorage"
)

func bi(v int64) *big.Int { return big.NewInt(v) }

// usdfc returns n whole USDFC as attoUSDFC.
func usdfc(n int64) *big.Int {
	return new(big.Int).Mul(bi(n), bi(1e18))
}

// usdfcFrac returns n/10 USDFC (e.g. usdfcFrac(25) = 2.5 USDFC).
func usdfcFrac(tenths int64) *big.Int {
	return new(big.Int).Mul(bi(tenths), big.NewInt(1e17))
}

func defaultPricing() *warmstorage.ServicePrice {
	return &warmstorage.ServicePrice{
		PricePerTiBPerMonthNoCDN: usdfcFrac(25), // 2.5 USDFC/TiB/month
		MinimumPricePerMonth:     usdfcFrac(1),  // 0.1 USDFC/month
		EpochsPerMonth:           bi(chain.EpochsPerMonth),
	}
}

// --- CalculateEffectiveRate ---

func TestCalculateEffectiveRate_ExactOneTiB(t *testing.T) {
	pricing := defaultPricing()
	rate := CalculateEffectiveRate(
		bi(chain.TiB),
		pricing.PricePerTiBPerMonthNoCDN,
		pricing.MinimumPricePerMonth,
		chain.EpochsPerMonth,
	)

	// ratePerMonth = 2.5 USDFC * 1 TiB / 1 TiB = 2.5 USDFC
	if rate.RatePerMonth.Cmp(usdfcFrac(25)) != 0 {
		t.Errorf("ratePerMonth: got %s, want %s", rate.RatePerMonth, usdfcFrac(25))
	}

	// ratePerEpoch = 2.5 USDFC / epochsPerMonth
	want := new(big.Int).Div(usdfcFrac(25), bi(chain.EpochsPerMonth))
	if rate.RatePerEpoch.Cmp(want) != 0 {
		t.Errorf("ratePerEpoch: got %s, want %s", rate.RatePerEpoch, want)
	}
}

func TestCalculateEffectiveRate_SubTiB_HitsMinimum(t *testing.T) {
	pricing := defaultPricing()
	rate := CalculateEffectiveRate(
		bi(1),
		pricing.PricePerTiBPerMonthNoCDN,
		pricing.MinimumPricePerMonth,
		chain.EpochsPerMonth,
	)

	// At the minimum floor, ratePerMonth is derived from ratePerEpoch × epm so the
	// two fields are consistent.  ratePerMonth is therefore slightly less than
	// MinimumPricePerMonth by at most (epm-1) attoUSDFC due to integer truncation.
	wantMonthly := new(big.Int).Mul(rate.RatePerEpoch, big.NewInt(chain.EpochsPerMonth))
	if rate.RatePerMonth.Cmp(wantMonthly) != 0 {
		t.Errorf("ratePerMonth should equal ratePerEpoch*epm at minimum: got %s, want %s",
			rate.RatePerMonth, wantMonthly)
	}
	if rate.RatePerEpoch.Cmp(bi(1)) < 0 {
		t.Errorf("ratePerEpoch should be at least 1: got %s", rate.RatePerEpoch)
	}
}

func TestCalculateEffectiveRate_MultiTiB(t *testing.T) {
	pricing := defaultPricing()
	size := new(big.Int).Mul(bi(5), bi(chain.TiB)) // 5 TiB

	rate := CalculateEffectiveRate(
		size,
		pricing.PricePerTiBPerMonthNoCDN,
		pricing.MinimumPricePerMonth,
		chain.EpochsPerMonth,
	)

	// ratePerMonth = 2.5 * 5 = 12.5 USDFC
	if rate.RatePerMonth.Cmp(usdfcFrac(125)) != 0 {
		t.Errorf("ratePerMonth: got %s, want %s", rate.RatePerMonth, usdfcFrac(125))
	}
}

func TestCalculateEffectiveRate_ZeroSize(t *testing.T) {
	pricing := defaultPricing()
	rate := CalculateEffectiveRate(
		bi(0),
		pricing.PricePerTiBPerMonthNoCDN,
		pricing.MinimumPricePerMonth,
		chain.EpochsPerMonth,
	)

	// Zero size hits the minimum floor; ratePerMonth must equal ratePerEpoch × epm.
	wantMonthly := new(big.Int).Mul(rate.RatePerEpoch, big.NewInt(chain.EpochsPerMonth))
	if rate.RatePerMonth.Cmp(wantMonthly) != 0 {
		t.Errorf("ratePerMonth should be epoch-aligned for zero size: got %s, want %s",
			rate.RatePerMonth, wantMonthly)
	}
}

// --- CalculateAdditionalLockupRequired ---

func TestAdditionalLockup_NewDataSet(t *testing.T) {
	pricing := defaultPricing()
	sybilFee := usdfcFrac(1)

	lockup := CalculateAdditionalLockupRequired(
		bi(chain.TiB), // uploading 1 TiB
		bi(0),         // empty dataset
		pricing,
		DefaultLockupPeriod,
		sybilFee,
		true,  // new dataset
		false, // no CDN
	)

	if lockup.RateDelta.Sign() <= 0 {
		t.Errorf("rateDelta should be positive for new dataset: got %s", lockup.RateDelta)
	}

	expected := new(big.Int).Add(
		new(big.Int).Mul(lockup.RateDelta, bi(DefaultLockupPeriod)),
		sybilFee,
	)
	if lockup.TotalLockup.Cmp(expected) != 0 {
		t.Errorf("totalLockup: got %s, want %s", lockup.TotalLockup, expected)
	}
}

func TestAdditionalLockup_NewDataSet_WithCDN(t *testing.T) {
	pricing := defaultPricing()
	sybilFee := usdfcFrac(1)

	lockup := CalculateAdditionalLockupRequired(
		bi(chain.TiB),
		bi(0),
		pricing,
		DefaultLockupPeriod,
		sybilFee,
		true,
		true,
	)

	rateLockup := new(big.Int).Mul(lockup.RateDelta, bi(DefaultLockupPeriod))
	expected := new(big.Int).Add(rateLockup, cdnFixedLockup)
	expected.Add(expected, sybilFee)

	if lockup.TotalLockup.Cmp(expected) != 0 {
		t.Errorf("totalLockup with CDN: got %s, want %s", lockup.TotalLockup, expected)
	}
	if lockup.RateLockup.Cmp(rateLockup) != 0 {
		t.Errorf("RateLockup: got %s, want %s", lockup.RateLockup, rateLockup)
	}
	if lockup.CDNFixedLockup.Cmp(cdnFixedLockup) != 0 {
		t.Errorf("CDNFixedLockup: got %s, want %s", lockup.CDNFixedLockup, cdnFixedLockup)
	}
	if lockup.SybilFee.Cmp(sybilFee) != 0 {
		t.Errorf("SybilFee: got %s, want %s", lockup.SybilFee, sybilFee)
	}
}

func TestAdditionalLockup_ExistingDataSet(t *testing.T) {
	pricing := defaultPricing()

	lockup := CalculateAdditionalLockupRequired(
		bi(chain.TiB),
		bi(chain.TiB),
		pricing,
		DefaultLockupPeriod,
		usdfcFrac(1),
		false, // existing dataset
		false,
	)

	if lockup.RateDelta.Sign() < 0 {
		t.Errorf("rateDelta should not be negative: got %s", lockup.RateDelta)
	}

	// No sybil fee or CDN for existing dataset.
	expectedLockup := new(big.Int).Mul(lockup.RateDelta, bi(DefaultLockupPeriod))
	if lockup.TotalLockup.Cmp(expectedLockup) != 0 {
		t.Errorf("totalLockup for existing dataset should not include sybil: got %s, want %s",
			lockup.TotalLockup, expectedLockup)
	}
}

func TestAdditionalLockup_ExistingDataSet_NilSybilFee(t *testing.T) {
	pricing := defaultPricing()

	lockup := CalculateAdditionalLockupRequired(
		bi(chain.TiB),
		bi(0),
		pricing,
		DefaultLockupPeriod,
		nil,
		true,
		false,
	)

	expectedLockup := new(big.Int).Mul(lockup.RateDelta, bi(DefaultLockupPeriod))
	if lockup.TotalLockup.Cmp(expectedLockup) != 0 {
		t.Errorf("totalLockup with nil sybil: got %s, want %s",
			lockup.TotalLockup, expectedLockup)
	}
}

func TestAdditionalLockup_Breakdown_ExistingDataSet(t *testing.T) {
	pricing := defaultPricing()

	lockup := CalculateAdditionalLockupRequired(
		bi(chain.TiB),
		bi(chain.TiB),
		pricing,
		DefaultLockupPeriod,
		usdfcFrac(1),
		false,
		true,
	)

	if lockup.CDNFixedLockup.Sign() != 0 {
		t.Errorf("CDNFixedLockup should be 0 for existing dataset: got %s", lockup.CDNFixedLockup)
	}
	if lockup.SybilFee.Sign() != 0 {
		t.Errorf("SybilFee should be 0 for existing dataset: got %s", lockup.SybilFee)
	}
	if lockup.TotalLockup.Cmp(lockup.RateLockup) != 0 {
		t.Errorf("TotalLockup should equal RateLockup for existing dataset")
	}
}

func TestAdditionalLockup_Breakdown_SumsCorrectly(t *testing.T) {
	pricing := defaultPricing()

	lockup := CalculateAdditionalLockupRequired(
		bi(chain.TiB),
		bi(0),
		pricing,
		DefaultLockupPeriod,
		usdfcFrac(1),
		true,
		true,
	)

	expected := new(big.Int).Add(lockup.RateLockup, lockup.CDNFixedLockup)
	expected.Add(expected, lockup.SybilFee)
	if lockup.TotalLockup.Cmp(expected) != 0 {
		t.Errorf("TotalLockup != sum of components: total=%s, sum=%s",
			lockup.TotalLockup, expected)
	}

	expectedRate := new(big.Int).Mul(lockup.RateDelta, bi(DefaultLockupPeriod))
	if lockup.RateLockup.Cmp(expectedRate) != 0 {
		t.Errorf("RateLockup != rateDelta * lockupPeriod: got %s, want %s",
			lockup.RateLockup, expectedRate)
	}
}

// --- CalculateDepositNeeded ---

func TestDepositNeeded_InsufficientFunds(t *testing.T) {
	deposit := CalculateDepositNeeded(
		usdfc(10),
		bi(100),
		bi(50),
		bi(0),
		usdfc(1),
		DefaultExtraRunwayEpochs,
		DefaultBufferEpochs,
		false,
	)

	if deposit.Sign() <= 0 {
		t.Errorf("deposit should be positive when funds are insufficient: got %s", deposit)
	}
}

func TestDepositNeeded_SufficientFunds(t *testing.T) {
	huge := new(big.Int).Mul(usdfc(1_000_000), bi(1e18))
	deposit := CalculateDepositNeeded(
		usdfc(1),
		bi(1),
		bi(1),
		bi(0),
		huge,
		DefaultExtraRunwayEpochs,
		DefaultBufferEpochs,
		false,
	)

	// raw → clamped to 0; buffer = (1+1) * 5 = 10
	expectedBuffer := new(big.Int).Mul(bi(2), bi(DefaultBufferEpochs))
	if deposit.Cmp(expectedBuffer) != 0 {
		t.Errorf("deposit should equal buffer when funds are sufficient: got %s, want %s",
			deposit, expectedBuffer)
	}
}

func TestDepositNeeded_WithDebt(t *testing.T) {
	depositNoDebt := CalculateDepositNeeded(
		usdfc(10), bi(100), bi(50), bi(0), usdfc(1),
		DefaultExtraRunwayEpochs, DefaultBufferEpochs, false,
	)
	depositWithDebt := CalculateDepositNeeded(
		usdfc(10), bi(100), bi(50), usdfc(5), usdfc(1),
		DefaultExtraRunwayEpochs, DefaultBufferEpochs, false,
	)

	if depositWithDebt.Cmp(depositNoDebt) <= 0 {
		t.Errorf("deposit with debt should be larger: debt=%s, noDebt=%s",
			depositWithDebt, depositNoDebt)
	}
}

func TestDepositNeeded_BufferSkipped_NewDataSet_ZeroRate(t *testing.T) {
	depositNew := CalculateDepositNeeded(
		usdfc(10), bi(100), bi(0), bi(0), bi(0),
		DefaultExtraRunwayEpochs, DefaultBufferEpochs,
		true, // new dataset, zero current rate → buffer skipped
	)
	depositExisting := CalculateDepositNeeded(
		usdfc(10), bi(100), bi(0), bi(0), bi(0),
		DefaultExtraRunwayEpochs, DefaultBufferEpochs,
		false,
	)

	if depositNew.Cmp(depositExisting) >= 0 {
		t.Errorf("new dataset with zero rate should skip buffer and be smaller: new=%s, existing=%s",
			depositNew, depositExisting)
	}
}

func TestDepositNeeded_ZeroEverything(t *testing.T) {
	deposit := CalculateDepositNeeded(
		bi(0), bi(0), bi(0), bi(0), bi(0), 0, 0, true,
	)
	if deposit.Sign() != 0 {
		t.Errorf("deposit should be zero when all inputs are zero: got %s", deposit)
	}
}

// --- isFWSSMaxApproved ---

func TestIsFWSSMaxApproved_AllConditionsMet(t *testing.T) {
	if !isFWSSMaxApproved(true, maxUint256, maxUint256, bi(DefaultLockupPeriod)) {
		t.Error("should be approved when all conditions met")
	}
}

func TestIsFWSSMaxApproved_NotApproved(t *testing.T) {
	if isFWSSMaxApproved(false, maxUint256, maxUint256, bi(DefaultLockupPeriod)) {
		t.Error("should not be approved when approved=false")
	}
}

func TestIsFWSSMaxApproved_RateAllowanceTooLow(t *testing.T) {
	low := new(big.Int).Sub(maxUint256, bi(1))
	if isFWSSMaxApproved(true, low, maxUint256, bi(DefaultLockupPeriod)) {
		t.Error("should not be approved when rateAllowance < maxUint256")
	}
}

func TestIsFWSSMaxApproved_LockAllowanceAtThreshold(t *testing.T) {
	if !isFWSSMaxApproved(true, maxUint256, halfMaxUint256, bi(DefaultLockupPeriod)) {
		t.Error("should be approved at lockAllowance == maxUint256/2")
	}
}

func TestIsFWSSMaxApproved_LockAllowanceBelowThreshold(t *testing.T) {
	below := new(big.Int).Sub(halfMaxUint256, bi(1))
	if isFWSSMaxApproved(true, maxUint256, below, bi(DefaultLockupPeriod)) {
		t.Error("should not be approved when lockAllowance < maxUint256/2")
	}
}

func TestIsFWSSMaxApproved_MaxLockPeriodTooShort(t *testing.T) {
	if isFWSSMaxApproved(true, maxUint256, maxUint256, bi(DefaultLockupPeriod-1)) {
		t.Error("should not be approved when maxLockPeriod < DefaultLockupPeriod")
	}
}

// --- CDNFixedLockupValue ---

func TestCDNFixedLockupValue(t *testing.T) {
	v := CDNFixedLockupValue()
	want := big.NewInt(1_000_000_000_000_000_000) // 1 USDFC
	if v.Cmp(want) != 0 {
		t.Errorf("CDNFixedLockupValue() = %s, want %s", v, want)
	}
}

func TestCDNFixedLockupValue_MutationSafety(t *testing.T) {
	v1 := CDNFixedLockupValue()
	v1.SetInt64(0) // mutate the returned copy
	v2 := CDNFixedLockupValue()
	if v2.Sign() == 0 {
		t.Fatal("mutation of returned value affected global cdnFixedLockup")
	}
}

// --- CalculateDepositNeeded edge cases ---

func TestDepositNeeded_NilInputs(t *testing.T) {
	deposit := CalculateDepositNeeded(nil, nil, nil, nil, nil, 0, 0, true)
	if deposit.Sign() != 0 {
		t.Errorf("expected zero deposit for all-nil inputs, got %s", deposit)
	}
}

func TestDepositNeeded_NegativeEpochs(t *testing.T) {
	deposit := CalculateDepositNeeded(
		usdfc(10), bi(100), bi(50), bi(0), usdfc(1),
		-100, -50,
		false,
	)
	// Negative epochs clamped to 0, so runway=0 and buffer=0.
	// raw = 10e18 + 0 - 1e18 + 0 = 9e18 (clamped to non-negative); buffer = 0
	// result = 9e18 = usdfc(9)
	expected := usdfc(9)
	if deposit.Cmp(expected) != 0 {
		t.Errorf("deposit = %s, want %s", deposit, expected)
	}
}

func TestDepositNeeded_LargeValues(t *testing.T) {
	huge := new(big.Int).Lsh(big.NewInt(1), 128) // 2^128
	deposit := CalculateDepositNeeded(
		huge, huge, huge, huge, new(big.Int),
		chain.EpochsPerMonth, DefaultBufferEpochs,
		false,
	)
	// combinedRate = huge + huge = 2^129
	// runway = 2^129 * EpochsPerMonth
	// raw = huge + runway - 0 + huge = 2^129 * (1 + EpochsPerMonth)
	// buffer = 2^129 * DefaultBufferEpochs
	// result = 2^129 * (1 + EpochsPerMonth + DefaultBufferEpochs)
	combinedRate := new(big.Int).Lsh(big.NewInt(1), 129) // 2^129
	expected := new(big.Int).Mul(combinedRate,
		new(big.Int).Add(
			big.NewInt(1),
			big.NewInt(chain.EpochsPerMonth+int64(DefaultBufferEpochs)),
		),
	)
	if deposit.Cmp(expected) != 0 {
		t.Errorf("deposit = %s, want %s", deposit, expected)
	}
}

func TestDepositNeeded_ZeroTotalLockup(t *testing.T) {
	deposit := CalculateDepositNeeded(
		bi(0), bi(100), bi(50), bi(0), usdfc(100),
		0, DefaultBufferEpochs,
		false,
	)
	// raw = 0 + 0 - availableFunds (clamped to 0); buffer = combinedRate * bufferEpochs = 150 * 5 = 750
	expected := new(big.Int).Mul(bi(150), bi(DefaultBufferEpochs))
	if deposit.Cmp(expected) != 0 {
		t.Errorf("deposit = %s, want %s", deposit, expected)
	}
}

// --- CalculateEffectiveRate edge cases ---

func TestCalculateEffectiveRate_NilInputs(t *testing.T) {
	rate := CalculateEffectiveRate(bi(chain.TiB), nil, nil, 0)
	if rate.RatePerEpoch == nil || rate.RatePerMonth == nil {
		t.Fatal("nil rate fields returned")
	}
	// nil price and nil minRate both treated as 0; ratePerEpoch is clamped to bigOne=1
	if rate.RatePerEpoch.Cmp(big.NewInt(1)) != 0 {
		t.Errorf("RatePerEpoch = %s, want 1", rate.RatePerEpoch)
	}
	if rate.RatePerMonth.Sign() != 0 {
		t.Errorf("RatePerMonth = %s, want 0", rate.RatePerMonth)
	}
}

func TestCalculateEffectiveRate_NegativeEpochsPerMonth(t *testing.T) {
	rate := CalculateEffectiveRate(bi(chain.TiB), usdfcFrac(25), usdfcFrac(1), -1)
	// -1 falls back to chain.EpochsPerMonth; result must match the explicit positive call
	expected := CalculateEffectiveRate(bi(chain.TiB), usdfcFrac(25), usdfcFrac(1), chain.EpochsPerMonth)
	if rate.RatePerEpoch.Cmp(expected.RatePerEpoch) != 0 {
		t.Errorf("RatePerEpoch = %s, want %s (fallback to EpochsPerMonth)", rate.RatePerEpoch, expected.RatePerEpoch)
	}
	if rate.RatePerMonth.Cmp(expected.RatePerMonth) != 0 {
		t.Errorf("RatePerMonth = %s, want %s (fallback to EpochsPerMonth)", rate.RatePerMonth, expected.RatePerMonth)
	}
}
