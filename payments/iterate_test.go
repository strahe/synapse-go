package payments

import (
	"context"
	"errors"
	"iter"
	"math/big"
	"slices"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	filpaybind "github.com/strahe/synapse-go/internal/contracts/filpay"
	"github.com/strahe/synapse-go/internal/lifecycle"
	sdktypes "github.com/strahe/synapse-go/types"
)

type railReadRoute struct {
	name    string
	method  string
	list    func(*Service, context.Context, common.Address, common.Address, sdktypes.ListOptions) (*RailPage, error)
	iterate func(*Service, context.Context, common.Address, common.Address) iter.Seq2[RailListItem, error]
}

func railReadRoutes() []railReadRoute {
	return []railReadRoute{
		{"payer", "getRailsForPayerAndToken", (*Service).GetRailsAsPayer, (*Service).IterateAllRailsAsPayer},
		{"payee", "getRailsForPayeeAndToken", (*Service).GetRailsAsPayee, (*Service).IterateAllRailsAsPayee},
	}
}

func TestIterateAllRails_Pagination(t *testing.T) {
	wideOffset := new(big.Int).Lsh(big.NewInt(1), 80)
	wideTotal := new(big.Int).Add(wideOffset, big.NewInt(1))
	wideRailID := new(big.Int).Lsh(big.NewInt(1), 200)
	fullPageIDs := make([]*big.Int, 100)
	fullPageWant := make([]string, len(fullPageIDs))
	for i := range fullPageIDs {
		fullPageIDs[i] = big.NewInt(int64(i + 1))
		fullPageWant[i] = fullPageIDs[i].String()
	}
	type pageReply struct {
		ids         []*big.Int
		next, total *big.Int
		err         error
	}
	cases := []struct {
		name    string
		pages   []pageReply
		wantIDs []string
		wantErr error
	}{
		{
			name:  "empty collection",
			pages: []pageReply{{next: big.NewInt(0), total: big.NewInt(0)}},
		},
		{
			name: "short and empty intermediate pages",
			pages: []pageReply{
				{ids: []*big.Int{big.NewInt(1), big.NewInt(2)}, next: big.NewInt(100), total: big.NewInt(201)},
				{next: big.NewInt(200), total: big.NewInt(201)},
				{ids: []*big.Int{big.NewInt(3)}, next: big.NewInt(201), total: big.NewInt(201)},
			},
			wantIDs: []string{"1", "2", "3"},
		},
		{
			name:    "full final page needs no extra request",
			pages:   []pageReply{{ids: fullPageIDs, next: big.NewInt(100), total: big.NewInt(100)}},
			wantIDs: fullPageWant,
		},
		{
			name: "collection shrinks between pages",
			pages: []pageReply{
				{ids: []*big.Int{big.NewInt(1)}, next: big.NewInt(100), total: big.NewInt(200)},
				{next: big.NewInt(50), total: big.NewInt(50)},
			},
			wantIDs: []string{"1"},
		},
		{
			name: "later RPC failure",
			pages: []pageReply{
				{ids: []*big.Int{big.NewInt(1)}, next: big.NewInt(100), total: big.NewInt(200)},
				{err: context.DeadlineExceeded},
			},
			wantIDs: []string{"1"},
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "full width cursor and rail ID",
			pages: []pageReply{
				{ids: []*big.Int{big.NewInt(1)}, next: wideOffset, total: wideTotal},
				{ids: []*big.Int{wideRailID}, next: wideTotal, total: wideTotal},
			},
			wantIDs: []string{"1", wideRailID.String()},
		},
		{
			name:    "first cursor does not advance",
			pages:   []pageReply{{ids: []*big.Int{big.NewInt(1)}, next: big.NewInt(0), total: big.NewInt(100)}},
			wantErr: ErrInvalidRailPage,
		},
		{
			name: "repeated cursor",
			pages: []pageReply{
				{ids: []*big.Int{big.NewInt(1)}, next: big.NewInt(100), total: big.NewInt(200)},
				{ids: []*big.Int{big.NewInt(2)}, next: big.NewInt(100), total: big.NewInt(200)},
			},
			wantIDs: []string{"1"},
			wantErr: ErrInvalidRailPage,
		},
		{
			name: "backward cursor",
			pages: []pageReply{
				{ids: []*big.Int{big.NewInt(1)}, next: big.NewInt(100), total: big.NewInt(200)},
				{ids: []*big.Int{big.NewInt(2)}, next: big.NewInt(50), total: big.NewInt(200)},
			},
			wantIDs: []string{"1"},
			wantErr: ErrInvalidRailPage,
		},
	}
	for _, route := range railReadRoutes() {
		t.Run(route.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					s, mb := newTestService(t)
					method := mb.filPayABI.Methods[route.method]
					calls := 0
					wantOffset := new(big.Int)
					mb.callReplyFn = func(_, name string, data []byte) ([]byte, bool, error) {
						if name != route.method {
							return nil, false, nil
						}
						if calls >= len(tc.pages) {
							t.Fatal("iterator requested an unexpected extra page")
						}
						args, err := method.Inputs.Unpack(data[4:])
						if err != nil {
							t.Fatal(err)
						}
						if args[0].(common.Address) != otherAddr || args[1].(common.Address) != tokenAddr {
							t.Fatalf("account/token = %v, want %s/%s", args[:2], otherAddr, tokenAddr)
						}
						if got := args[2].(*big.Int); got.Cmp(wantOffset) != 0 {
							t.Fatalf("offset = %s, want %s", got, wantOffset)
						}
						if got := args[3].(*big.Int); got.Cmp(big.NewInt(100)) != 0 {
							t.Fatalf("limit = %s, want 100", got)
						}
						page := tc.pages[calls]
						calls++
						if page.err != nil {
							return nil, true, page.err
						}
						wantOffset = page.next
						items := make([]filpaybind.FilecoinPayV1RailInfo, len(page.ids))
						for i, id := range page.ids {
							items[i] = filpaybind.FilecoinPayV1RailInfo{RailId: id, EndEpoch: big.NewInt(0)}
						}
						out, err := method.Outputs.Pack(items, page.next, page.total)
						if err != nil {
							t.Fatal(err)
						}
						return out, true, nil
					}
					var gotIDs []string
					errorCount := 0
					for rail, err := range route.iterate(s, context.Background(), otherAddr, tokenAddr) {
						if err != nil {
							errorCount++
							if tc.wantErr == nil || !errors.Is(err, tc.wantErr) {
								t.Fatalf("iteration error = %v, want %v", err, tc.wantErr)
							}
							if !rail.RailID.IsZero() {
								t.Fatalf("error yielded a rail: %+v", rail)
							}
							continue
						}
						gotIDs = append(gotIDs, rail.RailID.String())
					}
					if !slices.Equal(gotIDs, tc.wantIDs) {
						t.Fatalf("rails = %v, want %v", gotIDs, tc.wantIDs)
					}
					wantErrors := 0
					if tc.wantErr != nil {
						wantErrors = 1
					}
					if errorCount != wantErrors || calls != len(tc.pages) {
						t.Fatalf("errors/calls = %d/%d, want %d/%d", errorCount, calls, wantErrors, len(tc.pages))
					}
				})
			}
		})
	}
}

func TestIterateAllRails_StopAndCancellation(t *testing.T) {
	for _, route := range railReadRoutes() {
		t.Run(route.name, func(t *testing.T) {
			for _, kind := range []string{"consumer stops", "canceled before reading", "canceled during reading", "canceled between records", "closed between pages"} {
				t.Run(kind, func(t *testing.T) {
					s, mb := newTestService(t)
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					life := new(lifecycle.Lifecycle)
					s.lifecycle = life
					items := []filpaybind.FilecoinPayV1RailInfo{
						{RailId: big.NewInt(1), EndEpoch: big.NewInt(0)},
						{RailId: big.NewInt(2), EndEpoch: big.NewInt(0)},
					}
					if kind == "closed between pages" {
						items = items[:1]
					}
					mb.setFilPayReply(t, filPayAddr, route.method, items, big.NewInt(100), big.NewInt(200))
					calls := 0
					mb.callReplyFn = func(_, method string, _ []byte) ([]byte, bool, error) {
						if method == route.method {
							calls++
							if calls > 1 {
								t.Fatal("iterator made a request after stopping, cancellation, or closure")
							}
							if kind == "canceled during reading" {
								cancel()
							}
						}
						return nil, false, nil
					}
					if kind == "canceled before reading" {
						cancel()
					}
					wantErr := error(context.Canceled)
					wantRecords, wantCalls := 1, 1
					switch kind {
					case "consumer stops":
						wantErr = nil
					case "canceled before reading":
						wantRecords, wantCalls = 0, 0
					case "canceled during reading":
						wantRecords = 0
					case "closed between pages":
						wantErr = ErrClosed
					}
					records, errorCount := 0, 0
					for rail, err := range route.iterate(s, ctx, otherAddr, tokenAddr) {
						if err != nil {
							errorCount++
							if wantErr == nil || !errors.Is(err, wantErr) {
								t.Fatalf("iteration error = %v, want %v", err, wantErr)
							}
							continue
						}
						records++
						if !rail.RailID.Equal(sdktypes.NewBigInt(1)) {
							t.Fatalf("yielded unexpected rail %s", rail.RailID)
						}
						switch kind {
						case "canceled between records":
							cancel()
						case "closed between pages":
							life.Close()
						}
						if kind == "consumer stops" {
							break
						}
					}
					wantErrors := 1
					if wantErr == nil {
						wantErrors = 0
					}
					if records != wantRecords || calls != wantCalls || errorCount != wantErrors {
						t.Fatalf("records/calls/errors = %d/%d/%d, want %d/%d/%d", records, calls, errorCount, wantRecords, wantCalls, wantErrors)
					}
				})
			}
		})
	}
}

func TestIterateAllRails_LazyAndReusable(t *testing.T) {
	for _, route := range railReadRoutes() {
		t.Run(route.name, func(t *testing.T) {
			s, mb := newTestService(t)
			calls := 0
			mb.callReplyFn = func(_, method string, data []byte) ([]byte, bool, error) {
				if method == route.method {
					args, err := mb.filPayABI.Methods[method].Inputs.Unpack(data[4:])
					if err != nil {
						t.Fatal(err)
					}
					wantOffset := big.NewInt(int64(calls%2) * 100)
					if got := args[2].(*big.Int); got.Cmp(wantOffset) != 0 {
						t.Fatalf("offset = %s, want %s", got, wantOffset)
					}
					id, next := int64(7), int64(100)
					if calls%2 == 1 {
						id, next = 8, 101
					}
					calls++
					out, err := mb.filPayABI.Methods[method].Outputs.Pack(
						[]filpaybind.FilecoinPayV1RailInfo{{RailId: big.NewInt(id), EndEpoch: big.NewInt(0)}}, big.NewInt(next), big.NewInt(101),
					)
					if err != nil {
						t.Fatal(err)
					}
					return out, true, nil
				}
				return nil, false, nil
			}
			seq := route.iterate(s, context.Background(), otherAddr, tokenAddr)
			if calls != 0 {
				t.Fatal("creating the iterator made an RPC call")
			}
			for range 2 {
				var gotIDs []string
				for rail, err := range seq {
					if err != nil {
						t.Fatal(err)
					}
					gotIDs = append(gotIDs, rail.RailID.String())
				}
				if !slices.Equal(gotIDs, []string{"7", "8"}) {
					t.Fatalf("rails = %v, want [7 8]", gotIDs)
				}
			}
			if calls != 4 {
				t.Fatalf("calls = %d, want 4", calls)
			}
		})
	}
}

func TestRailReads_ValidateBeforeRPC(t *testing.T) {
	for _, route := range railReadRoutes() {
		t.Run(route.name, func(t *testing.T) {
			for _, kind := range []string{"nil service", "zero service", "closed", "zero account", "zero token"} {
				t.Run(kind, func(t *testing.T) {
					s, mb := newTestService(t)
					account, token := otherAddr, tokenAddr
					wantErr := ErrInvalidArgument
					switch kind {
					case "nil service":
						s = nil
						wantErr = ErrUninitialized
					case "zero service":
						s = new(Service)
						wantErr = ErrUninitialized
					case "closed":
						life := new(lifecycle.Lifecycle)
						life.Close()
						s.lifecycle = life
						wantErr = ErrClosed
					case "zero account":
						account = common.Address{}
					case "zero token":
						token = common.Address{}
					}
					if _, err := route.list(s, context.Background(), account, token, sdktypes.ListOptions{Limit: 1}); !errors.Is(err, wantErr) {
						t.Fatalf("list error = %v, want %v", err, wantErr)
					}
					errorsSeen := 0
					for _, err := range route.iterate(s, context.Background(), account, token) {
						if !errors.Is(err, wantErr) {
							t.Fatalf("iteration error = %v, want %v", err, wantErr)
						}
						errorsSeen++
					}
					if errorsSeen != 1 || len(mb.lastIn) != 0 {
						t.Fatalf("errors/RPC calls = %d/%d, want 1/0", errorsSeen, len(mb.lastIn))
					}
				})
			}
		})
	}
}
