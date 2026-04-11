.PHONY: build test bench lint vet generate generate-contracts clean fmt tidy check

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

# Run integration tests (requires env vars)
test-integration:
	go test -tags=integration -count=1 -v -timeout 20m ./tests/integration

# Run linter
lint:
	golangci-lint run ./...

# Run go vet
vet:
	go vet ./...

# Format code
fmt:
	gofumpt -w .

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
