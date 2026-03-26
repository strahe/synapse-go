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

// WarmStorage returns the lazily-initialized [warmstorage.Service].
func (c *Client) WarmStorage() *warmstorage.Service {
	c.warmStorageOnce.Do(func() {
		svc, err := warmstorage.New(warmstorage.Options{
			Client:       c.ethClient,
			FWSS:         c.addresses.FWSS,
			ViewContract: c.addresses.ViewContract,
		})
		if err != nil {
			panic(fmt.Sprintf("synapse: create warmstorage service: %v", err))
		}
		c.warmStorage = svc
	})
	return c.warmStorage
}

// SPRegistry returns the lazily-initialized [spregistry.Service].
func (c *Client) SPRegistry() *spregistry.Service {
	c.spRegistryOnce.Do(func() {
		svc, err := spregistry.New(spregistry.Options{
			Client:  c.ethClient,
			Address: c.addresses.SPRegistry,
		})
		if err != nil {
			panic(fmt.Sprintf("synapse: create spregistry service: %v", err))
		}
		c.spRegistry = svc
	})
	return c.spRegistry
}

// Payments returns the lazily-initialized [payments.Service].
func (c *Client) Payments() *payments.Service {
	c.paymentsOnce.Do(func() {
		svc, err := payments.New(payments.Options{
			Backend:       c.ethClient,
			ChainID:       c.selectedChain.BigChainID(),
			FilPayAddress: c.addresses.Payments,
			Signer:        c.evmSigner,
			Logger:        c.logger,
			NonceManager:  c.nonces,
		})
		if err != nil {
			panic(fmt.Sprintf("synapse: create payments service: %v", err))
		}
		c.payments = svc
	})
	return c.payments
}

// SessionKey returns the lazily-initialized [sessionkey.Service].
func (c *Client) SessionKey() *sessionkey.Service {
	c.sessionKeyOnce.Do(func() {
		svc, err := sessionkey.New(sessionkey.Options{
			Backend:         c.ethClient,
			ChainID:         c.selectedChain.BigChainID(),
			RegistryAddress: c.addresses.SessionKeyRegistry,
			Signer:          c.evmSigner,
			Logger:          c.logger,
			NonceManager:    c.nonces,
		})
		if err != nil {
			panic(fmt.Sprintf("synapse: create sessionkey service: %v", err))
		}
		c.sessionKey = svc
	})
	return c.sessionKey
}

// Costs returns the lazily-initialized [costs.Service].
// Internally triggers [WarmStorage] and [Payments] initialization.
func (c *Client) Costs() *costs.Service {
	ws := c.WarmStorage()
	pay := c.Payments()
	c.costsOnce.Do(func() {
		svc, err := costs.NewService(
			c.selectedChain,
			ws,
			pay,
			c.ethClient,
			costs.WithLogger(c.logger),
		)
		if err != nil {
			panic(fmt.Sprintf("synapse: create costs service: %v", err))
		}
		c.costs = svc
	})
	return c.costs
}

// FilBeam returns the lazily-initialized [filbeam.Service].
func (c *Client) FilBeam() *filbeam.Service {
	c.filbeamOnce.Do(func() {
		var opts []filbeam.Option
		if c.httpClient != nil {
			opts = append(opts, filbeam.WithHTTPClient(c.httpClient))
		}
		if c.logger != nil {
			opts = append(opts, filbeam.WithLogger(c.logger))
		}
		c.filbeam = filbeam.NewService(c.selectedChain, opts...)
	})
	return c.filbeam
}

// Storage returns the lazily-initialized [storage.Manager].
//
// The manager is wired with a [storage.ServiceResolver] that uses
// [WarmStorage] and [SPRegistry]. A per-provider curio client is
// created inside the [storage.ContextFactory] closure on each upload.
func (c *Client) Storage() *storage.Manager {
	spReg := c.SPRegistry()
	ws := c.WarmStorage()
	c.storageOnce.Do(func() {
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
					return nil, fmt.Errorf("synapse: create curio client for %s: %w", sel.Provider.ServiceURL, err)
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
			panic(fmt.Sprintf("synapse: create storage resolver: %v", err))
		}

		managerOpts := []storage.Option{storage.WithUploadResolver(resolver)}
		if c.httpClient != nil {
			managerOpts = append(managerOpts, storage.WithHTTPClient(c.httpClient))
		}
		if c.source != "" {
			managerOpts = append(managerOpts, storage.WithSource(c.source))
		}
		c.storage = storage.NewManager(managerOpts...)
	})
	return c.storage
}
