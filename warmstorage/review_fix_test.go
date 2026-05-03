package warmstorage

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	fwssviewbind "github.com/strahe/synapse-go/internal/contracts/fwssview"
	"github.com/strahe/synapse-go/signer"
	"github.com/strahe/synapse-go/types"
)

func TestGetClientDataSetsWithDetails_PropagatesEnrichmentFailure(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		{
			PdpRailId:       big.NewInt(11),
			CacheMissRailId: big.NewInt(0),
			CdnRailId:       big.NewInt(0),
			Payer:           payer,
			Payee:           common.HexToAddress("0x5555555555555555555555555555555555555555"),
			ServiceProvider: common.HexToAddress("0x6666666666666666666666666666666666666666"),
			CommissionBps:   big.NewInt(100),
			ClientDataSetId: big.NewInt(1),
			PdpEndEpoch:     big.NewInt(0),
			ProviderId:      big.NewInt(7),
			DataSetId:       big.NewInt(1),
		},
		{
			PdpRailId:       big.NewInt(12),
			CacheMissRailId: big.NewInt(0),
			CdnRailId:       big.NewInt(0),
			Payer:           payer,
			Payee:           common.HexToAddress("0x7777777777777777777777777777777777777777"),
			ServiceProvider: common.HexToAddress("0x8888888888888888888888888888888888888888"),
			CommissionBps:   big.NewInt(200),
			ClientDataSetId: big.NewInt(2),
			PdpEndEpoch:     big.NewInt(0),
			ProviderId:      big.NewInt(8),
			DataSetId:       big.NewInt(2),
		},
	})
	mc.setPDPReply(t, "getDataSetListener", s.fwssAddr)
	mc.handlers["dataSetLive"] = func(data []byte) ([]byte, error) {
		args, err := mc.pdpABI.Methods["dataSetLive"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		if args[0].(*big.Int).Cmp(big.NewInt(1)) == 0 {
			return nil, errors.New("boom")
		}
		return mc.pdpABI.Methods["dataSetLive"].Outputs.Pack(false)
	}

	got, err := s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err == nil {
		t.Fatalf("GetClientDataSetsWithDetails err=nil, got=%+v want enrichment failure", got)
	}
	if !strings.Contains(err.Error(), "dataSetLive") {
		t.Fatalf("GetClientDataSetsWithDetails err=%v, want dataSetLive context", err)
	}
}

func TestTopUpCDNPaymentRails_RejectsDoubleZeroTopUp(t *testing.T) {
	s, backend := newWriteTestService(t)

	got, err := s.TopUpCDNPaymentRails(context.Background(), types.NewBigInt(1), big.NewInt(0), big.NewInt(0))
	if err == nil || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("TopUpCDNPaymentRails double zero err=%v result=%+v, want ErrInvalidArgument", err, got)
	}
	if len(backend.sent) != 0 {
		t.Fatalf("sent tx count = %d, want 0", len(backend.sent))
	}
}

func TestGetClientDataSetsWithDetails_IncludesParityMetadata(t *testing.T) {
	s, mc := newTestServiceWithPDP(t)
	payer := common.HexToAddress("0x4444444444444444444444444444444444444444")

	mc.setViewReply(t, "getClientDataSets", []fwssviewbind.FilecoinWarmStorageServiceDataSetInfoView{
		{
			PdpRailId:       big.NewInt(11),
			CacheMissRailId: big.NewInt(0),
			CdnRailId:       big.NewInt(99),
			Payer:           payer,
			Payee:           common.HexToAddress("0x5555555555555555555555555555555555555555"),
			ServiceProvider: common.HexToAddress("0x6666666666666666666666666666666666666666"),
			CommissionBps:   big.NewInt(100),
			ClientDataSetId: big.NewInt(1),
			PdpEndEpoch:     big.NewInt(0),
			ProviderId:      big.NewInt(7),
			DataSetId:       big.NewInt(42),
		},
	})
	mc.setPDPReply(t, "getDataSetListener", s.fwssAddr)
	mc.setPDPReply(t, "dataSetLive", true)
	mc.setPDPReply(t, "getActivePieceCount", big.NewInt(3))
	mc.setViewReply(t, "getAllDataSetMetadata", []string{"withCDN", "source"}, []string{"", "app"})

	got, err := s.GetClientDataSetsWithDetails(context.Background(), payer, false)
	if err != nil {
		t.Fatalf("GetClientDataSetsWithDetails: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("result len = %d, want 1", len(got))
	}

	val := reflect.ValueOf(*got[0])
	pdpID := val.FieldByName("PDPVerifierDataSetID")
	if !pdpID.IsValid() {
		t.Fatalf("PDPVerifierDataSetID missing or wrong: %v", pdpID)
	}
	gotPDPID, ok := pdpID.Interface().(types.BigInt)
	if !ok || !gotPDPID.Equal(types.NewBigInt(42)) {
		t.Fatalf("PDPVerifierDataSetID missing or wrong: %v", pdpID)
	}
	withCDN := val.FieldByName("WithCDN")
	if !withCDN.IsValid() || !withCDN.Bool() {
		t.Fatalf("WithCDN missing or false: %v", withCDN)
	}
	metadata := val.FieldByName("Metadata")
	if !metadata.IsValid() {
		t.Fatal("Metadata field missing")
	}
	meta, ok := metadata.Interface().(map[string]string)
	if !ok {
		t.Fatalf("Metadata has wrong type: %T", metadata.Interface())
	}
	if meta["source"] != "app" {
		t.Fatalf("Metadata[source]=%q, want app", meta["source"])
	}
	if _, ok := meta["withCDN"]; !ok {
		t.Fatalf("Metadata missing withCDN key: %v", meta)
	}
}

type mockWriteBackend struct {
	*mockCaller

	mu     sync.Mutex
	sent   []*coretypes.Transaction
	nonces map[common.Address]uint64
}

func newMockWriteBackend(t *testing.T) *mockWriteBackend {
	t.Helper()
	return &mockWriteBackend{
		mockCaller: newMockCaller(t),
		nonces:     map[common.Address]uint64{},
	}
}

func (m *mockWriteBackend) HeaderByNumber(_ context.Context, _ *big.Int) (*coretypes.Header, error) {
	return &coretypes.Header{BaseFee: big.NewInt(1_000_000_000), Number: big.NewInt(1)}, nil
}

func (m *mockWriteBackend) PendingCodeAt(_ context.Context, _ common.Address) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockWriteBackend) PendingNonceAt(_ context.Context, account common.Address) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nonces[account], nil
}

func (m *mockWriteBackend) SuggestGasPrice(_ context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (m *mockWriteBackend) SuggestGasTipCap(_ context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (m *mockWriteBackend) EstimateGas(_ context.Context, _ ethereum.CallMsg) (uint64, error) {
	return 100_000, nil
}

func (m *mockWriteBackend) SendTransaction(_ context.Context, tx *coretypes.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, tx)
	return nil
}

func (m *mockWriteBackend) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]coretypes.Log, error) {
	return nil, nil
}

func (m *mockWriteBackend) SubscribeFilterLogs(_ context.Context, _ ethereum.FilterQuery, _ chan<- coretypes.Log) (ethereum.Subscription, error) {
	return nil, errors.New("subscription not supported")
}

func (m *mockWriteBackend) TransactionReceipt(context.Context, common.Hash) (*coretypes.Receipt, error) {
	return nil, ethereum.NotFound
}

func (m *mockWriteBackend) BlockNumber(context.Context) (uint64, error) {
	return 10, nil
}

func newWriteTestSigner(t *testing.T) signer.EVMSigner {
	t.Helper()
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := signer.NewSecp256k1Signer(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newWriteTestService(t *testing.T) (*Service, *mockWriteBackend) {
	t.Helper()
	backend := newMockWriteBackend(t)
	s, err := New(Options{
		Client:       backend,
		Backend:      backend,
		Signer:       newWriteTestSigner(t),
		ChainID:      314159,
		FWSS:         common.HexToAddress("0x1111111111111111111111111111111111111111"),
		ViewContract: common.HexToAddress("0x2222222222222222222222222222222222222222"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, backend
}
