package synapse

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	sdktypes "github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// dataSetFinderAdapter adapts *warmstorage.Service to
// storage.DataSetFinder. storage.DataSetInfo aliases
// warmstorage.EnhancedDataSetInfo, so no conversion is required.
type dataSetFinderAdapter struct {
	ws *warmstorage.Service
}

func (a *dataSetFinderAdapter) FindDataSets(ctx context.Context, payer common.Address, onlyManaged bool) ([]*storage.DataSetInfo, error) {
	return a.ws.GetClientDataSetsWithDetails(ctx, payer, onlyManaged)
}

// storageInfoAdapter assembles the TS-parity StorageInfo view by
// parallel-fetching pricing, approved providers, client allowances and
// the PDPConfig.
type storageInfoAdapter struct {
	ws         *warmstorage.Service
	sp         *spregistry.Service
	pay        *payments.Service
	usdfcToken common.Address
	fwss       common.Address
}

func (a *storageInfoAdapter) GetStorageInfo(ctx context.Context, client common.Address) (*storage.StorageInfo, error) {
	var (
		price     *warmstorage.ServicePrice
		providers []spregistry.PDPProvider
		approval  *payments.OperatorApproval
		mu        sync.Mutex
		errs      []error
		wg        sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		p, err := a.ws.GetServicePrice(ctx)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			errs = append(errs, fmt.Errorf("GetServicePrice: %w", err))
			return
		}
		price = p
	}()

	go func() {
		defer wg.Done()
		var collected []spregistry.PDPProvider
		for id, err := range a.ws.IterateAllApprovedProviderIDs(ctx) {
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("GetApprovedProviderIDs: %w", err))
				mu.Unlock()
				return
			}
			p, err := a.sp.GetPDPProvider(ctx, id)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("GetPDPProvider(%d): %w", id, err))
				mu.Unlock()
				return
			}
			// Filter out zero-address providers (unregistered / tombstoned).
			// Mirrors TS manager.ts:1013 which excludes providers whose
			// serviceProvider address is the zero address.
			if p == nil || p.Info.ServiceProvider == (common.Address{}) {
				continue
			}
			collected = append(collected, *p)
		}
		mu.Lock()
		defer mu.Unlock()
		providers = collected
	}()

	if client != (common.Address{}) && a.pay != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ap, err := a.pay.ServiceApproval(ctx, a.usdfcToken, client, a.fwss)
			if err != nil {
				// TS parity: approval is best-effort; failure returns
				// allowances=nil rather than aborting the whole call.
				// See synapse-sdk/.../storage/manager.ts:1003-1007.
				return
			}
			mu.Lock()
			defer mu.Unlock()
			approval = ap
		}()
	}

	wg.Wait()

	if len(errs) > 0 {
		return nil, fmt.Errorf("storageInfoAdapter.GetStorageInfo: %w", errors.Join(errs...))
	}

	var allowances *storage.Allowances
	if approval != nil {
		allowances = &storage.Allowances{
			Service:         a.fwss,
			IsApproved:      approval.IsApproved,
			RateAllowance:   approval.RateAllowance,
			LockupAllowance: approval.LockupAllowance,
			RateUsed:        approval.RateUsage,
			LockupUsed:      approval.LockupUsage,
			MaxLockupPeriod: approval.MaxLockupPeriod,
		}
	}

	return &storage.StorageInfo{
		Pricing:           buildPricingInfo(price),
		Providers:         providers,
		ServiceParameters: buildServiceParameters(price),
		Allowances:        allowances,
	}, nil
}

func buildPricingInfo(p *warmstorage.ServicePrice) storage.PricingInfo {
	if p == nil {
		return storage.PricingInfo{}
	}
	// TS parity: both noCDN and withCDN pricing derive from
	// PricePerTiBPerMonthNoCDN. The CDN fixed-lockup is applied
	// separately during cost calculation, not baked into per-TiB
	// pricing. See synapse-sdk/.../storage/manager.ts:1035-1056.
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
		out.PerDay = new(big.Int).Quo(perMonth, big.NewInt(30))
	}
	return out
}

func buildServiceParameters(p *warmstorage.ServicePrice) storage.ServiceParameters {
	out := storage.ServiceParameters{
		EpochDuration: 30,
		MinUploadSize: chain.MinUploadSize,
		MaxUploadSize: chain.MaxUploadSize,
	}
	if p != nil && p.EpochsPerMonth != nil {
		out.EpochsPerMonth = p.EpochsPerMonth.Int64()
		out.EpochsPerDay = out.EpochsPerMonth / 30
	}
	return out
}

// costsAdapter adapts *costs.Service to storage.MultiCostCalculator,
// translating storage.ContextCostRef -> costs.MultiContextRef.
type costsAdapter struct {
	c *costs.Service
}

func (a *costsAdapter) CalculateMultiContextCosts(
	ctx context.Context,
	payer common.Address,
	dataSizeBytes *big.Int,
	refs []storage.ContextCostRef,
	opts storage.MultiCostOptions,
) (*storage.MultiContextCosts, error) {
	out := make([]costs.MultiContextRef, 0, len(refs))
	for _, r := range refs {
		currentSize := r.CurrentDataSetSizeBytes
		if r.DataSetID == nil {
			currentSize = nil
		}
		// The manager-level EnableCDN flag force-enables CDN across the
		// batch. Per-ref WithCDN still matters for mixed batches when the
		// caller leaves EnableCDN false.
		out = append(out, costs.MultiContextRef{
			IsNewDataSet:            r.DataSetID == nil,
			CurrentDataSetSizeBytes: currentSize,
			WithCDN:                 r.WithCDN || opts.EnableCDN,
		})
	}
	got, err := a.c.CalculateMultiContextCosts(ctx, payer, dataSizeBytes, out, &costs.UploadCostOptions{
		RunwayEpochs: opts.ExtraRunwayEpochs,
		BufferEpochs: opts.BufferEpochs,
	})
	if err != nil {
		return nil, err
	}
	return &storage.MultiContextCosts{
		RatePerEpoch:         got.RatePerEpoch,
		RatePerMonth:         got.RatePerMonth,
		DepositNeeded:        got.DepositNeeded,
		NeedsFWSSMaxApproval: got.NeedsFWSSMaxApproval,
		Ready:                got.Ready,
	}, nil
}

// paymentsFunderAdapter adapts *payments.Service to storage.PaymentsFunder.
type paymentsFunderAdapter struct {
	p *payments.Service
}

func (a *paymentsFunderAdapter) FundSync(ctx context.Context, amount *big.Int, opts ...payments.WriteOption) (*sdktypes.WriteResult, error) {
	return a.p.FundSync(ctx, amount, opts...)
}
