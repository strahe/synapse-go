// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package spregistry

import (
	"errors"
	"math/big"
	"strings"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/event"
)

// Reference imports to suppress errors if they are not otherwise used.
var (
	_ = errors.New
	_ = big.NewInt
	_ = strings.NewReader
	_ = ethereum.NotFound
	_ = bind.Bind
	_ = common.Big1
	_ = types.BloomLookup
	_ = event.NewSubscription
	_ = abi.ConvertType
)

// ServiceProviderRegistryPlannedUpgrade is an auto generated low-level Go binding around an user-defined struct.
type ServiceProviderRegistryPlannedUpgrade struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}

// ServiceProviderRegistryServiceProviderInfoView is an auto generated low-level Go binding around an user-defined struct.
type ServiceProviderRegistryServiceProviderInfoView struct {
	ProviderId *big.Int
	Info       ServiceProviderRegistryStorageServiceProviderInfo
}

// ServiceProviderRegistryStoragePaginatedProviders is an auto generated low-level Go binding around an user-defined struct.
type ServiceProviderRegistryStoragePaginatedProviders struct {
	Providers []ServiceProviderRegistryStorageProviderWithProduct
	HasMore   bool
}

// ServiceProviderRegistryStorageProviderWithProduct is an auto generated low-level Go binding around an user-defined struct.
type ServiceProviderRegistryStorageProviderWithProduct struct {
	ProviderId              *big.Int
	ProviderInfo            ServiceProviderRegistryStorageServiceProviderInfo
	Product                 ServiceProviderRegistryStorageServiceProduct
	ProductCapabilityValues [][]byte
}

// ServiceProviderRegistryStorageServiceProduct is an auto generated low-level Go binding around an user-defined struct.
type ServiceProviderRegistryStorageServiceProduct struct {
	ProductType    uint8
	CapabilityKeys []string
	IsActive       bool
}

// ServiceProviderRegistryStorageServiceProviderInfo is an auto generated low-level Go binding around an user-defined struct.
type ServiceProviderRegistryStorageServiceProviderInfo struct {
	ServiceProvider common.Address
	Payee           common.Address
	Name            string
	Description     string
	IsActive        bool
}

// SPRegistryMetaData contains all meta data concerning the SPRegistry contract.
var SPRegistryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"_reinitializer_version\",\"type\":\"uint64\",\"internalType\":\"uint64\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"MAX_CAPABILITIES\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MAX_CAPABILITY_KEY_LENGTH\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"MAX_CAPABILITY_VALUE_LENGTH\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"REGISTRATION_FEE\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"UPGRADE_INTERFACE_VERSION\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"VERSION\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeProductTypeProviderCount\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"outputs\":[{\"name\":\"count\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"activeProviderCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"addProduct\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"capabilityKeys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"capabilityValues\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"addressToProviderId\",\"inputs\":[{\"name\":\"providerAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"announcePlannedUpgrade\",\"inputs\":[{\"name\":\"plannedUpgrade\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistry.PlannedUpgrade\",\"components\":[{\"name\":\"nextImplementation\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"afterEpoch\",\"type\":\"uint96\",\"internalType\":\"uint96\"}]}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAllActiveProviders\",\"inputs\":[{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"providerIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"},{\"name\":\"hasMore\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAllProductCapabilities\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"outputs\":[{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"keys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"values\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getNextProviderId\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProductCapabilities\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"keys\",\"type\":\"string[]\",\"internalType\":\"string[]\"}],\"outputs\":[{\"name\":\"values\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProvider\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistry.ServiceProviderInfoView\",\"components\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ServiceProviderInfo\",\"components\":[{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}]}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProviderByAddress\",\"inputs\":[{\"name\":\"providerAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistry.ServiceProviderInfoView\",\"components\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ServiceProviderInfo\",\"components\":[{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}]}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProviderCount\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProviderIdByAddress\",\"inputs\":[{\"name\":\"providerAddress\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProviderPayee\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProviderWithProduct\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"outputs\":[{\"name\":\"\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ProviderWithProduct\",\"components\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerInfo\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ServiceProviderInfo\",\"components\":[{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"product\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ServiceProduct\",\"components\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"capabilityKeys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"productCapabilityValues\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProvidersByIds\",\"inputs\":[{\"name\":\"providerIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"outputs\":[{\"name\":\"providerInfos\",\"type\":\"tuple[]\",\"internalType\":\"structServiceProviderRegistry.ServiceProviderInfoView[]\",\"components\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ServiceProviderInfo\",\"components\":[{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}]}]},{\"name\":\"validIds\",\"type\":\"bool[]\",\"internalType\":\"bool[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProvidersByProductType\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"onlyActive\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"result\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.PaginatedProviders\",\"components\":[{\"name\":\"providers\",\"type\":\"tuple[]\",\"internalType\":\"structServiceProviderRegistryStorage.ProviderWithProduct[]\",\"components\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerInfo\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ServiceProviderInfo\",\"components\":[{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"product\",\"type\":\"tuple\",\"internalType\":\"structServiceProviderRegistryStorage.ServiceProduct\",\"components\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"capabilityKeys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}]},{\"name\":\"productCapabilityValues\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}]},{\"name\":\"hasMore\",\"type\":\"bool\",\"internalType\":\"bool\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"initialize\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"isProviderActive\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isRegisteredProvider\",\"inputs\":[{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"newVersion\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"nextUpgrade\",\"inputs\":[],\"outputs\":[{\"name\":\"nextImplementation\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"afterEpoch\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"productCapabilities\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"value\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"productTypeProviderCount\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"outputs\":[{\"name\":\"count\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"providerHasProduct\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"providerProducts\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"outputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"providers\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"isActive\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"proxiableUUID\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"registerProvider\",\"inputs\":[{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"capabilityKeys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"capabilityValues\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"removeProduct\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"removeProvider\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"updateProduct\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"capabilityKeys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"capabilityValues\",\"type\":\"bytes[]\",\"internalType\":\"bytes[]\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"updateProviderInfo\",\"inputs\":[{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"description\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"upgradeToAndCall\",\"inputs\":[{\"name\":\"newImplementation\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"payable\"},{\"type\":\"event\",\"name\":\"ContractUpgraded\",\"inputs\":[{\"name\":\"version\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"},{\"name\":\"implementation\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ProductAdded\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"indexed\":true,\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"capabilityKeys\",\"type\":\"string[]\",\"indexed\":false,\"internalType\":\"string[]\"},{\"name\":\"capabilityValues\",\"type\":\"bytes[]\",\"indexed\":false,\"internalType\":\"bytes[]\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ProductRemoved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"indexed\":true,\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ProductUpdated\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"productType\",\"type\":\"uint8\",\"indexed\":true,\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"capabilityKeys\",\"type\":\"string[]\",\"indexed\":false,\"internalType\":\"string[]\"},{\"name\":\"capabilityValues\",\"type\":\"bytes[]\",\"indexed\":false,\"internalType\":\"bytes[]\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ProviderInfoUpdated\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ProviderRegistered\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ProviderRemoved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"UpgradeAnnounced\",\"inputs\":[{\"name\":\"plannedUpgrade\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structServiceProviderRegistry.PlannedUpgrade\",\"components\":[{\"name\":\"nextImplementation\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"afterEpoch\",\"type\":\"uint96\",\"internalType\":\"uint96\"}]}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Upgraded\",\"inputs\":[{\"name\":\"implementation\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AddressEmptyCode\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC1967InvalidImplementation\",\"inputs\":[{\"name\":\"implementation\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ERC1967NonPayable\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"FailedCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InsufficientCapabilitiesForProduct\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"UUPSUnauthorizedCallContext\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"UUPSUnsupportedProxiableUUID\",\"inputs\":[{\"name\":\"slot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}]},{\"type\":\"error\",\"name\":\"AddressAlreadySet\",\"inputs\":[{\"name\":\"field\",\"type\":\"uint8\",\"internalType\":\"enumErrors.AddressField\"}]},{\"type\":\"error\",\"name\":\"AtLeastOnePriceMustBeNonZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CDNPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CacheMissPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayerOrPayee\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expectedPayee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayments\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ChallengeWindowTooEarly\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"windowStart\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ClientDataSetAlreadyRegistered\",\"inputs\":[{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CommissionExceedsMaximum\",\"inputs\":[{\"name\":\"commissionType\",\"type\":\"uint8\",\"internalType\":\"enumErrors.CommissionType\"},{\"name\":\"max\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotFoundForRail\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotRegistered\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentBeyondEndEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DivisionByZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DuplicateMetadataKey\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"ExtraDataRequired\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExtraDataTooLarge\",\"inputs\":[{\"name\":\"actualSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowedSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"FilBeamServiceNotConfigured\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientCapabilitiesForProduct\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lockupAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"lockupUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minimumLockupRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupFunds\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"minimumRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"available\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientMaxLockupPeriod\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"maxLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"requiredLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientRateAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"rateAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"rateUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minimumRateRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeCount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minExpected\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeWindowSize\",\"inputs\":[{\"name\":\"maxProvingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengeWindowSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidDataSetId\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidEpochRange\",\"inputs\":[{\"name\":\"fromEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"toEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidServiceDescriptionLength\",\"inputs\":[{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidServiceNameLength\",\"inputs\":[{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InvalidSignatureLength\",\"inputs\":[{\"name\":\"expectedLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actualLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidTopUpAmount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MaxProvingPeriodZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MetadataArrayCountMismatch\",\"inputs\":[{\"name\":\"metadataArrayCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyAndValueLengthMismatch\",\"inputs\":[{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"valuesLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataValueExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NextProvingPeriodAlreadyCalled\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"periodDeadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NoPDPPaymentRail\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"OldServiceProviderMismatch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyFilBeamControllerAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyPDPVerifierAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlySelf\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OperatorNotApproved\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"PaymentRailsNotFinalized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"PriceExceedsMaximum\",\"inputs\":[{\"name\":\"priceType\",\"type\":\"uint8\",\"internalType\":\"enumErrors.PriceType\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProofAlreadySubmitted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderAlreadyApproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderIdMismatchAtIndex\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotInApprovedList\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotRegistered\",\"inputs\":[{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ProvingNotStarted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingPeriodNotInitialized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingPeriodPassed\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"RailNotAssociated\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"RailNotFullySettled\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"settledUpTo\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"endEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ServiceContractMustTerminateRail\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"StorageProviderChangesNotSupported\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooManyMetadataKeys\",\"inputs\":[{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"UnsupportedSignatureV\",\"inputs\":[{\"name\":\"v\",\"type\":\"uint8\",\"internalType\":\"uint8\"}]},{\"type\":\"error\",\"name\":\"ZeroAddress\",\"inputs\":[{\"name\":\"field\",\"type\":\"uint8\",\"internalType\":\"enumErrors.AddressField\"}]}]",
}

// SPRegistryABI is the input ABI used to generate the binding from.
// Deprecated: Use SPRegistryMetaData.ABI instead.
var SPRegistryABI = SPRegistryMetaData.ABI

// SPRegistry is an auto generated Go binding around an Ethereum contract.
type SPRegistry struct {
	SPRegistryCaller     // Read-only binding to the contract
	SPRegistryTransactor // Write-only binding to the contract
	SPRegistryFilterer   // Log filterer for contract events
}

// SPRegistryCaller is an auto generated read-only Go binding around an Ethereum contract.
type SPRegistryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SPRegistryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SPRegistryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SPRegistryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SPRegistryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SPRegistrySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SPRegistrySession struct {
	Contract     *SPRegistry       // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// SPRegistryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SPRegistryCallerSession struct {
	Contract *SPRegistryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts     // Call options to use throughout this session
}

// SPRegistryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SPRegistryTransactorSession struct {
	Contract     *SPRegistryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts     // Transaction auth options to use throughout this session
}

// SPRegistryRaw is an auto generated low-level Go binding around an Ethereum contract.
type SPRegistryRaw struct {
	Contract *SPRegistry // Generic contract binding to access the raw methods on
}

// SPRegistryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SPRegistryCallerRaw struct {
	Contract *SPRegistryCaller // Generic read-only contract binding to access the raw methods on
}

// SPRegistryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SPRegistryTransactorRaw struct {
	Contract *SPRegistryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSPRegistry creates a new instance of SPRegistry, bound to a specific deployed contract.
func NewSPRegistry(address common.Address, backend bind.ContractBackend) (*SPRegistry, error) {
	contract, err := bindSPRegistry(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &SPRegistry{SPRegistryCaller: SPRegistryCaller{contract: contract}, SPRegistryTransactor: SPRegistryTransactor{contract: contract}, SPRegistryFilterer: SPRegistryFilterer{contract: contract}}, nil
}

// NewSPRegistryCaller creates a new read-only instance of SPRegistry, bound to a specific deployed contract.
func NewSPRegistryCaller(address common.Address, caller bind.ContractCaller) (*SPRegistryCaller, error) {
	contract, err := bindSPRegistry(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SPRegistryCaller{contract: contract}, nil
}

// NewSPRegistryTransactor creates a new write-only instance of SPRegistry, bound to a specific deployed contract.
func NewSPRegistryTransactor(address common.Address, transactor bind.ContractTransactor) (*SPRegistryTransactor, error) {
	contract, err := bindSPRegistry(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SPRegistryTransactor{contract: contract}, nil
}

// NewSPRegistryFilterer creates a new log filterer instance of SPRegistry, bound to a specific deployed contract.
func NewSPRegistryFilterer(address common.Address, filterer bind.ContractFilterer) (*SPRegistryFilterer, error) {
	contract, err := bindSPRegistry(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SPRegistryFilterer{contract: contract}, nil
}

// bindSPRegistry binds a generic wrapper to an already deployed contract.
func bindSPRegistry(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SPRegistryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SPRegistry *SPRegistryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SPRegistry.Contract.SPRegistryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SPRegistry *SPRegistryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SPRegistry.Contract.SPRegistryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SPRegistry *SPRegistryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SPRegistry.Contract.SPRegistryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SPRegistry *SPRegistryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SPRegistry.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SPRegistry *SPRegistryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SPRegistry.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SPRegistry *SPRegistryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SPRegistry.Contract.contract.Transact(opts, method, params...)
}

// MAXCAPABILITIES is a free data retrieval call binding the contract method 0x6e36e974.
//
// Solidity: function MAX_CAPABILITIES() view returns(uint256)
func (_SPRegistry *SPRegistryCaller) MAXCAPABILITIES(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "MAX_CAPABILITIES")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MAXCAPABILITIES is a free data retrieval call binding the contract method 0x6e36e974.
//
// Solidity: function MAX_CAPABILITIES() view returns(uint256)
func (_SPRegistry *SPRegistrySession) MAXCAPABILITIES() (*big.Int, error) {
	return _SPRegistry.Contract.MAXCAPABILITIES(&_SPRegistry.CallOpts)
}

// MAXCAPABILITIES is a free data retrieval call binding the contract method 0x6e36e974.
//
// Solidity: function MAX_CAPABILITIES() view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) MAXCAPABILITIES() (*big.Int, error) {
	return _SPRegistry.Contract.MAXCAPABILITIES(&_SPRegistry.CallOpts)
}

// MAXCAPABILITYKEYLENGTH is a free data retrieval call binding the contract method 0x7f657567.
//
// Solidity: function MAX_CAPABILITY_KEY_LENGTH() view returns(uint256)
func (_SPRegistry *SPRegistryCaller) MAXCAPABILITYKEYLENGTH(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "MAX_CAPABILITY_KEY_LENGTH")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MAXCAPABILITYKEYLENGTH is a free data retrieval call binding the contract method 0x7f657567.
//
// Solidity: function MAX_CAPABILITY_KEY_LENGTH() view returns(uint256)
func (_SPRegistry *SPRegistrySession) MAXCAPABILITYKEYLENGTH() (*big.Int, error) {
	return _SPRegistry.Contract.MAXCAPABILITYKEYLENGTH(&_SPRegistry.CallOpts)
}

// MAXCAPABILITYKEYLENGTH is a free data retrieval call binding the contract method 0x7f657567.
//
// Solidity: function MAX_CAPABILITY_KEY_LENGTH() view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) MAXCAPABILITYKEYLENGTH() (*big.Int, error) {
	return _SPRegistry.Contract.MAXCAPABILITYKEYLENGTH(&_SPRegistry.CallOpts)
}

// MAXCAPABILITYVALUELENGTH is a free data retrieval call binding the contract method 0xdcea1c6f.
//
// Solidity: function MAX_CAPABILITY_VALUE_LENGTH() view returns(uint256)
func (_SPRegistry *SPRegistryCaller) MAXCAPABILITYVALUELENGTH(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "MAX_CAPABILITY_VALUE_LENGTH")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// MAXCAPABILITYVALUELENGTH is a free data retrieval call binding the contract method 0xdcea1c6f.
//
// Solidity: function MAX_CAPABILITY_VALUE_LENGTH() view returns(uint256)
func (_SPRegistry *SPRegistrySession) MAXCAPABILITYVALUELENGTH() (*big.Int, error) {
	return _SPRegistry.Contract.MAXCAPABILITYVALUELENGTH(&_SPRegistry.CallOpts)
}

// MAXCAPABILITYVALUELENGTH is a free data retrieval call binding the contract method 0xdcea1c6f.
//
// Solidity: function MAX_CAPABILITY_VALUE_LENGTH() view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) MAXCAPABILITYVALUELENGTH() (*big.Int, error) {
	return _SPRegistry.Contract.MAXCAPABILITYVALUELENGTH(&_SPRegistry.CallOpts)
}

// REGISTRATIONFEE is a free data retrieval call binding the contract method 0x64b4f751.
//
// Solidity: function REGISTRATION_FEE() view returns(uint256)
func (_SPRegistry *SPRegistryCaller) REGISTRATIONFEE(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "REGISTRATION_FEE")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// REGISTRATIONFEE is a free data retrieval call binding the contract method 0x64b4f751.
//
// Solidity: function REGISTRATION_FEE() view returns(uint256)
func (_SPRegistry *SPRegistrySession) REGISTRATIONFEE() (*big.Int, error) {
	return _SPRegistry.Contract.REGISTRATIONFEE(&_SPRegistry.CallOpts)
}

// REGISTRATIONFEE is a free data retrieval call binding the contract method 0x64b4f751.
//
// Solidity: function REGISTRATION_FEE() view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) REGISTRATIONFEE() (*big.Int, error) {
	return _SPRegistry.Contract.REGISTRATIONFEE(&_SPRegistry.CallOpts)
}

// UPGRADEINTERFACEVERSION is a free data retrieval call binding the contract method 0xad3cb1cc.
//
// Solidity: function UPGRADE_INTERFACE_VERSION() view returns(string)
func (_SPRegistry *SPRegistryCaller) UPGRADEINTERFACEVERSION(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "UPGRADE_INTERFACE_VERSION")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// UPGRADEINTERFACEVERSION is a free data retrieval call binding the contract method 0xad3cb1cc.
//
// Solidity: function UPGRADE_INTERFACE_VERSION() view returns(string)
func (_SPRegistry *SPRegistrySession) UPGRADEINTERFACEVERSION() (string, error) {
	return _SPRegistry.Contract.UPGRADEINTERFACEVERSION(&_SPRegistry.CallOpts)
}

// UPGRADEINTERFACEVERSION is a free data retrieval call binding the contract method 0xad3cb1cc.
//
// Solidity: function UPGRADE_INTERFACE_VERSION() view returns(string)
func (_SPRegistry *SPRegistryCallerSession) UPGRADEINTERFACEVERSION() (string, error) {
	return _SPRegistry.Contract.UPGRADEINTERFACEVERSION(&_SPRegistry.CallOpts)
}

// VERSION is a free data retrieval call binding the contract method 0xffa1ad74.
//
// Solidity: function VERSION() view returns(string)
func (_SPRegistry *SPRegistryCaller) VERSION(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "VERSION")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// VERSION is a free data retrieval call binding the contract method 0xffa1ad74.
//
// Solidity: function VERSION() view returns(string)
func (_SPRegistry *SPRegistrySession) VERSION() (string, error) {
	return _SPRegistry.Contract.VERSION(&_SPRegistry.CallOpts)
}

// VERSION is a free data retrieval call binding the contract method 0xffa1ad74.
//
// Solidity: function VERSION() view returns(string)
func (_SPRegistry *SPRegistryCallerSession) VERSION() (string, error) {
	return _SPRegistry.Contract.VERSION(&_SPRegistry.CallOpts)
}

// ActiveProductTypeProviderCount is a free data retrieval call binding the contract method 0x8bdc7747.
//
// Solidity: function activeProductTypeProviderCount(uint8 productType) view returns(uint256 count)
func (_SPRegistry *SPRegistryCaller) ActiveProductTypeProviderCount(opts *bind.CallOpts, productType uint8) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "activeProductTypeProviderCount", productType)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveProductTypeProviderCount is a free data retrieval call binding the contract method 0x8bdc7747.
//
// Solidity: function activeProductTypeProviderCount(uint8 productType) view returns(uint256 count)
func (_SPRegistry *SPRegistrySession) ActiveProductTypeProviderCount(productType uint8) (*big.Int, error) {
	return _SPRegistry.Contract.ActiveProductTypeProviderCount(&_SPRegistry.CallOpts, productType)
}

// ActiveProductTypeProviderCount is a free data retrieval call binding the contract method 0x8bdc7747.
//
// Solidity: function activeProductTypeProviderCount(uint8 productType) view returns(uint256 count)
func (_SPRegistry *SPRegistryCallerSession) ActiveProductTypeProviderCount(productType uint8) (*big.Int, error) {
	return _SPRegistry.Contract.ActiveProductTypeProviderCount(&_SPRegistry.CallOpts, productType)
}

// ActiveProviderCount is a free data retrieval call binding the contract method 0xf08bbda0.
//
// Solidity: function activeProviderCount() view returns(uint256)
func (_SPRegistry *SPRegistryCaller) ActiveProviderCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "activeProviderCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ActiveProviderCount is a free data retrieval call binding the contract method 0xf08bbda0.
//
// Solidity: function activeProviderCount() view returns(uint256)
func (_SPRegistry *SPRegistrySession) ActiveProviderCount() (*big.Int, error) {
	return _SPRegistry.Contract.ActiveProviderCount(&_SPRegistry.CallOpts)
}

// ActiveProviderCount is a free data retrieval call binding the contract method 0xf08bbda0.
//
// Solidity: function activeProviderCount() view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) ActiveProviderCount() (*big.Int, error) {
	return _SPRegistry.Contract.ActiveProviderCount(&_SPRegistry.CallOpts)
}

// AddressToProviderId is a free data retrieval call binding the contract method 0xe835440e.
//
// Solidity: function addressToProviderId(address providerAddress) view returns(uint256 providerId)
func (_SPRegistry *SPRegistryCaller) AddressToProviderId(opts *bind.CallOpts, providerAddress common.Address) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "addressToProviderId", providerAddress)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AddressToProviderId is a free data retrieval call binding the contract method 0xe835440e.
//
// Solidity: function addressToProviderId(address providerAddress) view returns(uint256 providerId)
func (_SPRegistry *SPRegistrySession) AddressToProviderId(providerAddress common.Address) (*big.Int, error) {
	return _SPRegistry.Contract.AddressToProviderId(&_SPRegistry.CallOpts, providerAddress)
}

// AddressToProviderId is a free data retrieval call binding the contract method 0xe835440e.
//
// Solidity: function addressToProviderId(address providerAddress) view returns(uint256 providerId)
func (_SPRegistry *SPRegistryCallerSession) AddressToProviderId(providerAddress common.Address) (*big.Int, error) {
	return _SPRegistry.Contract.AddressToProviderId(&_SPRegistry.CallOpts, providerAddress)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_SPRegistry *SPRegistryCaller) Eip712Domain(opts *bind.CallOpts) (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "eip712Domain")

	outstruct := new(struct {
		Fields            [1]byte
		Name              string
		Version           string
		ChainId           *big.Int
		VerifyingContract common.Address
		Salt              [32]byte
		Extensions        []*big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Fields = *abi.ConvertType(out[0], new([1]byte)).(*[1]byte)
	outstruct.Name = *abi.ConvertType(out[1], new(string)).(*string)
	outstruct.Version = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.ChainId = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)
	outstruct.VerifyingContract = *abi.ConvertType(out[4], new(common.Address)).(*common.Address)
	outstruct.Salt = *abi.ConvertType(out[5], new([32]byte)).(*[32]byte)
	outstruct.Extensions = *abi.ConvertType(out[6], new([]*big.Int)).(*[]*big.Int)

	return *outstruct, err

}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_SPRegistry *SPRegistrySession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _SPRegistry.Contract.Eip712Domain(&_SPRegistry.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_SPRegistry *SPRegistryCallerSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _SPRegistry.Contract.Eip712Domain(&_SPRegistry.CallOpts)
}

// GetAllActiveProviders is a free data retrieval call binding the contract method 0x2f67c065.
//
// Solidity: function getAllActiveProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds, bool hasMore)
func (_SPRegistry *SPRegistryCaller) GetAllActiveProviders(opts *bind.CallOpts, offset *big.Int, limit *big.Int) (struct {
	ProviderIds []*big.Int
	HasMore     bool
}, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getAllActiveProviders", offset, limit)

	outstruct := new(struct {
		ProviderIds []*big.Int
		HasMore     bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ProviderIds = *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)
	outstruct.HasMore = *abi.ConvertType(out[1], new(bool)).(*bool)

	return *outstruct, err

}

// GetAllActiveProviders is a free data retrieval call binding the contract method 0x2f67c065.
//
// Solidity: function getAllActiveProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds, bool hasMore)
func (_SPRegistry *SPRegistrySession) GetAllActiveProviders(offset *big.Int, limit *big.Int) (struct {
	ProviderIds []*big.Int
	HasMore     bool
}, error) {
	return _SPRegistry.Contract.GetAllActiveProviders(&_SPRegistry.CallOpts, offset, limit)
}

// GetAllActiveProviders is a free data retrieval call binding the contract method 0x2f67c065.
//
// Solidity: function getAllActiveProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds, bool hasMore)
func (_SPRegistry *SPRegistryCallerSession) GetAllActiveProviders(offset *big.Int, limit *big.Int) (struct {
	ProviderIds []*big.Int
	HasMore     bool
}, error) {
	return _SPRegistry.Contract.GetAllActiveProviders(&_SPRegistry.CallOpts, offset, limit)
}

// GetAllProductCapabilities is a free data retrieval call binding the contract method 0xa6771f8b.
//
// Solidity: function getAllProductCapabilities(uint256 providerId, uint8 productType) view returns(bool isActive, string[] keys, bytes[] values)
func (_SPRegistry *SPRegistryCaller) GetAllProductCapabilities(opts *bind.CallOpts, providerId *big.Int, productType uint8) (struct {
	IsActive bool
	Keys     []string
	Values   [][]byte
}, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getAllProductCapabilities", providerId, productType)

	outstruct := new(struct {
		IsActive bool
		Keys     []string
		Values   [][]byte
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.IsActive = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.Keys = *abi.ConvertType(out[1], new([]string)).(*[]string)
	outstruct.Values = *abi.ConvertType(out[2], new([][]byte)).(*[][]byte)

	return *outstruct, err

}

// GetAllProductCapabilities is a free data retrieval call binding the contract method 0xa6771f8b.
//
// Solidity: function getAllProductCapabilities(uint256 providerId, uint8 productType) view returns(bool isActive, string[] keys, bytes[] values)
func (_SPRegistry *SPRegistrySession) GetAllProductCapabilities(providerId *big.Int, productType uint8) (struct {
	IsActive bool
	Keys     []string
	Values   [][]byte
}, error) {
	return _SPRegistry.Contract.GetAllProductCapabilities(&_SPRegistry.CallOpts, providerId, productType)
}

// GetAllProductCapabilities is a free data retrieval call binding the contract method 0xa6771f8b.
//
// Solidity: function getAllProductCapabilities(uint256 providerId, uint8 productType) view returns(bool isActive, string[] keys, bytes[] values)
func (_SPRegistry *SPRegistryCallerSession) GetAllProductCapabilities(providerId *big.Int, productType uint8) (struct {
	IsActive bool
	Keys     []string
	Values   [][]byte
}, error) {
	return _SPRegistry.Contract.GetAllProductCapabilities(&_SPRegistry.CallOpts, providerId, productType)
}

// GetNextProviderId is a free data retrieval call binding the contract method 0xd1329d4e.
//
// Solidity: function getNextProviderId() view returns(uint256)
func (_SPRegistry *SPRegistryCaller) GetNextProviderId(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getNextProviderId")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetNextProviderId is a free data retrieval call binding the contract method 0xd1329d4e.
//
// Solidity: function getNextProviderId() view returns(uint256)
func (_SPRegistry *SPRegistrySession) GetNextProviderId() (*big.Int, error) {
	return _SPRegistry.Contract.GetNextProviderId(&_SPRegistry.CallOpts)
}

// GetNextProviderId is a free data retrieval call binding the contract method 0xd1329d4e.
//
// Solidity: function getNextProviderId() view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) GetNextProviderId() (*big.Int, error) {
	return _SPRegistry.Contract.GetNextProviderId(&_SPRegistry.CallOpts)
}

// GetProductCapabilities is a free data retrieval call binding the contract method 0xa6433240.
//
// Solidity: function getProductCapabilities(uint256 providerId, uint8 productType, string[] keys) view returns(bytes[] values)
func (_SPRegistry *SPRegistryCaller) GetProductCapabilities(opts *bind.CallOpts, providerId *big.Int, productType uint8, keys []string) ([][]byte, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProductCapabilities", providerId, productType, keys)

	if err != nil {
		return *new([][]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][]byte)).(*[][]byte)

	return out0, err

}

// GetProductCapabilities is a free data retrieval call binding the contract method 0xa6433240.
//
// Solidity: function getProductCapabilities(uint256 providerId, uint8 productType, string[] keys) view returns(bytes[] values)
func (_SPRegistry *SPRegistrySession) GetProductCapabilities(providerId *big.Int, productType uint8, keys []string) ([][]byte, error) {
	return _SPRegistry.Contract.GetProductCapabilities(&_SPRegistry.CallOpts, providerId, productType, keys)
}

// GetProductCapabilities is a free data retrieval call binding the contract method 0xa6433240.
//
// Solidity: function getProductCapabilities(uint256 providerId, uint8 productType, string[] keys) view returns(bytes[] values)
func (_SPRegistry *SPRegistryCallerSession) GetProductCapabilities(providerId *big.Int, productType uint8, keys []string) ([][]byte, error) {
	return _SPRegistry.Contract.GetProductCapabilities(&_SPRegistry.CallOpts, providerId, productType, keys)
}

// GetProvider is a free data retrieval call binding the contract method 0x5c42d079.
//
// Solidity: function getProvider(uint256 providerId) view returns((uint256,(address,address,string,string,bool)) info)
func (_SPRegistry *SPRegistryCaller) GetProvider(opts *bind.CallOpts, providerId *big.Int) (ServiceProviderRegistryServiceProviderInfoView, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProvider", providerId)

	if err != nil {
		return *new(ServiceProviderRegistryServiceProviderInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new(ServiceProviderRegistryServiceProviderInfoView)).(*ServiceProviderRegistryServiceProviderInfoView)

	return out0, err

}

// GetProvider is a free data retrieval call binding the contract method 0x5c42d079.
//
// Solidity: function getProvider(uint256 providerId) view returns((uint256,(address,address,string,string,bool)) info)
func (_SPRegistry *SPRegistrySession) GetProvider(providerId *big.Int) (ServiceProviderRegistryServiceProviderInfoView, error) {
	return _SPRegistry.Contract.GetProvider(&_SPRegistry.CallOpts, providerId)
}

// GetProvider is a free data retrieval call binding the contract method 0x5c42d079.
//
// Solidity: function getProvider(uint256 providerId) view returns((uint256,(address,address,string,string,bool)) info)
func (_SPRegistry *SPRegistryCallerSession) GetProvider(providerId *big.Int) (ServiceProviderRegistryServiceProviderInfoView, error) {
	return _SPRegistry.Contract.GetProvider(&_SPRegistry.CallOpts, providerId)
}

// GetProviderByAddress is a free data retrieval call binding the contract method 0x2335bde0.
//
// Solidity: function getProviderByAddress(address providerAddress) view returns((uint256,(address,address,string,string,bool)) info)
func (_SPRegistry *SPRegistryCaller) GetProviderByAddress(opts *bind.CallOpts, providerAddress common.Address) (ServiceProviderRegistryServiceProviderInfoView, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProviderByAddress", providerAddress)

	if err != nil {
		return *new(ServiceProviderRegistryServiceProviderInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new(ServiceProviderRegistryServiceProviderInfoView)).(*ServiceProviderRegistryServiceProviderInfoView)

	return out0, err

}

// GetProviderByAddress is a free data retrieval call binding the contract method 0x2335bde0.
//
// Solidity: function getProviderByAddress(address providerAddress) view returns((uint256,(address,address,string,string,bool)) info)
func (_SPRegistry *SPRegistrySession) GetProviderByAddress(providerAddress common.Address) (ServiceProviderRegistryServiceProviderInfoView, error) {
	return _SPRegistry.Contract.GetProviderByAddress(&_SPRegistry.CallOpts, providerAddress)
}

// GetProviderByAddress is a free data retrieval call binding the contract method 0x2335bde0.
//
// Solidity: function getProviderByAddress(address providerAddress) view returns((uint256,(address,address,string,string,bool)) info)
func (_SPRegistry *SPRegistryCallerSession) GetProviderByAddress(providerAddress common.Address) (ServiceProviderRegistryServiceProviderInfoView, error) {
	return _SPRegistry.Contract.GetProviderByAddress(&_SPRegistry.CallOpts, providerAddress)
}

// GetProviderCount is a free data retrieval call binding the contract method 0x46ce4175.
//
// Solidity: function getProviderCount() view returns(uint256)
func (_SPRegistry *SPRegistryCaller) GetProviderCount(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProviderCount")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetProviderCount is a free data retrieval call binding the contract method 0x46ce4175.
//
// Solidity: function getProviderCount() view returns(uint256)
func (_SPRegistry *SPRegistrySession) GetProviderCount() (*big.Int, error) {
	return _SPRegistry.Contract.GetProviderCount(&_SPRegistry.CallOpts)
}

// GetProviderCount is a free data retrieval call binding the contract method 0x46ce4175.
//
// Solidity: function getProviderCount() view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) GetProviderCount() (*big.Int, error) {
	return _SPRegistry.Contract.GetProviderCount(&_SPRegistry.CallOpts)
}

// GetProviderIdByAddress is a free data retrieval call binding the contract method 0x93ecb91e.
//
// Solidity: function getProviderIdByAddress(address providerAddress) view returns(uint256)
func (_SPRegistry *SPRegistryCaller) GetProviderIdByAddress(opts *bind.CallOpts, providerAddress common.Address) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProviderIdByAddress", providerAddress)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetProviderIdByAddress is a free data retrieval call binding the contract method 0x93ecb91e.
//
// Solidity: function getProviderIdByAddress(address providerAddress) view returns(uint256)
func (_SPRegistry *SPRegistrySession) GetProviderIdByAddress(providerAddress common.Address) (*big.Int, error) {
	return _SPRegistry.Contract.GetProviderIdByAddress(&_SPRegistry.CallOpts, providerAddress)
}

// GetProviderIdByAddress is a free data retrieval call binding the contract method 0x93ecb91e.
//
// Solidity: function getProviderIdByAddress(address providerAddress) view returns(uint256)
func (_SPRegistry *SPRegistryCallerSession) GetProviderIdByAddress(providerAddress common.Address) (*big.Int, error) {
	return _SPRegistry.Contract.GetProviderIdByAddress(&_SPRegistry.CallOpts, providerAddress)
}

// GetProviderPayee is a free data retrieval call binding the contract method 0x60f4d53a.
//
// Solidity: function getProviderPayee(uint256 providerId) view returns(address payee)
func (_SPRegistry *SPRegistryCaller) GetProviderPayee(opts *bind.CallOpts, providerId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProviderPayee", providerId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetProviderPayee is a free data retrieval call binding the contract method 0x60f4d53a.
//
// Solidity: function getProviderPayee(uint256 providerId) view returns(address payee)
func (_SPRegistry *SPRegistrySession) GetProviderPayee(providerId *big.Int) (common.Address, error) {
	return _SPRegistry.Contract.GetProviderPayee(&_SPRegistry.CallOpts, providerId)
}

// GetProviderPayee is a free data retrieval call binding the contract method 0x60f4d53a.
//
// Solidity: function getProviderPayee(uint256 providerId) view returns(address payee)
func (_SPRegistry *SPRegistryCallerSession) GetProviderPayee(providerId *big.Int) (common.Address, error) {
	return _SPRegistry.Contract.GetProviderPayee(&_SPRegistry.CallOpts, providerId)
}

// GetProviderWithProduct is a free data retrieval call binding the contract method 0xadd33358.
//
// Solidity: function getProviderWithProduct(uint256 providerId, uint8 productType) view returns((uint256,(address,address,string,string,bool),(uint8,string[],bool),bytes[]))
func (_SPRegistry *SPRegistryCaller) GetProviderWithProduct(opts *bind.CallOpts, providerId *big.Int, productType uint8) (ServiceProviderRegistryStorageProviderWithProduct, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProviderWithProduct", providerId, productType)

	if err != nil {
		return *new(ServiceProviderRegistryStorageProviderWithProduct), err
	}

	out0 := *abi.ConvertType(out[0], new(ServiceProviderRegistryStorageProviderWithProduct)).(*ServiceProviderRegistryStorageProviderWithProduct)

	return out0, err

}

// GetProviderWithProduct is a free data retrieval call binding the contract method 0xadd33358.
//
// Solidity: function getProviderWithProduct(uint256 providerId, uint8 productType) view returns((uint256,(address,address,string,string,bool),(uint8,string[],bool),bytes[]))
func (_SPRegistry *SPRegistrySession) GetProviderWithProduct(providerId *big.Int, productType uint8) (ServiceProviderRegistryStorageProviderWithProduct, error) {
	return _SPRegistry.Contract.GetProviderWithProduct(&_SPRegistry.CallOpts, providerId, productType)
}

// GetProviderWithProduct is a free data retrieval call binding the contract method 0xadd33358.
//
// Solidity: function getProviderWithProduct(uint256 providerId, uint8 productType) view returns((uint256,(address,address,string,string,bool),(uint8,string[],bool),bytes[]))
func (_SPRegistry *SPRegistryCallerSession) GetProviderWithProduct(providerId *big.Int, productType uint8) (ServiceProviderRegistryStorageProviderWithProduct, error) {
	return _SPRegistry.Contract.GetProviderWithProduct(&_SPRegistry.CallOpts, providerId, productType)
}

// GetProvidersByIds is a free data retrieval call binding the contract method 0x5bfe9146.
//
// Solidity: function getProvidersByIds(uint256[] providerIds) view returns((uint256,(address,address,string,string,bool))[] providerInfos, bool[] validIds)
func (_SPRegistry *SPRegistryCaller) GetProvidersByIds(opts *bind.CallOpts, providerIds []*big.Int) (struct {
	ProviderInfos []ServiceProviderRegistryServiceProviderInfoView
	ValidIds      []bool
}, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProvidersByIds", providerIds)

	outstruct := new(struct {
		ProviderInfos []ServiceProviderRegistryServiceProviderInfoView
		ValidIds      []bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ProviderInfos = *abi.ConvertType(out[0], new([]ServiceProviderRegistryServiceProviderInfoView)).(*[]ServiceProviderRegistryServiceProviderInfoView)
	outstruct.ValidIds = *abi.ConvertType(out[1], new([]bool)).(*[]bool)

	return *outstruct, err

}

// GetProvidersByIds is a free data retrieval call binding the contract method 0x5bfe9146.
//
// Solidity: function getProvidersByIds(uint256[] providerIds) view returns((uint256,(address,address,string,string,bool))[] providerInfos, bool[] validIds)
func (_SPRegistry *SPRegistrySession) GetProvidersByIds(providerIds []*big.Int) (struct {
	ProviderInfos []ServiceProviderRegistryServiceProviderInfoView
	ValidIds      []bool
}, error) {
	return _SPRegistry.Contract.GetProvidersByIds(&_SPRegistry.CallOpts, providerIds)
}

// GetProvidersByIds is a free data retrieval call binding the contract method 0x5bfe9146.
//
// Solidity: function getProvidersByIds(uint256[] providerIds) view returns((uint256,(address,address,string,string,bool))[] providerInfos, bool[] validIds)
func (_SPRegistry *SPRegistryCallerSession) GetProvidersByIds(providerIds []*big.Int) (struct {
	ProviderInfos []ServiceProviderRegistryServiceProviderInfoView
	ValidIds      []bool
}, error) {
	return _SPRegistry.Contract.GetProvidersByIds(&_SPRegistry.CallOpts, providerIds)
}

// GetProvidersByProductType is a free data retrieval call binding the contract method 0x6ba44226.
//
// Solidity: function getProvidersByProductType(uint8 productType, bool onlyActive, uint256 offset, uint256 limit) view returns(((uint256,(address,address,string,string,bool),(uint8,string[],bool),bytes[])[],bool) result)
func (_SPRegistry *SPRegistryCaller) GetProvidersByProductType(opts *bind.CallOpts, productType uint8, onlyActive bool, offset *big.Int, limit *big.Int) (ServiceProviderRegistryStoragePaginatedProviders, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "getProvidersByProductType", productType, onlyActive, offset, limit)

	if err != nil {
		return *new(ServiceProviderRegistryStoragePaginatedProviders), err
	}

	out0 := *abi.ConvertType(out[0], new(ServiceProviderRegistryStoragePaginatedProviders)).(*ServiceProviderRegistryStoragePaginatedProviders)

	return out0, err

}

// GetProvidersByProductType is a free data retrieval call binding the contract method 0x6ba44226.
//
// Solidity: function getProvidersByProductType(uint8 productType, bool onlyActive, uint256 offset, uint256 limit) view returns(((uint256,(address,address,string,string,bool),(uint8,string[],bool),bytes[])[],bool) result)
func (_SPRegistry *SPRegistrySession) GetProvidersByProductType(productType uint8, onlyActive bool, offset *big.Int, limit *big.Int) (ServiceProviderRegistryStoragePaginatedProviders, error) {
	return _SPRegistry.Contract.GetProvidersByProductType(&_SPRegistry.CallOpts, productType, onlyActive, offset, limit)
}

// GetProvidersByProductType is a free data retrieval call binding the contract method 0x6ba44226.
//
// Solidity: function getProvidersByProductType(uint8 productType, bool onlyActive, uint256 offset, uint256 limit) view returns(((uint256,(address,address,string,string,bool),(uint8,string[],bool),bytes[])[],bool) result)
func (_SPRegistry *SPRegistryCallerSession) GetProvidersByProductType(productType uint8, onlyActive bool, offset *big.Int, limit *big.Int) (ServiceProviderRegistryStoragePaginatedProviders, error) {
	return _SPRegistry.Contract.GetProvidersByProductType(&_SPRegistry.CallOpts, productType, onlyActive, offset, limit)
}

// IsProviderActive is a free data retrieval call binding the contract method 0x83df54a5.
//
// Solidity: function isProviderActive(uint256 providerId) view returns(bool)
func (_SPRegistry *SPRegistryCaller) IsProviderActive(opts *bind.CallOpts, providerId *big.Int) (bool, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "isProviderActive", providerId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsProviderActive is a free data retrieval call binding the contract method 0x83df54a5.
//
// Solidity: function isProviderActive(uint256 providerId) view returns(bool)
func (_SPRegistry *SPRegistrySession) IsProviderActive(providerId *big.Int) (bool, error) {
	return _SPRegistry.Contract.IsProviderActive(&_SPRegistry.CallOpts, providerId)
}

// IsProviderActive is a free data retrieval call binding the contract method 0x83df54a5.
//
// Solidity: function isProviderActive(uint256 providerId) view returns(bool)
func (_SPRegistry *SPRegistryCallerSession) IsProviderActive(providerId *big.Int) (bool, error) {
	return _SPRegistry.Contract.IsProviderActive(&_SPRegistry.CallOpts, providerId)
}

// IsRegisteredProvider is a free data retrieval call binding the contract method 0x51ca236f.
//
// Solidity: function isRegisteredProvider(address provider) view returns(bool)
func (_SPRegistry *SPRegistryCaller) IsRegisteredProvider(opts *bind.CallOpts, provider common.Address) (bool, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "isRegisteredProvider", provider)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsRegisteredProvider is a free data retrieval call binding the contract method 0x51ca236f.
//
// Solidity: function isRegisteredProvider(address provider) view returns(bool)
func (_SPRegistry *SPRegistrySession) IsRegisteredProvider(provider common.Address) (bool, error) {
	return _SPRegistry.Contract.IsRegisteredProvider(&_SPRegistry.CallOpts, provider)
}

// IsRegisteredProvider is a free data retrieval call binding the contract method 0x51ca236f.
//
// Solidity: function isRegisteredProvider(address provider) view returns(bool)
func (_SPRegistry *SPRegistryCallerSession) IsRegisteredProvider(provider common.Address) (bool, error) {
	return _SPRegistry.Contract.IsRegisteredProvider(&_SPRegistry.CallOpts, provider)
}

// NextUpgrade is a free data retrieval call binding the contract method 0x315e49ea.
//
// Solidity: function nextUpgrade() view returns(address nextImplementation, uint96 afterEpoch)
func (_SPRegistry *SPRegistryCaller) NextUpgrade(opts *bind.CallOpts) (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "nextUpgrade")

	outstruct := new(struct {
		NextImplementation common.Address
		AfterEpoch         *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.NextImplementation = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.AfterEpoch = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// NextUpgrade is a free data retrieval call binding the contract method 0x315e49ea.
//
// Solidity: function nextUpgrade() view returns(address nextImplementation, uint96 afterEpoch)
func (_SPRegistry *SPRegistrySession) NextUpgrade() (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	return _SPRegistry.Contract.NextUpgrade(&_SPRegistry.CallOpts)
}

// NextUpgrade is a free data retrieval call binding the contract method 0x315e49ea.
//
// Solidity: function nextUpgrade() view returns(address nextImplementation, uint96 afterEpoch)
func (_SPRegistry *SPRegistryCallerSession) NextUpgrade() (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	return _SPRegistry.Contract.NextUpgrade(&_SPRegistry.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_SPRegistry *SPRegistryCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_SPRegistry *SPRegistrySession) Owner() (common.Address, error) {
	return _SPRegistry.Contract.Owner(&_SPRegistry.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_SPRegistry *SPRegistryCallerSession) Owner() (common.Address, error) {
	return _SPRegistry.Contract.Owner(&_SPRegistry.CallOpts)
}

// ProductCapabilities is a free data retrieval call binding the contract method 0x4368bafb.
//
// Solidity: function productCapabilities(uint256 providerId, uint8 productType, string key) view returns(bytes value)
func (_SPRegistry *SPRegistryCaller) ProductCapabilities(opts *bind.CallOpts, providerId *big.Int, productType uint8, key string) ([]byte, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "productCapabilities", providerId, productType, key)

	if err != nil {
		return *new([]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([]byte)).(*[]byte)

	return out0, err

}

// ProductCapabilities is a free data retrieval call binding the contract method 0x4368bafb.
//
// Solidity: function productCapabilities(uint256 providerId, uint8 productType, string key) view returns(bytes value)
func (_SPRegistry *SPRegistrySession) ProductCapabilities(providerId *big.Int, productType uint8, key string) ([]byte, error) {
	return _SPRegistry.Contract.ProductCapabilities(&_SPRegistry.CallOpts, providerId, productType, key)
}

// ProductCapabilities is a free data retrieval call binding the contract method 0x4368bafb.
//
// Solidity: function productCapabilities(uint256 providerId, uint8 productType, string key) view returns(bytes value)
func (_SPRegistry *SPRegistryCallerSession) ProductCapabilities(providerId *big.Int, productType uint8, key string) ([]byte, error) {
	return _SPRegistry.Contract.ProductCapabilities(&_SPRegistry.CallOpts, providerId, productType, key)
}

// ProductTypeProviderCount is a free data retrieval call binding the contract method 0xe459382f.
//
// Solidity: function productTypeProviderCount(uint8 productType) view returns(uint256 count)
func (_SPRegistry *SPRegistryCaller) ProductTypeProviderCount(opts *bind.CallOpts, productType uint8) (*big.Int, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "productTypeProviderCount", productType)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ProductTypeProviderCount is a free data retrieval call binding the contract method 0xe459382f.
//
// Solidity: function productTypeProviderCount(uint8 productType) view returns(uint256 count)
func (_SPRegistry *SPRegistrySession) ProductTypeProviderCount(productType uint8) (*big.Int, error) {
	return _SPRegistry.Contract.ProductTypeProviderCount(&_SPRegistry.CallOpts, productType)
}

// ProductTypeProviderCount is a free data retrieval call binding the contract method 0xe459382f.
//
// Solidity: function productTypeProviderCount(uint8 productType) view returns(uint256 count)
func (_SPRegistry *SPRegistryCallerSession) ProductTypeProviderCount(productType uint8) (*big.Int, error) {
	return _SPRegistry.Contract.ProductTypeProviderCount(&_SPRegistry.CallOpts, productType)
}

// ProviderHasProduct is a free data retrieval call binding the contract method 0xcde24beb.
//
// Solidity: function providerHasProduct(uint256 providerId, uint8 productType) view returns(bool)
func (_SPRegistry *SPRegistryCaller) ProviderHasProduct(opts *bind.CallOpts, providerId *big.Int, productType uint8) (bool, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "providerHasProduct", providerId, productType)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ProviderHasProduct is a free data retrieval call binding the contract method 0xcde24beb.
//
// Solidity: function providerHasProduct(uint256 providerId, uint8 productType) view returns(bool)
func (_SPRegistry *SPRegistrySession) ProviderHasProduct(providerId *big.Int, productType uint8) (bool, error) {
	return _SPRegistry.Contract.ProviderHasProduct(&_SPRegistry.CallOpts, providerId, productType)
}

// ProviderHasProduct is a free data retrieval call binding the contract method 0xcde24beb.
//
// Solidity: function providerHasProduct(uint256 providerId, uint8 productType) view returns(bool)
func (_SPRegistry *SPRegistryCallerSession) ProviderHasProduct(providerId *big.Int, productType uint8) (bool, error) {
	return _SPRegistry.Contract.ProviderHasProduct(&_SPRegistry.CallOpts, providerId, productType)
}

// ProviderProducts is a free data retrieval call binding the contract method 0x6bf6d74f.
//
// Solidity: function providerProducts(uint256 providerId, uint8 productType) view returns(uint8 productType, bool isActive)
func (_SPRegistry *SPRegistryCaller) ProviderProducts(opts *bind.CallOpts, providerId *big.Int, productType uint8) (struct {
	ProductType uint8
	IsActive    bool
}, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "providerProducts", providerId, productType)

	outstruct := new(struct {
		ProductType uint8
		IsActive    bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ProductType = *abi.ConvertType(out[0], new(uint8)).(*uint8)
	outstruct.IsActive = *abi.ConvertType(out[1], new(bool)).(*bool)

	return *outstruct, err

}

// ProviderProducts is a free data retrieval call binding the contract method 0x6bf6d74f.
//
// Solidity: function providerProducts(uint256 providerId, uint8 productType) view returns(uint8 productType, bool isActive)
func (_SPRegistry *SPRegistrySession) ProviderProducts(providerId *big.Int, productType uint8) (struct {
	ProductType uint8
	IsActive    bool
}, error) {
	return _SPRegistry.Contract.ProviderProducts(&_SPRegistry.CallOpts, providerId, productType)
}

// ProviderProducts is a free data retrieval call binding the contract method 0x6bf6d74f.
//
// Solidity: function providerProducts(uint256 providerId, uint8 productType) view returns(uint8 productType, bool isActive)
func (_SPRegistry *SPRegistryCallerSession) ProviderProducts(providerId *big.Int, productType uint8) (struct {
	ProductType uint8
	IsActive    bool
}, error) {
	return _SPRegistry.Contract.ProviderProducts(&_SPRegistry.CallOpts, providerId, productType)
}

// Providers is a free data retrieval call binding the contract method 0x50f3fc81.
//
// Solidity: function providers(uint256 providerId) view returns(address serviceProvider, address payee, string name, string description, bool isActive)
func (_SPRegistry *SPRegistryCaller) Providers(opts *bind.CallOpts, providerId *big.Int) (struct {
	ServiceProvider common.Address
	Payee           common.Address
	Name            string
	Description     string
	IsActive        bool
}, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "providers", providerId)

	outstruct := new(struct {
		ServiceProvider common.Address
		Payee           common.Address
		Name            string
		Description     string
		IsActive        bool
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ServiceProvider = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.Payee = *abi.ConvertType(out[1], new(common.Address)).(*common.Address)
	outstruct.Name = *abi.ConvertType(out[2], new(string)).(*string)
	outstruct.Description = *abi.ConvertType(out[3], new(string)).(*string)
	outstruct.IsActive = *abi.ConvertType(out[4], new(bool)).(*bool)

	return *outstruct, err

}

// Providers is a free data retrieval call binding the contract method 0x50f3fc81.
//
// Solidity: function providers(uint256 providerId) view returns(address serviceProvider, address payee, string name, string description, bool isActive)
func (_SPRegistry *SPRegistrySession) Providers(providerId *big.Int) (struct {
	ServiceProvider common.Address
	Payee           common.Address
	Name            string
	Description     string
	IsActive        bool
}, error) {
	return _SPRegistry.Contract.Providers(&_SPRegistry.CallOpts, providerId)
}

// Providers is a free data retrieval call binding the contract method 0x50f3fc81.
//
// Solidity: function providers(uint256 providerId) view returns(address serviceProvider, address payee, string name, string description, bool isActive)
func (_SPRegistry *SPRegistryCallerSession) Providers(providerId *big.Int) (struct {
	ServiceProvider common.Address
	Payee           common.Address
	Name            string
	Description     string
	IsActive        bool
}, error) {
	return _SPRegistry.Contract.Providers(&_SPRegistry.CallOpts, providerId)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_SPRegistry *SPRegistryCaller) ProxiableUUID(opts *bind.CallOpts) ([32]byte, error) {
	var out []interface{}
	err := _SPRegistry.contract.Call(opts, &out, "proxiableUUID")

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_SPRegistry *SPRegistrySession) ProxiableUUID() ([32]byte, error) {
	return _SPRegistry.Contract.ProxiableUUID(&_SPRegistry.CallOpts)
}

// ProxiableUUID is a free data retrieval call binding the contract method 0x52d1902d.
//
// Solidity: function proxiableUUID() view returns(bytes32)
func (_SPRegistry *SPRegistryCallerSession) ProxiableUUID() ([32]byte, error) {
	return _SPRegistry.Contract.ProxiableUUID(&_SPRegistry.CallOpts)
}

// AddProduct is a paid mutator transaction binding the contract method 0x360cc6ac.
//
// Solidity: function addProduct(uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) returns()
func (_SPRegistry *SPRegistryTransactor) AddProduct(opts *bind.TransactOpts, productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "addProduct", productType, capabilityKeys, capabilityValues)
}

// AddProduct is a paid mutator transaction binding the contract method 0x360cc6ac.
//
// Solidity: function addProduct(uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) returns()
func (_SPRegistry *SPRegistrySession) AddProduct(productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.AddProduct(&_SPRegistry.TransactOpts, productType, capabilityKeys, capabilityValues)
}

// AddProduct is a paid mutator transaction binding the contract method 0x360cc6ac.
//
// Solidity: function addProduct(uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) returns()
func (_SPRegistry *SPRegistryTransactorSession) AddProduct(productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.AddProduct(&_SPRegistry.TransactOpts, productType, capabilityKeys, capabilityValues)
}

// AnnouncePlannedUpgrade is a paid mutator transaction binding the contract method 0xbd003827.
//
// Solidity: function announcePlannedUpgrade((address,uint96) plannedUpgrade) returns()
func (_SPRegistry *SPRegistryTransactor) AnnouncePlannedUpgrade(opts *bind.TransactOpts, plannedUpgrade ServiceProviderRegistryPlannedUpgrade) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "announcePlannedUpgrade", plannedUpgrade)
}

// AnnouncePlannedUpgrade is a paid mutator transaction binding the contract method 0xbd003827.
//
// Solidity: function announcePlannedUpgrade((address,uint96) plannedUpgrade) returns()
func (_SPRegistry *SPRegistrySession) AnnouncePlannedUpgrade(plannedUpgrade ServiceProviderRegistryPlannedUpgrade) (*types.Transaction, error) {
	return _SPRegistry.Contract.AnnouncePlannedUpgrade(&_SPRegistry.TransactOpts, plannedUpgrade)
}

// AnnouncePlannedUpgrade is a paid mutator transaction binding the contract method 0xbd003827.
//
// Solidity: function announcePlannedUpgrade((address,uint96) plannedUpgrade) returns()
func (_SPRegistry *SPRegistryTransactorSession) AnnouncePlannedUpgrade(plannedUpgrade ServiceProviderRegistryPlannedUpgrade) (*types.Transaction, error) {
	return _SPRegistry.Contract.AnnouncePlannedUpgrade(&_SPRegistry.TransactOpts, plannedUpgrade)
}

// Initialize is a paid mutator transaction binding the contract method 0x8129fc1c.
//
// Solidity: function initialize() returns()
func (_SPRegistry *SPRegistryTransactor) Initialize(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "initialize")
}

// Initialize is a paid mutator transaction binding the contract method 0x8129fc1c.
//
// Solidity: function initialize() returns()
func (_SPRegistry *SPRegistrySession) Initialize() (*types.Transaction, error) {
	return _SPRegistry.Contract.Initialize(&_SPRegistry.TransactOpts)
}

// Initialize is a paid mutator transaction binding the contract method 0x8129fc1c.
//
// Solidity: function initialize() returns()
func (_SPRegistry *SPRegistryTransactorSession) Initialize() (*types.Transaction, error) {
	return _SPRegistry.Contract.Initialize(&_SPRegistry.TransactOpts)
}

// Migrate is a paid mutator transaction binding the contract method 0xc9c5b5b4.
//
// Solidity: function migrate(string newVersion) returns()
func (_SPRegistry *SPRegistryTransactor) Migrate(opts *bind.TransactOpts, newVersion string) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "migrate", newVersion)
}

// Migrate is a paid mutator transaction binding the contract method 0xc9c5b5b4.
//
// Solidity: function migrate(string newVersion) returns()
func (_SPRegistry *SPRegistrySession) Migrate(newVersion string) (*types.Transaction, error) {
	return _SPRegistry.Contract.Migrate(&_SPRegistry.TransactOpts, newVersion)
}

// Migrate is a paid mutator transaction binding the contract method 0xc9c5b5b4.
//
// Solidity: function migrate(string newVersion) returns()
func (_SPRegistry *SPRegistryTransactorSession) Migrate(newVersion string) (*types.Transaction, error) {
	return _SPRegistry.Contract.Migrate(&_SPRegistry.TransactOpts, newVersion)
}

// RegisterProvider is a paid mutator transaction binding the contract method 0x90d270c2.
//
// Solidity: function registerProvider(address payee, string name, string description, uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) payable returns(uint256 providerId)
func (_SPRegistry *SPRegistryTransactor) RegisterProvider(opts *bind.TransactOpts, payee common.Address, name string, description string, productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "registerProvider", payee, name, description, productType, capabilityKeys, capabilityValues)
}

// RegisterProvider is a paid mutator transaction binding the contract method 0x90d270c2.
//
// Solidity: function registerProvider(address payee, string name, string description, uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) payable returns(uint256 providerId)
func (_SPRegistry *SPRegistrySession) RegisterProvider(payee common.Address, name string, description string, productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.RegisterProvider(&_SPRegistry.TransactOpts, payee, name, description, productType, capabilityKeys, capabilityValues)
}

// RegisterProvider is a paid mutator transaction binding the contract method 0x90d270c2.
//
// Solidity: function registerProvider(address payee, string name, string description, uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) payable returns(uint256 providerId)
func (_SPRegistry *SPRegistryTransactorSession) RegisterProvider(payee common.Address, name string, description string, productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.RegisterProvider(&_SPRegistry.TransactOpts, payee, name, description, productType, capabilityKeys, capabilityValues)
}

// RemoveProduct is a paid mutator transaction binding the contract method 0xa9d239b6.
//
// Solidity: function removeProduct(uint8 productType) returns()
func (_SPRegistry *SPRegistryTransactor) RemoveProduct(opts *bind.TransactOpts, productType uint8) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "removeProduct", productType)
}

// RemoveProduct is a paid mutator transaction binding the contract method 0xa9d239b6.
//
// Solidity: function removeProduct(uint8 productType) returns()
func (_SPRegistry *SPRegistrySession) RemoveProduct(productType uint8) (*types.Transaction, error) {
	return _SPRegistry.Contract.RemoveProduct(&_SPRegistry.TransactOpts, productType)
}

// RemoveProduct is a paid mutator transaction binding the contract method 0xa9d239b6.
//
// Solidity: function removeProduct(uint8 productType) returns()
func (_SPRegistry *SPRegistryTransactorSession) RemoveProduct(productType uint8) (*types.Transaction, error) {
	return _SPRegistry.Contract.RemoveProduct(&_SPRegistry.TransactOpts, productType)
}

// RemoveProvider is a paid mutator transaction binding the contract method 0xb6363b99.
//
// Solidity: function removeProvider() returns()
func (_SPRegistry *SPRegistryTransactor) RemoveProvider(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "removeProvider")
}

// RemoveProvider is a paid mutator transaction binding the contract method 0xb6363b99.
//
// Solidity: function removeProvider() returns()
func (_SPRegistry *SPRegistrySession) RemoveProvider() (*types.Transaction, error) {
	return _SPRegistry.Contract.RemoveProvider(&_SPRegistry.TransactOpts)
}

// RemoveProvider is a paid mutator transaction binding the contract method 0xb6363b99.
//
// Solidity: function removeProvider() returns()
func (_SPRegistry *SPRegistryTransactorSession) RemoveProvider() (*types.Transaction, error) {
	return _SPRegistry.Contract.RemoveProvider(&_SPRegistry.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_SPRegistry *SPRegistryTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_SPRegistry *SPRegistrySession) RenounceOwnership() (*types.Transaction, error) {
	return _SPRegistry.Contract.RenounceOwnership(&_SPRegistry.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_SPRegistry *SPRegistryTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _SPRegistry.Contract.RenounceOwnership(&_SPRegistry.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_SPRegistry *SPRegistryTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_SPRegistry *SPRegistrySession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _SPRegistry.Contract.TransferOwnership(&_SPRegistry.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_SPRegistry *SPRegistryTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _SPRegistry.Contract.TransferOwnership(&_SPRegistry.TransactOpts, newOwner)
}

// UpdateProduct is a paid mutator transaction binding the contract method 0xa128c005.
//
// Solidity: function updateProduct(uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) returns()
func (_SPRegistry *SPRegistryTransactor) UpdateProduct(opts *bind.TransactOpts, productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "updateProduct", productType, capabilityKeys, capabilityValues)
}

// UpdateProduct is a paid mutator transaction binding the contract method 0xa128c005.
//
// Solidity: function updateProduct(uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) returns()
func (_SPRegistry *SPRegistrySession) UpdateProduct(productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.UpdateProduct(&_SPRegistry.TransactOpts, productType, capabilityKeys, capabilityValues)
}

// UpdateProduct is a paid mutator transaction binding the contract method 0xa128c005.
//
// Solidity: function updateProduct(uint8 productType, string[] capabilityKeys, bytes[] capabilityValues) returns()
func (_SPRegistry *SPRegistryTransactorSession) UpdateProduct(productType uint8, capabilityKeys []string, capabilityValues [][]byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.UpdateProduct(&_SPRegistry.TransactOpts, productType, capabilityKeys, capabilityValues)
}

// UpdateProviderInfo is a paid mutator transaction binding the contract method 0xd1c21b5b.
//
// Solidity: function updateProviderInfo(string name, string description) returns()
func (_SPRegistry *SPRegistryTransactor) UpdateProviderInfo(opts *bind.TransactOpts, name string, description string) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "updateProviderInfo", name, description)
}

// UpdateProviderInfo is a paid mutator transaction binding the contract method 0xd1c21b5b.
//
// Solidity: function updateProviderInfo(string name, string description) returns()
func (_SPRegistry *SPRegistrySession) UpdateProviderInfo(name string, description string) (*types.Transaction, error) {
	return _SPRegistry.Contract.UpdateProviderInfo(&_SPRegistry.TransactOpts, name, description)
}

// UpdateProviderInfo is a paid mutator transaction binding the contract method 0xd1c21b5b.
//
// Solidity: function updateProviderInfo(string name, string description) returns()
func (_SPRegistry *SPRegistryTransactorSession) UpdateProviderInfo(name string, description string) (*types.Transaction, error) {
	return _SPRegistry.Contract.UpdateProviderInfo(&_SPRegistry.TransactOpts, name, description)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_SPRegistry *SPRegistryTransactor) UpgradeToAndCall(opts *bind.TransactOpts, newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _SPRegistry.contract.Transact(opts, "upgradeToAndCall", newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_SPRegistry *SPRegistrySession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.UpgradeToAndCall(&_SPRegistry.TransactOpts, newImplementation, data)
}

// UpgradeToAndCall is a paid mutator transaction binding the contract method 0x4f1ef286.
//
// Solidity: function upgradeToAndCall(address newImplementation, bytes data) payable returns()
func (_SPRegistry *SPRegistryTransactorSession) UpgradeToAndCall(newImplementation common.Address, data []byte) (*types.Transaction, error) {
	return _SPRegistry.Contract.UpgradeToAndCall(&_SPRegistry.TransactOpts, newImplementation, data)
}

// SPRegistryContractUpgradedIterator is returned from FilterContractUpgraded and is used to iterate over the raw logs and unpacked data for ContractUpgraded events raised by the SPRegistry contract.
type SPRegistryContractUpgradedIterator struct {
	Event *SPRegistryContractUpgraded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryContractUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryContractUpgraded)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryContractUpgraded)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryContractUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryContractUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryContractUpgraded represents a ContractUpgraded event raised by the SPRegistry contract.
type SPRegistryContractUpgraded struct {
	Version        string
	Implementation common.Address
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterContractUpgraded is a free log retrieval operation binding the contract event 0x2b51ff7c4cc8e6fe1c72e9d9685b7d2a88a5d82ad3a644afbdceb0272c89c1c3.
//
// Solidity: event ContractUpgraded(string version, address implementation)
func (_SPRegistry *SPRegistryFilterer) FilterContractUpgraded(opts *bind.FilterOpts) (*SPRegistryContractUpgradedIterator, error) {

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "ContractUpgraded")
	if err != nil {
		return nil, err
	}
	return &SPRegistryContractUpgradedIterator{contract: _SPRegistry.contract, event: "ContractUpgraded", logs: logs, sub: sub}, nil
}

// WatchContractUpgraded is a free log subscription operation binding the contract event 0x2b51ff7c4cc8e6fe1c72e9d9685b7d2a88a5d82ad3a644afbdceb0272c89c1c3.
//
// Solidity: event ContractUpgraded(string version, address implementation)
func (_SPRegistry *SPRegistryFilterer) WatchContractUpgraded(opts *bind.WatchOpts, sink chan<- *SPRegistryContractUpgraded) (event.Subscription, error) {

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "ContractUpgraded")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryContractUpgraded)
				if err := _SPRegistry.contract.UnpackLog(event, "ContractUpgraded", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseContractUpgraded is a log parse operation binding the contract event 0x2b51ff7c4cc8e6fe1c72e9d9685b7d2a88a5d82ad3a644afbdceb0272c89c1c3.
//
// Solidity: event ContractUpgraded(string version, address implementation)
func (_SPRegistry *SPRegistryFilterer) ParseContractUpgraded(log types.Log) (*SPRegistryContractUpgraded, error) {
	event := new(SPRegistryContractUpgraded)
	if err := _SPRegistry.contract.UnpackLog(event, "ContractUpgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryEIP712DomainChangedIterator is returned from FilterEIP712DomainChanged and is used to iterate over the raw logs and unpacked data for EIP712DomainChanged events raised by the SPRegistry contract.
type SPRegistryEIP712DomainChangedIterator struct {
	Event *SPRegistryEIP712DomainChanged // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryEIP712DomainChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryEIP712DomainChanged)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryEIP712DomainChanged)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryEIP712DomainChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryEIP712DomainChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryEIP712DomainChanged represents a EIP712DomainChanged event raised by the SPRegistry contract.
type SPRegistryEIP712DomainChanged struct {
	Raw types.Log // Blockchain specific contextual infos
}

// FilterEIP712DomainChanged is a free log retrieval operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_SPRegistry *SPRegistryFilterer) FilterEIP712DomainChanged(opts *bind.FilterOpts) (*SPRegistryEIP712DomainChangedIterator, error) {

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return &SPRegistryEIP712DomainChangedIterator{contract: _SPRegistry.contract, event: "EIP712DomainChanged", logs: logs, sub: sub}, nil
}

// WatchEIP712DomainChanged is a free log subscription operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_SPRegistry *SPRegistryFilterer) WatchEIP712DomainChanged(opts *bind.WatchOpts, sink chan<- *SPRegistryEIP712DomainChanged) (event.Subscription, error) {

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryEIP712DomainChanged)
				if err := _SPRegistry.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseEIP712DomainChanged is a log parse operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_SPRegistry *SPRegistryFilterer) ParseEIP712DomainChanged(log types.Log) (*SPRegistryEIP712DomainChanged, error) {
	event := new(SPRegistryEIP712DomainChanged)
	if err := _SPRegistry.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the SPRegistry contract.
type SPRegistryInitializedIterator struct {
	Event *SPRegistryInitialized // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryInitialized)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryInitialized)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryInitialized represents a Initialized event raised by the SPRegistry contract.
type SPRegistryInitialized struct {
	Version uint64
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_SPRegistry *SPRegistryFilterer) FilterInitialized(opts *bind.FilterOpts) (*SPRegistryInitializedIterator, error) {

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &SPRegistryInitializedIterator{contract: _SPRegistry.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_SPRegistry *SPRegistryFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *SPRegistryInitialized) (event.Subscription, error) {

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryInitialized)
				if err := _SPRegistry.contract.UnpackLog(event, "Initialized", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseInitialized is a log parse operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_SPRegistry *SPRegistryFilterer) ParseInitialized(log types.Log) (*SPRegistryInitialized, error) {
	event := new(SPRegistryInitialized)
	if err := _SPRegistry.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the SPRegistry contract.
type SPRegistryOwnershipTransferredIterator struct {
	Event *SPRegistryOwnershipTransferred // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryOwnershipTransferred)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryOwnershipTransferred)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryOwnershipTransferred represents a OwnershipTransferred event raised by the SPRegistry contract.
type SPRegistryOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_SPRegistry *SPRegistryFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*SPRegistryOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryOwnershipTransferredIterator{contract: _SPRegistry.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_SPRegistry *SPRegistryFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *SPRegistryOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryOwnershipTransferred)
				if err := _SPRegistry.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseOwnershipTransferred is a log parse operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_SPRegistry *SPRegistryFilterer) ParseOwnershipTransferred(log types.Log) (*SPRegistryOwnershipTransferred, error) {
	event := new(SPRegistryOwnershipTransferred)
	if err := _SPRegistry.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryProductAddedIterator is returned from FilterProductAdded and is used to iterate over the raw logs and unpacked data for ProductAdded events raised by the SPRegistry contract.
type SPRegistryProductAddedIterator struct {
	Event *SPRegistryProductAdded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryProductAddedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryProductAdded)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryProductAdded)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryProductAddedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryProductAddedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryProductAdded represents a ProductAdded event raised by the SPRegistry contract.
type SPRegistryProductAdded struct {
	ProviderId       *big.Int
	ProductType      uint8
	ServiceProvider  common.Address
	CapabilityKeys   []string
	CapabilityValues [][]byte
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterProductAdded is a free log retrieval operation binding the contract event 0xf736f1c7fa0eb68f0384383abc0d4dcc8208127acfb5c87f03f965f2a8a69686.
//
// Solidity: event ProductAdded(uint256 indexed providerId, uint8 indexed productType, address serviceProvider, string[] capabilityKeys, bytes[] capabilityValues)
func (_SPRegistry *SPRegistryFilterer) FilterProductAdded(opts *bind.FilterOpts, providerId []*big.Int, productType []uint8) (*SPRegistryProductAddedIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var productTypeRule []interface{}
	for _, productTypeItem := range productType {
		productTypeRule = append(productTypeRule, productTypeItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "ProductAdded", providerIdRule, productTypeRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryProductAddedIterator{contract: _SPRegistry.contract, event: "ProductAdded", logs: logs, sub: sub}, nil
}

// WatchProductAdded is a free log subscription operation binding the contract event 0xf736f1c7fa0eb68f0384383abc0d4dcc8208127acfb5c87f03f965f2a8a69686.
//
// Solidity: event ProductAdded(uint256 indexed providerId, uint8 indexed productType, address serviceProvider, string[] capabilityKeys, bytes[] capabilityValues)
func (_SPRegistry *SPRegistryFilterer) WatchProductAdded(opts *bind.WatchOpts, sink chan<- *SPRegistryProductAdded, providerId []*big.Int, productType []uint8) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var productTypeRule []interface{}
	for _, productTypeItem := range productType {
		productTypeRule = append(productTypeRule, productTypeItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "ProductAdded", providerIdRule, productTypeRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryProductAdded)
				if err := _SPRegistry.contract.UnpackLog(event, "ProductAdded", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseProductAdded is a log parse operation binding the contract event 0xf736f1c7fa0eb68f0384383abc0d4dcc8208127acfb5c87f03f965f2a8a69686.
//
// Solidity: event ProductAdded(uint256 indexed providerId, uint8 indexed productType, address serviceProvider, string[] capabilityKeys, bytes[] capabilityValues)
func (_SPRegistry *SPRegistryFilterer) ParseProductAdded(log types.Log) (*SPRegistryProductAdded, error) {
	event := new(SPRegistryProductAdded)
	if err := _SPRegistry.contract.UnpackLog(event, "ProductAdded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryProductRemovedIterator is returned from FilterProductRemoved and is used to iterate over the raw logs and unpacked data for ProductRemoved events raised by the SPRegistry contract.
type SPRegistryProductRemovedIterator struct {
	Event *SPRegistryProductRemoved // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryProductRemovedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryProductRemoved)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryProductRemoved)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryProductRemovedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryProductRemovedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryProductRemoved represents a ProductRemoved event raised by the SPRegistry contract.
type SPRegistryProductRemoved struct {
	ProviderId  *big.Int
	ProductType uint8
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterProductRemoved is a free log retrieval operation binding the contract event 0x4c363c6cd3d80189ef501b26de41894b3ed5e7b4a85b096be6cbcaa8a13e5e4d.
//
// Solidity: event ProductRemoved(uint256 indexed providerId, uint8 indexed productType)
func (_SPRegistry *SPRegistryFilterer) FilterProductRemoved(opts *bind.FilterOpts, providerId []*big.Int, productType []uint8) (*SPRegistryProductRemovedIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var productTypeRule []interface{}
	for _, productTypeItem := range productType {
		productTypeRule = append(productTypeRule, productTypeItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "ProductRemoved", providerIdRule, productTypeRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryProductRemovedIterator{contract: _SPRegistry.contract, event: "ProductRemoved", logs: logs, sub: sub}, nil
}

// WatchProductRemoved is a free log subscription operation binding the contract event 0x4c363c6cd3d80189ef501b26de41894b3ed5e7b4a85b096be6cbcaa8a13e5e4d.
//
// Solidity: event ProductRemoved(uint256 indexed providerId, uint8 indexed productType)
func (_SPRegistry *SPRegistryFilterer) WatchProductRemoved(opts *bind.WatchOpts, sink chan<- *SPRegistryProductRemoved, providerId []*big.Int, productType []uint8) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var productTypeRule []interface{}
	for _, productTypeItem := range productType {
		productTypeRule = append(productTypeRule, productTypeItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "ProductRemoved", providerIdRule, productTypeRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryProductRemoved)
				if err := _SPRegistry.contract.UnpackLog(event, "ProductRemoved", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseProductRemoved is a log parse operation binding the contract event 0x4c363c6cd3d80189ef501b26de41894b3ed5e7b4a85b096be6cbcaa8a13e5e4d.
//
// Solidity: event ProductRemoved(uint256 indexed providerId, uint8 indexed productType)
func (_SPRegistry *SPRegistryFilterer) ParseProductRemoved(log types.Log) (*SPRegistryProductRemoved, error) {
	event := new(SPRegistryProductRemoved)
	if err := _SPRegistry.contract.UnpackLog(event, "ProductRemoved", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryProductUpdatedIterator is returned from FilterProductUpdated and is used to iterate over the raw logs and unpacked data for ProductUpdated events raised by the SPRegistry contract.
type SPRegistryProductUpdatedIterator struct {
	Event *SPRegistryProductUpdated // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryProductUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryProductUpdated)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryProductUpdated)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryProductUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryProductUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryProductUpdated represents a ProductUpdated event raised by the SPRegistry contract.
type SPRegistryProductUpdated struct {
	ProviderId       *big.Int
	ProductType      uint8
	ServiceProvider  common.Address
	CapabilityKeys   []string
	CapabilityValues [][]byte
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterProductUpdated is a free log retrieval operation binding the contract event 0x19305e69de03c2e3298427ad2c225fef7bc07a55c9a1a6b930f5d21ad6f22148.
//
// Solidity: event ProductUpdated(uint256 indexed providerId, uint8 indexed productType, address serviceProvider, string[] capabilityKeys, bytes[] capabilityValues)
func (_SPRegistry *SPRegistryFilterer) FilterProductUpdated(opts *bind.FilterOpts, providerId []*big.Int, productType []uint8) (*SPRegistryProductUpdatedIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var productTypeRule []interface{}
	for _, productTypeItem := range productType {
		productTypeRule = append(productTypeRule, productTypeItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "ProductUpdated", providerIdRule, productTypeRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryProductUpdatedIterator{contract: _SPRegistry.contract, event: "ProductUpdated", logs: logs, sub: sub}, nil
}

// WatchProductUpdated is a free log subscription operation binding the contract event 0x19305e69de03c2e3298427ad2c225fef7bc07a55c9a1a6b930f5d21ad6f22148.
//
// Solidity: event ProductUpdated(uint256 indexed providerId, uint8 indexed productType, address serviceProvider, string[] capabilityKeys, bytes[] capabilityValues)
func (_SPRegistry *SPRegistryFilterer) WatchProductUpdated(opts *bind.WatchOpts, sink chan<- *SPRegistryProductUpdated, providerId []*big.Int, productType []uint8) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var productTypeRule []interface{}
	for _, productTypeItem := range productType {
		productTypeRule = append(productTypeRule, productTypeItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "ProductUpdated", providerIdRule, productTypeRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryProductUpdated)
				if err := _SPRegistry.contract.UnpackLog(event, "ProductUpdated", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseProductUpdated is a log parse operation binding the contract event 0x19305e69de03c2e3298427ad2c225fef7bc07a55c9a1a6b930f5d21ad6f22148.
//
// Solidity: event ProductUpdated(uint256 indexed providerId, uint8 indexed productType, address serviceProvider, string[] capabilityKeys, bytes[] capabilityValues)
func (_SPRegistry *SPRegistryFilterer) ParseProductUpdated(log types.Log) (*SPRegistryProductUpdated, error) {
	event := new(SPRegistryProductUpdated)
	if err := _SPRegistry.contract.UnpackLog(event, "ProductUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryProviderInfoUpdatedIterator is returned from FilterProviderInfoUpdated and is used to iterate over the raw logs and unpacked data for ProviderInfoUpdated events raised by the SPRegistry contract.
type SPRegistryProviderInfoUpdatedIterator struct {
	Event *SPRegistryProviderInfoUpdated // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryProviderInfoUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryProviderInfoUpdated)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryProviderInfoUpdated)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryProviderInfoUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryProviderInfoUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryProviderInfoUpdated represents a ProviderInfoUpdated event raised by the SPRegistry contract.
type SPRegistryProviderInfoUpdated struct {
	ProviderId *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterProviderInfoUpdated is a free log retrieval operation binding the contract event 0xae10af73bdb200f240b1ea85ef806346fb24c82388af00414f4c5fcfeef68f76.
//
// Solidity: event ProviderInfoUpdated(uint256 indexed providerId)
func (_SPRegistry *SPRegistryFilterer) FilterProviderInfoUpdated(opts *bind.FilterOpts, providerId []*big.Int) (*SPRegistryProviderInfoUpdatedIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "ProviderInfoUpdated", providerIdRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryProviderInfoUpdatedIterator{contract: _SPRegistry.contract, event: "ProviderInfoUpdated", logs: logs, sub: sub}, nil
}

// WatchProviderInfoUpdated is a free log subscription operation binding the contract event 0xae10af73bdb200f240b1ea85ef806346fb24c82388af00414f4c5fcfeef68f76.
//
// Solidity: event ProviderInfoUpdated(uint256 indexed providerId)
func (_SPRegistry *SPRegistryFilterer) WatchProviderInfoUpdated(opts *bind.WatchOpts, sink chan<- *SPRegistryProviderInfoUpdated, providerId []*big.Int) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "ProviderInfoUpdated", providerIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryProviderInfoUpdated)
				if err := _SPRegistry.contract.UnpackLog(event, "ProviderInfoUpdated", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseProviderInfoUpdated is a log parse operation binding the contract event 0xae10af73bdb200f240b1ea85ef806346fb24c82388af00414f4c5fcfeef68f76.
//
// Solidity: event ProviderInfoUpdated(uint256 indexed providerId)
func (_SPRegistry *SPRegistryFilterer) ParseProviderInfoUpdated(log types.Log) (*SPRegistryProviderInfoUpdated, error) {
	event := new(SPRegistryProviderInfoUpdated)
	if err := _SPRegistry.contract.UnpackLog(event, "ProviderInfoUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryProviderRegisteredIterator is returned from FilterProviderRegistered and is used to iterate over the raw logs and unpacked data for ProviderRegistered events raised by the SPRegistry contract.
type SPRegistryProviderRegisteredIterator struct {
	Event *SPRegistryProviderRegistered // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryProviderRegisteredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryProviderRegistered)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryProviderRegistered)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryProviderRegisteredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryProviderRegisteredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryProviderRegistered represents a ProviderRegistered event raised by the SPRegistry contract.
type SPRegistryProviderRegistered struct {
	ProviderId      *big.Int
	ServiceProvider common.Address
	Payee           common.Address
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterProviderRegistered is a free log retrieval operation binding the contract event 0xaff7a33d237d3d600a92c556cda34cb73cf7cccc667e163c90b1d2d392b031a5.
//
// Solidity: event ProviderRegistered(uint256 indexed providerId, address indexed serviceProvider, address indexed payee)
func (_SPRegistry *SPRegistryFilterer) FilterProviderRegistered(opts *bind.FilterOpts, providerId []*big.Int, serviceProvider []common.Address, payee []common.Address) (*SPRegistryProviderRegisteredIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var serviceProviderRule []interface{}
	for _, serviceProviderItem := range serviceProvider {
		serviceProviderRule = append(serviceProviderRule, serviceProviderItem)
	}
	var payeeRule []interface{}
	for _, payeeItem := range payee {
		payeeRule = append(payeeRule, payeeItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "ProviderRegistered", providerIdRule, serviceProviderRule, payeeRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryProviderRegisteredIterator{contract: _SPRegistry.contract, event: "ProviderRegistered", logs: logs, sub: sub}, nil
}

// WatchProviderRegistered is a free log subscription operation binding the contract event 0xaff7a33d237d3d600a92c556cda34cb73cf7cccc667e163c90b1d2d392b031a5.
//
// Solidity: event ProviderRegistered(uint256 indexed providerId, address indexed serviceProvider, address indexed payee)
func (_SPRegistry *SPRegistryFilterer) WatchProviderRegistered(opts *bind.WatchOpts, sink chan<- *SPRegistryProviderRegistered, providerId []*big.Int, serviceProvider []common.Address, payee []common.Address) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}
	var serviceProviderRule []interface{}
	for _, serviceProviderItem := range serviceProvider {
		serviceProviderRule = append(serviceProviderRule, serviceProviderItem)
	}
	var payeeRule []interface{}
	for _, payeeItem := range payee {
		payeeRule = append(payeeRule, payeeItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "ProviderRegistered", providerIdRule, serviceProviderRule, payeeRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryProviderRegistered)
				if err := _SPRegistry.contract.UnpackLog(event, "ProviderRegistered", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseProviderRegistered is a log parse operation binding the contract event 0xaff7a33d237d3d600a92c556cda34cb73cf7cccc667e163c90b1d2d392b031a5.
//
// Solidity: event ProviderRegistered(uint256 indexed providerId, address indexed serviceProvider, address indexed payee)
func (_SPRegistry *SPRegistryFilterer) ParseProviderRegistered(log types.Log) (*SPRegistryProviderRegistered, error) {
	event := new(SPRegistryProviderRegistered)
	if err := _SPRegistry.contract.UnpackLog(event, "ProviderRegistered", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryProviderRemovedIterator is returned from FilterProviderRemoved and is used to iterate over the raw logs and unpacked data for ProviderRemoved events raised by the SPRegistry contract.
type SPRegistryProviderRemovedIterator struct {
	Event *SPRegistryProviderRemoved // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryProviderRemovedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryProviderRemoved)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryProviderRemoved)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryProviderRemovedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryProviderRemovedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryProviderRemoved represents a ProviderRemoved event raised by the SPRegistry contract.
type SPRegistryProviderRemoved struct {
	ProviderId *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterProviderRemoved is a free log retrieval operation binding the contract event 0x452148878c72ebab44f2761cb8b0b79c50628a437350aee5f3aab66625addcc4.
//
// Solidity: event ProviderRemoved(uint256 indexed providerId)
func (_SPRegistry *SPRegistryFilterer) FilterProviderRemoved(opts *bind.FilterOpts, providerId []*big.Int) (*SPRegistryProviderRemovedIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "ProviderRemoved", providerIdRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryProviderRemovedIterator{contract: _SPRegistry.contract, event: "ProviderRemoved", logs: logs, sub: sub}, nil
}

// WatchProviderRemoved is a free log subscription operation binding the contract event 0x452148878c72ebab44f2761cb8b0b79c50628a437350aee5f3aab66625addcc4.
//
// Solidity: event ProviderRemoved(uint256 indexed providerId)
func (_SPRegistry *SPRegistryFilterer) WatchProviderRemoved(opts *bind.WatchOpts, sink chan<- *SPRegistryProviderRemoved, providerId []*big.Int) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "ProviderRemoved", providerIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryProviderRemoved)
				if err := _SPRegistry.contract.UnpackLog(event, "ProviderRemoved", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseProviderRemoved is a log parse operation binding the contract event 0x452148878c72ebab44f2761cb8b0b79c50628a437350aee5f3aab66625addcc4.
//
// Solidity: event ProviderRemoved(uint256 indexed providerId)
func (_SPRegistry *SPRegistryFilterer) ParseProviderRemoved(log types.Log) (*SPRegistryProviderRemoved, error) {
	event := new(SPRegistryProviderRemoved)
	if err := _SPRegistry.contract.UnpackLog(event, "ProviderRemoved", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryUpgradeAnnouncedIterator is returned from FilterUpgradeAnnounced and is used to iterate over the raw logs and unpacked data for UpgradeAnnounced events raised by the SPRegistry contract.
type SPRegistryUpgradeAnnouncedIterator struct {
	Event *SPRegistryUpgradeAnnounced // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryUpgradeAnnouncedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryUpgradeAnnounced)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryUpgradeAnnounced)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryUpgradeAnnouncedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryUpgradeAnnouncedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryUpgradeAnnounced represents a UpgradeAnnounced event raised by the SPRegistry contract.
type SPRegistryUpgradeAnnounced struct {
	PlannedUpgrade ServiceProviderRegistryPlannedUpgrade
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterUpgradeAnnounced is a free log retrieval operation binding the contract event 0xbcf8666408d712c75c2cbd790925afbec6495ca9e04186b1182902260a1d53cd.
//
// Solidity: event UpgradeAnnounced((address,uint96) plannedUpgrade)
func (_SPRegistry *SPRegistryFilterer) FilterUpgradeAnnounced(opts *bind.FilterOpts) (*SPRegistryUpgradeAnnouncedIterator, error) {

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "UpgradeAnnounced")
	if err != nil {
		return nil, err
	}
	return &SPRegistryUpgradeAnnouncedIterator{contract: _SPRegistry.contract, event: "UpgradeAnnounced", logs: logs, sub: sub}, nil
}

// WatchUpgradeAnnounced is a free log subscription operation binding the contract event 0xbcf8666408d712c75c2cbd790925afbec6495ca9e04186b1182902260a1d53cd.
//
// Solidity: event UpgradeAnnounced((address,uint96) plannedUpgrade)
func (_SPRegistry *SPRegistryFilterer) WatchUpgradeAnnounced(opts *bind.WatchOpts, sink chan<- *SPRegistryUpgradeAnnounced) (event.Subscription, error) {

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "UpgradeAnnounced")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryUpgradeAnnounced)
				if err := _SPRegistry.contract.UnpackLog(event, "UpgradeAnnounced", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUpgradeAnnounced is a log parse operation binding the contract event 0xbcf8666408d712c75c2cbd790925afbec6495ca9e04186b1182902260a1d53cd.
//
// Solidity: event UpgradeAnnounced((address,uint96) plannedUpgrade)
func (_SPRegistry *SPRegistryFilterer) ParseUpgradeAnnounced(log types.Log) (*SPRegistryUpgradeAnnounced, error) {
	event := new(SPRegistryUpgradeAnnounced)
	if err := _SPRegistry.contract.UnpackLog(event, "UpgradeAnnounced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// SPRegistryUpgradedIterator is returned from FilterUpgraded and is used to iterate over the raw logs and unpacked data for Upgraded events raised by the SPRegistry contract.
type SPRegistryUpgradedIterator struct {
	Event *SPRegistryUpgraded // Event containing the contract specifics and raw log

	contract *bind.BoundContract // Generic contract to use for unpacking event data
	event    string              // Event name to use for unpacking event data

	logs chan types.Log        // Log channel receiving the found contract events
	sub  ethereum.Subscription // Subscription for errors, completion and termination
	done bool                  // Whether the subscription completed delivering logs
	fail error                 // Occurred error to stop iteration
}

// Next advances the iterator to the subsequent event, returning whether there
// are any more events found. In case of a retrieval or parsing error, false is
// returned and Error() can be queried for the exact failure.
func (it *SPRegistryUpgradedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SPRegistryUpgraded)
			if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
				it.fail = err
				return false
			}
			it.Event.Raw = log
			return true

		default:
			return false
		}
	}
	// Iterator still in progress, wait for either a data or an error event
	select {
	case log := <-it.logs:
		it.Event = new(SPRegistryUpgraded)
		if err := it.contract.UnpackLog(it.Event, it.event, log); err != nil {
			it.fail = err
			return false
		}
		it.Event.Raw = log
		return true

	case err := <-it.sub.Err():
		it.done = true
		it.fail = err
		return it.Next()
	}
}

// Error returns any retrieval or parsing error occurred during filtering.
func (it *SPRegistryUpgradedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SPRegistryUpgradedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SPRegistryUpgraded represents a Upgraded event raised by the SPRegistry contract.
type SPRegistryUpgraded struct {
	Implementation common.Address
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterUpgraded is a free log retrieval operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_SPRegistry *SPRegistryFilterer) FilterUpgraded(opts *bind.FilterOpts, implementation []common.Address) (*SPRegistryUpgradedIterator, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _SPRegistry.contract.FilterLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return &SPRegistryUpgradedIterator{contract: _SPRegistry.contract, event: "Upgraded", logs: logs, sub: sub}, nil
}

// WatchUpgraded is a free log subscription operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_SPRegistry *SPRegistryFilterer) WatchUpgraded(opts *bind.WatchOpts, sink chan<- *SPRegistryUpgraded, implementation []common.Address) (event.Subscription, error) {

	var implementationRule []interface{}
	for _, implementationItem := range implementation {
		implementationRule = append(implementationRule, implementationItem)
	}

	logs, sub, err := _SPRegistry.contract.WatchLogs(opts, "Upgraded", implementationRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SPRegistryUpgraded)
				if err := _SPRegistry.contract.UnpackLog(event, "Upgraded", log); err != nil {
					return err
				}
				event.Raw = log

				select {
				case sink <- event:
				case err := <-sub.Err():
					return err
				case <-quit:
					return nil
				}
			case err := <-sub.Err():
				return err
			case <-quit:
				return nil
			}
		}
	}), nil
}

// ParseUpgraded is a log parse operation binding the contract event 0xbc7cd75a20ee27fd9adebab32041f755214dbc6bffa90cc0225b39da2e5c2d3b.
//
// Solidity: event Upgraded(address indexed implementation)
func (_SPRegistry *SPRegistryFilterer) ParseUpgraded(log types.Log) (*SPRegistryUpgraded, error) {
	event := new(SPRegistryUpgraded)
	if err := _SPRegistry.contract.UnpackLog(event, "Upgraded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
