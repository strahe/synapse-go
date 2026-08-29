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
	rootAddress := c.evmSigner.EVMAddress()

	ws, err := warmstorage.New(warmstorage.Options{
		Client:            c.ethClient,
		Backend:           c.ethClient,
		ChainID:           types.ChainID(c.selectedChain.ChainID()),
		FWSS:              c.addresses.FWSS,
		ViewContract:      c.addresses.ViewContract,
		PDPVerifier:       c.addresses.PDPVerifier,
		Signer:            c.evmSigner,
		Logger:            c.logger,
		NonceManager:      c.nonces,
		Lifecycle:         c.lifecycle,
		MaxMulticallCalls: c.maxMulticallCalls,
	})
	if err != nil {
		return fmt.Errorf("create warmstorage service: %w", err)
	}
	c.warmStorage = ws

	spReg, err := spregistry.New(spregistry.Options{
		Client:              c.ethClient,
		Address:             c.addresses.SPRegistry,
		EndorsementsAddress: c.selectedChain.Addresses().Endorsements,
		ChainID:             types.ChainID(c.selectedChain.ChainID()),
		Backend:             c.ethClient,
		Signer:              c.evmSigner,
		NonceManager:        c.nonces,
		Logger:              c.logger,
		Lifecycle:           c.lifecycle,
		MaxMulticallCalls:   c.maxMulticallCalls,
	})
	if err != nil {
		return fmt.Errorf("create spregistry service: %w", err)
	}
	c.spRegistry = spReg

	pay, err := payments.New(payments.Options{
		Backend:              c.ethClient,
		ChainID:              types.ChainID(c.selectedChain.ChainID()),
		FilPayAddress:        c.addresses.Payments,
		WarmStorageAddress:   c.addresses.FWSS,
		USDFCTokenAddress:    c.addresses.USDFC,
		Signer:               c.evmSigner,
		ApprovalLockupPeriod: adapters.NewApprovalLockupPeriodReader(ws),
		Logger:               c.logger,
		NonceManager:         c.nonces,
		Lifecycle:            c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create payments service: %w", err)
	}
	c.payments = pay

	sk, err := sessionkey.New(sessionkey.Options{
		Backend:           c.ethClient,
		ChainID:           types.ChainID(c.selectedChain.ChainID()),
		RegistryAddress:   c.addresses.SessionKeyRegistry,
		Signer:            c.evmSigner,
		Logger:            c.logger,
		NonceManager:      c.nonces,
		Lifecycle:         c.lifecycle,
		MaxMulticallCalls: c.maxMulticallCalls,
	})
	if err != nil {
		return fmt.Errorf("create sessionkey service: %w", err)
	}
	c.sessionKey = sk

	fb, err := filbeam.New(filbeam.Options{
		Chain:           c.selectedChain,
		HTTPClient:      c.httpClient,
		RetrievalDomain: c.filbeamRetrievalDomain,
		Logger:          c.logger,
		Lifecycle:       c.lifecycle,
	})
	if err != nil {
		return fmt.Errorf("create filbeam service: %w", err)
	}
	c.filbeam = fb
	fbRetriever, err := fb.NewRetriever(rootAddress)
	if err != nil {
		return fmt.Errorf("create filbeam retriever: %w", err)
	}

	costsvc, err := costs.New(costs.Options{
		Chain:              c.selectedChain,
		USDFCTokenAddress:  c.addresses.USDFC,
		WarmStorageAddress: c.addresses.FWSS,
		WarmStorage:        ws,
		Payments:           pay,
		Caller:             c.ethClient,
		Logger:             c.logger,
		Lifecycle:          c.lifecycle,
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
		c.pdpReader = adapters.NewPDPVerifierReader(
			caller,
			c.ethClient,
			c.addresses.PDPVerifier,
			c.maxMulticallCalls,
		)
	}

	resolver, err := storage.NewServiceResolver(storage.ServiceResolverOptions{
		Payer:        rootAddress,
		SPRegistry:   spReg,
		Endorsements: spReg,
		WarmStorage:  ws,
		ProviderPing: c.pingProvider,
		NewContext: func(provider storage.Provider, opts storage.ContextFactoryOptions) (*storage.ProviderContext, error) {
			pdpClient, err := c.newPDPClient(provider.ServiceURL)
			if err != nil {
				return nil, fmt.Errorf("create PDP client for %s: %w", provider.ServiceURL, err)
			}
			ctxOpts := []storage.ContextOption{
				storage.WithPayer(rootAddress),
				storage.WithChainID(types.ChainID(c.selectedChain.ChainID())),
				storage.WithRecordKeeper(c.addresses.FWSS),
				storage.WithDataSetMetadata(opts.DataSetMetadata),
				storage.WithCDN(opts.WithCDN),
				storage.WithCDNRetriever(fbRetriever),
				storage.WithLogger(c.logger),
				storage.WithPDPVerifierReader(c.pdpReader),
				storage.WithPDPConfigReader(ws),
				storage.WithFWSSTerminator(ws),
				storage.WithFWSSDataSetReader(ws),
				storage.WithDataSetValidator(ws),
				storage.WithPaymentStateReader(pay, c.ethClient, c.addresses.USDFC),
			}
			return storage.NewProviderContext(
				provider,
				pdpClient,
				c.storageSigner,
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
		Logger:               c.logger,

		DataSetFinder:      adapters.NewDataSetFinder(ws),
		StorageInfoReader:  adapters.NewStorageInfoReader(ws, spReg, pay, c.addresses.USDFC, c.addresses.FWSS),
		DataSetTerminator:  ws,
		FWSSDataSetReader:  ws,
		PaymentStateReader: pay,
		EpochReader:        c.ethClient,
		PaymentToken:       c.addresses.USDFC,
		Signer:             c.storageSigner,
		ChainID:            types.ChainID(c.selectedChain.ChainID()),
		RecordKeeper:       c.addresses.FWSS,
		CostCalculator:     adapters.NewCostCalculator(costsvc),
		PaymentsFunder:     adapters.NewPaymentsFunder(pay),
		PayerAddress:       rootAddress,
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

func (c *Client) newPDPClient(serviceURL string, opts ...pdp.Option) (*pdp.Client, error) {
	pdpOpts := make([]pdp.Option, 0, len(opts)+2)
	if c.logger != nil {
		pdpOpts = append(pdpOpts, pdp.WithLogger(c.logger))
	}
	if c.httpClient != nil {
		pdpOpts = append(pdpOpts, pdp.WithHTTPClient(c.httpClient))
	}
	pdpOpts = append(pdpOpts, opts...)
	return pdp.New(serviceURL, pdpOpts...)
}

func (c *Client) pingProvider(ctx context.Context, serviceURL string) error {
	pdpClient, err := c.newPDPClient(serviceURL, pdp.WithMaxRetries(2))
	if err != nil {
		return err
	}
	return pdpClient.Ping(ctx)
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
// created inside the [storage.ContextFactory] closure whenever an immutable
// storage context is resolved.
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
