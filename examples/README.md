# Synapse Go Examples

Set the environment:

```bash
export SYNAPSE_PRIVATE_KEY=0x...
# Optional. Defaults to the public Calibration RPC endpoint.
export SYNAPSE_RPC_URL=https://api.calibration.node.glif.io/rpc/v1
# Optional. Use calibration or mainnet. Empty means auto-detect from RPC.
export SYNAPSE_CHAIN=calibration
```

Examples default to allowing private / local network downloads so local proxy
environments can run retrieval flows. The SDK's normal client default still
rejects private-network downloads unless callers explicitly opt in.

## Quickstart

Upload, download, and verify a small test payload:

```bash
go run ./examples/quickstart
```

Upload a file instead:

```bash
go run ./examples/quickstart --file ./payload.bin
```

## Storage Workflows

Upload a file and print the IDs and retrieval URLs needed by later workflows:

```bash
go run ./examples/upload-file --file ./payload.bin --copies 2
```

Download and validate a piece:

```bash
go run ./examples/download-piece --piece-cid <piece-cid> --url <retrieval-url> --out payload.bin
```

List datasets and the current storage account view:

```bash
go run ./examples/list-datasets --managed
```

Discover active PDP providers:

```bash
go run ./examples/list-providers --piece-size 1048576
```

## Local Utilities

Inspect Filecoin piece commitment information without RPC or a private key:

```bash
go run ./examples/piece-info --file ./payload.bin
```

This prints PieceCIDv1, PieceCIDv2 when the payload is large enough, raw size,
padded size, and the commitment root.

Full API documentation: <https://pkg.go.dev/github.com/strahe/synapse-go>.
