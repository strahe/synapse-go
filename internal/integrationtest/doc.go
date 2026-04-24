// Package integrationtest provides small helpers shared by the SDK's
// integration tests (files built under `//go:build integration`).
//
// It is intentionally minimal and has no external dependencies beyond
// stdlib and the SDK itself. Callers wire helpers through t.Skip when a
// required environment variable is missing, so individual test files do
// not need to duplicate the same boilerplate across packages.
//
// This package itself does NOT carry the `integration` build tag — only
// the test files that import it do. That way `go build ./...` continues
// to compile the helpers as ordinary code.
//
// # Running the integration suite
//
// The per-package integration tests and tests/integration/ all operate
// against the same on-chain wallet (INTEGRATION_PRIVATE_KEY), so their
// transactions must be serialised to avoid Filecoin mempool nonce
// collisions. Always invoke `go test` with `-p 1` so packages run one
// at a time:
//
//	go test -tags=integration -p 1 -run '^TestIntegration' -timeout 60m ./...
//
// The repository Makefile encodes this as `make test-integration`. Use
// `make test-integration-cross` only when you specifically want the
// cross-package `./tests/integration` flows rather than the full
// per-package integration coverage.
//
// `-run '^TestIntegration'` restricts the selection to the integration
// entry points and skips the fast unit tests in the same packages; drop
// it if you want the full test set in a single invocation. `-tags` is
// additive to the normal test build — unit tests without the tag always
// compile and run, which is standard Go behaviour.
package integrationtest
