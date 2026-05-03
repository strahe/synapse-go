package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/ipfs/go-cid"
	"github.com/strahe/synapse-go/storage"
	"github.com/strahe/synapse-go/types"
)

// Example demonstrates uploading a payload via a storage.Service. In practice
// a Service is obtained from [synapse.Client.Storage] after calling
// [synapse.New]; this example assumes svc is already wired.
//
// [synapse.Client.Storage]: https://pkg.go.dev/github.com/strahe/synapse-go#Client.Storage
// [synapse.New]: https://pkg.go.dev/github.com/strahe/synapse-go#New
func Example() {
	var svc *storage.Service // obtained from synapse.Client.Storage()

	ctx := context.Background()
	payload := bytes.NewReader(bytes.Repeat([]byte("data"), 64))

	res, err := svc.Upload(ctx, payload, &storage.UploadOptions{
		OnProgress: func(uploaded int64) {
			log.Printf("uploaded %d bytes", uploaded)
		},
		OnStored: func(providerID types.BigInt, pieceCID cid.Cid) {
			log.Printf("stored %s on provider %s", pieceCID, providerID)
		},
		OnPiecesConfirmed: func(dataSetID, providerID types.BigInt, pieces []storage.ConfirmedPiece) {
			log.Printf("provider %s confirmed %d piece(s) in dataset %s", providerID, len(pieces), dataSetID)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.PieceCID)
}
