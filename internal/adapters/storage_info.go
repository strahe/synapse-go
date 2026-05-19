package adapters

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// storageInfoReader assembles the TS-parity StorageInfo view by
// parallel-fetching pricing, approved providers, client allowances and
// the PDPConfig.
type storageInfoReader struct {
	ws         *warmstorage.Service
	sp         *spregistry.Service
	pay        *payments.Service
	usdfcToken common.Address
	fwss       common.Address
}

// NewStorageInfoReader returns a [storage.StorageInfoReader] that
// assembles pricing, providers and allowances from ws, sp and pay.
func NewStorageInfoReader(ws *warmstorage.Service, sp *spregistry.Service, pay *payments.Service, usdfcToken, fwss common.Address) storage.StorageInfoReader {
	return &storageInfoReader{ws: ws, sp: sp, pay: pay, usdfcToken: usdfcToken, fwss: fwss}
}

func (a *storageInfoReader) GetStorageInfo(ctx context.Context, client common.Address) (*storage.StorageInfo, error) {
	var (
		price     *warmstorage.ServicePrice
		providers []spregistry.PDPProvider
		approval  *payments.OperatorApproval
		mu        sync.Mutex
		errs      []error
		wg        sync.WaitGroup
	)
	appendErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		errs = append(errs, err)
	}

	wg.Add(2)

	go func() {
		defer wg.Done()
		p, err := a.ws.GetServicePrice(ctx)
		if err != nil {
			appendErr(fmt.Errorf("GetServicePrice: %w", err))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		price = p
	}()

	go func() {
		defer wg.Done()
		var ids []types.BigInt
		for id, err := range a.ws.IterateAllApprovedProviderIDs(ctx) {
			if err != nil {
				appendErr(fmt.Errorf("GetApprovedProviderIDs: %w", err))
				return
			}
			ids = append(ids, id)
		}
		fetched := make([]*spregistry.PDPProvider, len(ids))
		var providerWG sync.WaitGroup
		providerWG.Add(len(ids))
		for i, id := range ids {
			go func(i int, id types.BigInt) {
				defer providerWG.Done()
				p, err := a.sp.GetPDPProvider(ctx, id)
				if err != nil {
					if errors.Is(err, spregistry.ErrNotFound) {
						return
					}
					appendErr(fmt.Errorf("GetPDPProvider(%s): %w", id.String(), err))
					return
				}
				fetched[i] = p
			}(i, id)
		}
		providerWG.Wait()
		collected := make([]spregistry.PDPProvider, 0, len(fetched))
		for _, p := range fetched {
			// Filter out zero-address providers (unregistered / tombstoned).
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
				// Approval lookup is best-effort: keep allowances=nil
				// instead of aborting the whole StorageInfo call.
				return
			}
			mu.Lock()
			defer mu.Unlock()
			approval = ap
		}()
	}

	wg.Wait()

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

	info := &storage.StorageInfo{
		Pricing:           buildPricingInfo(price),
		Providers:         providers,
		ServiceParameters: buildServiceParameters(price),
		Allowances:        allowances,
	}
	if len(errs) > 0 {
		return info, fmt.Errorf("adapters.storageInfoReader.GetStorageInfo: %w", errors.Join(errs...))
	}
	return info, nil
}
