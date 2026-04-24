.PHONY: build test bench lint vet generate generate-contracts clean fmt tidy check test-integration test-integration-cross

# Default target
all: check

# Build all packages
build:
	go build ./...

# Run all tests
test:
	go test ./...

# Run benchmarks — auto-discovers packages containing *_bench_test.go files
bench:
	go test -bench=. -benchmem $(shell find . -name '*_bench_test.go' | sed 's|/[^/]*$$||' | sort -u)

# Run tests with race detector
test-race:
	go test -race ./...

# Run tests with coverage
test-cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run the full integration suite (requires env vars).
# -tags adds integration-only files to the build.
# -run restricts execution to integration entry points so unit tests do not also run.
# -p 1 is required when using a single shared wallet, otherwise package-level
# parallelism races on FEVM nonces and causes mpool conflicts.
test-integration:
	go test -tags=integration -run '^TestIntegration' -p 1 -count=1 -v -timeout 60m ./...

# Run only the cross-package integration flows under tests/integration.
test-integration-cross:
	go test -tags=integration -run '^TestIntegration$$' -count=1 -v -timeout 20m ./tests/integration

# Run linter
lint:
	golangci-lint run ./...

# Run go vet
vet:
	go vet ./...

# Format code
fmt:
	gofumpt -extra -w .

# Tidy modules
tidy:
	go mod tidy

# Generate code (contract bindings, etc.)
generate-contracts:
	go run ./internal/contracts/cmd/syncabi
	go generate ./internal/contracts/...

generate:
	$(MAKE) generate-contracts

# Run all checks (build + vet + lint + test)
check: build vet lint test

# Clean build artifacts
clean:
	rm -f coverage.out coverage.html
	go clean ./...
