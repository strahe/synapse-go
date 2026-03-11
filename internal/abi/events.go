package abi

import (
	"errors"
	"fmt"

	ethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// ErrEventNotFound is returned when the named event is not present in the
// receipt's logs.
var ErrEventNotFound = errors.New("event not found in receipt")

// ExtractEvent finds the first log in receipt matching the event named
// eventName in contractABI, emitted by the optional address filter
// (zero address = any emitter). Non-indexed fields are unpacked into dst,
// which must be a pointer to a compatible struct or slice as expected by
// go-ethereum's abi.UnpackIntoInterface. Indexed fields are not populated;
// use a typed bindings helper if you need indexed topic values.
func ExtractEvent(receipt *types.Receipt, contractABI *ethabi.ABI, eventName string, emitter common.Address, dst any) error {
	if receipt == nil {
		return fmt.Errorf("abi.ExtractEvent: nil receipt")
	}
	if contractABI == nil {
		return fmt.Errorf("abi.ExtractEvent: nil abi")
	}
	ev, ok := contractABI.Events[eventName]
	if !ok {
		return fmt.Errorf("abi.ExtractEvent: event %q not in ABI", eventName)
	}
	topic := ev.ID
	for _, log := range receipt.Logs {
		if log == nil || len(log.Topics) == 0 || log.Topics[0] != topic {
			continue
		}
		if emitter != (common.Address{}) && log.Address != emitter {
			continue
		}
		if dst != nil {
			if err := contractABI.UnpackIntoInterface(dst, eventName, log.Data); err != nil {
				return fmt.Errorf("abi.ExtractEvent: unpack %s: %w", eventName, err)
			}
		}
		return nil
	}
	return fmt.Errorf("%w: event=%s", ErrEventNotFound, eventName)
}
