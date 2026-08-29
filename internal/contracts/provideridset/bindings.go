// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package provideridset

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

// ProviderIDSetMetaData contains all meta data concerning the ProviderIDSet contract.
var ProviderIDSetMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"constructor\",\"inputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"addProviderId\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"containsProviderId\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[{\"name\":\"\",\"type\":\"bool\",\"internalType\":\"bool\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"getProviderIds\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"uint256[]\",\"internalType\":\"uint256[]\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"owner\",\"inputs\":[],\"outputs\":[{\"name\":\"\",\"type\":\"address\",\"internalType\":\"address\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"removeProviderId\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"renounceOwnership\",\"inputs\":[],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"transferOwnership\",\"inputs\":[{\"name\":\"newOwner\",\"type\":\"address\",\"internalType\":\"address\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"OwnershipTransferred\",\"inputs\":[{\"name\":\"previousOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"newOwner\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"}],\"anonymous\":false},{\"type\":\"error\",\"name\":\"OwnableInvalidOwner\",\"inputs\":[{\"name\":\"owner\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"OwnableUnauthorizedAccount\",\"inputs\":[{\"name\":\"account\",\"type\":\"address\",\"internalType\":\"address\"}]},{\"type\":\"error\",\"name\":\"ProviderIdNotFound\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]},{\"type\":\"error\",\"name\":\"ProviderIdTooLarge\",\"inputs\":[{\"name\":\"providerId\",\"type\":\"uint256\",\"internalType\":\"uint256\"}]}]",
}

// ProviderIDSetABI is the input ABI used to generate the binding from.
// Deprecated: Use ProviderIDSetMetaData.ABI instead.
var ProviderIDSetABI = ProviderIDSetMetaData.ABI

// ProviderIDSet is an auto generated Go binding around an Ethereum contract.
type ProviderIDSet struct {
	ProviderIDSetCaller     // Read-only binding to the contract
	ProviderIDSetTransactor // Write-only binding to the contract
	ProviderIDSetFilterer   // Log filterer for contract events
}

// ProviderIDSetCaller is an auto generated read-only Go binding around an Ethereum contract.
type ProviderIDSetCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ProviderIDSetTransactor is an auto generated write-only Go binding around an Ethereum contract.
type ProviderIDSetTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ProviderIDSetFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type ProviderIDSetFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// ProviderIDSetSession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type ProviderIDSetSession struct {
	Contract     *ProviderIDSet    // Generic contract binding to set the session for
	CallOpts     bind.CallOpts     // Call options to use throughout this session
	TransactOpts bind.TransactOpts // Transaction auth options to use throughout this session
}

// ProviderIDSetCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type ProviderIDSetCallerSession struct {
	Contract *ProviderIDSetCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts        // Call options to use throughout this session
}

// ProviderIDSetTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type ProviderIDSetTransactorSession struct {
	Contract     *ProviderIDSetTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts        // Transaction auth options to use throughout this session
}

// ProviderIDSetRaw is an auto generated low-level Go binding around an Ethereum contract.
type ProviderIDSetRaw struct {
	Contract *ProviderIDSet // Generic contract binding to access the raw methods on
}

// ProviderIDSetCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type ProviderIDSetCallerRaw struct {
	Contract *ProviderIDSetCaller // Generic read-only contract binding to access the raw methods on
}

// ProviderIDSetTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type ProviderIDSetTransactorRaw struct {
	Contract *ProviderIDSetTransactor // Generic write-only contract binding to access the raw methods on
}

// NewProviderIDSet creates a new instance of ProviderIDSet, bound to a specific deployed contract.
func NewProviderIDSet(address common.Address, backend bind.ContractBackend) (*ProviderIDSet, error) {
	contract, err := bindProviderIDSet(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &ProviderIDSet{ProviderIDSetCaller: ProviderIDSetCaller{contract: contract}, ProviderIDSetTransactor: ProviderIDSetTransactor{contract: contract}, ProviderIDSetFilterer: ProviderIDSetFilterer{contract: contract}}, nil
}

// NewProviderIDSetCaller creates a new read-only instance of ProviderIDSet, bound to a specific deployed contract.
func NewProviderIDSetCaller(address common.Address, caller bind.ContractCaller) (*ProviderIDSetCaller, error) {
	contract, err := bindProviderIDSet(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &ProviderIDSetCaller{contract: contract}, nil
}

// NewProviderIDSetTransactor creates a new write-only instance of ProviderIDSet, bound to a specific deployed contract.
func NewProviderIDSetTransactor(address common.Address, transactor bind.ContractTransactor) (*ProviderIDSetTransactor, error) {
	contract, err := bindProviderIDSet(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &ProviderIDSetTransactor{contract: contract}, nil
}

// NewProviderIDSetFilterer creates a new log filterer instance of ProviderIDSet, bound to a specific deployed contract.
func NewProviderIDSetFilterer(address common.Address, filterer bind.ContractFilterer) (*ProviderIDSetFilterer, error) {
	contract, err := bindProviderIDSet(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &ProviderIDSetFilterer{contract: contract}, nil
}

// bindProviderIDSet binds a generic wrapper to an already deployed contract.
func bindProviderIDSet(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := ProviderIDSetMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ProviderIDSet *ProviderIDSetRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ProviderIDSet.Contract.ProviderIDSetCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ProviderIDSet *ProviderIDSetRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.ProviderIDSetTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ProviderIDSet *ProviderIDSetRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.ProviderIDSetTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_ProviderIDSet *ProviderIDSetCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _ProviderIDSet.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_ProviderIDSet *ProviderIDSetTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_ProviderIDSet *ProviderIDSetTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.contract.Transact(opts, method, params...)
}

// ContainsProviderId is a free data retrieval call binding the contract method 0x131d6f8c.
//
// Solidity: function containsProviderId(uint256 providerId) view returns(bool)
func (_ProviderIDSet *ProviderIDSetCaller) ContainsProviderId(opts *bind.CallOpts, providerId *big.Int) (bool, error) {
	var out []interface{}
	err := _ProviderIDSet.contract.Call(opts, &out, "containsProviderId", providerId)

	if err != nil {
		return *new(bool), err
	}

	out0 := *abi.ConvertType(out[0], new(bool)).(*bool)

	return out0, err

}

// ContainsProviderId is a free data retrieval call binding the contract method 0x131d6f8c.
//
// Solidity: function containsProviderId(uint256 providerId) view returns(bool)
func (_ProviderIDSet *ProviderIDSetSession) ContainsProviderId(providerId *big.Int) (bool, error) {
	return _ProviderIDSet.Contract.ContainsProviderId(&_ProviderIDSet.CallOpts, providerId)
}

// ContainsProviderId is a free data retrieval call binding the contract method 0x131d6f8c.
//
// Solidity: function containsProviderId(uint256 providerId) view returns(bool)
func (_ProviderIDSet *ProviderIDSetCallerSession) ContainsProviderId(providerId *big.Int) (bool, error) {
	return _ProviderIDSet.Contract.ContainsProviderId(&_ProviderIDSet.CallOpts, providerId)
}

// GetProviderIds is a free data retrieval call binding the contract method 0x0a9cb4a7.
//
// Solidity: function getProviderIds() view returns(uint256[])
func (_ProviderIDSet *ProviderIDSetCaller) GetProviderIds(opts *bind.CallOpts) ([]*big.Int, error) {
	var out []interface{}
	err := _ProviderIDSet.contract.Call(opts, &out, "getProviderIds")

	if err != nil {
		return *new([]*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new([]*big.Int)).(*[]*big.Int)

	return out0, err

}

// GetProviderIds is a free data retrieval call binding the contract method 0x0a9cb4a7.
//
// Solidity: function getProviderIds() view returns(uint256[])
func (_ProviderIDSet *ProviderIDSetSession) GetProviderIds() ([]*big.Int, error) {
	return _ProviderIDSet.Contract.GetProviderIds(&_ProviderIDSet.CallOpts)
}

// GetProviderIds is a free data retrieval call binding the contract method 0x0a9cb4a7.
//
// Solidity: function getProviderIds() view returns(uint256[])
func (_ProviderIDSet *ProviderIDSetCallerSession) GetProviderIds() ([]*big.Int, error) {
	return _ProviderIDSet.Contract.GetProviderIds(&_ProviderIDSet.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ProviderIDSet *ProviderIDSetCaller) Owner(opts *bind.CallOpts) (common.Address, error) {
	var out []interface{}
	err := _ProviderIDSet.contract.Call(opts, &out, "owner")

	if err != nil {
		return *new(common.Address), err
	}

	out0 := *abi.ConvertType(out[0], new(common.Address)).(*common.Address)

	return out0, err

}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ProviderIDSet *ProviderIDSetSession) Owner() (common.Address, error) {
	return _ProviderIDSet.Contract.Owner(&_ProviderIDSet.CallOpts)
}

// Owner is a free data retrieval call binding the contract method 0x8da5cb5b.
//
// Solidity: function owner() view returns(address)
func (_ProviderIDSet *ProviderIDSetCallerSession) Owner() (common.Address, error) {
	return _ProviderIDSet.Contract.Owner(&_ProviderIDSet.CallOpts)
}

// AddProviderId is a paid mutator transaction binding the contract method 0xafa0cfd5.
//
// Solidity: function addProviderId(uint256 providerId) returns()
func (_ProviderIDSet *ProviderIDSetTransactor) AddProviderId(opts *bind.TransactOpts, providerId *big.Int) (*types.Transaction, error) {
	return _ProviderIDSet.contract.Transact(opts, "addProviderId", providerId)
}

// AddProviderId is a paid mutator transaction binding the contract method 0xafa0cfd5.
//
// Solidity: function addProviderId(uint256 providerId) returns()
func (_ProviderIDSet *ProviderIDSetSession) AddProviderId(providerId *big.Int) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.AddProviderId(&_ProviderIDSet.TransactOpts, providerId)
}

// AddProviderId is a paid mutator transaction binding the contract method 0xafa0cfd5.
//
// Solidity: function addProviderId(uint256 providerId) returns()
func (_ProviderIDSet *ProviderIDSetTransactorSession) AddProviderId(providerId *big.Int) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.AddProviderId(&_ProviderIDSet.TransactOpts, providerId)
}

// RemoveProviderId is a paid mutator transaction binding the contract method 0xdd328bf9.
//
// Solidity: function removeProviderId(uint256 providerId) returns()
func (_ProviderIDSet *ProviderIDSetTransactor) RemoveProviderId(opts *bind.TransactOpts, providerId *big.Int) (*types.Transaction, error) {
	return _ProviderIDSet.contract.Transact(opts, "removeProviderId", providerId)
}

// RemoveProviderId is a paid mutator transaction binding the contract method 0xdd328bf9.
//
// Solidity: function removeProviderId(uint256 providerId) returns()
func (_ProviderIDSet *ProviderIDSetSession) RemoveProviderId(providerId *big.Int) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.RemoveProviderId(&_ProviderIDSet.TransactOpts, providerId)
}

// RemoveProviderId is a paid mutator transaction binding the contract method 0xdd328bf9.
//
// Solidity: function removeProviderId(uint256 providerId) returns()
func (_ProviderIDSet *ProviderIDSetTransactorSession) RemoveProviderId(providerId *big.Int) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.RemoveProviderId(&_ProviderIDSet.TransactOpts, providerId)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ProviderIDSet *ProviderIDSetTransactor) RenounceOwnership(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _ProviderIDSet.contract.Transact(opts, "renounceOwnership")
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ProviderIDSet *ProviderIDSetSession) RenounceOwnership() (*types.Transaction, error) {
	return _ProviderIDSet.Contract.RenounceOwnership(&_ProviderIDSet.TransactOpts)
}

// RenounceOwnership is a paid mutator transaction binding the contract method 0x715018a6.
//
// Solidity: function renounceOwnership() returns()
func (_ProviderIDSet *ProviderIDSetTransactorSession) RenounceOwnership() (*types.Transaction, error) {
	return _ProviderIDSet.Contract.RenounceOwnership(&_ProviderIDSet.TransactOpts)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ProviderIDSet *ProviderIDSetTransactor) TransferOwnership(opts *bind.TransactOpts, newOwner common.Address) (*types.Transaction, error) {
	return _ProviderIDSet.contract.Transact(opts, "transferOwnership", newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ProviderIDSet *ProviderIDSetSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.TransferOwnership(&_ProviderIDSet.TransactOpts, newOwner)
}

// TransferOwnership is a paid mutator transaction binding the contract method 0xf2fde38b.
//
// Solidity: function transferOwnership(address newOwner) returns()
func (_ProviderIDSet *ProviderIDSetTransactorSession) TransferOwnership(newOwner common.Address) (*types.Transaction, error) {
	return _ProviderIDSet.Contract.TransferOwnership(&_ProviderIDSet.TransactOpts, newOwner)
}

// ProviderIDSetOwnershipTransferredIterator is returned from FilterOwnershipTransferred and is used to iterate over the raw logs and unpacked data for OwnershipTransferred events raised by the ProviderIDSet contract.
type ProviderIDSetOwnershipTransferredIterator struct {
	Event *ProviderIDSetOwnershipTransferred // Event containing the contract specifics and raw log

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
func (it *ProviderIDSetOwnershipTransferredIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(ProviderIDSetOwnershipTransferred)
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
		it.Event = new(ProviderIDSetOwnershipTransferred)
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
func (it *ProviderIDSetOwnershipTransferredIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *ProviderIDSetOwnershipTransferredIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// ProviderIDSetOwnershipTransferred represents a OwnershipTransferred event raised by the ProviderIDSet contract.
type ProviderIDSetOwnershipTransferred struct {
	PreviousOwner common.Address
	NewOwner      common.Address
	Raw           types.Log // Blockchain specific contextual infos
}

// FilterOwnershipTransferred is a free log retrieval operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_ProviderIDSet *ProviderIDSetFilterer) FilterOwnershipTransferred(opts *bind.FilterOpts, previousOwner []common.Address, newOwner []common.Address) (*ProviderIDSetOwnershipTransferredIterator, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _ProviderIDSet.contract.FilterLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return &ProviderIDSetOwnershipTransferredIterator{contract: _ProviderIDSet.contract, event: "OwnershipTransferred", logs: logs, sub: sub}, nil
}

// WatchOwnershipTransferred is a free log subscription operation binding the contract event 0x8be0079c531659141344cd1fd0a4f28419497f9722a3daafe3b4186f6b6457e0.
//
// Solidity: event OwnershipTransferred(address indexed previousOwner, address indexed newOwner)
func (_ProviderIDSet *ProviderIDSetFilterer) WatchOwnershipTransferred(opts *bind.WatchOpts, sink chan<- *ProviderIDSetOwnershipTransferred, previousOwner []common.Address, newOwner []common.Address) (event.Subscription, error) {

	var previousOwnerRule []interface{}
	for _, previousOwnerItem := range previousOwner {
		previousOwnerRule = append(previousOwnerRule, previousOwnerItem)
	}
	var newOwnerRule []interface{}
	for _, newOwnerItem := range newOwner {
		newOwnerRule = append(newOwnerRule, newOwnerItem)
	}

	logs, sub, err := _ProviderIDSet.contract.WatchLogs(opts, "OwnershipTransferred", previousOwnerRule, newOwnerRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(ProviderIDSetOwnershipTransferred)
				if err := _ProviderIDSet.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
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
func (_ProviderIDSet *ProviderIDSetFilterer) ParseOwnershipTransferred(log types.Log) (*ProviderIDSetOwnershipTransferred, error) {
	event := new(ProviderIDSetOwnershipTransferred)
	if err := _ProviderIDSet.contract.UnpackLog(event, "OwnershipTransferred", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
