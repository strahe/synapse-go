package costs_test

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/costs"
)

// Example demonstrates fetching an account summary via costs.Service. In
// practice a Service is obtained from [synapse.Client.Costs].
func Example() {
	var svc *costs.Service // obtained from synapse.Client.Costs()

	ctx := context.Background()
	owner := common.HexToAddress("0x...")

	summary, err := svc.GetAccountSummary(ctx, owner)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(summary.AvailableFunds)
}
