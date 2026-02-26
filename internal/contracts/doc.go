// Package contracts contains go:generate directives for regenerating
// smart contract bindings from ABI JSON files using abigen.
//
// To regenerate all bindings:
//
//	go generate ./internal/contracts/...
//
// Prerequisites:
//   - abigen (from go-ethereum: go install github.com/ethereum/go-ethereum/cmd/abigen@latest)
//   - ABI JSON files in each sub-package
package contracts
