# synapse-go

Go SDK for [Filecoin Onchain Cloud (FOC)](https://docs.filecoin.io/), ported from the TypeScript [@filoz/synapse-sdk](https://github.com/FilOzone/synapse-sdk).

> **Status: Pre-release** — scaffolding complete, implementation in progress.

## Installation

```bash
go get github.com/strahe/synapse-go
```

Requires **Go 1.25+**.

## Quick Start

```go
import "github.com/strahe/synapse-go"

client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex("0x..."),
    synapse.WithChain(chain.Calibration),
)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

result, err := client.Upload(ctx, data, nil)
```

## Development

```bash
make build       # Build all packages
make test        # Run unit tests
make lint        # Run golangci-lint
make check       # Build + vet + lint + test
```

## License

See [LICENSE](LICENSE).
