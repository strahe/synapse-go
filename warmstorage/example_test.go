package warmstorage_test

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/types"
	"github.com/strahe/synapse-go/warmstorage"
)

// Example demonstrates listing a client's data sets via warmstorage.Service.
// In practice a Service is obtained from [synapse.Client.WarmStorage].
func Example() {
	var svc *warmstorage.Service // obtained from synapse.Client.WarmStorage()

	ctx := context.Background()
	payer := common.HexToAddress("0x...")

	sets, err := svc.GetClientDataSets(ctx, payer, types.ListOptions{Limit: 10})
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range sets {
		fmt.Println(s.DataSetID)
	}
}
