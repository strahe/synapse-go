//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"

	synapse "github.com/strahe/synapse-go"
	"github.com/strahe/synapse-go/chain"
	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/contracts/fwss"
	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/internal/testutil"
)

type contractAddressMutation struct {
	name     string
	getter   string
	expected common.Address
}

func TestIntegration_ContractAddresses(t *testing.T) {
	integrationtest.EnsureEnv(t)
	fwssABI, err := fwss.FWSSMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	multicallABI := testutil.MulticallABI(t)
	injected := common.HexToAddress("0x000000000000000000000000000000000000dEaD")
	for _, network := range []struct {
		chain  chain.Chain
		rpcURL string
	}{
		{chain.Mainnet, chain.Mainnet.DefaultRPCURL()},
		{chain.Calibration, integrationtest.RPCURL()},
	} {
		t.Run(network.chain.String(), func(t *testing.T) {
			endpoint, err := url.Parse(network.rpcURL)
			if err != nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
				t.Fatal("contract address integration tests require an HTTP(S) RPC endpoint")
			}
			deployment := network.chain.Addresses()
			mutations := []contractAddressMutation{
				{name: "normal"},
				{name: "Payments", getter: "paymentsContractAddress", expected: deployment.Payments},
				{name: "USDFC", getter: "usdfcTokenAddress", expected: deployment.USDFC},
				{name: "SessionKeyRegistry", getter: "sessionKeyRegistry", expected: deployment.SessionKeyRegistry},
				{name: "SPRegistry", getter: "serviceProviderRegistry", expected: deployment.SPRegistry},
			}
			for _, mutation := range mutations {
				t.Run(mutation.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
					defer cancel()
					key, err := crypto.GenerateKey()
					if err != nil {
						t.Fatal(err)
					}
					base := http.DefaultTransport.(*http.Transport).Clone()
					t.Cleanup(base.CloseIdleConnections)
					transport := &contractAddressTransport{
						base:         base,
						multicallABI: multicallABI,
						fwssABI:      fwssABI,
						deployment:   deployment,
						mutation:     mutation,
						injected:     injected,
					}
					rpcClient, err := rpc.DialOptions(ctx, network.rpcURL, rpc.WithHTTPClient(&http.Client{Transport: transport}))
					if err != nil {
						t.Fatal(err)
					}
					ethClient := ethclient.NewClient(rpcClient)
					t.Cleanup(ethClient.Close)
					client, err := synapse.New(ctx, synapse.WithPrivateKey(key), synapse.WithEthClient(ethClient),
						synapse.WithChain(network.chain), synapse.WithoutUploadBatching())
					if mutation.getter == "" {
						if err != nil {
							t.Fatalf("New: %v", err)
						}
						t.Cleanup(func() { _ = client.Close() })
						got := client.ResolvedAddresses()
						if got.Payments != deployment.Payments || got.USDFC != deployment.USDFC ||
							got.SessionKeyRegistry != deployment.SessionKeyRegistry || got.SPRegistry != deployment.SPRegistry {
							t.Fatalf("resolved funding contracts differ from %s deployment: %+v", network.chain, got)
						}
						return
					}
					if client != nil {
						_ = client.Close()
						t.Fatal("New accepted a replaced contract address")
					}
					if transport.mutations != 1 {
						t.Fatalf("injected responses = %d, want 1; New error: %v", transport.mutations, err)
					}
					if !errors.Is(err, synapse.ErrContractAddressMismatch) || errors.Is(err, synapse.ErrInvalidArgument) {
						t.Fatalf("New error = %v, want ErrContractAddressMismatch without ErrInvalidArgument", err)
					}
					for _, part := range []string{mutation.name, mutation.expected.Hex(), injected.Hex()} {
						if !strings.Contains(err.Error(), part) {
							t.Errorf("New error %q does not contain %q", err, part)
						}
					}
				})
			}
		})
	}
}

type contractAddressTransport struct {
	base         *http.Transport
	multicallABI gethabi.ABI
	fwssABI      *gethabi.ABI
	deployment   chain.ContractAddresses
	mutation     contractAddressMutation
	injected     common.Address
	mutations    int
}

func (tr *contractAddressTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.GetBody == nil {
		return nil, errors.New("RPC request has no replayable body")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, err
	}
	var call struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	err = json.NewDecoder(body).Decode(&call)
	_ = body.Close()
	if err != nil {
		return nil, fmt.Errorf("decode initialization RPC: %w", err)
	}
	if call.Method != "eth_chainId" && call.Method != "eth_call" {
		return nil, fmt.Errorf("unexpected initialization RPC method %q", call.Method)
	}
	index := -1
	if call.Method == "eth_call" && tr.mutation.getter != "" {
		index, err = tr.getterIndex(call.Params)
		if err != nil {
			return nil, err
		}
	}
	response, err := tr.base.RoundTrip(req)
	if err != nil || index < 0 {
		return response, err
	}
	raw, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("read address response: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("address RPC returned HTTP %d", response.StatusCode)
	}
	raw, err = tr.replaceGetter(raw, index)
	if err != nil {
		return nil, err
	}
	response.Body = io.NopCloser(bytes.NewReader(raw))
	response.ContentLength = int64(len(raw))
	response.Header.Set("Content-Length", strconv.Itoa(len(raw)))
	tr.mutations++
	return response, nil
}

func (tr *contractAddressTransport) getterIndex(params []json.RawMessage) (int, error) {
	if len(params) == 0 {
		return -1, errors.New("eth_call has no parameters")
	}
	var call struct {
		To    common.Address `json:"to"`
		Input hexutil.Bytes  `json:"input"`
		Data  hexutil.Bytes  `json:"data"`
	}
	if err := json.Unmarshal(params[0], &call); err != nil {
		return -1, err
	}
	data := call.Input
	if len(data) == 0 {
		data = call.Data
	}
	aggregate := tr.multicallABI.Methods["aggregate3"]
	if call.To != tr.deployment.Multicall3 || len(data) < 4 || !bytes.Equal(data[:4], aggregate.ID) {
		return -1, errors.New("address lookup did not call Multicall3.aggregate3")
	}
	values, err := aggregate.Inputs.Unpack(data[4:])
	if err != nil {
		return -1, err
	}
	calls := *gethabi.ConvertType(values[0], new([]iabi.Call3)).(*[]iabi.Call3)
	getter := tr.fwssABI.Methods[tr.mutation.getter]
	index := -1
	for i, subcall := range calls {
		if subcall.Target == tr.deployment.FWSS && bytes.Equal(subcall.CallData, getter.ID) {
			if index >= 0 {
				return -1, fmt.Errorf("duplicate %s getter", tr.mutation.getter)
			}
			index = i
		}
	}
	if index < 0 {
		return -1, fmt.Errorf("missing %s getter", tr.mutation.getter)
	}
	return index, nil
}

func (tr *contractAddressTransport) replaceGetter(raw []byte, index int) ([]byte, error) {
	var response map[string]json.RawMessage
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, err
	}
	if rpcErr := response["error"]; len(rpcErr) > 0 && !bytes.Equal(rpcErr, []byte("null")) {
		return nil, fmt.Errorf("address RPC returned an error: %s", rpcErr)
	}
	var encoded hexutil.Bytes
	if err := json.Unmarshal(response["result"], &encoded); err != nil {
		return nil, err
	}
	var results []iabi.Result3
	if err := tr.multicallABI.UnpackIntoInterface(&results, "aggregate3", encoded); err != nil {
		return nil, err
	}
	if index >= len(results) || !results[index].Success {
		return nil, fmt.Errorf("%s getter did not succeed", tr.mutation.getter)
	}
	getter := tr.fwssABI.Methods[tr.mutation.getter]
	values, err := getter.Outputs.Unpack(results[index].ReturnData)
	if err != nil {
		return nil, err
	}
	if len(values) != 1 || values[0] != tr.mutation.expected {
		return nil, fmt.Errorf("%s original address differs from deployment: %v", tr.mutation.name, values)
	}
	results[index].ReturnData, err = getter.Outputs.Pack(tr.injected)
	if err != nil {
		return nil, err
	}
	encoded, err = tr.multicallABI.Methods["aggregate3"].Outputs.Pack(results)
	if err != nil {
		return nil, err
	}
	response["result"], err = json.Marshal(encoded)
	if err != nil {
		return nil, err
	}
	return json.Marshal(response)
}
