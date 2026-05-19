package spregistry

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	coretypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/strahe/synapse-go/signer"
)

// mockWriteBackend extends the read-only mockCaller with the
// ContractTransactor surface (PendingCodeAt / PendingNonceAt /
// Suggest* / EstimateGas / SendTransaction / TransactionReceipt /
// BlockNumber) so spregistry.New can bind a transactor and the write
// methods can broadcast through it. Captured transactions are held on
// `sent` for assertions; SendTransaction always returns nil so the
// finalize path is exercised.
type mockWriteBackend struct {
	*mockCaller

	mu     sync.Mutex
	sent   []*coretypes.Transaction
	nonces map[common.Address]uint64

	// failRegistrationFee causes the REGISTRATION_FEE view call to fail
	// before any transaction is assembled. Used to exercise the fee-read
	// error branch of RegisterProvider.
	failRegistrationFee error

	// receiptFn, when non-nil, overrides TransactionReceipt so tests can
	// drive the finalize branches (revert, success, timeout). The default
	// returns ethereum.NotFound which causes WaitForReceipt to loop until
	// its context / timeout elapses.
	receiptFn func(context.Context, common.Hash) (*coretypes.Receipt, error)
}

func newMockWriteBackend(t *testing.T) *mockWriteBackend {
	t.Helper()
	return &mockWriteBackend{
		mockCaller: newMockCaller(t),
		nonces:     map[common.Address]uint64{},
	}
}

// CallContract overrides the mockCaller's default dispatcher so that the
// REGISTRATION_FEE view call can be made to fail on demand. All other
// methods delegate to the base mockCaller replies/argCheck/errs tables.
func (m *mockWriteBackend) CallContract(ctx context.Context, call ethereum.CallMsg, bn *big.Int) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if m.failRegistrationFee != nil && len(call.Data) >= 4 {
		selector := [4]byte{call.Data[0], call.Data[1], call.Data[2], call.Data[3]}
		if mth, ok := m.sprABI.Methods["REGISTRATION_FEE"]; ok {
			if [4]byte(mth.ID) == selector {
				return nil, m.failRegistrationFee
			}
		}
	}
	return m.mockCaller.CallContract(ctx, call, bn)
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

func (m *mockWriteBackend) TransactionReceipt(ctx context.Context, hash common.Hash) (*coretypes.Receipt, error) {
	m.mu.Lock()
	fn := m.receiptFn
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, hash)
	}
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

// newWriteTestService constructs a Service wired for writes using the
// mock backend + an ad-hoc signer. Chain ID 314159 matches calibnet for
// EIP-155 signing; the registry address is arbitrary because the mock
// never enforces it.
func newWriteTestService(t *testing.T) (*Service, *mockWriteBackend) {
	t.Helper()
	backend := newMockWriteBackend(t)
	s, err := New(Options{
		Client:  backend,
		Backend: backend,
		Signer:  newWriteTestSigner(t),
		ChainID: 314159,
		Address: common.HexToAddress("0xabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, backend
}

func newSplitWriteTestService(t *testing.T) (*Service, *mockCaller, *mockWriteBackend) {
	t.Helper()
	client := newMockCaller(t)
	backend := newMockWriteBackend(t)
	s, err := New(Options{
		Client:  client,
		Backend: backend,
		Signer:  newWriteTestSigner(t),
		ChainID: 314159,
		Address: common.HexToAddress("0xabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, client, backend
}

func sampleRegistration() ProviderRegistrationInfo {
	return ProviderRegistrationInfo{
		Payee:        common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Name:         "test-provider",
		Description:  "test desc",
		PDPOffering:  sampleOffering(),
		Capabilities: map[string]string{"region": "us-west"},
	}
}

// decodeLastSentCalldata pulls the ABI method name and decoded inputs out
// of the most recent transaction captured by the mock backend.
func decodeLastSentCalldata(t *testing.T, backend *mockWriteBackend) (string, []any) {
	t.Helper()
	if len(backend.sent) == 0 {
		t.Fatalf("no transactions sent")
	}
	tx := backend.sent[len(backend.sent)-1]
	data := tx.Data()
	if len(data) < 4 {
		t.Fatalf("calldata too short")
	}
	selector := [4]byte{data[0], data[1], data[2], data[3]}
	for name, method := range backend.sprABI.Methods {
		if [4]byte(method.ID) == selector {
			args, err := method.Inputs.Unpack(data[4:])
			if err != nil {
				t.Fatalf("unpack %s: %v", name, err)
			}
			return name, args
		}
	}
	t.Fatalf("no method matches selector %x", selector)
	return "", nil
}

func TestRequireSigner_ReadOnlyService(t *testing.T) {
	s, _ := newTestService(t)

	_, err := s.RegisterProvider(context.Background(), sampleRegistration())
	if !errors.Is(err, ErrWriteNotConfigured) {
		t.Errorf("RegisterProvider: got %v want ErrWriteNotConfigured", err)
	}
	_, err = s.UpdateProviderInfo(context.Background(), "x", "y")
	if !errors.Is(err, ErrWriteNotConfigured) {
		t.Errorf("UpdateProviderInfo: got %v", err)
	}
	_, err = s.RemoveProvider(context.Background())
	if !errors.Is(err, ErrWriteNotConfigured) {
		t.Errorf("RemoveProvider: got %v", err)
	}
	_, err = s.AddPDPProduct(context.Background(), sampleOffering(), nil)
	if !errors.Is(err, ErrWriteNotConfigured) {
		t.Errorf("AddPDPProduct: got %v", err)
	}
	_, err = s.UpdatePDPProduct(context.Background(), sampleOffering(), nil)
	if !errors.Is(err, ErrWriteNotConfigured) {
		t.Errorf("UpdatePDPProduct: got %v", err)
	}
	_, err = s.RemoveProduct(context.Background(), ProductTypePDP)
	if !errors.Is(err, ErrWriteNotConfigured) {
		t.Errorf("RemoveProduct: got %v", err)
	}
}

func TestRegisterProvider_FetchesRegistrationFee(t *testing.T) {
	s, backend := newWriteTestService(t)
	backend.set(t, "REGISTRATION_FEE", big.NewInt(5_000_000_000_000_000)) // 0.005 FIL

	reg := sampleRegistration()
	res, err := s.RegisterProvider(context.Background(), reg)
	if err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}
	if res == nil || res.Hash == (common.Hash{}) {
		t.Fatal("expected non-zero tx hash")
	}
	if len(backend.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(backend.sent))
	}
	tx := backend.sent[0]
	if tx.Value() == nil || tx.Value().Cmp(big.NewInt(5_000_000_000_000_000)) != 0 {
		t.Errorf("tx.Value = %v, want 5_000_000_000_000_000", tx.Value())
	}

	name, args := decodeLastSentCalldata(t, backend)
	if name != "registerProvider" {
		t.Fatalf("called %s, want registerProvider", name)
	}
	if got := args[0].(common.Address); got != reg.Payee {
		t.Errorf("payee = %s, want %s", got, reg.Payee)
	}
	if got := args[1].(string); got != reg.Name {
		t.Errorf("name = %q, want %q", got, reg.Name)
	}
	if got := args[3].(uint8); got != uint8(ProductTypePDP) {
		t.Errorf("productType = %d, want %d", got, ProductTypePDP)
	}
	keys := args[4].([]string)
	if len(keys) == 0 || keys[0] != CapServiceURL {
		t.Errorf("keys[0] = %q, want serviceURL", keys[0])
	}
}

func TestRegisterProvider_UsesBackendForRegistrationFee(t *testing.T) {
	s, client, backend := newSplitWriteTestService(t)
	client.set(t, "REGISTRATION_FEE", big.NewInt(1))
	backend.set(t, "REGISTRATION_FEE", big.NewInt(2))

	if _, err := s.RegisterProvider(context.Background(), sampleRegistration()); err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}
	if len(backend.sent) != 1 {
		t.Fatalf("sent tx count = %d, want 1", len(backend.sent))
	}
	if got := backend.sent[0].Value(); got == nil || got.Cmp(big.NewInt(2)) != 0 {
		t.Fatalf("tx.Value = %v, want backend fee 2", got)
	}
}

func TestRegisterProvider_AllowsEmptyDescription(t *testing.T) {
	s, backend := newWriteTestService(t)
	backend.set(t, "REGISTRATION_FEE", big.NewInt(1))

	reg := sampleRegistration()
	reg.Description = ""
	if _, err := s.RegisterProvider(context.Background(), reg); err != nil {
		t.Fatalf("RegisterProvider: %v", err)
	}
	name, args := decodeLastSentCalldata(t, backend)
	if name != "registerProvider" {
		t.Fatalf("called %s, want registerProvider", name)
	}
	if got := args[2].(string); got != "" {
		t.Fatalf("description = %q, want empty", got)
	}
}

func TestRegisterProvider_WithExactValueSkipsFeeCall(t *testing.T) {
	s, backend := newWriteTestService(t)
	// Do NOT set REGISTRATION_FEE; if the code still fetched it, the
	// mock would error on missing reply and the test would fail.
	backend.failRegistrationFee = errors.New("fee fetch should not happen")

	explicitFee := big.NewInt(5_000_000_000_000_000_000)
	res, err := s.RegisterProvider(context.Background(), sampleRegistration(), WithValue(explicitFee))
	if err != nil {
		t.Fatalf("RegisterProvider with value: %v", err)
	}
	if res == nil {
		t.Fatal("nil result")
	}
	tx := backend.sent[0]
	if tx.Value().Cmp(explicitFee) != 0 {
		t.Errorf("tx.Value = %v, want %v", tx.Value(), explicitFee)
	}
}

func TestRegisterProvider_RejectsIncorrectExplicitValue(t *testing.T) {
	s, backend := newWriteTestService(t)
	backend.failRegistrationFee = errors.New("fee fetch should not happen")

	_, err := s.RegisterProvider(context.Background(), sampleRegistration(), WithValue(big.NewInt(42)))
	if !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("expected ErrInvalidArgument, got %v", err)
	}
	if len(backend.sent) != 0 {
		t.Fatalf("sent %d transactions, want 0", len(backend.sent))
	}
}

func TestRegisterProvider_FeeFetchFailurePropagates(t *testing.T) {
	s, backend := newWriteTestService(t)
	boom := errors.New("RPC down")
	backend.failRegistrationFee = boom

	_, err := s.RegisterProvider(context.Background(), sampleRegistration())
	if err == nil || !errors.Is(err, boom) {
		t.Fatalf("got %v, want wrapped %v", err, boom)
	}
	if len(backend.sent) != 0 {
		t.Errorf("should not broadcast tx on fee error; sent=%d", len(backend.sent))
	}
}

func TestRegisterProvider_RejectsInvalidInputs(t *testing.T) {
	s, _ := newWriteTestService(t)
	ctx := context.Background()

	// zero payee
	reg := sampleRegistration()
	reg.Payee = common.Address{}
	if _, err := s.RegisterProvider(ctx, reg); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("zero payee: got %v want ErrInvalidArgument", err)
	}

	// empty name
	reg = sampleRegistration()
	reg.Name = ""
	if _, err := s.RegisterProvider(ctx, reg); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("empty name: got %v want ErrInvalidArgument", err)
	}

	reg = sampleRegistration()
	reg.Name = strings.Repeat("x", 129)
	if _, err := s.RegisterProvider(ctx, reg); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("long name: got %v want ErrInvalidArgument", err)
	}

	reg = sampleRegistration()
	reg.Description = strings.Repeat("x", 257)
	if _, err := s.RegisterProvider(ctx, reg); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("long description: got %v want ErrInvalidArgument", err)
	}

	// invalid offering (zero proving period)
	reg = sampleRegistration()
	reg.PDPOffering.MinProvingPeriodInEpochs = big.NewInt(0)
	if _, err := s.RegisterProvider(ctx, reg); !errors.Is(err, ErrInvalidOffering) {
		t.Errorf("invalid offering: got %v want ErrInvalidOffering", err)
	}

	// empty description is intentionally allowed; see TestRegisterProvider_AllowsEmptyDescription.

	// negative WithValue
	reg = sampleRegistration()
	if _, err := s.RegisterProvider(ctx, reg, WithValue(big.NewInt(-1))); !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("negative WithValue: got %v want ErrInvalidArgument", err)
	}
}

func TestRegisterProvider_ContextCancelAbortsBeforeSend(t *testing.T) {
	s, backend := newWriteTestService(t)
	backend.set(t, "REGISTRATION_FEE", big.NewInt(1))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.RegisterProvider(ctx, sampleRegistration())
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
	if len(backend.sent) != 0 {
		t.Errorf("transactions sent despite cancelled ctx: %d", len(backend.sent))
	}
}

func TestUpdateProviderInfo_BroadcastsDecodedArgs(t *testing.T) {
	s, backend := newWriteTestService(t)

	if _, err := s.UpdateProviderInfo(context.Background(), "new-name", "new-desc"); err != nil {
		t.Fatalf("UpdateProviderInfo: %v", err)
	}
	name, args := decodeLastSentCalldata(t, backend)
	if name != "updateProviderInfo" {
		t.Fatalf("called %s", name)
	}
	if args[0].(string) != "new-name" || args[1].(string) != "new-desc" {
		t.Errorf("args: %v", args)
	}
}

func TestUpdateProviderInfo_AllowsEmptyDescription(t *testing.T) {
	s, backend := newWriteTestService(t)

	if _, err := s.UpdateProviderInfo(context.Background(), "new-name", ""); err != nil {
		t.Fatalf("UpdateProviderInfo: %v", err)
	}
	name, args := decodeLastSentCalldata(t, backend)
	if name != "updateProviderInfo" {
		t.Fatalf("called %s", name)
	}
	if got := args[1].(string); got != "" {
		t.Fatalf("description = %q, want empty", got)
	}
}

func TestUpdateProviderInfo_RejectsEmptyName(t *testing.T) {
	s, _ := newWriteTestService(t)
	_, err := s.UpdateProviderInfo(context.Background(), "", "desc")
	if !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("got %v want ErrInvalidArgument", err)
	}
}

func TestUpdateProviderInfo_RejectsOversizedFields(t *testing.T) {
	s, _ := newWriteTestService(t)
	if _, err := s.UpdateProviderInfo(context.Background(), strings.Repeat("x", 129), "desc"); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("long name: got %v want ErrInvalidArgument", err)
	}
	if _, err := s.UpdateProviderInfo(context.Background(), "name", strings.Repeat("x", 257)); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("long description: got %v want ErrInvalidArgument", err)
	}
}

func TestRemoveProvider_Broadcasts(t *testing.T) {
	s, backend := newWriteTestService(t)
	if _, err := s.RemoveProvider(context.Background()); err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}
	name, _ := decodeLastSentCalldata(t, backend)
	if name != "removeProvider" {
		t.Fatalf("called %s", name)
	}
}

func TestAddPDPProduct_EncodesCalldata(t *testing.T) {
	s, backend := newWriteTestService(t)
	if _, err := s.AddPDPProduct(context.Background(), sampleOffering(), nil); err != nil {
		t.Fatalf("AddPDPProduct: %v", err)
	}
	name, args := decodeLastSentCalldata(t, backend)
	if name != "addProduct" {
		t.Fatalf("called %s", name)
	}
	if args[0].(uint8) != uint8(ProductTypePDP) {
		t.Errorf("productType = %d", args[0])
	}
	keys := args[1].([]string)
	if len(keys) < 7 || keys[0] != CapServiceURL {
		t.Errorf("unexpected keys: %v", keys)
	}
}

func TestUpdatePDPProduct_EncodesCalldata(t *testing.T) {
	s, backend := newWriteTestService(t)
	if _, err := s.UpdatePDPProduct(context.Background(), sampleOffering(), nil); err != nil {
		t.Fatalf("UpdatePDPProduct: %v", err)
	}
	name, args := decodeLastSentCalldata(t, backend)
	if name != "updateProduct" {
		t.Fatalf("called %s", name)
	}
	if args[0].(uint8) != uint8(ProductTypePDP) {
		t.Errorf("productType = %d", args[0])
	}
}

func TestAddPDPProduct_InvalidOfferingRejected(t *testing.T) {
	s, backend := newWriteTestService(t)
	bad := sampleOffering()
	bad.ServiceURL = ""
	if _, err := s.AddPDPProduct(context.Background(), bad, nil); !errors.Is(err, ErrInvalidOffering) {
		t.Errorf("got %v", err)
	}
	if len(backend.sent) != 0 {
		t.Errorf("should not broadcast invalid offering; sent=%d", len(backend.sent))
	}
}

func TestRemoveProduct_Broadcasts(t *testing.T) {
	s, backend := newWriteTestService(t)
	if _, err := s.RemoveProduct(context.Background(), ProductTypePDP); err != nil {
		t.Fatalf("RemoveProduct: %v", err)
	}
	name, args := decodeLastSentCalldata(t, backend)
	if name != "removeProduct" {
		t.Fatalf("called %s", name)
	}
	if args[0].(uint8) != uint8(ProductTypePDP) {
		t.Errorf("productType = %d", args[0])
	}
}

// Sanity-check for wiring: an ad-hoc use of the ABI decoder outside a
// live test keeps unused-import warnings at bay when the bindings evolve.
var _ = abi.Arguments{}

// TestFinalize_RevertPopulatesReceipt ensures that when the receipt is
// mined with Status == 0 the error wraps types.ErrTxFailed *and* the
// WriteResult still carries the populated receipt for inspection. This
// guards against regressions in the ErrTxFailed alias chain
// (types.ErrTxFailed = txutil.ErrTxFailed = spregistry.ErrTxFailed) and
// the double-wrap fix at service.go:182.
func TestFinalize_RevertPopulatesReceipt(t *testing.T) {
	s, backend := newWriteTestService(t)
	backend.mu.Lock()
	backend.receiptFn = func(_ context.Context, hash common.Hash) (*coretypes.Receipt, error) {
		return &coretypes.Receipt{
			Status:      0,
			TxHash:      hash,
			BlockNumber: big.NewInt(12),
		}, nil
	}
	backend.mu.Unlock()

	res, err := s.RemoveProvider(context.Background(), WithWait(2*time.Second))
	if err == nil {
		t.Fatal("expected error from reverted receipt")
	}
	if !errors.Is(err, ErrTxFailed) {
		t.Errorf("errors.Is(err, spregistry.ErrTxFailed) = false; err=%v", err)
	}
	if res == nil || res.Receipt == nil {
		t.Fatalf("expected receipt populated on revert; got res=%+v", res)
	}
	if res.Receipt.Status != 0 {
		t.Errorf("receipt status = %d, want 0", res.Receipt.Status)
	}
	if res.Hash == (common.Hash{}) {
		t.Error("result.Hash is zero")
	}
}

// TestFinalize_Success verifies the happy path: receipt Status==1 yields
// no error and the populated Receipt is returned to the caller.
func TestFinalize_Success(t *testing.T) {
	s, backend := newWriteTestService(t)
	backend.mu.Lock()
	backend.receiptFn = func(_ context.Context, hash common.Hash) (*coretypes.Receipt, error) {
		return &coretypes.Receipt{
			Status:      1,
			TxHash:      hash,
			BlockNumber: big.NewInt(12),
			GasUsed:     21000,
		}, nil
	}
	backend.mu.Unlock()

	res, err := s.RemoveProvider(context.Background(), WithWait(2*time.Second))
	if err != nil {
		t.Fatalf("RemoveProvider: %v", err)
	}
	if res.Receipt == nil || res.Receipt.Status != 1 {
		t.Fatalf("expected success receipt; got %+v", res.Receipt)
	}
}

// TestFinalize_Timeout verifies that when the receipt never materialises
// within the WithWait window the error is surfaced and the nonce lock is
// still released (otherwise the next call would deadlock).
func TestFinalize_Timeout(t *testing.T) {
	s, backend := newWriteTestService(t)
	// receiptFn returning NotFound simulates a still-pending tx.
	backend.mu.Lock()
	backend.receiptFn = func(_ context.Context, _ common.Hash) (*coretypes.Receipt, error) {
		return nil, ethereum.NotFound
	}
	backend.mu.Unlock()

	_, err := s.RemoveProvider(context.Background(), WithWait(50*time.Millisecond))
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Second call must succeed — if the nonce lock had leaked the
	// Acquire() call inside newTransactOpts would block forever.
	backend.mu.Lock()
	backend.receiptFn = func(_ context.Context, hash common.Hash) (*coretypes.Receipt, error) {
		return &coretypes.Receipt{Status: 1, TxHash: hash, BlockNumber: big.NewInt(13)}, nil
	}
	backend.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := s.RemoveProvider(ctx, WithWait(2*time.Second)); err != nil {
		t.Fatalf("second call failed — nonce lock likely leaked: %v", err)
	}
}
