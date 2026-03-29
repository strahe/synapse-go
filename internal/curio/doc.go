// Package curio provides an HTTP client for the Curio storage provider
// PDP API consumed by Filecoin Warm Storage clients.
//
// The SDK side is unauthenticated at the HTTP layer (curio uses NullAuth
// for public PDP endpoints); authorization for state-changing calls is
// carried inside the request body as an EIP-712 signed extraData blob,
// produced by internal/typeddata and warmstorage/payments.
//
// Endpoints covered:
//
//   - GET    /piece/{pieceCid}                              (download bytes)
//   - GET    /pdp/ping
//   - POST   /pdp/piece                                    (pre-register)
//   - PUT    /pdp/piece/upload/{uploadUUID}                (upload bytes)
//   - GET    /pdp/piece?pieceCid=...                       (find)
//   - POST   /pdp/data-sets                                (create)
//   - GET    /pdp/data-sets/created/{txHash}               (poll create)
//   - GET    /pdp/data-sets/{id}                           (read)
//   - POST   /pdp/data-sets/{id}/pieces                    (add pieces)
//   - GET    /pdp/data-sets/{id}/pieces/added/{txHash}     (poll add)
//   - DELETE /pdp/data-sets/{id}/pieces/{pieceId}          (schedule remove)
package curio
