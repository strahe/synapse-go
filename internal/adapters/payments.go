package adapters

import (
	"context"
	"math/big"

	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/storage"
	sdktypes "github.com/strahe/synapse-go/types"
)

// paymentsFunder adapts *payments.Service to [storage.PaymentsFunder].
type paymentsFunder struct {
	p *payments.Service
}

// NewPaymentsFunder returns a [storage.PaymentsFunder] backed by p.
func NewPaymentsFunder(p *payments.Service) storage.PaymentsFunder {
	return &paymentsFunder{p: p}
}

func (a *paymentsFunder) FundSync(ctx context.Context, amount *big.Int, opts ...payments.WriteOption) (*sdktypes.WriteResult, error) {
	return a.p.FundSync(ctx, amount, opts...)
}
