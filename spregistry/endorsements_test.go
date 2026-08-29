package spregistry

import (
	"context"
	"errors"
	"math/big"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"

	provideridset "github.com/strahe/synapse-go/internal/contracts/provideridset"
	"github.com/strahe/synapse-go/types"
)

type endorsementTestCaller struct {
	t        *testing.T
	contract abi.ABI
	address  common.Address
	ids      []*big.Int
	err      error
	calls    int
}

func newEndorsementTestCaller(t *testing.T, address common.Address, ids []*big.Int) *endorsementTestCaller {
	t.Helper()
	contract, err := provideridset.ProviderIDSetMetaData.GetAbi()
	if err != nil {
		t.Fatalf("parse ProviderIdSet ABI: %v", err)
	}
	return &endorsementTestCaller{t: t, contract: *contract, address: address, ids: ids}
}

func (c *endorsementTestCaller) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return []byte{1}, nil
}

func (c *endorsementTestCaller) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	c.calls++
	if call.To == nil || *call.To != c.address {
		c.t.Fatalf("endorsement call target = %v, want %s", call.To, c.address)
	}
	if c.err != nil {
		return nil, c.err
	}
	method := c.contract.Methods["getProviderIds"]
	if len(call.Data) < 4 || string(call.Data[:4]) != string(method.ID) {
		c.t.Fatalf("endorsement call data = %x, want selector %x", call.Data, method.ID)
	}
	return method.Outputs.Pack(c.ids)
}

func TestGetEndorsedProviderIDs(t *testing.T) {
	address := common.HexToAddress("0x1234")
	caller := newEndorsementTestCaller(t, address, []*big.Int{
		big.NewInt(7), big.NewInt(1), big.NewInt(7), big.NewInt(5),
	})
	service, err := New(Options{
		Client:              caller,
		Address:             common.HexToAddress("0xabcd"),
		EndorsementsAddress: address,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	got, err := service.GetEndorsedProviderIDs(context.Background())
	if err != nil {
		t.Fatalf("GetEndorsedProviderIDs: %v", err)
	}
	if len(got) != 3 || !got[0].Equal(types.NewBigInt(7)) || !got[1].Equal(types.NewBigInt(1)) || !got[2].Equal(types.NewBigInt(5)) {
		t.Fatalf("GetEndorsedProviderIDs = %v, want [7 1 5]", got)
	}
	if caller.calls != 1 {
		t.Fatalf("endorsement calls = %d, want 1", caller.calls)
	}
}

func TestGetEndorsedProviderIDsErrors(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		service, _ := newTestService(t)
		_, err := service.GetEndorsedProviderIDs(context.Background())
		if !errors.Is(err, ErrEndorsementsNotConfigured) {
			t.Fatalf("error = %v, want ErrEndorsementsNotConfigured", err)
		}
	})

	t.Run("preserves RPC error", func(t *testing.T) {
		address := common.HexToAddress("0x1234")
		want := errors.New("RPC unavailable")
		caller := newEndorsementTestCaller(t, address, nil)
		caller.err = want
		service, err := New(Options{
			Client:              caller,
			Address:             common.HexToAddress("0xabcd"),
			EndorsementsAddress: address,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		_, err = service.GetEndorsedProviderIDs(context.Background())
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want wrapped RPC error", err)
		}
	})
}
