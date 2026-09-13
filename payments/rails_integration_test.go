//go:build integration

package payments_test

import (
	"context"
	"iter"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/internal/integrationtest"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
)

// TestIntegration_PaymentRailsPagination performs only contract reads.
func TestIntegration_PaymentRailsPagination(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	client := integrationtest.NewDefaultClient(t, ctx)
	service := client.Payments()
	account, token := client.Address(), client.ResolvedAddresses().USDFC

	for _, route := range []struct {
		name    string
		list    func(context.Context, common.Address, common.Address, types.ListOptions) (*payments.RailPage, error)
		iterate func(context.Context, common.Address, common.Address) iter.Seq2[payments.RailListItem, error]
	}{
		{"payer", service.GetRailsAsPayer, service.IterateAllRailsAsPayer},
		{"payee", service.GetRailsAsPayee, service.IterateAllRailsAsPayee},
	} {
		t.Run(route.name, func(t *testing.T) {
			page, err := route.list(ctx, account, token, types.ListOptions{Limit: 2})
			if err != nil {
				t.Fatal(err)
			}
			if page == nil || page.NextOffset == nil || page.Total == nil {
				t.Fatalf("incomplete rail page: %+v", page)
			}
			if len(page.Rails) > 2 {
				t.Fatalf("page returned %d rails, want at most 2", len(page.Rails))
			}
			t.Logf("first page: count=%d next offset=%s total slots=%s", len(page.Rails), page.NextOffset, page.Total)

			for rail, err := range route.iterate(ctx, account, token) {
				if err != nil {
					t.Fatal(err)
				}
				if rail.RailID.IsZero() || rail.EndEpoch == nil {
					t.Fatalf("incomplete rail: %+v", rail)
				}
				t.Logf("iterator first rail: %s", rail.RailID)
				break
			}
		})
	}
}
