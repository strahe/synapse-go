package abi

import (
	"context"
	"fmt"
	"math/big"
	"reflect"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/contracts/fwss"
)

func TestResolveAddresses_ZeroFWSS(t *testing.T) {
	_, err := ResolveAddresses(context.Background(), nil, common.Address{})
	if err == nil {
		t.Fatal("expected error for zero fwss")
	}
}

func TestResolveAddresses_NilCaller(t *testing.T) {
	_, err := ResolveAddresses(context.Background(), nil, common.HexToAddress("0x1111111111111111111111111111111111111111"))
	if err == nil {
		t.Fatal("expected error for nil caller")
	}
}

type fakeMulticallCaller struct {
	t             *testing.T
	multicallAddr common.Address
	response      []byte
	calls         int
	checkMsg      func(t *testing.T, msg ethereum.CallMsg)
}

func (f *fakeMulticallCaller) CodeAt(context.Context, common.Address, *big.Int) ([]byte, error) {
	return []byte{0x1}, nil
}

func (f *fakeMulticallCaller) CallContract(_ context.Context, msg ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if msg.To == nil {
		return nil, fmt.Errorf("nil call target")
	}
	if *msg.To != f.multicallAddr {
		return nil, fmt.Errorf("unexpected direct call to %s", msg.To.Hex())
	}
	if f.checkMsg != nil {
		f.checkMsg(f.t, msg)
	}
	f.calls++
	return f.response, nil
}

func TestResolveAddresses_UsesMulticall3(t *testing.T) {
	fwssABI, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	pdp := common.HexToAddress("0x1000000000000000000000000000000000000001")
	reg := common.HexToAddress("0x1000000000000000000000000000000000000002")
	usdfc := common.HexToAddress("0x1000000000000000000000000000000000000003")
	pay := common.HexToAddress("0x1000000000000000000000000000000000000004")
	view := common.HexToAddress("0x1000000000000000000000000000000000000005")
	beam := common.HexToAddress("0x1000000000000000000000000000000000000006")
	skr := common.HexToAddress("0x1000000000000000000000000000000000000007")

	pack := func(name string, addr common.Address) []byte {
		t.Helper()
		b, err := fwssABI.Methods[name].Outputs.Pack(addr)
		if err != nil {
			t.Fatalf("pack %s: %v", name, err)
		}
		return b
	}
	type result struct {
		Success    bool
		ReturnData []byte
	}
	response, err := multicall3ABI.Methods["aggregate3"].Outputs.Pack([]result{
		{Success: true, ReturnData: pack("pdpVerifierAddress", pdp)},
		{Success: true, ReturnData: pack("serviceProviderRegistry", reg)},
		{Success: true, ReturnData: pack("usdfcTokenAddress", usdfc)},
		{Success: true, ReturnData: pack("paymentsContractAddress", pay)},
		{Success: true, ReturnData: pack("viewContractAddress", view)},
		{Success: true, ReturnData: pack("filBeamBeneficiaryAddress", beam)},
		{Success: true, ReturnData: pack("sessionKeyRegistry", skr)},
	})
	if err != nil {
		t.Fatal(err)
	}

	caller := &fakeMulticallCaller{
		t:             t,
		multicallAddr: chain.Mainnet.Addresses().Multicall3,
		response:      response,
	}
	fwssAddr := common.HexToAddress("0x2000000000000000000000000000000000000001")
	got, err := ResolveAddresses(context.Background(), caller, fwssAddr)
	if err != nil {
		t.Fatal(err)
	}
	if caller.calls != 1 {
		t.Fatalf("expected one multicall, got %d", caller.calls)
	}
	if got.FWSS != fwssAddr || got.PDPVerifier != pdp || got.SPRegistry != reg || got.USDFC != usdfc || got.Payments != pay || got.ViewContract != view || got.FilBeamBeneficiary != beam || got.SessionKeyRegistry != skr {
		t.Fatalf("unexpected resolved addresses: %+v", got)
	}
}

func TestResolveAddresses_AllowsPerMethodFailuresInMulticall(t *testing.T) {
	fwssABI, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	pack := func(name string, addr common.Address) []byte {
		t.Helper()
		b, err := fwssABI.Methods[name].Outputs.Pack(addr)
		if err != nil {
			t.Fatalf("pack %s: %v", name, err)
		}
		return b
	}
	type result struct {
		Success    bool
		ReturnData []byte
	}
	response, err := multicall3ABI.Methods["aggregate3"].Outputs.Pack([]result{
		{Success: true, ReturnData: pack("pdpVerifierAddress", common.HexToAddress("0x1000000000000000000000000000000000000001"))},
		{Success: false, ReturnData: nil},
		{Success: true, ReturnData: pack("usdfcTokenAddress", common.HexToAddress("0x1000000000000000000000000000000000000003"))},
		{Success: true, ReturnData: pack("paymentsContractAddress", common.HexToAddress("0x1000000000000000000000000000000000000004"))},
		{Success: true, ReturnData: pack("viewContractAddress", common.HexToAddress("0x1000000000000000000000000000000000000005"))},
		{Success: true, ReturnData: pack("filBeamBeneficiaryAddress", common.HexToAddress("0x1000000000000000000000000000000000000006"))},
		{Success: true, ReturnData: pack("sessionKeyRegistry", common.HexToAddress("0x1000000000000000000000000000000000000007"))},
	})
	if err != nil {
		t.Fatal(err)
	}

	caller := &fakeMulticallCaller{
		t:             t,
		multicallAddr: chain.Mainnet.Addresses().Multicall3,
		response:      response,
		checkMsg: func(t *testing.T, msg ethereum.CallMsg) {
			t.Helper()
			values, err := multicall3ABI.Methods["aggregate3"].Inputs.Unpack(msg.Data[4:])
			if err != nil {
				t.Fatalf("unpack aggregate3 input: %v", err)
			}
			if len(values) != 1 {
				t.Fatalf("expected 1 aggregate3 input, got %d", len(values))
			}
			calls := reflect.ValueOf(values[0])
			if calls.Len() != 7 {
				t.Fatalf("expected 7 batched calls, got %d", calls.Len())
			}
			for i := 0; i < calls.Len(); i++ {
				if !calls.Index(i).FieldByName("AllowFailure").Bool() {
					t.Fatalf("call %d did not allow failure", i)
				}
			}
		},
	}

	_, err = ResolveAddresses(context.Background(), caller, common.HexToAddress("0x2000000000000000000000000000000000000001"))
	if err == nil || !strings.Contains(err.Error(), "serviceProviderRegistry returned unsuccessful result") {
		t.Fatalf("expected method-specific unsuccessful result error, got %v", err)
	}
}

func TestResolvedAddressesFromChain_Calibration(t *testing.T) {
	a := ResolvedAddressesFromChain(chain.Calibration)
	want := chain.Calibration.Addresses()
	if a.FWSS != want.FWSS {
		t.Errorf("FWSS: got %s want %s", a.FWSS, want.FWSS)
	}
	if a.PDPVerifier != want.PDPVerifier {
		t.Errorf("PDPVerifier mismatch")
	}
	if a.SPRegistry != want.SPRegistry {
		t.Errorf("SPRegistry mismatch")
	}
	if a.USDFC != want.USDFC {
		t.Errorf("USDFC mismatch")
	}
	if a.Payments != want.Payments {
		t.Errorf("Payments mismatch")
	}
	if a.ViewContract != want.StateView {
		t.Errorf("ViewContract mismatch")
	}
	if a.SessionKeyRegistry != want.SessionKeyRegistry {
		t.Errorf("SessionKeyRegistry mismatch")
	}
	if a.Multicall3 != want.Multicall3 {
		t.Errorf("Multicall3 mismatch")
	}
}

func TestResolvedAddressesFromChain_Mainnet(t *testing.T) {
	a := ResolvedAddressesFromChain(chain.Mainnet)
	if a.FWSS != chain.Mainnet.Addresses().FWSS {
		t.Fatal("FWSS mismatch for mainnet")
	}
}
