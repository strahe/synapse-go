package payments

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"testing/synctest"

	ethereum "github.com/ethereum/go-ethereum"
	gethabi "github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/strahe/synapse-go/chain"
	iabi "github.com/strahe/synapse-go/internal/abi"
	"github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/retry"
	sdktypes "github.com/strahe/synapse-go/types"
)

type summaryPageReply struct {
	ids         []*big.Int
	next, total *big.Int
}

type summaryReadLog struct {
	pages   int
	batches []int
	ids     []string
}

func summaryPages(counts ...int) []summaryPageReply {
	pages := make([]summaryPageReply, len(counts))
	id := int64(1)
	for i, count := range counts {
		pages[i] = summaryPageReply{next: big.NewInt(int64(i+1) * 100), total: big.NewInt(int64(len(counts)) * 100)}
		for range count {
			pages[i].ids = append(pages[i].ids, big.NewInt(id))
			id++
		}
	}
	return pages
}

func summaryCalls(t *testing.T, mb *mockBackend, data []byte) []iabi.Call3 {
	t.Helper()
	values, err := mb.multicallABI.Methods["aggregate3"].Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatal(err)
	}
	return *gethabi.ConvertType(values[0], new([]iabi.Call3)).(*[]iabi.Call3)
}

func newSummaryFixture(t *testing.T, maxCalls int, pages []summaryPageReply) (*Service, *mockBackend, *summaryReadLog) {
	t.Helper()
	mb := newMockBackend(t)
	s, err := New(Options{Backend: mb, ChainID: 1, FilPayAddress: filPayAddr, USDFCTokenAddress: tokenAddr, MaxMulticallCalls: maxCalls})
	if err != nil {
		t.Fatal(err)
	}
	mb.blockFn = func(context.Context) (uint64, error) { return 100, nil }
	mb.setFilPayReply(t, filPayAddr, "accounts", big.NewInt(1000), big.NewInt(0), big.NewInt(0), big.NewInt(100))
	log := new(summaryReadLog)
	mb.callReplyFn = func(_, method string, data []byte) ([]byte, bool, error) {
		switch method {
		case "getRailsForPayerAndToken":
			if log.pages >= len(pages) {
				t.Fatal("unexpected extra rail page")
			}
			args, err := mb.filPayABI.Methods[method].Inputs.Unpack(data[4:])
			if err != nil {
				t.Fatal(err)
			}
			wantOffset := new(big.Int)
			if log.pages > 0 {
				wantOffset = pages[log.pages-1].next
			}
			if args[2].(*big.Int).Cmp(wantOffset) != 0 || args[3].(*big.Int).Cmp(big.NewInt(100)) != 0 {
				t.Fatalf("page offset/limit = %v/%v, want %s/100", args[2], args[3], wantOffset)
			}
			page := pages[log.pages]
			log.pages++
			rails := make([]filpay.FilecoinPayV1RailInfo, len(page.ids))
			for i, id := range page.ids {
				rails[i] = filpay.FilecoinPayV1RailInfo{RailId: id, IsTerminated: i%2 == 1, EndEpoch: big.NewInt(99)}
			}
			out, err := mb.filPayABI.Methods[method].Outputs.Pack(rails, page.next, page.total)
			return out, true, err
		case "aggregate3":
			calls := summaryCalls(t, mb, data)
			for _, call := range calls {
				if call.Target != filPayAddr || !call.AllowFailure {
					t.Fatalf("rail subcall target/AllowFailure = %s/%v", call.Target, call.AllowFailure)
				}
			}
			log.batches = append(log.batches, len(calls))
		case "getRail":
			args, err := mb.filPayABI.Methods[method].Inputs.Unpack(data[4:])
			if err != nil {
				t.Fatal(err)
			}
			id := args[0].(*big.Int)
			log.ids = append(log.ids, id.String())
			amount := new(big.Int).Add(id, big.NewInt(10))
			out, err := mb.filPayABI.Methods[method].Outputs.Pack(testRailView(otherAddr, amount))
			return out, true, err
		}
		return nil, false, nil
	}
	return s, mb, log
}

func TestTotalAccountFixedLockup_PagesAndBatchLimit(t *testing.T) {
	wideID := new(big.Int).Lsh(big.NewInt(1), 200)
	for _, tc := range []struct {
		name    string
		limit   int
		pages   []summaryPageReply
		batches []int
	}{
		{"empty account", 0, []summaryPageReply{{next: new(big.Int), total: new(big.Int)}}, nil},
		{"default across empty middle page", 0, summaryPages(70, 0, 65), []int{64, 64, 7}},
		{"one call per batch", 1, summaryPages(3, 2), []int{1, 1, 1, 1, 1}},
		{"full last page", 64, summaryPages(100), []int{64, 36}},
		{"custom limit across pages", 65, summaryPages(70, 40), []int{65, 45}},
		{"full width rail ID", 64, []summaryPageReply{
			{ids: []*big.Int{wideID}, next: big.NewInt(1), total: big.NewInt(1)},
		}, []int{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mb, log := newSummaryFixture(t, tc.limit, tc.pages)
			total, err := s.TotalAccountFixedLockup(context.Background(), otherAddr)
			if err != nil {
				t.Fatal(err)
			}
			wantTotal := new(big.Int)
			var wantIDs []string
			for _, page := range tc.pages {
				for _, id := range page.ids {
					wantTotal.Add(wantTotal, new(big.Int).Add(id, big.NewInt(10)))
					wantIDs = append(wantIDs, id.String())
				}
			}
			if total.Cmp(wantTotal) != 0 || !slices.Equal(log.batches, tc.batches) || !slices.Equal(log.ids, wantIDs) || log.pages != len(tc.pages) {
				t.Fatalf("total=%s batches=%v IDs=%v pages=%d; want %s/%v/%v/%d", total, log.batches, log.ids, log.pages, wantTotal, tc.batches, wantIDs, len(tc.pages))
			}
			for key, block := range mb.lastBlock {
				if block == nil || block.Int64() != 100 {
					t.Errorf("%s read block = %v, want 100", key, block)
				}
			}
		})
	}
	if _, err := New(Options{Backend: newMockBackend(t), ChainID: 1, FilPayAddress: filPayAddr, MaxMulticallCalls: -1}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("negative batch limit error = %v", err)
	}
}

func summaryRead(t *testing.T, s *Service, ctx context.Context, accountSummary bool) (*big.Int, error) {
	t.Helper()
	if !accountSummary {
		return s.TotalAccountFixedLockup(ctx, otherAddr)
	}
	summary, err := s.AccountSummary(ctx, otherAddr)
	if err != nil {
		if summary != nil {
			t.Error("failed AccountSummary returned a partial result")
		}
		return nil, err
	}
	if summary.CurrentEpoch.Int64() != 100 {
		t.Errorf("CurrentEpoch = %s, want 100", summary.CurrentEpoch)
	}
	return summary.TotalFixedLockup, nil
}

func TestAccountSummary_InvalidPagination(t *testing.T) {
	for _, tc := range []struct {
		name               string
		next, total, slots int64
		rails              int
	}{
		{"stalled", 100, 200, 200, 1},
		{"backward", 50, 200, 200, 1},
		{"beyond total", 201, 200, 200, 1},
		{"total changed", 150, 150, 200, 1},
		{"unfinished page advances one slot", 101, 201, 201, 1},
		{"premature completion skips slots", 300, 300, 300, 1},
		{"results exceed scanned slots", 101, 101, 101, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, summary := range []bool{false, true} {
				pages := summaryPages(3, tc.rails)
				pages[0].total = big.NewInt(tc.slots)
				pages[1].next, pages[1].total = big.NewInt(tc.next), big.NewInt(tc.total)
				s, _, log := newSummaryFixture(t, 3, pages)
				value, err := summaryRead(t, s, context.Background(), summary)
				if value != nil || !errors.Is(err, ErrInvalidRailPage) || !slices.Equal(log.batches, []int{3}) {
					t.Fatalf("summary=%v: result=%v err=%v batches=%v", summary, value, err, log.batches)
				}
			}
		})
	}
}

func TestAccountSummary_FullWidthPageValidation(t *testing.T) {
	wide := new(big.Int).Lsh(big.NewInt(1), 80)
	for _, tc := range []struct {
		name         string
		offset       *big.Int
		advance, end int64
		count        int
	}{
		{"empty intermediate page", wide, 100, 201, 0},
		{"full intermediate page", wide, 100, 201, 100},
		{"short final page", wide, 7, 7, 2},
		{"cursor crosses uint64 boundary", new(big.Int).SetUint64(^uint64(0) - 20), 100, 201, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			next := new(big.Int).Add(tc.offset, big.NewInt(tc.advance))
			total := new(big.Int).Add(tc.offset, big.NewInt(tc.end))
			if !validAccountRailPage(tc.offset, next, total, tc.count) {
				t.Fatalf("valid full-width page rejected: offset=%s next=%s total=%s count=%d", tc.offset, next, total, tc.count)
			}
		})
	}
}

func TestAccountSummary_EmptyAccount(t *testing.T) {
	s, _, log := newSummaryFixture(t, 0, []summaryPageReply{{next: new(big.Int), total: new(big.Int)}})
	value, err := s.AccountSummary(context.Background(), otherAddr)
	if err != nil {
		t.Fatal(err)
	}
	if value.TotalFixedLockup.Sign() != 0 || value.TotalLockup.Sign() != 0 || value.AvailableFunds.Cmp(value.Funds) != 0 || len(log.batches) != 0 {
		t.Fatalf("summary=%+v batches=%v", value, log.batches)
	}
}

func TestAccountSummary_MalformedReadABI(t *testing.T) {
	for _, method := range []string{"accounts", "getRailsForPayerAndToken"} {
		t.Run(method, func(t *testing.T) {
			s, mb, _ := newSummaryFixture(t, 3, summaryPages(1))
			original := mb.callReplyFn
			calls := 0
			mb.callReplyFn = func(contract, name string, data []byte) ([]byte, bool, error) {
				if name == method {
					calls++
					return []byte{1}, true, nil
				}
				return original(contract, name, data)
			}
			value, err := s.AccountSummary(context.Background(), otherAddr)
			if value != nil || err == nil || calls != 1 {
				t.Fatalf("result=%v err=%v calls=%d", value, err, calls)
			}
		})
	}
}

func TestAccountSummary_BatchFailures(t *testing.T) {
	stringType, err := gethabi.NewType("string", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	revertData := func(reason string) []byte {
		t.Helper()
		data, err := (gethabi.Arguments{{Type: stringType}}).Pack(reason)
		if err != nil {
			t.Fatal(err)
		}
		return append(common.FromHex("0x08c379a0"), data...)
	}
	for _, tc := range []struct {
		name, want string
		mutate     func([]iabi.Result3) []iabi.Result3
		raw        []byte
	}{
		{"middle child timeout revert", "get rail 5: execution reverted: timeout", func(r []iabi.Result3) []iabi.Result3 {
			r[1] = iabi.Result3{ReturnData: revertData("timeout")}
			r[2] = iabi.Result3{ReturnData: revertData("later failure")}
			return r
		}, nil},
		{"middle child rate limit revert", "get rail 5: execution reverted: rate limit", func(r []iabi.Result3) []iabi.Result3 {
			r[1] = iabi.Result3{ReturnData: revertData("rate limit")}
			return r
		}, nil},
		{"unknown child revert", "get rail 5: call failed", func(r []iabi.Result3) []iabi.Result3 {
			r[1] = iabi.Result3{ReturnData: []byte{1}}
			return r
		}, nil},
		{"custom child revert", "get rail 5: execution reverted: RailInactiveOrSettled[5]", func(r []iabi.Result3) []iabi.Result3 {
			fp, err := filpay.FilPayMetaData.GetAbi()
			if err != nil {
				t.Fatal(err)
			}
			custom := fp.Errors["RailInactiveOrSettled"]
			data, err := custom.Inputs.Pack(big.NewInt(5))
			if err != nil {
				t.Fatal(err)
			}
			r[1] = iabi.Result3{ReturnData: append(custom.ID.Bytes()[:4], data...)}
			return r
		}, nil},
		{"damaged detail ABI", "decode rail 5", func(r []iabi.Result3) []iabi.Result3 {
			r[1].ReturnData = []byte{1}
			return r
		}, nil},
		{"missing result", "expected 3 rail results, got 2", func(r []iabi.Result3) []iabi.Result3 { return r[:2] }, nil},
		{"extra result", "expected 3 rail results, got 4", func(r []iabi.Result3) []iabi.Result3 { return append(r, r[0]) }, nil},
		{"damaged aggregate ABI", "decode aggregate3", nil, []byte{1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, summary := range []bool{false, true} {
				s, mb, _ := newSummaryFixture(t, 3, summaryPages(3, 3))
				original := mb.callReplyFn
				batches := 0
				mb.callReplyFn = func(contract, method string, data []byte) ([]byte, bool, error) {
					if method == "aggregate3" {
						batches++
						if batches >= 2 {
							if tc.raw != nil {
								return tc.raw, true, nil
							}
							calls := summaryCalls(t, mb, data)
							results := make([]iabi.Result3, len(calls))
							for i, call := range calls {
								out, err := mb.callContractLocked(ethereum.CallMsg{To: &call.Target, Data: call.CallData}, big.NewInt(100))
								if err != nil {
									t.Fatal(err)
								}
								results[i] = iabi.Result3{Success: true, ReturnData: out}
							}
							out, err := mb.multicallABI.Methods[method].Outputs.Pack(tc.mutate(results))
							return out, true, err
						}
					}
					return original(contract, method, data)
				}
				value, err := summaryRead(t, s, context.Background(), summary)
				if value != nil || err == nil || !strings.Contains(err.Error(), tc.want) || batches != 2 {
					t.Fatalf("summary=%v: result=%v err=%v batches=%d; want nil/%s/2", summary, value, err, batches, tc.want)
				}
			}
		})
	}
}

type summaryRPCError struct {
	code    int
	message string
}

func (e *summaryRPCError) Error() string  { return e.message }
func (e *summaryRPCError) ErrorCode() int { return e.code }

func TestAccountSummary_ReadErrorPolicy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		attempts int
	}{
		{"EOF", io.EOF, 3},
		{"unexpected EOF", io.ErrUnexpectedEOF, 3},
		{"typed EOF precedes wrapper text", fmt.Errorf("revert lookup: %w", io.EOF), 3},
		{"connection reset", syscall.ECONNRESET, 3},
		{"broken pipe", syscall.EPIPE, 3},
		{"network timeout", &net.DNSError{Err: "read timed out", IsTimeout: true}, 3},
		{"permanent DNS error containing timeout", &net.DNSError{Err: "no such host", Name: "timeout.example"}, 1},
		{"HTTP 429", rpc.HTTPError{StatusCode: 429}, 3},
		{"HTTP 502", rpc.HTTPError{StatusCode: 502}, 3},
		{"HTTP 503", &rpc.HTTPError{StatusCode: 503}, 3},
		{"HTTP 504", rpc.HTTPError{StatusCode: 504}, 3},
		{"RPC rate limit", &summaryRPCError{-32005, "rate limit exceeded"}, 3},
		{"RPC too many requests", &summaryRPCError{-32000, "too many requests"}, 3},
		{"RPC server timeout", &summaryRPCError{-32002, "request timed out"}, 3},
		{"RPC generic server timeout", &summaryRPCError{-32000, "request timed out"}, 3},
		{"RPC execution timeout", &summaryRPCError{-32000, "execution aborted (timeout = 5s)"}, 3},
		{"RPC internal server timeout", &summaryRPCError{-32603, "call timed out"}, 3},
		{"HTTP authentication failure", rpc.HTTPError{StatusCode: 401, Body: []byte("timeout rate limit")}, 1},
		{"typed revert overrides retryable text", &summaryRPCError{3, "timeout rate limit"}, 1},
		{"RPC revert with timeout", &summaryRPCError{-32000, "execution reverted: timeout"}, 1},
		{"RPC internal revert with timeout", &summaryRPCError{-32603, "execution reverted: timeout"}, 1},
		{"RPC invalid params override retryable text", &summaryRPCError{-32602, "timeout rate limit parameter invalid"}, 1},
		{"RPC server parameter error with timeout", &summaryRPCError{-32000, "invalid argument: timeout"}, 1},
		{"RPC unknown error code with timeout", &summaryRPCError{42, "request timed out"}, 1},
		{"legacy revert with timeout", errors.New("execution reverted: timeout"), 1},
		{"invalid argument with timeout", errors.New("invalid argument: timeout"), 1},
		{"context deadline", context.DeadlineExceeded, 1},
		{"context canceled", context.Canceled, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, mb, _ := newSummaryFixture(t, 3, summaryPages(3, 3))
				original := mb.callReplyFn
				batches := 0
				mb.callReplyFn = func(contract, method string, data []byte) ([]byte, bool, error) {
					if method == "aggregate3" {
						batches++
						if batches >= 2 {
							return nil, true, fmt.Errorf("node read: %w", tc.err)
						}
					}
					return original(contract, method, data)
				}
				value, err := summaryRead(t, s, context.Background(), true)
				matches := errors.Is(err, tc.err)
				if want, ok := errors.AsType[rpc.HTTPError](tc.err); ok {
					got, ok := errors.AsType[rpc.HTTPError](err)
					matches = ok && got.StatusCode == want.StatusCode
				}
				if value != nil || !matches || batches != tc.attempts+1 {
					t.Fatalf("result=%v err=%v attempts=%d, want nil/%v/%d", value, err, batches-1, tc.err, tc.attempts)
				}
				if tc.attempts > 1 && !errors.Is(err, retry.ErrMaxRetries) {
					t.Fatalf("retry exhaustion lost its sentinel: %v", err)
				}
				if _, ok := errors.AsType[*summaryRPCError](tc.err); ok {
					if _, ok := errors.AsType[rpc.Error](err); !ok {
						t.Fatalf("RPC error type lost: %v", err)
					}
				}
			})
		})
	}
}

func TestAccountSummary_RetryOnlyFailedRead(t *testing.T) {
	for _, point := range []string{"block number", "accounts", "second page", "second batch"} {
		t.Run(point, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, mb, log := newSummaryFixture(t, 3, summaryPages(3, 3))
				backend := &summaryBackend{mockBackend: mb}
				backend.call = func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
					if block == nil || block.Int64() != 100 {
						t.Errorf("retry block=%v, want 100", block)
					}
					return mb.CallContract(ctx, msg, block)
				}
				s.backend = backend
				original := mb.callReplyFn
				blocks, failures, accounts := 0, 0, 0
				mb.blockFn = func(context.Context) (uint64, error) {
					blocks++
					if point == "block number" && blocks < 3 {
						return 0, io.EOF
					}
					return 100, nil
				}
				mb.callReplyFn = func(contract, method string, data []byte) ([]byte, bool, error) {
					if method == "accounts" {
						accounts++
					}
					if failures < 2 && ((point == "accounts" && method == "accounts") ||
						(point == "second page" && method == "getRailsForPayerAndToken" && log.pages == 1) ||
						(point == "second batch" && method == "aggregate3" && len(log.batches) == 1)) {
						failures++
						return nil, true, io.EOF
					}
					return original(contract, method, data)
				}
				value, err := summaryRead(t, s, context.Background(), true)
				wantBlocks, wantAccounts := 1, 1
				if point == "block number" {
					wantBlocks = 3
				}
				if point == "accounts" {
					wantAccounts = 3
				}
				if err != nil || value == nil || value.Int64() != 81 || blocks != wantBlocks || accounts != wantAccounts || log.pages != 2 || !slices.Equal(log.batches, []int{3, 3}) || len(log.ids) != 6 {
					t.Fatalf("result=%v err=%v blocks=%d accounts=%d log=%+v", value, err, blocks, accounts, log)
				}
				for key, block := range mb.lastBlock {
					if block == nil || block.Int64() != 100 {
						t.Errorf("%s block=%v, want 100", key, block)
					}
				}
			})
		})
	}
}

func TestAccountSummary_ServerTimeoutRecovery(t *testing.T) {
	for _, fault := range []*summaryRPCError{
		{-32000, "request timed out"},
		{-32002, "request timed out"},
		{-32603, "call timed out"},
	} {
		for _, summary := range []bool{false, true} {
			t.Run(fmt.Sprintf("code=%d/summary=%v", fault.code, summary), func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					s, mb, log := newSummaryFixture(t, 3, summaryPages(3, 3))
					backend := &summaryBackend{mockBackend: mb}
					backend.call = func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
						if block == nil || block.Int64() != 100 {
							t.Errorf("read block=%v, want 100", block)
						}
						return mb.CallContract(ctx, msg, block)
					}
					s.backend = backend
					original := mb.callReplyFn
					attempts := 0
					mb.callReplyFn = func(contract, method string, data []byte) ([]byte, bool, error) {
						if method == "aggregate3" {
							attempts++
							if attempts == 2 {
								return nil, true, fmt.Errorf("node read: %w", fault)
							}
						}
						return original(contract, method, data)
					}
					value, err := summaryRead(t, s, context.Background(), summary)
					if err != nil || value == nil || value.Int64() != 81 || attempts != 3 ||
						log.pages != 2 || !slices.Equal(log.batches, []int{3, 3}) || len(log.ids) != 6 {
						t.Fatalf("result=%v err=%v attempts=%d log=%+v", value, err, attempts, log)
					}
				})
			})
		}
	}
}

func TestAccountSummary_BlockNumberFailure(t *testing.T) {
	for _, summary := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			s, mb, log := newSummaryFixture(t, 0, summaryPages(1))
			blocks := 0
			mb.blockFn = func(context.Context) (uint64, error) {
				blocks++
				return 0, io.EOF
			}
			value, err := summaryRead(t, s, context.Background(), summary)
			if value != nil || !errors.Is(err, io.EOF) || blocks != 3 || log.pages != 0 || len(mb.lastIn) != 0 {
				t.Fatalf("summary=%v result=%v err=%v blocks=%d", summary, value, err, blocks)
			}
		})
	}
}

type summaryBackend struct {
	*mockBackend
	call func(context.Context, ethereum.CallMsg, *big.Int) ([]byte, error)
	code func(context.Context, common.Address, *big.Int) ([]byte, error)
}

func (b *summaryBackend) CallContract(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
	if b.call != nil {
		return b.call(ctx, msg, block)
	}
	return b.mockBackend.CallContract(ctx, msg, block)
}

func (b *summaryBackend) CodeAt(ctx context.Context, address common.Address, block *big.Int) ([]byte, error) {
	if b.code != nil {
		return b.code(ctx, address, block)
	}
	return b.mockBackend.CodeAt(ctx, address, block)
}

func TestAccountSummary_CodeAtSnapshotAndRetry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, mb, _ := newSummaryFixture(t, 3, summaryPages(1))
		calls, codes := 0, 0
		backend := &summaryBackend{mockBackend: mb}
		backend.call = func(_ context.Context, _ ethereum.CallMsg, block *big.Int) ([]byte, error) {
			calls++
			if block == nil || block.Int64() != 100 {
				t.Errorf("call block=%v", block)
			}
			return nil, nil
		}
		backend.code = func(_ context.Context, _ common.Address, block *big.Int) ([]byte, error) {
			codes++
			if block == nil || block.Int64() != 100 {
				t.Errorf("code block=%v", block)
			}
			if codes < 3 {
				return nil, io.EOF
			}
			return nil, nil
		}
		s.backend = backend
		value, err := summaryRead(t, s, context.Background(), true)
		if value != nil || !errors.Is(err, bind.ErrNoCode) || calls != 1 || codes != 3 {
			t.Fatalf("result=%v err=%v calls=%d codes=%d", value, err, calls, codes)
		}
	})
}

func TestAccountSummary_Cancellation(t *testing.T) {
	for _, duringBackoff := range []bool{false, true} {
		t.Run(fmt.Sprintf("backoff=%v", duringBackoff), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s, mb, _ := newSummaryFixture(t, 3, summaryPages(3, 3))
				backend := &summaryBackend{mockBackend: mb}
				entered := make(chan struct{})
				batches := 0
				backend.call = func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
					if *msg.To == chain.Mainnet.Addresses().Multicall3 {
						batches++
						if batches >= 2 {
							if batches == 2 {
								close(entered)
							}
							if duringBackoff {
								return nil, io.EOF
							}
							<-ctx.Done()
							return nil, ctx.Err()
						}
					}
					return mb.CallContract(ctx, msg, block)
				}
				s.backend = backend
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := make(chan struct{})
				go func() {
					defer close(done)
					value, err := s.AccountSummary(ctx, otherAddr)
					if value != nil || !errors.Is(err, context.Canceled) {
						t.Errorf("result=%v err=%v", value, err)
					}
				}()
				<-entered
				synctest.Wait()
				cancel()
				<-done
				if batches != 2 {
					t.Fatalf("canceled read retried: batches=%d", batches)
				}
			})
		})
	}
}

func TestAccountSummary_ConcurrentSnapshots(t *testing.T) {
	mb := newMockBackend(t)
	backend := &summaryBackend{mockBackend: mb}
	s, err := New(Options{Backend: backend, ChainID: 1, FilPayAddress: filPayAddr, USDFCTokenAddress: tokenAddr})
	if err != nil {
		t.Fatal(err)
	}
	type snapshotKey struct{}
	mb.blockFn = func(ctx context.Context) (uint64, error) { return ctx.Value(snapshotKey{}).(uint64), nil }
	var ready sync.WaitGroup
	ready.Add(2)
	backend.call = func(ctx context.Context, msg ethereum.CallMsg, block *big.Int) ([]byte, error) {
		if *msg.To == filPayAddr && [4]byte(msg.Data[:4]) == [4]byte(mb.filPayABI.Methods["accounts"].ID) {
			ready.Done()
			ready.Wait()
		}
		want := ctx.Value(snapshotKey{}).(uint64)
		if block == nil || block.Uint64() != want {
			t.Errorf("snapshot=%d read block=%v", want, block)
			return nil, errors.New("wrong block")
		}
		id := new(big.Int).SetUint64(want)
		if *msg.To == chain.Mainnet.Addresses().Multicall3 {
			calls := summaryCalls(t, mb, msg.Data)
			args, err := mb.filPayABI.Methods["getRail"].Inputs.Unpack(calls[0].CallData[4:])
			if err != nil || args[0].(*big.Int).Cmp(id) != 0 {
				t.Errorf("snapshot=%d rail args=%v err=%v", want, args, err)
			}
			out, err := mb.filPayABI.Methods["getRail"].Outputs.Pack(testRailView(otherAddr, id))
			if err != nil {
				return nil, err
			}
			return mb.multicallABI.Methods["aggregate3"].Outputs.Pack([]iabi.Result3{{Success: true, ReturnData: out}})
		}
		method, err := mb.filPayABI.MethodById(msg.Data[:4])
		if err != nil {
			return nil, err
		}
		if method.Name == "accounts" {
			return method.Outputs.Pack(big.NewInt(1000), new(big.Int), new(big.Int), id)
		}
		return method.Outputs.Pack([]filpay.FilecoinPayV1RailInfo{{RailId: id, EndEpoch: new(big.Int)}}, big.NewInt(1), big.NewInt(1))
	}
	var done sync.WaitGroup
	for _, block := range []uint64{101, 202} {
		done.Go(func() {
			ctx := context.WithValue(context.Background(), snapshotKey{}, block)
			value, err := s.AccountSummary(ctx, otherAddr)
			if err != nil || value == nil || value.CurrentEpoch.Uint64() != block || value.TotalFixedLockup.Uint64() != block {
				t.Errorf("snapshot=%d result=%+v err=%v", block, value, err)
			}
		})
	}
	done.Wait()
	backend.call = nil
	mb.setFilPayReply(t, filPayAddr, "getRail", testRailView(otherAddr, big.NewInt(3)))
	if _, err := s.GetRail(context.Background(), sdktypes.NewBigInt(3)); err != nil {
		t.Fatal(err)
	}
	if mb.lastBlock[filPayAddr.Hex()+":getRail"] != nil {
		t.Fatal("summary changed the shared service's latest-state binding")
	}
}
