package synapse

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	icurio "github.com/strahe/synapse-go/internal/curio"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/filbeam"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// initServices initialises all sub-services eagerly. It is called once by
// New() before the Client is returned to the caller, so every getter is a
// simple field read with no synchronisation overhead.
func (c *Client) initServices() error {
	ws, err := warmstorage.New(warmstorage.Options{
		Client:       c.ethClient,
		Backend:      c.ethClient,
		ChainID:      types.ChainID(c.selectedChain.ChainID()),
		FWSS:         c.addresses.FWSS,
		ViewContract: c.addresses.ViewContract,
		PDPVerifier:  c.addresses.PDPVerifier,
		Signer:       c.evmSigner,
		Logger:       c.logger,
		NonceManager: c.nonces,
		Lifecycle:    c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create warmstorage service: %w", err)
	}
	c.warmStorage = ws

	spReg, err := spregistry.New(spregistry.Options{
		Client:    c.ethClient,
		Address:   c.addresses.SPRegistry,
		Lifecycle: c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create spregistry service: %w", err)
	}
	c.spRegistry = spReg

	pay, err := payments.New(payments.Options{
		Backend:            c.ethClient,
		ChainID:            types.ChainID(c.selectedChain.ChainID()),
		FilPayAddress:      c.addresses.Payments,
		WarmStorageAddress: c.addresses.FWSS,
		USDFCTokenAddress:  c.addresses.USDFC,
		Signer:             c.evmSigner,
		Logger:             c.logger,
		NonceManager:       c.nonces,
		Lifecycle:          c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create payments service: %w", err)
	}
	c.payments = pay

	sk, err := sessionkey.New(sessionkey.Options{
		Backend:         c.ethClient,
		ChainID:         types.ChainID(c.selectedChain.ChainID()),
		RegistryAddress: c.addresses.SessionKeyRegistry,
		Signer:          c.evmSigner,
		Logger:          c.logger,
		NonceManager:    c.nonces,
		Lifecycle:       c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create sessionkey service: %w", err)
	}
	c.sessionKey = sk

	fb, err := filbeam.New(filbeam.Options{
		Chain:      c.selectedChain,
		HTTPClient: c.httpClient,
		Logger:     c.logger,
		Lifecycle:  c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create filbeam service: %w", err)
	}
	c.filbeam = fb

	costsvc, err := costs.New(costs.Options{
		Chain:       c.selectedChain,
		WarmStorage: ws,
		Payments:    pay,
		Caller:      c.ethClient,
		Logger:      c.logger,
		Lifecycle:   c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create costs service: %w", err)
	}
	c.costs = costsvc

	if c.addresses.PDPVerifier != (common.Address{}) {
		caller, err := pdpverifier.NewPDPVerifierCaller(c.addresses.PDPVerifier, c.ethClient)
		if err != nil {
			return fmt.Errorf("create pdpverifier caller: %w", err)
		}
		c.pdpReader = &pdpVerifierAdapter{caller: caller, backend: c.ethClient}
	}

	resolver, err := storage.NewServiceResolver(storage.ServiceResolverOptions{
		Payer:       c.evmSigner.EVMAddress(),
		SPRegistry:  spReg,
		WarmStorage: ws,
		NewContext: func(sel storage.ResolvedUploadContext, opts *storage.UploadOptions) (storage.UploadContext, error) {
			var curioOpts []icurio.Option
			if c.logger != nil {
				curioOpts = append(curioOpts, icurio.WithLogger(c.logger))
			}
			if c.httpClient != nil {
				curioOpts = append(curioOpts, icurio.WithHTTPClient(c.httpClient))
			}
			curioClient, err := icurio.New(sel.Provider.ServiceURL, curioOpts...)
			if err != nil {
				return nil, fmt.Errorf("create curio client for %s: %w", sel.Provider.ServiceURL, err)
			}
			ctxOpts := []storage.ContextOption{
				storage.WithPayer(c.evmSigner.EVMAddress()),
				storage.WithChainID(types.ChainID(c.selectedChain.ChainID())),
				storage.WithRecordKeeper(c.addresses.FWSS),
				storage.WithDataSetMetadata(sel.DataSetMetadata),
				storage.WithCDN(opts != nil && opts.WithCDN),
				storage.WithPDPVerifierReader(c.pdpReader),
				storage.WithPDPConfigReader(ws),
				storage.WithFWSSTerminator(ws),
			}
			if sel.DataSetID != nil {
				ctxOpts = append(ctxOpts, storage.WithDataSetID(*sel.DataSetID))
			}
			if sel.ClientDataSetID != nil {
				ctxOpts = append(ctxOpts, storage.WithClientDataSetID(sel.ClientDataSetID))
			}
			return storage.NewContext(
				sel.Provider,
				curioClient,
				c.evmSigner,
				ctxOpts...,
			)
		},
	})
	if err != nil {
		return fmt.Errorf("create storage resolver: %w", err)
	}
	storageOpts := storage.Options{
		Resolver:   resolver,
		HTTPClient: c.httpClient,
		Source:     c.source,
		Lifecycle:  c.lifecycle,

		DataSetFinder:     &dataSetFinderAdapter{ws: ws},
		StorageInfoReader: &storageInfoAdapter{ws: ws, sp: c.spRegistry, pay: c.payments, usdfcToken: c.addresses.USDFC, fwss: c.addresses.FWSS},
		DataSetTerminator: ws,
		CostCalculator:    &costsAdapter{c: c.costs},
		PaymentsFunder:    &paymentsFunderAdapter{p: c.payments},
		SignerAddress:     c.evmSigner.EVMAddress(),
	}
	if c.pdpReader != nil {
		// Guard against Go's typed-nil-interface trap: assigning a nil
		// *pdpVerifierAdapter directly to the interface field would yield
		// a non-nil DataSetSizeReader whose underlying value is nil,
		// causing GetDataSetSizeBytes to panic when PDPVerifier is
		// unwired.
		storageOpts.DataSetSizeReader = c.pdpReader
	}
	svc, err := storage.New(storageOpts)
	if err != nil {
		return fmt.Errorf("create storage service: %w", err)
	}
	c.storage = svc

	return nil
}

// WarmStorage returns the [warmstorage.Service].
func (c *Client) WarmStorage() *warmstorage.Service {
	return c.warmStorage
}

// SPRegistry returns the [spregistry.Service].
func (c *Client) SPRegistry() *spregistry.Service {
	return c.spRegistry
}

// Payments returns the [payments.Service].
func (c *Client) Payments() *payments.Service {
	return c.payments
}

// SessionKey returns the [sessionkey.Service].
func (c *Client) SessionKey() *sessionkey.Service {
	return c.sessionKey
}

// Costs returns the [costs.Service].
func (c *Client) Costs() *costs.Service {
	return c.costs
}

// FilBeam returns the [filbeam.Service].
func (c *Client) FilBeam() *filbeam.Service {
	return c.filbeam
}

// Storage returns the [storage.Service].
//
// The service is wired with a [storage.ServiceResolver] that uses
// [WarmStorage] and [SPRegistry]. A per-provider curio client is
// created inside the [storage.ContextFactory] closure on each upload.
func (c *Client) Storage() *storage.Service {
	return c.storage
}

// GetProviderInfo is a convenience shortcut for looking up a storage
// provider on [SPRegistry]. `idOrAddress` may be either a numeric
// [types.ProviderID] or a [common.Address] — any other type returns
// [spregistry.ErrInvalidArgument].
//
// Mirrors the TS synapse.getProviderInfo helper.
func (c *Client) GetProviderInfo(ctx context.Context, idOrAddress any) (*spregistry.ProviderInfo, error) {
	switch v := idOrAddress.(type) {
	case types.ProviderID:
		return c.spRegistry.GetProvider(ctx, v)
	case common.Address:
		return c.spRegistry.GetProviderByAddress(ctx, v)
	default:
		return nil, fmt.Errorf("synapse.GetProviderInfo: %w: unsupported idOrAddress type %T", spregistry.ErrInvalidArgument, v)
	}
}
