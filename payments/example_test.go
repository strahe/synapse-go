package payments_test

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/payments"
	"github.com/strahe/synapse-go/types"
)

// Example shows reading an account balance via payments.Service. In practice
// a Service is obtained from [synapse.Client.Payments].
//
// [synapse.Client.Payments]: https://pkg.go.dev/github.com/strahe/synapse-go#Client.Payments
func Example() {
	var svc *payments.Service // obtained from synapse.Client.Payments()

	ctx := context.Background()
	usdfc := common.HexToAddress("0x...")
	owner := common.HexToAddress("0x...")

	bal, err := svc.Balance(ctx, usdfc, owner)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(new(big.Int).Set(bal))
}

func ExampleService_GetRailsAsPayer() {
	var svc *payments.Service // obtained from synapse.Client.Payments()
	ctx := context.Background()
	payer := common.HexToAddress("0x...")
	token := common.HexToAddress("0x...")
	opts := types.ListOptions{Limit: 100}

	for {
		page, err := svc.GetRailsAsPayer(ctx, payer, token, opts)
		if err != nil {
			log.Fatal(err)
		}
		for _, rail := range page.Rails {
			fmt.Println(rail.RailID)
		}
		if page.NextOffset.Cmp(page.Total) >= 0 {
			break
		}
		if !page.NextOffset.IsUint64() {
			log.Fatal("next rail offset exceeds uint64; use IterateAllRailsAsPayer")
		}
		if page.NextOffset.Uint64() <= opts.Offset {
			log.Fatal("next rail offset must advance")
		}
		opts.Offset = page.NextOffset.Uint64()
	}
}

func ExampleService_IterateAllRailsAsPayer() {
	var svc *payments.Service // obtained from synapse.Client.Payments()
	ctx := context.Background()
	payer := common.HexToAddress("0x...")
	token := common.HexToAddress("0x...")

	for rail, err := range svc.IterateAllRailsAsPayer(ctx, payer, token) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(rail.RailID)
	}
}
