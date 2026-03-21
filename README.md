# synapse-go

Go SDK for [Filecoin Onchain Cloud (FOC)](https://docs.filecoin.io/), ported from the TypeScript [@filoz/synapse-sdk](https://github.com/FilOzone/synapse-sdk).

> **Status: Pre-release** — scaffolding complete, implementation in progress.

## Installation

```bash
go get github.com/strahe/synapse-go
```

Requires **Go 1.25+**.

## Current package surface

The implemented MVP surface is currently package-first rather than a single
root client:

| Package | Purpose |
| --- | --- |
| `piece` | PieceCIDv2 calculation / validation helpers |
| `internal/curio` | Curio PDP HTTP client used by provider-local storage flows |
| `payments` | FilPay / ERC-20 balance and funding helpers |
| `warmstorage` | FilecoinWarmStorageService read surface |
| `spregistry` | ServiceProviderRegistry read + PDP provider selection |
| `storage` | Multi-copy `store -> presign -> pull -> commit` orchestration |

For storage uploads, compose `storage.Manager` with `storage.NewServiceResolver`
plus a provider-local `storage.Context` factory. The eventual root `synapse.New`
convenience client is not wired yet, so the old quick-start snippet has been
removed to avoid advertising an API that does not exist in this branch.

## Development

```bash
make build       # Build all packages
make test        # Run unit tests
make lint        # Run golangci-lint
make check       # Build + vet + lint + test
```

## License

See [LICENSE](LICENSE).
