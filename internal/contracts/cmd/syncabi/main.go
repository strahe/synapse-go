package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/strahe/synapse-go/internal/contracts/syncabi"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := syncabi.Config{
		Ref:     syncabi.DefaultRef,
		RootDir: filepath.Join("internal", "contracts"),
		Client:  http.DefaultClient,
	}

	return syncabi.Sync(ctx, cfg)
}
