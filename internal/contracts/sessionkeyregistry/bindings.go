// Code generated - DO NOT EDIT.
// This file is a generated binding and any manual changes will be lost.

package sessionkeyregistry

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

// SessionKeyRegistryMetaData contains all meta data concerning the SessionKeyRegistry contract.
var SessionKeyRegistryMetaData = &bind.MetaData{
	ABI: "[{\"type\":\"function\",\"name\":\"authorizationExpiry\",\"inputs\":[{\"name\":\"user\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"permission\",\"type\":\"bytes32\",\"internalType\":\"bytes32\"}],\"outputs\":[{\"name\":\"\",\"type\":\"uint256\",\"internalType\":\"uint256\"}],\"stateMutability\":\"view\"},{\"type\":\"function\",\"name\":\"login\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"expiry\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"permissions\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"origin\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"function\",\"name\":\"loginAndFund\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"addresspayable\"},{\"name\":\"expiry\",\"type\":\"uint256\",\"internalType\":\"uint256\"},{\"name\":\"permissions\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"origin\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[],\"stateMutability\":\"payable\"},{\"type\":\"function\",\"name\":\"revoke\",\"inputs\":[{\"name\":\"signer\",\"type\":\"address\",\"internalType\":\"address\"},{\"name\":\"permissions\",\"type\":\"bytes32[]\",\"internalType\":\"bytes32[]\"},{\"name\":\"origin\",\"type\":\"string\",\"internalType\":\"string\"}],\"outputs\":[],\"stateMutability\":\"nonpayable\"},{\"type\":\"event\",\"name\":\"AuthorizationsUpdated\",\"inputs\":[{\"name\":\"identity\",\"type\":\"address\",\"indexed\":true,\"internalType\":\"address\"},{\"name\":\"signer\",\"type\":\"address\",\"indexed\":false,\"internalType\":\"address\"},{\"name\":\"expiry\",\"type\":\"uint256\",\"indexed\":false,\"internalType\":\"uint256\"},{\"name\":\"permissions\",\"type\":\"bytes32[]\",\"indexed\":false,\"internalType\":\"bytes32[]\"},{\"name\":\"origin\",\"type\":\"string\",\"indexed\":false,\"internalType\":\"string\"}],\"anonymous\":false}]",
}

// SessionKeyRegistryABI is the input ABI used to generate the binding from.
// Deprecated: Use SessionKeyRegistryMetaData.ABI instead.
var SessionKeyRegistryABI = SessionKeyRegistryMetaData.ABI

// SessionKeyRegistry is an auto generated Go binding around an Ethereum contract.
type SessionKeyRegistry struct {
	SessionKeyRegistryCaller     // Read-only binding to the contract
	SessionKeyRegistryTransactor // Write-only binding to the contract
	SessionKeyRegistryFilterer   // Log filterer for contract events
}

// SessionKeyRegistryCaller is an auto generated read-only Go binding around an Ethereum contract.
type SessionKeyRegistryCaller struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SessionKeyRegistryTransactor is an auto generated write-only Go binding around an Ethereum contract.
type SessionKeyRegistryTransactor struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SessionKeyRegistryFilterer is an auto generated log filtering Go binding around an Ethereum contract events.
type SessionKeyRegistryFilterer struct {
	contract *bind.BoundContract // Generic contract wrapper for the low level calls
}

// SessionKeyRegistrySession is an auto generated Go binding around an Ethereum contract,
// with pre-set call and transact options.
type SessionKeyRegistrySession struct {
	Contract     *SessionKeyRegistry // Generic contract binding to set the session for
	CallOpts     bind.CallOpts       // Call options to use throughout this session
	TransactOpts bind.TransactOpts   // Transaction auth options to use throughout this session
}

// SessionKeyRegistryCallerSession is an auto generated read-only Go binding around an Ethereum contract,
// with pre-set call options.
type SessionKeyRegistryCallerSession struct {
	Contract *SessionKeyRegistryCaller // Generic contract caller binding to set the session for
	CallOpts bind.CallOpts             // Call options to use throughout this session
}

// SessionKeyRegistryTransactorSession is an auto generated write-only Go binding around an Ethereum contract,
// with pre-set transact options.
type SessionKeyRegistryTransactorSession struct {
	Contract     *SessionKeyRegistryTransactor // Generic contract transactor binding to set the session for
	TransactOpts bind.TransactOpts             // Transaction auth options to use throughout this session
}

// SessionKeyRegistryRaw is an auto generated low-level Go binding around an Ethereum contract.
type SessionKeyRegistryRaw struct {
	Contract *SessionKeyRegistry // Generic contract binding to access the raw methods on
}

// SessionKeyRegistryCallerRaw is an auto generated low-level read-only Go binding around an Ethereum contract.
type SessionKeyRegistryCallerRaw struct {
	Contract *SessionKeyRegistryCaller // Generic read-only contract binding to access the raw methods on
}

// SessionKeyRegistryTransactorRaw is an auto generated low-level write-only Go binding around an Ethereum contract.
type SessionKeyRegistryTransactorRaw struct {
	Contract *SessionKeyRegistryTransactor // Generic write-only contract binding to access the raw methods on
}

// NewSessionKeyRegistry creates a new instance of SessionKeyRegistry, bound to a specific deployed contract.
func NewSessionKeyRegistry(address common.Address, backend bind.ContractBackend) (*SessionKeyRegistry, error) {
	contract, err := bindSessionKeyRegistry(address, backend, backend, backend)
	if err != nil {
		return nil, err
	}
	return &SessionKeyRegistry{SessionKeyRegistryCaller: SessionKeyRegistryCaller{contract: contract}, SessionKeyRegistryTransactor: SessionKeyRegistryTransactor{contract: contract}, SessionKeyRegistryFilterer: SessionKeyRegistryFilterer{contract: contract}}, nil
}

// NewSessionKeyRegistryCaller creates a new read-only instance of SessionKeyRegistry, bound to a specific deployed contract.
func NewSessionKeyRegistryCaller(address common.Address, caller bind.ContractCaller) (*SessionKeyRegistryCaller, error) {
	contract, err := bindSessionKeyRegistry(address, caller, nil, nil)
	if err != nil {
		return nil, err
	}
	return &SessionKeyRegistryCaller{contract: contract}, nil
}

// NewSessionKeyRegistryTransactor creates a new write-only instance of SessionKeyRegistry, bound to a specific deployed contract.
func NewSessionKeyRegistryTransactor(address common.Address, transactor bind.ContractTransactor) (*SessionKeyRegistryTransactor, error) {
	contract, err := bindSessionKeyRegistry(address, nil, transactor, nil)
	if err != nil {
		return nil, err
	}
	return &SessionKeyRegistryTransactor{contract: contract}, nil
}

// NewSessionKeyRegistryFilterer creates a new log filterer instance of SessionKeyRegistry, bound to a specific deployed contract.
func NewSessionKeyRegistryFilterer(address common.Address, filterer bind.ContractFilterer) (*SessionKeyRegistryFilterer, error) {
	contract, err := bindSessionKeyRegistry(address, nil, nil, filterer)
	if err != nil {
		return nil, err
	}
	return &SessionKeyRegistryFilterer{contract: contract}, nil
}

// bindSessionKeyRegistry binds a generic wrapper to an already deployed contract.
func bindSessionKeyRegistry(address common.Address, caller bind.ContractCaller, transactor bind.ContractTransactor, filterer bind.ContractFilterer) (*bind.BoundContract, error) {
	parsed, err := SessionKeyRegistryMetaData.GetAbi()
	if err != nil {
		return nil, err
	}
	return bind.NewBoundContract(address, *parsed, caller, transactor, filterer), nil
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SessionKeyRegistry *SessionKeyRegistryRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SessionKeyRegistry.Contract.SessionKeyRegistryCaller.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SessionKeyRegistry *SessionKeyRegistryRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.SessionKeyRegistryTransactor.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SessionKeyRegistry *SessionKeyRegistryRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.SessionKeyRegistryTransactor.contract.Transact(opts, method, params...)
}

// Call invokes the (constant) contract method with params as input values and
// sets the output to result. The result type might be a single field for simple
// returns, a slice of interfaces for anonymous returns and a struct for named
// returns.
func (_SessionKeyRegistry *SessionKeyRegistryCallerRaw) Call(opts *bind.CallOpts, result *[]interface{}, method string, params ...interface{}) error {
	return _SessionKeyRegistry.Contract.contract.Call(opts, result, method, params...)
}

// Transfer initiates a plain transaction to move funds to the contract, calling
// its default method if one is available.
func (_SessionKeyRegistry *SessionKeyRegistryTransactorRaw) Transfer(opts *bind.TransactOpts) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.contract.Transfer(opts)
}

// Transact invokes the (paid) contract method with params as input values.
func (_SessionKeyRegistry *SessionKeyRegistryTransactorRaw) Transact(opts *bind.TransactOpts, method string, params ...interface{}) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.contract.Transact(opts, method, params...)
}

// AuthorizationExpiry is a free data retrieval call binding the contract method 0x9501b2cc.
//
// Solidity: function authorizationExpiry(address user, address signer, bytes32 permission) view returns(uint256)
func (_SessionKeyRegistry *SessionKeyRegistryCaller) AuthorizationExpiry(opts *bind.CallOpts, user common.Address, signer common.Address, permission [32]byte) (*big.Int, error) {
	var out []interface{}
	err := _SessionKeyRegistry.contract.Call(opts, &out, "authorizationExpiry", user, signer, permission)

	if err != nil {
		return *new(*big.Int), err
	}

	out0 := *abi.ConvertType(out[0], new(*big.Int)).(**big.Int)

	return out0, err

}

// AuthorizationExpiry is a free data retrieval call binding the contract method 0x9501b2cc.
//
// Solidity: function authorizationExpiry(address user, address signer, bytes32 permission) view returns(uint256)
func (_SessionKeyRegistry *SessionKeyRegistrySession) AuthorizationExpiry(user common.Address, signer common.Address, permission [32]byte) (*big.Int, error) {
	return _SessionKeyRegistry.Contract.AuthorizationExpiry(&_SessionKeyRegistry.CallOpts, user, signer, permission)
}

// AuthorizationExpiry is a free data retrieval call binding the contract method 0x9501b2cc.
//
// Solidity: function authorizationExpiry(address user, address signer, bytes32 permission) view returns(uint256)
func (_SessionKeyRegistry *SessionKeyRegistryCallerSession) AuthorizationExpiry(user common.Address, signer common.Address, permission [32]byte) (*big.Int, error) {
	return _SessionKeyRegistry.Contract.AuthorizationExpiry(&_SessionKeyRegistry.CallOpts, user, signer, permission)
}

// Login is a paid mutator transaction binding the contract method 0x0848f33f.
//
// Solidity: function login(address signer, uint256 expiry, bytes32[] permissions, string origin) returns()
func (_SessionKeyRegistry *SessionKeyRegistryTransactor) Login(opts *bind.TransactOpts, signer common.Address, expiry *big.Int, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.contract.Transact(opts, "login", signer, expiry, permissions, origin)
}

// Login is a paid mutator transaction binding the contract method 0x0848f33f.
//
// Solidity: function login(address signer, uint256 expiry, bytes32[] permissions, string origin) returns()
func (_SessionKeyRegistry *SessionKeyRegistrySession) Login(signer common.Address, expiry *big.Int, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.Login(&_SessionKeyRegistry.TransactOpts, signer, expiry, permissions, origin)
}

// Login is a paid mutator transaction binding the contract method 0x0848f33f.
//
// Solidity: function login(address signer, uint256 expiry, bytes32[] permissions, string origin) returns()
func (_SessionKeyRegistry *SessionKeyRegistryTransactorSession) Login(signer common.Address, expiry *big.Int, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.Login(&_SessionKeyRegistry.TransactOpts, signer, expiry, permissions, origin)
}

// LoginAndFund is a paid mutator transaction binding the contract method 0xb318f2e2.
//
// Solidity: function loginAndFund(address signer, uint256 expiry, bytes32[] permissions, string origin) payable returns()
func (_SessionKeyRegistry *SessionKeyRegistryTransactor) LoginAndFund(opts *bind.TransactOpts, signer common.Address, expiry *big.Int, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.contract.Transact(opts, "loginAndFund", signer, expiry, permissions, origin)
}

// LoginAndFund is a paid mutator transaction binding the contract method 0xb318f2e2.
//
// Solidity: function loginAndFund(address signer, uint256 expiry, bytes32[] permissions, string origin) payable returns()
func (_SessionKeyRegistry *SessionKeyRegistrySession) LoginAndFund(signer common.Address, expiry *big.Int, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.LoginAndFund(&_SessionKeyRegistry.TransactOpts, signer, expiry, permissions, origin)
}

// LoginAndFund is a paid mutator transaction binding the contract method 0xb318f2e2.
//
// Solidity: function loginAndFund(address signer, uint256 expiry, bytes32[] permissions, string origin) payable returns()
func (_SessionKeyRegistry *SessionKeyRegistryTransactorSession) LoginAndFund(signer common.Address, expiry *big.Int, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.LoginAndFund(&_SessionKeyRegistry.TransactOpts, signer, expiry, permissions, origin)
}

// Revoke is a paid mutator transaction binding the contract method 0xfd89202e.
//
// Solidity: function revoke(address signer, bytes32[] permissions, string origin) returns()
func (_SessionKeyRegistry *SessionKeyRegistryTransactor) Revoke(opts *bind.TransactOpts, signer common.Address, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.contract.Transact(opts, "revoke", signer, permissions, origin)
}

// Revoke is a paid mutator transaction binding the contract method 0xfd89202e.
//
// Solidity: function revoke(address signer, bytes32[] permissions, string origin) returns()
func (_SessionKeyRegistry *SessionKeyRegistrySession) Revoke(signer common.Address, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.Revoke(&_SessionKeyRegistry.TransactOpts, signer, permissions, origin)
}

// Revoke is a paid mutator transaction binding the contract method 0xfd89202e.
//
// Solidity: function revoke(address signer, bytes32[] permissions, string origin) returns()
func (_SessionKeyRegistry *SessionKeyRegistryTransactorSession) Revoke(signer common.Address, permissions [][32]byte, origin string) (*types.Transaction, error) {
	return _SessionKeyRegistry.Contract.Revoke(&_SessionKeyRegistry.TransactOpts, signer, permissions, origin)
}

// SessionKeyRegistryAuthorizationsUpdatedIterator is returned from FilterAuthorizationsUpdated and is used to iterate over the raw logs and unpacked data for AuthorizationsUpdated events raised by the SessionKeyRegistry contract.
type SessionKeyRegistryAuthorizationsUpdatedIterator struct {
	Event *SessionKeyRegistryAuthorizationsUpdated // Event containing the contract specifics and raw log

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
func (it *SessionKeyRegistryAuthorizationsUpdatedIterator) Next() bool {
	// If the iterator failed, stop iterating
	if it.fail != nil {
		return false
	}
	// If the iterator completed, deliver directly whatever's available
	if it.done {
		select {
		case log := <-it.logs:
			it.Event = new(SessionKeyRegistryAuthorizationsUpdated)
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
		it.Event = new(SessionKeyRegistryAuthorizationsUpdated)
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
func (it *SessionKeyRegistryAuthorizationsUpdatedIterator) Error() error {
	return it.fail
}

// Close terminates the iteration process, releasing any pending underlying
// resources.
func (it *SessionKeyRegistryAuthorizationsUpdatedIterator) Close() error {
	it.sub.Unsubscribe()
	return nil
}

// SessionKeyRegistryAuthorizationsUpdated represents a AuthorizationsUpdated event raised by the SessionKeyRegistry contract.
type SessionKeyRegistryAuthorizationsUpdated struct {
	Identity    common.Address
	Signer      common.Address
	Expiry      *big.Int
	Permissions [][32]byte
	Origin      string
	Raw         types.Log // Blockchain specific contextual infos
}

// FilterAuthorizationsUpdated is a free log retrieval operation binding the contract event 0x12b32aa5a9f9ab940b704a81602a4d1ba5066d82c4e4a5cbf13fce29771b675f.
//
// Solidity: event AuthorizationsUpdated(address indexed identity, address signer, uint256 expiry, bytes32[] permissions, string origin)
func (_SessionKeyRegistry *SessionKeyRegistryFilterer) FilterAuthorizationsUpdated(opts *bind.FilterOpts, identity []common.Address) (*SessionKeyRegistryAuthorizationsUpdatedIterator, error) {

	var identityRule []interface{}
	for _, identityItem := range identity {
		identityRule = append(identityRule, identityItem)
	}

	logs, sub, err := _SessionKeyRegistry.contract.FilterLogs(opts, "AuthorizationsUpdated", identityRule)
	if err != nil {
		return nil, err
	}
	return &SessionKeyRegistryAuthorizationsUpdatedIterator{contract: _SessionKeyRegistry.contract, event: "AuthorizationsUpdated", logs: logs, sub: sub}, nil
}

// WatchAuthorizationsUpdated is a free log subscription operation binding the contract event 0x12b32aa5a9f9ab940b704a81602a4d1ba5066d82c4e4a5cbf13fce29771b675f.
//
// Solidity: event AuthorizationsUpdated(address indexed identity, address signer, uint256 expiry, bytes32[] permissions, string origin)
func (_SessionKeyRegistry *SessionKeyRegistryFilterer) WatchAuthorizationsUpdated(opts *bind.WatchOpts, sink chan<- *SessionKeyRegistryAuthorizationsUpdated, identity []common.Address) (event.Subscription, error) {

	var identityRule []interface{}
	for _, identityItem := range identity {
		identityRule = append(identityRule, identityItem)
	}

	logs, sub, err := _SessionKeyRegistry.contract.WatchLogs(opts, "AuthorizationsUpdated", identityRule)
	if err != nil {
		return nil, err
	}
	return event.NewSubscription(func(quit <-chan struct{}) error {
		defer sub.Unsubscribe()
		for {
			select {
			case log := <-logs:
				// New log arrived, parse the event and forward to the user
				event := new(SessionKeyRegistryAuthorizationsUpdated)
				if err := _SessionKeyRegistry.contract.UnpackLog(event, "AuthorizationsUpdated", log); err != nil {
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

// ParseAuthorizationsUpdated is a log parse operation binding the contract event 0x12b32aa5a9f9ab940b704a81602a4d1ba5066d82c4e4a5cbf13fce29771b675f.
//
// Solidity: event AuthorizationsUpdated(address indexed identity, address signer, uint256 expiry, bytes32[] permissions, string origin)
func (_SessionKeyRegistry *SessionKeyRegistryFilterer) ParseAuthorizationsUpdated(log types.Log) (*SessionKeyRegistryAuthorizationsUpdated, error) {
	event := new(SessionKeyRegistryAuthorizationsUpdated)
	if err := _SessionKeyRegistry.contract.UnpackLog(event, "AuthorizationsUpdated", log); err != nil {
		return nil, err
	}
	event.Raw = log
	return event, nil
}
