# synapse-go

Go SDK for Filecoin Onchain Cloud (FOC), ported from the [@filoz/synapse-sdk](https://github.com/FilOzone/synapse-sdk).

> **Status:** Alpha — API may change.

## Install

```bash
go get github.com/strahe/synapse-go
```

Requires Go 1.25+.

## Quick Start

```go
client, err := synapse.New(ctx,
    synapse.WithPrivateKeyHex(os.Getenv("PRIVATE_KEY")),
    synapse.WithRPCURL(os.Getenv("RPC_URL")),
)
if err != nil { return err }
defer func() { _ = client.Close() }()

// file is any io.Reader
result, err := client.Storage().Upload(ctx, file, &storage.UploadOptions{Copies: 2})
```

Chain is auto-detected from the RPC endpoint. See [`examples/`](examples/) for runnable programs.

## Packages

| Package | Purpose |
|---------|---------|
| `synapse` | Root client — composes all services via `synapse.New()` |
| `storage` | Multi-copy upload / download orchestration |
| `payments` | USDFC balance, deposit, withdraw, ERC-20 approval |
| `costs` | Upload cost estimation |
| `warmstorage` | FWSS on-chain reads |
| `spregistry` | Provider registry + selection |
| `sessionkey` | Delegated session key management |
| `chain` | Chain config, addresses, epoch utilities |
| `signer` | Secp256k1 / BLS signing |
| `piece` | PieceCID v1/v2 calculation and validation |
| `filbeam` | FilBeam CDN statistics |

## Testing

```bash
make test                  # Unit tests
make test-integration      # Full integration suite (Calibration)
make test-integration-cross # High-level cross-package flows
```

Integration tests require `INTEGRATION_PRIVATE_KEY` in `.env` (needs **tFIL** for gas + **5 USDFC**).

`INTEGRATION_RPC_URL` is optional.

## Development

```bash
make check   # build + vet + lint + test
```

## License

[MIT](LICENSE)
