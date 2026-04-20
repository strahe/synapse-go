package synapse

import (
	"fmt"

	icurio "github.com/strahe/synapse-go/internal/curio"

	"github.com/strahe/synapse-go/costs"
	"github.com/strahe/synapse-go/filbeam"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/sessionkey"
	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/warmstorage"
)

// initServices initialises all sub-services eagerly. It is called once by
// New() before the Client is returned to the caller, so every getter is a
// simple field read with no synchronisation overhead.
func (c *Client) initServices() error {
	ws, err := warmstorage.New(warmstorage.Options{
		Client:       c.ethClient,
		FWSS:         c.addresses.FWSS,
		ViewContract: c.addresses.ViewContract,
	})
	if err != nil {
		return fmt.Errorf("create warmstorage service: %w", err)
	}
	c.warmStorage = ws

	spReg, err := spregistry.New(spregistry.Options{
		Client:  c.ethClient,
		Address: c.addresses.SPRegistry,
	})
	if err != nil {
		return fmt.Errorf("create spregistry service: %w", err)
	}
	c.spRegistry = spReg

	pay, err := payments.New(payments.Options{
		Backend:       c.ethClient,
		ChainID:       c.selectedChain.BigChainID(),
		FilPayAddress: c.addresses.Payments,
		Signer:        c.evmSigner,
		Logger:        c.logger,
		NonceManager:  c.nonces,
	})
	if err != nil {
		return fmt.Errorf("create payments service: %w", err)
	}
	c.payments = pay

	sk, err := sessionkey.New(sessionkey.Options{
		Backend:         c.ethClient,
		ChainID:         c.selectedChain.BigChainID(),
		RegistryAddress: c.addresses.SessionKeyRegistry,
		Signer:          c.evmSigner,
		Logger:          c.logger,
		NonceManager:    c.nonces,
	})
	if err != nil {
		return fmt.Errorf("create sessionkey service: %w", err)
	}
	c.sessionKey = sk

	fb, err := filbeam.New(filbeam.Options{
		Chain:      c.selectedChain,
		HTTPClient: c.httpClient,
		Logger:     c.logger,
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
	})
	if err != nil {
		return fmt.Errorf("create costs service: %w", err)
	}
	c.costs = costsvc

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
			return storage.NewContext(
				sel.Provider,
				curioClient,
				c.evmSigner,
				storage.WithPayer(c.evmSigner.EVMAddress()),
				storage.WithChainID(c.selectedChain.BigChainID()),
				storage.WithRecordKeeper(c.addresses.FWSS),
				storage.WithDataSetID(sel.DataSetID),
				storage.WithClientDataSetID(sel.ClientDataSetID),
				storage.WithDataSetMetadata(sel.DataSetMetadata),
				storage.WithCDN(opts != nil && opts.WithCDN),
			)
		},
	})
	if err != nil {
		return fmt.Errorf("create storage resolver: %w", err)
	}
	managerOpts := []storage.Option{storage.WithUploadResolver(resolver)}
	if c.httpClient != nil {
		managerOpts = append(managerOpts, storage.WithHTTPClient(c.httpClient))
	}
	if c.source != "" {
		managerOpts = append(managerOpts, storage.WithSource(c.source))
	}
	c.storage = storage.NewManager(managerOpts...)

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

// Storage returns the [storage.Manager].
//
// The manager is wired with a [storage.ServiceResolver] that uses
// [WarmStorage] and [SPRegistry]. A per-provider curio client is
// created inside the [storage.ContextFactory] closure on each upload.
func (c *Client) Storage() *storage.Manager {
	return c.storage
}
