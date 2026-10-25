package warmstorage

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"

	fwssbind "github.com/strahe/synapse-go/internal/contracts/fwss"
	sdktypes "github.com/strahe/synapse-go/types"
)

func TestPDPPaymentTerminatedEvent_VerifiesContractAndDataSet(t *testing.T) {
	svc, _ := newWriteTestService(t)
	address := svc.FWSSAddress()
	id := sdktypes.NewBigInt(17)
	contractABI, err := fwssbind.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	event := contractABI.Events["PDPPaymentTerminated"]
	data, err := event.Inputs.NonIndexed().Pack(big.NewInt(99), big.NewInt(31))
	if err != nil {
		t.Fatal(err)
	}
	log := func(address common.Address, dataSetID int64) *coretypes.Log {
		return &coretypes.Log{
			Address: address,
			Topics:  []common.Hash{event.ID, common.BigToHash(big.NewInt(dataSetID))},
			Data:    data,
		}
	}
	foreignContract := log(common.HexToAddress("0x9999"), 17)
	otherDataSet := log(address, 99)
	matching := log(address, 17)
	for _, tt := range []struct {
		name string
		logs []*coretypes.Log
		want bool
	}{
		{"foreign contract", []*coretypes.Log{foreignContract}, false},
		{"other data set", []*coretypes.Log{otherDataSet}, false},
		{"matching after unrelated events", []*coretypes.Log{nil, foreignContract, otherDataSet, matching}, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := svc.PDPPaymentTerminatedEvent(&coretypes.Receipt{Logs: tt.logs}, id)
			if !tt.want {
				if err == nil || got != nil {
					t.Fatalf("PDPPaymentTerminatedEvent = %+v, %v, want no matching event", got, err)
				}
				return
			}
			if err != nil || got == nil || !got.DataSetID.Equal(id) || got.EndEpoch != 99 || !got.PDPRailID.Equal(sdktypes.NewBigInt(31)) {
				t.Fatalf("PDPPaymentTerminatedEvent = %+v, %v", got, err)
			}
		})
	}
	if _, err := svc.PDPPaymentTerminatedEvent(&coretypes.Receipt{}, sdktypes.BigInt{}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("zero data set error = %v", err)
	}
}
