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
func Example() {
	var svc *storage.Service // obtained from synapse.Client.Storage()

	ctx := context.Background()
	payload := bytes.NewReader([]byte("hello"))

	res, err := svc.Upload(ctx, payload, &storage.UploadOptions{
		OnProgress: func(uploaded int64) {
			log.Printf("uploaded %d bytes", uploaded)
		},
		OnStored: func(providerID types.ProviderID, pieceCID cid.Cid) {
			log.Printf("stored %s on provider %d", pieceCID, providerID)
		},
		OnPiecesConfirmed: func(dataSetID types.DataSetID, providerID types.ProviderID, pieces []storage.ConfirmedPiece) {
			log.Printf("provider %d confirmed %d piece(s) in dataset %d", providerID, len(pieces), dataSetID)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.PieceCID)
}
