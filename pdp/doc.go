// Package pdp provides a Curio-compatible PDP provider HTTP client.
//
// PDP endpoints are unauthenticated at the HTTP layer. Authorization for
// state-changing calls is carried inside the request body as caller-provided
// EIP-712 signed extraData.
//
// # Retry policy
//
// Control-plane GET requests (getJSON and poll endpoints) retry on
// transient errors — HTTP 5xx (except 501), 429, connection resets,
// DNS temporaries, unexpected EOF, and request timeouts — with
// exponential backoff up to MaxRetries.
//
// Streaming piece downloads (DownloadPiece) are executed once with the
// caller's context as the sole lifetime control; they do not go through
// the automatic retry loop.
// Non-success PDP HTTP responses surface *HTTPError. Use
// errors.AsType[*pdp.HTTPError] to inspect status code, response body, and
// Retry-After.
//
// POST and DELETE requests are executed exactly once. These verbs may
// mutate server state, and a client-side retry after a server-side
// partial success can cause duplicate work or inconsistent state.
// Callers that need retry behavior for POST/DELETE must implement it
// at the business layer with appropriate deduplication (e.g. by
// polling the resulting resource via a GET endpoint before retrying).
//
// # Response size cap
//
// Control-plane JSON responses are capped at MaxControlResponseBytes
// (16 MiB). Anything larger is treated as a server bug or attack and
// fails the request. Streaming endpoints (piece download) are not
// subject to this cap.
//
// Endpoints covered:
//
//   - GET    /piece/{pieceCid}                              (download bytes)
//   - GET    /pdp/ping
//   - POST   /pdp/piece/uploads                           (create upload)
//   - PUT    /pdp/piece/uploads/{uploadUUID}               (upload bytes)
//   - POST   /pdp/piece/uploads/{uploadUUID}               (finalize upload)
//   - GET    /pdp/piece?pieceCid=...                       (find)
//   - POST   /pdp/piece/pull                              (pull pieces)
//   - POST   /pdp/data-sets                                (create)
//   - GET    /pdp/data-sets/created/{txHash}               (poll create)
//   - GET    /pdp/data-sets/{id}                           (read)
//   - POST   /pdp/data-sets/{id}/pieces                    (add pieces)
//   - GET    /pdp/data-sets/{id}/pieces/added/{txHash}     (poll add)
//   - DELETE /pdp/data-sets/{id}/pieces/{pieceId}          (schedule remove)
package pdp
