# synapse-go

[![CI](https://github.com/strahe/synapse-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/strahe/synapse-go/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/strahe/synapse-go)](https://github.com/strahe/synapse-go/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/strahe/synapse-go.svg)](https://pkg.go.dev/github.com/strahe/synapse-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/strahe/synapse-go)](https://goreportcard.com/report/github.com/strahe/synapse-go)
[![License](https://img.shields.io/github/license/strahe/synapse-go)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.25%2B-00ADD8)](go.mod)

Go SDK for Filecoin Onchain Cloud (FOC), ported from the
[@filoz/synapse-sdk](https://github.com/FilOzone/synapse-sdk).

> **Status:** Beta - API may change.

**Docs:** [getting started](docs/GETTING_STARTED.md) |
[API reference](https://pkg.go.dev/github.com/strahe/synapse-go) |
[examples](examples/)

## Install

```bash
go get github.com/strahe/synapse-go
```

Requires Go 1.25+.

## Quick Start

```go
client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex("0x..."),
    synapse.WithRPCURL("https://api.calibration.node.glif.io/rpc/v1"),
    synapse.WithSource("my-app"),
)
if err != nil { return err }
defer client.Close()

// file is an io.Reader over the payload to upload.
upload, err := client.Storage().Upload(ctx, file, &storage.UploadOptions{Copies: 2})
if err != nil { return err }

fmt.Println("piece:", upload.PieceCID)
fmt.Printf("copies: %d/%d\n", upload.SuccessCount(), upload.RequestedCopies)
fmt.Println("retrieve:", upload.Copies[0].RetrievalURL)
```

Use real values from your config or secret manager. The chain is detected
automatically.

## Package Map

| Package | Purpose |
|---------|---------|
| `synapse` | Root client that initializes chain config, contract addresses, and services |
| `storage` | Multi-provider upload/download orchestration, dataset discovery, and prepare flows |
| `payments` | USDFC balances, deposits, withdrawals, approvals, and Filecoin Pay rails |
| `costs` | Storage pricing, lockup, runway, and funding cost calculations |
| `warmstorage` | FWSS dataset management, pricing, approvals, and provider allocation |
| `spregistry` | Storage provider registry discovery and provider/product management |
| `sessionkey` | Delegated session key authorization for FWSS EIP-712 operations |
| `chain` | Filecoin chain IDs, contract addresses, epochs, and token units |
| `signer` | Secp256k1 and BLS signing abstractions |
| `piece` | PieceCID v1/v2 calculation, parsing, and validation |
| `filbeam` | FilBeam egress quota and usage stats for FWSS datasets |
| `pdp` | Low-level Curio-compatible PDP provider HTTP client |

## Testing

CI covers build, vet, lint, tests, and govulncheck.

Integration tests require `INTEGRATION_PRIVATE_KEY` in `.env` (needs **tFIL** for gas + **5 USDFC**).

`INTEGRATION_RPC_URL` is optional.

Approximate local runtimes on Calibration:

```bash
make test                      # seconds; normal development loop
make test-integration-readonly # 30-60s; read-only Calibration checks
make test-integration-fast     # 5-10m; upload/download smoke with cleanup
make test-integration-cross    # 15-20m; full cross-package flow
make test-integration          # ~30m; final validation before merge
```

## Development

```bash
make check   # build + vet + lint + test
```

## License

[Apache-2.0](LICENSE)
