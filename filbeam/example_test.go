package filbeam_test

import (
	"context"
	"fmt"
	"log"

	"github.com/strahe/synapse-go/filbeam"
	"github.com/strahe/synapse-go/types"
)

// Example demonstrates fetching dataset stats via filbeam.Service. In
// practice a Service is obtained from [synapse.Client.FilBeam].
//
// [synapse.Client.FilBeam]: https://pkg.go.dev/github.com/strahe/synapse-go#Client.FilBeam
func Example() {
	var svc *filbeam.Service // obtained from synapse.Client.FilBeam()

	ctx := context.Background()
	stats, err := svc.GetDataSetStats(ctx, types.DataSetID(42))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(stats.CDNEgressQuota)
}
