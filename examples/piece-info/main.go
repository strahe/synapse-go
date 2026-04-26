// Piece-info calculates Filecoin PieceCID information for a local file.
//
// Usage:
//
//	go run ./examples/piece-info --file ./payload.bin
package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/strahe/synapse-go/examples/internal/exampleutil"
	"github.com/strahe/synapse-go/piece"
)

type pieceConfig struct {
	FilePath string
}

func main() {
	if err := realMain(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func realMain(_ context.Context, args []string, stdout io.Writer) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}
	file, err := exampleutil.OpenFile(cfg.FilePath)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return runPieceInfo(file, stdout)
}

func parseConfig(args []string) (pieceConfig, error) {
	cfg := pieceConfig{}
	fs := flag.NewFlagSet("piece-info", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.FilePath, "file", "", "file to inspect")
	if err := fs.Parse(args); err != nil {
		return pieceConfig{}, err
	}
	switch {
	case cfg.FilePath == "" && fs.NArg() == 1:
		cfg.FilePath = fs.Arg(0)
	case cfg.FilePath == "" || fs.NArg() != 0:
		return pieceConfig{}, errors.New("usage: go run ./examples/piece-info --file ./payload.bin")
	}
	return cfg, nil
}

func runPieceInfo(r io.Reader, stdout io.Writer) error {
	info, err := piece.Calculate(r)
	if err != nil {
		return err
	}
	padded, err := piece.PaddedSize(int64(info.RawSize))
	if err != nil {
		return err
	}
	root, err := piece.ExtractRoot(info.CIDv1)
	if err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "cidV1", info.CIDv1); err != nil {
		return err
	}
	cidV2 := "<undefined: raw payload smaller than 127 bytes>"
	if info.CIDv2.Defined() {
		cidV2 = info.CIDv2.String()
	}
	if err := exampleutil.WriteKV(stdout, "cidV2", cidV2); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "rawSize", info.RawSize); err != nil {
		return err
	}
	if err := exampleutil.WriteKV(stdout, "paddedSize", padded); err != nil {
		return err
	}
	return exampleutil.WriteKV(stdout, "pieceRoot", hex.EncodeToString(root[:]))
}
