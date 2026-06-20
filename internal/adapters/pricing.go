package adapters

import (
	"context"
	"math/big"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

const daysPerMonth int64 = chain.EpochsPerMonth / chain.EpochsPerDay

func buildPricingInfo(p *warmstorage.PriceList) storage.PricingInfo {
	if p == nil {
		return storage.PricingInfo{}
	}
	// Both noCDN and withCDN reuse the same base per-TiB storage rate.
	// CDN-specific fixed lockup is applied later during cost calculation,
	// not baked into the per-TiB pricing view.
	noCDN := perTiBGranularities(p.Rates.StoragePerTiBPerMonth)
	withCDN := perTiBGranularities(p.Rates.StoragePerTiBPerMonth)
	return storage.PricingInfo{
		NoCDN:        noCDN,
		WithCDN:      withCDN,
		TokenAddress: p.Token,
		TokenSymbol:  "USDFC",
		PriceList:    p,
	}
}

func perTiBGranularities(perMonth *big.Int) storage.PricePerTiB {
	out := storage.PricePerTiB{}
	if perMonth == nil {
		return out
	}
	out.PerMonth = new(big.Int).Set(perMonth)
	out.PerEpoch = new(big.Int).Quo(perMonth, big.NewInt(chain.EpochsPerMonth))
	out.PerDay = new(big.Int).Quo(perMonth, big.NewInt(daysPerMonth))
	return out
}

func buildServiceParameters() storage.ServiceParameters {
	return storage.ServiceParameters{
		EpochsPerMonth: chain.EpochsPerMonth,
		EpochsPerDay:   chain.EpochsPerDay,
		EpochDuration:  chain.EpochDurationSeconds,
		MinUploadSize:  chain.MinUploadSize,
		MaxUploadSize:  chain.MaxUploadSize,
	}
}

type approvalLockupPeriodReader struct {
	ws priceListReader
}

type priceListReader interface {
	GetPriceList(context.Context) (*warmstorage.PriceList, error)
}

// NewApprovalLockupPeriodReader returns the payments approval lockup reader
// backed by WarmStorage pricing.
func NewApprovalLockupPeriodReader(ws priceListReader) payments.ApprovalLockupPeriodReader {
	return &approvalLockupPeriodReader{ws: ws}
}

func (a *approvalLockupPeriodReader) ApprovalLockupPeriod(ctx context.Context) (*big.Int, error) {
	priceList, err := a.ws.GetPriceList(ctx)
	if err != nil {
		return nil, err
	}
	if priceList == nil || priceList.Lockups.DefaultLockupPeriod == nil {
		return nil, nil
	}
	return new(big.Int).Set(priceList.Lockups.DefaultLockupPeriod), nil
}
