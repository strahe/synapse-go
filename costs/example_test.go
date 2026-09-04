package costs_test

import (
	"context"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/costs"
)

// Example demonstrates estimating upload costs via costs.Service. In
// practice a Service is obtained from [synapse.Client.Costs].
//
// [synapse.Client.Costs]: https://pkg.go.dev/github.com/strahe/synapse-go#Client.Costs
func Example() {
	var svc *costs.Service // obtained from synapse.Client.Costs()

	ctx := context.Background()
	payer := common.HexToAddress("0x...")
	dataSize := big.NewInt(1 << 30)

	quote, err := svc.GetUploadCosts(ctx, payer, dataSize, nil)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(quote.Rate.RatePerMonth)
}
