package synapse

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/filbeam"
	"github.com/strahe/synapse-go/internal/adapters"
	"github.com/strahe/synapse-go/internal/contracts/pdpverifier"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/pdp"
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
		Client:       c.ethClient,
		Address:      c.addresses.SPRegistry,
		ChainID:      types.ChainID(c.selectedChain.ChainID()),
		Backend:      c.ethClient,
		Signer:       c.evmSigner,
		NonceManager: c.nonces,
		Logger:       c.logger,
		Lifecycle:    c.lifecycle,
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
		c.pdpReader = adapters.NewPDPVerifierReader(caller, c.ethClient)
	}

	resolver, err := storage.NewServiceResolver(storage.ServiceResolverOptions{
		Payer:       c.evmSigner.EVMAddress(),
		SPRegistry:  spReg,
		WarmStorage: ws,
		NewContext: func(sel storage.ResolvedUploadContext, opts *storage.UploadOptions) (storage.UploadContext, error) {
			var pdpOpts []pdp.Option
			if c.logger != nil {
				pdpOpts = append(pdpOpts, pdp.WithLogger(c.logger))
			}
			if c.httpClient != nil {
				pdpOpts = append(pdpOpts, pdp.WithHTTPClient(c.httpClient))
			}
			pdpClient, err := pdp.New(sel.Provider.ServiceURL, pdpOpts...)
			if err != nil {
				return nil, fmt.Errorf("create PDP client for %s: %w", sel.Provider.ServiceURL, err)
			}
			effectiveCDN := c.withCDN
			if opts != nil && opts.WithCDN != nil {
				effectiveCDN = *opts.WithCDN
			}
			ctxOpts := []storage.ContextOption{
				storage.WithPayer(c.evmSigner.EVMAddress()),
				storage.WithChainID(types.ChainID(c.selectedChain.ChainID())),
				storage.WithRecordKeeper(c.addresses.FWSS),
				storage.WithDataSetMetadata(sel.DataSetMetadata),
				storage.WithCDN(effectiveCDN),
				storage.WithPDPVerifierReader(c.pdpReader),
				storage.WithPDPConfigReader(ws),
				storage.WithFWSSTerminator(ws),
			}
			if sel.DataSetID != nil {
				ctxOpts = append(ctxOpts, storage.WithDataSetID(*sel.DataSetID))
			}
			if sel.ClientDataSetID != nil {
				ctxOpts = append(ctxOpts, storage.WithClientDataSetID(*sel.ClientDataSetID))
			}
			return storage.NewContext(
				sel.Provider,
				pdpClient,
				c.evmSigner,
				ctxOpts...,
			)
		},
	})
	if err != nil {
		return fmt.Errorf("create storage resolver: %w", err)
	}
	storageOpts := storage.Options{
		Resolver:             resolver,
		HTTPClient:           c.httpClient,
		Source:               c.source,
		DefaultWithCDN:       c.withCDN,
		AllowPrivateNetworks: c.allowPrivateNetworks,
		Lifecycle:            c.lifecycle,

		DataSetFinder:     adapters.NewDataSetFinder(ws),
		StorageInfoReader: adapters.NewStorageInfoReader(ws, spReg, pay, c.addresses.USDFC, c.addresses.FWSS),
		DataSetTerminator: ws,
		FWSSDataSetReader: ws,
		CostCalculator:    adapters.NewCostCalculator(costsvc),
		PaymentsFunder:    adapters.NewPaymentsFunder(pay),
		SignerAddress:     c.evmSigner.EVMAddress(),
	}
	if c.pdpReader != nil {
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
// [Client.WarmStorage] and [Client.SPRegistry]. A per-provider PDP client is
// created inside the [storage.ContextFactory] closure on each upload.
func (c *Client) Storage() *storage.Service {
	return c.storage
}

// GetProviderInfoByID looks up a storage provider on [Client.SPRegistry] by its
// numeric [types.BigInt] id.
func (c *Client) GetProviderInfoByID(ctx context.Context, id types.BigInt) (*spregistry.ProviderInfo, error) {
	return c.spRegistry.GetProvider(ctx, id)
}

// GetProviderInfoByAddress looks up a storage provider on [Client.SPRegistry] by
// its service-provider [common.Address].
func (c *Client) GetProviderInfoByAddress(ctx context.Context, addr common.Address) (*spregistry.ProviderInfo, error) {
	return c.spRegistry.GetProviderByAddress(ctx, addr)
}
