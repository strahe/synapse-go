# synapse-go

[![CI](https://github.com/strahe/synapse-go/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/strahe/synapse-go/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/strahe/synapse-go)](https://github.com/strahe/synapse-go/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/strahe/synapse-go.svg)](https://pkg.go.dev/github.com/strahe/synapse-go)
[![Go Report Card](https://goreportcard.com/badge/github.com/strahe/synapse-go)](https://goreportcard.com/report/github.com/strahe/synapse-go)
[![License](https://img.shields.io/github/license/strahe/synapse-go)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.26.3%2B-00ADD8)](go.mod)

Go SDK for Filecoin Onchain Cloud (FOC).

> **Status:** Beta - API may change.

**Docs:** [getting started](docs/GETTING_STARTED.md) |
[API reference](https://pkg.go.dev/github.com/strahe/synapse-go) |
[examples](examples/)

## Install

```bash
go get github.com/strahe/synapse-go
```

Requires Go 1.26.3+.

## Quick Start

```go
client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex("0x..."),
    synapse.WithRPCURL("https://api.calibration.node.glif.io/rpc/v1"),
    synapse.WithSource("my-app"),
)
if err != nil { return err }
defer client.Close()

// file is an io.Reader with 127 bytes to about 1 GiB of data.
upload, err := client.Storage().Upload(ctx, file, &storage.UploadOptions{Copies: 2})
if err != nil { return err }

fmt.Println("piece:", upload.PieceCID)
fmt.Printf("copies: %d/%d\n", upload.SuccessCount(), upload.RequestedCopies)
fmt.Println("retrieve:", upload.Copies[0].RetrievalURL)
```

Load the key from your config or secret manager. Mainnet and Calibration are
detected from the RPC endpoint. Uploads wait about 3 seconds before committing
so concurrent uploads can share a transaction; see
[Commit Batching](docs/GETTING_STARTED.md#commit-batching) to change this.

## Package Map

| Package | Purpose |
|---------|---------|
| `synapse` | Root client that initializes chain config, contract addresses, and services |
| `storage` | Multi-provider upload/download orchestration, dataset discovery, and prepare flows |
| `payments` | USDFC balances, deposits, withdrawals, approvals, and Filecoin Pay rails |
| `costs` | Storage pricing, lockup, runway, and funding cost calculations |
| `warmstorage` | FWSS datasets, pricing, approved-provider discovery, and termination |
| `spregistry` | Storage provider registry discovery and provider/product management |
| `sessionkey` | Delegated session key authorization for FWSS EIP-712 operations |
| `chain` | Filecoin chain IDs, contract addresses, epochs, and token units |
| `signer` | Secp256k1 and BLS signing abstractions |
| `piece` | PieceCID v1/v2 calculation, parsing, and validation |
| `filbeam` | FilBeam egress quota, usage stats, and CDN retrieval for FWSS datasets |
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
