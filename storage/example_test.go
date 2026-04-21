package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"log"

	"github.com/strahe/synapse-go/storage"
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
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.PieceCID)
}
