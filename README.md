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

**Docs:** [API reference](https://pkg.go.dev/github.com/strahe/synapse-go) |
[examples](examples/)

## Install

```bash
go get github.com/strahe/synapse-go
```

Requires Go 1.25+.

## Quick Start

```go
client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex(os.Getenv("SYNAPSE_PRIVATE_KEY")),
    synapse.WithRPCURL(os.Getenv("SYNAPSE_RPC_URL")),
)
if err != nil { return err }
defer func() { _ = client.Close() }()

// file is an io.Reader over the payload to upload.
upload, err := client.Storage().Upload(ctx, file, &storage.UploadOptions{Copies: 2})
if err != nil { return err }

fmt.Println("piece:", upload.PieceCID)
fmt.Printf("copies: %d/%d\n", upload.SuccessCount(), upload.RequestedCopies)
fmt.Println("retrieve:", upload.Copies[0].RetrievalURL)
```

Set `SYNAPSE_PRIVATE_KEY` and `SYNAPSE_RPC_URL`; the chain is detected automatically.

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

## Testing

CI covers build, vet, lint, tests, and govulncheck.

```bash
make test                   # Unit tests
make test-integration       # Full integration suite (Calibration)
make test-integration-cross # High-level cross-package flows
```

Integration tests require `INTEGRATION_PRIVATE_KEY` in `.env` (needs **tFIL** for gas + **5 USDFC**).

`INTEGRATION_RPC_URL` is optional.

## Development

```bash
make check   # build + vet + lint + test
```

## License

[Apache-2.0](LICENSE)
