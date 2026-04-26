package payments_test

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/payments"
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
