package costs_test

import (
	"context"
	"fmt"
	"log"

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
	pieceSizes := []uint64{256 << 20}

	quote, err := svc.GetUploadCosts(ctx, payer, pieceSizes, &costs.UploadCostOptions{IsNewDataSet: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(quote.Rate.RatePerMonth)
}
