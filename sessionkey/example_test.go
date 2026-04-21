package sessionkey_test

import (
	"context"
	"fmt"
	"log"

	"github.com/ethereum/go-ethereum/common"
	"github.com/strahe/synapse-go/sessionkey"
)

// Example shows querying session-key authorization expirations for a set of
// permissions. In practice a Service is obtained from
// [synapse.Client.SessionKey].
func Example() {
	var svc *sessionkey.Service // obtained from synapse.Client.SessionKey()

	ctx := context.Background()
	root := common.HexToAddress("0x...")
	sk := common.HexToAddress("0x...")

	perms := []sessionkey.Permission{
		sessionkey.AddPiecesPermission,
		sessionkey.DeleteDataSetPermission,
	}
	exp, err := svc.GetExpirations(ctx, root, sk, perms)
	for p, epoch := range exp {
		fmt.Printf("%s expires at %d\n", p, epoch)
	}
	if err != nil {
		log.Printf("partial expiry lookup failure: %v", err)
	}
}
