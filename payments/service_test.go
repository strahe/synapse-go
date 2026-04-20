package payments

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	erc20bind "github.com/strahe/synapse-go/internal/contracts/erc20"
	filpaybind "github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/txutil"
	"github.com/strahe/synapse-go/signer"
)

// mockBackend implements the Backend interface and captures every broadcast
// transaction for later inspection.
type mockBackend struct {
	mu sync.Mutex

	filPayABI abi.ABI
	erc20ABI  abi.ABI

	// replies keyed by (contract-address-hex, method-name)
	replies map[string][]byte
	// errors keyed the same way
	errs map[string]error

	// per-account balances; nil -> 0
	balances map[common.Address]*big.Int

	// sent transactions (hash -> tx)
	sent      []*types.Transaction
	receipts  map[common.Hash]*types.Receipt
	receiptFn func(context.Context, common.Hash) (*types.Receipt, error)
	blockFn   func(context.Context) (uint64, error)

	nonces map[common.Address]uint64

	// sendErr, when set, makes SendTransaction return this error
	sendErr error
	// estimateGasErr, when set, makes EstimateGas return this error
	estimateGasErr error
}

func newMockBackend(t *testing.T) *mockBackend {
	t.Helper()
	fp, err := filpaybind.FilPayMetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	ea, err := erc20bind.ERC20MetaData.GetAbi()
	if err != nil {
		t.Fatal(err)
	}
	return &mockBackend{
		filPayABI: *fp,
		erc20ABI:  *ea,
		replies:   map[string][]byte{},
		errs:      map[string]error{},
		balances:  map[common.Address]*big.Int{},
		receipts:  map[common.Hash]*types.Receipt{},
		nonces:    map[common.Address]uint64{},
	}
}

// --- ContractCaller ---

func (m *mockBackend) CodeAt(_ context.Context, _ common.Address, _ *big.Int) ([]byte, error) {
	return []byte{0x01}, nil
}

func (m *mockBackend) CallContract(_ context.Context, call ethereum.CallMsg, _ *big.Int) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(call.Data) < 4 || call.To == nil {
		return nil, errors.New("calldata too short")
	}
	selector := [4]byte{call.Data[0], call.Data[1], call.Data[2], call.Data[3]}
	toHex := call.To.Hex()
	for name, method := range m.filPayABI.Methods {
		if [4]byte(method.ID) == selector {
			key := toHex + ":" + name
			if err, ok := m.errs[key]; ok {
				return nil, err
			}
			return m.replies[key], nil
		}
	}
	for name, method := range m.erc20ABI.Methods {
		if [4]byte(method.ID) == selector {
			key := toHex + ":" + name
			if err, ok := m.errs[key]; ok {
				return nil, err
			}
			return m.replies[key], nil
		}
	}
	return nil, errors.New("no method matches selector")
}

// --- ContractTransactor ---

func (m *mockBackend) HeaderByNumber(_ context.Context, _ *big.Int) (*types.Header, error) {
	return &types.Header{BaseFee: big.NewInt(1_000_000_000), Number: big.NewInt(1)}, nil
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
	return big.NewInt(1), nil
}

func (m *mockBackend) EstimateGas(_ context.Context, _ ethereum.CallMsg) (uint64, error) {
	if m.estimateGasErr != nil {
		return 0, m.estimateGasErr
	}
	return 100_000, nil
}

func (m *mockBackend) SendTransaction(_ context.Context, tx *types.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, tx)
	return nil
}

// --- ContractFilterer ---

func (m *mockBackend) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	return nil, nil
}

func (m *mockBackend) SubscribeFilterLogs(_ context.Context, _ ethereum.FilterQuery, _ chan<- types.Log) (ethereum.Subscription, error) {
	return nil, errors.New("subscription not supported")
}

// --- extras ---

func (m *mockBackend) BalanceAt(_ context.Context, account common.Address, _ *big.Int) (*big.Int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.balances[account]; ok {
		return new(big.Int).Set(v), nil
	}
	return big.NewInt(0), nil
}

func (m *mockBackend) TransactionReceipt(ctx context.Context, h common.Hash) (*types.Receipt, error) {
	if m.receiptFn != nil {
		return m.receiptFn(ctx, h)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.receipts[h]
	if !ok {
		return nil, ethereum.NotFound
	}
	return r, nil
}

func (m *mockBackend) BlockNumber(ctx context.Context) (uint64, error) {
	if m.blockFn != nil {
		return m.blockFn(ctx)
	}
	return 10, nil
}

// helpers

func (m *mockBackend) setFilPayReply(t *testing.T, filPayAddr common.Address, method string, values ...any) {
	t.Helper()
	mth, ok := m.filPayABI.Methods[method]
	if !ok {
		t.Fatalf("filpay method %q not found", method)
	}
	b, err := mth.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	m.replies[filPayAddr.Hex()+":"+method] = b
}

func (m *mockBackend) setERC20Reply(t *testing.T, token common.Address, method string, values ...any) {
	t.Helper()
	mth, ok := m.erc20ABI.Methods[method]
	if !ok {
		t.Fatalf("erc20 method %q not found", method)
	}
	b, err := mth.Outputs.Pack(values...)
	if err != nil {
		t.Fatalf("pack %s: %v", method, err)
	}
	m.replies[token.Hex()+":"+method] = b
}

// --- fixtures ---

var (
	tokenAddr    = common.HexToAddress("0x1111111111111111111111111111111111111111")
	filPayAddr   = common.HexToAddress("0x2222222222222222222222222222222222222222")
	operatorAddr = common.HexToAddress("0x3333333333333333333333333333333333333333")
	otherAddr    = common.HexToAddress("0x4444444444444444444444444444444444444444")
)

func newTestSigner(t *testing.T) signer.EVMSigner {
	t.Helper()
	k, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := signer.NewSecp256k1Signer(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newTestServiceWith(t *testing.T, sg signer.EVMSigner) (*Service, *mockBackend) {
	t.Helper()
	mb := newMockBackend(t)
	s, err := New(Options{
		Backend:       mb,
		ChainID:       big.NewInt(314159),
		FilPayAddress: filPayAddr,
		Signer:        sg,
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, mb
}

func newTestService(t *testing.T) (*Service, *mockBackend) {
	t.Helper()
	return newTestServiceWith(t, newTestSigner(t))
}

func TestNewValidation(t *testing.T) {
	mb := newMockBackend(t)
	if _, err := New(Options{ChainID: big.NewInt(1), FilPayAddress: filPayAddr}); err == nil {
		t.Error("expected nil Backend error")
	}
	if _, err := New(Options{Backend: mb, FilPayAddress: filPayAddr}); err == nil {
		t.Error("expected nil ChainID error")
	}
	if _, err := New(Options{Backend: mb, ChainID: big.NewInt(1)}); err == nil {
		t.Error("expected zero FilPayAddress error")
	}
}

func TestAccountInfoAndBalance(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setFilPayReply(t, filPayAddr, "accounts",
		big.NewInt(1000), big.NewInt(200), big.NewInt(5), big.NewInt(42))
	mb.setFilPayReply(t, filPayAddr, "getAccountInfoIfSettled",
		big.NewInt(0), big.NewInt(1000), big.NewInt(123), big.NewInt(5))

	info, err := s.AccountInfo(context.Background(), tokenAddr, owner)
	if err != nil {
		t.Fatal(err)
	}
	if info.Funds.Int64() != 1000 || info.LockupCurrent.Int64() != 200 {
		t.Errorf("unexpected: %+v", info)
	}
	if avail := info.AvailableFunds(); avail.Int64() != 123 {
		t.Errorf("AvailableFunds = %s, want 123", avail)
	}
	bal, err := s.Balance(context.Background(), tokenAddr, owner)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Int64() != 1000 {
		t.Errorf("Balance = %s, want 1000", bal)
	}

	if _, err := s.AccountInfo(context.Background(), common.Address{}, owner); err != nil {
		t.Errorf("zero token (FIL path): unexpected error: %v", err)
	}
}

func TestWalletBalance_FIL(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.balances[owner] = big.NewInt(5_000_000)
	bal, err := s.WalletBalance(context.Background(), common.Address{}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Int64() != 5_000_000 {
		t.Errorf("wallet bal = %s", bal)
	}
}

func TestWalletBalance_ERC20(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setERC20Reply(t, tokenAddr, "balanceOf", big.NewInt(42))
	bal, err := s.WalletBalance(context.Background(), tokenAddr, owner)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Int64() != 42 {
		t.Errorf("balance = %s", bal)
	}
}

func TestAllowance(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setERC20Reply(t, tokenAddr, "allowance", big.NewInt(99))
	v, err := s.Allowance(context.Background(), tokenAddr, owner, filPayAddr)
	if err != nil {
		t.Fatal(err)
	}
	if v.Int64() != 99 {
		t.Errorf("allowance = %s", v)
	}
}

func TestServiceApproval(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setFilPayReply(t, filPayAddr, "operatorApprovals",
		true, big.NewInt(10), big.NewInt(20), big.NewInt(3), big.NewInt(4), big.NewInt(1000))

	ap, err := s.ServiceApproval(context.Background(), tokenAddr, owner, operatorAddr)
	if err != nil {
		t.Fatal(err)
	}
	if !ap.IsApproved {
		t.Error("IsApproved = false, want true")
	}
	if ap.RateAllowance.Int64() != 10 {
		t.Errorf("RateAllowance = %s, want 10", ap.RateAllowance)
	}
	if ap.LockupAllowance.Int64() != 20 {
		t.Errorf("LockupAllowance = %s, want 20", ap.LockupAllowance)
	}
	if ap.RateUsage.Int64() != 3 {
		t.Errorf("RateUsage = %s, want 3", ap.RateUsage)
	}
	if ap.LockupUsage.Int64() != 4 {
		t.Errorf("LockupUsage = %s, want 4", ap.LockupUsage)
	}
	if ap.MaxLockupPeriod.Int64() != 1000 {
		t.Errorf("MaxLockupPeriod = %s, want 1000", ap.MaxLockupPeriod)
	}
}

func TestApprove_Broadcasts(t *testing.T) {
	s, mb := newTestService(t)
	res, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(500))
	if err != nil {
		t.Fatal(err)
	}
	if res.Hash == (common.Hash{}) {
		t.Error("empty hash")
	}
	if got := len(mb.sent); got != 1 {
		t.Fatalf("sent = %d", got)
	}
	tx := mb.sent[0]
	if *tx.To() != tokenAddr {
		t.Errorf("tx.To = %s", tx.To().Hex())
	}
	// Decode calldata
	method, ok := mb.erc20ABI.Methods["approve"]
	if !ok {
		t.Fatal("no approve method")
	}
	if !bytes.Equal(tx.Data()[:4], method.ID) {
		t.Errorf("selector mismatch")
	}
	args, err := method.Inputs.Unpack(tx.Data()[4:])
	if err != nil {
		t.Fatal(err)
	}
	if args[0].(common.Address) != filPayAddr {
		t.Errorf("spender = %s", args[0])
	}
	if args[1].(*big.Int).Int64() != 500 {
		t.Errorf("amount = %s", args[1])
	}
}

func TestDeposit_PrecheckInsufficientBalance(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setERC20Reply(t, tokenAddr, "balanceOf", big.NewInt(10))
	mb.setERC20Reply(t, tokenAddr, "allowance", big.NewInt(1_000_000))
	_, err := s.Deposit(context.Background(), tokenAddr, owner, big.NewInt(100))
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("expected ErrInsufficientBalance, got %v", err)
	}
}

func TestDeposit_PrecheckInsufficientAllowance(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setERC20Reply(t, tokenAddr, "balanceOf", big.NewInt(1_000_000))
	mb.setERC20Reply(t, tokenAddr, "allowance", big.NewInt(1))
	_, err := s.Deposit(context.Background(), tokenAddr, owner, big.NewInt(100))
	if !errors.Is(err, ErrInsufficientAllowance) {
		t.Errorf("expected ErrInsufficientAllowance, got %v", err)
	}
}

func TestDeposit_Broadcasts(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setERC20Reply(t, tokenAddr, "balanceOf", big.NewInt(1_000_000))
	mb.setERC20Reply(t, tokenAddr, "allowance", big.NewInt(1_000_000))

	res, err := s.Deposit(context.Background(), tokenAddr, common.Address{}, big.NewInt(100))
	if err != nil {
		t.Fatal(err)
	}
	if res.Hash == (common.Hash{}) {
		t.Error("empty hash")
	}
	if len(mb.sent) != 1 {
		t.Fatalf("sent = %d", len(mb.sent))
	}
	tx := mb.sent[0]
	if *tx.To() != filPayAddr {
		t.Errorf("tx.To = %s", tx.To().Hex())
	}
	method := mb.filPayABI.Methods["deposit"]
	args, err := method.Inputs.Unpack(tx.Data()[4:])
	if err != nil {
		t.Fatal(err)
	}
	if args[0].(common.Address) != tokenAddr {
		t.Errorf("token = %s", args[0])
	}
	if args[1].(common.Address) != owner {
		t.Errorf("to = %s, want owner %s", args[1], owner.Hex())
	}
	if args[2].(*big.Int).Int64() != 100 {
		t.Errorf("amount = %s", args[2])
	}
}

func TestWithdraw_PrecheckInsufficient(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "accounts",
		big.NewInt(100), big.NewInt(90), big.NewInt(0), big.NewInt(0))
	mb.setFilPayReply(t, filPayAddr, "getAccountInfoIfSettled",
		big.NewInt(0), big.NewInt(100), big.NewInt(10), big.NewInt(0))
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(50))
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("expected ErrInsufficientBalance, got %v", err)
	}
}

func TestWithdraw_Broadcasts(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "accounts",
		big.NewInt(1000), big.NewInt(0), big.NewInt(0), big.NewInt(0))
	mb.setFilPayReply(t, filPayAddr, "getAccountInfoIfSettled",
		big.NewInt(0), big.NewInt(1000), big.NewInt(1000), big.NewInt(0))
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(400))
	if err != nil {
		t.Fatal(err)
	}
	if len(mb.sent) != 1 {
		t.Fatal("tx not sent")
	}
	method := mb.filPayABI.Methods["withdraw"]
	args, _ := method.Inputs.Unpack(mb.sent[0].Data()[4:])
	if args[0].(common.Address) != tokenAddr || args[1].(*big.Int).Int64() != 400 {
		t.Errorf("args %v", args)
	}
}

func TestApproveService_PassesArgs(t *testing.T) {
	s, mb := newTestService(t)
	_, err := s.ApproveService(context.Background(), tokenAddr, operatorAddr,
		big.NewInt(10), big.NewInt(20), big.NewInt(1000))
	if err != nil {
		t.Fatal(err)
	}
	method := mb.filPayABI.Methods["setOperatorApproval"]
	args, _ := method.Inputs.Unpack(mb.sent[0].Data()[4:])
	if args[0].(common.Address) != tokenAddr {
		t.Errorf("token")
	}
	if args[1].(common.Address) != operatorAddr {
		t.Errorf("operator")
	}
	if args[2].(bool) != true {
		t.Errorf("approved = %v", args[2])
	}
	if args[3].(*big.Int).Int64() != 10 {
		t.Errorf("rate = %s", args[3])
	}
	if args[4].(*big.Int).Int64() != 20 {
		t.Errorf("lockup = %s", args[4])
	}
	if args[5].(*big.Int).Int64() != 1000 {
		t.Errorf("maxPeriod = %s", args[5])
	}
}

func TestRevokeService_ZeroesAllowances(t *testing.T) {
	s, mb := newTestService(t)
	_, err := s.RevokeService(context.Background(), tokenAddr, operatorAddr)
	if err != nil {
		t.Fatal(err)
	}
	method := mb.filPayABI.Methods["setOperatorApproval"]
	args, _ := method.Inputs.Unpack(mb.sent[0].Data()[4:])
	if args[2].(bool) != false {
		t.Errorf("approved should be false")
	}
	if args[3].(*big.Int).Sign() != 0 || args[4].(*big.Int).Sign() != 0 || args[5].(*big.Int).Sign() != 0 {
		t.Errorf("allowances should be zero: %v", args[3:])
	}
}

func TestValidation_NegativeAmounts(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	neg := big.NewInt(-1)
	if _, err := s.Approve(ctx, tokenAddr, filPayAddr, neg); err == nil {
		t.Error("expected negative error")
	}
	if _, err := s.Deposit(ctx, tokenAddr, otherAddr, neg); err == nil {
		t.Error("expected negative error")
	}
	if _, err := s.Withdraw(ctx, tokenAddr, neg); err == nil {
		t.Error("expected negative error")
	}
	if _, err := s.ApproveService(ctx, tokenAddr, operatorAddr, neg, big.NewInt(0), big.NewInt(0)); err == nil {
		t.Error("expected negative error")
	}
}

func TestNoSigner_WritesFail(t *testing.T) {
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: big.NewInt(1), FilPayAddress: filPayAddr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(1)); err == nil {
		t.Error("expected signer required error")
	}
}

func TestFinalize_WithWait(t *testing.T) {
	s, mb := newTestService(t)

	// Predict the next tx hash by broadcasting with WithWait and a short
	// poll; register the receipt concurrently.
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Wait for the tx to be broadcast, then seed its receipt.
		for {
			mb.mu.Lock()
			n := len(mb.sent)
			mb.mu.Unlock()
			if n > 0 {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		mb.mu.Lock()
		h := mb.sent[0].Hash()
		mb.receipts[h] = &types.Receipt{Status: types.ReceiptStatusSuccessful, BlockNumber: big.NewInt(5)}
		mb.mu.Unlock()
	}()

	res, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(1),
		WithWait(5*time.Second))
	<-done
	if err != nil {
		t.Fatalf("WithWait: %v", err)
	}
	if res.Receipt == nil || res.Receipt.Status != types.ReceiptStatusSuccessful {
		t.Errorf("expected successful receipt, got %+v", res.Receipt)
	}
}

func TestFinalize_WithConfirmations_UsesWaitTimeout(t *testing.T) {
	s, mb := newTestService(t)
	mb.receiptFn = func(ctx context.Context, _ common.Hash) (*types.Receipt, error) {
		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("expected receipt polling context to have deadline")
		}
		if time.Until(deadline) > 250*time.Millisecond {
			return nil, errors.New("wait timeout was not propagated")
		}
		return nil, ethereum.NotFound
	}

	_, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(1),
		WithWait(25*time.Millisecond), WithConfirmations(2))
	if !errors.Is(err, txutil.ErrReceiptTimeout) {
		t.Fatalf("expected ErrReceiptTimeout, got %v", err)
	}
}

func TestFinalize_RevertedTx_ReleasesNonce(t *testing.T) {
	s, mb := newTestService(t)
	mb.receiptFn = func(context.Context, common.Hash) (*types.Receipt, error) {
		return &types.Receipt{Status: types.ReceiptStatusFailed, BlockNumber: big.NewInt(5)}, nil
	}

	res, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(1), WithWait(3*time.Second))
	if !errors.Is(err, ErrTxFailed) {
		t.Fatalf("expected ErrTxFailed, got %v", err)
	}
	if res == nil || res.Receipt == nil || res.Receipt.Status != types.ReceiptStatusFailed {
		t.Fatalf("expected failed receipt in result, got %+v", res)
	}
	if got := s.nonces.PendingCount(); got != 0 {
		t.Fatalf("expected nonce reservation released, got pending=%d", got)
	}
}

func TestFinalize_NoWait_ReleasesNonce(t *testing.T) {
	s, mb := newTestService(t)
	tx := types.NewTx(&types.LegacyTx{Nonce: 7, To: &filPayAddr})
	mb.nonces[s.Account()] = 7
	s.nonces = txutil.NewNonceManager(mb, s.Account())
	n, err := s.nonces.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("expected reserved nonce 7, got %d", n)
	}
	if got := s.nonces.PendingCount(); got != 1 {
		t.Fatalf("expected 1 pending before finalize, got %d", got)
	}

	res, err := s.finalize(context.Background(), tx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Hash != tx.Hash() {
		t.Fatalf("hash = %s, want %s", res.Hash, tx.Hash())
	}
	if got := s.nonces.PendingCount(); got != 0 {
		t.Fatalf("expected nonce reservation released, got pending=%d", got)
	}
}

func TestAddress(t *testing.T) {
	s, _ := newTestService(t)
	if got := s.Address(); got != filPayAddr {
		t.Errorf("Address() = %s, want %s", got.Hex(), filPayAddr.Hex())
	}
}

func TestChainID(t *testing.T) {
	s, _ := newTestService(t)
	got := s.ChainID()
	if got.Int64() != 314159 {
		t.Errorf("ChainID() = %s, want 314159", got)
	}
	// Ensure it's a copy, not the same pointer.
	got.SetInt64(0)
	if s.ChainID().Int64() != 314159 {
		t.Error("ChainID returned the internal pointer, not a copy")
	}
}

func TestWithSkipPrecheck(t *testing.T) {
	cfg := newWriteConfig([]WriteOption{WithSkipPrecheck()})
	if !cfg.skipPrecheck {
		t.Error("WithSkipPrecheck did not set skipPrecheck")
	}
}

func TestAvailableFunds_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		a    *AccountState
		want *big.Int
	}{
		{
			name: "nil receiver",
			a:    nil,
			want: nil,
		},
		{
			name: "precomputed availableFunds",
			a:    &AccountState{availableFunds: big.NewInt(42)},
			want: big.NewInt(42),
		},
		{
			name: "nil Funds",
			a:    &AccountState{Funds: nil, LockupCurrent: big.NewInt(1)},
			want: nil,
		},
		{
			name: "nil LockupCurrent",
			a:    &AccountState{Funds: big.NewInt(100), LockupCurrent: nil},
			want: nil,
		},
		{
			name: "negative clamped to zero",
			a:    &AccountState{Funds: big.NewInt(10), LockupCurrent: big.NewInt(20)},
			want: big.NewInt(0),
		},
		{
			name: "positive difference",
			a:    &AccountState{Funds: big.NewInt(100), LockupCurrent: big.NewInt(30)},
			want: big.NewInt(70),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.a.AvailableFunds()
			if tc.want == nil {
				if got != nil {
					t.Fatalf("want nil, got %s", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("want %s, got nil", tc.want)
			}
			if got.Cmp(tc.want) != 0 {
				t.Errorf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestServiceApproval_ZeroAddresses(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	owner := s.Account()

	if _, err := s.ServiceApproval(ctx, common.Address{}, owner, operatorAddr); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero token: want ErrZeroAddress, got %v", err)
	}
	if _, err := s.ServiceApproval(ctx, tokenAddr, common.Address{}, operatorAddr); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero client: want ErrZeroAddress, got %v", err)
	}
	if _, err := s.ServiceApproval(ctx, tokenAddr, owner, common.Address{}); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero operator: want ErrZeroAddress, got %v", err)
	}
}

func TestServiceApproval_RPCError(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.errs[filPayAddr.Hex()+":operatorApprovals"] = errors.New("rpc down")
	_, err := s.ServiceApproval(context.Background(), tokenAddr, owner, operatorAddr)
	if err == nil {
		t.Error("expected error")
	}
}

func TestRevokeService_ZeroAddresses(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	if _, err := s.RevokeService(ctx, common.Address{}, operatorAddr); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero token: want ErrZeroAddress, got %v", err)
	}
	if _, err := s.RevokeService(ctx, tokenAddr, common.Address{}); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero operator: want ErrZeroAddress, got %v", err)
	}
}

func TestRevokeService_NoSigner(t *testing.T) {
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: big.NewInt(1), FilPayAddress: filPayAddr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RevokeService(context.Background(), tokenAddr, operatorAddr); err == nil {
		t.Error("expected signer required error")
	}
}

func TestAllowance_ZeroAddresses(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	owner := s.Account()

	if _, err := s.Allowance(ctx, common.Address{}, owner, filPayAddr); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero token: want ErrZeroAddress, got %v", err)
	}
	if _, err := s.Allowance(ctx, tokenAddr, common.Address{}, filPayAddr); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero owner: want ErrZeroAddress, got %v", err)
	}
	if _, err := s.Allowance(ctx, tokenAddr, owner, common.Address{}); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero spender: want ErrZeroAddress, got %v", err)
	}
}

func TestApprove_ZeroAddresses(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	if _, err := s.Approve(ctx, common.Address{}, filPayAddr, big.NewInt(1)); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero token: want ErrZeroAddress, got %v", err)
	}
	if _, err := s.Approve(ctx, tokenAddr, common.Address{}, big.NewInt(1)); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero spender: want ErrZeroAddress, got %v", err)
	}
}

func TestWithdraw_ZeroToken(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.Withdraw(context.Background(), common.Address{}, big.NewInt(1)); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero token: want ErrZeroAddress, got %v", err)
	}
}

func TestWithdraw_ZeroAmount(t *testing.T) {
	s, mb := newTestService(t)
	mb.setFilPayReply(t, filPayAddr, "accounts",
		big.NewInt(1000), big.NewInt(0), big.NewInt(0), big.NewInt(0))
	mb.setFilPayReply(t, filPayAddr, "getAccountInfoIfSettled",
		big.NewInt(0), big.NewInt(1000), big.NewInt(1000), big.NewInt(0))
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(0))
	if err != nil {
		t.Fatalf("zero amount withdraw should succeed, got %v", err)
	}
}

func TestWithdraw_SkipPrecheck(t *testing.T) {
	s, _ := newTestService(t)
	// No mock replies for AccountInfo — precheck would fail. WithSkipPrecheck bypasses it.
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(1), WithSkipPrecheck())
	if err != nil {
		t.Fatalf("withdraw with skip precheck should succeed, got %v", err)
	}
}

func TestAccountInfo_ZeroOwner(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.AccountInfo(context.Background(), tokenAddr, common.Address{}); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero owner: want ErrZeroAddress, got %v", err)
	}
}

func TestAccountInfo_RPCError(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.errs[filPayAddr.Hex()+":accounts"] = errors.New("rpc down")
	if _, err := s.AccountInfo(context.Background(), tokenAddr, owner); err == nil {
		t.Error("expected RPC error")
	}
}

func TestAccountInfo_SettledError(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setFilPayReply(t, filPayAddr, "accounts",
		big.NewInt(1000), big.NewInt(200), big.NewInt(5), big.NewInt(42))
	mb.errs[filPayAddr.Hex()+":getAccountInfoIfSettled"] = errors.New("rpc down")
	if _, err := s.AccountInfo(context.Background(), tokenAddr, owner); err == nil {
		t.Error("expected settled RPC error")
	}
}

func TestBalance_Error(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.errs[filPayAddr.Hex()+":accounts"] = errors.New("rpc down")
	if _, err := s.Balance(context.Background(), tokenAddr, owner); err == nil {
		t.Error("expected error from Balance")
	}
}

func TestWalletBalance_ZeroAccount(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.WalletBalance(context.Background(), tokenAddr, common.Address{}); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero account: want ErrZeroAddress, got %v", err)
	}
}

func TestWalletBalance_ERC20Error(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.errs[tokenAddr.Hex()+":balanceOf"] = errors.New("rpc down")
	if _, err := s.WalletBalance(context.Background(), tokenAddr, owner); err == nil {
		t.Error("expected error from WalletBalance ERC20 path")
	}
}

func TestReleaseNonce_NilGuards(t *testing.T) {
	s, _ := newTestService(t)
	// nil nonce — should not panic
	s.releaseNonce(nil)
	// nil NonceManager — should not panic
	orig := s.nonces
	s.nonces = nil
	s.releaseNonce(big.NewInt(7))
	s.nonces = orig
}

func TestDeposit_SkipPrecheck(t *testing.T) {
	s, _ := newTestService(t)
	// No mock replies for balance/allowance — precheck would fail.
	_, err := s.Deposit(context.Background(), tokenAddr, common.Address{}, big.NewInt(1), WithSkipPrecheck())
	if err != nil {
		t.Fatalf("deposit with skip precheck should succeed, got %v", err)
	}
}

func TestApproveService_ZeroAddresses(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()

	if _, err := s.ApproveService(ctx, common.Address{}, operatorAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0)); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero token: want ErrZeroAddress, got %v", err)
	}
	if _, err := s.ApproveService(ctx, tokenAddr, common.Address{}, big.NewInt(0), big.NewInt(0), big.NewInt(0)); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero operator: want ErrZeroAddress, got %v", err)
	}
}

// ---------- additional coverage ----------

func TestAccount_NilSigner(t *testing.T) {
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: big.NewInt(1), FilPayAddress: filPayAddr})
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Account(); (got != common.Address{}) {
		t.Errorf("Account() = %s, want zero address", got.Hex())
	}
}

func TestNew_Validation(t *testing.T) {
	mb := newMockBackend(t)
	tests := []struct {
		name string
		opts Options
	}{
		{"nil backend", Options{ChainID: big.NewInt(1), FilPayAddress: filPayAddr}},
		{"nil chainID", Options{Backend: mb, FilPayAddress: filPayAddr}},
		{"zero chainID", Options{Backend: mb, ChainID: big.NewInt(0), FilPayAddress: filPayAddr}},
		{"negative chainID", Options{Backend: mb, ChainID: big.NewInt(-1), FilPayAddress: filPayAddr}},
		{"zero filpay", Options{Backend: mb, ChainID: big.NewInt(1)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestWalletBalance_NativeFIL(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.balances[owner] = big.NewInt(999)
	bal, err := s.WalletBalance(context.Background(), common.Address{}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Int64() != 999 {
		t.Errorf("native FIL balance = %s, want 999", bal)
	}
}

func TestAllowance_RPCError(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.errs[tokenAddr.Hex()+":allowance"] = errors.New("rpc down")
	if _, err := s.Allowance(context.Background(), tokenAddr, owner, filPayAddr); err == nil {
		t.Error("expected error from Allowance RPC path")
	}
}

func TestApprove_NegativeAmount(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(-1))
	if err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestApprove_NoSigner(t *testing.T) {
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: big.NewInt(1), FilPayAddress: filPayAddr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(1)); err == nil {
		t.Error("expected signer required error")
	}
}

func TestDeposit_ZeroToken(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.Deposit(context.Background(), common.Address{}, otherAddr, big.NewInt(1), WithSkipPrecheck()); !errors.Is(err, ErrZeroAddress) {
		t.Errorf("zero token: want ErrZeroAddress, got %v", err)
	}
}

func TestDeposit_NegativeAmount(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.Deposit(context.Background(), tokenAddr, otherAddr, big.NewInt(-1), WithSkipPrecheck())
	if err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestDeposit_PrecheckBalanceError(t *testing.T) {
	s, mb := newTestService(t)
	mb.errs[tokenAddr.Hex()+":balanceOf"] = errors.New("rpc down")
	_, err := s.Deposit(context.Background(), tokenAddr, otherAddr, big.NewInt(1))
	if err == nil {
		t.Error("expected error from balance precheck")
	}
}

func TestDeposit_PrecheckAllowanceError(t *testing.T) {
	s, mb := newTestService(t)
	mb.setERC20Reply(t, tokenAddr, "balanceOf", big.NewInt(1000))
	mb.errs[tokenAddr.Hex()+":allowance"] = errors.New("rpc down")
	_, err := s.Deposit(context.Background(), tokenAddr, otherAddr, big.NewInt(1))
	if err == nil {
		t.Error("expected error from allowance precheck")
	}
}

func TestWithdraw_PrecheckInsufficientFunds(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setFilPayReply(t, filPayAddr, "accounts",
		big.NewInt(100), big.NewInt(80), big.NewInt(0), big.NewInt(0))
	mb.setFilPayReply(t, filPayAddr, "getAccountInfoIfSettled",
		big.NewInt(0), big.NewInt(20), big.NewInt(20), big.NewInt(0))
	_ = owner
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(50))
	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("want ErrInsufficientBalance, got %v", err)
	}
}

func TestWithdraw_PrecheckError(t *testing.T) {
	s, mb := newTestService(t)
	mb.errs[filPayAddr.Hex()+":accounts"] = errors.New("rpc down")
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(1))
	if err == nil {
		t.Error("expected error from withdraw precheck")
	}
}

func TestWithdraw_NegativeAmount(t *testing.T) {
	s, _ := newTestService(t)
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(-1))
	if err == nil {
		t.Error("expected error for negative amount")
	}
}

func TestWithdraw_NoSigner(t *testing.T) {
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: big.NewInt(1), FilPayAddress: filPayAddr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(1)); err == nil {
		t.Error("expected signer required error")
	}
}

func TestApproveService_NegativeAllowances(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	tests := []struct {
		name                                         string
		rateAllowance, lockupAllowance, maxLockupPer *big.Int
	}{
		{"negative rate", big.NewInt(-1), big.NewInt(0), big.NewInt(0)},
		{"negative lockup", big.NewInt(0), big.NewInt(-1), big.NewInt(0)},
		{"negative maxLockup", big.NewInt(0), big.NewInt(0), big.NewInt(-1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := s.ApproveService(ctx, tokenAddr, operatorAddr, tc.rateAllowance, tc.lockupAllowance, tc.maxLockupPer)
			if err == nil {
				t.Error("expected error for negative allowance")
			}
		})
	}
}

func TestApproveService_NoSigner(t *testing.T) {
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: big.NewInt(1), FilPayAddress: filPayAddr})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ApproveService(context.Background(), tokenAddr, operatorAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0))
	if err == nil {
		t.Error("expected signer required error")
	}
}

func TestDeposit_NoSigner(t *testing.T) {
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: big.NewInt(1), FilPayAddress: filPayAddr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Deposit(context.Background(), tokenAddr, otherAddr, big.NewInt(1)); err == nil {
		t.Error("expected signer required error")
	}
}

func TestApproveService_NilAllowances(t *testing.T) {
	s, _ := newTestService(t)
	ctx := context.Background()
	_, err := s.ApproveService(ctx, tokenAddr, operatorAddr, nil, big.NewInt(0), big.NewInt(0))
	if err == nil {
		t.Error("expected error for nil rateAllowance")
	}
}

func TestCopyBig_Nil(t *testing.T) {
	if got := copyBig(nil); got != nil {
		t.Errorf("copyBig(nil) = %s, want nil", got)
	}
}

func TestValidateNonNegative_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		val     *big.Int
		wantErr bool
	}{
		{"nil", nil, true},
		{"negative", big.NewInt(-1), true},
		{"zero", big.NewInt(0), false},
		{"positive", big.NewInt(42), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateNonNegative("test", tc.val)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateNonNegative(%v) error = %v, wantErr %v", tc.val, err, tc.wantErr)
			}
		})
	}
}

func TestAccountInfo_ZeroToken(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.balances[owner] = big.NewInt(999)
	info, err := s.AccountInfo(context.Background(), common.Address{}, owner)
	if err != nil {
		t.Fatalf("zero token (FIL path): unexpected error: %v", err)
	}
	if info.Funds.Int64() != 999 {
		t.Errorf("Funds = %s, want 999", info.Funds)
	}
	if info.LockupCurrent.Sign() != 0 || info.LockupRate.Sign() != 0 ||
		info.LockupLastSettledAt.Sign() != 0 || info.FundedUntilEpoch.Sign() != 0 {
		t.Errorf("expected all lockup fields zero, got %+v", info)
	}
	if avail := info.AvailableFunds(); avail.Int64() != 999 {
		t.Errorf("AvailableFunds = %s, want 999", avail)
	}
}

func TestWalletBalance_ZeroToken(t *testing.T) {
	s, _ := newTestService(t)
	owner := s.Account()
	// zero token falls through to BalanceAt (native FIL)
	bal, err := s.WalletBalance(context.Background(), common.Address{}, owner)
	if err != nil {
		t.Fatal(err)
	}
	if bal == nil {
		t.Fatal("expected non-nil balance")
	}
}

func TestBalance_Success(t *testing.T) {
	s, mb := newTestService(t)
	owner := s.Account()
	mb.setFilPayReply(t, filPayAddr, "accounts",
		big.NewInt(500), big.NewInt(100), big.NewInt(0), big.NewInt(0))
	mb.setFilPayReply(t, filPayAddr, "getAccountInfoIfSettled",
		big.NewInt(0), big.NewInt(400), big.NewInt(400), big.NewInt(0))
	_ = owner
	bal, err := s.Balance(context.Background(), tokenAddr, owner)
	if err != nil {
		t.Fatal(err)
	}
	if bal.Int64() != 500 {
		t.Errorf("Balance() = %s, want 500", bal)
	}
}

func TestApprove_SendError(t *testing.T) {
	s, mb := newTestService(t)
	mb.sendErr = errors.New("send failed")
	_, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(1))
	if err == nil {
		t.Error("expected send error")
	}
}

func TestDeposit_SendError(t *testing.T) {
	s, mb := newTestService(t)
	mb.sendErr = errors.New("send failed")
	_, err := s.Deposit(context.Background(), tokenAddr, otherAddr, big.NewInt(1), WithSkipPrecheck())
	if err == nil {
		t.Error("expected send error")
	}
}

func TestWithdraw_SendError(t *testing.T) {
	s, mb := newTestService(t)
	mb.sendErr = errors.New("send failed")
	_, err := s.Withdraw(context.Background(), tokenAddr, big.NewInt(1), WithSkipPrecheck())
	if err == nil {
		t.Error("expected send error")
	}
}

func TestApproveService_SendError(t *testing.T) {
	s, mb := newTestService(t)
	mb.sendErr = errors.New("send failed")
	_, err := s.ApproveService(context.Background(), tokenAddr, operatorAddr, big.NewInt(0), big.NewInt(0), big.NewInt(0))
	if err == nil {
		t.Error("expected send error")
	}
}

func TestRevokeService_SendError(t *testing.T) {
	s, mb := newTestService(t)
	mb.sendErr = errors.New("send failed")
	_, err := s.RevokeService(context.Background(), tokenAddr, operatorAddr)
	if err == nil {
		t.Error("expected send error")
	}
}

func TestApprove_EstimateGasError(t *testing.T) {
	s, mb := newTestService(t)
	mb.estimateGasErr = errors.New("execution reverted: insufficient balance")
	_, err := s.Approve(context.Background(), tokenAddr, filPayAddr, big.NewInt(1))
	if err == nil {
		t.Error("expected gas estimation error")
	}
}
