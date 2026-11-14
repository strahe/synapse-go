// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package fwss

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

// CidsCid is an auto generated low-level Go binding around an user-defined struct.
type CidsCid struct {
	Data []byte
}

// DataSetInfoView is an auto generated low-level Go binding around an user-defined struct.
type DataSetInfoView struct {
	PdpRailId               *big.Int
	CacheMissRailId         *big.Int
	CdnRailId               *big.Int
	Payer                   common.Address
	Payee                   common.Address
	ServiceProvider         common.Address
	CommissionBps           *big.Int
	ClientDataSetId         *big.Int
	PdpEndEpoch             *big.Int
	ProviderId              *big.Int
	PendingOneTimePayments  *big.Int
	LifecycleReserveBalance *big.Int
	DataSetId               *big.Int
}

// FWSSDataSetModuleServicePricing is an auto generated low-level Go binding around an user-defined struct.
type FWSSDataSetModuleServicePricing struct {
	PricePerTiBPerMonthNoCDN   *big.Int
	PricePerTiBCdnEgress       *big.Int
	PricePerTiBCacheMissEgress *big.Int
	TokenAddress               common.Address
	EpochsPerMonth             *big.Int
	DatasetFeePerMonth         *big.Int
}

// FWSSStoragePlannedUpgrade is an auto generated low-level Go binding around an user-defined struct.
type FWSSStoragePlannedUpgrade struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}

// IValidatorValidationResult is an auto generated low-level Go binding around an user-defined struct.
type IValidatorValidationResult struct {
	ModifiedAmount *big.Int
	SettleUpto     *big.Int
	Note           string
}

// PriceList is an auto generated low-level Go binding around an user-defined struct.
type PriceList struct {
	Token   common.Address
	Rates   PriceListRates
	Fees    PriceListFees
	Lockups PriceListLockups
}

// PriceListFees is an auto generated low-level Go binding around an user-defined struct.
type PriceListFees struct {
	CreateDataSetFee         *big.Int
	AddPiecesBaseFee         *big.Int
	AddPiecesPerPieceFee     *big.Int
	SchedulePieceRemovalsFee *big.Int
	TerminateFee             *big.Int
}

// PriceListLockups is an auto generated low-level Go binding around an user-defined struct.
type PriceListLockups struct {
	LifecycleReserveTarget *big.Int
	ReplenishThreshold     *big.Int
	DefaultLockupPeriod    *big.Int
	CdnLockupAmount        *big.Int
	CacheMissLockupAmount  *big.Int
	CdnLockupPeriod        *big.Int
}

// PriceListRates is an auto generated low-level Go binding around an user-defined struct.
type PriceListRates struct {
	StoragePerTibPerMonth *big.Int
	DatasetFeePerMonth    *big.Int
	CdnEgressPerTib       *big.Int
	CacheMissEgressPerTib *big.Int
}

// FWSSMetaData contains all meta data concerning the FWSS contract.
var FWSSMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"extsload\",\"inputs\":[{\"name\":\"slot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"extsloadStruct\",\"inputs\":[{\"name\":\"slot\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"size\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"piecesAdded\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"firstAdded\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceData\",\"type\":\"tuple[]\",\"internalType\":\"structCids.Cid[]\",\"components\":[{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"name\":\"extraData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"piecesScheduledRemove\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"},{\"name\":\"extraData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"sessionKeyRegistry\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractSessionKeyRegistry\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"setDataSetAuthorizer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"authorizer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"terminateService\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"terminateService\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"extraData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"topUpLifecycleReserve\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"amount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"DataSetAuthorizerSet\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"authorizer\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PieceAdded\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"pieceId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"pieceCid\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structCids.Cid\",\"components\":[{\"name\":\"data\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"name\":\"keys\",\"type\":\"string[]\",\"indexed\":false,\"internalType\":\"string[]\"},{\"name\":\"values\",\"type\":\"string[]\",\"indexed\":false,\"internalType\":\"string[]\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"RailRateUpdated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"railId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"newRate\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ServiceTerminated\",\"inputs\":[{\"name\":\"approver\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AuthorizerReentrancy\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CallerNotPayer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayerOrPayee\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expectedPayee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotServiceProvider\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedServiceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ClientDataSetAlreadyRegistered\",\"inputs\":[{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotRegistered\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentBeyondEndEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DuplicateMetadataKey\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"ExtraDataRequired\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExtraDataTooLarge\",\"inputs\":[{\"name\":\"actualSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowedSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidDataSetAuthorizer\",\"inputs\":[{\"name\":\"authorizer\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InvalidDataSetId\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InvalidSignatureLength\",\"inputs\":[{\"name\":\"expectedLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actualLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataArrayCountMismatch\",\"inputs\":[{\"name\":\"metadataArrayCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyAndValueLengthMismatch\",\"inputs\":[{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"valuesLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataValueExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NoPDPPaymentRail\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"OnlyDataSetPayer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyPDPVerifierAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"TooManyMetadataKeys\",\"inputs\":[{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"Unauthorized\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operation\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"digest\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"UnsupportedSignatureV\",\"inputs\":[{\"name\":\"v\",\"type\":\"uint8\",\"internalType\":\"uint8\"}]},{\"type\":\"error\",\"name\":\"ZeroAddress\",\"inputs\":[{\"name\":\"field\",\"type\":\"uint8\",\"internalType\":\"enumErrors.AddressField\"}]},{\"type\":\"function\",\"name\":\"paymentsContractAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"pdpVerifierAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"usdfcTokenAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractIERC20Metadata\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"dataSetCreated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"extraData\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"dataSetDeleted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"eip712Domain\",\"inputs\":[],\"outputs\":[{\"name\":\"fields\",\"type\":\"bytes1\",\"internalType\":\"bytes1\"},{\"name\":\"name\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"version\",\"type\":\"string\",\"internalType\":\"string\"},{\"name\":\"chainId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"verifyingContract\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"salt\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"extensions\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"filBeamBeneficiaryAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getEffectiveRates\",\"inputs\":[],\"outputs\":[{\"name\":\"serviceFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"spPayment\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"getServicePrice\",\"inputs\":[],\"outputs\":[{\"name\":\"pricing\",\"type\":\"tuple\",\"internalType\":\"structFWSSDataSetModule.ServicePricing\",\"components\":[{\"name\":\"pricePerTiBPerMonthNoCDN\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pricePerTiBCdnEgress\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pricePerTiBCacheMissEgress\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"tokenAddress\",\"type\":\"address\",\"internalType\":\"contractIERC20\"},{\"name\":\"epochsPerMonth\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"datasetFeePerMonth\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"serviceProviderRegistry\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractServiceProviderRegistry\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"storageProviderChanged\",\"inputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"CDNPaymentRailsToppedUp\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"cdnAmountAdded\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"totalCdnLockup\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cacheMissAmountAdded\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"totalCacheMissLockup\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"CDNServiceTerminated\",\"inputs\":[{\"name\":\"caller\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DataSetAbandoned\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DataSetCreated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"payer\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"metadataKeys\",\"type\":\"string[]\",\"indexed\":false,\"internalType\":\"string[]\"},{\"name\":\"metadataValues\",\"type\":\"string[]\",\"indexed\":false,\"internalType\":\"string[]\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"DataSetServiceProviderChanged\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"oldServiceProvider\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newServiceProvider\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"EIP712DomainChanged\",\"inputs\":[],\"anonymous\":false},{\"type\":\"event\",\"name\":\"Initialized\",\"inputs\":[{\"name\":\"version\",\"type\":\"uint64\",\"indexed\":false,\"internalType\":\"uint64\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"DataSetNotAbandoned\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"requiredEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lockupAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"lockupUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"lockupRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupFunds\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"required\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"available\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientMaxLockupPeriod\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"maxLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"requiredLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientRateAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"rateAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"rateUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"rateRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidInitialization\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NotInitializing\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"OperatorNotApproved\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"PaymentRailsNotFinalized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotRegistered\",\"inputs\":[{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"RailNotFullySettled\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"settledUpTo\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"endEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"StorageProviderChangesNotSupported\",\"inputs\":[]},{\"type\":\"function\",\"name\":\"settleFilBeamPaymentRails\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"terminateCDNService\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"topUpCDNPaymentRails\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnAmountToAdd\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissAmountToAdd\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferFilBeamController\",\"inputs\":[{\"name\":\"newController\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"FilBeamControllerChanged\",\"inputs\":[{\"name\":\"oldController\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"newController\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"CDNPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CacheMissPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"FilBeamServiceNotConfigured\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidTopUpAmount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"OnlyFilBeamControllerAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"function\",\"name\":\"VERSION\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"description\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"homepage\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"name\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"announceMigration\",\"inputs\":[{\"name\":\"migration\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"delayEpochs\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"migrate\",\"inputs\":[{\"name\":\"migration\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"DiamondDelegateCall\",\"inputs\":[{\"name\":\"delegate\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"delegateCalldata\",\"type\":\"bytes\",\"indexed\":false,\"internalType\":\"bytes\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"UpgradeAnnounced\",\"inputs\":[{\"name\":\"plannedUpgrade\",\"type\":\"tuple\",\"indexed\":false,\"internalType\":\"structFWSSStorage.PlannedUpgrade\",\"components\":[{\"name\":\"nextImplementation\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"afterEpoch\",\"type\":\"uint96\",\"internalType\":\"uint96\"}]}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"AddressEmptyCode\",\"inputs\":[{\"name\":\"target\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"DispatcherChanged\",\"inputs\":[{\"name\":\"implementation\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"FailedCall\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"InvalidMigration\",\"inputs\":[{\"name\":\"migration\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"MigrationNotAnnounced\",\"inputs\":[{\"name\":\"migration\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"MigrationNotReady\",\"inputs\":[{\"name\":\"afterEpoch\",\"type\":\"uint96\",\"internalType\":\"uint96\"}]},{\"type\":\"error\",\"name\":\"MissingUpgradeRoute\",\"inputs\":[{\"name\":\"selector\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"Unauthorized\",\"inputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"function\",\"name\":\"addApprovedProvider\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"removeApprovedProvider\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"ProviderApproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"ProviderUnapproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"ProviderAlreadyApproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderIdMismatchAtIndex\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotInApprovedList\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"function\",\"name\":\"configureProvingPeriod\",\"inputs\":[{\"name\":\"_maxProvingPeriod\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"_challengeWindowSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"getProvingPeriodForEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"epoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"nextProvingPeriod\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengeEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"leafCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"bytes\",\"internalType\":\"bytes\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"possessionProven\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengeCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"railTerminated\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"terminator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"endEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"validatePayment\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"proposedAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"fromEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"toEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"result\",\"type\":\"tuple\",\"internalType\":\"structIValidator.ValidationResult\",\"components\":[{\"name\":\"modifiedAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"settleUpto\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"note\",\"type\":\"string\",\"internalType\":\"string\"}]}],\"stateMutability\":\"view\"},{\"type\":\"event\",\"name\":\"FaultRecord\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"periodsFaulted\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"event\",\"name\":\"PDPPaymentTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"indexed\":true,\"internalType\":\"uint256\"},{\"name\":\"endEpoch\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"CallerNotPayments\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ChallengeWindowTooEarly\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"windowStart\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotFoundForRail\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeCount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minExpected\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeWindowSize\",\"inputs\":[{\"name\":\"maxProvingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengeWindowSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidEpochRange\",\"inputs\":[{\"name\":\"fromEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"toEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MaxProvingPeriodZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"NextProvingPeriodAlreadyCalled\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"periodDeadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProofAlreadySubmitted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingNotStarted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingPeriodPassed\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ServiceContractMustTerminateRail\",\"inputs\":[]},{\"type\":\"function\",\"name\":\"viewContractAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"clientDataSets\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"dataSetIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"clientDataSets\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"dataSetIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"clientNonces\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"filBeamControllerAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAllDataSetMetadata\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"keys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"values\",\"type\":\"string[]\",\"internalType\":\"string[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApprovedProviders\",\"inputs\":[{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"providerIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApprovedProvidersLength\",\"inputs\":[],\"outputs\":[{\"name\":\"count\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClientDataSets\",\"inputs\":[{\"name\":\"client\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"infos\",\"type\":\"tuple[]\",\"internalType\":\"structDataSetInfoView[]\",\"components\":[{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"commissionBps\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pendingOneTimePayments\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"lifecycleReserveBalance\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClientDataSets\",\"inputs\":[{\"name\":\"client\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"infos\",\"type\":\"tuple[]\",\"internalType\":\"structDataSetInfoView[]\",\"components\":[{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"commissionBps\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pendingOneTimePayments\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"lifecycleReserveBalance\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClientDataSetsLength\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCurrentPricingRates\",\"inputs\":[],\"outputs\":[{\"name\":\"storagePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"datasetFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"getDataSet\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structDataSetInfoView\",\"components\":[{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"commissionBps\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pendingOneTimePayments\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"lifecycleReserveBalance\",\"type\":\"uint96\",\"internalType\":\"uint96\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataSetAuthorizer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataSetMetadata\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"exists\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"value\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataSetPayerAndRailId\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataSetSizeInBytes\",\"inputs\":[{\"name\":\"leafCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"getDataSetStatus\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"status\",\"type\":\"uint8\",\"internalType\":\"enumDataSetStatus\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getPDPConfig\",\"inputs\":[],\"outputs\":[{\"name\":\"maxProvingPeriod\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"challengeWindowSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengesPerProof\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"initChallengeWindowStart\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getPriceList\",\"inputs\":[],\"outputs\":[{\"name\":\"list\",\"type\":\"tuple\",\"internalType\":\"structPriceList\",\"components\":[{\"name\":\"token\",\"type\":\"address\",\"internalType\":\"contractIERC20\"},{\"name\":\"rates\",\"type\":\"tuple\",\"internalType\":\"structPriceListRates\",\"components\":[{\"name\":\"storagePerTibPerMonth\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"datasetFeePerMonth\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnEgressPerTib\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissEgressPerTib\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"fees\",\"type\":\"tuple\",\"internalType\":\"structPriceListFees\",\"components\":[{\"name\":\"createDataSetFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"addPiecesBaseFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"addPiecesPerPieceFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"schedulePieceRemovalsFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"terminateFee\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"name\":\"lockups\",\"type\":\"tuple\",\"internalType\":\"structPriceListLockups\",\"components\":[{\"name\":\"lifecycleReserveTarget\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"replenishThreshold\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"defaultLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnLockupAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissLockupAmount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"hasBeenProvenRecently\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isProviderApproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"nextPDPChallengeWindowStart\",\"inputs\":[{\"name\":\"setId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"nextUpgrade\",\"inputs\":[],\"outputs\":[{\"name\":\"nextImplementation\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"afterEpoch\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provenPeriods\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"periodId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provenThisPeriod\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provingActivationEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provingDeadline\",\"inputs\":[{\"name\":\"setId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"railToDataSet\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"serviceCommissionBps\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"pure\"},{\"type\":\"error\",\"name\":\"ProvingPeriodNotInitialized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"function\",\"name\":\"implementation\",\"inputs\":[{\"name\":\"selector\",\"type\":\"bytes4\",\"internalType\":\"bytes4\"}],\"outputs\":[{\"name\":\"delegate\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"selectors\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"bytes4[]\",\"internalType\":\"bytes4[]\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"AddressAlreadySet\",\"inputs\":[{\"name\":\"field\",\"type\":\"uint8\",\"internalType\":\"enumErrors.AddressField\"}]},{\"type\":\"error\",\"name\":\"AuthorizerReentrancy\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CDNPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CacheMissPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayerOrPayee\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expectedPayee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayments\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotServiceProvider\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedServiceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ChallengeWindowTooEarly\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"windowStart\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ClientDataSetAlreadyRegistered\",\"inputs\":[{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CommissionExceedsMaximum\",\"inputs\":[{\"name\":\"commissionType\",\"type\":\"uint8\",\"internalType\":\"enumErrors.CommissionType\"},{\"name\":\"max\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotAbandoned\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"requiredEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotFoundForRail\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotRegistered\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentBeyondEndEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DuplicateMetadataKey\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"ExtraDataRequired\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExtraDataTooLarge\",\"inputs\":[{\"name\":\"actualSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowedSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"FilBeamServiceNotConfigured\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientCapabilitiesForProduct\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lockupAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"lockupUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"lockupRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupFunds\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"required\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"available\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientMaxLockupPeriod\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"maxLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"requiredLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientRateAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"rateAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"rateUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"rateRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeCount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minExpected\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeWindowSize\",\"inputs\":[{\"name\":\"maxProvingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengeWindowSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidDataSetAuthorizer\",\"inputs\":[{\"name\":\"authorizer\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InvalidDataSetId\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidEpochRange\",\"inputs\":[{\"name\":\"fromEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"toEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InvalidSignatureLength\",\"inputs\":[{\"name\":\"expectedLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actualLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidTopUpAmount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MaxProvingPeriodZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MetadataArrayCountMismatch\",\"inputs\":[{\"name\":\"metadataArrayCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyAndValueLengthMismatch\",\"inputs\":[{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"valuesLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataValueExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NextProvingPeriodAlreadyCalled\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"periodDeadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NoPDPPaymentRail\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"OldServiceProviderMismatch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyDataSetPayer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyFilBeamControllerAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyPDPVerifierAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlySelf\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OperatorNotApproved\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"PaymentRailsNotFinalized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProofAlreadySubmitted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderAlreadyApproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderIdMismatchAtIndex\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotInApprovedList\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotRegistered\",\"inputs\":[{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ProvingNotStarted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingPeriodNotInitialized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingPeriodPassed\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"RailNotFullySettled\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"settledUpTo\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"endEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ServiceContractMustTerminateRail\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"StorageProviderChangesNotSupported\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooManyMetadataKeys\",\"inputs\":[{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"Unauthorized\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operation\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"digest\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"},{\"name\":\"signature\",\"type\":\"bytes\",\"internalType\":\"bytes\"}]},{\"type\":\"error\",\"name\":\"UnsupportedSignatureV\",\"inputs\":[{\"name\":\"v\",\"type\":\"uint8\",\"internalType\":\"uint8\"}]},{\"type\":\"error\",\"name\":\"ZeroAddress\",\"inputs\":[{\"name\":\"field\",\"type\":\"uint8\",\"internalType\":\"enumErrors.AddressField\"}]}]",
}

// FWSSABI is the input ABI used to generate the binding from.
// Deprecated: Use FWSSMetaData.ABI instead.
var FWSSABI = FWSSMetaData.ABI

// FWSS is an auto generated Go binding around an Ethereum contract.
type FWSS struct {
	FWSSCaller     // Read-only binding to the contract
	FWSSTransactor // Write-only binding to the contract
	FWSSFilterer   // Log filterer for contract events
}

// FWSSCaller is an auto generated read-only Go binding around an Ethereum contract.
type FWSSCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// FWSSTransactor is an auto generated write-only Go binding around an Ethereum contract.
type FWSSTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// FWSSFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type FWSSFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// FWSSSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type FWSSSession struct {
	Contract     *FWSS             // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// FWSSCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type FWSSCallerSession struct {
	Contract *FWSSCaller   // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts // Call options to use throughout this session
}

// FWSSTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type FWSSTransactorSession struct {
	Contract     *FWSSTransactor   // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// FWSSRaw is an auto generated low-level Go binding around an Ethereum contract.
type FWSSRaw struct {
	Contract *FWSS // Generic contract binding to access the raw methods on
}

// FWSSCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type FWSSCallerRaw struct {
	Contract *FWSSCaller // Generic read-only contract binding to access the raw methods on
}

// FWSSTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type FWSSTransactorRaw struct {
	Contract *FWSSTransactor // Generic write-only contract binding to access the raw methods on
}

// NewFWSS creates a new instance of FWSS, bound to a specific deployed contract.
func NewFWSS(address common.Address, backend bind.ContractBackend) (*FWSS, error) {
	contract, err := bindFWSS(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &FWSS{FWSSCaller: FWSSCaller{contract: contract}, FWSSTransactor: FWSSTransactor{contract: contract}, FWSSFilterer: FWSSFilterer{contract: contract}}, nil
}

// NewFWSSCaller creates a new read-only instance of FWSS, bound to a specific deployed contract.
func NewFWSSCaller(address common.Address, caller bind.ContractCaller) (*FWSSCaller, error) {
	contract, err := bindFWSS(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &FWSSCaller{contract: contract}, nil
}

// NewFWSSTransactor creates a new write-only instance of FWSS, bound to a specific deployed contract.
func NewFWSSTransactor(address common.Address, transactor bind.ContractTransactor) (*FWSSTransactor, error) {
	contract, err := bindFWSS(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &FWSSTransactor{contract: contract}, nil
}

// NewFWSSFilterer creates a new log filterer instance of FWSS, bound to a specific deployed contract.
func NewFWSSFilterer(address common.Address, filterer bind.ContractFilterer) (*FWSSFilterer, error) {
	contract, err := bindFWSS(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &FWSSFilterer{contract: contract}, nil
}

// bindFWSS binds a generic wrapper to an already deployed contract.
func bindFWSS(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := FWSSMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_FWSS *FWSSRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _FWSS.Contract.FWSSCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_FWSS *FWSSRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _FWSS.Contract.FWSSTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_FWSS *FWSSRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _FWSS.Contract.FWSSTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_FWSS *FWSSCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _FWSS.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_FWSS *FWSSTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _FWSS.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_FWSS *FWSSTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _FWSS.Contract.contract.Transact(opts, method, params...)
}

// VERSION is a free data retrieval call binding the contract method 0xffa1ad74.
//
// Solidity: function VERSION() view returns(string)
func (_FWSS *FWSSCaller) VERSION(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "VERSION")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// VERSION is a free data retrieval call binding the contract method 0xffa1ad74.
//
// Solidity: function VERSION() view returns(string)
func (_FWSS *FWSSSession) VERSION() (string, error) {
	return _FWSS.Contract.VERSION(&_FWSS.CallOpts)
}

// VERSION is a free data retrieval call binding the contract method 0xffa1ad74.
//
// Solidity: function VERSION() view returns(string)
func (_FWSS *FWSSCallerSession) VERSION() (string, error) {
	return _FWSS.Contract.VERSION(&_FWSS.CallOpts)
}

// ClientDataSets is a free data retrieval call binding the contract method 0x1047717c.
//
// Solidity: function clientDataSets(address payer, uint256 offset, uint256 limit) view returns(uint256[] dataSetIds)
func (_FWSS *FWSSCaller) ClientDataSets(opts *bind.CallOpts, payer common.Address, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "clientDataSets", payer, offset, limit)

	if err != nil {
		return *new([]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)

	return out0, err

}

// ClientDataSets is a free data retrieval call binding the contract method 0x1047717c.
//
// Solidity: function clientDataSets(address payer, uint256 offset, uint256 limit) view returns(uint256[] dataSetIds)
func (_FWSS *FWSSSession) ClientDataSets(payer common.Address, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSS.Contract.ClientDataSets(&_FWSS.CallOpts, payer, offset, limit)
}

// ClientDataSets is a free data retrieval call binding the contract method 0x1047717c.
//
// Solidity: function clientDataSets(address payer, uint256 offset, uint256 limit) view returns(uint256[] dataSetIds)
func (_FWSS *FWSSCallerSession) ClientDataSets(payer common.Address, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSS.Contract.ClientDataSets(&_FWSS.CallOpts, payer, offset, limit)
}

// ClientDataSets0 is a free data retrieval call binding the contract method 0x7dab7c40.
//
// Solidity: function clientDataSets(address payer) view returns(uint256[] dataSetIds)
func (_FWSS *FWSSCaller) ClientDataSets0(opts *bind.CallOpts, payer common.Address) ([]*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "clientDataSets0", payer)

	if err != nil {
		return *new([]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)

	return out0, err

}

// ClientDataSets0 is a free data retrieval call binding the contract method 0x7dab7c40.
//
// Solidity: function clientDataSets(address payer) view returns(uint256[] dataSetIds)
func (_FWSS *FWSSSession) ClientDataSets0(payer common.Address) ([]*big.Int, error) {
	return _FWSS.Contract.ClientDataSets0(&_FWSS.CallOpts, payer)
}

// ClientDataSets0 is a free data retrieval call binding the contract method 0x7dab7c40.
//
// Solidity: function clientDataSets(address payer) view returns(uint256[] dataSetIds)
func (_FWSS *FWSSCallerSession) ClientDataSets0(payer common.Address) ([]*big.Int, error) {
	return _FWSS.Contract.ClientDataSets0(&_FWSS.CallOpts, payer)
}

// ClientNonces is a free data retrieval call binding the contract method 0x35b0e3f4.
//
// Solidity: function clientNonces(address payer, uint256 nonce) view returns(uint256)
func (_FWSS *FWSSCaller) ClientNonces(opts *bind.CallOpts, payer common.Address, nonce *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "clientNonces", payer, nonce)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ClientNonces is a free data retrieval call binding the contract method 0x35b0e3f4.
//
// Solidity: function clientNonces(address payer, uint256 nonce) view returns(uint256)
func (_FWSS *FWSSSession) ClientNonces(payer common.Address, nonce *big.Int) (*big.Int, error) {
	return _FWSS.Contract.ClientNonces(&_FWSS.CallOpts, payer, nonce)
}

// ClientNonces is a free data retrieval call binding the contract method 0x35b0e3f4.
//
// Solidity: function clientNonces(address payer, uint256 nonce) view returns(uint256)
func (_FWSS *FWSSCallerSession) ClientNonces(payer common.Address, nonce *big.Int) (*big.Int, error) {
	return _FWSS.Contract.ClientNonces(&_FWSS.CallOpts, payer, nonce)
}

// Description is a free data retrieval call binding the contract method 0x7284e416.
//
// Solidity: function description() pure returns(string)
func (_FWSS *FWSSCaller) Description(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "description")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Description is a free data retrieval call binding the contract method 0x7284e416.
//
// Solidity: function description() pure returns(string)
func (_FWSS *FWSSSession) Description() (string, error) {
	return _FWSS.Contract.Description(&_FWSS.CallOpts)
}

// Description is a free data retrieval call binding the contract method 0x7284e416.
//
// Solidity: function description() pure returns(string)
func (_FWSS *FWSSCallerSession) Description() (string, error) {
	return _FWSS.Contract.Description(&_FWSS.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_FWSS *FWSSCaller) Eip712Domain(opts *bind.CallOpts) (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "eip712Domain")

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
func (_FWSS *FWSSSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _FWSS.Contract.Eip712Domain(&_FWSS.CallOpts)
}

// Eip712Domain is a free data retrieval call binding the contract method 0x84b0196e.
//
// Solidity: function eip712Domain() view returns(bytes1 fields, string name, string version, uint256 chainId, address verifyingContract, bytes32 salt, uint256[] extensions)
func (_FWSS *FWSSCallerSession) Eip712Domain() (struct {
	Fields            [1]byte
	Name              string
	Version           string
	ChainId           *big.Int
	VerifyingContract common.Address
	Salt              [32]byte
	Extensions        []*big.Int
}, error) {
	return _FWSS.Contract.Eip712Domain(&_FWSS.CallOpts)
}

// Extsload is a free data retrieval call binding the contract method 0x1e2eaeaf.
//
// Solidity: function extsload(bytes32 slot) view returns(bytes32)
func (_FWSS *FWSSCaller) Extsload(opts *bind.CallOpts, slot [32]byte) ([32]byte, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "extsload", slot)

	if err != nil {
		return *new([32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)

	return out0, err

}

// Extsload is a free data retrieval call binding the contract method 0x1e2eaeaf.
//
// Solidity: function extsload(bytes32 slot) view returns(bytes32)
func (_FWSS *FWSSSession) Extsload(slot [32]byte) ([32]byte, error) {
	return _FWSS.Contract.Extsload(&_FWSS.CallOpts, slot)
}

// Extsload is a free data retrieval call binding the contract method 0x1e2eaeaf.
//
// Solidity: function extsload(bytes32 slot) view returns(bytes32)
func (_FWSS *FWSSCallerSession) Extsload(slot [32]byte) ([32]byte, error) {
	return _FWSS.Contract.Extsload(&_FWSS.CallOpts, slot)
}

// ExtsloadStruct is a free data retrieval call binding the contract method 0x5379a435.
//
// Solidity: function extsloadStruct(bytes32 slot, uint256 size) view returns(bytes32[])
func (_FWSS *FWSSCaller) ExtsloadStruct(opts *bind.CallOpts, slot [32]byte, size *big.Int) ([][32]byte, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "extsloadStruct", slot, size)

	if err != nil {
		return *new([][32]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][32]byte)).(*[][32]byte)

	return out0, err

}

// ExtsloadStruct is a free data retrieval call binding the contract method 0x5379a435.
//
// Solidity: function extsloadStruct(bytes32 slot, uint256 size) view returns(bytes32[])
func (_FWSS *FWSSSession) ExtsloadStruct(slot [32]byte, size *big.Int) ([][32]byte, error) {
	return _FWSS.Contract.ExtsloadStruct(&_FWSS.CallOpts, slot, size)
}

// ExtsloadStruct is a free data retrieval call binding the contract method 0x5379a435.
//
// Solidity: function extsloadStruct(bytes32 slot, uint256 size) view returns(bytes32[])
func (_FWSS *FWSSCallerSession) ExtsloadStruct(slot [32]byte, size *big.Int) ([][32]byte, error) {
	return _FWSS.Contract.ExtsloadStruct(&_FWSS.CallOpts, slot, size)
}

// FilBeamBeneficiaryAddress is a free data retrieval call binding the contract method 0xdd6979bf.
//
// Solidity: function filBeamBeneficiaryAddress() view returns(address)
func (_FWSS *FWSSCaller) FilBeamBeneficiaryAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "filBeamBeneficiaryAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// FilBeamBeneficiaryAddress is a free data retrieval call binding the contract method 0xdd6979bf.
//
// Solidity: function filBeamBeneficiaryAddress() view returns(address)
func (_FWSS *FWSSSession) FilBeamBeneficiaryAddress() (common.Address, error) {
	return _FWSS.Contract.FilBeamBeneficiaryAddress(&_FWSS.CallOpts)
}

// FilBeamBeneficiaryAddress is a free data retrieval call binding the contract method 0xdd6979bf.
//
// Solidity: function filBeamBeneficiaryAddress() view returns(address)
func (_FWSS *FWSSCallerSession) FilBeamBeneficiaryAddress() (common.Address, error) {
	return _FWSS.Contract.FilBeamBeneficiaryAddress(&_FWSS.CallOpts)
}

// FilBeamControllerAddress is a free data retrieval call binding the contract method 0xd1147eee.
//
// Solidity: function filBeamControllerAddress() view returns(address)
func (_FWSS *FWSSCaller) FilBeamControllerAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "filBeamControllerAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// FilBeamControllerAddress is a free data retrieval call binding the contract method 0xd1147eee.
//
// Solidity: function filBeamControllerAddress() view returns(address)
func (_FWSS *FWSSSession) FilBeamControllerAddress() (common.Address, error) {
	return _FWSS.Contract.FilBeamControllerAddress(&_FWSS.CallOpts)
}

// FilBeamControllerAddress is a free data retrieval call binding the contract method 0xd1147eee.
//
// Solidity: function filBeamControllerAddress() view returns(address)
func (_FWSS *FWSSCallerSession) FilBeamControllerAddress() (common.Address, error) {
	return _FWSS.Contract.FilBeamControllerAddress(&_FWSS.CallOpts)
}

// GetAllDataSetMetadata is a free data retrieval call binding the contract method 0xf417c13f.
//
// Solidity: function getAllDataSetMetadata(uint256 dataSetId) view returns(string[] keys, string[] values)
func (_FWSS *FWSSCaller) GetAllDataSetMetadata(opts *bind.CallOpts, dataSetId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getAllDataSetMetadata", dataSetId)

	outstruct := new(struct {
		Keys   []string
		Values []string
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Keys = *abi.ConvertType(out[0], new([]string)).(*[]string)
	outstruct.Values = *abi.ConvertType(out[1], new([]string)).(*[]string)

	return *outstruct, err

}

// GetAllDataSetMetadata is a free data retrieval call binding the contract method 0xf417c13f.
//
// Solidity: function getAllDataSetMetadata(uint256 dataSetId) view returns(string[] keys, string[] values)
func (_FWSS *FWSSSession) GetAllDataSetMetadata(dataSetId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	return _FWSS.Contract.GetAllDataSetMetadata(&_FWSS.CallOpts, dataSetId)
}

// GetAllDataSetMetadata is a free data retrieval call binding the contract method 0xf417c13f.
//
// Solidity: function getAllDataSetMetadata(uint256 dataSetId) view returns(string[] keys, string[] values)
func (_FWSS *FWSSCallerSession) GetAllDataSetMetadata(dataSetId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	return _FWSS.Contract.GetAllDataSetMetadata(&_FWSS.CallOpts, dataSetId)
}

// GetApprovedProviders is a free data retrieval call binding the contract method 0x7709a7f7.
//
// Solidity: function getApprovedProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds)
func (_FWSS *FWSSCaller) GetApprovedProviders(opts *bind.CallOpts, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getApprovedProviders", offset, limit)

	if err != nil {
		return *new([]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)

	return out0, err

}

// GetApprovedProviders is a free data retrieval call binding the contract method 0x7709a7f7.
//
// Solidity: function getApprovedProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds)
func (_FWSS *FWSSSession) GetApprovedProviders(offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSS.Contract.GetApprovedProviders(&_FWSS.CallOpts, offset, limit)
}

// GetApprovedProviders is a free data retrieval call binding the contract method 0x7709a7f7.
//
// Solidity: function getApprovedProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds)
func (_FWSS *FWSSCallerSession) GetApprovedProviders(offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSS.Contract.GetApprovedProviders(&_FWSS.CallOpts, offset, limit)
}

// GetApprovedProvidersLength is a free data retrieval call binding the contract method 0x4d745000.
//
// Solidity: function getApprovedProvidersLength() view returns(uint256 count)
func (_FWSS *FWSSCaller) GetApprovedProvidersLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getApprovedProvidersLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetApprovedProvidersLength is a free data retrieval call binding the contract method 0x4d745000.
//
// Solidity: function getApprovedProvidersLength() view returns(uint256 count)
func (_FWSS *FWSSSession) GetApprovedProvidersLength() (*big.Int, error) {
	return _FWSS.Contract.GetApprovedProvidersLength(&_FWSS.CallOpts)
}

// GetApprovedProvidersLength is a free data retrieval call binding the contract method 0x4d745000.
//
// Solidity: function getApprovedProvidersLength() view returns(uint256 count)
func (_FWSS *FWSSCallerSession) GetApprovedProvidersLength() (*big.Int, error) {
	return _FWSS.Contract.GetApprovedProvidersLength(&_FWSS.CallOpts)
}

// GetClientDataSets is a free data retrieval call binding the contract method 0x3208aa1f.
//
// Solidity: function getClientDataSets(address client, uint256 offset, uint256 limit) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256)[] infos)
func (_FWSS *FWSSCaller) GetClientDataSets(opts *bind.CallOpts, client common.Address, offset *big.Int, limit *big.Int) ([]DataSetInfoView, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getClientDataSets", client, offset, limit)

	if err != nil {
		return *new([]DataSetInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataSetInfoView)).(*[]DataSetInfoView)

	return out0, err

}

// GetClientDataSets is a free data retrieval call binding the contract method 0x3208aa1f.
//
// Solidity: function getClientDataSets(address client, uint256 offset, uint256 limit) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256)[] infos)
func (_FWSS *FWSSSession) GetClientDataSets(client common.Address, offset *big.Int, limit *big.Int) ([]DataSetInfoView, error) {
	return _FWSS.Contract.GetClientDataSets(&_FWSS.CallOpts, client, offset, limit)
}

// GetClientDataSets is a free data retrieval call binding the contract method 0x3208aa1f.
//
// Solidity: function getClientDataSets(address client, uint256 offset, uint256 limit) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256)[] infos)
func (_FWSS *FWSSCallerSession) GetClientDataSets(client common.Address, offset *big.Int, limit *big.Int) ([]DataSetInfoView, error) {
	return _FWSS.Contract.GetClientDataSets(&_FWSS.CallOpts, client, offset, limit)
}

// GetClientDataSets0 is a free data retrieval call binding the contract method 0x967c6f21.
//
// Solidity: function getClientDataSets(address client) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256)[] infos)
func (_FWSS *FWSSCaller) GetClientDataSets0(opts *bind.CallOpts, client common.Address) ([]DataSetInfoView, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getClientDataSets0", client)

	if err != nil {
		return *new([]DataSetInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new([]DataSetInfoView)).(*[]DataSetInfoView)

	return out0, err

}

// GetClientDataSets0 is a free data retrieval call binding the contract method 0x967c6f21.
//
// Solidity: function getClientDataSets(address client) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256)[] infos)
func (_FWSS *FWSSSession) GetClientDataSets0(client common.Address) ([]DataSetInfoView, error) {
	return _FWSS.Contract.GetClientDataSets0(&_FWSS.CallOpts, client)
}

// GetClientDataSets0 is a free data retrieval call binding the contract method 0x967c6f21.
//
// Solidity: function getClientDataSets(address client) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256)[] infos)
func (_FWSS *FWSSCallerSession) GetClientDataSets0(client common.Address) ([]DataSetInfoView, error) {
	return _FWSS.Contract.GetClientDataSets0(&_FWSS.CallOpts, client)
}

// GetClientDataSetsLength is a free data retrieval call binding the contract method 0x98a0b04e.
//
// Solidity: function getClientDataSetsLength(address payer) view returns(uint256)
func (_FWSS *FWSSCaller) GetClientDataSetsLength(opts *bind.CallOpts, payer common.Address) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getClientDataSetsLength", payer)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetClientDataSetsLength is a free data retrieval call binding the contract method 0x98a0b04e.
//
// Solidity: function getClientDataSetsLength(address payer) view returns(uint256)
func (_FWSS *FWSSSession) GetClientDataSetsLength(payer common.Address) (*big.Int, error) {
	return _FWSS.Contract.GetClientDataSetsLength(&_FWSS.CallOpts, payer)
}

// GetClientDataSetsLength is a free data retrieval call binding the contract method 0x98a0b04e.
//
// Solidity: function getClientDataSetsLength(address payer) view returns(uint256)
func (_FWSS *FWSSCallerSession) GetClientDataSetsLength(payer common.Address) (*big.Int, error) {
	return _FWSS.Contract.GetClientDataSetsLength(&_FWSS.CallOpts, payer)
}

// GetCurrentPricingRates is a free data retrieval call binding the contract method 0xb5a578fc.
//
// Solidity: function getCurrentPricingRates() pure returns(uint256 storagePrice, uint256 datasetFee)
func (_FWSS *FWSSCaller) GetCurrentPricingRates(opts *bind.CallOpts) (struct {
	StoragePrice *big.Int
	DatasetFee   *big.Int
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getCurrentPricingRates")

	outstruct := new(struct {
		StoragePrice *big.Int
		DatasetFee   *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.StoragePrice = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.DatasetFee = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetCurrentPricingRates is a free data retrieval call binding the contract method 0xb5a578fc.
//
// Solidity: function getCurrentPricingRates() pure returns(uint256 storagePrice, uint256 datasetFee)
func (_FWSS *FWSSSession) GetCurrentPricingRates() (struct {
	StoragePrice *big.Int
	DatasetFee   *big.Int
}, error) {
	return _FWSS.Contract.GetCurrentPricingRates(&_FWSS.CallOpts)
}

// GetCurrentPricingRates is a free data retrieval call binding the contract method 0xb5a578fc.
//
// Solidity: function getCurrentPricingRates() pure returns(uint256 storagePrice, uint256 datasetFee)
func (_FWSS *FWSSCallerSession) GetCurrentPricingRates() (struct {
	StoragePrice *big.Int
	DatasetFee   *big.Int
}, error) {
	return _FWSS.Contract.GetCurrentPricingRates(&_FWSS.CallOpts)
}

// GetDataSet is a free data retrieval call binding the contract method 0xbdaac056.
//
// Solidity: function getDataSet(uint256 dataSetId) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256) info)
func (_FWSS *FWSSCaller) GetDataSet(opts *bind.CallOpts, dataSetId *big.Int) (DataSetInfoView, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getDataSet", dataSetId)

	if err != nil {
		return *new(DataSetInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new(DataSetInfoView)).(*DataSetInfoView)

	return out0, err

}

// GetDataSet is a free data retrieval call binding the contract method 0xbdaac056.
//
// Solidity: function getDataSet(uint256 dataSetId) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256) info)
func (_FWSS *FWSSSession) GetDataSet(dataSetId *big.Int) (DataSetInfoView, error) {
	return _FWSS.Contract.GetDataSet(&_FWSS.CallOpts, dataSetId)
}

// GetDataSet is a free data retrieval call binding the contract method 0xbdaac056.
//
// Solidity: function getDataSet(uint256 dataSetId) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint96,uint96,uint256) info)
func (_FWSS *FWSSCallerSession) GetDataSet(dataSetId *big.Int) (DataSetInfoView, error) {
	return _FWSS.Contract.GetDataSet(&_FWSS.CallOpts, dataSetId)
}

// GetDataSetAuthorizer is a free data retrieval call binding the contract method 0x2316f736.
//
// Solidity: function getDataSetAuthorizer(uint256 dataSetId) view returns(address)
func (_FWSS *FWSSCaller) GetDataSetAuthorizer(opts *bind.CallOpts, dataSetId *big.Int) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getDataSetAuthorizer", dataSetId)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// GetDataSetAuthorizer is a free data retrieval call binding the contract method 0x2316f736.
//
// Solidity: function getDataSetAuthorizer(uint256 dataSetId) view returns(address)
func (_FWSS *FWSSSession) GetDataSetAuthorizer(dataSetId *big.Int) (common.Address, error) {
	return _FWSS.Contract.GetDataSetAuthorizer(&_FWSS.CallOpts, dataSetId)
}

// GetDataSetAuthorizer is a free data retrieval call binding the contract method 0x2316f736.
//
// Solidity: function getDataSetAuthorizer(uint256 dataSetId) view returns(address)
func (_FWSS *FWSSCallerSession) GetDataSetAuthorizer(dataSetId *big.Int) (common.Address, error) {
	return _FWSS.Contract.GetDataSetAuthorizer(&_FWSS.CallOpts, dataSetId)
}

// GetDataSetMetadata is a free data retrieval call binding the contract method 0x4dc17df1.
//
// Solidity: function getDataSetMetadata(uint256 dataSetId, string key) view returns(bool exists, string value)
func (_FWSS *FWSSCaller) GetDataSetMetadata(opts *bind.CallOpts, dataSetId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getDataSetMetadata", dataSetId, key)

	outstruct := new(struct {
		Exists bool
		Value  string
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Exists = *abi.ConvertType(out[0], new(bool)).(*bool)
	outstruct.Value = *abi.ConvertType(out[1], new(string)).(*string)

	return *outstruct, err

}

// GetDataSetMetadata is a free data retrieval call binding the contract method 0x4dc17df1.
//
// Solidity: function getDataSetMetadata(uint256 dataSetId, string key) view returns(bool exists, string value)
func (_FWSS *FWSSSession) GetDataSetMetadata(dataSetId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	return _FWSS.Contract.GetDataSetMetadata(&_FWSS.CallOpts, dataSetId, key)
}

// GetDataSetMetadata is a free data retrieval call binding the contract method 0x4dc17df1.
//
// Solidity: function getDataSetMetadata(uint256 dataSetId, string key) view returns(bool exists, string value)
func (_FWSS *FWSSCallerSession) GetDataSetMetadata(dataSetId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	return _FWSS.Contract.GetDataSetMetadata(&_FWSS.CallOpts, dataSetId, key)
}

// GetDataSetPayerAndRailId is a free data retrieval call binding the contract method 0x85b37e8b.
//
// Solidity: function getDataSetPayerAndRailId(uint256 dataSetId) view returns(address payer, uint256 pdpRailId)
func (_FWSS *FWSSCaller) GetDataSetPayerAndRailId(opts *bind.CallOpts, dataSetId *big.Int) (struct {
	Payer     common.Address
	PdpRailId *big.Int
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getDataSetPayerAndRailId", dataSetId)

	outstruct := new(struct {
		Payer     common.Address
		PdpRailId *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.Payer = *abi.ConvertType(out[0], new(common.Address)).(*common.Address)
	outstruct.PdpRailId = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetDataSetPayerAndRailId is a free data retrieval call binding the contract method 0x85b37e8b.
//
// Solidity: function getDataSetPayerAndRailId(uint256 dataSetId) view returns(address payer, uint256 pdpRailId)
func (_FWSS *FWSSSession) GetDataSetPayerAndRailId(dataSetId *big.Int) (struct {
	Payer     common.Address
	PdpRailId *big.Int
}, error) {
	return _FWSS.Contract.GetDataSetPayerAndRailId(&_FWSS.CallOpts, dataSetId)
}

// GetDataSetPayerAndRailId is a free data retrieval call binding the contract method 0x85b37e8b.
//
// Solidity: function getDataSetPayerAndRailId(uint256 dataSetId) view returns(address payer, uint256 pdpRailId)
func (_FWSS *FWSSCallerSession) GetDataSetPayerAndRailId(dataSetId *big.Int) (struct {
	Payer     common.Address
	PdpRailId *big.Int
}, error) {
	return _FWSS.Contract.GetDataSetPayerAndRailId(&_FWSS.CallOpts, dataSetId)
}

// GetDataSetSizeInBytes is a free data retrieval call binding the contract method 0xfe295953.
//
// Solidity: function getDataSetSizeInBytes(uint256 leafCount) pure returns(uint256)
func (_FWSS *FWSSCaller) GetDataSetSizeInBytes(opts *bind.CallOpts, leafCount *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getDataSetSizeInBytes", leafCount)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDataSetSizeInBytes is a free data retrieval call binding the contract method 0xfe295953.
//
// Solidity: function getDataSetSizeInBytes(uint256 leafCount) pure returns(uint256)
func (_FWSS *FWSSSession) GetDataSetSizeInBytes(leafCount *big.Int) (*big.Int, error) {
	return _FWSS.Contract.GetDataSetSizeInBytes(&_FWSS.CallOpts, leafCount)
}

// GetDataSetSizeInBytes is a free data retrieval call binding the contract method 0xfe295953.
//
// Solidity: function getDataSetSizeInBytes(uint256 leafCount) pure returns(uint256)
func (_FWSS *FWSSCallerSession) GetDataSetSizeInBytes(leafCount *big.Int) (*big.Int, error) {
	return _FWSS.Contract.GetDataSetSizeInBytes(&_FWSS.CallOpts, leafCount)
}

// GetDataSetStatus is a free data retrieval call binding the contract method 0x617285ad.
//
// Solidity: function getDataSetStatus(uint256 dataSetId) view returns(uint8 status)
func (_FWSS *FWSSCaller) GetDataSetStatus(opts *bind.CallOpts, dataSetId *big.Int) (uint8, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getDataSetStatus", dataSetId)

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// GetDataSetStatus is a free data retrieval call binding the contract method 0x617285ad.
//
// Solidity: function getDataSetStatus(uint256 dataSetId) view returns(uint8 status)
func (_FWSS *FWSSSession) GetDataSetStatus(dataSetId *big.Int) (uint8, error) {
	return _FWSS.Contract.GetDataSetStatus(&_FWSS.CallOpts, dataSetId)
}

// GetDataSetStatus is a free data retrieval call binding the contract method 0x617285ad.
//
// Solidity: function getDataSetStatus(uint256 dataSetId) view returns(uint8 status)
func (_FWSS *FWSSCallerSession) GetDataSetStatus(dataSetId *big.Int) (uint8, error) {
	return _FWSS.Contract.GetDataSetStatus(&_FWSS.CallOpts, dataSetId)
}

// GetEffectiveRates is a free data retrieval call binding the contract method 0x93124a79.
//
// Solidity: function getEffectiveRates() pure returns(uint256 serviceFee, uint256 spPayment)
func (_FWSS *FWSSCaller) GetEffectiveRates(opts *bind.CallOpts) (struct {
	ServiceFee *big.Int
	SpPayment  *big.Int
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getEffectiveRates")

	outstruct := new(struct {
		ServiceFee *big.Int
		SpPayment  *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.ServiceFee = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.SpPayment = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetEffectiveRates is a free data retrieval call binding the contract method 0x93124a79.
//
// Solidity: function getEffectiveRates() pure returns(uint256 serviceFee, uint256 spPayment)
func (_FWSS *FWSSSession) GetEffectiveRates() (struct {
	ServiceFee *big.Int
	SpPayment  *big.Int
}, error) {
	return _FWSS.Contract.GetEffectiveRates(&_FWSS.CallOpts)
}

// GetEffectiveRates is a free data retrieval call binding the contract method 0x93124a79.
//
// Solidity: function getEffectiveRates() pure returns(uint256 serviceFee, uint256 spPayment)
func (_FWSS *FWSSCallerSession) GetEffectiveRates() (struct {
	ServiceFee *big.Int
	SpPayment  *big.Int
}, error) {
	return _FWSS.Contract.GetEffectiveRates(&_FWSS.CallOpts)
}

// GetPDPConfig is a free data retrieval call binding the contract method 0xea0f9354.
//
// Solidity: function getPDPConfig() view returns(uint64 maxProvingPeriod, uint256 challengeWindowSize, uint256 challengesPerProof, uint256 initChallengeWindowStart)
func (_FWSS *FWSSCaller) GetPDPConfig(opts *bind.CallOpts) (struct {
	MaxProvingPeriod         uint64
	ChallengeWindowSize      *big.Int
	ChallengesPerProof       *big.Int
	InitChallengeWindowStart *big.Int
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getPDPConfig")

	outstruct := new(struct {
		MaxProvingPeriod         uint64
		ChallengeWindowSize      *big.Int
		ChallengesPerProof       *big.Int
		InitChallengeWindowStart *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.MaxProvingPeriod = *abi.ConvertType(out[0], new(uint64)).(*uint64)
	outstruct.ChallengeWindowSize = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)
	outstruct.ChallengesPerProof = *abi.ConvertType(out[2], new(*big.Int)).(**big.Int)
	outstruct.InitChallengeWindowStart = *abi.ConvertType(out[3], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetPDPConfig is a free data retrieval call binding the contract method 0xea0f9354.
//
// Solidity: function getPDPConfig() view returns(uint64 maxProvingPeriod, uint256 challengeWindowSize, uint256 challengesPerProof, uint256 initChallengeWindowStart)
func (_FWSS *FWSSSession) GetPDPConfig() (struct {
	MaxProvingPeriod         uint64
	ChallengeWindowSize      *big.Int
	ChallengesPerProof       *big.Int
	InitChallengeWindowStart *big.Int
}, error) {
	return _FWSS.Contract.GetPDPConfig(&_FWSS.CallOpts)
}

// GetPDPConfig is a free data retrieval call binding the contract method 0xea0f9354.
//
// Solidity: function getPDPConfig() view returns(uint64 maxProvingPeriod, uint256 challengeWindowSize, uint256 challengesPerProof, uint256 initChallengeWindowStart)
func (_FWSS *FWSSCallerSession) GetPDPConfig() (struct {
	MaxProvingPeriod         uint64
	ChallengeWindowSize      *big.Int
	ChallengesPerProof       *big.Int
	InitChallengeWindowStart *big.Int
}, error) {
	return _FWSS.Contract.GetPDPConfig(&_FWSS.CallOpts)
}

// GetPriceList is a free data retrieval call binding the contract method 0xb7029144.
//
// Solidity: function getPriceList() view returns((address,(uint256,uint256,uint256,uint256),(uint256,uint256,uint256,uint256,uint256),(uint256,uint256,uint256,uint256,uint256,uint256)) list)
func (_FWSS *FWSSCaller) GetPriceList(opts *bind.CallOpts) (PriceList, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getPriceList")

	if err != nil {
		return *new(PriceList), err
	}

	out0 := *abi.ConvertType(out[0], new(PriceList)).(*PriceList)

	return out0, err

}

// GetPriceList is a free data retrieval call binding the contract method 0xb7029144.
//
// Solidity: function getPriceList() view returns((address,(uint256,uint256,uint256,uint256),(uint256,uint256,uint256,uint256,uint256),(uint256,uint256,uint256,uint256,uint256,uint256)) list)
func (_FWSS *FWSSSession) GetPriceList() (PriceList, error) {
	return _FWSS.Contract.GetPriceList(&_FWSS.CallOpts)
}

// GetPriceList is a free data retrieval call binding the contract method 0xb7029144.
//
// Solidity: function getPriceList() view returns((address,(uint256,uint256,uint256,uint256),(uint256,uint256,uint256,uint256,uint256),(uint256,uint256,uint256,uint256,uint256,uint256)) list)
func (_FWSS *FWSSCallerSession) GetPriceList() (PriceList, error) {
	return _FWSS.Contract.GetPriceList(&_FWSS.CallOpts)
}

// GetProvingPeriodForEpoch is a free data retrieval call binding the contract method 0x4a1fd7a3.
//
// Solidity: function getProvingPeriodForEpoch(uint256 dataSetId, uint256 epoch) view returns(uint256)
func (_FWSS *FWSSCaller) GetProvingPeriodForEpoch(opts *bind.CallOpts, dataSetId *big.Int, epoch *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getProvingPeriodForEpoch", dataSetId, epoch)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetProvingPeriodForEpoch is a free data retrieval call binding the contract method 0x4a1fd7a3.
//
// Solidity: function getProvingPeriodForEpoch(uint256 dataSetId, uint256 epoch) view returns(uint256)
func (_FWSS *FWSSSession) GetProvingPeriodForEpoch(dataSetId *big.Int, epoch *big.Int) (*big.Int, error) {
	return _FWSS.Contract.GetProvingPeriodForEpoch(&_FWSS.CallOpts, dataSetId, epoch)
}

// GetProvingPeriodForEpoch is a free data retrieval call binding the contract method 0x4a1fd7a3.
//
// Solidity: function getProvingPeriodForEpoch(uint256 dataSetId, uint256 epoch) view returns(uint256)
func (_FWSS *FWSSCallerSession) GetProvingPeriodForEpoch(dataSetId *big.Int, epoch *big.Int) (*big.Int, error) {
	return _FWSS.Contract.GetProvingPeriodForEpoch(&_FWSS.CallOpts, dataSetId, epoch)
}

// GetServicePrice is a free data retrieval call binding the contract method 0x5482bdf9.
//
// Solidity: function getServicePrice() view returns((uint256,uint256,uint256,address,uint256,uint256) pricing)
func (_FWSS *FWSSCaller) GetServicePrice(opts *bind.CallOpts) (FWSSDataSetModuleServicePricing, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "getServicePrice")

	if err != nil {
		return *new(FWSSDataSetModuleServicePricing), err
	}

	out0 := *abi.ConvertType(out[0], new(FWSSDataSetModuleServicePricing)).(*FWSSDataSetModuleServicePricing)

	return out0, err

}

// GetServicePrice is a free data retrieval call binding the contract method 0x5482bdf9.
//
// Solidity: function getServicePrice() view returns((uint256,uint256,uint256,address,uint256,uint256) pricing)
func (_FWSS *FWSSSession) GetServicePrice() (FWSSDataSetModuleServicePricing, error) {
	return _FWSS.Contract.GetServicePrice(&_FWSS.CallOpts)
}

// GetServicePrice is a free data retrieval call binding the contract method 0x5482bdf9.
//
// Solidity: function getServicePrice() view returns((uint256,uint256,uint256,address,uint256,uint256) pricing)
func (_FWSS *FWSSCallerSession) GetServicePrice() (FWSSDataSetModuleServicePricing, error) {
	return _FWSS.Contract.GetServicePrice(&_FWSS.CallOpts)
}

// HasBeenProvenRecently is a free data retrieval call binding the contract method 0x48086444.
//
// Solidity: function hasBeenProvenRecently(uint256 dataSetId) view returns(bool)
func (_FWSS *FWSSCaller) HasBeenProvenRecently(opts *bind.CallOpts, dataSetId *big.Int) (bool, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "hasBeenProvenRecently", dataSetId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// HasBeenProvenRecently is a free data retrieval call binding the contract method 0x48086444.
//
// Solidity: function hasBeenProvenRecently(uint256 dataSetId) view returns(bool)
func (_FWSS *FWSSSession) HasBeenProvenRecently(dataSetId *big.Int) (bool, error) {
	return _FWSS.Contract.HasBeenProvenRecently(&_FWSS.CallOpts, dataSetId)
}

// HasBeenProvenRecently is a free data retrieval call binding the contract method 0x48086444.
//
// Solidity: function hasBeenProvenRecently(uint256 dataSetId) view returns(bool)
func (_FWSS *FWSSCallerSession) HasBeenProvenRecently(dataSetId *big.Int) (bool, error) {
	return _FWSS.Contract.HasBeenProvenRecently(&_FWSS.CallOpts, dataSetId)
}

// Homepage is a free data retrieval call binding the contract method 0x686c4d3c.
//
// Solidity: function homepage() pure returns(string)
func (_FWSS *FWSSCaller) Homepage(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "homepage")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Homepage is a free data retrieval call binding the contract method 0x686c4d3c.
//
// Solidity: function homepage() pure returns(string)
func (_FWSS *FWSSSession) Homepage() (string, error) {
	return _FWSS.Contract.Homepage(&_FWSS.CallOpts)
}

// Homepage is a free data retrieval call binding the contract method 0x686c4d3c.
//
// Solidity: function homepage() pure returns(string)
func (_FWSS *FWSSCallerSession) Homepage() (string, error) {
	return _FWSS.Contract.Homepage(&_FWSS.CallOpts)
}

// Implementation is a free data retrieval call binding the contract method 0x0d741577.
//
// Solidity: function implementation(bytes4 selector) view returns(address delegate)
func (_FWSS *FWSSCaller) Implementation(opts *bind.CallOpts, selector [4]byte) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "implementation", selector)

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Implementation is a free data retrieval call binding the contract method 0x0d741577.
//
// Solidity: function implementation(bytes4 selector) view returns(address delegate)
func (_FWSS *FWSSSession) Implementation(selector [4]byte) (common.Address, error) {
	return _FWSS.Contract.Implementation(&_FWSS.CallOpts, selector)
}

// Implementation is a free data retrieval call binding the contract method 0x0d741577.
//
// Solidity: function implementation(bytes4 selector) view returns(address delegate)
func (_FWSS *FWSSCallerSession) Implementation(selector [4]byte) (common.Address, error) {
	return _FWSS.Contract.Implementation(&_FWSS.CallOpts, selector)
}

// IsProviderApproved is a free data retrieval call binding the contract method 0xb6133b7a.
//
// Solidity: function isProviderApproved(uint256 providerId) view returns(bool)
func (_FWSS *FWSSCaller) IsProviderApproved(opts *bind.CallOpts, providerId *big.Int) (bool, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "isProviderApproved", providerId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsProviderApproved is a free data retrieval call binding the contract method 0xb6133b7a.
//
// Solidity: function isProviderApproved(uint256 providerId) view returns(bool)
func (_FWSS *FWSSSession) IsProviderApproved(providerId *big.Int) (bool, error) {
	return _FWSS.Contract.IsProviderApproved(&_FWSS.CallOpts, providerId)
}

// IsProviderApproved is a free data retrieval call binding the contract method 0xb6133b7a.
//
// Solidity: function isProviderApproved(uint256 providerId) view returns(bool)
func (_FWSS *FWSSCallerSession) IsProviderApproved(providerId *big.Int) (bool, error) {
	return _FWSS.Contract.IsProviderApproved(&_FWSS.CallOpts, providerId)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() pure returns(string)
func (_FWSS *FWSSCaller) Name(opts *bind.CallOpts) (string, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "name")

	if err != nil {
		return *new(string), err
	}

	out0 := *abi.ConvertType(out[0], new(string)).(*string)

	return out0, err

}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() pure returns(string)
func (_FWSS *FWSSSession) Name() (string, error) {
	return _FWSS.Contract.Name(&_FWSS.CallOpts)
}

// Name is a free data retrieval call binding the contract method 0x06fdde03.
//
// Solidity: function name() pure returns(string)
func (_FWSS *FWSSCallerSession) Name() (string, error) {
	return _FWSS.Contract.Name(&_FWSS.CallOpts)
}

// NextPDPChallengeWindowStart is a free data retrieval call binding the contract method 0x11d41294.
//
// Solidity: function nextPDPChallengeWindowStart(uint256 setId) view returns(uint256)
func (_FWSS *FWSSCaller) NextPDPChallengeWindowStart(opts *bind.CallOpts, setId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "nextPDPChallengeWindowStart", setId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NextPDPChallengeWindowStart is a free data retrieval call binding the contract method 0x11d41294.
//
// Solidity: function nextPDPChallengeWindowStart(uint256 setId) view returns(uint256)
func (_FWSS *FWSSSession) NextPDPChallengeWindowStart(setId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.NextPDPChallengeWindowStart(&_FWSS.CallOpts, setId)
}

// NextPDPChallengeWindowStart is a free data retrieval call binding the contract method 0x11d41294.
//
// Solidity: function nextPDPChallengeWindowStart(uint256 setId) view returns(uint256)
func (_FWSS *FWSSCallerSession) NextPDPChallengeWindowStart(setId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.NextPDPChallengeWindowStart(&_FWSS.CallOpts, setId)
}

// NextUpgrade is a free data retrieval call binding the contract method 0x315e49ea.
//
// Solidity: function nextUpgrade() view returns(address nextImplementation, uint96 afterEpoch)
func (_FWSS *FWSSCaller) NextUpgrade(opts *bind.CallOpts) (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "nextUpgrade")

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
func (_FWSS *FWSSSession) NextUpgrade() (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	return _FWSS.Contract.NextUpgrade(&_FWSS.CallOpts)
}

// NextUpgrade is a free data retrieval call binding the contract method 0x315e49ea.
//
// Solidity: function nextUpgrade() view returns(address nextImplementation, uint96 afterEpoch)
func (_FWSS *FWSSCallerSession) NextUpgrade() (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	return _FWSS.Contract.NextUpgrade(&_FWSS.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_FWSS *FWSSCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_FWSS *FWSSSession) Owner() (common.Address, error) {
	return _FWSS.Contract.Owner(&_FWSS.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_FWSS *FWSSCallerSession) Owner() (common.Address, error) {
	return _FWSS.Contract.Owner(&_FWSS.CallOpts)
}

// PaymentsContractAddress is a free data retrieval call binding the contract method 0xbc471469.
//
// Solidity: function paymentsContractAddress() view returns(address)
func (_FWSS *FWSSCaller) PaymentsContractAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "paymentsContractAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PaymentsContractAddress is a free data retrieval call binding the contract method 0xbc471469.
//
// Solidity: function paymentsContractAddress() view returns(address)
func (_FWSS *FWSSSession) PaymentsContractAddress() (common.Address, error) {
	return _FWSS.Contract.PaymentsContractAddress(&_FWSS.CallOpts)
}

// PaymentsContractAddress is a free data retrieval call binding the contract method 0xbc471469.
//
// Solidity: function paymentsContractAddress() view returns(address)
func (_FWSS *FWSSCallerSession) PaymentsContractAddress() (common.Address, error) {
	return _FWSS.Contract.PaymentsContractAddress(&_FWSS.CallOpts)
}

// PdpVerifierAddress is a free data retrieval call binding the contract method 0xde4b6b71.
//
// Solidity: function pdpVerifierAddress() view returns(address)
func (_FWSS *FWSSCaller) PdpVerifierAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "pdpVerifierAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// PdpVerifierAddress is a free data retrieval call binding the contract method 0xde4b6b71.
//
// Solidity: function pdpVerifierAddress() view returns(address)
func (_FWSS *FWSSSession) PdpVerifierAddress() (common.Address, error) {
	return _FWSS.Contract.PdpVerifierAddress(&_FWSS.CallOpts)
}

// PdpVerifierAddress is a free data retrieval call binding the contract method 0xde4b6b71.
//
// Solidity: function pdpVerifierAddress() view returns(address)
func (_FWSS *FWSSCallerSession) PdpVerifierAddress() (common.Address, error) {
	return _FWSS.Contract.PdpVerifierAddress(&_FWSS.CallOpts)
}

// ProvenPeriods is a free data retrieval call binding the contract method 0x698762cb.
//
// Solidity: function provenPeriods(uint256 dataSetId, uint256 periodId) view returns(bool)
func (_FWSS *FWSSCaller) ProvenPeriods(opts *bind.CallOpts, dataSetId *big.Int, periodId *big.Int) (bool, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "provenPeriods", dataSetId, periodId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ProvenPeriods is a free data retrieval call binding the contract method 0x698762cb.
//
// Solidity: function provenPeriods(uint256 dataSetId, uint256 periodId) view returns(bool)
func (_FWSS *FWSSSession) ProvenPeriods(dataSetId *big.Int, periodId *big.Int) (bool, error) {
	return _FWSS.Contract.ProvenPeriods(&_FWSS.CallOpts, dataSetId, periodId)
}

// ProvenPeriods is a free data retrieval call binding the contract method 0x698762cb.
//
// Solidity: function provenPeriods(uint256 dataSetId, uint256 periodId) view returns(bool)
func (_FWSS *FWSSCallerSession) ProvenPeriods(dataSetId *big.Int, periodId *big.Int) (bool, error) {
	return _FWSS.Contract.ProvenPeriods(&_FWSS.CallOpts, dataSetId, periodId)
}

// ProvenThisPeriod is a free data retrieval call binding the contract method 0x7598a1cd.
//
// Solidity: function provenThisPeriod(uint256 dataSetId) view returns(bool)
func (_FWSS *FWSSCaller) ProvenThisPeriod(opts *bind.CallOpts, dataSetId *big.Int) (bool, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "provenThisPeriod", dataSetId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ProvenThisPeriod is a free data retrieval call binding the contract method 0x7598a1cd.
//
// Solidity: function provenThisPeriod(uint256 dataSetId) view returns(bool)
func (_FWSS *FWSSSession) ProvenThisPeriod(dataSetId *big.Int) (bool, error) {
	return _FWSS.Contract.ProvenThisPeriod(&_FWSS.CallOpts, dataSetId)
}

// ProvenThisPeriod is a free data retrieval call binding the contract method 0x7598a1cd.
//
// Solidity: function provenThisPeriod(uint256 dataSetId) view returns(bool)
func (_FWSS *FWSSCallerSession) ProvenThisPeriod(dataSetId *big.Int) (bool, error) {
	return _FWSS.Contract.ProvenThisPeriod(&_FWSS.CallOpts, dataSetId)
}

// ProvingActivationEpoch is a free data retrieval call binding the contract method 0x725e3216.
//
// Solidity: function provingActivationEpoch(uint256 dataSetId) view returns(uint256)
func (_FWSS *FWSSCaller) ProvingActivationEpoch(opts *bind.CallOpts, dataSetId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "provingActivationEpoch", dataSetId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ProvingActivationEpoch is a free data retrieval call binding the contract method 0x725e3216.
//
// Solidity: function provingActivationEpoch(uint256 dataSetId) view returns(uint256)
func (_FWSS *FWSSSession) ProvingActivationEpoch(dataSetId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.ProvingActivationEpoch(&_FWSS.CallOpts, dataSetId)
}

// ProvingActivationEpoch is a free data retrieval call binding the contract method 0x725e3216.
//
// Solidity: function provingActivationEpoch(uint256 dataSetId) view returns(uint256)
func (_FWSS *FWSSCallerSession) ProvingActivationEpoch(dataSetId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.ProvingActivationEpoch(&_FWSS.CallOpts, dataSetId)
}

// ProvingDeadline is a free data retrieval call binding the contract method 0x149ac5cc.
//
// Solidity: function provingDeadline(uint256 setId) view returns(uint256)
func (_FWSS *FWSSCaller) ProvingDeadline(opts *bind.CallOpts, setId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "provingDeadline", setId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ProvingDeadline is a free data retrieval call binding the contract method 0x149ac5cc.
//
// Solidity: function provingDeadline(uint256 setId) view returns(uint256)
func (_FWSS *FWSSSession) ProvingDeadline(setId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.ProvingDeadline(&_FWSS.CallOpts, setId)
}

// ProvingDeadline is a free data retrieval call binding the contract method 0x149ac5cc.
//
// Solidity: function provingDeadline(uint256 setId) view returns(uint256)
func (_FWSS *FWSSCallerSession) ProvingDeadline(setId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.ProvingDeadline(&_FWSS.CallOpts, setId)
}

// RailToDataSet is a free data retrieval call binding the contract method 0x2ad6e6b5.
//
// Solidity: function railToDataSet(uint256 railId) view returns(uint256)
func (_FWSS *FWSSCaller) RailToDataSet(opts *bind.CallOpts, railId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "railToDataSet", railId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RailToDataSet is a free data retrieval call binding the contract method 0x2ad6e6b5.
//
// Solidity: function railToDataSet(uint256 railId) view returns(uint256)
func (_FWSS *FWSSSession) RailToDataSet(railId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.RailToDataSet(&_FWSS.CallOpts, railId)
}

// RailToDataSet is a free data retrieval call binding the contract method 0x2ad6e6b5.
//
// Solidity: function railToDataSet(uint256 railId) view returns(uint256)
func (_FWSS *FWSSCallerSession) RailToDataSet(railId *big.Int) (*big.Int, error) {
	return _FWSS.Contract.RailToDataSet(&_FWSS.CallOpts, railId)
}

// Selectors is a free data retrieval call binding the contract method 0x6e25b978.
//
// Solidity: function selectors() view returns(bytes4[])
func (_FWSS *FWSSCaller) Selectors(opts *bind.CallOpts) ([][4]byte, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "selectors")

	if err != nil {
		return *new([][4]byte), err
	}

	out0 := *abi.ConvertType(out[0], new([][4]byte)).(*[][4]byte)

	return out0, err

}

// Selectors is a free data retrieval call binding the contract method 0x6e25b978.
//
// Solidity: function selectors() view returns(bytes4[])
func (_FWSS *FWSSSession) Selectors() ([][4]byte, error) {
	return _FWSS.Contract.Selectors(&_FWSS.CallOpts)
}

// Selectors is a free data retrieval call binding the contract method 0x6e25b978.
//
// Solidity: function selectors() view returns(bytes4[])
func (_FWSS *FWSSCallerSession) Selectors() ([][4]byte, error) {
	return _FWSS.Contract.Selectors(&_FWSS.CallOpts)
}

// ServiceCommissionBps is a free data retrieval call binding the contract method 0x2afcc1a4.
//
// Solidity: function serviceCommissionBps() pure returns(uint256)
func (_FWSS *FWSSCaller) ServiceCommissionBps(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "serviceCommissionBps")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ServiceCommissionBps is a free data retrieval call binding the contract method 0x2afcc1a4.
//
// Solidity: function serviceCommissionBps() pure returns(uint256)
func (_FWSS *FWSSSession) ServiceCommissionBps() (*big.Int, error) {
	return _FWSS.Contract.ServiceCommissionBps(&_FWSS.CallOpts)
}

// ServiceCommissionBps is a free data retrieval call binding the contract method 0x2afcc1a4.
//
// Solidity: function serviceCommissionBps() pure returns(uint256)
func (_FWSS *FWSSCallerSession) ServiceCommissionBps() (*big.Int, error) {
	return _FWSS.Contract.ServiceCommissionBps(&_FWSS.CallOpts)
}

// ServiceProviderRegistry is a free data retrieval call binding the contract method 0x05f892ec.
//
// Solidity: function serviceProviderRegistry() view returns(address)
func (_FWSS *FWSSCaller) ServiceProviderRegistry(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "serviceProviderRegistry")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// ServiceProviderRegistry is a free data retrieval call binding the contract method 0x05f892ec.
//
// Solidity: function serviceProviderRegistry() view returns(address)
func (_FWSS *FWSSSession) ServiceProviderRegistry() (common.Address, error) {
	return _FWSS.Contract.ServiceProviderRegistry(&_FWSS.CallOpts)
}

// ServiceProviderRegistry is a free data retrieval call binding the contract method 0x05f892ec.
//
// Solidity: function serviceProviderRegistry() view returns(address)
func (_FWSS *FWSSCallerSession) ServiceProviderRegistry() (common.Address, error) {
	return _FWSS.Contract.ServiceProviderRegistry(&_FWSS.CallOpts)
}

// SessionKeyRegistry is a free data retrieval call binding the contract method 0x9f6aa572.
//
// Solidity: function sessionKeyRegistry() view returns(address)
func (_FWSS *FWSSCaller) SessionKeyRegistry(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "sessionKeyRegistry")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// SessionKeyRegistry is a free data retrieval call binding the contract method 0x9f6aa572.
//
// Solidity: function sessionKeyRegistry() view returns(address)
func (_FWSS *FWSSSession) SessionKeyRegistry() (common.Address, error) {
	return _FWSS.Contract.SessionKeyRegistry(&_FWSS.CallOpts)
}

// SessionKeyRegistry is a free data retrieval call binding the contract method 0x9f6aa572.
//
// Solidity: function sessionKeyRegistry() view returns(address)
func (_FWSS *FWSSCallerSession) SessionKeyRegistry() (common.Address, error) {
	return _FWSS.Contract.SessionKeyRegistry(&_FWSS.CallOpts)
}

// StorageProviderChanged is a free data retrieval call binding the contract method 0x4059b6d7.
//
// Solidity: function storageProviderChanged(uint256 , address , address , bytes ) view returns()
func (_FWSS *FWSSCaller) StorageProviderChanged(opts *bind.CallOpts, arg0 *big.Int, arg1 common.Address, arg2 common.Address, arg3 []byte) error {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "storageProviderChanged", arg0, arg1, arg2, arg3)

	if err != nil {
		return err
	}

	return err

}

// StorageProviderChanged is a free data retrieval call binding the contract method 0x4059b6d7.
//
// Solidity: function storageProviderChanged(uint256 , address , address , bytes ) view returns()
func (_FWSS *FWSSSession) StorageProviderChanged(arg0 *big.Int, arg1 common.Address, arg2 common.Address, arg3 []byte) error {
	return _FWSS.Contract.StorageProviderChanged(&_FWSS.CallOpts, arg0, arg1, arg2, arg3)
}

// StorageProviderChanged is a free data retrieval call binding the contract method 0x4059b6d7.
//
// Solidity: function storageProviderChanged(uint256 , address , address , bytes ) view returns()
func (_FWSS *FWSSCallerSession) StorageProviderChanged(arg0 *big.Int, arg1 common.Address, arg2 common.Address, arg3 []byte) error {
	return _FWSS.Contract.StorageProviderChanged(&_FWSS.CallOpts, arg0, arg1, arg2, arg3)
}

// UsdfcTokenAddress is a free data retrieval call binding the contract method 0xd39b33ab.
//
// Solidity: function usdfcTokenAddress() view returns(address)
func (_FWSS *FWSSCaller) UsdfcTokenAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "usdfcTokenAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// UsdfcTokenAddress is a free data retrieval call binding the contract method 0xd39b33ab.
//
// Solidity: function usdfcTokenAddress() view returns(address)
func (_FWSS *FWSSSession) UsdfcTokenAddress() (common.Address, error) {
	return _FWSS.Contract.UsdfcTokenAddress(&_FWSS.CallOpts)
}

// UsdfcTokenAddress is a free data retrieval call binding the contract method 0xd39b33ab.
//
// Solidity: function usdfcTokenAddress() view returns(address)
func (_FWSS *FWSSCallerSession) UsdfcTokenAddress() (common.Address, error) {
	return _FWSS.Contract.UsdfcTokenAddress(&_FWSS.CallOpts)
}

// ValidatePayment is a free data retrieval call binding the contract method 0x1a7bf46f.
//
// Solidity: function validatePayment(uint256 railId, uint256 proposedAmount, uint256 fromEpoch, uint256 toEpoch, uint256 ) view returns((uint256,uint256,string) result)
func (_FWSS *FWSSCaller) ValidatePayment(opts *bind.CallOpts, railId *big.Int, proposedAmount *big.Int, fromEpoch *big.Int, toEpoch *big.Int, arg4 *big.Int) (IValidatorValidationResult, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "validatePayment", railId, proposedAmount, fromEpoch, toEpoch, arg4)

	if err != nil {
		return *new(IValidatorValidationResult), err
	}

	out0 := *abi.ConvertType(out[0], new(IValidatorValidationResult)).(*IValidatorValidationResult)

	return out0, err

}

// ValidatePayment is a free data retrieval call binding the contract method 0x1a7bf46f.
//
// Solidity: function validatePayment(uint256 railId, uint256 proposedAmount, uint256 fromEpoch, uint256 toEpoch, uint256 ) view returns((uint256,uint256,string) result)
func (_FWSS *FWSSSession) ValidatePayment(railId *big.Int, proposedAmount *big.Int, fromEpoch *big.Int, toEpoch *big.Int, arg4 *big.Int) (IValidatorValidationResult, error) {
	return _FWSS.Contract.ValidatePayment(&_FWSS.CallOpts, railId, proposedAmount, fromEpoch, toEpoch, arg4)
}

// ValidatePayment is a free data retrieval call binding the contract method 0x1a7bf46f.
//
// Solidity: function validatePayment(uint256 railId, uint256 proposedAmount, uint256 fromEpoch, uint256 toEpoch, uint256 ) view returns((uint256,uint256,string) result)
func (_FWSS *FWSSCallerSession) ValidatePayment(railId *big.Int, proposedAmount *big.Int, fromEpoch *big.Int, toEpoch *big.Int, arg4 *big.Int) (IValidatorValidationResult, error) {
	return _FWSS.Contract.ValidatePayment(&_FWSS.CallOpts, railId, proposedAmount, fromEpoch, toEpoch, arg4)
}

// ViewContractAddress is a free data retrieval call binding the contract method 0x7a9ebc15.
//
// Solidity: function viewContractAddress() view returns(address)
func (_FWSS *FWSSCaller) ViewContractAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSS.contract.Call(opts, &out, "viewContractAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// ViewContractAddress is a free data retrieval call binding the contract method 0x7a9ebc15.
//
// Solidity: function viewContractAddress() view returns(address)
func (_FWSS *FWSSSession) ViewContractAddress() (common.Address, error) {
	return _FWSS.Contract.ViewContractAddress(&_FWSS.CallOpts)
}

// ViewContractAddress is a free data retrieval call binding the contract method 0x7a9ebc15.
//
// Solidity: function viewContractAddress() view returns(address)
func (_FWSS *FWSSCallerSession) ViewContractAddress() (common.Address, error) {
	return _FWSS.Contract.ViewContractAddress(&_FWSS.CallOpts)
}

// AddApprovedProvider is a paid mutator transaction binding the contract method 0xa71f9fec.
//
// Solidity: function addApprovedProvider(uint256 providerId) returns()
func (_FWSS *FWSSTransactor) AddApprovedProvider(opts *bind.TransactOpts, providerId *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "addApprovedProvider", providerId)
}

// AddApprovedProvider is a paid mutator transaction binding the contract method 0xa71f9fec.
//
// Solidity: function addApprovedProvider(uint256 providerId) returns()
func (_FWSS *FWSSSession) AddApprovedProvider(providerId *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.AddApprovedProvider(&_FWSS.TransactOpts, providerId)
}

// AddApprovedProvider is a paid mutator transaction binding the contract method 0xa71f9fec.
//
// Solidity: function addApprovedProvider(uint256 providerId) returns()
func (_FWSS *FWSSTransactorSession) AddApprovedProvider(providerId *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.AddApprovedProvider(&_FWSS.TransactOpts, providerId)
}

// AnnounceMigration is a paid mutator transaction binding the contract method 0x0e8ae05f.
//
// Solidity: function announceMigration(address migration, uint96 delayEpochs) returns()
func (_FWSS *FWSSTransactor) AnnounceMigration(opts *bind.TransactOpts, migration common.Address, delayEpochs *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "announceMigration", migration, delayEpochs)
}

// AnnounceMigration is a paid mutator transaction binding the contract method 0x0e8ae05f.
//
// Solidity: function announceMigration(address migration, uint96 delayEpochs) returns()
func (_FWSS *FWSSSession) AnnounceMigration(migration common.Address, delayEpochs *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.AnnounceMigration(&_FWSS.TransactOpts, migration, delayEpochs)
}

// AnnounceMigration is a paid mutator transaction binding the contract method 0x0e8ae05f.
//
// Solidity: function announceMigration(address migration, uint96 delayEpochs) returns()
func (_FWSS *FWSSTransactorSession) AnnounceMigration(migration common.Address, delayEpochs *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.AnnounceMigration(&_FWSS.TransactOpts, migration, delayEpochs)
}

// ConfigureProvingPeriod is a paid mutator transaction binding the contract method 0xcee4f4c7.
//
// Solidity: function configureProvingPeriod(uint64 _maxProvingPeriod, uint256 _challengeWindowSize) returns()
func (_FWSS *FWSSTransactor) ConfigureProvingPeriod(opts *bind.TransactOpts, _maxProvingPeriod uint64, _challengeWindowSize *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "configureProvingPeriod", _maxProvingPeriod, _challengeWindowSize)
}

// ConfigureProvingPeriod is a paid mutator transaction binding the contract method 0xcee4f4c7.
//
// Solidity: function configureProvingPeriod(uint64 _maxProvingPeriod, uint256 _challengeWindowSize) returns()
func (_FWSS *FWSSSession) ConfigureProvingPeriod(_maxProvingPeriod uint64, _challengeWindowSize *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.ConfigureProvingPeriod(&_FWSS.TransactOpts, _maxProvingPeriod, _challengeWindowSize)
}

// ConfigureProvingPeriod is a paid mutator transaction binding the contract method 0xcee4f4c7.
//
// Solidity: function configureProvingPeriod(uint64 _maxProvingPeriod, uint256 _challengeWindowSize) returns()
func (_FWSS *FWSSTransactorSession) ConfigureProvingPeriod(_maxProvingPeriod uint64, _challengeWindowSize *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.ConfigureProvingPeriod(&_FWSS.TransactOpts, _maxProvingPeriod, _challengeWindowSize)
}

// DataSetCreated is a paid mutator transaction binding the contract method 0x101c1eab.
//
// Solidity: function dataSetCreated(uint256 dataSetId, address serviceProvider, bytes extraData) returns()
func (_FWSS *FWSSTransactor) DataSetCreated(opts *bind.TransactOpts, dataSetId *big.Int, serviceProvider common.Address, extraData []byte) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "dataSetCreated", dataSetId, serviceProvider, extraData)
}

// DataSetCreated is a paid mutator transaction binding the contract method 0x101c1eab.
//
// Solidity: function dataSetCreated(uint256 dataSetId, address serviceProvider, bytes extraData) returns()
func (_FWSS *FWSSSession) DataSetCreated(dataSetId *big.Int, serviceProvider common.Address, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.DataSetCreated(&_FWSS.TransactOpts, dataSetId, serviceProvider, extraData)
}

// DataSetCreated is a paid mutator transaction binding the contract method 0x101c1eab.
//
// Solidity: function dataSetCreated(uint256 dataSetId, address serviceProvider, bytes extraData) returns()
func (_FWSS *FWSSTransactorSession) DataSetCreated(dataSetId *big.Int, serviceProvider common.Address, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.DataSetCreated(&_FWSS.TransactOpts, dataSetId, serviceProvider, extraData)
}

// DataSetDeleted is a paid mutator transaction binding the contract method 0x2abd465c.
//
// Solidity: function dataSetDeleted(uint256 dataSetId, uint256 , bytes ) returns()
func (_FWSS *FWSSTransactor) DataSetDeleted(opts *bind.TransactOpts, dataSetId *big.Int, arg1 *big.Int, arg2 []byte) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "dataSetDeleted", dataSetId, arg1, arg2)
}

// DataSetDeleted is a paid mutator transaction binding the contract method 0x2abd465c.
//
// Solidity: function dataSetDeleted(uint256 dataSetId, uint256 , bytes ) returns()
func (_FWSS *FWSSSession) DataSetDeleted(dataSetId *big.Int, arg1 *big.Int, arg2 []byte) (*types.Transaction, error) {
	return _FWSS.Contract.DataSetDeleted(&_FWSS.TransactOpts, dataSetId, arg1, arg2)
}

// DataSetDeleted is a paid mutator transaction binding the contract method 0x2abd465c.
//
// Solidity: function dataSetDeleted(uint256 dataSetId, uint256 , bytes ) returns()
func (_FWSS *FWSSTransactorSession) DataSetDeleted(dataSetId *big.Int, arg1 *big.Int, arg2 []byte) (*types.Transaction, error) {
	return _FWSS.Contract.DataSetDeleted(&_FWSS.TransactOpts, dataSetId, arg1, arg2)
}

// Migrate is a paid mutator transaction binding the contract method 0xce5494bb.
//
// Solidity: function migrate(address migration) returns()
func (_FWSS *FWSSTransactor) Migrate(opts *bind.TransactOpts, migration common.Address) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "migrate", migration)
}

// Migrate is a paid mutator transaction binding the contract method 0xce5494bb.
//
// Solidity: function migrate(address migration) returns()
func (_FWSS *FWSSSession) Migrate(migration common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.Migrate(&_FWSS.TransactOpts, migration)
}

// Migrate is a paid mutator transaction binding the contract method 0xce5494bb.
//
// Solidity: function migrate(address migration) returns()
func (_FWSS *FWSSTransactorSession) Migrate(migration common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.Migrate(&_FWSS.TransactOpts, migration)
}

// NextProvingPeriod is a paid mutator transaction binding the contract method 0xaa27ebcc.
//
// Solidity: function nextProvingPeriod(uint256 dataSetId, uint256 challengeEpoch, uint256 leafCount, bytes ) returns()
func (_FWSS *FWSSTransactor) NextProvingPeriod(opts *bind.TransactOpts, dataSetId *big.Int, challengeEpoch *big.Int, leafCount *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "nextProvingPeriod", dataSetId, challengeEpoch, leafCount, arg3)
}

// NextProvingPeriod is a paid mutator transaction binding the contract method 0xaa27ebcc.
//
// Solidity: function nextProvingPeriod(uint256 dataSetId, uint256 challengeEpoch, uint256 leafCount, bytes ) returns()
func (_FWSS *FWSSSession) NextProvingPeriod(dataSetId *big.Int, challengeEpoch *big.Int, leafCount *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _FWSS.Contract.NextProvingPeriod(&_FWSS.TransactOpts, dataSetId, challengeEpoch, leafCount, arg3)
}

// NextProvingPeriod is a paid mutator transaction binding the contract method 0xaa27ebcc.
//
// Solidity: function nextProvingPeriod(uint256 dataSetId, uint256 challengeEpoch, uint256 leafCount, bytes ) returns()
func (_FWSS *FWSSTransactorSession) NextProvingPeriod(dataSetId *big.Int, challengeEpoch *big.Int, leafCount *big.Int, arg3 []byte) (*types.Transaction, error) {
	return _FWSS.Contract.NextProvingPeriod(&_FWSS.TransactOpts, dataSetId, challengeEpoch, leafCount, arg3)
}

// PiecesAdded is a paid mutator transaction binding the contract method 0xf6814d79.
//
// Solidity: function piecesAdded(uint256 dataSetId, uint256 firstAdded, (bytes)[] pieceData, bytes extraData) returns()
func (_FWSS *FWSSTransactor) PiecesAdded(opts *bind.TransactOpts, dataSetId *big.Int, firstAdded *big.Int, pieceData []CidsCid, extraData []byte) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "piecesAdded", dataSetId, firstAdded, pieceData, extraData)
}

// PiecesAdded is a paid mutator transaction binding the contract method 0xf6814d79.
//
// Solidity: function piecesAdded(uint256 dataSetId, uint256 firstAdded, (bytes)[] pieceData, bytes extraData) returns()
func (_FWSS *FWSSSession) PiecesAdded(dataSetId *big.Int, firstAdded *big.Int, pieceData []CidsCid, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.PiecesAdded(&_FWSS.TransactOpts, dataSetId, firstAdded, pieceData, extraData)
}

// PiecesAdded is a paid mutator transaction binding the contract method 0xf6814d79.
//
// Solidity: function piecesAdded(uint256 dataSetId, uint256 firstAdded, (bytes)[] pieceData, bytes extraData) returns()
func (_FWSS *FWSSTransactorSession) PiecesAdded(dataSetId *big.Int, firstAdded *big.Int, pieceData []CidsCid, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.PiecesAdded(&_FWSS.TransactOpts, dataSetId, firstAdded, pieceData, extraData)
}

// PiecesScheduledRemove is a paid mutator transaction binding the contract method 0xe7954aa7.
//
// Solidity: function piecesScheduledRemove(uint256 dataSetId, uint256[] pieceIds, bytes extraData) returns()
func (_FWSS *FWSSTransactor) PiecesScheduledRemove(opts *bind.TransactOpts, dataSetId *big.Int, pieceIds []*big.Int, extraData []byte) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "piecesScheduledRemove", dataSetId, pieceIds, extraData)
}

// PiecesScheduledRemove is a paid mutator transaction binding the contract method 0xe7954aa7.
//
// Solidity: function piecesScheduledRemove(uint256 dataSetId, uint256[] pieceIds, bytes extraData) returns()
func (_FWSS *FWSSSession) PiecesScheduledRemove(dataSetId *big.Int, pieceIds []*big.Int, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.PiecesScheduledRemove(&_FWSS.TransactOpts, dataSetId, pieceIds, extraData)
}

// PiecesScheduledRemove is a paid mutator transaction binding the contract method 0xe7954aa7.
//
// Solidity: function piecesScheduledRemove(uint256 dataSetId, uint256[] pieceIds, bytes extraData) returns()
func (_FWSS *FWSSTransactorSession) PiecesScheduledRemove(dataSetId *big.Int, pieceIds []*big.Int, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.PiecesScheduledRemove(&_FWSS.TransactOpts, dataSetId, pieceIds, extraData)
}

// PossessionProven is a paid mutator transaction binding the contract method 0x356de02b.
//
// Solidity: function possessionProven(uint256 dataSetId, uint256 , uint256 , uint256 challengeCount) returns()
func (_FWSS *FWSSTransactor) PossessionProven(opts *bind.TransactOpts, dataSetId *big.Int, arg1 *big.Int, arg2 *big.Int, challengeCount *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "possessionProven", dataSetId, arg1, arg2, challengeCount)
}

// PossessionProven is a paid mutator transaction binding the contract method 0x356de02b.
//
// Solidity: function possessionProven(uint256 dataSetId, uint256 , uint256 , uint256 challengeCount) returns()
func (_FWSS *FWSSSession) PossessionProven(dataSetId *big.Int, arg1 *big.Int, arg2 *big.Int, challengeCount *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.PossessionProven(&_FWSS.TransactOpts, dataSetId, arg1, arg2, challengeCount)
}

// PossessionProven is a paid mutator transaction binding the contract method 0x356de02b.
//
// Solidity: function possessionProven(uint256 dataSetId, uint256 , uint256 , uint256 challengeCount) returns()
func (_FWSS *FWSSTransactorSession) PossessionProven(dataSetId *big.Int, arg1 *big.Int, arg2 *big.Int, challengeCount *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.PossessionProven(&_FWSS.TransactOpts, dataSetId, arg1, arg2, challengeCount)
}

// RailTerminated is a paid mutator transaction binding the contract method 0xc5153f70.
//
// Solidity: function railTerminated(uint256 railId, address terminator, uint256 endEpoch) returns()
func (_FWSS *FWSSTransactor) RailTerminated(opts *bind.TransactOpts, railId *big.Int, terminator common.Address, endEpoch *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "railTerminated", railId, terminator, endEpoch)
}

// RailTerminated is a paid mutator transaction binding the contract method 0xc5153f70.
//
// Solidity: function railTerminated(uint256 railId, address terminator, uint256 endEpoch) returns()
func (_FWSS *FWSSSession) RailTerminated(railId *big.Int, terminator common.Address, endEpoch *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.RailTerminated(&_FWSS.TransactOpts, railId, terminator, endEpoch)
}

// RailTerminated is a paid mutator transaction binding the contract method 0xc5153f70.
//
// Solidity: function railTerminated(uint256 railId, address terminator, uint256 endEpoch) returns()
func (_FWSS *FWSSTransactorSession) RailTerminated(railId *big.Int, terminator common.Address, endEpoch *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.RailTerminated(&_FWSS.TransactOpts, railId, terminator, endEpoch)
}

// RemoveApprovedProvider is a paid mutator transaction binding the contract method 0x5840b83d.
//
// Solidity: function removeApprovedProvider(uint256 providerId, uint256 index) returns()
func (_FWSS *FWSSTransactor) RemoveApprovedProvider(opts *bind.TransactOpts, providerId *big.Int, index *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "removeApprovedProvider", providerId, index)
}

// RemoveApprovedProvider is a paid mutator transaction binding the contract method 0x5840b83d.
//
// Solidity: function removeApprovedProvider(uint256 providerId, uint256 index) returns()
func (_FWSS *FWSSSession) RemoveApprovedProvider(providerId *big.Int, index *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.RemoveApprovedProvider(&_FWSS.TransactOpts, providerId, index)
}

// RemoveApprovedProvider is a paid mutator transaction binding the contract method 0x5840b83d.
//
// Solidity: function removeApprovedProvider(uint256 providerId, uint256 index) returns()
func (_FWSS *FWSSTransactorSession) RemoveApprovedProvider(providerId *big.Int, index *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.RemoveApprovedProvider(&_FWSS.TransactOpts, providerId, index)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_FWSS *FWSSTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_FWSS *FWSSSession) RenounceOwnership() (*types.Transaction, error) {
	return _FWSS.Contract.RenounceOwnership(&_FWSS.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_FWSS *FWSSTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _FWSS.Contract.RenounceOwnership(&_FWSS.TransactOpts)
}

// SetDataSetAuthorizer is a paid mutator transaction binding the contract method 0xca2e5728.
//
// Solidity: function setDataSetAuthorizer(uint256 dataSetId, address authorizer) returns()
func (_FWSS *FWSSTransactor) SetDataSetAuthorizer(opts *bind.TransactOpts, dataSetId *big.Int, authorizer common.Address) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "setDataSetAuthorizer", dataSetId, authorizer)
}

// SetDataSetAuthorizer is a paid mutator transaction binding the contract method 0xca2e5728.
//
// Solidity: function setDataSetAuthorizer(uint256 dataSetId, address authorizer) returns()
func (_FWSS *FWSSSession) SetDataSetAuthorizer(dataSetId *big.Int, authorizer common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.SetDataSetAuthorizer(&_FWSS.TransactOpts, dataSetId, authorizer)
}

// SetDataSetAuthorizer is a paid mutator transaction binding the contract method 0xca2e5728.
//
// Solidity: function setDataSetAuthorizer(uint256 dataSetId, address authorizer) returns()
func (_FWSS *FWSSTransactorSession) SetDataSetAuthorizer(dataSetId *big.Int, authorizer common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.SetDataSetAuthorizer(&_FWSS.TransactOpts, dataSetId, authorizer)
}

// SettleFilBeamPaymentRails is a paid mutator transaction binding the contract method 0x3615edff.
//
// Solidity: function settleFilBeamPaymentRails(uint256 dataSetId, uint256 cdnAmount, uint256 cacheMissAmount) returns()
func (_FWSS *FWSSTransactor) SettleFilBeamPaymentRails(opts *bind.TransactOpts, dataSetId *big.Int, cdnAmount *big.Int, cacheMissAmount *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "settleFilBeamPaymentRails", dataSetId, cdnAmount, cacheMissAmount)
}

// SettleFilBeamPaymentRails is a paid mutator transaction binding the contract method 0x3615edff.
//
// Solidity: function settleFilBeamPaymentRails(uint256 dataSetId, uint256 cdnAmount, uint256 cacheMissAmount) returns()
func (_FWSS *FWSSSession) SettleFilBeamPaymentRails(dataSetId *big.Int, cdnAmount *big.Int, cacheMissAmount *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.SettleFilBeamPaymentRails(&_FWSS.TransactOpts, dataSetId, cdnAmount, cacheMissAmount)
}

// SettleFilBeamPaymentRails is a paid mutator transaction binding the contract method 0x3615edff.
//
// Solidity: function settleFilBeamPaymentRails(uint256 dataSetId, uint256 cdnAmount, uint256 cacheMissAmount) returns()
func (_FWSS *FWSSTransactorSession) SettleFilBeamPaymentRails(dataSetId *big.Int, cdnAmount *big.Int, cacheMissAmount *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.SettleFilBeamPaymentRails(&_FWSS.TransactOpts, dataSetId, cdnAmount, cacheMissAmount)
}

// TerminateCDNService is a paid mutator transaction binding the contract method 0x648564c0.
//
// Solidity: function terminateCDNService(uint256 dataSetId) returns()
func (_FWSS *FWSSTransactor) TerminateCDNService(opts *bind.TransactOpts, dataSetId *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "terminateCDNService", dataSetId)
}

// TerminateCDNService is a paid mutator transaction binding the contract method 0x648564c0.
//
// Solidity: function terminateCDNService(uint256 dataSetId) returns()
func (_FWSS *FWSSSession) TerminateCDNService(dataSetId *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TerminateCDNService(&_FWSS.TransactOpts, dataSetId)
}

// TerminateCDNService is a paid mutator transaction binding the contract method 0x648564c0.
//
// Solidity: function terminateCDNService(uint256 dataSetId) returns()
func (_FWSS *FWSSTransactorSession) TerminateCDNService(dataSetId *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TerminateCDNService(&_FWSS.TransactOpts, dataSetId)
}

// TerminateService is a paid mutator transaction binding the contract method 0xb997a71e.
//
// Solidity: function terminateService(uint256 dataSetId) returns()
func (_FWSS *FWSSTransactor) TerminateService(opts *bind.TransactOpts, dataSetId *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "terminateService", dataSetId)
}

// TerminateService is a paid mutator transaction binding the contract method 0xb997a71e.
//
// Solidity: function terminateService(uint256 dataSetId) returns()
func (_FWSS *FWSSSession) TerminateService(dataSetId *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TerminateService(&_FWSS.TransactOpts, dataSetId)
}

// TerminateService is a paid mutator transaction binding the contract method 0xb997a71e.
//
// Solidity: function terminateService(uint256 dataSetId) returns()
func (_FWSS *FWSSTransactorSession) TerminateService(dataSetId *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TerminateService(&_FWSS.TransactOpts, dataSetId)
}

// TerminateService0 is a paid mutator transaction binding the contract method 0xd0e3c954.
//
// Solidity: function terminateService(uint256 dataSetId, bytes extraData) returns()
func (_FWSS *FWSSTransactor) TerminateService0(opts *bind.TransactOpts, dataSetId *big.Int, extraData []byte) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "terminateService0", dataSetId, extraData)
}

// TerminateService0 is a paid mutator transaction binding the contract method 0xd0e3c954.
//
// Solidity: function terminateService(uint256 dataSetId, bytes extraData) returns()
func (_FWSS *FWSSSession) TerminateService0(dataSetId *big.Int, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.TerminateService0(&_FWSS.TransactOpts, dataSetId, extraData)
}

// TerminateService0 is a paid mutator transaction binding the contract method 0xd0e3c954.
//
// Solidity: function terminateService(uint256 dataSetId, bytes extraData) returns()
func (_FWSS *FWSSTransactorSession) TerminateService0(dataSetId *big.Int, extraData []byte) (*types.Transaction, error) {
	return _FWSS.Contract.TerminateService0(&_FWSS.TransactOpts, dataSetId, extraData)
}

// TopUpCDNPaymentRails is a paid mutator transaction binding the contract method 0xeb561d9c.
//
// Solidity: function topUpCDNPaymentRails(uint256 dataSetId, uint256 cdnAmountToAdd, uint256 cacheMissAmountToAdd) returns()
func (_FWSS *FWSSTransactor) TopUpCDNPaymentRails(opts *bind.TransactOpts, dataSetId *big.Int, cdnAmountToAdd *big.Int, cacheMissAmountToAdd *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "topUpCDNPaymentRails", dataSetId, cdnAmountToAdd, cacheMissAmountToAdd)
}

// TopUpCDNPaymentRails is a paid mutator transaction binding the contract method 0xeb561d9c.
//
// Solidity: function topUpCDNPaymentRails(uint256 dataSetId, uint256 cdnAmountToAdd, uint256 cacheMissAmountToAdd) returns()
func (_FWSS *FWSSSession) TopUpCDNPaymentRails(dataSetId *big.Int, cdnAmountToAdd *big.Int, cacheMissAmountToAdd *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TopUpCDNPaymentRails(&_FWSS.TransactOpts, dataSetId, cdnAmountToAdd, cacheMissAmountToAdd)
}

// TopUpCDNPaymentRails is a paid mutator transaction binding the contract method 0xeb561d9c.
//
// Solidity: function topUpCDNPaymentRails(uint256 dataSetId, uint256 cdnAmountToAdd, uint256 cacheMissAmountToAdd) returns()
func (_FWSS *FWSSTransactorSession) TopUpCDNPaymentRails(dataSetId *big.Int, cdnAmountToAdd *big.Int, cacheMissAmountToAdd *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TopUpCDNPaymentRails(&_FWSS.TransactOpts, dataSetId, cdnAmountToAdd, cacheMissAmountToAdd)
}

// TopUpLifecycleReserve is a paid mutator transaction binding the contract method 0x8f9324dd.
//
// Solidity: function topUpLifecycleReserve(uint256 dataSetId, uint256 amount) returns()
func (_FWSS *FWSSTransactor) TopUpLifecycleReserve(opts *bind.TransactOpts, dataSetId *big.Int, amount *big.Int) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "topUpLifecycleReserve", dataSetId, amount)
}

// TopUpLifecycleReserve is a paid mutator transaction binding the contract method 0x8f9324dd.
//
// Solidity: function topUpLifecycleReserve(uint256 dataSetId, uint256 amount) returns()
func (_FWSS *FWSSSession) TopUpLifecycleReserve(dataSetId *big.Int, amount *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TopUpLifecycleReserve(&_FWSS.TransactOpts, dataSetId, amount)
}

// TopUpLifecycleReserve is a paid mutator transaction binding the contract method 0x8f9324dd.
//
// Solidity: function topUpLifecycleReserve(uint256 dataSetId, uint256 amount) returns()
func (_FWSS *FWSSTransactorSession) TopUpLifecycleReserve(dataSetId *big.Int, amount *big.Int) (*types.Transaction, error) {
	return _FWSS.Contract.TopUpLifecycleReserve(&_FWSS.TransactOpts, dataSetId, amount)
}

// TransferFilBeamController is a paid mutator transaction binding the contract method 0x5e786446.
//
// Solidity: function transferFilBeamController(address newController) returns()
func (_FWSS *FWSSTransactor) TransferFilBeamController(opts *bind.TransactOpts, newController common.Address) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "transferFilBeamController", newController)
}

// TransferFilBeamController is a paid mutator transaction binding the contract method 0x5e786446.
//
// Solidity: function transferFilBeamController(address newController) returns()
func (_FWSS *FWSSSession) TransferFilBeamController(newController common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.TransferFilBeamController(&_FWSS.TransactOpts, newController)
}

// TransferFilBeamController is a paid mutator transaction binding the contract method 0x5e786446.
//
// Solidity: function transferFilBeamController(address newController) returns()
func (_FWSS *FWSSTransactorSession) TransferFilBeamController(newController common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.TransferFilBeamController(&_FWSS.TransactOpts, newController)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_FWSS *FWSSTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _FWSS.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_FWSS *FWSSSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.TransferOwnership(&_FWSS.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_FWSS *FWSSTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _FWSS.Contract.TransferOwnership(&_FWSS.TransactOpts, newOwner)
}

// FWSSCDNPaymentRailsToppedUpIterator is returned from FilterCDNPaymentRailsToppedUp and is used to iterate over the raw logs and unpacked data for CDNPaymentRailsToppedUp events raised by the FWSS contract.
type FWSSCDNPaymentRailsToppedUpIterator struct {
	Event *FWSSCDNPaymentRailsToppedUp // Event containing the contract specifics and raw log

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
func (it *FWSSCDNPaymentRailsToppedUpIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSCDNPaymentRailsToppedUp)
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
		it.Event = new(FWSSCDNPaymentRailsToppedUp)
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
func (it *FWSSCDNPaymentRailsToppedUpIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSCDNPaymentRailsToppedUpIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSCDNPaymentRailsToppedUp represents a CDNPaymentRailsToppedUp event raised by the FWSS contract.
type FWSSCDNPaymentRailsToppedUp struct {
	DataSetId            *big.Int
	CdnAmountAdded       *big.Int
	TotalCdnLockup       *big.Int
	CacheMissAmountAdded *big.Int
	TotalCacheMissLockup *big.Int
	Raw                  types.Log // Blockchain specific contextual infos
}

// FilterCDNPaymentRailsToppedUp is a free log retrieval operation binding the contract event 0x6b6e3adced39b19ee0a9f68ef785f7275ed75801e5f126964678fdf0f0552711.
//
// Solidity: event CDNPaymentRailsToppedUp(uint256 indexed dataSetId, uint256 cdnAmountAdded, uint256 totalCdnLockup, uint256 cacheMissAmountAdded, uint256 totalCacheMissLockup)
func (_FWSS *FWSSFilterer) FilterCDNPaymentRailsToppedUp(opts *bind.FilterOpts, dataSetId []*big.Int) (*FWSSCDNPaymentRailsToppedUpIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "CDNPaymentRailsToppedUp", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSCDNPaymentRailsToppedUpIterator{contract: _FWSS.contract, event: "CDNPaymentRailsToppedUp", logs: logs, sub: sub}, nil
}

// WatchCDNPaymentRailsToppedUp is a free log subscription operation binding the contract event 0x6b6e3adced39b19ee0a9f68ef785f7275ed75801e5f126964678fdf0f0552711.
//
// Solidity: event CDNPaymentRailsToppedUp(uint256 indexed dataSetId, uint256 cdnAmountAdded, uint256 totalCdnLockup, uint256 cacheMissAmountAdded, uint256 totalCacheMissLockup)
func (_FWSS *FWSSFilterer) WatchCDNPaymentRailsToppedUp(opts *bind.WatchOpts, sink chan<- *FWSSCDNPaymentRailsToppedUp, dataSetId []*big.Int) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "CDNPaymentRailsToppedUp", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSCDNPaymentRailsToppedUp)
				if err := _FWSS.contract.UnpackLog(event, "CDNPaymentRailsToppedUp", log); err != nil {
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

// ParseCDNPaymentRailsToppedUp is a log parse operation binding the contract event 0x6b6e3adced39b19ee0a9f68ef785f7275ed75801e5f126964678fdf0f0552711.
//
// Solidity: event CDNPaymentRailsToppedUp(uint256 indexed dataSetId, uint256 cdnAmountAdded, uint256 totalCdnLockup, uint256 cacheMissAmountAdded, uint256 totalCacheMissLockup)
func (_FWSS *FWSSFilterer) ParseCDNPaymentRailsToppedUp(log types.Log) (*FWSSCDNPaymentRailsToppedUp, error) {
	event := new(FWSSCDNPaymentRailsToppedUp)
	if err := _FWSS.contract.UnpackLog(event, "CDNPaymentRailsToppedUp", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSCDNServiceTerminatedIterator is returned from FilterCDNServiceTerminated and is used to iterate over the raw logs and unpacked data for CDNServiceTerminated events raised by the FWSS contract.
type FWSSCDNServiceTerminatedIterator struct {
	Event *FWSSCDNServiceTerminated // Event containing the contract specifics and raw log

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
func (it *FWSSCDNServiceTerminatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSCDNServiceTerminated)
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
		it.Event = new(FWSSCDNServiceTerminated)
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
func (it *FWSSCDNServiceTerminatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSCDNServiceTerminatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSCDNServiceTerminated represents a CDNServiceTerminated event raised by the FWSS contract.
type FWSSCDNServiceTerminated struct {
	Caller          common.Address
	DataSetId       *big.Int
	CacheMissRailId *big.Int
	CdnRailId       *big.Int
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterCDNServiceTerminated is a free log retrieval operation binding the contract event 0xe050575f2f51273412c3b1a9a74ce3a2abc98172b48f6d19442de80a3744367d.
//
// Solidity: event CDNServiceTerminated(address indexed caller, uint256 indexed dataSetId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) FilterCDNServiceTerminated(opts *bind.FilterOpts, caller []common.Address, dataSetId []*big.Int) (*FWSSCDNServiceTerminatedIterator, error) {

	var callerRule []interface{}
	for _, callerItem := range caller {
		callerRule = append(callerRule, callerItem)
	}
	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "CDNServiceTerminated", callerRule, dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSCDNServiceTerminatedIterator{contract: _FWSS.contract, event: "CDNServiceTerminated", logs: logs, sub: sub}, nil
}

// WatchCDNServiceTerminated is a free log subscription operation binding the contract event 0xe050575f2f51273412c3b1a9a74ce3a2abc98172b48f6d19442de80a3744367d.
//
// Solidity: event CDNServiceTerminated(address indexed caller, uint256 indexed dataSetId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) WatchCDNServiceTerminated(opts *bind.WatchOpts, sink chan<- *FWSSCDNServiceTerminated, caller []common.Address, dataSetId []*big.Int) (event.Subscription, error) {

	var callerRule []interface{}
	for _, callerItem := range caller {
		callerRule = append(callerRule, callerItem)
	}
	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "CDNServiceTerminated", callerRule, dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSCDNServiceTerminated)
				if err := _FWSS.contract.UnpackLog(event, "CDNServiceTerminated", log); err != nil {
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

// ParseCDNServiceTerminated is a log parse operation binding the contract event 0xe050575f2f51273412c3b1a9a74ce3a2abc98172b48f6d19442de80a3744367d.
//
// Solidity: event CDNServiceTerminated(address indexed caller, uint256 indexed dataSetId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) ParseCDNServiceTerminated(log types.Log) (*FWSSCDNServiceTerminated, error) {
	event := new(FWSSCDNServiceTerminated)
	if err := _FWSS.contract.UnpackLog(event, "CDNServiceTerminated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSDataSetAbandonedIterator is returned from FilterDataSetAbandoned and is used to iterate over the raw logs and unpacked data for DataSetAbandoned events raised by the FWSS contract.
type FWSSDataSetAbandonedIterator struct {
	Event *FWSSDataSetAbandoned // Event containing the contract specifics and raw log

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
func (it *FWSSDataSetAbandonedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSDataSetAbandoned)
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
		it.Event = new(FWSSDataSetAbandoned)
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
func (it *FWSSDataSetAbandonedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSDataSetAbandonedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSDataSetAbandoned represents a DataSetAbandoned event raised by the FWSS contract.
type FWSSDataSetAbandoned struct {
	DataSetId       *big.Int
	PdpRailId       *big.Int
	CacheMissRailId *big.Int
	CdnRailId       *big.Int
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterDataSetAbandoned is a free log retrieval operation binding the contract event 0x2299c9051e3efb30d43a1a50966a7a5e1ecc1e0fdc0cc6587c9141facbbdee88.
//
// Solidity: event DataSetAbandoned(uint256 indexed dataSetId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) FilterDataSetAbandoned(opts *bind.FilterOpts, dataSetId []*big.Int) (*FWSSDataSetAbandonedIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "DataSetAbandoned", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSDataSetAbandonedIterator{contract: _FWSS.contract, event: "DataSetAbandoned", logs: logs, sub: sub}, nil
}

// WatchDataSetAbandoned is a free log subscription operation binding the contract event 0x2299c9051e3efb30d43a1a50966a7a5e1ecc1e0fdc0cc6587c9141facbbdee88.
//
// Solidity: event DataSetAbandoned(uint256 indexed dataSetId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) WatchDataSetAbandoned(opts *bind.WatchOpts, sink chan<- *FWSSDataSetAbandoned, dataSetId []*big.Int) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "DataSetAbandoned", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSDataSetAbandoned)
				if err := _FWSS.contract.UnpackLog(event, "DataSetAbandoned", log); err != nil {
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

// ParseDataSetAbandoned is a log parse operation binding the contract event 0x2299c9051e3efb30d43a1a50966a7a5e1ecc1e0fdc0cc6587c9141facbbdee88.
//
// Solidity: event DataSetAbandoned(uint256 indexed dataSetId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) ParseDataSetAbandoned(log types.Log) (*FWSSDataSetAbandoned, error) {
	event := new(FWSSDataSetAbandoned)
	if err := _FWSS.contract.UnpackLog(event, "DataSetAbandoned", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSDataSetAuthorizerSetIterator is returned from FilterDataSetAuthorizerSet and is used to iterate over the raw logs and unpacked data for DataSetAuthorizerSet events raised by the FWSS contract.
type FWSSDataSetAuthorizerSetIterator struct {
	Event *FWSSDataSetAuthorizerSet // Event containing the contract specifics and raw log

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
func (it *FWSSDataSetAuthorizerSetIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSDataSetAuthorizerSet)
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
		it.Event = new(FWSSDataSetAuthorizerSet)
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
func (it *FWSSDataSetAuthorizerSetIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSDataSetAuthorizerSetIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSDataSetAuthorizerSet represents a DataSetAuthorizerSet event raised by the FWSS contract.
type FWSSDataSetAuthorizerSet struct {
	DataSetId  *big.Int
	Authorizer common.Address
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterDataSetAuthorizerSet is a free log retrieval operation binding the contract event 0x446f5e7ee362ce45d3ac8588bf637c5a9f08f867c72a2202d85555c1fa319df1.
//
// Solidity: event DataSetAuthorizerSet(uint256 indexed dataSetId, address indexed authorizer)
func (_FWSS *FWSSFilterer) FilterDataSetAuthorizerSet(opts *bind.FilterOpts, dataSetId []*big.Int, authorizer []common.Address) (*FWSSDataSetAuthorizerSetIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var authorizerRule []interface{}
	for _, authorizerItem := range authorizer {
		authorizerRule = append(authorizerRule, authorizerItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "DataSetAuthorizerSet", dataSetIdRule, authorizerRule)
	if err != nil {
		return nil, err
	}
	return &FWSSDataSetAuthorizerSetIterator{contract: _FWSS.contract, event: "DataSetAuthorizerSet", logs: logs, sub: sub}, nil
}

// WatchDataSetAuthorizerSet is a free log subscription operation binding the contract event 0x446f5e7ee362ce45d3ac8588bf637c5a9f08f867c72a2202d85555c1fa319df1.
//
// Solidity: event DataSetAuthorizerSet(uint256 indexed dataSetId, address indexed authorizer)
func (_FWSS *FWSSFilterer) WatchDataSetAuthorizerSet(opts *bind.WatchOpts, sink chan<- *FWSSDataSetAuthorizerSet, dataSetId []*big.Int, authorizer []common.Address) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var authorizerRule []interface{}
	for _, authorizerItem := range authorizer {
		authorizerRule = append(authorizerRule, authorizerItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "DataSetAuthorizerSet", dataSetIdRule, authorizerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSDataSetAuthorizerSet)
				if err := _FWSS.contract.UnpackLog(event, "DataSetAuthorizerSet", log); err != nil {
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

// ParseDataSetAuthorizerSet is a log parse operation binding the contract event 0x446f5e7ee362ce45d3ac8588bf637c5a9f08f867c72a2202d85555c1fa319df1.
//
// Solidity: event DataSetAuthorizerSet(uint256 indexed dataSetId, address indexed authorizer)
func (_FWSS *FWSSFilterer) ParseDataSetAuthorizerSet(log types.Log) (*FWSSDataSetAuthorizerSet, error) {
	event := new(FWSSDataSetAuthorizerSet)
	if err := _FWSS.contract.UnpackLog(event, "DataSetAuthorizerSet", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSDataSetCreatedIterator is returned from FilterDataSetCreated and is used to iterate over the raw logs and unpacked data for DataSetCreated events raised by the FWSS contract.
type FWSSDataSetCreatedIterator struct {
	Event *FWSSDataSetCreated // Event containing the contract specifics and raw log

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
func (it *FWSSDataSetCreatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSDataSetCreated)
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
		it.Event = new(FWSSDataSetCreated)
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
func (it *FWSSDataSetCreatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSDataSetCreatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSDataSetCreated represents a DataSetCreated event raised by the FWSS contract.
type FWSSDataSetCreated struct {
	DataSetId       *big.Int
	ProviderId      *big.Int
	PdpRailId       *big.Int
	CacheMissRailId *big.Int
	CdnRailId       *big.Int
	Payer           common.Address
	ServiceProvider common.Address
	Payee           common.Address
	MetadataKeys    []string
	MetadataValues  []string
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterDataSetCreated is a free log retrieval operation binding the contract event 0xc90cb3863281dc6e2e16e74064ed2e0ab91144ccfe5c3492b8c33f58fe90d0db.
//
// Solidity: event DataSetCreated(uint256 indexed dataSetId, uint256 indexed providerId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId, address payer, address serviceProvider, address payee, string[] metadataKeys, string[] metadataValues)
func (_FWSS *FWSSFilterer) FilterDataSetCreated(opts *bind.FilterOpts, dataSetId []*big.Int, providerId []*big.Int) (*FWSSDataSetCreatedIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "DataSetCreated", dataSetIdRule, providerIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSDataSetCreatedIterator{contract: _FWSS.contract, event: "DataSetCreated", logs: logs, sub: sub}, nil
}

// WatchDataSetCreated is a free log subscription operation binding the contract event 0xc90cb3863281dc6e2e16e74064ed2e0ab91144ccfe5c3492b8c33f58fe90d0db.
//
// Solidity: event DataSetCreated(uint256 indexed dataSetId, uint256 indexed providerId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId, address payer, address serviceProvider, address payee, string[] metadataKeys, string[] metadataValues)
func (_FWSS *FWSSFilterer) WatchDataSetCreated(opts *bind.WatchOpts, sink chan<- *FWSSDataSetCreated, dataSetId []*big.Int, providerId []*big.Int) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "DataSetCreated", dataSetIdRule, providerIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSDataSetCreated)
				if err := _FWSS.contract.UnpackLog(event, "DataSetCreated", log); err != nil {
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

// ParseDataSetCreated is a log parse operation binding the contract event 0xc90cb3863281dc6e2e16e74064ed2e0ab91144ccfe5c3492b8c33f58fe90d0db.
//
// Solidity: event DataSetCreated(uint256 indexed dataSetId, uint256 indexed providerId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId, address payer, address serviceProvider, address payee, string[] metadataKeys, string[] metadataValues)
func (_FWSS *FWSSFilterer) ParseDataSetCreated(log types.Log) (*FWSSDataSetCreated, error) {
	event := new(FWSSDataSetCreated)
	if err := _FWSS.contract.UnpackLog(event, "DataSetCreated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSDataSetServiceProviderChangedIterator is returned from FilterDataSetServiceProviderChanged and is used to iterate over the raw logs and unpacked data for DataSetServiceProviderChanged events raised by the FWSS contract.
type FWSSDataSetServiceProviderChangedIterator struct {
	Event *FWSSDataSetServiceProviderChanged // Event containing the contract specifics and raw log

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
func (it *FWSSDataSetServiceProviderChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSDataSetServiceProviderChanged)
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
		it.Event = new(FWSSDataSetServiceProviderChanged)
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
func (it *FWSSDataSetServiceProviderChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSDataSetServiceProviderChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSDataSetServiceProviderChanged represents a DataSetServiceProviderChanged event raised by the FWSS contract.
type FWSSDataSetServiceProviderChanged struct {
	DataSetId          *big.Int
	OldServiceProvider common.Address
	NewServiceProvider common.Address
	Raw                types.Log // Blockchain specific contextual infos
}

// FilterDataSetServiceProviderChanged is a free log retrieval operation binding the contract event 0x6bf4c2a87885bf6d2d69480d1835a60db52c95621e8b958542cfcdc1350ea991.
//
// Solidity: event DataSetServiceProviderChanged(uint256 indexed dataSetId, address indexed oldServiceProvider, address indexed newServiceProvider)
func (_FWSS *FWSSFilterer) FilterDataSetServiceProviderChanged(opts *bind.FilterOpts, dataSetId []*big.Int, oldServiceProvider []common.Address, newServiceProvider []common.Address) (*FWSSDataSetServiceProviderChangedIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var oldServiceProviderRule []interface{}
	for _, oldServiceProviderItem := range oldServiceProvider {
		oldServiceProviderRule = append(oldServiceProviderRule, oldServiceProviderItem)
	}
	var newServiceProviderRule []interface{}
	for _, newServiceProviderItem := range newServiceProvider {
		newServiceProviderRule = append(newServiceProviderRule, newServiceProviderItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "DataSetServiceProviderChanged", dataSetIdRule, oldServiceProviderRule, newServiceProviderRule)
	if err != nil {
		return nil, err
	}
	return &FWSSDataSetServiceProviderChangedIterator{contract: _FWSS.contract, event: "DataSetServiceProviderChanged", logs: logs, sub: sub}, nil
}

// WatchDataSetServiceProviderChanged is a free log subscription operation binding the contract event 0x6bf4c2a87885bf6d2d69480d1835a60db52c95621e8b958542cfcdc1350ea991.
//
// Solidity: event DataSetServiceProviderChanged(uint256 indexed dataSetId, address indexed oldServiceProvider, address indexed newServiceProvider)
func (_FWSS *FWSSFilterer) WatchDataSetServiceProviderChanged(opts *bind.WatchOpts, sink chan<- *FWSSDataSetServiceProviderChanged, dataSetId []*big.Int, oldServiceProvider []common.Address, newServiceProvider []common.Address) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var oldServiceProviderRule []interface{}
	for _, oldServiceProviderItem := range oldServiceProvider {
		oldServiceProviderRule = append(oldServiceProviderRule, oldServiceProviderItem)
	}
	var newServiceProviderRule []interface{}
	for _, newServiceProviderItem := range newServiceProvider {
		newServiceProviderRule = append(newServiceProviderRule, newServiceProviderItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "DataSetServiceProviderChanged", dataSetIdRule, oldServiceProviderRule, newServiceProviderRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSDataSetServiceProviderChanged)
				if err := _FWSS.contract.UnpackLog(event, "DataSetServiceProviderChanged", log); err != nil {
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

// ParseDataSetServiceProviderChanged is a log parse operation binding the contract event 0x6bf4c2a87885bf6d2d69480d1835a60db52c95621e8b958542cfcdc1350ea991.
//
// Solidity: event DataSetServiceProviderChanged(uint256 indexed dataSetId, address indexed oldServiceProvider, address indexed newServiceProvider)
func (_FWSS *FWSSFilterer) ParseDataSetServiceProviderChanged(log types.Log) (*FWSSDataSetServiceProviderChanged, error) {
	event := new(FWSSDataSetServiceProviderChanged)
	if err := _FWSS.contract.UnpackLog(event, "DataSetServiceProviderChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSDiamondDelegateCallIterator is returned from FilterDiamondDelegateCall and is used to iterate over the raw logs and unpacked data for DiamondDelegateCall events raised by the FWSS contract.
type FWSSDiamondDelegateCallIterator struct {
	Event *FWSSDiamondDelegateCall // Event containing the contract specifics and raw log

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
func (it *FWSSDiamondDelegateCallIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSDiamondDelegateCall)
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
		it.Event = new(FWSSDiamondDelegateCall)
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
func (it *FWSSDiamondDelegateCallIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSDiamondDelegateCallIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSDiamondDelegateCall represents a DiamondDelegateCall event raised by the FWSS contract.
type FWSSDiamondDelegateCall struct {
	Delegate         common.Address
	DelegateCalldata []byte
	Raw              types.Log // Blockchain specific contextual infos
}

// FilterDiamondDelegateCall is a free log retrieval operation binding the contract event 0xfb4514022f0c20335bef459c51d3b7d7ec4a30ac3ab8ccc1b3f83422c51ff3fc.
//
// Solidity: event DiamondDelegateCall(address indexed delegate, bytes delegateCalldata)
func (_FWSS *FWSSFilterer) FilterDiamondDelegateCall(opts *bind.FilterOpts, delegate []common.Address) (*FWSSDiamondDelegateCallIterator, error) {

	var delegateRule []interface{}
	for _, delegateItem := range delegate {
		delegateRule = append(delegateRule, delegateItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "DiamondDelegateCall", delegateRule)
	if err != nil {
		return nil, err
	}
	return &FWSSDiamondDelegateCallIterator{contract: _FWSS.contract, event: "DiamondDelegateCall", logs: logs, sub: sub}, nil
}

// WatchDiamondDelegateCall is a free log subscription operation binding the contract event 0xfb4514022f0c20335bef459c51d3b7d7ec4a30ac3ab8ccc1b3f83422c51ff3fc.
//
// Solidity: event DiamondDelegateCall(address indexed delegate, bytes delegateCalldata)
func (_FWSS *FWSSFilterer) WatchDiamondDelegateCall(opts *bind.WatchOpts, sink chan<- *FWSSDiamondDelegateCall, delegate []common.Address) (event.Subscription, error) {

	var delegateRule []interface{}
	for _, delegateItem := range delegate {
		delegateRule = append(delegateRule, delegateItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "DiamondDelegateCall", delegateRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSDiamondDelegateCall)
				if err := _FWSS.contract.UnpackLog(event, "DiamondDelegateCall", log); err != nil {
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

// ParseDiamondDelegateCall is a log parse operation binding the contract event 0xfb4514022f0c20335bef459c51d3b7d7ec4a30ac3ab8ccc1b3f83422c51ff3fc.
//
// Solidity: event DiamondDelegateCall(address indexed delegate, bytes delegateCalldata)
func (_FWSS *FWSSFilterer) ParseDiamondDelegateCall(log types.Log) (*FWSSDiamondDelegateCall, error) {
	event := new(FWSSDiamondDelegateCall)
	if err := _FWSS.contract.UnpackLog(event, "DiamondDelegateCall", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSEIP712DomainChangedIterator is returned from FilterEIP712DomainChanged and is used to iterate over the raw logs and unpacked data for EIP712DomainChanged events raised by the FWSS contract.
type FWSSEIP712DomainChangedIterator struct {
	Event *FWSSEIP712DomainChanged // Event containing the contract specifics and raw log

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
func (it *FWSSEIP712DomainChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSEIP712DomainChanged)
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
		it.Event = new(FWSSEIP712DomainChanged)
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
func (it *FWSSEIP712DomainChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSEIP712DomainChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSEIP712DomainChanged represents a EIP712DomainChanged event raised by the FWSS contract.
type FWSSEIP712DomainChanged struct {
	Raw types.Log // Blockchain specific contextual infos
}

// FilterEIP712DomainChanged is a free log retrieval operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_FWSS *FWSSFilterer) FilterEIP712DomainChanged(opts *bind.FilterOpts) (*FWSSEIP712DomainChangedIterator, error) {

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return &FWSSEIP712DomainChangedIterator{contract: _FWSS.contract, event: "EIP712DomainChanged", logs: logs, sub: sub}, nil
}

// WatchEIP712DomainChanged is a free log subscription operation binding the contract event 0x0a6387c9ea3628b88a633bb4f3b151770f70085117a15f9bf3787cda53f13d31.
//
// Solidity: event EIP712DomainChanged()
func (_FWSS *FWSSFilterer) WatchEIP712DomainChanged(opts *bind.WatchOpts, sink chan<- *FWSSEIP712DomainChanged) (event.Subscription, error) {

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "EIP712DomainChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSEIP712DomainChanged)
				if err := _FWSS.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
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
func (_FWSS *FWSSFilterer) ParseEIP712DomainChanged(log types.Log) (*FWSSEIP712DomainChanged, error) {
	event := new(FWSSEIP712DomainChanged)
	if err := _FWSS.contract.UnpackLog(event, "EIP712DomainChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSFaultRecordIterator is returned from FilterFaultRecord and is used to iterate over the raw logs and unpacked data for FaultRecord events raised by the FWSS contract.
type FWSSFaultRecordIterator struct {
	Event *FWSSFaultRecord // Event containing the contract specifics and raw log

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
func (it *FWSSFaultRecordIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSFaultRecord)
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
		it.Event = new(FWSSFaultRecord)
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
func (it *FWSSFaultRecordIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSFaultRecordIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSFaultRecord represents a FaultRecord event raised by the FWSS contract.
type FWSSFaultRecord struct {
	DataSetId      *big.Int
	PeriodsFaulted *big.Int
	Deadline       *big.Int
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterFaultRecord is a free log retrieval operation binding the contract event 0xff5f076c63706be9f7eaafa8329db4a9ce9b9e3cd6e53470f05491e2043e1a81.
//
// Solidity: event FaultRecord(uint256 indexed dataSetId, uint256 periodsFaulted, uint256 deadline)
func (_FWSS *FWSSFilterer) FilterFaultRecord(opts *bind.FilterOpts, dataSetId []*big.Int) (*FWSSFaultRecordIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "FaultRecord", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSFaultRecordIterator{contract: _FWSS.contract, event: "FaultRecord", logs: logs, sub: sub}, nil
}

// WatchFaultRecord is a free log subscription operation binding the contract event 0xff5f076c63706be9f7eaafa8329db4a9ce9b9e3cd6e53470f05491e2043e1a81.
//
// Solidity: event FaultRecord(uint256 indexed dataSetId, uint256 periodsFaulted, uint256 deadline)
func (_FWSS *FWSSFilterer) WatchFaultRecord(opts *bind.WatchOpts, sink chan<- *FWSSFaultRecord, dataSetId []*big.Int) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "FaultRecord", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSFaultRecord)
				if err := _FWSS.contract.UnpackLog(event, "FaultRecord", log); err != nil {
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

// ParseFaultRecord is a log parse operation binding the contract event 0xff5f076c63706be9f7eaafa8329db4a9ce9b9e3cd6e53470f05491e2043e1a81.
//
// Solidity: event FaultRecord(uint256 indexed dataSetId, uint256 periodsFaulted, uint256 deadline)
func (_FWSS *FWSSFilterer) ParseFaultRecord(log types.Log) (*FWSSFaultRecord, error) {
	event := new(FWSSFaultRecord)
	if err := _FWSS.contract.UnpackLog(event, "FaultRecord", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSFilBeamControllerChangedIterator is returned from FilterFilBeamControllerChanged and is used to iterate over the raw logs and unpacked data for FilBeamControllerChanged events raised by the FWSS contract.
type FWSSFilBeamControllerChangedIterator struct {
	Event *FWSSFilBeamControllerChanged // Event containing the contract specifics and raw log

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
func (it *FWSSFilBeamControllerChangedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSFilBeamControllerChanged)
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
		it.Event = new(FWSSFilBeamControllerChanged)
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
func (it *FWSSFilBeamControllerChangedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSFilBeamControllerChangedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSFilBeamControllerChanged represents a FilBeamControllerChanged event raised by the FWSS contract.
type FWSSFilBeamControllerChanged struct {
	OldController common.Address
	NewController common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterFilBeamControllerChanged is a free log retrieval operation binding the contract event 0x08d1f43979b2dfd11b4a8873e1df33bb20726f776c16863b31c775ef2a0bf488.
//
// Solidity: event FilBeamControllerChanged(address oldController, address newController)
func (_FWSS *FWSSFilterer) FilterFilBeamControllerChanged(opts *bind.FilterOpts) (*FWSSFilBeamControllerChangedIterator, error) {

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "FilBeamControllerChanged")
	if err != nil {
		return nil, err
	}
	return &FWSSFilBeamControllerChangedIterator{contract: _FWSS.contract, event: "FilBeamControllerChanged", logs: logs, sub: sub}, nil
}

// WatchFilBeamControllerChanged is a free log subscription operation binding the contract event 0x08d1f43979b2dfd11b4a8873e1df33bb20726f776c16863b31c775ef2a0bf488.
//
// Solidity: event FilBeamControllerChanged(address oldController, address newController)
func (_FWSS *FWSSFilterer) WatchFilBeamControllerChanged(opts *bind.WatchOpts, sink chan<- *FWSSFilBeamControllerChanged) (event.Subscription, error) {

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "FilBeamControllerChanged")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSFilBeamControllerChanged)
				if err := _FWSS.contract.UnpackLog(event, "FilBeamControllerChanged", log); err != nil {
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

// ParseFilBeamControllerChanged is a log parse operation binding the contract event 0x08d1f43979b2dfd11b4a8873e1df33bb20726f776c16863b31c775ef2a0bf488.
//
// Solidity: event FilBeamControllerChanged(address oldController, address newController)
func (_FWSS *FWSSFilterer) ParseFilBeamControllerChanged(log types.Log) (*FWSSFilBeamControllerChanged, error) {
	event := new(FWSSFilBeamControllerChanged)
	if err := _FWSS.contract.UnpackLog(event, "FilBeamControllerChanged", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSInitializedIterator is returned from FilterInitialized and is used to iterate over the raw logs and unpacked data for Initialized events raised by the FWSS contract.
type FWSSInitializedIterator struct {
	Event *FWSSInitialized // Event containing the contract specifics and raw log

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
func (it *FWSSInitializedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSInitialized)
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
		it.Event = new(FWSSInitialized)
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
func (it *FWSSInitializedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSInitializedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSInitialized represents a Initialized event raised by the FWSS contract.
type FWSSInitialized struct {
	Version uint64
	Raw     types.Log // Blockchain specific contextual infos
}

// FilterInitialized is a free log retrieval operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_FWSS *FWSSFilterer) FilterInitialized(opts *bind.FilterOpts) (*FWSSInitializedIterator, error) {

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return &FWSSInitializedIterator{contract: _FWSS.contract, event: "Initialized", logs: logs, sub: sub}, nil
}

// WatchInitialized is a free log subscription operation binding the contract event 0xc7f505b2f371ae2175ee4913f4499e1f2633a7b5936321eed1cdaeb6115181d2.
//
// Solidity: event Initialized(uint64 version)
func (_FWSS *FWSSFilterer) WatchInitialized(opts *bind.WatchOpts, sink chan<- *FWSSInitialized) (event.Subscription, error) {

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "Initialized")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSInitialized)
				if err := _FWSS.contract.UnpackLog(event, "Initialized", log); err != nil {
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
func (_FWSS *FWSSFilterer) ParseInitialized(log types.Log) (*FWSSInitialized, error) {
	event := new(FWSSInitialized)
	if err := _FWSS.contract.UnpackLog(event, "Initialized", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the FWSS contract.
type FWSSOwnershipTransferredIterator struct {
	Event *FWSSOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *FWSSOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSOwnershipTransferred)
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
		it.Event = new(FWSSOwnershipTransferred)
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
func (it *FWSSOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSOwnershipTransferred represents a OwnershipTransferred event raised by the FWSS contract.
type FWSSOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_FWSS *FWSSFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*FWSSOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &FWSSOwnershipTransferredIterator{contract: _FWSS.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_FWSS *FWSSFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *FWSSOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSOwnershipTransferred)
				if err := _FWSS.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_FWSS *FWSSFilterer) ParseOwnershipTransferred(log types.Log) (*FWSSOwnershipTransferred, error) {
	event := new(FWSSOwnershipTransferred)
	if err := _FWSS.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSPDPPaymentTerminatedIterator is returned from FilterPDPPaymentTerminated and is used to iterate over the raw logs and unpacked data for PDPPaymentTerminated events raised by the FWSS contract.
type FWSSPDPPaymentTerminatedIterator struct {
	Event *FWSSPDPPaymentTerminated // Event containing the contract specifics and raw log

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
func (it *FWSSPDPPaymentTerminatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSPDPPaymentTerminated)
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
		it.Event = new(FWSSPDPPaymentTerminated)
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
func (it *FWSSPDPPaymentTerminatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSPDPPaymentTerminatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSPDPPaymentTerminated represents a PDPPaymentTerminated event raised by the FWSS contract.
type FWSSPDPPaymentTerminated struct {
	DataSetId *big.Int
	EndEpoch  *big.Int
	PdpRailId *big.Int
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterPDPPaymentTerminated is a free log retrieval operation binding the contract event 0x15371708a8f4745aad266e85741738fc10741627fcc63fd79f29843c59bb3eaf.
//
// Solidity: event PDPPaymentTerminated(uint256 indexed dataSetId, uint256 endEpoch, uint256 pdpRailId)
func (_FWSS *FWSSFilterer) FilterPDPPaymentTerminated(opts *bind.FilterOpts, dataSetId []*big.Int) (*FWSSPDPPaymentTerminatedIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "PDPPaymentTerminated", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSPDPPaymentTerminatedIterator{contract: _FWSS.contract, event: "PDPPaymentTerminated", logs: logs, sub: sub}, nil
}

// WatchPDPPaymentTerminated is a free log subscription operation binding the contract event 0x15371708a8f4745aad266e85741738fc10741627fcc63fd79f29843c59bb3eaf.
//
// Solidity: event PDPPaymentTerminated(uint256 indexed dataSetId, uint256 endEpoch, uint256 pdpRailId)
func (_FWSS *FWSSFilterer) WatchPDPPaymentTerminated(opts *bind.WatchOpts, sink chan<- *FWSSPDPPaymentTerminated, dataSetId []*big.Int) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "PDPPaymentTerminated", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSPDPPaymentTerminated)
				if err := _FWSS.contract.UnpackLog(event, "PDPPaymentTerminated", log); err != nil {
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

// ParsePDPPaymentTerminated is a log parse operation binding the contract event 0x15371708a8f4745aad266e85741738fc10741627fcc63fd79f29843c59bb3eaf.
//
// Solidity: event PDPPaymentTerminated(uint256 indexed dataSetId, uint256 endEpoch, uint256 pdpRailId)
func (_FWSS *FWSSFilterer) ParsePDPPaymentTerminated(log types.Log) (*FWSSPDPPaymentTerminated, error) {
	event := new(FWSSPDPPaymentTerminated)
	if err := _FWSS.contract.UnpackLog(event, "PDPPaymentTerminated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSPieceAddedIterator is returned from FilterPieceAdded and is used to iterate over the raw logs and unpacked data for PieceAdded events raised by the FWSS contract.
type FWSSPieceAddedIterator struct {
	Event *FWSSPieceAdded // Event containing the contract specifics and raw log

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
func (it *FWSSPieceAddedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSPieceAdded)
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
		it.Event = new(FWSSPieceAdded)
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
func (it *FWSSPieceAddedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSPieceAddedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSPieceAdded represents a PieceAdded event raised by the FWSS contract.
type FWSSPieceAdded struct {
	DataSetId *big.Int
	PieceId   *big.Int
	PieceCid  CidsCid
	Keys      []string
	Values    []string
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterPieceAdded is a free log retrieval operation binding the contract event 0xe919e037e2ba38e953115496aafcfc43555ef39f79c2f5f996608a78628eabd7.
//
// Solidity: event PieceAdded(uint256 indexed dataSetId, uint256 indexed pieceId, (bytes) pieceCid, string[] keys, string[] values)
func (_FWSS *FWSSFilterer) FilterPieceAdded(opts *bind.FilterOpts, dataSetId []*big.Int, pieceId []*big.Int) (*FWSSPieceAddedIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var pieceIdRule []interface{}
	for _, pieceIdItem := range pieceId {
		pieceIdRule = append(pieceIdRule, pieceIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "PieceAdded", dataSetIdRule, pieceIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSPieceAddedIterator{contract: _FWSS.contract, event: "PieceAdded", logs: logs, sub: sub}, nil
}

// WatchPieceAdded is a free log subscription operation binding the contract event 0xe919e037e2ba38e953115496aafcfc43555ef39f79c2f5f996608a78628eabd7.
//
// Solidity: event PieceAdded(uint256 indexed dataSetId, uint256 indexed pieceId, (bytes) pieceCid, string[] keys, string[] values)
func (_FWSS *FWSSFilterer) WatchPieceAdded(opts *bind.WatchOpts, sink chan<- *FWSSPieceAdded, dataSetId []*big.Int, pieceId []*big.Int) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}
	var pieceIdRule []interface{}
	for _, pieceIdItem := range pieceId {
		pieceIdRule = append(pieceIdRule, pieceIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "PieceAdded", dataSetIdRule, pieceIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSPieceAdded)
				if err := _FWSS.contract.UnpackLog(event, "PieceAdded", log); err != nil {
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

// ParsePieceAdded is a log parse operation binding the contract event 0xe919e037e2ba38e953115496aafcfc43555ef39f79c2f5f996608a78628eabd7.
//
// Solidity: event PieceAdded(uint256 indexed dataSetId, uint256 indexed pieceId, (bytes) pieceCid, string[] keys, string[] values)
func (_FWSS *FWSSFilterer) ParsePieceAdded(log types.Log) (*FWSSPieceAdded, error) {
	event := new(FWSSPieceAdded)
	if err := _FWSS.contract.UnpackLog(event, "PieceAdded", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSProviderApprovedIterator is returned from FilterProviderApproved and is used to iterate over the raw logs and unpacked data for ProviderApproved events raised by the FWSS contract.
type FWSSProviderApprovedIterator struct {
	Event *FWSSProviderApproved // Event containing the contract specifics and raw log

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
func (it *FWSSProviderApprovedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSProviderApproved)
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
		it.Event = new(FWSSProviderApproved)
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
func (it *FWSSProviderApprovedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSProviderApprovedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSProviderApproved represents a ProviderApproved event raised by the FWSS contract.
type FWSSProviderApproved struct {
	ProviderId *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterProviderApproved is a free log retrieval operation binding the contract event 0xa58a9113199b8ca6ab27dcb19489338356a3870ca0467736c7dff7769d9d0e4b.
//
// Solidity: event ProviderApproved(uint256 indexed providerId)
func (_FWSS *FWSSFilterer) FilterProviderApproved(opts *bind.FilterOpts, providerId []*big.Int) (*FWSSProviderApprovedIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "ProviderApproved", providerIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSProviderApprovedIterator{contract: _FWSS.contract, event: "ProviderApproved", logs: logs, sub: sub}, nil
}

// WatchProviderApproved is a free log subscription operation binding the contract event 0xa58a9113199b8ca6ab27dcb19489338356a3870ca0467736c7dff7769d9d0e4b.
//
// Solidity: event ProviderApproved(uint256 indexed providerId)
func (_FWSS *FWSSFilterer) WatchProviderApproved(opts *bind.WatchOpts, sink chan<- *FWSSProviderApproved, providerId []*big.Int) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "ProviderApproved", providerIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSProviderApproved)
				if err := _FWSS.contract.UnpackLog(event, "ProviderApproved", log); err != nil {
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

// ParseProviderApproved is a log parse operation binding the contract event 0xa58a9113199b8ca6ab27dcb19489338356a3870ca0467736c7dff7769d9d0e4b.
//
// Solidity: event ProviderApproved(uint256 indexed providerId)
func (_FWSS *FWSSFilterer) ParseProviderApproved(log types.Log) (*FWSSProviderApproved, error) {
	event := new(FWSSProviderApproved)
	if err := _FWSS.contract.UnpackLog(event, "ProviderApproved", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSProviderUnapprovedIterator is returned from FilterProviderUnapproved and is used to iterate over the raw logs and unpacked data for ProviderUnapproved events raised by the FWSS contract.
type FWSSProviderUnapprovedIterator struct {
	Event *FWSSProviderUnapproved // Event containing the contract specifics and raw log

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
func (it *FWSSProviderUnapprovedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSProviderUnapproved)
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
		it.Event = new(FWSSProviderUnapproved)
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
func (it *FWSSProviderUnapprovedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSProviderUnapprovedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSProviderUnapproved represents a ProviderUnapproved event raised by the FWSS contract.
type FWSSProviderUnapproved struct {
	ProviderId *big.Int
	Raw        types.Log // Blockchain specific contextual infos
}

// FilterProviderUnapproved is a free log retrieval operation binding the contract event 0xba4e32ee0678ec258ee0a93a97d502407f44c84993025385cd10a7f565c82b24.
//
// Solidity: event ProviderUnapproved(uint256 indexed providerId)
func (_FWSS *FWSSFilterer) FilterProviderUnapproved(opts *bind.FilterOpts, providerId []*big.Int) (*FWSSProviderUnapprovedIterator, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "ProviderUnapproved", providerIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSProviderUnapprovedIterator{contract: _FWSS.contract, event: "ProviderUnapproved", logs: logs, sub: sub}, nil
}

// WatchProviderUnapproved is a free log subscription operation binding the contract event 0xba4e32ee0678ec258ee0a93a97d502407f44c84993025385cd10a7f565c82b24.
//
// Solidity: event ProviderUnapproved(uint256 indexed providerId)
func (_FWSS *FWSSFilterer) WatchProviderUnapproved(opts *bind.WatchOpts, sink chan<- *FWSSProviderUnapproved, providerId []*big.Int) (event.Subscription, error) {

	var providerIdRule []interface{}
	for _, providerIdItem := range providerId {
		providerIdRule = append(providerIdRule, providerIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "ProviderUnapproved", providerIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSProviderUnapproved)
				if err := _FWSS.contract.UnpackLog(event, "ProviderUnapproved", log); err != nil {
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

// ParseProviderUnapproved is a log parse operation binding the contract event 0xba4e32ee0678ec258ee0a93a97d502407f44c84993025385cd10a7f565c82b24.
//
// Solidity: event ProviderUnapproved(uint256 indexed providerId)
func (_FWSS *FWSSFilterer) ParseProviderUnapproved(log types.Log) (*FWSSProviderUnapproved, error) {
	event := new(FWSSProviderUnapproved)
	if err := _FWSS.contract.UnpackLog(event, "ProviderUnapproved", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSRailRateUpdatedIterator is returned from FilterRailRateUpdated and is used to iterate over the raw logs and unpacked data for RailRateUpdated events raised by the FWSS contract.
type FWSSRailRateUpdatedIterator struct {
	Event *FWSSRailRateUpdated // Event containing the contract specifics and raw log

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
func (it *FWSSRailRateUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSRailRateUpdated)
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
		it.Event = new(FWSSRailRateUpdated)
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
func (it *FWSSRailRateUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSRailRateUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSRailRateUpdated represents a RailRateUpdated event raised by the FWSS contract.
type FWSSRailRateUpdated struct {
	DataSetId *big.Int
	RailId    *big.Int
	NewRate   *big.Int
	Raw       types.Log // Blockchain specific contextual infos
}

// FilterRailRateUpdated is a free log retrieval operation binding the contract event 0xe48d2ac923afa407ac53fd133176c8ba21d06ab27a0a79391ce837609fe19a63.
//
// Solidity: event RailRateUpdated(uint256 indexed dataSetId, uint256 railId, uint256 newRate)
func (_FWSS *FWSSFilterer) FilterRailRateUpdated(opts *bind.FilterOpts, dataSetId []*big.Int) (*FWSSRailRateUpdatedIterator, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "RailRateUpdated", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSRailRateUpdatedIterator{contract: _FWSS.contract, event: "RailRateUpdated", logs: logs, sub: sub}, nil
}

// WatchRailRateUpdated is a free log subscription operation binding the contract event 0xe48d2ac923afa407ac53fd133176c8ba21d06ab27a0a79391ce837609fe19a63.
//
// Solidity: event RailRateUpdated(uint256 indexed dataSetId, uint256 railId, uint256 newRate)
func (_FWSS *FWSSFilterer) WatchRailRateUpdated(opts *bind.WatchOpts, sink chan<- *FWSSRailRateUpdated, dataSetId []*big.Int) (event.Subscription, error) {

	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "RailRateUpdated", dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSRailRateUpdated)
				if err := _FWSS.contract.UnpackLog(event, "RailRateUpdated", log); err != nil {
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

// ParseRailRateUpdated is a log parse operation binding the contract event 0xe48d2ac923afa407ac53fd133176c8ba21d06ab27a0a79391ce837609fe19a63.
//
// Solidity: event RailRateUpdated(uint256 indexed dataSetId, uint256 railId, uint256 newRate)
func (_FWSS *FWSSFilterer) ParseRailRateUpdated(log types.Log) (*FWSSRailRateUpdated, error) {
	event := new(FWSSRailRateUpdated)
	if err := _FWSS.contract.UnpackLog(event, "RailRateUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSServiceTerminatedIterator is returned from FilterServiceTerminated and is used to iterate over the raw logs and unpacked data for ServiceTerminated events raised by the FWSS contract.
type FWSSServiceTerminatedIterator struct {
	Event *FWSSServiceTerminated // Event containing the contract specifics and raw log

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
func (it *FWSSServiceTerminatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSServiceTerminated)
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
		it.Event = new(FWSSServiceTerminated)
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
func (it *FWSSServiceTerminatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSServiceTerminatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSServiceTerminated represents a ServiceTerminated event raised by the FWSS contract.
type FWSSServiceTerminated struct {
	Approver        common.Address
	DataSetId       *big.Int
	PdpRailId       *big.Int
	CacheMissRailId *big.Int
	CdnRailId       *big.Int
	Raw             types.Log // Blockchain specific contextual infos
}

// FilterServiceTerminated is a free log retrieval operation binding the contract event 0x10c867634d8e51bbfd5ddd2e06b4f4a97a91274488ee3afbe1e146aa79e85293.
//
// Solidity: event ServiceTerminated(address indexed approver, uint256 indexed dataSetId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) FilterServiceTerminated(opts *bind.FilterOpts, approver []common.Address, dataSetId []*big.Int) (*FWSSServiceTerminatedIterator, error) {

	var approverRule []interface{}
	for _, approverItem := range approver {
		approverRule = append(approverRule, approverItem)
	}
	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "ServiceTerminated", approverRule, dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return &FWSSServiceTerminatedIterator{contract: _FWSS.contract, event: "ServiceTerminated", logs: logs, sub: sub}, nil
}

// WatchServiceTerminated is a free log subscription operation binding the contract event 0x10c867634d8e51bbfd5ddd2e06b4f4a97a91274488ee3afbe1e146aa79e85293.
//
// Solidity: event ServiceTerminated(address indexed approver, uint256 indexed dataSetId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) WatchServiceTerminated(opts *bind.WatchOpts, sink chan<- *FWSSServiceTerminated, approver []common.Address, dataSetId []*big.Int) (event.Subscription, error) {

	var approverRule []interface{}
	for _, approverItem := range approver {
		approverRule = append(approverRule, approverItem)
	}
	var dataSetIdRule []interface{}
	for _, dataSetIdItem := range dataSetId {
		dataSetIdRule = append(dataSetIdRule, dataSetIdItem)
	}

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "ServiceTerminated", approverRule, dataSetIdRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSServiceTerminated)
				if err := _FWSS.contract.UnpackLog(event, "ServiceTerminated", log); err != nil {
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

// ParseServiceTerminated is a log parse operation binding the contract event 0x10c867634d8e51bbfd5ddd2e06b4f4a97a91274488ee3afbe1e146aa79e85293.
//
// Solidity: event ServiceTerminated(address indexed approver, uint256 indexed dataSetId, uint256 pdpRailId, uint256 cacheMissRailId, uint256 cdnRailId)
func (_FWSS *FWSSFilterer) ParseServiceTerminated(log types.Log) (*FWSSServiceTerminated, error) {
	event := new(FWSSServiceTerminated)
	if err := _FWSS.contract.UnpackLog(event, "ServiceTerminated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}

// FWSSUpgradeAnnouncedIterator is returned from FilterUpgradeAnnounced and is used to iterate over the raw logs and unpacked data for UpgradeAnnounced events raised by the FWSS contract.
type FWSSUpgradeAnnouncedIterator struct {
	Event *FWSSUpgradeAnnounced // Event containing the contract specifics and raw log

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
func (it *FWSSUpgradeAnnouncedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(FWSSUpgradeAnnounced)
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
		it.Event = new(FWSSUpgradeAnnounced)
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
func (it *FWSSUpgradeAnnouncedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *FWSSUpgradeAnnouncedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// FWSSUpgradeAnnounced represents a UpgradeAnnounced event raised by the FWSS contract.
type FWSSUpgradeAnnounced struct {
	PlannedUpgrade FWSSStoragePlannedUpgrade
	Raw            types.Log // Blockchain specific contextual infos
}

// FilterUpgradeAnnounced is a free log retrieval operation binding the contract event 0xbcf8666408d712c75c2cbd790925afbec6495ca9e04186b1182902260a1d53cd.
//
// Solidity: event UpgradeAnnounced((address,uint96) plannedUpgrade)
func (_FWSS *FWSSFilterer) FilterUpgradeAnnounced(opts *bind.FilterOpts) (*FWSSUpgradeAnnouncedIterator, error) {

	logs, sub, err := _FWSS.contract.FilterLogs(opts, "UpgradeAnnounced")
	if err != nil {
		return nil, err
	}
	return &FWSSUpgradeAnnouncedIterator{contract: _FWSS.contract, event: "UpgradeAnnounced", logs: logs, sub: sub}, nil
}

// WatchUpgradeAnnounced is a free log subscription operation binding the contract event 0xbcf8666408d712c75c2cbd790925afbec6495ca9e04186b1182902260a1d53cd.
//
// Solidity: event UpgradeAnnounced((address,uint96) plannedUpgrade)
func (_FWSS *FWSSFilterer) WatchUpgradeAnnounced(opts *bind.WatchOpts, sink chan<- *FWSSUpgradeAnnounced) (event.Subscription, error) {

	logs, sub, err := _FWSS.contract.WatchLogs(opts, "UpgradeAnnounced")
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(FWSSUpgradeAnnounced)
				if err := _FWSS.contract.UnpackLog(event, "UpgradeAnnounced", log); err != nil {
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
func (_FWSS *FWSSFilterer) ParseUpgradeAnnounced(log types.Log) (*FWSSUpgradeAnnounced, error) {
	event := new(FWSSUpgradeAnnounced)
	if err := _FWSS.contract.UnpackLog(event, "UpgradeAnnounced", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
