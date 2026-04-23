package costs

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"

	"github.com/strahe/synapse-go/chain"
	"github.com/strahe/synapse-go/payments"
)

func TestCalculateMultiContextCosts_ReadyWhenFunded(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: usdfc(1_000_000), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		usdfcFrac(1),
	)

	refs := []MultiContextRef{
		{IsNewDataSet: true},
		{IsNewDataSet: true, WithCDN: true},
	}
	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(1024),
		refs,
		nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.NeedsFWSSMaxApproval {
		t.Error("expected approval satisfied")
	}
	if got.DepositNeeded.Sign() != 0 {
		t.Errorf("expected zero deposit: got %s", got.DepositNeeded)
	}
	if !got.Ready {
		t.Error("expected Ready=true")
	}
}

func TestCalculateMultiContextCosts_AggregatesRates(t *testing.T) {
	svc := buildSvc(t,
		&mockWS{price: defaultPrice()},
		&mockPay{
			account:  &payments.AccountState{Funds: new(big.Int), LockupCurrent: new(big.Int), LockupRate: new(big.Int)},
			approval: maxApproval(),
		},
		usdfcFrac(1),
	)

	single, err := svc.GetUploadCosts(
		context.Background(),
		common.Address{},
		bi(chain.TiB),
		&UploadCostOptions{IsNewDataSet: true},
	)
	if err != nil {
		t.Fatalf("GetUploadCosts: %v", err)
	}

	refs := []MultiContextRef{
		{IsNewDataSet: true},
		{IsNewDataSet: true},
	}
	got, err := svc.CalculateMultiContextCosts(
		context.Background(),
		common.Address{},
		bi(chain.TiB),
		refs,
		nil,
	)
	if err != nil {
		t.Fatalf("CalculateMultiContextCosts: %v", err)
	}

	wantRate := new(big.Int).Mul(single.Rate.RatePerEpoch, big.NewInt(2))
	if got.RatePerEpoch.Cmp(wantRate) != 0 {
		t.Errorf("RatePerEpoch: want %s got %s", wantRate, got.RatePerEpoch)
	}
	if got.DepositNeeded.Sign() <= 0 {
		t.Errorf("expected positive deposit: got %s", got.DepositNeeded)
	}
}

func TestCalculateMultiContextCosts_EmptyRefs(t *testing.T) {
	svc := buildSvc(t, &mockWS{price: defaultPrice()}, &mockPay{}, usdfcFrac(1))
	if _, err := svc.CalculateMultiContextCosts(
		context.Background(), common.Address{}, bi(1024), nil, nil,
	); err == nil {
		t.Error("expected error for empty refs")
	}
}
