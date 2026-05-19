package spregistry

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	spr "github.com/strahe/synapse-go/internal/contracts/spregistry"
)

// RegisterProvider registers the caller as a storage provider and, in the
// same transaction, declares a PDP product. The caller is taken from the
// service's configured Signer; the Payee in info designates the account that
// receives payments (typically but not necessarily identical to the caller).
//
// Pricing:
//   - When a WithValue option is provided it must equal the current 5 FIL
//     registration fee mirrored by the SDK.
//   - Otherwise the current REGISTRATION_FEE() is read from the contract
//     and used; an RPC error there is wrapped and returned before any
//     transaction is broadcast.
//
// The PDP offering is validated and encoded before broadcast: an invalid
// offering surfaces as ErrInvalidOffering with no on-chain side effects.
//
// Returns a WriteResult carrying the broadcast tx hash. Use WithWait to
// block for the receipt; when the transaction reverts on-chain the
// receipt is preserved on the WriteResult alongside ErrTxFailed.
func (s *Service) RegisterProvider(ctx context.Context, info ProviderRegistrationInfo, opts ...WriteOption) (*WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner(); err != nil {
		return nil, fmt.Errorf("spregistry.RegisterProvider: %w", err)
	}
	if (info.Payee == common.Address{}) {
		return nil, fmt.Errorf("spregistry.RegisterProvider: %w: zero Payee", ErrInvalidArgument)
	}
	if info.Name == "" {
		return nil, fmt.Errorf("spregistry.RegisterProvider: %w: empty Name", ErrInvalidArgument)
	}
	if err := validateProviderInfo(info.Name, info.Description); err != nil {
		return nil, fmt.Errorf("spregistry.RegisterProvider: %w", err)
	}

	keys, values, err := EncodePDPCapabilities(info.PDPOffering, info.Capabilities)
	if err != nil {
		return nil, fmt.Errorf("spregistry.RegisterProvider: %w", err)
	}

	cfg := newWriteConfig(opts)
	var fee *big.Int
	if cfg.valueSet {
		if cfg.value == nil {
			return nil, fmt.Errorf("spregistry.RegisterProvider: %w: WithValue(nil)", ErrInvalidArgument)
		}
		if cfg.value.Sign() < 0 {
			return nil, fmt.Errorf("spregistry.RegisterProvider: %w: negative WithValue", ErrInvalidArgument)
		}
		if cfg.value.Cmp(registrationFeeWei()) != 0 {
			return nil, fmt.Errorf("spregistry.RegisterProvider: %w: incorrect registration fee", ErrInvalidArgument)
		}
		fee = new(big.Int).Set(cfg.value)
	} else {
		feeCaller, err := spr.NewSPRegistryCaller(s.addr, s.backend)
		if err != nil {
			return nil, fmt.Errorf("spregistry.RegisterProvider: bind REGISTRATION_FEE caller: %w", err)
		}
		f, err := feeCaller.REGISTRATIONFEE(&bind.CallOpts{Context: ctx})
		if err != nil {
			return nil, fmt.Errorf("spregistry.RegisterProvider: read REGISTRATION_FEE: %w", err)
		}
		fee = f
	}

	topts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("spregistry.RegisterProvider: %w", err)
	}
	defer release()
	topts.Value = fee

	tx, err := s.write.RegisterProvider(topts, info.Payee, info.Name, info.Description, uint8(ProductTypePDP), keys, values)
	release()
	if err != nil {
		return nil, fmt.Errorf("spregistry.RegisterProvider: broadcast: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// UpdateProviderInfo updates the caller's on-chain display metadata. Name
// must be non-empty; description may be empty to clear the current value.
func (s *Service) UpdateProviderInfo(ctx context.Context, name, description string, opts ...WriteOption) (*WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner(); err != nil {
		return nil, fmt.Errorf("spregistry.UpdateProviderInfo: %w", err)
	}
	if name == "" {
		return nil, fmt.Errorf("spregistry.UpdateProviderInfo: %w: empty name", ErrInvalidArgument)
	}
	if err := validateProviderInfo(name, description); err != nil {
		return nil, fmt.Errorf("spregistry.UpdateProviderInfo: %w", err)
	}
	topts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("spregistry.UpdateProviderInfo: %w", err)
	}
	defer release()
	tx, err := s.write.UpdateProviderInfo(topts, name, description)
	release()
	if err != nil {
		return nil, fmt.Errorf("spregistry.UpdateProviderInfo: broadcast: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// RemoveProvider deregisters the caller from the registry. Existing data
// sets on the provider continue to settle; the registry merely flags the
// provider inactive for discovery purposes.
func (s *Service) RemoveProvider(ctx context.Context, opts ...WriteOption) (*WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner(); err != nil {
		return nil, fmt.Errorf("spregistry.RemoveProvider: %w", err)
	}
	topts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("spregistry.RemoveProvider: %w", err)
	}
	defer release()
	tx, err := s.write.RemoveProvider(topts)
	release()
	if err != nil {
		return nil, fmt.Errorf("spregistry.RemoveProvider: broadcast: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// AddPDPProduct declares the caller's PDP product after an initial
// registerProvider that did not include one, or after RemoveProduct.
// Currently only PDP products are supported by the public SDK surface.
func (s *Service) AddPDPProduct(ctx context.Context, offering PDPOffering, capabilities map[string]string, opts ...WriteOption) (*WriteResult, error) {
	return s.writeProduct(ctx, "AddPDPProduct", offering, capabilities, s.write.AddProduct, opts)
}

// UpdatePDPProduct replaces the capability set associated with the caller's
// PDP product. The full offering + extras must be supplied; partial updates
// are not supported by the contract ABI.
func (s *Service) UpdatePDPProduct(ctx context.Context, offering PDPOffering, capabilities map[string]string, opts ...WriteOption) (*WriteResult, error) {
	return s.writeProduct(ctx, "UpdatePDPProduct", offering, capabilities, s.write.UpdateProduct, opts)
}

// RemoveProduct removes the given product type for the caller. PDP
// (ProductTypePDP) is the only variant exposed by the SDK today but the
// signature accepts the enum so callers are forward-compatible with
// future product types defined in the registry ABI.
func (s *Service) RemoveProduct(ctx context.Context, productType ProductType, opts ...WriteOption) (*WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner(); err != nil {
		return nil, fmt.Errorf("spregistry.RemoveProduct: %w", err)
	}
	topts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("spregistry.RemoveProduct: %w", err)
	}
	defer release()
	tx, err := s.write.RemoveProduct(topts, uint8(productType))
	release()
	if err != nil {
		return nil, fmt.Errorf("spregistry.RemoveProduct: broadcast: %w", err)
	}
	return s.finalize(ctx, tx, opts)
}

// writeProduct is the shared body of AddPDPProduct and UpdatePDPProduct.
// The contract function is passed in as a closure so both code paths share
// identical validation, encoding, and finalize logic.
func (s *Service) writeProduct(
	ctx context.Context,
	method string,
	offering PDPOffering,
	capabilities map[string]string,
	fn func(*bind.TransactOpts, uint8, []string, [][]byte) (*ethtypes.Transaction, error),
	opts []WriteOption,
) (*WriteResult, error) {
	if err := s.checkInit(); err != nil {
		return nil, err
	}
	if err := s.requireSigner(); err != nil {
		return nil, fmt.Errorf("spregistry.%s: %w", method, err)
	}
	keys, values, err := EncodePDPCapabilities(offering, capabilities)
	if err != nil {
		return nil, fmt.Errorf("spregistry.%s: %w", method, err)
	}
	topts, release, err := s.newTransactOpts(ctx)
	if err != nil {
		return nil, fmt.Errorf("spregistry.%s: %w", method, err)
	}
	defer release()
	tx, err := fn(topts, uint8(ProductTypePDP), keys, values)
	release()
	if err != nil {
		return nil, fmt.Errorf("spregistry.%s: broadcast: %w", method, err)
	}
	return s.finalize(ctx, tx, opts)
}
