package adapters

import (
	"math/big"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

const daysPerMonth int64 = chain.EpochsPerMonth / chain.EpochsPerDay

func buildPricingInfo(p *warmstorage.ServicePrice) storage.PricingInfo {
	if p == nil {
		return storage.PricingInfo{}
	}
	// Both noCDN and withCDN reuse the same base per-TiB storage rate.
	// CDN-specific fixed lockup is applied later during cost calculation,
	// not baked into the per-TiB pricing view.
	noCDN := perTiBGranularities(p.PricePerTiBPerMonthNoCDN, p.EpochsPerMonth)
	withCDN := perTiBGranularities(p.PricePerTiBPerMonthNoCDN, p.EpochsPerMonth)
	return storage.PricingInfo{
		NoCDN:        noCDN,
		WithCDN:      withCDN,
		TokenAddress: p.TokenAddress,
		TokenSymbol:  "USDFC",
	}
}

func perTiBGranularities(perMonth, epochsPerMonth *big.Int) storage.PricePerTiB {
	out := storage.PricePerTiB{}
	if perMonth == nil {
		return out
	}
	out.PerMonth = new(big.Int).Set(perMonth)
	if epochsPerMonth != nil && epochsPerMonth.Sign() > 0 {
		out.PerEpoch = new(big.Int).Quo(perMonth, epochsPerMonth)
		out.PerDay = new(big.Int).Quo(perMonth, big.NewInt(daysPerMonth))
	}
	return out
}

func buildServiceParameters(p *warmstorage.ServicePrice) storage.ServiceParameters {
	out := storage.ServiceParameters{
		EpochDuration: chain.EpochDurationSeconds,
		MinUploadSize: chain.MinUploadSize,
		MaxUploadSize: chain.MaxUploadSize,
	}
	if p != nil && p.EpochsPerMonth != nil {
		out.EpochsPerMonth = p.EpochsPerMonth.Int64()
		out.EpochsPerDay = out.EpochsPerMonth / daysPerMonth
	}
	return out
}
