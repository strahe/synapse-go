// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package fwssview

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

// FilecoinWarmStorageServiceDataSetInfoView is an auto generated low-level Go binding around an user-defined struct.
type FilecoinWarmStorageServiceDataSetInfoView struct {
	PdpRailId       *big.Int
	CacheMissRailId *big.Int
	CdnRailId       *big.Int
	Payer           common.Address
	Payee           common.Address
	ServiceProvider common.Address
	CommissionBps   *big.Int
	ClientDataSetId *big.Int
	PdpEndEpoch     *big.Int
	ProviderId      *big.Int
	DataSetId       *big.Int
}

// FWSSViewMetaData contains all meta data concerning the FWSSView contract.
var FWSSViewMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[{\"name\":\"_service\",\"type\":\"address\",\"internalType\":\"contractFilecoinWarmStorageService\"}],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"clientDataSets\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"dataSetIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"clientDataSets\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"dataSetIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"clientNonces\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"nonce\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"filBeamControllerAddress\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAllDataSetMetadata\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"keys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"values\",\"type\":\"string[]\",\"internalType\":\"string[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getAllPieceMetadata\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"keys\",\"type\":\"string[]\",\"internalType\":\"string[]\"},{\"name\":\"values\",\"type\":\"string[]\",\"internalType\":\"string[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApprovedProviders\",\"inputs\":[{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"providerIds\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getApprovedProvidersLength\",\"inputs\":[],\"outputs\":[{\"name\":\"count\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClientDataSets\",\"inputs\":[{\"name\":\"client\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"offset\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"limit\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"infos\",\"type\":\"tuple[]\",\"internalType\":\"structFilecoinWarmStorageService.DataSetInfoView[]\",\"components\":[{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"commissionBps\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClientDataSets\",\"inputs\":[{\"name\":\"client\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"infos\",\"type\":\"tuple[]\",\"internalType\":\"structFilecoinWarmStorageService.DataSetInfoView[]\",\"components\":[{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"commissionBps\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getClientDataSetsLength\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getCurrentPricingRates\",\"inputs\":[],\"outputs\":[{\"name\":\"storagePrice\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minimumRate\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataSet\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"info\",\"type\":\"tuple\",\"internalType\":\"structFilecoinWarmStorageService.DataSetInfoView\",\"components\":[{\"name\":\"pdpRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cacheMissRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"cdnRailId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"payee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"serviceProvider\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"commissionBps\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataSetMetadata\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"exists\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"value\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getDataSetSizeInBytes\",\"inputs\":[{\"name\":\"leafCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"pure\"},{\"type\":\"function\",\"name\":\"getDataSetStatus\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"status\",\"type\":\"uint8\",\"internalType\":\"enumFilecoinWarmStorageService.DataSetStatus\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getPDPConfig\",\"inputs\":[],\"outputs\":[{\"name\":\"maxProvingPeriod\",\"type\":\"uint64\",\"internalType\":\"uint64\"},{\"name\":\"challengeWindowSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengesPerProof\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"initChallengeWindowStart\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getPieceMetadata\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[{\"name\":\"exists\",\"type\":\"bool\",\"internalType\":\"bool\"},{\"name\":\"value\",\"type\":\"string\",\"internalType\":\"string\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"isProviderApproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"nextPDPChallengeWindowStart\",\"inputs\":[{\"name\":\"setId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"nextUpgrade\",\"inputs\":[],\"outputs\":[{\"name\":\"nextImplementation\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"afterEpoch\",\"type\":\"uint96\",\"internalType\":\"uint96\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provenPeriods\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"periodId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provenThisPeriod\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provingActivationEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"provingDeadline\",\"inputs\":[{\"name\":\"setId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"railToDataSet\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"service\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"contractFilecoinWarmStorageService\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"serviceCommissionBps\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"error\",\"name\":\"ProvingPeriodNotInitialized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"AddressAlreadySet\",\"inputs\":[{\"name\":\"field\",\"type\":\"uint8\",\"internalType\":\"enumErrors.AddressField\"}]},{\"type\":\"error\",\"name\":\"AtLeastOnePriceMustBeNonZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"CDNPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CacheMissPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayer\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayerOrPayee\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expectedPayer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expectedPayee\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"caller\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"CallerNotPayments\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ChallengeWindowTooEarly\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"windowStart\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ClientDataSetAlreadyRegistered\",\"inputs\":[{\"name\":\"clientDataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"CommissionExceedsMaximum\",\"inputs\":[{\"name\":\"commissionType\",\"type\":\"uint8\",\"internalType\":\"enumErrors.CommissionType\"},{\"name\":\"max\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotFoundForRail\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetNotRegistered\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentAlreadyTerminated\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DataSetPaymentBeyondEndEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"currentBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"DivisionByZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"DuplicateMetadataKey\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"key\",\"type\":\"string\",\"internalType\":\"string\"}]},{\"type\":\"error\",\"name\":\"ExtraDataRequired\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"ExtraDataTooLarge\",\"inputs\":[{\"name\":\"actualSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowedSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"FilBeamServiceNotConfigured\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientCapabilitiesForProduct\",\"inputs\":[{\"name\":\"productType\",\"type\":\"uint8\",\"internalType\":\"enumServiceProviderRegistryStorage.ProductType\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"lockupAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"lockupUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minimumLockupRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientLockupFunds\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"minimumRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"available\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientMaxLockupPeriod\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"maxLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"requiredLockupPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InsufficientRateAllowance\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"rateAllowance\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"rateUsage\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minimumRateRequired\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeCount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minExpected\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeEpoch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"minAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidChallengeWindowSize\",\"inputs\":[{\"name\":\"maxProvingPeriod\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"challengeWindowSize\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidDataSetId\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidEpochRange\",\"inputs\":[{\"name\":\"fromEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"toEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidServiceDescriptionLength\",\"inputs\":[{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidServiceNameLength\",\"inputs\":[{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidSignature\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"InvalidSignatureLength\",\"inputs\":[{\"name\":\"expectedLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actualLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"InvalidTopUpAmount\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MaxProvingPeriodZero\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"MetadataArrayCountMismatch\",\"inputs\":[{\"name\":\"metadataArrayCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pieceCount\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyAndValueLengthMismatch\",\"inputs\":[{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"valuesLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataKeyExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"MetadataValueExceedsMaxLength\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"length\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NextProvingPeriodAlreadyCalled\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"periodDeadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"NoPDPPaymentRail\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"OldServiceProviderMismatch\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyFilBeamControllerAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlyPDPVerifierAllowed\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OnlySelf\",\"inputs\":[{\"name\":\"expected\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"actual\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OperatorNotApproved\",\"inputs\":[{\"name\":\"payer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"operator\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"PaymentRailsNotFinalized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"pdpEndEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"PriceExceedsMaximum\",\"inputs\":[{\"name\":\"priceType\",\"type\":\"uint8\",\"internalType\":\"enumErrors.PriceType\"},{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"actual\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProofAlreadySubmitted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderAlreadyApproved\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderIdMismatchAtIndex\",\"inputs\":[{\"name\":\"index\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotInApprovedList\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderNotRegistered\",\"inputs\":[{\"name\":\"provider\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ProvingNotStarted\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingPeriodNotInitialized\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProvingPeriodPassed\",\"inputs\":[{\"name\":\"dataSetId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"deadline\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"nowBlock\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"RailNotAssociated\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"RailNotFullySettled\",\"inputs\":[{\"name\":\"railId\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"settledUpTo\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"endEpoch\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ServiceContractMustTerminateRail\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"StorageProviderChangesNotSupported\",\"inputs\":[]},{\"type\":\"error\",\"name\":\"TooManyMetadataKeys\",\"inputs\":[{\"name\":\"maxAllowed\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"keysLength\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"UnsupportedSignatureV\",\"inputs\":[{\"name\":\"v\",\"type\":\"uint8\",\"internalType\":\"uint8\"}]},{\"type\":\"error\",\"name\":\"ZeroAddress\",\"inputs\":[{\"name\":\"field\",\"type\":\"uint8\",\"internalType\":\"enumErrors.AddressField\"}]}]",
}

// FWSSViewABI is the input ABI used to generate the binding from.
// Deprecated: Use FWSSViewMetaData.ABI instead.
var FWSSViewABI = FWSSViewMetaData.ABI

// FWSSView is an auto generated Go binding around an Ethereum contract.
type FWSSView struct {
	FWSSViewCaller     // Read-only binding to the contract
	FWSSViewTransactor // Write-only binding to the contract
	FWSSViewFilterer   // Log filterer for contract events
}

// FWSSViewCaller is an auto generated read-only Go binding around an Ethereum contract.
type FWSSViewCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// FWSSViewTransactor is an auto generated write-only Go binding around an Ethereum contract.
type FWSSViewTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// FWSSViewFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type FWSSViewFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// FWSSViewSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type FWSSViewSession struct {
	Contract     *FWSSView         // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// FWSSViewCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type FWSSViewCallerSession struct {
	Contract *FWSSViewCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts   // Call options to use throughout this session
}

// FWSSViewTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type FWSSViewTransactorSession struct {
	Contract     *FWSSViewTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// FWSSViewRaw is an auto generated low-level Go binding around an Ethereum contract.
type FWSSViewRaw struct {
	Contract *FWSSView // Generic contract binding to access the raw methods on
}

// FWSSViewCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type FWSSViewCallerRaw struct {
	Contract *FWSSViewCaller // Generic read-only contract binding to access the raw methods on
}

// FWSSViewTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type FWSSViewTransactorRaw struct {
	Contract *FWSSViewTransactor // Generic write-only contract binding to access the raw methods on
}

// NewFWSSView creates a new instance of FWSSView, bound to a specific deployed contract.
func NewFWSSView(address common.Address, backend bind.ContractBackend) (*FWSSView, error) {
	contract, err := bindFWSSView(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &FWSSView{FWSSViewCaller: FWSSViewCaller{contract: contract}, FWSSViewTransactor: FWSSViewTransactor{contract: contract}, FWSSViewFilterer: FWSSViewFilterer{contract: contract}}, nil
}

// NewFWSSViewCaller creates a new read-only instance of FWSSView, bound to a specific deployed contract.
func NewFWSSViewCaller(address common.Address, caller bind.ContractCaller) (*FWSSViewCaller, error) {
	contract, err := bindFWSSView(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &FWSSViewCaller{contract: contract}, nil
}

// NewFWSSViewTransactor creates a new write-only instance of FWSSView, bound to a specific deployed contract.
func NewFWSSViewTransactor(address common.Address, transactor bind.ContractTransactor) (*FWSSViewTransactor, error) {
	contract, err := bindFWSSView(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &FWSSViewTransactor{contract: contract}, nil
}

// NewFWSSViewFilterer creates a new log filterer instance of FWSSView, bound to a specific deployed contract.
func NewFWSSViewFilterer(address common.Address, filterer bind.ContractFilterer) (*FWSSViewFilterer, error) {
	contract, err := bindFWSSView(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &FWSSViewFilterer{contract: contract}, nil
}

// bindFWSSView binds a generic wrapper to an already deployed contract.
func bindFWSSView(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := FWSSViewMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_FWSSView *FWSSViewRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _FWSSView.Contract.FWSSViewCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_FWSSView *FWSSViewRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _FWSSView.Contract.FWSSViewTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_FWSSView *FWSSViewRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _FWSSView.Contract.FWSSViewTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_FWSSView *FWSSViewCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _FWSSView.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_FWSSView *FWSSViewTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _FWSSView.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_FWSSView *FWSSViewTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _FWSSView.Contract.contract.Transact(opts, method, params...)
}

// ClientDataSets is a free data retrieval call binding the contract method 0x1047717c.
//
// Solidity: function clientDataSets(address payer, uint256 offset, uint256 limit) view returns(uint256[] dataSetIds)
func (_FWSSView *FWSSViewCaller) ClientDataSets(opts *bind.CallOpts, payer common.Address, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "clientDataSets", payer, offset, limit)

	if err != nil {
		return *new([]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)

	return out0, err

}

// ClientDataSets is a free data retrieval call binding the contract method 0x1047717c.
//
// Solidity: function clientDataSets(address payer, uint256 offset, uint256 limit) view returns(uint256[] dataSetIds)
func (_FWSSView *FWSSViewSession) ClientDataSets(payer common.Address, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSSView.Contract.ClientDataSets(&_FWSSView.CallOpts, payer, offset, limit)
}

// ClientDataSets is a free data retrieval call binding the contract method 0x1047717c.
//
// Solidity: function clientDataSets(address payer, uint256 offset, uint256 limit) view returns(uint256[] dataSetIds)
func (_FWSSView *FWSSViewCallerSession) ClientDataSets(payer common.Address, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSSView.Contract.ClientDataSets(&_FWSSView.CallOpts, payer, offset, limit)
}

// ClientDataSets0 is a free data retrieval call binding the contract method 0x7dab7c40.
//
// Solidity: function clientDataSets(address payer) view returns(uint256[] dataSetIds)
func (_FWSSView *FWSSViewCaller) ClientDataSets0(opts *bind.CallOpts, payer common.Address) ([]*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "clientDataSets0", payer)

	if err != nil {
		return *new([]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)

	return out0, err

}

// ClientDataSets0 is a free data retrieval call binding the contract method 0x7dab7c40.
//
// Solidity: function clientDataSets(address payer) view returns(uint256[] dataSetIds)
func (_FWSSView *FWSSViewSession) ClientDataSets0(payer common.Address) ([]*big.Int, error) {
	return _FWSSView.Contract.ClientDataSets0(&_FWSSView.CallOpts, payer)
}

// ClientDataSets0 is a free data retrieval call binding the contract method 0x7dab7c40.
//
// Solidity: function clientDataSets(address payer) view returns(uint256[] dataSetIds)
func (_FWSSView *FWSSViewCallerSession) ClientDataSets0(payer common.Address) ([]*big.Int, error) {
	return _FWSSView.Contract.ClientDataSets0(&_FWSSView.CallOpts, payer)
}

// ClientNonces is a free data retrieval call binding the contract method 0x35b0e3f4.
//
// Solidity: function clientNonces(address payer, uint256 nonce) view returns(uint256)
func (_FWSSView *FWSSViewCaller) ClientNonces(opts *bind.CallOpts, payer common.Address, nonce *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "clientNonces", payer, nonce)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ClientNonces is a free data retrieval call binding the contract method 0x35b0e3f4.
//
// Solidity: function clientNonces(address payer, uint256 nonce) view returns(uint256)
func (_FWSSView *FWSSViewSession) ClientNonces(payer common.Address, nonce *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.ClientNonces(&_FWSSView.CallOpts, payer, nonce)
}

// ClientNonces is a free data retrieval call binding the contract method 0x35b0e3f4.
//
// Solidity: function clientNonces(address payer, uint256 nonce) view returns(uint256)
func (_FWSSView *FWSSViewCallerSession) ClientNonces(payer common.Address, nonce *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.ClientNonces(&_FWSSView.CallOpts, payer, nonce)
}

// FilBeamControllerAddress is a free data retrieval call binding the contract method 0xd1147eee.
//
// Solidity: function filBeamControllerAddress() view returns(address)
func (_FWSSView *FWSSViewCaller) FilBeamControllerAddress(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "filBeamControllerAddress")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// FilBeamControllerAddress is a free data retrieval call binding the contract method 0xd1147eee.
//
// Solidity: function filBeamControllerAddress() view returns(address)
func (_FWSSView *FWSSViewSession) FilBeamControllerAddress() (common.Address, error) {
	return _FWSSView.Contract.FilBeamControllerAddress(&_FWSSView.CallOpts)
}

// FilBeamControllerAddress is a free data retrieval call binding the contract method 0xd1147eee.
//
// Solidity: function filBeamControllerAddress() view returns(address)
func (_FWSSView *FWSSViewCallerSession) FilBeamControllerAddress() (common.Address, error) {
	return _FWSSView.Contract.FilBeamControllerAddress(&_FWSSView.CallOpts)
}

// GetAllDataSetMetadata is a free data retrieval call binding the contract method 0xf417c13f.
//
// Solidity: function getAllDataSetMetadata(uint256 dataSetId) view returns(string[] keys, string[] values)
func (_FWSSView *FWSSViewCaller) GetAllDataSetMetadata(opts *bind.CallOpts, dataSetId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getAllDataSetMetadata", dataSetId)

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
func (_FWSSView *FWSSViewSession) GetAllDataSetMetadata(dataSetId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	return _FWSSView.Contract.GetAllDataSetMetadata(&_FWSSView.CallOpts, dataSetId)
}

// GetAllDataSetMetadata is a free data retrieval call binding the contract method 0xf417c13f.
//
// Solidity: function getAllDataSetMetadata(uint256 dataSetId) view returns(string[] keys, string[] values)
func (_FWSSView *FWSSViewCallerSession) GetAllDataSetMetadata(dataSetId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	return _FWSSView.Contract.GetAllDataSetMetadata(&_FWSSView.CallOpts, dataSetId)
}

// GetAllPieceMetadata is a free data retrieval call binding the contract method 0x3c0bd253.
//
// Solidity: function getAllPieceMetadata(uint256 dataSetId, uint256 pieceId) view returns(string[] keys, string[] values)
func (_FWSSView *FWSSViewCaller) GetAllPieceMetadata(opts *bind.CallOpts, dataSetId *big.Int, pieceId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getAllPieceMetadata", dataSetId, pieceId)

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

// GetAllPieceMetadata is a free data retrieval call binding the contract method 0x3c0bd253.
//
// Solidity: function getAllPieceMetadata(uint256 dataSetId, uint256 pieceId) view returns(string[] keys, string[] values)
func (_FWSSView *FWSSViewSession) GetAllPieceMetadata(dataSetId *big.Int, pieceId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	return _FWSSView.Contract.GetAllPieceMetadata(&_FWSSView.CallOpts, dataSetId, pieceId)
}

// GetAllPieceMetadata is a free data retrieval call binding the contract method 0x3c0bd253.
//
// Solidity: function getAllPieceMetadata(uint256 dataSetId, uint256 pieceId) view returns(string[] keys, string[] values)
func (_FWSSView *FWSSViewCallerSession) GetAllPieceMetadata(dataSetId *big.Int, pieceId *big.Int) (struct {
	Keys   []string
	Values []string
}, error) {
	return _FWSSView.Contract.GetAllPieceMetadata(&_FWSSView.CallOpts, dataSetId, pieceId)
}

// GetApprovedProviders is a free data retrieval call binding the contract method 0x7709a7f7.
//
// Solidity: function getApprovedProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds)
func (_FWSSView *FWSSViewCaller) GetApprovedProviders(opts *bind.CallOpts, offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getApprovedProviders", offset, limit)

	if err != nil {
		return *new([]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)

	return out0, err

}

// GetApprovedProviders is a free data retrieval call binding the contract method 0x7709a7f7.
//
// Solidity: function getApprovedProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds)
func (_FWSSView *FWSSViewSession) GetApprovedProviders(offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSSView.Contract.GetApprovedProviders(&_FWSSView.CallOpts, offset, limit)
}

// GetApprovedProviders is a free data retrieval call binding the contract method 0x7709a7f7.
//
// Solidity: function getApprovedProviders(uint256 offset, uint256 limit) view returns(uint256[] providerIds)
func (_FWSSView *FWSSViewCallerSession) GetApprovedProviders(offset *big.Int, limit *big.Int) ([]*big.Int, error) {
	return _FWSSView.Contract.GetApprovedProviders(&_FWSSView.CallOpts, offset, limit)
}

// GetApprovedProvidersLength is a free data retrieval call binding the contract method 0x4d745000.
//
// Solidity: function getApprovedProvidersLength() view returns(uint256 count)
func (_FWSSView *FWSSViewCaller) GetApprovedProvidersLength(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getApprovedProvidersLength")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetApprovedProvidersLength is a free data retrieval call binding the contract method 0x4d745000.
//
// Solidity: function getApprovedProvidersLength() view returns(uint256 count)
func (_FWSSView *FWSSViewSession) GetApprovedProvidersLength() (*big.Int, error) {
	return _FWSSView.Contract.GetApprovedProvidersLength(&_FWSSView.CallOpts)
}

// GetApprovedProvidersLength is a free data retrieval call binding the contract method 0x4d745000.
//
// Solidity: function getApprovedProvidersLength() view returns(uint256 count)
func (_FWSSView *FWSSViewCallerSession) GetApprovedProvidersLength() (*big.Int, error) {
	return _FWSSView.Contract.GetApprovedProvidersLength(&_FWSSView.CallOpts)
}

// GetClientDataSets is a free data retrieval call binding the contract method 0x3208aa1f.
//
// Solidity: function getClientDataSets(address client, uint256 offset, uint256 limit) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256)[] infos)
func (_FWSSView *FWSSViewCaller) GetClientDataSets(opts *bind.CallOpts, client common.Address, offset *big.Int, limit *big.Int) ([]FilecoinWarmStorageServiceDataSetInfoView, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getClientDataSets", client, offset, limit)

	if err != nil {
		return *new([]FilecoinWarmStorageServiceDataSetInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new([]FilecoinWarmStorageServiceDataSetInfoView)).(*[]FilecoinWarmStorageServiceDataSetInfoView)

	return out0, err

}

// GetClientDataSets is a free data retrieval call binding the contract method 0x3208aa1f.
//
// Solidity: function getClientDataSets(address client, uint256 offset, uint256 limit) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256)[] infos)
func (_FWSSView *FWSSViewSession) GetClientDataSets(client common.Address, offset *big.Int, limit *big.Int) ([]FilecoinWarmStorageServiceDataSetInfoView, error) {
	return _FWSSView.Contract.GetClientDataSets(&_FWSSView.CallOpts, client, offset, limit)
}

// GetClientDataSets is a free data retrieval call binding the contract method 0x3208aa1f.
//
// Solidity: function getClientDataSets(address client, uint256 offset, uint256 limit) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256)[] infos)
func (_FWSSView *FWSSViewCallerSession) GetClientDataSets(client common.Address, offset *big.Int, limit *big.Int) ([]FilecoinWarmStorageServiceDataSetInfoView, error) {
	return _FWSSView.Contract.GetClientDataSets(&_FWSSView.CallOpts, client, offset, limit)
}

// GetClientDataSets0 is a free data retrieval call binding the contract method 0x967c6f21.
//
// Solidity: function getClientDataSets(address client) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256)[] infos)
func (_FWSSView *FWSSViewCaller) GetClientDataSets0(opts *bind.CallOpts, client common.Address) ([]FilecoinWarmStorageServiceDataSetInfoView, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getClientDataSets0", client)

	if err != nil {
		return *new([]FilecoinWarmStorageServiceDataSetInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new([]FilecoinWarmStorageServiceDataSetInfoView)).(*[]FilecoinWarmStorageServiceDataSetInfoView)

	return out0, err

}

// GetClientDataSets0 is a free data retrieval call binding the contract method 0x967c6f21.
//
// Solidity: function getClientDataSets(address client) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256)[] infos)
func (_FWSSView *FWSSViewSession) GetClientDataSets0(client common.Address) ([]FilecoinWarmStorageServiceDataSetInfoView, error) {
	return _FWSSView.Contract.GetClientDataSets0(&_FWSSView.CallOpts, client)
}

// GetClientDataSets0 is a free data retrieval call binding the contract method 0x967c6f21.
//
// Solidity: function getClientDataSets(address client) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256)[] infos)
func (_FWSSView *FWSSViewCallerSession) GetClientDataSets0(client common.Address) ([]FilecoinWarmStorageServiceDataSetInfoView, error) {
	return _FWSSView.Contract.GetClientDataSets0(&_FWSSView.CallOpts, client)
}

// GetClientDataSetsLength is a free data retrieval call binding the contract method 0x98a0b04e.
//
// Solidity: function getClientDataSetsLength(address payer) view returns(uint256)
func (_FWSSView *FWSSViewCaller) GetClientDataSetsLength(opts *bind.CallOpts, payer common.Address) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getClientDataSetsLength", payer)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetClientDataSetsLength is a free data retrieval call binding the contract method 0x98a0b04e.
//
// Solidity: function getClientDataSetsLength(address payer) view returns(uint256)
func (_FWSSView *FWSSViewSession) GetClientDataSetsLength(payer common.Address) (*big.Int, error) {
	return _FWSSView.Contract.GetClientDataSetsLength(&_FWSSView.CallOpts, payer)
}

// GetClientDataSetsLength is a free data retrieval call binding the contract method 0x98a0b04e.
//
// Solidity: function getClientDataSetsLength(address payer) view returns(uint256)
func (_FWSSView *FWSSViewCallerSession) GetClientDataSetsLength(payer common.Address) (*big.Int, error) {
	return _FWSSView.Contract.GetClientDataSetsLength(&_FWSSView.CallOpts, payer)
}

// GetCurrentPricingRates is a free data retrieval call binding the contract method 0xb5a578fc.
//
// Solidity: function getCurrentPricingRates() view returns(uint256 storagePrice, uint256 minimumRate)
func (_FWSSView *FWSSViewCaller) GetCurrentPricingRates(opts *bind.CallOpts) (struct {
	StoragePrice *big.Int
	MinimumRate  *big.Int
}, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getCurrentPricingRates")

	outstruct := new(struct {
		StoragePrice *big.Int
		MinimumRate  *big.Int
	})
	if err != nil {
		return *outstruct, err
	}

	outstruct.StoragePrice = *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)
	outstruct.MinimumRate = *abi.ConvertType(out[1], new(*big.Int)).(**big.Int)

	return *outstruct, err

}

// GetCurrentPricingRates is a free data retrieval call binding the contract method 0xb5a578fc.
//
// Solidity: function getCurrentPricingRates() view returns(uint256 storagePrice, uint256 minimumRate)
func (_FWSSView *FWSSViewSession) GetCurrentPricingRates() (struct {
	StoragePrice *big.Int
	MinimumRate  *big.Int
}, error) {
	return _FWSSView.Contract.GetCurrentPricingRates(&_FWSSView.CallOpts)
}

// GetCurrentPricingRates is a free data retrieval call binding the contract method 0xb5a578fc.
//
// Solidity: function getCurrentPricingRates() view returns(uint256 storagePrice, uint256 minimumRate)
func (_FWSSView *FWSSViewCallerSession) GetCurrentPricingRates() (struct {
	StoragePrice *big.Int
	MinimumRate  *big.Int
}, error) {
	return _FWSSView.Contract.GetCurrentPricingRates(&_FWSSView.CallOpts)
}

// GetDataSet is a free data retrieval call binding the contract method 0xbdaac056.
//
// Solidity: function getDataSet(uint256 dataSetId) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256) info)
func (_FWSSView *FWSSViewCaller) GetDataSet(opts *bind.CallOpts, dataSetId *big.Int) (FilecoinWarmStorageServiceDataSetInfoView, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getDataSet", dataSetId)

	if err != nil {
		return *new(FilecoinWarmStorageServiceDataSetInfoView), err
	}

	out0 := *abi.ConvertType(out[0], new(FilecoinWarmStorageServiceDataSetInfoView)).(*FilecoinWarmStorageServiceDataSetInfoView)

	return out0, err

}

// GetDataSet is a free data retrieval call binding the contract method 0xbdaac056.
//
// Solidity: function getDataSet(uint256 dataSetId) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256) info)
func (_FWSSView *FWSSViewSession) GetDataSet(dataSetId *big.Int) (FilecoinWarmStorageServiceDataSetInfoView, error) {
	return _FWSSView.Contract.GetDataSet(&_FWSSView.CallOpts, dataSetId)
}

// GetDataSet is a free data retrieval call binding the contract method 0xbdaac056.
//
// Solidity: function getDataSet(uint256 dataSetId) view returns((uint256,uint256,uint256,address,address,address,uint256,uint256,uint256,uint256,uint256) info)
func (_FWSSView *FWSSViewCallerSession) GetDataSet(dataSetId *big.Int) (FilecoinWarmStorageServiceDataSetInfoView, error) {
	return _FWSSView.Contract.GetDataSet(&_FWSSView.CallOpts, dataSetId)
}

// GetDataSetMetadata is a free data retrieval call binding the contract method 0x4dc17df1.
//
// Solidity: function getDataSetMetadata(uint256 dataSetId, string key) view returns(bool exists, string value)
func (_FWSSView *FWSSViewCaller) GetDataSetMetadata(opts *bind.CallOpts, dataSetId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getDataSetMetadata", dataSetId, key)

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
func (_FWSSView *FWSSViewSession) GetDataSetMetadata(dataSetId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	return _FWSSView.Contract.GetDataSetMetadata(&_FWSSView.CallOpts, dataSetId, key)
}

// GetDataSetMetadata is a free data retrieval call binding the contract method 0x4dc17df1.
//
// Solidity: function getDataSetMetadata(uint256 dataSetId, string key) view returns(bool exists, string value)
func (_FWSSView *FWSSViewCallerSession) GetDataSetMetadata(dataSetId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	return _FWSSView.Contract.GetDataSetMetadata(&_FWSSView.CallOpts, dataSetId, key)
}

// GetDataSetSizeInBytes is a free data retrieval call binding the contract method 0xfe295953.
//
// Solidity: function getDataSetSizeInBytes(uint256 leafCount) pure returns(uint256)
func (_FWSSView *FWSSViewCaller) GetDataSetSizeInBytes(opts *bind.CallOpts, leafCount *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getDataSetSizeInBytes", leafCount)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// GetDataSetSizeInBytes is a free data retrieval call binding the contract method 0xfe295953.
//
// Solidity: function getDataSetSizeInBytes(uint256 leafCount) pure returns(uint256)
func (_FWSSView *FWSSViewSession) GetDataSetSizeInBytes(leafCount *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.GetDataSetSizeInBytes(&_FWSSView.CallOpts, leafCount)
}

// GetDataSetSizeInBytes is a free data retrieval call binding the contract method 0xfe295953.
//
// Solidity: function getDataSetSizeInBytes(uint256 leafCount) pure returns(uint256)
func (_FWSSView *FWSSViewCallerSession) GetDataSetSizeInBytes(leafCount *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.GetDataSetSizeInBytes(&_FWSSView.CallOpts, leafCount)
}

// GetDataSetStatus is a free data retrieval call binding the contract method 0x617285ad.
//
// Solidity: function getDataSetStatus(uint256 dataSetId) view returns(uint8 status)
func (_FWSSView *FWSSViewCaller) GetDataSetStatus(opts *bind.CallOpts, dataSetId *big.Int) (uint8, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getDataSetStatus", dataSetId)

	if err != nil {
		return *new(uint8), err
	}

	out0 := *abi.ConvertType(out[0], new(uint8)).(*uint8)

	return out0, err

}

// GetDataSetStatus is a free data retrieval call binding the contract method 0x617285ad.
//
// Solidity: function getDataSetStatus(uint256 dataSetId) view returns(uint8 status)
func (_FWSSView *FWSSViewSession) GetDataSetStatus(dataSetId *big.Int) (uint8, error) {
	return _FWSSView.Contract.GetDataSetStatus(&_FWSSView.CallOpts, dataSetId)
}

// GetDataSetStatus is a free data retrieval call binding the contract method 0x617285ad.
//
// Solidity: function getDataSetStatus(uint256 dataSetId) view returns(uint8 status)
func (_FWSSView *FWSSViewCallerSession) GetDataSetStatus(dataSetId *big.Int) (uint8, error) {
	return _FWSSView.Contract.GetDataSetStatus(&_FWSSView.CallOpts, dataSetId)
}

// GetPDPConfig is a free data retrieval call binding the contract method 0xea0f9354.
//
// Solidity: function getPDPConfig() view returns(uint64 maxProvingPeriod, uint256 challengeWindowSize, uint256 challengesPerProof, uint256 initChallengeWindowStart)
func (_FWSSView *FWSSViewCaller) GetPDPConfig(opts *bind.CallOpts) (struct {
	MaxProvingPeriod         uint64
	ChallengeWindowSize      *big.Int
	ChallengesPerProof       *big.Int
	InitChallengeWindowStart *big.Int
}, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getPDPConfig")

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
func (_FWSSView *FWSSViewSession) GetPDPConfig() (struct {
	MaxProvingPeriod         uint64
	ChallengeWindowSize      *big.Int
	ChallengesPerProof       *big.Int
	InitChallengeWindowStart *big.Int
}, error) {
	return _FWSSView.Contract.GetPDPConfig(&_FWSSView.CallOpts)
}

// GetPDPConfig is a free data retrieval call binding the contract method 0xea0f9354.
//
// Solidity: function getPDPConfig() view returns(uint64 maxProvingPeriod, uint256 challengeWindowSize, uint256 challengesPerProof, uint256 initChallengeWindowStart)
func (_FWSSView *FWSSViewCallerSession) GetPDPConfig() (struct {
	MaxProvingPeriod         uint64
	ChallengeWindowSize      *big.Int
	ChallengesPerProof       *big.Int
	InitChallengeWindowStart *big.Int
}, error) {
	return _FWSSView.Contract.GetPDPConfig(&_FWSSView.CallOpts)
}

// GetPieceMetadata is a free data retrieval call binding the contract method 0x837a7f49.
//
// Solidity: function getPieceMetadata(uint256 dataSetId, uint256 pieceId, string key) view returns(bool exists, string value)
func (_FWSSView *FWSSViewCaller) GetPieceMetadata(opts *bind.CallOpts, dataSetId *big.Int, pieceId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "getPieceMetadata", dataSetId, pieceId, key)

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

// GetPieceMetadata is a free data retrieval call binding the contract method 0x837a7f49.
//
// Solidity: function getPieceMetadata(uint256 dataSetId, uint256 pieceId, string key) view returns(bool exists, string value)
func (_FWSSView *FWSSViewSession) GetPieceMetadata(dataSetId *big.Int, pieceId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	return _FWSSView.Contract.GetPieceMetadata(&_FWSSView.CallOpts, dataSetId, pieceId, key)
}

// GetPieceMetadata is a free data retrieval call binding the contract method 0x837a7f49.
//
// Solidity: function getPieceMetadata(uint256 dataSetId, uint256 pieceId, string key) view returns(bool exists, string value)
func (_FWSSView *FWSSViewCallerSession) GetPieceMetadata(dataSetId *big.Int, pieceId *big.Int, key string) (struct {
	Exists bool
	Value  string
}, error) {
	return _FWSSView.Contract.GetPieceMetadata(&_FWSSView.CallOpts, dataSetId, pieceId, key)
}

// IsProviderApproved is a free data retrieval call binding the contract method 0xb6133b7a.
//
// Solidity: function isProviderApproved(uint256 providerId) view returns(bool)
func (_FWSSView *FWSSViewCaller) IsProviderApproved(opts *bind.CallOpts, providerId *big.Int) (bool, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "isProviderApproved", providerId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// IsProviderApproved is a free data retrieval call binding the contract method 0xb6133b7a.
//
// Solidity: function isProviderApproved(uint256 providerId) view returns(bool)
func (_FWSSView *FWSSViewSession) IsProviderApproved(providerId *big.Int) (bool, error) {
	return _FWSSView.Contract.IsProviderApproved(&_FWSSView.CallOpts, providerId)
}

// IsProviderApproved is a free data retrieval call binding the contract method 0xb6133b7a.
//
// Solidity: function isProviderApproved(uint256 providerId) view returns(bool)
func (_FWSSView *FWSSViewCallerSession) IsProviderApproved(providerId *big.Int) (bool, error) {
	return _FWSSView.Contract.IsProviderApproved(&_FWSSView.CallOpts, providerId)
}

// NextPDPChallengeWindowStart is a free data retrieval call binding the contract method 0x11d41294.
//
// Solidity: function nextPDPChallengeWindowStart(uint256 setId) view returns(uint256)
func (_FWSSView *FWSSViewCaller) NextPDPChallengeWindowStart(opts *bind.CallOpts, setId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "nextPDPChallengeWindowStart", setId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// NextPDPChallengeWindowStart is a free data retrieval call binding the contract method 0x11d41294.
//
// Solidity: function nextPDPChallengeWindowStart(uint256 setId) view returns(uint256)
func (_FWSSView *FWSSViewSession) NextPDPChallengeWindowStart(setId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.NextPDPChallengeWindowStart(&_FWSSView.CallOpts, setId)
}

// NextPDPChallengeWindowStart is a free data retrieval call binding the contract method 0x11d41294.
//
// Solidity: function nextPDPChallengeWindowStart(uint256 setId) view returns(uint256)
func (_FWSSView *FWSSViewCallerSession) NextPDPChallengeWindowStart(setId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.NextPDPChallengeWindowStart(&_FWSSView.CallOpts, setId)
}

// NextUpgrade is a free data retrieval call binding the contract method 0x315e49ea.
//
// Solidity: function nextUpgrade() view returns(address nextImplementation, uint96 afterEpoch)
func (_FWSSView *FWSSViewCaller) NextUpgrade(opts *bind.CallOpts) (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "nextUpgrade")

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
func (_FWSSView *FWSSViewSession) NextUpgrade() (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	return _FWSSView.Contract.NextUpgrade(&_FWSSView.CallOpts)
}

// NextUpgrade is a free data retrieval call binding the contract method 0x315e49ea.
//
// Solidity: function nextUpgrade() view returns(address nextImplementation, uint96 afterEpoch)
func (_FWSSView *FWSSViewCallerSession) NextUpgrade() (struct {
	NextImplementation common.Address
	AfterEpoch         *big.Int
}, error) {
	return _FWSSView.Contract.NextUpgrade(&_FWSSView.CallOpts)
}

// ProvenPeriods is a free data retrieval call binding the contract method 0x698762cb.
//
// Solidity: function provenPeriods(uint256 dataSetId, uint256 periodId) view returns(bool)
func (_FWSSView *FWSSViewCaller) ProvenPeriods(opts *bind.CallOpts, dataSetId *big.Int, periodId *big.Int) (bool, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "provenPeriods", dataSetId, periodId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ProvenPeriods is a free data retrieval call binding the contract method 0x698762cb.
//
// Solidity: function provenPeriods(uint256 dataSetId, uint256 periodId) view returns(bool)
func (_FWSSView *FWSSViewSession) ProvenPeriods(dataSetId *big.Int, periodId *big.Int) (bool, error) {
	return _FWSSView.Contract.ProvenPeriods(&_FWSSView.CallOpts, dataSetId, periodId)
}

// ProvenPeriods is a free data retrieval call binding the contract method 0x698762cb.
//
// Solidity: function provenPeriods(uint256 dataSetId, uint256 periodId) view returns(bool)
func (_FWSSView *FWSSViewCallerSession) ProvenPeriods(dataSetId *big.Int, periodId *big.Int) (bool, error) {
	return _FWSSView.Contract.ProvenPeriods(&_FWSSView.CallOpts, dataSetId, periodId)
}

// ProvenThisPeriod is a free data retrieval call binding the contract method 0x7598a1cd.
//
// Solidity: function provenThisPeriod(uint256 dataSetId) view returns(bool)
func (_FWSSView *FWSSViewCaller) ProvenThisPeriod(opts *bind.CallOpts, dataSetId *big.Int) (bool, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "provenThisPeriod", dataSetId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ProvenThisPeriod is a free data retrieval call binding the contract method 0x7598a1cd.
//
// Solidity: function provenThisPeriod(uint256 dataSetId) view returns(bool)
func (_FWSSView *FWSSViewSession) ProvenThisPeriod(dataSetId *big.Int) (bool, error) {
	return _FWSSView.Contract.ProvenThisPeriod(&_FWSSView.CallOpts, dataSetId)
}

// ProvenThisPeriod is a free data retrieval call binding the contract method 0x7598a1cd.
//
// Solidity: function provenThisPeriod(uint256 dataSetId) view returns(bool)
func (_FWSSView *FWSSViewCallerSession) ProvenThisPeriod(dataSetId *big.Int) (bool, error) {
	return _FWSSView.Contract.ProvenThisPeriod(&_FWSSView.CallOpts, dataSetId)
}

// ProvingActivationEpoch is a free data retrieval call binding the contract method 0x725e3216.
//
// Solidity: function provingActivationEpoch(uint256 dataSetId) view returns(uint256)
func (_FWSSView *FWSSViewCaller) ProvingActivationEpoch(opts *bind.CallOpts, dataSetId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "provingActivationEpoch", dataSetId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ProvingActivationEpoch is a free data retrieval call binding the contract method 0x725e3216.
//
// Solidity: function provingActivationEpoch(uint256 dataSetId) view returns(uint256)
func (_FWSSView *FWSSViewSession) ProvingActivationEpoch(dataSetId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.ProvingActivationEpoch(&_FWSSView.CallOpts, dataSetId)
}

// ProvingActivationEpoch is a free data retrieval call binding the contract method 0x725e3216.
//
// Solidity: function provingActivationEpoch(uint256 dataSetId) view returns(uint256)
func (_FWSSView *FWSSViewCallerSession) ProvingActivationEpoch(dataSetId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.ProvingActivationEpoch(&_FWSSView.CallOpts, dataSetId)
}

// ProvingDeadline is a free data retrieval call binding the contract method 0x149ac5cc.
//
// Solidity: function provingDeadline(uint256 setId) view returns(uint256)
func (_FWSSView *FWSSViewCaller) ProvingDeadline(opts *bind.CallOpts, setId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "provingDeadline", setId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ProvingDeadline is a free data retrieval call binding the contract method 0x149ac5cc.
//
// Solidity: function provingDeadline(uint256 setId) view returns(uint256)
func (_FWSSView *FWSSViewSession) ProvingDeadline(setId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.ProvingDeadline(&_FWSSView.CallOpts, setId)
}

// ProvingDeadline is a free data retrieval call binding the contract method 0x149ac5cc.
//
// Solidity: function provingDeadline(uint256 setId) view returns(uint256)
func (_FWSSView *FWSSViewCallerSession) ProvingDeadline(setId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.ProvingDeadline(&_FWSSView.CallOpts, setId)
}

// RailToDataSet is a free data retrieval call binding the contract method 0x2ad6e6b5.
//
// Solidity: function railToDataSet(uint256 railId) view returns(uint256)
func (_FWSSView *FWSSViewCaller) RailToDataSet(opts *bind.CallOpts, railId *big.Int) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "railToDataSet", railId)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// RailToDataSet is a free data retrieval call binding the contract method 0x2ad6e6b5.
//
// Solidity: function railToDataSet(uint256 railId) view returns(uint256)
func (_FWSSView *FWSSViewSession) RailToDataSet(railId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.RailToDataSet(&_FWSSView.CallOpts, railId)
}

// RailToDataSet is a free data retrieval call binding the contract method 0x2ad6e6b5.
//
// Solidity: function railToDataSet(uint256 railId) view returns(uint256)
func (_FWSSView *FWSSViewCallerSession) RailToDataSet(railId *big.Int) (*big.Int, error) {
	return _FWSSView.Contract.RailToDataSet(&_FWSSView.CallOpts, railId)
}

// Service is a free data retrieval call binding the contract method 0xd598d4c9.
//
// Solidity: function service() view returns(address)
func (_FWSSView *FWSSViewCaller) Service(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "service")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Service is a free data retrieval call binding the contract method 0xd598d4c9.
//
// Solidity: function service() view returns(address)
func (_FWSSView *FWSSViewSession) Service() (common.Address, error) {
	return _FWSSView.Contract.Service(&_FWSSView.CallOpts)
}

// Service is a free data retrieval call binding the contract method 0xd598d4c9.
//
// Solidity: function service() view returns(address)
func (_FWSSView *FWSSViewCallerSession) Service() (common.Address, error) {
	return _FWSSView.Contract.Service(&_FWSSView.CallOpts)
}

// ServiceCommissionBps is a free data retrieval call binding the contract method 0x2afcc1a4.
//
// Solidity: function serviceCommissionBps() view returns(uint256)
func (_FWSSView *FWSSViewCaller) ServiceCommissionBps(opts *bind.CallOpts) (*big.Int, error) {
	var out []interface{}
	err := _FWSSView.contract.Call(opts, &out, "serviceCommissionBps")

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// ServiceCommissionBps is a free data retrieval call binding the contract method 0x2afcc1a4.
//
// Solidity: function serviceCommissionBps() view returns(uint256)
func (_FWSSView *FWSSViewSession) ServiceCommissionBps() (*big.Int, error) {
	return _FWSSView.Contract.ServiceCommissionBps(&_FWSSView.CallOpts)
}

// ServiceCommissionBps is a free data retrieval call binding the contract method 0x2afcc1a4.
//
// Solidity: function serviceCommissionBps() view returns(uint256)
func (_FWSSView *FWSSViewCallerSession) ServiceCommissionBps() (*big.Int, error) {
	return _FWSSView.Contract.ServiceCommissionBps(&_FWSSView.CallOpts)
}
