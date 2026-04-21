package spregistry_test

import (
	"context"
	"fmt"
	"log"

	"github.com/strahe/synapse-go/spregistry"
	"github.com/strahe/synapse-go/types"
)

// Example demonstrates looking up a storage provider by ID via
// spregistry.Service. In practice a Service is obtained from
// [synapse.Client.SPRegistry].
func Example() {
	var svc *spregistry.Service // obtained from synapse.Client.SPRegistry()

	ctx := context.Background()
	info, err := svc.GetProvider(ctx, types.ProviderID(1))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(info.Name)
}
