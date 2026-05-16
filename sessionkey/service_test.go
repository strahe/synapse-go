package sessionkey

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/internal/contracts/sessionkeyregistry"
	"github.com/strahe/synapse-go/signer"
	sdktypes "github.com/strahe/synapse-go/types"
)

// ---------------------------------------------------------------------------
// mock backend
// ---------------------------------------------------------------------------

var multicall3Addr = chain.Mainnet.Addresses().Multicall3

type mockBackend struct {
	mu sync.Mutex

	regABI       abi.ABI
	multicallABI abi.ABI

	replies map[string][]byte
	errs    map[string]error

	// multicallErr, if set, makes the aggregate3 call fail entirely.
	multicallErr error
	// multicallFn, if set, overrides the default multicall handling.
	multicallFn func(data []byte) ([]byte, error)

	sent      []*types.Transaction
	receipts  map[common.Hash]*types.Receipt
	receiptFn func(context.Context, common.Hash) (*types.Receipt, error)
	blockFn   func(context.Context) (uint64, error)
	nonces    map[common.Address]uint64
}

func newMockBackend(t *testing.T) *mockBackend {
	t.Helper()
	ra, err := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	mc, err := abi.JSON(strings.NewReader(`[{"inputs":[{"components":[{"internalType":"address","name":"target","type":"address"},{"internalType":"bool","name":"allowFailure","type":"bool"},{"internalType":"bytes","name":"callData","type":"bytes"}],"internalType":"struct Multicall3.Call3[]","name":"calls","type":"tuple[]"}],"name":"aggregate3","outputs":[{"components":[{"internalType":"bool","name":"success","type":"bool"},{"internalType":"bytes","name":"returnData","type":"bytes"}],"internalType":"struct Multicall3.Result[]","name":"returnData","type":"tuple[]"}],"stateMutability":"payable","type":"function"}]`))
	if err != nil {
		t.Fatal(err)
	}
	return &mockBackend{
		regABI:       *ra,
		multicallABI: mc,
		replies:      map[string][]byte{},
		errs:         map[string]error{},
		receipts:     map[common.Hash]*types.Receipt{},
		nonces:       map[common.Address]uint64{},
	}
}

func (m *mockBackend) CodeAt(_ context.Context, _ common.Address, _ *big.Int) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockBackend) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	if len(call.Data) < 4 || call.To == nil {
		return nil, errors.New("calldata too short")
	}

	// Handle Multicall3 aggregate3 calls.
	// Read callbacks before acquiring the lock to prevent deadlock if a
	// callback re-enters any mock method that also acquires m.mu.
	if *call.To == multicall3Addr {
		m.mu.Lock()
		fn := m.multicallFn
		mcErr := m.multicallErr
		m.mu.Unlock()
		if fn != nil {
			return fn(call.Data)
		}
		if mcErr != nil {
			return nil, mcErr
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		return m.handleMulticall(call.Data)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	selector := [4]byte(call.Data[:4])
	toHex := call.To.Hex()
	for name, method := range m.regABI.Methods {
		if [4]byte(method.ID) == selector {
			key := toHex + ":" + name
			if err, ok := m.errs[key]; ok {
				return nil, err
			}
			return m.replies[key], nil
		}
	}
	return nil, errors.New("unknown selector")
}

// handleMulticall simulates Multicall3 aggregate3 by dispatching each sub-call
// through the mock's registry ABI handler (replies/errs maps). Must be called with m.mu held.
func (m *mockBackend) handleMulticall(data []byte) ([]byte, error) {
	vals, err := m.multicallABI.Methods["aggregate3"].Inputs.Unpack(data[4:])
	if err != nil {
		return nil, fmt.Errorf("mock: unpack aggregate3 inputs: %w", err)
	}

	// vals[0] is []struct{Target, AllowFailure, CallData}
	rawCalls, ok := vals[0].([]struct {
		Target       common.Address `json:"target"`
		AllowFailure bool           `json:"allowFailure"`
		CallData     []byte         `json:"callData"`
	})
	if !ok {
		return nil, errors.New("mock: unexpected aggregate3 input type")
	}

	type result3 struct {
		Success    bool
		ReturnData []byte
	}
	results := make([]result3, len(rawCalls))
	for i, c := range rawCalls {
		if len(c.CallData) < 4 {
			if c.AllowFailure {
				results[i] = result3{Success: false}
				continue
			}
			return nil, errors.New("mock: calldata too short in aggregate3 sub-call")
		}
		selector := [4]byte(c.CallData[:4])
		found := false
		for name, method := range m.regABI.Methods {
			if [4]byte(method.ID) == selector {
				found = true
				key := c.Target.Hex() + ":" + name
				if err, ok := m.errs[key]; ok {
					results[i] = result3{Success: false, ReturnData: []byte(err.Error())}
				} else if reply, ok := m.replies[key]; ok {
					results[i] = result3{Success: true, ReturnData: reply}
				} else {
					results[i] = result3{Success: false}
				}
				break
			}
		}
		if !found {
			results[i] = result3{Success: false}
		}
	}

	return m.multicallABI.Methods["aggregate3"].Outputs.Pack(results)
}

func (m *mockBackend) HeaderByNumber(_ context.Context, _ *big.Int) (*types.Header, error) {
	return &types.Header{Number: big.NewInt(100)}, nil
}

func (m *mockBackend) PendingCodeAt(_ context.Context, _ common.Address) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockBackend) PendingNonceAt(_ context.Context, account common.Address) (uint64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.nonces[account], nil
}

func (m *mockBackend) SuggestGasPrice(_ context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (m *mockBackend) SuggestGasTipCap(_ context.Context) (*big.Int, error) {
	return big.NewInt(1_000_000_000), nil
}

func (m *mockBackend) EstimateGas(_ context.Context, _ ethereum.CallMsg) (uint64, error) {
	return 200_000, nil
}

func (m *mockBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, tx)
	return nil
}

func (m *mockBackend) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func (m *mockBackend) SubscribeFilterLogs(_ context.Context, _ ethereum.FilterQuery, _ chan<- types.Log) (ethereum.Subscription, error) {
	return nil, errors.New("not supported")
}

func (m *mockBackend) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	// Read callback before acquiring the lock to prevent deadlock if the
	// callback re-enters any mock method that also acquires m.mu.
	m.mu.Lock()
	fn := m.receiptFn
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, h)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if r, ok := m.receipts[h]; ok {
		return r, nil
	}
	return nil, ethereum.NotFound
}

func (m *mockBackend) BlockNumber(ctx context.Context) (uint64, error) {
	if m.blockFn != nil {
		return m.blockFn(ctx)
	}
	return 100, nil
}

func (m *mockBackend) lastSent() *types.Transaction {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.sent) == 0 {
		return nil
	}
	return m.sent[len(m.sent)-1]
}

func (m *mockBackend) setReply(t *testing.T, addr common.Address, method string, values ...any) {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	data, err := m.regABI.Methods[method].Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	m.replies[addr.Hex()+":"+method] = data
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

var (
	testRegistryAddr = common.HexToAddress("0x1111111111111111111111111111111111111111")
	testChainID      = sdktypes.ChainID(314159)
)

func newTestSigner(t *testing.T) signer.EVMSigner {
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

func newTestService(t *testing.T, mb *mockBackend, s signer.EVMSigner) *Service {
	t.Helper()
	svc, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
		Signer:          s,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// ---------------------------------------------------------------------------
// Constructor tests
// ---------------------------------------------------------------------------

func TestNew_ValidatesRequired(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{"nil backend", Options{ChainID: testChainID, RegistryAddress: testRegistryAddr}},
		{"zero chainID", Options{Backend: newMockBackend(t), RegistryAddress: testRegistryAddr}},
		{"negative chainID", Options{Backend: newMockBackend(t), ChainID: sdktypes.ChainID(-1), RegistryAddress: testRegistryAddr}},
		{"zero registry", Options{Backend: newMockBackend(t), ChainID: testChainID}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNew_ReadOnly(t *testing.T) {
	mb := newMockBackend(t)
	svc, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
	})
	if err != nil {
		t.Fatal(err)
	}
	if svc.signer != nil {
		t.Error("read-only service should have nil signer")
	}
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestLogin_DefaultParams(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)
	sessionKey := common.HexToAddress("0xDEAD")

	before := time.Now().Unix()
	res, err := svc.Login(context.Background(), sessionKey)
	after := time.Now().Unix()
	if err != nil {
		t.Fatal(err)
	}
	if res.Hash == (common.Hash{}) {
		t.Error("expected non-zero tx hash")
	}
	if res.Receipt != nil {
		t.Error("receipt should be nil without WithWait")
	}

	tx := mb.lastSent()
	if tx == nil {
		t.Fatal("no tx sent")
	}

	// Decode tx calldata to verify login parameters.
	regABI, _ := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	method, err := regABI.MethodById(tx.Data()[:4])
	if err != nil {
		t.Fatal(err)
	}
	if method.Name != "login" {
		t.Fatalf("expected login, got %s", method.Name)
	}

	args, err := method.Inputs.Unpack(tx.Data()[4:])
	if err != nil {
		t.Fatal(err)
	}
	// args: signer, expiry, permissions, origin
	gotSessionKey := args[0].(common.Address)
	if gotSessionKey != sessionKey {
		t.Errorf("sessionKey = %s, want %s", gotSessionKey, sessionKey)
	}

	gotExpiry := args[1].(*big.Int).Uint64()
	if gotExpiry < uint64(before)+3600 || gotExpiry > uint64(after)+3600 {
		t.Errorf("expiry %d out of expected range [%d, %d]", gotExpiry, before+3600, after+3600)
	}

	// Check default permissions (4 FWSS permissions, deduplicated).
	gotPerms := args[2].([][32]byte)
	if len(gotPerms) != len(DefaultFWSSPermissions) {
		t.Fatalf("permissions count = %d, want %d", len(gotPerms), len(DefaultFWSSPermissions))
	}
	for i, p := range gotPerms {
		if Permission(p) != DefaultFWSSPermissions[i] {
			t.Errorf("permission[%d] mismatch", i)
		}
	}

	gotOrigin := args[3].(string)
	if gotOrigin != "synapse" {
		t.Errorf("origin = %q, want %q", gotOrigin, "synapse")
	}
}

func TestLoginWithOptions_CustomOverrides(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	customExpiry := uint64(2000000000)
	customPerms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	customOrigin := "my-app"
	sessionKey := common.HexToAddress("0xBEEF")

	_, err := svc.LoginWithOptions(context.Background(), sessionKey, &LoginOptions{
		Permissions: customPerms,
		ExpiresAt:   customExpiry,
		Origin:      customOrigin,
	})
	if err != nil {
		t.Fatal(err)
	}

	tx := mb.lastSent()
	regABI, _ := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	args, _ := regABI.Methods["login"].Inputs.Unpack(tx.Data()[4:])

	gotExpiry := args[1].(*big.Int).Uint64()
	if gotExpiry != customExpiry {
		t.Errorf("expiry = %d, want %d", gotExpiry, customExpiry)
	}

	gotPerms := args[2].([][32]byte)
	if len(gotPerms) != 2 {
		t.Fatalf("got %d perms, want 2", len(gotPerms))
	}

	gotOrigin := args[3].(string)
	if gotOrigin != customOrigin {
		t.Errorf("origin = %q, want %q", gotOrigin, customOrigin)
	}
}

func TestLogin_DedupPermissions(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// Deliberately pass duplicates.
	duped := []Permission{
		CreateDataSetPermission,
		AddPiecesPermission,
		CreateDataSetPermission, // duplicate
		AddPiecesPermission,     // duplicate
	}
	_, err := svc.LoginWithOptions(context.Background(), common.HexToAddress("0xBEEF"), &LoginOptions{
		Permissions: duped,
		ExpiresAt:   uint64(time.Now().Unix()) + 7200,
		Origin:      "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	tx := mb.lastSent()
	regABI, _ := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	args, _ := regABI.Methods["login"].Inputs.Unpack(tx.Data()[4:])

	gotPerms := args[2].([][32]byte)
	if len(gotPerms) != 2 {
		t.Errorf("expected 2 deduplicated permissions, got %d", len(gotPerms))
	}
}

func TestLogin_NilSigner(t *testing.T) {
	mb := newMockBackend(t)
	svc, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Login(context.Background(), common.HexToAddress("0xBEEF"))
	if err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestLogin_ZeroAddress(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	_, err := svc.Login(context.Background(), common.Address{})
	if err == nil {
		t.Fatal("expected error for zero address")
	}
}

// ---------------------------------------------------------------------------
// LoginAndFund tests
// ---------------------------------------------------------------------------

func TestLoginAndFund_PayableValue(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	value := big.NewInt(1_000_000_000_000_000_000) // 1 FIL
	sessionKey := common.HexToAddress("0xCAFE")

	_, err := svc.LoginAndFund(context.Background(), sessionKey, value)
	if err != nil {
		t.Fatal(err)
	}

	tx := mb.lastSent()
	if tx.Value().Cmp(value) != 0 {
		t.Errorf("tx value = %s, want %s", tx.Value(), value)
	}

	regABI, _ := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	method, _ := regABI.MethodById(tx.Data()[:4])
	if method.Name != "loginAndFund" {
		t.Errorf("expected loginAndFund, got %s", method.Name)
	}
}

func TestLoginAndFund_NilValue(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	_, err := svc.LoginAndFund(context.Background(), common.HexToAddress("0xBEEF"), nil)
	if err == nil {
		t.Fatal("expected error for nil value")
	}
}

func TestLoginAndFund_NegativeValue(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	_, err := svc.LoginAndFund(context.Background(), common.HexToAddress("0xBEEF"), big.NewInt(-1))
	if err == nil {
		t.Fatal("expected error for negative value")
	}
}

// ---------------------------------------------------------------------------
// Revoke tests
// ---------------------------------------------------------------------------

func TestRevoke_DefaultParams(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	sessionKey := common.HexToAddress("0xDEAD")
	_, err := svc.Revoke(context.Background(), sessionKey)
	if err != nil {
		t.Fatal(err)
	}

	tx := mb.lastSent()
	regABI, _ := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	method, _ := regABI.MethodById(tx.Data()[:4])
	if method.Name != "revoke" {
		t.Fatalf("expected revoke, got %s", method.Name)
	}

	args, _ := method.Inputs.Unpack(tx.Data()[4:])
	// args: signer, permissions, origin
	gotPerms := args[1].([][32]byte)
	if len(gotPerms) != len(DefaultFWSSPermissions) {
		t.Errorf("permissions count = %d, want %d", len(gotPerms), len(DefaultFWSSPermissions))
	}

	gotOrigin := args[2].(string)
	if gotOrigin != "synapse" {
		t.Errorf("origin = %q, want %q", gotOrigin, "synapse")
	}
}

func TestRevokeWithOptions_CustomParams(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	sessionKey := common.HexToAddress("0xBEEF")
	_, err := svc.RevokeWithOptions(context.Background(), sessionKey, &RevokeOptions{
		Permissions: []Permission{CreateDataSetPermission},
		Origin:      "my-app",
	})
	if err != nil {
		t.Fatal(err)
	}

	tx := mb.lastSent()
	regABI, _ := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	args, _ := regABI.Methods["revoke"].Inputs.Unpack(tx.Data()[4:])

	gotPerms := args[1].([][32]byte)
	if len(gotPerms) != 1 {
		t.Fatalf("got %d perms, want 1", len(gotPerms))
	}
	if Permission(gotPerms[0]) != CreateDataSetPermission {
		t.Error("wrong permission")
	}

	gotOrigin := args[2].(string)
	if gotOrigin != "my-app" {
		t.Errorf("origin = %q, want %q", gotOrigin, "my-app")
	}
}

// ---------------------------------------------------------------------------
// AuthorizationExpiry tests
// ---------------------------------------------------------------------------

func TestAuthorizationExpiry_Success(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	expected := uint64(1700000000)
	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(expected))

	root := common.HexToAddress("0xAAAA")
	sessionKey := common.HexToAddress("0xBBBB")

	got, err := svc.AuthorizationExpiry(context.Background(), root, sessionKey, CreateDataSetPermission)
	if err != nil {
		t.Fatal(err)
	}
	if got != expected {
		t.Errorf("got %d, want %d", got, expected)
	}
}

func TestAuthorizationExpiry_Uint64Overflow(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// max uint256 → can't fit in uint64
	huge := new(big.Int).Lsh(big.NewInt(1), 128)
	mb.setReply(t, testRegistryAddr, "authorizationExpiry", huge)

	_, err := svc.AuthorizationExpiry(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		CreateDataSetPermission,
	)
	if err == nil {
		t.Fatal("expected overflow error")
	}
}

// ---------------------------------------------------------------------------
// IsExpired tests
// ---------------------------------------------------------------------------

func TestAuthorizationExpired(t *testing.T) {
	tests := []struct {
		name string
		exp  uint64
		now  uint64
		want bool
	}{
		{name: "past", exp: 99, now: 100, want: true},
		{name: "same second", exp: 100, now: 100, want: false},
		{name: "future", exp: 101, now: 100, want: false},
		{name: "zero before now", exp: 0, now: 1, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authorizationExpired(tt.exp, tt.now); got != tt.want {
				t.Fatalf("authorizationExpired(%d, %d) = %v, want %v", tt.exp, tt.now, got, tt.want)
			}
		})
	}
}

func TestIsExpired_True(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	pastExpiry := uint64(1000000000) // well in the past
	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(pastExpiry))

	expired, err := svc.IsExpired(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		CreateDataSetPermission,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Error("expected expired=true")
	}
}

func TestIsExpired_False(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	futureExpiry := uint64(time.Now().Unix()) + 3600
	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(futureExpiry))

	expired, err := svc.IsExpired(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		CreateDataSetPermission,
	)
	if err != nil {
		t.Fatal(err)
	}
	if expired {
		t.Error("expected expired=false")
	}
}

func TestIsExpired_ZeroMeansExpired(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	mb.setReply(t, testRegistryAddr, "authorizationExpiry", big.NewInt(0))

	expired, err := svc.IsExpired(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		CreateDataSetPermission,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !expired {
		t.Error("expected expired=true for zero expiry (not authorized)")
	}
}

// ---------------------------------------------------------------------------
// GetExpirations tests (sequential path via multicall error)
// ---------------------------------------------------------------------------

func TestGetExpirations_Sequential(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// Force multicall failure to exercise sequential path.
	mb.multicallErr = errors.New("multicall disabled")

	expected := uint64(1800000000)
	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(expected))

	perms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	expirations, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		perms,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range perms {
		if expirations[p] != expected {
			t.Errorf("permission %s: got %d, want %d", p, expirations[p], expected)
		}
	}
}

func TestGetExpirations_DefaultPermissions(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(9999))

	expirations, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		nil, // should default to DefaultFWSSPermissions
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expirations) != len(DefaultFWSSPermissions) {
		t.Fatalf("expected %d entries, got %d", len(DefaultFWSSPermissions), len(expirations))
	}
}

func TestGetExpirations_ExplicitEmptyPermissions(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(9999))

	expirations, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		[]Permission{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(expirations) != 0 {
		t.Fatalf("expected empty expirations, got %v", expirations)
	}
}

// ---------------------------------------------------------------------------
// GetExpirations tests (multicall batch path)
// ---------------------------------------------------------------------------

func TestGetExpirations_Batch_Success(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// Set per-permission expiries so we can verify differentiated results.
	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(1800000001))

	perms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	expirations, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		perms,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range perms {
		if expirations[p] != 1800000001 {
			t.Errorf("permission %s: got %d, want 1800000001", p, expirations[p])
		}
	}
}

func TestGetExpirations_Batch_AllSuccess(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(2000000000))

	perms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	expirations, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		perms,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range perms {
		if expirations[p] != 2000000000 {
			t.Errorf("permission %s: got %d, want 2000000000", p, expirations[p])
		}
	}
}

func TestGetExpirations_Batch_PartialFailure(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// Use multicallFn to return mixed results: first sub-call succeeds,
	// second sub-call returns Success=false (simulating a revert).
	successExpiry := new(big.Int).SetUint64(2000000000)
	uint256Type, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	successData, err := abi.Arguments{{Type: uint256Type}}.Pack(successExpiry)
	if err != nil {
		t.Fatal(err)
	}

	mb.multicallFn = func(data []byte) ([]byte, error) {
		vals, err := mb.multicallABI.Methods["aggregate3"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		rawCalls := vals[0].([]struct {
			Target       common.Address `json:"target"`
			AllowFailure bool           `json:"allowFailure"`
			CallData     []byte         `json:"callData"`
		})

		type result3 struct {
			Success    bool
			ReturnData []byte
		}
		results := make([]result3, len(rawCalls))
		for i := range rawCalls {
			if i == 0 {
				results[i] = result3{Success: true, ReturnData: successData}
			} else {
				results[i] = result3{Success: false, ReturnData: nil}
			}
		}
		return mb.multicallABI.Methods["aggregate3"].Outputs.Pack(results)
	}

	perms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	expirations, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		perms,
	)
	if err == nil {
		t.Fatal("expected partial-failure error, got nil")
	}
	if !errors.Is(err, errBatchPartial) {
		t.Fatalf("errors.Is(err, errBatchPartial)=false; err=%v", err)
	}
	if !strings.Contains(err.Error(), AddPiecesPermission.String()) {
		t.Fatalf("partial-failure error missing permission name; err=%v", err)
	}

	// First permission should have the expiry from the successful sub-call.
	if expirations[CreateDataSetPermission] != 2000000000 {
		t.Errorf("CreateDataSet: got %d, want 2000000000", expirations[CreateDataSetPermission])
	}
	// Second permission should retain zero and be surfaced in the joined error.
	if expirations[AddPiecesPermission] != 0 {
		t.Errorf("AddPieces: got %d, want 0 (failed sub-call)", expirations[AddPiecesPermission])
	}
}

func TestGetExpirations_Batch_Fails_FallsBackToSequential(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// Make multicall fail entirely → should fall back to sequential.
	mb.multicallErr = errors.New("multicall not deployed")

	mb.setReply(t, testRegistryAddr, "authorizationExpiry", new(big.Int).SetUint64(1700000000))

	perms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	expirations, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		perms,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range perms {
		if expirations[p] != 1700000000 {
			t.Errorf("permission %s: got %d, want 1700000000 (sequential fallback)", p, expirations[p])
		}
	}
}

// ---------------------------------------------------------------------------
// SessionKey type tests
// ---------------------------------------------------------------------------

func TestSessionKey_HasPermission(t *testing.T) {
	now := time.Now()
	sk := SessionKey{
		Address:     common.HexToAddress("0xBBBB"),
		RootAddress: common.HexToAddress("0xAAAA"),
		Expirations: Expirations{
			CreateDataSetPermission: uint64(now.Add(1 * time.Hour).Unix()),
			AddPiecesPermission:     uint64(now.Add(-1 * time.Hour).Unix()), // expired
		},
	}

	if !sk.HasPermission(CreateDataSetPermission) {
		t.Error("expected HasPermission=true for future expiry")
	}
	if sk.HasPermission(AddPiecesPermission) {
		t.Error("expected HasPermission=false for past expiry")
	}
	if sk.HasPermission(DeleteDataSetPermission) {
		t.Error("expected HasPermission=false for missing permission")
	}
}

func TestSessionKey_HasPermissionAt(t *testing.T) {
	expiry := uint64(2000000000)
	sk := SessionKey{
		Expirations: Expirations{
			CreateDataSetPermission: expiry,
		},
	}

	before := time.Unix(1999999999, 0)
	after := time.Unix(2000000001, 0)

	if !sk.HasPermissionAt(before, CreateDataSetPermission) {
		t.Error("expected true before expiry")
	}
	if sk.HasPermissionAt(after, CreateDataSetPermission) {
		t.Error("expected false after expiry")
	}
}

func TestSessionKey_HasPermissions(t *testing.T) {
	future := uint64(time.Now().Add(1 * time.Hour).Unix())
	sk := SessionKey{
		Expirations: Expirations{
			CreateDataSetPermission: future,
			AddPiecesPermission:     future,
		},
	}

	if !sk.HasPermissions([]Permission{CreateDataSetPermission, AddPiecesPermission}) {
		t.Error("expected HasPermissions=true")
	}
	if sk.HasPermissions([]Permission{CreateDataSetPermission, DeleteDataSetPermission}) {
		t.Error("expected HasPermissions=false when one permission missing")
	}
}

// ---------------------------------------------------------------------------
// dedup tests
// ---------------------------------------------------------------------------

func TestDedup(t *testing.T) {
	input := []Permission{
		CreateDataSetPermission,
		AddPiecesPermission,
		CreateDataSetPermission,
		DeleteDataSetPermission,
		AddPiecesPermission,
	}
	got := dedup(input)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	// Check order preserved.
	if Permission(got[0]) != CreateDataSetPermission {
		t.Error("first should be CreateDataSet")
	}
	if Permission(got[1]) != AddPiecesPermission {
		t.Error("second should be AddPieces")
	}
	if Permission(got[2]) != DeleteDataSetPermission {
		t.Error("third should be DeleteDataSet")
	}
}

// ---------------------------------------------------------------------------
// WriteOption tests
// ---------------------------------------------------------------------------

func TestWriteOption_Defaults(t *testing.T) {
	cfg := newWriteConfig(nil)
	if cfg.waitTimeout != 0 {
		t.Errorf("default waitTimeout = %v, want 0", cfg.waitTimeout)
	}
	if cfg.confirmations != 0 {
		t.Errorf("default confirmations = %d, want 0", cfg.confirmations)
	}
}

func TestWriteOption_WithWait(t *testing.T) {
	cfg := newWriteConfig([]WriteOption{WithWait(30 * time.Second)})
	if cfg.waitTimeout != 30*time.Second {
		t.Errorf("waitTimeout = %v, want 30s", cfg.waitTimeout)
	}
}

func TestWriteOption_WithConfirmations(t *testing.T) {
	cfg := newWriteConfig([]WriteOption{WithConfirmations(5)})
	if cfg.confirmations != 5 {
		t.Errorf("confirmations = %d, want 5", cfg.confirmations)
	}
}

// ---------------------------------------------------------------------------
// resolveLoginOptions / resolveRevokeOptions tests
// ---------------------------------------------------------------------------

func TestResolveLoginOptions_Defaults(t *testing.T) {
	before := time.Now().Unix()
	lo := resolveLoginOptions(nil)
	after := time.Now().Unix()

	if len(lo.Permissions) != len(DefaultFWSSPermissions) {
		t.Errorf("permissions count = %d, want %d", len(lo.Permissions), len(DefaultFWSSPermissions))
	}
	if lo.ExpiresAt < uint64(before)+3600 || lo.ExpiresAt > uint64(after)+3600 {
		t.Errorf("expiry %d out of range", lo.ExpiresAt)
	}
	if lo.Origin != "synapse" {
		t.Errorf("origin = %q, want %q", lo.Origin, "synapse")
	}
}

func TestResolveLoginOptions_ExplicitEmptyPermissions(t *testing.T) {
	lo := resolveLoginOptions(&LoginOptions{Permissions: []Permission{}})
	if lo.Permissions == nil {
		t.Fatal("permissions = nil, want explicit empty slice")
	}
	if len(lo.Permissions) != 0 {
		t.Fatalf("permissions count = %d, want 0", len(lo.Permissions))
	}
}

func TestResolveRevokeOptions_Defaults(t *testing.T) {
	ro := resolveRevokeOptions(nil)

	if len(ro.Permissions) != len(DefaultFWSSPermissions) {
		t.Errorf("permissions count = %d, want %d", len(ro.Permissions), len(DefaultFWSSPermissions))
	}
	if ro.Origin != "synapse" {
		t.Errorf("origin = %q, want %q", ro.Origin, "synapse")
	}
}

func TestResolveRevokeOptions_ExplicitEmptyPermissions(t *testing.T) {
	ro := resolveRevokeOptions(&RevokeOptions{Permissions: []Permission{}})
	if ro.Permissions == nil {
		t.Fatal("permissions = nil, want explicit empty slice")
	}
	if len(ro.Permissions) != 0 {
		t.Fatalf("permissions count = %d, want 0", len(ro.Permissions))
	}
}

// ---------------------------------------------------------------------------
// RegistryAddress getter test
// ---------------------------------------------------------------------------

func TestRegistryAddress(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	if svc.RegistryAddress() != testRegistryAddr {
		t.Errorf("RegistryAddress() = %s, want %s", svc.RegistryAddress(), testRegistryAddr)
	}
}

// ---------------------------------------------------------------------------
// finalize branch coverage
// ---------------------------------------------------------------------------

func TestFinalize_NoWait(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// Login without WithWait → finalize takes the waitTimeout<=0 path.
	res, err := svc.Login(context.Background(), common.HexToAddress("0xAA"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt != nil {
		t.Error("expected nil receipt without WithWait")
	}
}

func TestFinalize_WithWait_Success(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	sessionKey := common.HexToAddress("0xAA")
	// Pre-set a receipt for any tx hash.
	mb.receiptFn = func(_ context.Context, h common.Hash) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusSuccessful, TxHash: h}, nil
	}

	res, err := svc.Login(context.Background(), sessionKey, WithWait(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt == nil {
		t.Error("expected receipt with WithWait")
	}
	if res.Receipt.Status != types.ReceiptStatusSuccessful {
		t.Errorf("receipt status = %d, want successful", res.Receipt.Status)
	}
}

func TestFinalize_WithWait_TxFailed(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	sessionKey := common.HexToAddress("0xAA")
	// Return a failed receipt.
	mb.receiptFn = func(_ context.Context, h common.Hash) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusFailed, TxHash: h}, nil
	}

	res, err := svc.Login(context.Background(), sessionKey, WithWait(5*time.Second))
	if err == nil {
		t.Fatal("expected error for failed tx")
	}
	if !errors.Is(err, ErrTxFailed) {
		t.Fatalf("expected ErrTxFailed, got %v", err)
	}
	// Receipt should still be attached when ErrTxFailed.
	if res == nil || res.Receipt == nil {
		t.Error("expected receipt attached on tx failure")
	}
}

func TestFinalize_WithConfirmations(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	sessionKey := common.HexToAddress("0xAA")
	mb.receiptFn = func(_ context.Context, h common.Hash) (*types.Receipt, error) {
		return &types.Receipt{
			Status:      types.ReceiptStatusSuccessful,
			TxHash:      h,
			BlockNumber: big.NewInt(90),
		}, nil
	}
	mb.blockFn = func(_ context.Context) (uint64, error) { return 100, nil }

	res, err := svc.Login(context.Background(), sessionKey, WithWait(5*time.Second), WithConfirmations(3))
	if err != nil {
		t.Fatal(err)
	}
	if res.Receipt == nil {
		t.Error("expected receipt with confirmations")
	}
}

func TestFinalize_NoncesNil(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
		Signer:          sig,
	})
	if err != nil {
		t.Fatal(err)
	}
	// nonces is auto-created by New; nil it explicitly to exercise the nil-nonces path.
	svc.nonces = nil

	res, err := svc.Login(context.Background(), common.HexToAddress("0xAA"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Hash == (common.Hash{}) {
		t.Error("expected non-zero tx hash")
	}
	// When nonces is nil, bind leaves TransactOpts.Nonce unset, so the backend
	// provides the pending nonce (0 in the mock). Verify the tx used nonce 0.
	mb.mu.Lock()
	sentLen := len(mb.sent)
	var txNonce uint64
	if sentLen > 0 {
		txNonce = mb.sent[sentLen-1].Nonce()
	}
	mb.mu.Unlock()
	if sentLen == 0 {
		t.Fatal("no transaction was sent")
	}
	if txNonce != 0 {
		t.Errorf("expected nonce 0 from backend, got %d", txNonce)
	}
}

// ---------------------------------------------------------------------------
// LoginAndFundWithOptions edge cases
// ---------------------------------------------------------------------------

func TestLoginAndFundWithOptions_ExpiredExpiresAt(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	pastExpiry := uint64(1000) // well in the past
	_, err := svc.LoginAndFundWithOptions(context.Background(), common.HexToAddress("0xBEEF"), big.NewInt(100), &LoginOptions{
		ExpiresAt: pastExpiry,
	})
	if err == nil {
		t.Fatal("expected error for expired ExpiresAt")
	}
}

func TestLoginAndFundWithOptions_ZeroValue(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// Zero value should be fine (not nil, not negative).
	_, err := svc.LoginAndFund(context.Background(), common.HexToAddress("0xBEEF"), big.NewInt(0))
	if err != nil {
		t.Fatalf("unexpected error for zero value: %v", err)
	}
}

func TestLoginAndFundWithOptions_NilSigner(t *testing.T) {
	mb := newMockBackend(t)
	svc, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.LoginAndFund(context.Background(), common.HexToAddress("0xBEEF"), big.NewInt(100))
	if err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestLoginAndFundWithOptions_ZeroAddress(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	_, err := svc.LoginAndFund(context.Background(), common.Address{}, big.NewInt(100))
	if err == nil {
		t.Fatal("expected error for zero address")
	}
}

// ---------------------------------------------------------------------------
// RevokeWithOptions edge cases
// ---------------------------------------------------------------------------

func TestRevokeWithOptions_NilOptions(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	// nil RevokeOptions should use defaults (same as Revoke).
	_, err := svc.RevokeWithOptions(context.Background(), common.HexToAddress("0xBEEF"), nil)
	if err != nil {
		t.Fatal(err)
	}

	tx := mb.lastSent()
	regABI, _ := sessionkeyregistry.SessionKeyRegistryMetaData.GetAbi()
	args, _ := regABI.Methods["revoke"].Inputs.Unpack(tx.Data()[4:])

	gotPerms := args[1].([][32]byte)
	if len(gotPerms) != len(DefaultFWSSPermissions) {
		t.Errorf("permissions count = %d, want %d", len(gotPerms), len(DefaultFWSSPermissions))
	}
	gotOrigin := args[2].(string)
	if gotOrigin != "synapse" {
		t.Errorf("origin = %q, want %q", gotOrigin, "synapse")
	}
}

func TestRevoke_NilSigner(t *testing.T) {
	mb := newMockBackend(t)
	svc, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
	})
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.Revoke(context.Background(), common.HexToAddress("0xBEEF"))
	if err == nil {
		t.Fatal("expected error for nil signer")
	}
}

func TestRevoke_ZeroAddress(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	_, err := svc.Revoke(context.Background(), common.Address{})
	if err == nil {
		t.Fatal("expected error for zero address")
	}
}

// ---------------------------------------------------------------------------
// logger nil-path tests
// ---------------------------------------------------------------------------

func TestLog_NilLogger(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig) // logger is nil by default

	// These should not panic.
	svc.log("test info message")
	svc.logWarn("test warn message")
}

func TestLogWarn_WithLogger(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc, err := New(Options{
		Backend:         mb,
		ChainID:         testChainID,
		RegistryAddress: testRegistryAddr,
		Signer:          sig,
		Logger:          slog.Default(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// These should call the logger without panicking.
	svc.log("test info message", "key", "val")
	svc.logWarn("test warn message", "key", "val")
}

// ---------------------------------------------------------------------------
// SessionKey nil receiver test
// ---------------------------------------------------------------------------

func TestSessionKey_HasPermissionAt_NilReceiver(t *testing.T) {
	var sk *SessionKey
	if sk.HasPermissionAt(time.Now(), CreateDataSetPermission) {
		t.Error("expected false for nil receiver")
	}
}

func TestSessionKey_HasPermissionAt_NilExpirations(t *testing.T) {
	sk := &SessionKey{}
	if sk.HasPermissionAt(time.Now(), CreateDataSetPermission) {
		t.Error("expected false for nil expirations")
	}
}

// ---------------------------------------------------------------------------
// GetExpirations partial-failure tests (BREAKING: errors.Join surfacing)
// ---------------------------------------------------------------------------

// TestGetExpirations_Batch_UnpackFailure_ReturnsPartialError verifies that a
// decode error on one sub-call returns (partial Expirations, errors.Join(...))
// without falling back to sequential.
func TestGetExpirations_Batch_UnpackFailure_ReturnsPartialError(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	uint256Type, err := abi.NewType("uint256", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	goodData, err := abi.Arguments{{Type: uint256Type}}.Pack(new(big.Int).SetUint64(42))
	if err != nil {
		t.Fatal(err)
	}

	mb.multicallFn = func(data []byte) ([]byte, error) {
		vals, err := mb.multicallABI.Methods["aggregate3"].Inputs.Unpack(data[4:])
		if err != nil {
			return nil, err
		}
		rawCalls := vals[0].([]struct {
			Target       common.Address `json:"target"`
			AllowFailure bool           `json:"allowFailure"`
			CallData     []byte         `json:"callData"`
		})
		type result3 struct {
			Success    bool
			ReturnData []byte
		}
		results := make([]result3, len(rawCalls))
		for i := range rawCalls {
			if i == 0 {
				results[i] = result3{Success: true, ReturnData: goodData}
			} else {
				// Non-empty but malformed return data → unpack fails.
				results[i] = result3{Success: true, ReturnData: []byte{0x01, 0x02, 0x03}}
			}
		}
		return mb.multicallABI.Methods["aggregate3"].Outputs.Pack(results)
	}

	perms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	exps, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		perms,
	)
	if err == nil {
		t.Fatal("expected partial-failure error, got nil")
	}
	if !errors.Is(err, errBatchPartial) {
		t.Fatalf("errors.Is(err, errBatchPartial)=false; err=%v", err)
	}
	if exps[CreateDataSetPermission] != 42 {
		t.Errorf("CreateDataSet: got %d, want 42 (partial success)", exps[CreateDataSetPermission])
	}
	if exps[AddPiecesPermission] != 0 {
		t.Errorf("AddPieces: got %d, want 0 (decode failed)", exps[AddPiecesPermission])
	}
}

// TestGetExpirations_Sequential_PartialFailure_Joins verifies that when the
// batch call fails and sequential reads also have per-permission errors,
// the resulting error is errors.Join of the per-permission errors and the
// partial Expirations is still returned.
func TestGetExpirations_Sequential_PartialFailure_Joins(t *testing.T) {
	mb := newMockBackend(t)
	sig := newTestSigner(t)
	svc := newTestService(t, mb, sig)

	mb.multicallErr = errors.New("multicall not deployed")

	// Make the registry call error every time → every sequential call fails.
	mb.errs[testRegistryAddr.Hex()+":authorizationExpiry"] = errors.New("boom: registry unavailable")

	perms := []Permission{CreateDataSetPermission, AddPiecesPermission}
	exps, err := svc.GetExpirations(context.Background(),
		common.HexToAddress("0xAAAA"),
		common.HexToAddress("0xBBBB"),
		perms,
	)
	if err == nil {
		t.Fatal("expected aggregate error, got nil")
	}
	// The aggregated error must mention both permissions.
	if !strings.Contains(err.Error(), CreateDataSetPermission.String()) ||
		!strings.Contains(err.Error(), AddPiecesPermission.String()) {
		t.Fatalf("aggregated error missing permission names; err=%v", err)
	}
	// Partial result should still be a non-nil map with zero values.
	if exps == nil {
		t.Fatal("expected non-nil partial Expirations")
	}
}
